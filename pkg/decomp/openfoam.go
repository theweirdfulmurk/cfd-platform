package decomp

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// OpenFOAM parses an OpenFOAM case decomposed by `decomposePar`.
//
// caseDir must contain processor0/, processor1/, ..., each with
// constant/polyMesh/boundary. Inside each boundary file there are
// sections of type `processor` with fields myProcNo, neighbProcNo and
// nFaces — these become entries of the communication graph.
//
// F(i,j) = nFaces shared between processor i and processor j.
// The same edge appears in both boundary files (once with myProcNo=i,
// neighbProcNo=j and once mirrored); we take it once per unordered pair.
func OpenFOAM(caseDir string) (*Graph, error) {
	entries, err := os.ReadDir(caseDir)
	if err != nil {
		return nil, fmt.Errorf("decomp/openfoam: read %s: %w", caseDir, err)
	}
	procDirs := []string{}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if strings.HasPrefix(e.Name(), "processor") {
			procDirs = append(procDirs, e.Name())
		}
	}
	if len(procDirs) == 0 {
		return nil, fmt.Errorf("decomp/openfoam: no processorN/ subdirs in %s (run decomposePar first)", caseDir)
	}
	sort.Strings(procDirs)

	weights := make(map[Pair]float64)
	for _, d := range procDirs {
		path := filepath.Join(caseDir, d, "constant", "polyMesh", "boundary")
		f, err := os.Open(path)
		if err != nil {
			return nil, fmt.Errorf("decomp/openfoam: open %s: %w", path, err)
		}
		sections, err := parseFoamBoundary(f)
		f.Close()
		if err != nil {
			return nil, fmt.Errorf("decomp/openfoam: parse %s: %w", path, err)
		}
		for _, s := range sections {
			if s.Type != "processor" {
				continue
			}
			p, err := NewPair(s.MyProcNo, s.NeighbProcNo)
			if err != nil {
				return nil, fmt.Errorf("decomp/openfoam: %s: %w", path, err)
			}
			// Each procBoundary appears twice (once on each side); we want the
			// value once, but reading both gives us a consistency check.
			if existing, ok := weights[p]; ok {
				if existing != float64(s.NFaces) {
					return nil, fmt.Errorf("decomp/openfoam: nFaces mismatch for pair {%d,%d}: %v vs %d",
						p.I, p.J, existing, s.NFaces)
				}
				continue
			}
			weights[p] = float64(s.NFaces)
		}
	}
	if len(weights) == 0 {
		return nil, ErrEmpty
	}
	return Build(len(procDirs), weights)
}

// foamProcSection is one entry of the boundary file relevant to us.
type foamProcSection struct {
	Name         string
	Type         string
	NFaces       int
	MyProcNo     int
	NeighbProcNo int
}

// parseFoamBoundary reads an OpenFOAM polyMesh/boundary file and returns
// every named section that has a `type` field. The OpenFOAM dictionary
// format is structurally simple:
//
//	N
//	(
//	    name1 { key value; ... }
//	    name2 { key value; ... }
//	    ...
//	)
//
// with C-style comments and a header in FoamFile { ... }. We strip
// comments, drop the FoamFile header, then walk top-level `name { ... }`
// pairs inside the outer (...) list.
func parseFoamBoundary(r io.Reader) ([]foamProcSection, error) {
	raw, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	text := stripFoamComments(string(raw))
	text = dropFoamHeader(text)

	// Find the outer list "( ... )".
	openIdx := strings.Index(text, "(")
	closeIdx := strings.LastIndex(text, ")")
	if openIdx < 0 || closeIdx < 0 || closeIdx <= openIdx {
		return nil, fmt.Errorf("malformed boundary file: missing outer parentheses")
	}
	body := text[openIdx+1 : closeIdx]

	return walkFoamSections(body)
}

var (
	foamLineComment  = regexp.MustCompile(`//[^\n]*`)
	foamBlockComment = regexp.MustCompile(`(?s)/\*.*?\*/`)
)

func stripFoamComments(s string) string {
	s = foamBlockComment.ReplaceAllString(s, "")
	s = foamLineComment.ReplaceAllString(s, "")
	return s
}

// dropFoamHeader removes the leading FoamFile { ... } block, if present.
func dropFoamHeader(s string) string {
	idx := strings.Index(s, "FoamFile")
	if idx < 0 {
		return s
	}
	rest := s[idx:]
	brace := strings.Index(rest, "{")
	if brace < 0 {
		return s
	}
	end, ok := findMatchingBrace(rest, brace)
	if !ok {
		return s
	}
	return s[:idx] + rest[end+1:]
}

// findMatchingBrace returns the index of the '}' that closes the '{' at start.
func findMatchingBrace(s string, start int) (int, bool) {
	depth := 0
	for i := start; i < len(s); i++ {
		switch s[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return i, true
			}
		}
	}
	return 0, false
}

// walkFoamSections scans body for top-level `name { ... }` entries and
// extracts foamProcSection records.
func walkFoamSections(body string) ([]foamProcSection, error) {
	out := []foamProcSection{}
	i := 0
	for i < len(body) {
		// Skip whitespace.
		for i < len(body) && isFoamSpace(body[i]) {
			i++
		}
		if i >= len(body) {
			break
		}
		// Read section name (until whitespace or '{').
		nameStart := i
		for i < len(body) && !isFoamSpace(body[i]) && body[i] != '{' {
			i++
		}
		name := body[nameStart:i]
		if name == "" {
			break
		}
		// Skip whitespace, expect '{'.
		for i < len(body) && isFoamSpace(body[i]) {
			i++
		}
		if i >= len(body) || body[i] != '{' {
			return nil, fmt.Errorf("expected '{' after section name %q", name)
		}
		end, ok := findMatchingBrace(body, i)
		if !ok {
			return nil, fmt.Errorf("unmatched '{' after section name %q", name)
		}
		inner := body[i+1 : end]
		section, err := parseFoamSection(name, inner)
		if err != nil {
			return nil, err
		}
		out = append(out, section)
		i = end + 1
	}
	return out, nil
}

func isFoamSpace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\r'
}

// parseFoamSection extracts key/value entries from the section body.
// Only fields relevant to communication graph extraction are recorded.
func parseFoamSection(name, body string) (foamProcSection, error) {
	s := foamProcSection{Name: name}
	scanner := bufio.NewScanner(strings.NewReader(body))
	scanner.Buffer(make([]byte, 0, 4096), 1<<20)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		line = strings.TrimSuffix(line, ";")
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		key, val := fields[0], fields[len(fields)-1]
		switch key {
		case "type":
			s.Type = val
		case "nFaces":
			n, err := strconv.Atoi(val)
			if err != nil {
				return s, fmt.Errorf("section %s: nFaces=%q: %w", name, val, err)
			}
			s.NFaces = n
		case "myProcNo":
			n, err := strconv.Atoi(val)
			if err != nil {
				return s, fmt.Errorf("section %s: myProcNo=%q: %w", name, val, err)
			}
			s.MyProcNo = n
		case "neighbProcNo":
			n, err := strconv.Atoi(val)
			if err != nil {
				return s, fmt.Errorf("section %s: neighbProcNo=%q: %w", name, val, err)
			}
			s.NeighbProcNo = n
		}
	}
	return s, nil
}
