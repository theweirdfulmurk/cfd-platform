package algorithms

// GreedyNode picks the node for `newRank` that minimises the partial cost
// of communicating with the already-placed ranks of the same job.
//
// Used in the streaming online flow: each MPI pod arrives independently
// and the Score plugin asks GreedyNode to pick a host for it. Stateful
// only in the supplied Placement: nothing persists across calls.
//
// Complexity O(|placed| · |M|) per call ≈ O(n²) over a job. For typical
// 128-rank cases and 100-node clusters this is ≤ 10 ms.
//
// candidates is the subset of node indices that pass any prior Filter
// stage (e.g. resource availability, taints). When empty, all M nodes
// are considered.
func GreedyNode(in Input, current Placement, newRank int, candidates []int) (int, bool) {
	if in.F == nil || newRank < 0 || newRank >= in.F.NumRanks {
		return -1, false
	}
	if current[newRank] != -1 {
		return current[newRank], true
	}

	matrix := in.F.Matrix()

	// Pre-compute weights from newRank to every other rank (one row).
	weights := matrix[newRank]

	allNodes := candidates
	if len(allNodes) == 0 {
		allNodes = make([]int, in.NumNodes)
		for i := range allNodes {
			allNodes[i] = i
		}
	}

	bestNode := -1
	bestCost := 0.0
	first := true
	usedNodes := make(map[int]struct{}, in.F.NumRanks)
	for _, n := range current {
		if n >= 0 {
			usedNodes[n] = struct{}{}
		}
	}

	for _, candidate := range allNodes {
		if _, taken := usedNodes[candidate]; taken {
			continue
		}
		cost := 0.0
		for placedRank, placedNode := range current {
			if placedNode < 0 || placedRank == newRank {
				continue
			}
			cost += weights[placedRank] * in.L[candidate][placedNode]
		}
		if first || cost < bestCost {
			bestCost = cost
			bestNode = candidate
			first = false
		}
	}
	return bestNode, bestNode != -1
}

// GreedyAll is the batch form: runs GreedyNode for every rank in input
// order, building a complete placement. Used by tests and by the offline
// baseline benchmark to compare against MüllerMerbach.
func GreedyAll(in Input) Placement {
	if in.F == nil {
		return nil
	}
	n := in.F.NumRanks
	p := make(Placement, n)
	for i := range p {
		p[i] = -1
	}
	for rank := 0; rank < n; rank++ {
		node, ok := GreedyNode(in, p, rank, nil)
		if !ok {
			return nil
		}
		p[rank] = node
	}
	return p
}
