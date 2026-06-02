// Package decomp parses solver decomposition files and produces a weighted
// communication graph F(i,j) — the input for the topology-aware scheduler.
//
// The graph is the off-diagonal upper triangle of a symmetric matrix:
// entry Weight for pair {I, J} (I < J) is the intensity of MPI exchanges
// between ranks I and J (number of shared boundary faces, nodes, or DoFs,
// depending on the solver).
package decomp

import (
	"errors"
	"fmt"
	"sort"
)

// Pair identifies an unordered communication edge between two MPI ranks.
// Invariant: I < J.
type Pair struct {
	I, J int
}

// Edge is one entry of the communication graph.
type Edge struct {
	Pair
	Weight float64
}

// Graph holds the result of parsing a single decomposition artefact.
type Graph struct {
	NumRanks int
	Edges    []Edge
}

// NewPair returns a canonical Pair with I < J.
func NewPair(a, b int) (Pair, error) {
	if a == b {
		return Pair{}, fmt.Errorf("decomp: self-loop not allowed (rank %d)", a)
	}
	if a < b {
		return Pair{I: a, J: b}, nil
	}
	return Pair{I: b, J: a}, nil
}

// Build constructs a Graph from rank count and per-edge weights.
// Duplicate pairs are summed; pairs with zero weight are dropped.
// Edges are returned sorted by (I, J).
func Build(numRanks int, weights map[Pair]float64) (*Graph, error) {
	if numRanks < 1 {
		return nil, fmt.Errorf("decomp: numRanks must be >= 1, got %d", numRanks)
	}
	edges := make([]Edge, 0, len(weights))
	for p, w := range weights {
		if p.I < 0 || p.J >= numRanks {
			return nil, fmt.Errorf("decomp: edge {%d,%d} out of range [0,%d)", p.I, p.J, numRanks)
		}
		if w == 0 {
			continue
		}
		edges = append(edges, Edge{Pair: p, Weight: w})
	}
	sort.Slice(edges, func(i, j int) bool {
		if edges[i].I != edges[j].I {
			return edges[i].I < edges[j].I
		}
		return edges[i].J < edges[j].J
	})
	return &Graph{NumRanks: numRanks, Edges: edges}, nil
}

// Matrix returns F as a dense N×N symmetric matrix with zero diagonal.
// Useful for QAP solvers that expect a matrix form.
func (g *Graph) Matrix() [][]float64 {
	n := g.NumRanks
	m := make([][]float64, n)
	for i := range m {
		m[i] = make([]float64, n)
	}
	for _, e := range g.Edges {
		m[e.I][e.J] = e.Weight
		m[e.J][e.I] = e.Weight
	}
	return m
}

// ErrEmpty is returned when a parser finds no inter-rank edges.
// This is usually a hint that the wrong file/directory was supplied.
var ErrEmpty = errors.New("decomp: no inter-rank communication edges found")
