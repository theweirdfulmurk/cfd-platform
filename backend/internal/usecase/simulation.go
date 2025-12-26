package usecase

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/theweirdfulmurk/cfd-platform/internal/domain"
)

type SimulationUseCase struct {
	repo       domain.SimulationRepository
	k8sManager domain.SimulationK8sManager
}

func NewSimulationUseCase(
	repo domain.SimulationRepository,
	k8s domain.SimulationK8sManager,
) *SimulationUseCase {
	return &SimulationUseCase{
		repo:       repo,
		k8sManager: k8s,
	}
}

func (uc *SimulationUseCase) Create(name string, simType domain.SimulationType, configPath string) (*domain.Simulation, error) {
	simID := uuid.New().String()[:8]
	now := time.Now()

	sim := &domain.Simulation{
		ID:         simID,
		Name:       name,
		Type:       simType,
		Status:     domain.SimStatusPending,
		PodName:    fmt.Sprintf("sim-%s", simID),
		ResultPath: fmt.Sprintf("results/%s", simID),
		ConfigPath: configPath,
		CreatedAt:  now,
	}

	if err := uc.k8sManager.CreateJob(simID, simType, configPath); err != nil {
		return nil, fmt.Errorf("failed to create simulation job: %w", err)
	}

	if err := uc.repo.Create(sim); err != nil {
		return nil, fmt.Errorf("failed to save simulation: %w", err)
	}

	return sim, nil
}

func (uc *SimulationUseCase) GetByID(simID string) (*domain.Simulation, error) {
	sim, err := uc.repo.GetByID(simID)
	if err != nil {
		return nil, err
	}

	// Update status from k8s
	status, err := uc.k8sManager.GetJobStatus(simID)
	if err == nil && status != sim.Status {
		sim.Status = status
		
		now := time.Now()
		if status == domain.SimStatusRunning && sim.StartedAt == nil {
			sim.StartedAt = &now
		}
		if status == domain.SimStatusCompleted || status == domain.SimStatusFailed {
			sim.CompletedAt = &now
		}
		
		uc.repo.Update(sim)
	}

	return sim, nil
}

func (uc *SimulationUseCase) List() ([]*domain.Simulation, error) {
	return uc.repo.List()
}

func (uc *SimulationUseCase) Delete(simID string) error {
	if err := uc.k8sManager.DeleteJob(simID); err != nil {
		return fmt.Errorf("failed to delete job: %w", err)
	}

	if err := uc.repo.Delete(simID); err != nil {
		return fmt.Errorf("failed to delete simulation record: %w", err)
	}

	return nil
}