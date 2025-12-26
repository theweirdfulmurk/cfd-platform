package domain

import "time"

// Simulation represents a CFD/FEA computation task
type Simulation struct {
	ID          string
	Name        string
	Type        SimulationType
	Status      SimulationStatus
	PodName     string
	ResultPath  string
	ConfigPath  string
	CreatedAt   time.Time
	StartedAt   *time.Time
	CompletedAt *time.Time
}

// SimulationType defines the simulation solver type
type SimulationType string

const (
	SimTypeCFD SimulationType = "cfd" // OpenFOAM
	SimTypeFEA SimulationType = "fea" // CalculiX
)

// SimulationStatus represents the current state of simulation
type SimulationStatus string

const (
	SimStatusPending   SimulationStatus = "pending"
	SimStatusRunning   SimulationStatus = "running"
	SimStatusCompleted SimulationStatus = "completed"
	SimStatusFailed    SimulationStatus = "failed"
)

// SimulationRepository defines the interface for simulation data access
type SimulationRepository interface {
	Create(sim *Simulation) error
	GetByID(id string) (*Simulation, error)
	List() ([]*Simulation, error)
	Update(sim *Simulation) error
	Delete(id string) error
}

// SimulationK8sManager defines the interface for Kubernetes operations
type SimulationK8sManager interface {
	CreateJob(simID string, simType SimulationType, configPath string) error
	GetJobStatus(simID string) (SimulationStatus, error)
	DeleteJob(simID string) error
}