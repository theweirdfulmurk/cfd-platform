package decomp_test

import (
	"path/filepath"
	"testing"

	"github.com/theweirdfulmurk/cfd-platform/pkg/decomp"
)

func TestMETIS_SmallCase(t *testing.T) {
	graph := filepath.Join("testdata", "metis_case", "mesh.graph")
	part := filepath.Join("testdata", "metis_case", "mesh.part.3")

	g, err := decomp.METIS(graph, part)
	if err != nil {
		t.Fatalf("METIS parse failed: %v", err)
	}
	if g.NumRanks != 3 {
		t.Errorf("NumRanks = %d, want 3", g.NumRanks)
	}

	// Expected from the fixture:
	//   vertex partitions: [0,0,1,1,2,2]
	//   edges that cross partitions:
	//     2-3 w=5   → pair {0,1}: 5
	//     4-5 w=7   → pair {1,2}: 7
	//     1-5 w=3   → pair {0,2}: 3
	want := map[decomp.Pair]float64{
		{I: 0, J: 1}: 5,
		{I: 0, J: 2}: 3,
		{I: 1, J: 2}: 7,
	}
	if len(g.Edges) != len(want) {
		t.Fatalf("got %d edges, want %d: %+v", len(g.Edges), len(want), g.Edges)
	}
	for _, e := range g.Edges {
		w, ok := want[e.Pair]
		if !ok {
			t.Errorf("unexpected edge %+v", e)
			continue
		}
		if e.Weight != w {
			t.Errorf("edge %+v: weight = %v, want %v", e.Pair, e.Weight, w)
		}
	}
}

func TestOpenRadiossAliasWorks(t *testing.T) {
	graph := filepath.Join("testdata", "metis_case", "mesh.graph")
	part := filepath.Join("testdata", "metis_case", "mesh.part.3")
	if _, err := decomp.OpenRadioss(graph, part); err != nil {
		t.Fatalf("OpenRadioss wrapper failed: %v", err)
	}
}

func TestCodeAsterAliasWorks(t *testing.T) {
	graph := filepath.Join("testdata", "metis_case", "mesh.graph")
	part := filepath.Join("testdata", "metis_case", "mesh.part.3")
	if _, err := decomp.CodeAster(graph, part); err != nil {
		t.Fatalf("CodeAster wrapper failed: %v", err)
	}
}

func TestMETIS_VertexWeightsFormat(t *testing.T) {
	// Format that OpenRadioss writes when IDB_METIS=1: fmt=010, ncon=1.
	// The parser must skip the vertex weight column and still count edges.
	graph := filepath.Join("testdata", "metis_vweight_case", "mesh.graph")
	part := filepath.Join("testdata", "metis_vweight_case", "mesh.part.2")

	g, err := decomp.METIS(graph, part)
	if err != nil {
		t.Fatalf("METIS parse failed: %v", err)
	}
	if g.NumRanks != 2 {
		t.Errorf("NumRanks = %d, want 2", g.NumRanks)
	}
	// Partitions: vertices [1,2] -> 0, [3,4] -> 1.
	// Cross-partition edges: 1-3 and 2-4, each default weight 1.0.
	// Pair {0,1}: 1 + 1 = 2.
	want := map[decomp.Pair]float64{
		{I: 0, J: 1}: 2,
	}
	if len(g.Edges) != len(want) {
		t.Fatalf("got %d edges, want %d: %+v", len(g.Edges), len(want), g.Edges)
	}
	for _, e := range g.Edges {
		if w := want[e.Pair]; w != e.Weight {
			t.Errorf("edge %+v: weight = %v, want %v", e.Pair, e.Weight, w)
		}
	}
}

func TestMETIS_VertexCountMismatch(t *testing.T) {
	// Build a graph with more vertices than the partition file has lines.
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "g"), "4 2 001\n2 1 3 1\n1 1\n4 1\n3 1\n")
	writeFile(t, filepath.Join(dir, "p"), "0\n1\n")

	_, err := decomp.METIS(filepath.Join(dir, "g"), filepath.Join(dir, "p"))
	if err == nil {
		t.Fatal("expected size mismatch error")
	}
}
