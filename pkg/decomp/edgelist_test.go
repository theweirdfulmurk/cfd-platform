package decomp_test

import (
	"path/filepath"
	"testing"

	"github.com/theweirdfulmurk/cfd-platform/pkg/decomp"
)

func TestEdgeList_DenseFourRank(t *testing.T) {
	g, err := decomp.EdgeList(filepath.Join("testdata", "edgelist_case", "graph.edgelist"))
	if err != nil {
		t.Fatalf("EdgeList parse failed: %v", err)
	}
	if g.NumRanks != 4 {
		t.Errorf("NumRanks = %d, want 4", g.NumRanks)
	}
	if len(g.Edges) != 6 {
		t.Errorf("got %d edges, want 6: %+v", len(g.Edges), g.Edges)
	}
	want := map[decomp.Pair]float64{
		{I: 0, J: 1}: 100,
		{I: 0, J: 2}: 50,
		{I: 0, J: 3}: 25,
		{I: 1, J: 2}: 80,
		{I: 1, J: 3}: 60,
		{I: 2, J: 3}: 90,
	}
	for _, e := range g.Edges {
		if w, ok := want[e.Pair]; !ok || w != e.Weight {
			t.Errorf("edge %+v: weight = %v, want %v (ok=%v)", e.Pair, e.Weight, w, ok)
		}
	}
}

func TestEdgeList_DuplicatesAreSummed(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "g"), "3\n0 1 5\n1 0 3\n0 2 7\n")
	g, err := decomp.EdgeList(filepath.Join(dir, "g"))
	if err != nil {
		t.Fatal(err)
	}
	got := map[decomp.Pair]float64{}
	for _, e := range g.Edges {
		got[e.Pair] = e.Weight
	}
	if got[decomp.Pair{I: 0, J: 1}] != 8 {
		t.Errorf("pair {0,1} weight = %v, want 8 (5+3)", got[decomp.Pair{I: 0, J: 1}])
	}
	if got[decomp.Pair{I: 0, J: 2}] != 7 {
		t.Errorf("pair {0,2} weight = %v, want 7", got[decomp.Pair{I: 0, J: 2}])
	}
}

func TestEdgeList_RejectsMalformed(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "g"), "3\n0 1\n")
	if _, err := decomp.EdgeList(filepath.Join(dir, "g")); err == nil {
		t.Fatal("expected error for malformed edge line")
	}
}

func TestEdgeList_RequiresHeader(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "g"), "# just comments\n")
	if _, err := decomp.EdgeList(filepath.Join(dir, "g")); err == nil {
		t.Fatal("expected error for missing NumRanks header")
	}
}
