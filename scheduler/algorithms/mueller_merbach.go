package algorithms

import "math"

// MuellerMerbach implements the offline construction heuristic for QAP
// described in Burkard, Dell'Amico, Martello "Assignment Problems"
// (SIAM 2009), §8.2.1.
//
// For each rank, taken in order of descending row sum of F (heaviest
// communicators first), we evaluate every candidate node by its *full*
// delta cost — for free nodes that's Δz(j); for occupied nodes it's
// Δz(j) plus the cheapest reassignment cost of the displaced rank. Then
// we pick the global minimum and, if it was an occupied node, perform
// the cascading swap.
//
// This is the cost-accurate variant of the cascading rule from §8.2.1:
// the greedy from earlier paragraphs ignores occupied nodes, MM enriches
// it with one level of look-ahead. Complexity O(n²·M) per problem
// instance (≤ 100 ms for n=128, M=100 on commodity hardware).
func MuellerMerbach(in Input) Placement {
	if in.F == nil {
		return nil
	}
	n := in.F.NumRanks
	if n == 0 {
		return Placement{}
	}
	matrix := in.F.Matrix()
	order := orderByRowSum(matrix)

	p := make(Placement, n)
	for i := range p {
		p[i] = -1
	}
	usedNodes := make([]bool, in.NumNodes)
	nodeToRank := make([]int, in.NumNodes)
	for i := range nodeToRank {
		nodeToRank[i] = -1
	}

	for _, rank := range order {
		bestNode := -1
		bestCost := math.Inf(1)

		for cand := 0; cand < in.NumNodes; cand++ {
			cost := candidateCost(in, matrix, p, rank, cand, usedNodes, nodeToRank)
			if cost < bestCost {
				bestCost = cost
				bestNode = cand
			}
		}
		if bestNode == -1 {
			return nil
		}

		if usedNodes[bestNode] {
			// Cascade: displaced rank goes to the cheapest free node *after*
			// rank is in place on bestNode.
			displaced := nodeToRank[bestNode]
			p[rank] = bestNode
			nodeToRank[bestNode] = rank

			p[displaced] = -1
			altNode := -1
			altCost := math.Inf(1)
			for cand := 0; cand < in.NumNodes; cand++ {
				if usedNodes[cand] && cand != bestNode {
					continue
				}
				if cand == bestNode {
					continue
				}
				delta := deltaForFreeNode(in, matrix, p, displaced, cand)
				if delta < altCost {
					altCost = delta
					altNode = cand
				}
			}
			if altNode == -1 {
				return nil
			}
			p[displaced] = altNode
			usedNodes[altNode] = true
			nodeToRank[altNode] = displaced
		} else {
			p[rank] = bestNode
			usedNodes[bestNode] = true
			nodeToRank[bestNode] = rank
		}
	}
	return p
}

// candidateCost is the full Δ for assigning rank → cand, including the
// cost of evicting the previous occupant of cand (if any).
func candidateCost(
	in Input,
	matrix [][]float64,
	p Placement,
	rank, cand int,
	usedNodes []bool,
	nodeToRank []int,
) float64 {
	free := !usedNodes[cand]
	if free {
		return deltaForFreeNode(in, matrix, p, rank, cand)
	}
	// Occupied: simulate kicking the displaced rank to the best free node.
	displaced := nodeToRank[cand]

	deltaNew := 0.0
	for other, otherNode := range p {
		if other == rank || other == displaced || otherNode < 0 {
			continue
		}
		deltaNew += matrix[rank][other] * in.L[cand][otherNode]
	}

	bestAltDelta := math.Inf(1)
	for alt := 0; alt < in.NumNodes; alt++ {
		if alt == cand || usedNodes[alt] {
			continue
		}
		alt2 := 0.0
		for other, otherNode := range p {
			if other == rank || other == displaced || otherNode < 0 {
				continue
			}
			alt2 += matrix[displaced][other] * in.L[alt][otherNode]
		}
		// also account for the direct rank↔displaced contribution.
		alt2 += matrix[rank][displaced] * in.L[cand][alt]
		if alt2 < bestAltDelta {
			bestAltDelta = alt2
		}
	}
	if math.IsInf(bestAltDelta, 1) {
		return math.Inf(1)
	}
	return deltaNew + bestAltDelta
}

// deltaForFreeNode is Δz(j) — the additional cost of placing rank on a
// currently-free node `cand`, considering only the already-placed ranks.
func deltaForFreeNode(
	in Input,
	matrix [][]float64,
	p Placement,
	rank, cand int,
) float64 {
	delta := 0.0
	for other, otherNode := range p {
		if otherNode < 0 || other == rank {
			continue
		}
		delta += matrix[rank][other] * in.L[cand][otherNode]
	}
	return delta
}

// orderByRowSum returns rank indices sorted by descending sum of row F[i].
// Ties broken by ascending index for determinism.
func orderByRowSum(matrix [][]float64) []int {
	n := len(matrix)
	sums := make([]float64, n)
	order := make([]int, n)
	for i := 0; i < n; i++ {
		order[i] = i
		for j := 0; j < n; j++ {
			sums[i] += matrix[i][j]
		}
	}
	for i := 1; i < n; i++ {
		for j := i; j > 0; j-- {
			if sums[order[j]] > sums[order[j-1]] ||
				(sums[order[j]] == sums[order[j-1]] && order[j] < order[j-1]) {
				order[j], order[j-1] = order[j-1], order[j]
			} else {
				break
			}
		}
	}
	return order
}
