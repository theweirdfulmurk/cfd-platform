package usecase

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/theweirdfulmurk/cfd-platform/internal/domain"
)

type SimulationUseCase struct {
	repo        domain.SimulationRepository
	k8sManager  domain.SimulationK8sManager
	storagePath string
}

func NewSimulationUseCase(
	repo domain.SimulationRepository,
	k8s domain.SimulationK8sManager,
) *SimulationUseCase {
	return &SimulationUseCase{
		repo:        repo,
		k8sManager:  k8s,
		storagePath: "/pvc/simulations", // монтируется из PVC
	}
}

func (uc *SimulationUseCase) CreateWithFile(
	name string,
	simType domain.SimulationType,
	file io.Reader,
	filename string,
) (*domain.Simulation, error) {
	simID := uuid.New().String()[:8]

	// Создаём директорию для симуляции
	simDir := filepath.Join(uc.storagePath, simID)
	if err := os.MkdirAll(simDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create simulation directory: %w", err)
	}

	var destPath string
	if simType == domain.SimTypeFEA {
		destPath = filepath.Join(simDir, "input.inp")
	} else {
		destPath = filepath.Join(simDir, filename)
	}

	destFile, err := os.Create(destPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create file: %w", err)
	}
	defer destFile.Close()

	if _, err := io.Copy(destFile, file); err != nil {
		return nil, fmt.Errorf("failed to save file: %w", err)
	}

	now := time.Now()
	sim := &domain.Simulation{
		ID:         simID,
		Name:       name,
		Type:       simType,
		Status:     domain.SimStatusPending,
		PodName:    fmt.Sprintf("sim-%s", simID),
		ResultPath: fmt.Sprintf("results/%s", simID),
		ConfigPath: simID, // путь в PVC
		CreatedAt:  now,
	}

	// Создаём K8s Job
	if err := uc.k8sManager.CreateJob(simID, simType, simID); err != nil {
		return nil, fmt.Errorf("failed to create job: %w", err)
	}

	if err := uc.repo.Create(sim); err != nil {
		return nil, fmt.Errorf("failed to save simulation: %w", err)
	}

	return sim, nil
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
		return nil, fmt.Errorf("failed to create job: %w", err)
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
		if status == domain.SimStatusCompleted {
			now := time.Now()
			sim.CompletedAt = &now
		}
		uc.repo.Update(sim)
	}

	return sim, nil
}

func (uc *SimulationUseCase) List() ([]*domain.Simulation, error) {
	sims, err := uc.repo.List()
	if err != nil {
		return nil, err
	}

	for _, sim := range sims {
		if status, err := uc.k8sManager.GetJobStatus(sim.ID); err == nil {
			sim.Status = status
			if status == domain.SimStatusCompleted {
				now := time.Now()
				sim.CompletedAt = &now
			}
			uc.repo.Update(sim)
		}
	}

	return sims, nil
}

func (uc *SimulationUseCase) Delete(simID string) error {
	if err := uc.k8sManager.DeleteJob(simID); err != nil {
		return fmt.Errorf("failed to delete job: %w", err)
	}

	if err := uc.repo.Delete(simID); err != nil {
		return fmt.Errorf("failed to delete simulation: %w", err)
	}

	return nil
}
