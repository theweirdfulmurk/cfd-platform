package decomp

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

// EdgeList reads a flat weighted edge list:
//
//	# any comment line (also '%' is accepted)
//	NumRanks
//	I J Weight
//	I J Weight
//	...
//
// Ranks are 0-indexed; only the upper-triangle entry per pair is needed
// but duplicates are summed silently.
//
// This format is the fallback for any solver whose decomposition cannot
// be read directly — a thin Python preprocessor can convert the solver's
// internal artefacts to this two-line-per-edge form.
func EdgeList(path string) (*Graph, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("decomp/edgelist: open %s: %w", path, err)
	}
	defer f.Close()
	return parseEdgeList(f)
}

func parseEdgeList(r io.Reader) (*Graph, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 1<<16), 1<<22)

	numRanks := -1
	weights := make(map[Pair]float64)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "%") {
			continue
		}
		fields := strings.Fields(line)

		if numRanks < 0 {
			if len(fields) != 1 {
				return nil, fmt.Errorf("decomp/edgelist: first non-comment line must hold NumRanks, got %q", line)
			}
			n, err := strconv.Atoi(fields[0])
			if err != nil {
				return nil, fmt.Errorf("decomp/edgelist: NumRanks %q: %w", fields[0], err)
			}
			numRanks = n
			continue
		}

		if len(fields) != 3 {
			return nil, fmt.Errorf("decomp/edgelist: expected 'I J Weight', got %q", line)
		}
		i, err := strconv.Atoi(fields[0])
		if err != nil {
			return nil, fmt.Errorf("decomp/edgelist: I %q: %w", fields[0], err)
		}
		j, err := strconv.Atoi(fields[1])
		if err != nil {
			return nil, fmt.Errorf("decomp/edgelist: J %q: %w", fields[1], err)
		}
		w, err := strconv.ParseFloat(fields[2], 64)
		if err != nil {
			return nil, fmt.Errorf("decomp/edgelist: Weight %q: %w", fields[2], err)
		}
		if w == 0 {
			continue
		}
		p, err := NewPair(i, j)
		if err != nil {
			return nil, err
		}
		weights[p] += w
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if numRanks < 0 {
		return nil, fmt.Errorf("decomp/edgelist: file has no NumRanks header")
	}
	if len(weights) == 0 {
		return nil, ErrEmpty
	}
	return Build(numRanks, weights)
}
