package decomp

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// METIS parses a pair of METIS files — graph adjacency and partitioning —
// and aggregates edge weights crossing partition boundaries into the
// inter-rank communication matrix F.
//
// The graph file format (.graph) is the standard METIS/CSR text format:
//
//	nvertices nedges [fmt [ncon]]
//	<line for vertex 1>
//	<line for vertex 2>
//	...
//	<line for vertex nvertices>
//
// fmt is up to three binary digits VHE: V=1 vsize present, H=1 vertex
// weights present, E=1 edge weights present. ncon is the number of vertex
// weights per vertex. Vertices are 1-indexed in METIS; each edge is
// listed in both endpoint lines.
//
// The partition file (.part.N) contains nvertices lines, each holding the
// partition index (0..N-1) the vertex was assigned to.
//
// METIS is the partitioner used internally by OpenRadioss and Code_Aster,
// so this single reader covers both solvers when their preprocessor exposes
// the .graph + .part.N artefacts (a small Python wrapper is enough for any
// solver that uses MED, HDF5 or proprietary restart files).
func METIS(graphPath, partPath string) (*Graph, error) {
	adj, err := readMetisGraph(graphPath)
	if err != nil {
		return nil, fmt.Errorf("decomp/metis: graph %s: %w", graphPath, err)
	}
	part, err := readMetisPartition(partPath)
	if err != nil {
		return nil, fmt.Errorf("decomp/metis: partition %s: %w", partPath, err)
	}
	if len(adj) != len(part) {
		return nil, fmt.Errorf("decomp/metis: graph has %d vertices, partition has %d", len(adj), len(part))
	}

	numRanks := 0
	for _, p := range part {
		if p+1 > numRanks {
			numRanks = p + 1
		}
	}
	if numRanks < 1 {
		return nil, fmt.Errorf("decomp/metis: partition is empty")
	}

	weights := make(map[Pair]float64)
	for u, neigh := range adj {
		for _, e := range neigh {
			v := e.target
			if v <= u {
				// every edge appears twice; count it once
				continue
			}
			pu, pv := part[u], part[v]
			if pu == pv {
				continue
			}
			pair, err := NewPair(pu, pv)
			if err != nil {
				return nil, err
			}
			weights[pair] += e.weight
		}
	}
	if len(weights) == 0 {
		return nil, ErrEmpty
	}
	return Build(numRanks, weights)
}

// metisEdge is one half-edge in the adjacency list.
type metisEdge struct {
	target int     // 0-indexed
	weight float64 // 1.0 by default
}

// readMetisGraph returns adj[u] = list of (v, w) half-edges, 0-indexed.
func readMetisGraph(path string) ([][]metisEdge, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 1<<16), 1<<22)

	header, err := nextMetisLine(scanner)
	if err != nil {
		return nil, fmt.Errorf("missing header: %w", err)
	}
	hdrFields := strings.Fields(header)
	if len(hdrFields) < 2 {
		return nil, fmt.Errorf("malformed header %q", header)
	}
	nv, err := strconv.Atoi(hdrFields[0])
	if err != nil {
		return nil, fmt.Errorf("nvertices %q: %w", hdrFields[0], err)
	}
	fmtField := "000"
	if len(hdrFields) >= 3 {
		fmtField = hdrFields[2]
	}
	ncon := 1
	if len(hdrFields) >= 4 {
		ncon, err = strconv.Atoi(hdrFields[3])
		if err != nil {
			return nil, fmt.Errorf("ncon %q: %w", hdrFields[3], err)
		}
	}
	hasVsize, hasVweight, hasEweight, err := parseFmt(fmtField)
	if err != nil {
		return nil, err
	}

	adj := make([][]metisEdge, nv)
	for u := 0; u < nv; u++ {
		line, err := nextMetisLine(scanner)
		if err != nil {
			return nil, fmt.Errorf("vertex %d: %w", u+1, err)
		}
		tokens := strings.Fields(line)
		idx := 0
		if hasVsize {
			idx++
		}
		if hasVweight {
			idx += ncon
		}
		if idx > len(tokens) {
			return nil, fmt.Errorf("vertex %d: truncated line", u+1)
		}
		edges := []metisEdge{}
		for idx < len(tokens) {
			vid1, err := strconv.Atoi(tokens[idx])
			if err != nil {
				return nil, fmt.Errorf("vertex %d: bad neighbour %q: %w", u+1, tokens[idx], err)
			}
			if vid1 < 1 || vid1 > nv {
				return nil, fmt.Errorf("vertex %d: neighbour %d out of range [1,%d]", u+1, vid1, nv)
			}
			idx++
			w := 1.0
			if hasEweight {
				if idx >= len(tokens) {
					return nil, fmt.Errorf("vertex %d: missing edge weight after neighbour %d", u+1, vid1)
				}
				w, err = strconv.ParseFloat(tokens[idx], 64)
				if err != nil {
					return nil, fmt.Errorf("vertex %d: edge weight %q: %w", u+1, tokens[idx], err)
				}
				idx++
			}
			edges = append(edges, metisEdge{target: vid1 - 1, weight: w})
		}
		adj[u] = edges
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return adj, nil
}

// readMetisPartition returns part[u] = partition index (0-based).
func readMetisPartition(path string) ([]int, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	out := []int{}
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "%") || strings.HasPrefix(line, "#") {
			continue
		}
		p, err := strconv.Atoi(line)
		if err != nil {
			return nil, fmt.Errorf("partition entry %q: %w", line, err)
		}
		if p < 0 {
			return nil, fmt.Errorf("partition index %d must be >= 0", p)
		}
		out = append(out, p)
	}
	return out, scanner.Err()
}

// nextMetisLine returns the next non-empty, non-comment line.
func nextMetisLine(scanner *bufio.Scanner) (string, error) {
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "%") || strings.HasPrefix(line, "#") {
			continue
		}
		return line, nil
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	return "", fmt.Errorf("unexpected EOF")
}

// parseFmt decodes the up-to-3-digit fmt field: VHE.
func parseFmt(s string) (vsize, vweight, eweight bool, err error) {
	// Pad to length 3.
	for len(s) < 3 {
		s = "0" + s
	}
	if len(s) != 3 {
		return false, false, false, fmt.Errorf("fmt %q must be 1..3 digits", s)
	}
	for _, c := range s {
		if c != '0' && c != '1' {
			return false, false, false, fmt.Errorf("fmt %q must contain only 0/1", s)
		}
	}
	return s[0] == '1', s[1] == '1', s[2] == '1', nil
}
