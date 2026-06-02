package algorithms_test

import (
	"testing"

	"github.com/theweirdfulmurk/cfd-platform/pkg/decomp"
	"github.com/theweirdfulmurk/cfd-platform/scheduler/algorithms"
)

// threeRankInput builds a synthetic case with one heavy edge (0,1) and a
// latency layout where node 0 and node 1 are co-located (zone A) and
// node 2 is in a remote zone. The optimum places ranks 0 and 1 inside
// the cheap zone and rank 2 on node 2.
func threeRankInput(t *testing.T) algorithms.Input {
	t.Helper()
	g, err := decomp.Build(3, map[decomp.Pair]float64{
		{I: 0, J: 1}: 100, // heavy edge
		{I: 0, J: 2}: 1,
		{I: 1, J: 2}: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	L := algorithms.LatencyMatrix{
		{0, 1, 10},  // node 0 → nodes 0,1,2
		{1, 0, 10},
		{10, 10, 0},
	}
	return algorithms.Input{F: g, L: L, NumNodes: 3}
}

func TestGreedyAll_HeavyEdgeCoLocated(t *testing.T) {
	in := threeRankInput(t)
	if err := in.Validate(); err != nil {
		t.Fatal(err)
	}
	p := algorithms.GreedyAll(in)
	if p == nil {
		t.Fatal("nil placement")
	}
	// We expect ranks 0 and 1 to land on the cheap pair (nodes 0 and 1
	// in some order) — i.e. NOT on node 2.
	if p[0] == 2 || p[1] == 2 {
		t.Errorf("ranks 0,1 placed across the expensive node 2: %v", p)
	}
	if p[2] != 2 {
		t.Errorf("rank 2 expected on node 2, got %v", p)
	}
	cost := algorithms.Cost(in, p)
	t.Logf("greedy placement %v cost=%v", p, cost)
}

func TestMuellerMerbach_HeavyEdgeCoLocated(t *testing.T) {
	in := threeRankInput(t)
	p := algorithms.MuellerMerbach(in)
	if p == nil {
		t.Fatal("nil placement")
	}
	if p[0] == 2 || p[1] == 2 {
		t.Errorf("ranks 0,1 placed across expensive node: %v", p)
	}
	cost := algorithms.Cost(in, p)
	t.Logf("MM placement %v cost=%v", p, cost)
}

func TestGreedy_ExtraNodesUnused(t *testing.T) {
	g, err := decomp.Build(2, map[decomp.Pair]float64{
		{I: 0, J: 1}: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	in := algorithms.Input{
		F: g,
		L: algorithms.LatencyMatrix{
			{0, 1, 5, 5},
			{1, 0, 5, 5},
			{5, 5, 0, 1},
			{5, 5, 1, 0},
		},
		NumNodes: 4,
	}
	p := algorithms.GreedyAll(in)
	if p == nil || len(p) != 2 {
		t.Fatalf("bad placement: %v", p)
	}
	// Both ranks should land on one of the cheap pairs (0,1) or (2,3).
	d := abs(p[0] - p[1])
	if d != 1 {
		t.Errorf("ranks placed on non-adjacent nodes: %v", p)
	}
}

func TestMuellerMerbach_DensePreferences(t *testing.T) {
	// Dense graph: every pair has weight 5 except (0,3) which has 100.
	// MM should co-locate ranks 0 and 3.
	g, err := decomp.Build(4, map[decomp.Pair]float64{
		{I: 0, J: 1}: 5,
		{I: 0, J: 2}: 5,
		{I: 0, J: 3}: 100,
		{I: 1, J: 2}: 5,
		{I: 1, J: 3}: 5,
		{I: 2, J: 3}: 5,
	})
	if err != nil {
		t.Fatal(err)
	}
	// 4 nodes, two zones: (0,1) and (2,3) cheap inside, expensive across.
	in := algorithms.Input{
		F: g,
		L: algorithms.LatencyMatrix{
			{0, 1, 10, 10},
			{1, 0, 10, 10},
			{10, 10, 0, 1},
			{10, 10, 1, 0},
		},
		NumNodes: 4,
	}
	p := algorithms.MuellerMerbach(in)
	if p == nil {
		t.Fatal("nil placement")
	}
	// Ranks 0 and 3 must share a zone (i.e. nodes differing by exactly 1).
	if abs(p[0]-p[3]) != 1 {
		t.Errorf("ranks 0 and 3 should be co-zone; got nodes %d, %d", p[0], p[3])
	}
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
