package algorithms

import (
	"hash/fnv"
	"math/rand"
)

// RandomNode picks a uniformly-random feasible node for `newRank`,
// ignoring the F-graph entirely. This is the experimental baseline that
// imitates the kube-scheduler default behaviour: no topology awareness,
// just any node that has capacity (= survives the Filter stage).
//
// `seed` lets the caller make the choice reproducible per simulation —
// passing the FNV-1a hash of the job id gives a deterministic but
// per-job-different placement, which is what the benchmark wants.
//
// Complexity O(|candidates|) per call.
func RandomNode(in Input, current Placement, newRank int, candidates []int, seed int64) (int, bool) {
	if newRank < 0 || newRank >= len(current) {
		return -1, false
	}
	if current[newRank] != -1 {
		return current[newRank], true
	}

	// Build list of feasible nodes (= candidates minus already-occupied).
	occupied := make(map[int]struct{}, len(current))
	for _, n := range current {
		if n >= 0 {
			occupied[n] = struct{}{}
		}
	}

	pool := candidates
	if len(pool) == 0 {
		pool = make([]int, in.NumNodes)
		for i := range pool {
			pool[i] = i
		}
	}
	free := pool[:0:0]
	for _, n := range pool {
		if _, taken := occupied[n]; !taken {
			free = append(free, n)
		}
	}
	if len(free) == 0 {
		return -1, false
	}

	rng := rand.New(rand.NewSource(seed + int64(newRank)))
	return free[rng.Intn(len(free))], true
}

// RandomAll is the offline batch variant — produces a complete random
// placement at once. Used when caller has all ranks visible (i.e. not
// streaming through the Score plugin).
func RandomAll(in Input, seed int64) Placement {
	p := make(Placement, in.F.NumRanks)
	for i := range p {
		p[i] = -1
	}
	for rank := 0; rank < in.F.NumRanks; rank++ {
		node, ok := RandomNode(in, p, rank, nil, seed)
		if !ok {
			break
		}
		p[rank] = node
	}
	return p
}

// SeedFromJobID returns a stable 64-bit seed derived from the MPIJob id.
// Two replicas of the same job get the same random placement (good for
// repeating an experiment).
func SeedFromJobID(jobID string) int64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(jobID))
	return int64(h.Sum64())
}
