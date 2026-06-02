package http

import (
	"archive/zip"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/theweirdfulmurk/cfd-platform/internal/domain"
	"github.com/theweirdfulmurk/cfd-platform/internal/usecase"
)

const topologyAwareSchedulerName = "topology-aware-scheduler"

type SimulationHandler struct {
	useCase *usecase.SimulationUseCase
}

func NewSimulationHandler(uc *usecase.SimulationUseCase) *SimulationHandler {
	return &SimulationHandler{useCase: uc}
}

// Create parses a multipart/form-data request:
//
//	name           string  (required)        — display name
//	type           string  (required)        — openfoam | openradioss | code_aster
//	np             int     (optional, =1)    — MPI ranks
//	scheduler      string  (optional, ="")   — "topology-aware" picks our plugin
//	file           upload  (required)        — .tar.gz with the case
func (h *SimulationHandler) Create(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(maxUploadBytes); err != nil {
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
	if !simType.IsValid() {
		respondError(w, http.StatusBadRequest,
			"invalid type: expected one of openfoam, openradioss, code_aster")
		return
	}

	np := 1
	if v := r.FormValue("np"); v != "" {
		parsed, err := strconv.Atoi(v)
		if err != nil || parsed < 1 || parsed > 1024 {
			respondError(w, http.StatusBadRequest, "np must be an integer in [1, 1024]")
			return
		}
		np = parsed
	}

	schedulerName := ""
	switch r.FormValue("scheduler") {
	case "", "default":
		// empty schedulerName -> default kube-scheduler
	case "topology-aware":
		schedulerName = topologyAwareSchedulerName
	default:
		respondError(w, http.StatusBadRequest,
			"scheduler must be one of: default, topology-aware")
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		respondError(w, http.StatusBadRequest, "file is required")
		return
	}
	defer file.Close()

	if err := ValidateSimulationFile(file, header, simType); err != nil {
		respondError(w, http.StatusBadRequest, fmt.Sprintf("validation failed: %v", err))
		return
	}
	if seeker, ok := file.(io.Seeker); ok {
		_, _ = seeker.Seek(0, io.SeekStart)
	}

	sim, err := h.useCase.CreateWithFile(
		name, simType, np, schedulerName, file, header.Filename,
	)
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

	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition",
		fmt.Sprintf("attachment; filename=results-%s.zip", simID))

	zipWriter := zip.NewWriter(w)
	defer zipWriter.Close()

	err := filepath.Walk(resultsPath, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		relPath, err := filepath.Rel(resultsPath, path)
		if err != nil {
			return err
		}
		zf, err := zipWriter.Create(relPath)
		if err != nil {
			return err
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()
		_, err = io.Copy(zf, f)
		return err
	})
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to create archive")
	}
}
