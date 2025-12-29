package http

import (
	"archive/zip"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/go-chi/chi/v5"
	"github.com/theweirdfulmurk/cfd-platform/internal/domain"
	"github.com/theweirdfulmurk/cfd-platform/internal/usecase"
)

type SimulationHandler struct {
	useCase *usecase.SimulationUseCase
}

func NewSimulationHandler(uc *usecase.SimulationUseCase) *SimulationHandler {
	return &SimulationHandler{useCase: uc}
}

func (h *SimulationHandler) Create(w http.ResponseWriter, r *http.Request) {
	// Parse multipart form (max 100MB)
	if err := r.ParseMultipartForm(100 << 20); err != nil {
		respondError(w, http.StatusBadRequest, "failed to parse form")
		return
	}

	name := r.FormValue("name")
	simTypeStr := r.FormValue("type")

	if name == "" || simTypeStr == "" {
		respondError(w, http.StatusBadRequest, "name and type are required")
		return
	}

	simType := domain.SimulationType(simTypeStr)
	if simType != domain.SimTypeCFD && simType != domain.SimTypeFEA {
		respondError(w, http.StatusBadRequest, "invalid simulation type")
		return
	}

	// Get uploaded file
	file, header, err := r.FormFile("file")
	if err != nil {
		respondError(w, http.StatusBadRequest, "file is required")
		return
	}
	defer file.Close()

	// Validate file
	if err := ValidateSimulationFile(file, header, simType); err != nil {
		respondError(w, http.StatusBadRequest, fmt.Sprintf("validation failed: %v", err))
		return
	}

	// Reset file pointer after validation
	if seeker, ok := file.(io.Seeker); ok {
		seeker.Seek(0, 0)
	}

	// Create simulation with uploaded file
	sim, err := h.useCase.CreateWithFile(name, simType, file, header.Filename)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondJSON(w, http.StatusCreated, sim)
}

func (h *SimulationHandler) Get(w http.ResponseWriter, r *http.Request) {
	simID := chi.URLParam(r, "simId")

	sim, err := h.useCase.GetByID(simID)
	if err != nil {
		respondError(w, http.StatusNotFound, "simulation not found")
		return
	}

	respondJSON(w, http.StatusOK, sim)
}

func (h *SimulationHandler) List(w http.ResponseWriter, r *http.Request) {
	sims, err := h.useCase.List()
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, sims)
}

func (h *SimulationHandler) Delete(w http.ResponseWriter, r *http.Request) {
	simID := chi.URLParam(r, "simId")

	if err := h.useCase.Delete(simID); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *SimulationHandler) DownloadResults(w http.ResponseWriter, r *http.Request) {
	simID := chi.URLParam(r, "simId")

	resultsPath := fmt.Sprintf("/results/%s", simID)

	if _, err := os.Stat(resultsPath); os.IsNotExist(err) {
		respondError(w, http.StatusNotFound, "results not found")
		return
	}

	resultsPath = fmt.Sprintf("/results/%s", simID)

	if _, err := os.Stat(resultsPath); os.IsNotExist(err) {
		respondError(w, http.StatusNotFound, "results not found")
		return
	}

	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=results-%s.zip", simID))

	zipWriter := zip.NewWriter(w)
	defer zipWriter.Close()

	var err error
	err = filepath.Walk(resultsPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}

		relPath, err := filepath.Rel(resultsPath, path)
		if err != nil {
			return err
		}

		zipFile, err := zipWriter.Create(relPath)
		if err != nil {
			return err
		}

		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()

		_, err = io.Copy(zipFile, file)
		return err
	})

	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to create archive")
	}
}
