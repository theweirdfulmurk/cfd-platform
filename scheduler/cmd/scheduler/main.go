// Topology-aware MPI scheduler — implemented as a Kubernetes Scheduler
// Extender. kube-scheduler is left untouched; it talks HTTP to this
// process whenever a pod labelled `mpi-job-id=...` arrives at the
// Prioritize phase.
//
// Configuration (mounted from a ConfigMap):
//
//   /etc/scheduler/latency.yaml   — node-to-node round-trip times
//   /etc/scheduler/graphs/<jobID>.edgelist — per-job F(i,j) (optional;
//                                              the backend pushes these
//                                              when it parses solver
//                                              decomposition output)
//
// The /prioritize endpoint follows the upstream KubeSchedulerConfiguration
// extender API (see k8s/scheduler-config.yaml in the repo).
package main

import (
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/theweirdfulmurk/cfd-platform/pkg/decomp"
	"github.com/theweirdfulmurk/cfd-platform/scheduler/algorithms"
	"github.com/theweirdfulmurk/cfd-platform/scheduler/plugin"
)

func main() {
	addr := envOr("LISTEN_ADDR", ":8090")
	latencyPath := envOr("LATENCY_PATH", "/etc/scheduler/latency.yaml")
	graphsDir := envOr("GRAPHS_DIR", "/etc/scheduler/graphs")

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	ext := plugin.NewExtender(logger)

	if err := loadLatency(ext, latencyPath); err != nil {
		logger.Warn("latency load failed", "path", latencyPath, "err", err)
	}
	loadGraphs(ext, graphsDir, logger)

	mux := http.NewServeMux()
	mux.HandleFunc("/prioritize", ext.PrioritizeHandler)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	logger.Info("extender starting", "addr", addr, "graphsDir", graphsDir)
	if err := http.ListenAndServe(addr, mux); err != nil {
		logger.Error("server crashed", "err", err)
		os.Exit(1)
	}
}

// loadLatency parses a simple "node1 node2 rtt_ms" file and installs
// the resulting matrix on the extender.
//
// We deliberately use a tiny line-oriented format (not YAML) to keep
// the extender free of YAML deps; the backend writes this file from
// node labels.
func loadLatency(ext *plugin.Extender, path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	nodes := []string{}
	idx := map[string]int{}
	edges := map[[2]int]float64{}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 3 {
			return fmt.Errorf("malformed latency line: %q", line)
		}
		a := ensureIndex(fields[0], &nodes, idx)
		b := ensureIndex(fields[1], &nodes, idx)
		var rtt float64
		if _, err := fmt.Sscanf(fields[2], "%f", &rtt); err != nil {
			return fmt.Errorf("parse rtt %q: %w", fields[2], err)
		}
		edges[[2]int{a, b}] = rtt
	}
	n := len(nodes)
	L := make(algorithms.LatencyMatrix, n)
	for i := range L {
		L[i] = make([]float64, n)
	}
	for pair, rtt := range edges {
		L[pair[0]][pair[1]] = rtt
		L[pair[1]][pair[0]] = rtt
	}
	ext.SetLatency(nodes, L)
	return nil
}

func ensureIndex(name string, nodes *[]string, idx map[string]int) int {
	if i, ok := idx[name]; ok {
		return i
	}
	i := len(*nodes)
	*nodes = append(*nodes, name)
	idx[name] = i
	return i
}

// loadGraphs reads every <jobID>.edgelist file in graphsDir and registers
// the corresponding F(i,j) graph with the extender. The backend pushes
// new files into this dir; for a hot-reload story we would add an fsnotify
// watcher — left as a TODO since the experiment fleet is small.
func loadGraphs(ext *plugin.Extender, dir string, logger *slog.Logger) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		logger.Warn("graphs dir unreadable", "dir", dir, "err", err)
		return
	}
	for _, ent := range entries {
		if ent.IsDir() || !strings.HasSuffix(ent.Name(), ".edgelist") {
			continue
		}
		jobID := strings.TrimSuffix(ent.Name(), ".edgelist")
		g, err := decomp.EdgeList(filepath.Join(dir, ent.Name()))
		if err != nil {
			logger.Warn("graph load failed", "file", ent.Name(), "err", err)
			continue
		}
		ext.SetGraph(jobID, g)
		logger.Info("graph loaded", "jobID", jobID, "ranks", g.NumRanks, "edges", len(g.Edges))
	}
}

func envOr(key, dflt string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return dflt
}
