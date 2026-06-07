package http

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/theweirdfulmurk/cfd-platform/internal/domain"
	"github.com/theweirdfulmurk/cfd-platform/internal/usecase"
)

const topologyAwareSchedulerName = "topology-aware-scheduler"

// simulationsRoot is the shared PVC mount where CreateWithFile extracts each
// case (mirrors usecase.SimulationUseCase.storagePath). The post-solve export
// step writes surface.vtp + stats.json next to the case here.
const simulationsRoot = "/pvc/simulations"

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
//	scheduler      string  (optional, ="")   — default | random | topology-aware | mueller-merbach
//	file           upload  (required)        — .tar.gz with the case
//
// The three non-default choices all run under the topology-aware-scheduler
// (the profile wired to our extender, see k8s/50-scheduler-config.yaml) and
// differ only by the placement algorithm the extender applies. The benchmark
// orchestrator (experiment/run_benchmark.py) submits these three to compare
// random / greedy / Müller-Merbach placement.
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

	// Map the public scheduler choice to (k8s schedulerName, extender
	// algorithm label). All topology-aware variants share the SAME k8s
	// schedulerName — the extender picks the algorithm from the pod label.
	var schedulerName, algorithm string
	switch r.FormValue("scheduler") {
	case "", "default":
		// empty schedulerName -> default kube-scheduler, extender not consulted
	case "random", "random-scheduler":
		schedulerName, algorithm = topologyAwareSchedulerName, "random"
	case "topology-aware", "greedy":
		schedulerName, algorithm = topologyAwareSchedulerName, "greedy"
	case "mueller-merbach", "mm", "mm-scheduler":
		schedulerName, algorithm = topologyAwareSchedulerName, "mueller-merbach"
	default:
		respondError(w, http.StatusBadRequest,
			"scheduler must be one of: default, random, topology-aware, mueller-merbach")
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
		name, simType, np, schedulerName, algorithm, file, header.Filename,
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

// Surface streams the exported result surface (surface.vtp) for a finished
// simulation. It is produced by the post-solve export step (reconstructPar +
// foamToVTK -surfaceFields, see experiment/export_surface.sh) and lives next to
// the case in the shared PVC. The frontend Visualizer fetches it and renders the
// real geometry coloured by the real field; a 404 makes the viewer fall back to
// the representative preview (solvers/runs without an export yet).
func (h *SimulationHandler) Surface(w http.ResponseWriter, r *http.Request) {
	simID := chi.URLParam(r, "simId")
	path := filepath.Join(simulationsRoot, simID, "surface.vtp")
	f, err := os.Open(path)
	if err != nil {
		// A missing export is a 404 (viewer falls back); any OTHER open error
		// (permissions, PVC mount fault, fd exhaustion) is a real 5xx so it
		// isn't silently masked as "no export yet".
		if errors.Is(err, fs.ErrNotExist) {
			respondError(w, http.StatusNotFound, "surface not found")
		} else {
			respondError(w, http.StatusInternalServerError, "failed to open surface")
		}
		return
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to stat surface")
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition",
		fmt.Sprintf("inline; filename=%s-surface.vtp", simID))
	// ServeContent gives us range requests + caching for free; *os.File is a
	// ReadSeeker.
	http.ServeContent(w, r, "surface.vtp", info.ModTime(), f)
}

// FieldStats returns the real per-field numeric summary (array name, label,
// unit, min/max/mean) the export step writes alongside the surface. It powers
// the viewer legend and the "Скачать результаты" CSV with actual numbers
// instead of the synthetic placeholders. 404 → frontend uses its fallback.
func (h *SimulationHandler) FieldStats(w http.ResponseWriter, r *http.Request) {
	simID := chi.URLParam(r, "simId")
	path := filepath.Join(simulationsRoot, simID, "stats.json")
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			respondError(w, http.StatusNotFound, "field stats not found")
		} else {
			respondError(w, http.StatusInternalServerError, "failed to open field stats")
		}
		return
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to stat field stats")
		return
	}
	// ServeContent (over io.Copy) sets Content-Length and keeps the status at
	// 200 only when the read succeeds — a mid-stream read error won't leave the
	// client with a silently truncated 200 body the way a discarded io.Copy err did.
	w.Header().Set("Content-Type", "application/json")
	http.ServeContent(w, r, "stats.json", info.ModTime(), f)
}

func (h *SimulationHandler) DownloadResults(w http.ResponseWriter, r *http.Request) {
	simID := chi.URLParam(r, "simId")
	resultsPath := fmt.Sprintf("/results/%s", simID)

	if _, err := os.Stat(resultsPath); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			respondError(w, http.StatusNotFound, "results not found")
		} else {
			respondError(w, http.StatusInternalServerError, "failed to stat results")
		}
		return
	}

	// Build the archive into a buffer FIRST. If anything fails mid-walk (a file
	// vanishes on the shared PVC, an I/O error) we can still return a clean 500
	// — once we start writing to w the 200 status is committed and a later
	// respondError would corrupt the zip byte stream instead of signalling.
	var buf bytes.Buffer
	zipWriter := zip.NewWriter(&buf)
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
	if err == nil {
		err = zipWriter.Close() // flush central directory; check the error
	} else {
		_ = zipWriter.Close()
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to create archive")
		return
	}

	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition",
		fmt.Sprintf("attachment; filename=results-%s.zip", simID))
	w.Header().Set("Content-Length", strconv.Itoa(buf.Len()))
	_, _ = io.Copy(w, &buf)
}
