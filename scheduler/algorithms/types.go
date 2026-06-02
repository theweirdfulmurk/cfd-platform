// Package algorithms holds the QAP placement algorithms used by the
// topology-aware scheduler: a streaming greedy (online, O(n²)) and a
// Müller-Merbach offline heuristic with cascading reassignments (O(n³)).
//
// Both consume the same input — communication graph F(i,j) between MPI
// ranks and latency matrix L(a,b) between cluster nodes — and produce
// the same output — a placement π: rank → node minimising
//
//     Σᵢⱼ F(i,j) · L(π(i), π(j)).
package algorithms

import (
	"fmt"

	"github.com/theweirdfulmurk/cfd-platform/pkg/decomp"
)

// Placement maps an MPI rank to a cluster node index. Length N (one entry
// per rank). Placement[i] == -1 means "rank i not assigned yet" (used by
// the streaming greedy mid-job).
type Placement []int

// LatencyMatrix is a symmetric M×M matrix of pairwise node latencies. Zero
// diagonal. Index 0..M-1 enumerates available cluster nodes.
type LatencyMatrix [][]float64

// Input bundles everything an algorithm needs.
type Input struct {
	F        *decomp.Graph // communication graph; F.NumRanks = N
	L        LatencyMatrix // M×M latency matrix
	NumNodes int           // M; must equal len(L)
}

// Validate checks shape invariants.
func (in Input) Validate() error {
	if in.F == nil {
		return fmt.Errorf("nil communication graph")
	}
	if in.NumNodes < in.F.NumRanks {
		return fmt.Errorf("not enough nodes (%d) for %d ranks", in.NumNodes, in.F.NumRanks)
	}
	if len(in.L) != in.NumNodes {
		return fmt.Errorf("latency matrix has %d rows, want %d", len(in.L), in.NumNodes)
	}
	for i, row := range in.L {
		if len(row) != in.NumNodes {
			return fmt.Errorf("latency matrix row %d has %d cols, want %d",
				i, len(row), in.NumNodes)
		}
		if row[i] != 0 {
			return fmt.Errorf("latency matrix diagonal[%d] = %v, want 0", i, row[i])
		}
	}
	return nil
}

// Cost returns Σᵢⱼ F(i,j) · L(π(i), π(j)) for a complete placement. Unset
// ranks (-1) are skipped — useful for partial placements mid-stream.
func Cost(in Input, p Placement) float64 {
	if in.F == nil {
		return 0
	}
	matrix := in.F.Matrix()
	total := 0.0
	for i := 0; i < in.F.NumRanks; i++ {
		ni := p[i]
		if ni < 0 {
			continue
		}
		for j := i + 1; j < in.F.NumRanks; j++ {
			nj := p[j]
			if nj < 0 {
				continue
			}
			total += matrix[i][j] * in.L[ni][nj]
		}
	}
	return total
}
