package decomp_test

import (
	"path/filepath"
	"testing"

	"github.com/theweirdfulmurk/cfd-platform/pkg/decomp"
)

func TestOpenFOAM_ThreeProcCase(t *testing.T) {
	g, err := decomp.OpenFOAM(filepath.Join("testdata", "openfoam_case"))
	if err != nil {
		t.Fatalf("OpenFOAM parse failed: %v", err)
	}
	if g.NumRanks != 3 {
		t.Errorf("NumRanks = %d, want 3", g.NumRanks)
	}

	want := map[decomp.Pair]float64{
		{I: 0, J: 1}: 12,
		{I: 0, J: 2}: 7,
		{I: 1, J: 2}: 9,
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

func TestOpenFOAM_MissingCase(t *testing.T) {
	_, err := decomp.OpenFOAM(filepath.Join("testdata", "does-not-exist"))
	if err == nil {
		t.Fatal("expected error for missing case dir")
	}
}

func TestOpenFOAM_NoProcDirs(t *testing.T) {
	// docs/examples/cavity has only the un-decomposed mesh — no processorN/ dirs.
	// We expect a clean error pointing at decomposePar.
	_, err := decomp.OpenFOAM(filepath.Join("..", "..", "docs", "examples", "cavity"))
	if err == nil {
		t.Fatal("expected error for case without processor dirs")
	}
}
