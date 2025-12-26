package http

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/theweirdfulmurk/cfd-platform/internal/usecase"
)

type VisualizationHandler struct {
	useCase *usecase.VisualizationUseCase
}

func NewVisualizationHandler(uc *usecase.VisualizationUseCase) *VisualizationHandler {
	return &VisualizationHandler{useCase: uc}
}

type createVisualizationRequest struct {
	SimulationID string `json:"simulationId"`
	ResultPath   string `json:"resultPath"`
}

func (h *VisualizationHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req createVisualizationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	viz, err := h.useCase.Create(req.SimulationID, req.ResultPath)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondJSON(w, http.StatusCreated, viz)
}

func (h *VisualizationHandler) GetStatus(w http.ResponseWriter, r *http.Request) {
	vizID := chi.URLParam(r, "vizId")

	viz, err := h.useCase.GetByID(vizID)
	if err != nil {
		respondError(w, http.StatusNotFound, "visualization not found")
		return
	}

	respondJSON(w, http.StatusOK, viz)
}

func (h *VisualizationHandler) GetWebSocketURL(w http.ResponseWriter, r *http.Request) {
	vizID := chi.URLParam(r, "vizId")

	wsURL, err := h.useCase.GetWebSocketURL(vizID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"wsUrl": wsURL})
}

func (h *VisualizationHandler) Delete(w http.ResponseWriter, r *http.Request) {
	vizID := chi.URLParam(r, "vizId")

	if err := h.useCase.Delete(vizID); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *VisualizationHandler) ListBySimulation(w http.ResponseWriter, r *http.Request) {
	simID := chi.URLParam(r, "simId")

	vizList, err := h.useCase.ListBySimulation(simID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, vizList)
}