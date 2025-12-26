package http

import (
	"encoding/json"
	"net/http"

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

type createSimulationRequest struct {
	Name       string `json:"name"`
	Type       string `json:"type"`
	ConfigPath string `json:"configPath"`
}

func (h *SimulationHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req createSimulationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	simType := domain.SimulationType(req.Type)
	if simType != domain.SimTypeCFD && simType != domain.SimTypeFEA {
		respondError(w, http.StatusBadRequest, "invalid simulation type")
		return
	}

	sim, err := h.useCase.Create(req.Name, simType, req.ConfigPath)
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