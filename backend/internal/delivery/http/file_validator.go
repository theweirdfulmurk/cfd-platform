package http

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"mime/multipart"
	"path/filepath"
	"strings"

	"github.com/theweirdfulmurk/cfd-platform/internal/domain"
)

func ValidateSimulationFile(file multipart.File, header *multipart.FileHeader, simType domain.SimulationType) error {
	switch simType {
	case domain.SimTypeCFD:
		return validateOpenFOAMArchive(file, header)
	case domain.SimTypeFEA:
		return validateCalculiXInput(file, header)
	default:
		return fmt.Errorf("unknown simulation type: %s", simType)
	}
}

func validateOpenFOAMArchive(file multipart.File, header *multipart.FileHeader) error {
	if !strings.HasSuffix(header.Filename, ".tar.gz") {
		return fmt.Errorf("CFD simulation requires .tar.gz archive, got: %s", header.Filename)
	}

	if header.Size > 100*1024*1024 {
		return fmt.Errorf("archive too large: %d bytes (max 100MB)", header.Size)
	}

	buf := new(bytes.Buffer)
	if _, err := io.Copy(buf, file); err != nil {
		return fmt.Errorf("failed to read archive: %w", err)
	}

	gzr, err := gzip.NewReader(buf)
	if err != nil {
		return fmt.Errorf("invalid gzip format: %w", err)
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)

	requiredFiles := map[string]bool{
		"system/controlDict":   false,
		"system/fvSchemes":     false,
		"system/fvSolution":    false,
		"constant/transportProperties": false,
	}

	hasPolyMesh := false

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("failed to read tar: %w", err)
		}

		path := strings.TrimPrefix(hdr.Name, filepath.Dir(hdr.Name)+"/")

		for required := range requiredFiles {
			if strings.HasSuffix(path, required) {
				requiredFiles[required] = true
			}
		}

		if strings.Contains(path, "constant/polyMesh") {
			hasPolyMesh = true
		}
	}

	var missing []string
	for file, found := range requiredFiles {
		if !found {
			missing = append(missing, file)
		}
	}

	if len(missing) > 0 {
		return fmt.Errorf("missing required OpenFOAM files: %v", missing)
	}

	if !hasPolyMesh {
		return fmt.Errorf("missing constant/polyMesh directory")
	}

	return nil
}

func validateCalculiXInput(file multipart.File, header *multipart.FileHeader) error {
	if !strings.HasSuffix(header.Filename, ".inp") {
		return fmt.Errorf("FEA simulation requires .inp file, got: %s", header.Filename)
	}

	if header.Size > 50*1024*1024 {
		return fmt.Errorf("input file too large: %d bytes (max 50MB)", header.Size)
	}

	buf := make([]byte, 1024)
	n, err := file.Read(buf)
	if err != nil && err != io.EOF {
		return fmt.Errorf("failed to read file: %w", err)
	}

	content := string(buf[:n])

	requiredKeywords := []string{"*NODE", "*ELEMENT"}
	var found []string

	for _, kw := range requiredKeywords {
		if strings.Contains(strings.ToUpper(content), kw) {
			found = append(found, kw)
		}
	}

	if len(found) < len(requiredKeywords) {
		return fmt.Errorf("invalid CalculiX .inp file: missing keywords %v", requiredKeywords)
	}

	return nil
}