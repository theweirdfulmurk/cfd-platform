package usecase

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/theweirdfulmurk/cfd-platform/internal/domain"
)

type VisualizationUseCase struct {
	repo       domain.VisualizationRepository
	k8sManager domain.VisualizationK8sManager
}

func NewVisualizationUseCase(
	repo domain.VisualizationRepository,
	k8s domain.VisualizationK8sManager,
) *VisualizationUseCase {
	return &VisualizationUseCase{
		repo:       repo,
		k8sManager: k8s,
	}
}

func (uc *VisualizationUseCase) Create(simulationID, resultPath string) (*domain.Visualization, error) {
	vizID := uuid.New().String()[:8]
	
	viz := &domain.Visualization{
		ID:           vizID,
		SimulationID: simulationID,
		Status:       domain.VizStatusPending,
		PodName:      fmt.Sprintf("viz-%s", vizID),
		ResultPath:   resultPath,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	if err := uc.k8sManager.CreatePod(vizID, resultPath); err != nil {
		return nil, fmt.Errorf("failed to create visualization pod: %w", err)
	}

	if err := uc.repo.Create(viz); err != nil {
		return nil, fmt.Errorf("failed to save visualization: %w", err)
	}

	return viz, nil
}

func (uc *VisualizationUseCase) GetByID(vizID string) (*domain.Visualization, error) {
	viz, err := uc.repo.GetByID(vizID)
	if err != nil {
		return nil, err
	}

	// Update status from k8s
	status, err := uc.k8sManager.GetPodStatus(vizID)
	if err == nil && status != viz.Status {
		viz.Status = status
		viz.UpdatedAt = time.Now()
		uc.repo.Update(viz)
	}

	return viz, nil
}

func (uc *VisualizationUseCase) GetWebSocketURL(vizID string) (string, error) {
	viz, err := uc.repo.GetByID(vizID)
	if err != nil {
		return "", err
	}

	if viz.Status != domain.VizStatusReady {
		return "", fmt.Errorf("visualization not ready, current status: %s", viz.Status)
	}

	podIP, err := uc.k8sManager.GetPodIP(vizID)
	if err != nil {
		return "", fmt.Errorf("failed to get pod IP: %w", err)
	}

	wsURL := fmt.Sprintf("ws://%s:9000/ws", podIP)
	viz.WebSocketURL = wsURL
	viz.UpdatedAt = time.Now()
	uc.repo.Update(viz)

	return wsURL, nil
}

func (uc *VisualizationUseCase) Delete(vizID string) error {
	if err := uc.k8sManager.DeletePod(vizID); err != nil {
		return fmt.Errorf("failed to delete pod: %w", err)
	}

	if err := uc.repo.Delete(vizID); err != nil {
		return fmt.Errorf("failed to delete visualization record: %w", err)
	}

	return nil
}

func (uc *VisualizationUseCase) ListBySimulation(simulationID string) ([]*domain.Visualization, error) {
	return uc.repo.GetBySimulationID(simulationID)
}