// Package plugin implements the topology-aware scheduler logic exposed
// as a Kubernetes Scheduler Extender.
//
// We chose the Extender route over the in-tree Score Plugin route for two
// pragmatic reasons:
//
//   * In-tree plugins require importing k8s.io/kubernetes/cmd/kube-scheduler,
//     which pins ~50 staging packages at v0.0.0 in its go.mod — a Go
//     module hazard that the upstream scheduler-plugins project mitigates
//     with a long list of replace directives. For a thesis-scale codebase
//     the cost of replicating that machinery is not justified.
//
//   * The Extender API is a stable, documented extension point of
//     kube-scheduler itself; it speaks JSON over HTTP and lets us run
//     the topology-aware logic in a separate pod (own resources, own
//     restart cycle, easier debugging).
//
// The HTTP contract follows the upstream
// k8s.io/kube-scheduler/extender/v1 types: /prioritize is called with a
// pod and the surviving node candidates and must return a HostPriority
// list (one score per node).
package plugin

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"

	v1 "k8s.io/api/core/v1"
	extenderv1 "k8s.io/kube-scheduler/extender/v1"

	"github.com/theweirdfulmurk/cfd-platform/pkg/decomp"
	"github.com/theweirdfulmurk/cfd-platform/scheduler/algorithms"
)

const (
	// LabelMPIJobID groups all pods of one MPI job. Set by the backend
	// when constructing the MPIJob spec.
	LabelMPIJobID = "mpi-job-id"

	// MaxPriority is the canonical extender score ceiling.
	MaxPriority = int64(10)
)

// Extender holds the in-memory state of the topology-aware scheduler.
// One instance per process; concurrent /prioritize requests are safe.
type Extender struct {
	mu        sync.RWMutex
	jobGraphs map[string]*decomp.Graph
	latency   algorithms.LatencyMatrix
	nodeIndex map[string]int // node name → row in latency matrix

	// placements caches the per-job decision of "rank r → node name".
	// The extender is called per pod; we accumulate to inform Greedy.
	placements map[string]map[int]string

	logger *slog.Logger
}

func NewExtender(logger *slog.Logger) *Extender {
	if logger == nil {
		logger = slog.Default()
	}
	return &Extender{
		jobGraphs:  make(map[string]*decomp.Graph),
		nodeIndex:  make(map[string]int),
		placements: make(map[string]map[int]string),
		logger:     logger,
	}
}

// SetGraph caches the F(i,j) matrix for a job. Caller is the admission
// controller / backend webhook on MPIJob creation.
func (e *Extender) SetGraph(jobID string, g *decomp.Graph) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.jobGraphs[jobID] = g
}

// SetLatency installs the global L(a,b) matrix and node name → index map.
func (e *Extender) SetLatency(nodes []string, L algorithms.LatencyMatrix) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.latency = L
	e.nodeIndex = make(map[string]int, len(nodes))
	for i, n := range nodes {
		e.nodeIndex[n] = i
	}
}

// PrioritizeHandler implements the /prioritize HTTP endpoint.
//
// Request:  ExtenderArgs { Pod, Nodes | NodeNames }
// Response: HostPriorityList — score [0..MaxPriority] per node, larger=better.
func (e *Extender) PrioritizeHandler(w http.ResponseWriter, r *http.Request) {
	var args extenderv1.ExtenderArgs
	if err := json.NewDecoder(r.Body).Decode(&args); err != nil {
		http.Error(w, fmt.Sprintf("decode args: %v", err), http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	pod := args.Pod
	if pod == nil {
		http.Error(w, "missing pod", http.StatusBadRequest)
		return
	}
	jobID, ok := pod.Labels[LabelMPIJobID]
	if !ok {
		e.respondUniform(w, args)
		return
	}

	e.mu.RLock()
	graph := e.jobGraphs[jobID]
	latency := e.latency
	nodeIdx := e.nodeIndex
	placements := e.placements[jobID]
	e.mu.RUnlock()

	if graph == nil || len(latency) == 0 {
		e.respondUniform(w, args)
		return
	}

	thisRank, err := rankFromPod(pod, graph.NumRanks)
	if err != nil {
		http.Error(w, fmt.Sprintf("derive rank: %v", err), http.StatusBadRequest)
		return
	}

	nodeNames := nodeNamesFromArgs(args)
	scores := e.scoreNodes(graph, latency, nodeIdx, placements, thisRank, nodeNames)

	// Record the chosen node (highest score) into placements cache.
	if best := bestNode(scores); best != "" {
		e.mu.Lock()
		if e.placements[jobID] == nil {
			e.placements[jobID] = make(map[int]string)
		}
		e.placements[jobID][thisRank] = best
		e.mu.Unlock()
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(scores); err != nil {
		e.logger.Error("encode response", "err", err)
	}
}

func (e *Extender) scoreNodes(
	graph *decomp.Graph,
	latency algorithms.LatencyMatrix,
	nodeIdx map[string]int,
	placements map[int]string,
	thisRank int,
	nodeNames []string,
) extenderv1.HostPriorityList {
	matrix := graph.Matrix()

	costs := make([]float64, len(nodeNames))
	maxCost := 0.0
	for i, name := range nodeNames {
		idx, known := nodeIdx[name]
		if !known {
			costs[i] = -1 // sentinel: penalise unknown nodes
			continue
		}
		c := 0.0
		for peerRank, peerNode := range placements {
			if peerRank == thisRank {
				continue
			}
			peerIdx, ok := nodeIdx[peerNode]
			if !ok {
				continue
			}
			c += matrix[thisRank][peerRank] * latency[idx][peerIdx]
		}
		costs[i] = c
		if c > maxCost {
			maxCost = c
		}
	}

	out := make(extenderv1.HostPriorityList, len(nodeNames))
	for i, name := range nodeNames {
		var score int64
		switch {
		case costs[i] < 0:
			score = 0
		case maxCost == 0:
			score = MaxPriority
		default:
			score = MaxPriority - int64(costs[i]*float64(MaxPriority)/maxCost)
			if score < 0 {
				score = 0
			}
		}
		out[i] = extenderv1.HostPriority{Host: name, Score: score}
	}
	return out
}

// respondUniform answers with equal score so kube-scheduler falls back to
// its own decision (used when we have no data for the job).
func (e *Extender) respondUniform(w http.ResponseWriter, args extenderv1.ExtenderArgs) {
	names := nodeNamesFromArgs(args)
	out := make(extenderv1.HostPriorityList, len(names))
	for i, n := range names {
		out[i] = extenderv1.HostPriority{Host: n, Score: MaxPriority / 2}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

func nodeNamesFromArgs(args extenderv1.ExtenderArgs) []string {
	if args.NodeNames != nil {
		return *args.NodeNames
	}
	if args.Nodes != nil {
		out := make([]string, 0, len(args.Nodes.Items))
		for _, n := range args.Nodes.Items {
			out = append(out, n.Name)
		}
		return out
	}
	return nil
}

func bestNode(scores extenderv1.HostPriorityList) string {
	var best string
	var bestScore int64 = -1
	for _, s := range scores {
		if s.Score > bestScore {
			bestScore = s.Score
			best = s.Host
		}
	}
	return best
}

// rankFromPod tries the `mpi-rank` annotation first; falls back to the
// trailing integer of the pod name (`sim-XYZ-worker-3` → 3).
func rankFromPod(pod *v1.Pod, numRanks int) (int, error) {
	if v, ok := pod.Annotations["mpi-rank"]; ok {
		if r, err := strconv.Atoi(v); err == nil && r >= 0 && r < numRanks {
			return r, nil
		}
	}
	return rankFromPodName(pod.Name)
}

func rankFromPodName(name string) (int, error) {
	idx := strings.LastIndex(name, "-")
	if idx < 0 {
		return 0, fmt.Errorf("no rank suffix in pod name %q", name)
	}
	r, err := strconv.Atoi(name[idx+1:])
	if err != nil {
		return 0, fmt.Errorf("parse rank from %q: %w", name, err)
	}
	return r, nil
}
