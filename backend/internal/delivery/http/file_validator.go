package http

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"mime/multipart"
	"strings"

	"github.com/theweirdfulmurk/cfd-platform/internal/domain"
)

const maxUploadBytes = 200 * 1024 * 1024 // 200 MB

// ValidateSimulationFile checks that the uploaded archive contains the
// minimum files each solver expects. The check is structural only —
// solver-specific consistency (e.g. valid mesh geometry) is left to the
// solver itself, which fails fast with a clear error if the input is bad.
func ValidateSimulationFile(file multipart.File, header *multipart.FileHeader, simType domain.SimulationType) error {
	if !strings.HasSuffix(header.Filename, ".tar.gz") {
		return fmt.Errorf("upload must be a .tar.gz archive, got: %s", header.Filename)
	}
	if header.Size > maxUploadBytes {
		return fmt.Errorf("archive too large: %d bytes (max %d)", header.Size, maxUploadBytes)
	}

	entries, err := tarballEntries(file)
	if err != nil {
		return err
	}

	switch simType {
	case domain.SimTypeOpenFOAM:
		return validateOpenFOAMCase(entries)
	case domain.SimTypeOpenRadioss:
		return validateOpenRadiossCase(entries)
	case domain.SimTypeCodeAster:
		return validateCodeAsterCase(entries)
	}
	return fmt.Errorf("unsupported simulation type: %s", simType)
}

// tarballEntries returns the list of file paths inside a .tar.gz upload.
func tarballEntries(file multipart.File) ([]string, error) {
	buf := new(bytes.Buffer)
	if _, err := io.Copy(buf, file); err != nil {
		return nil, fmt.Errorf("read archive: %w", err)
	}
	gzr, err := gzip.NewReader(buf)
	if err != nil {
		return nil, fmt.Errorf("invalid gzip: %w", err)
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)
	names := []string{}
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read tar: %w", err)
		}
		names = append(names, hdr.Name)
	}
	return names, nil
}

// validateOpenFOAMCase checks for the canonical OpenFOAM case layout:
// system/{controlDict,fvSchemes,fvSolution} + constant/polyMesh/.
func validateOpenFOAMCase(entries []string) error {
	required := []string{"system/controlDict", "system/fvSchemes", "system/fvSolution"}
	missing := missingPaths(entries, required, false)
	if len(missing) > 0 {
		return fmt.Errorf("OpenFOAM case missing required files: %v", missing)
	}
	if !anyMatch(entries, "constant/polyMesh/") {
		return fmt.Errorf("OpenFOAM case missing constant/polyMesh/ directory")
	}
	return nil
}

// validateOpenRadiossCase requires a Radioss Starter input deck (.rad).
// The Starter itself decides whether the model is consistent.
func validateOpenRadiossCase(entries []string) error {
	if !anyMatch(entries, ".rad") {
		return fmt.Errorf("OpenRadioss case requires a .rad Starter input deck")
	}
	return nil
}

// validateCodeAsterCase requires a Code_Aster .export driver and at least
// one .med mesh.
func validateCodeAsterCase(entries []string) error {
	if !anyMatch(entries, ".export") {
		return fmt.Errorf("Code_Aster case requires a .export driver file")
	}
	if !anyMatch(entries, ".med") {
		return fmt.Errorf("Code_Aster case requires a .med mesh file")
	}
	return nil
}

// missingPaths returns required suffixes not found in entries.
func missingPaths(entries, required []string, exact bool) []string {
	missing := []string{}
	for _, req := range required {
		found := false
		for _, e := range entries {
			if exact && e == req {
				found = true
				break
			}
			if !exact && strings.HasSuffix(e, req) {
				found = true
				break
			}
		}
		if !found {
			missing = append(missing, req)
		}
	}
	return missing
}

func anyMatch(entries []string, substr string) bool {
	for _, e := range entries {
		if strings.Contains(e, substr) {
			return true
		}
	}
	return false
}
