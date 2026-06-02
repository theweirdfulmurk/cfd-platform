package usecase

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
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
		storagePath: "/pvc/simulations",
	}
}

// CreateWithFile receives a validated .tar.gz upload, extracts it into the
// shared PVC and creates an MPIJob via the k8s manager.
func (uc *SimulationUseCase) CreateWithFile(
	name string,
	simType domain.SimulationType,
	numProcs int,
	schedulerName string,
	file io.Reader,
	filename string,
) (*domain.Simulation, error) {
	simID := uuid.New().String()[:8]

	simDir := filepath.Join(uc.storagePath, simID)
	if err := os.MkdirAll(simDir, 0o755); err != nil {
		return nil, fmt.Errorf("create simulation dir: %w", err)
	}

	if err := extractTarGz(file, simDir); err != nil {
		return nil, fmt.Errorf("extract archive: %w", err)
	}

	sim := &domain.Simulation{
		ID:            simID,
		Name:          name,
		Type:          simType,
		Status:        domain.SimStatusPending,
		NumProcs:      numProcs,
		SchedulerName: schedulerName,
		PodName:       fmt.Sprintf("sim-%s", simID),
		ResultPath:    fmt.Sprintf("results/%s", simID),
		ConfigPath:    simID, // relative path inside the PVC
		CreatedAt:     time.Now(),
	}

	if err := uc.k8sManager.CreateJob(sim); err != nil {
		return nil, fmt.Errorf("create MPIJob: %w", err)
	}
	if err := uc.repo.Create(sim); err != nil {
		return nil, fmt.Errorf("persist simulation: %w", err)
	}
	return sim, nil
}

// extractTarGz unpacks a .tar.gz stream into dstDir. Paths containing
// `..` are rejected to prevent zip-slip.
func extractTarGz(src io.Reader, dstDir string) error {
	gzr, err := gzip.NewReader(src)
	if err != nil {
		return err
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		// zip-slip guard: reject absolute paths and parent traversal.
		clean := filepath.Clean(hdr.Name)
		if filepath.IsAbs(clean) || hasParentTraversal(clean) {
			return fmt.Errorf("refusing unsafe path in archive: %q", hdr.Name)
		}
		dst := filepath.Join(dstDir, clean)
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(dst, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
				return err
			}
			out, err := os.Create(dst)
			if err != nil {
				return err
			}
			if _, err := io.Copy(out, tr); err != nil {
				out.Close()
				return err
			}
			out.Close()
		}
	}
}

// hasParentTraversal reports whether any segment of a tar entry name is
// "..". tar headers always use '/' as the separator regardless of host OS.
func hasParentTraversal(p string) bool {
	return slices.Contains(strings.Split(p, "/"), "..")
}

func (uc *SimulationUseCase) GetByID(simID string) (*domain.Simulation, error) {
	sim, err := uc.repo.GetByID(simID)
	if err != nil {
		return nil, err
	}
	if status, err := uc.k8sManager.GetJobStatus(simID); err == nil && status != sim.Status {
		sim.Status = status
		if status == domain.SimStatusCompleted {
			now := time.Now()
			sim.CompletedAt = &now
		}
		_ = uc.repo.Update(sim)
	}
	return sim, nil
}

func (uc *SimulationUseCase) List() ([]*domain.Simulation, error) {
	sims, err := uc.repo.List()
	if err != nil {
		return nil, err
	}
	for _, sim := range sims {
		if status, err := uc.k8sManager.GetJobStatus(sim.ID); err == nil && status != sim.Status {
			sim.Status = status
			if status == domain.SimStatusCompleted {
				now := time.Now()
				sim.CompletedAt = &now
			}
			_ = uc.repo.Update(sim)
		}
	}
	return sims, nil
}

func (uc *SimulationUseCase) Delete(simID string) error {
	if err := uc.k8sManager.DeleteJob(simID); err != nil {
		return fmt.Errorf("delete MPIJob: %w", err)
	}
	if err := uc.repo.Delete(simID); err != nil {
		return fmt.Errorf("delete simulation: %w", err)
	}
	return nil
}
