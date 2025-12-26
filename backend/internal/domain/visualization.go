package domain

import "time"

// Visualization represents a visualization session for simulation results
type Visualization struct {
	ID           string
	SimulationID string
	Status       VisualizationStatus
	PodName      string
	WebSocketURL string
	ResultPath   string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// VisualizationStatus represents the current state of visualization
type VisualizationStatus string

const (
	VizStatusPending VisualizationStatus = "pending"
	VizStatusRunning VisualizationStatus = "running"
	VizStatusReady   VisualizationStatus = "ready"
	VizStatusFailed  VisualizationStatus = "failed"
)

// VisualizationRepository defines the interface for visualization data access
type VisualizationRepository interface {
	Create(viz *Visualization) error
	GetByID(id string) (*Visualization, error)
	GetBySimulationID(simulationID string) ([]*Visualization, error)
	Update(viz *Visualization) error
	Delete(id string) error
}

// VisualizationK8sManager defines the interface for Kubernetes operations
type VisualizationK8sManager interface {
	CreatePod(vizID, resultPath string) error
	GetPodStatus(vizID string) (VisualizationStatus, error)
	GetPodIP(vizID string) (string, error)
	DeletePod(vizID string) error
}