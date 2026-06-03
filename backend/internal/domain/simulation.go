package domain

import "time"

// Simulation represents an MPI-parallel engineering computation task.
type Simulation struct {
	ID            string
	Name          string
	Type          SimulationType
	Status        SimulationStatus
	NumProcs      int    // MPI ranks (parallelism level)
	SchedulerName string // empty = default kube-scheduler; "topology-aware-scheduler" = ours
	PodName       string
	ResultPath    string
	ConfigPath    string
	CreatedAt     time.Time
	StartedAt     *time.Time
	CompletedAt   *time.Time
}

// SimulationType identifies the solver backing the simulation. The three
// solvers cover a spectrum of MPI communication-graph densities, which is
// the basis of the experimental comparison in the thesis.
type SimulationType string

const (
	// SimTypeOpenFOAM — finite-volume CFD. Sparse communication graph
	// (geometric neighbours only), dominated by MPI_Allreduce.
	SimTypeOpenFOAM SimulationType = "openfoam"

	// SimTypeOpenRadioss — explicit-dynamics FEM. Sparse communication
	// graph (boundary nodes only), no global reductions.
	SimTypeOpenRadioss SimulationType = "openradioss"

	// SimTypeCodeAster — implicit FEM with MUMPS direct solver. Dense
	// communication graph due to LU-factorisation fill-in.
	SimTypeCodeAster SimulationType = "code_aster"
)

// IsValid reports whether the value is one of the supported solver types.
func (t SimulationType) IsValid() bool {
	switch t {
	case SimTypeOpenFOAM, SimTypeOpenRadioss, SimTypeCodeAster:
		return true
	}
	return false
}

// SimulationStatus represents the current state of simulation.
type SimulationStatus string

const (
	SimStatusPending   SimulationStatus = "pending"
	SimStatusRunning   SimulationStatus = "running"
	SimStatusCompleted SimulationStatus = "completed"
	SimStatusFailed    SimulationStatus = "failed"
)

// SimulationRepository defines the interface for simulation data access.
type SimulationRepository interface {
	Create(sim *Simulation) error
	GetByID(id string) (*Simulation, error)
	List() ([]*Simulation, error)
	Update(sim *Simulation) error
	Delete(id string) error
}

// SimulationK8sManager defines the interface for Kubernetes operations.
type SimulationK8sManager interface {
	// Extraction: one-shot Job that decomposes the case and writes
	// /scheduler-graphs/<simID>.edgelist for the topology-aware extender.
	// Must complete BEFORE CreateJob so the scheduler has F-graph data
	// at MPIJob placement time.
	CreateExtractionJob(sim *Simulation) error
	GetExtractionStatus(simID string) (string, error) // "pending"|"succeeded"|"failed"
	DeleteExtractionJob(simID string) error

	// MPIJob: actual parallel solver run.
	CreateJob(sim *Simulation) error
	GetJobStatus(simID string) (SimulationStatus, error)
	DeleteJob(simID string) error
}
