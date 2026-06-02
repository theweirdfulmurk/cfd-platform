package decomp_test

import (
	"testing"

	"github.com/theweirdfulmurk/cfd-platform/pkg/decomp"
)

func TestNewPair(t *testing.T) {
	tests := []struct {
		a, b   int
		wantI  int
		wantJ  int
		wantOK bool
	}{
		{0, 1, 0, 1, true},
		{1, 0, 0, 1, true},
		{5, 3, 3, 5, true},
		{2, 2, 0, 0, false},
	}
	for _, tt := range tests {
		p, err := decomp.NewPair(tt.a, tt.b)
		if (err == nil) != tt.wantOK {
			t.Errorf("NewPair(%d,%d): err=%v, wantOK=%v", tt.a, tt.b, err, tt.wantOK)
			continue
		}
		if !tt.wantOK {
			continue
		}
		if p.I != tt.wantI || p.J != tt.wantJ {
			t.Errorf("NewPair(%d,%d) = {%d,%d}; want {%d,%d}", tt.a, tt.b, p.I, p.J, tt.wantI, tt.wantJ)
		}
	}
}

func TestBuildSortsAndDropsZeros(t *testing.T) {
	g, err := decomp.Build(4, map[decomp.Pair]float64{
		{I: 2, J: 3}: 5,
		{I: 0, J: 1}: 10,
		{I: 1, J: 3}: 0, // dropped
		{I: 0, J: 3}: 3,
	})
	if err != nil {
		t.Fatal(err)
	}
	if g.NumRanks != 4 {
		t.Errorf("NumRanks = %d, want 4", g.NumRanks)
	}
	if len(g.Edges) != 3 {
		t.Fatalf("Edges len = %d, want 3", len(g.Edges))
	}
	want := []decomp.Edge{
		{Pair: decomp.Pair{I: 0, J: 1}, Weight: 10},
		{Pair: decomp.Pair{I: 0, J: 3}, Weight: 3},
		{Pair: decomp.Pair{I: 2, J: 3}, Weight: 5},
	}
	for i, e := range g.Edges {
		if e != want[i] {
			t.Errorf("Edges[%d] = %+v, want %+v", i, e, want[i])
		}
	}
}

func TestBuildRejectsOutOfRange(t *testing.T) {
	_, err := decomp.Build(2, map[decomp.Pair]float64{
		{I: 0, J: 5}: 1,
	})
	if err == nil {
		t.Fatal("expected error for out-of-range edge, got nil")
	}
}

func TestMatrixSymmetry(t *testing.T) {
	g, err := decomp.Build(3, map[decomp.Pair]float64{
		{I: 0, J: 1}: 4,
		{I: 1, J: 2}: 7,
	})
	if err != nil {
		t.Fatal(err)
	}
	m := g.Matrix()
	if m[0][1] != 4 || m[1][0] != 4 {
		t.Errorf("pair {0,1}: got %v / %v, want 4 / 4", m[0][1], m[1][0])
	}
	if m[1][2] != 7 || m[2][1] != 7 {
		t.Errorf("pair {1,2}: got %v / %v, want 7 / 7", m[1][2], m[2][1])
	}
	if m[0][2] != 0 || m[2][0] != 0 {
		t.Errorf("pair {0,2}: got %v / %v, want 0 / 0", m[0][2], m[2][0])
	}
	for i := 0; i < 3; i++ {
		if m[i][i] != 0 {
			t.Errorf("diag[%d] = %v, want 0", i, m[i][i])
		}
	}
}
