package repository

import (
	"errors"
	"sync"

	"github.com/theweirdfulmurk/cfd-platform/internal/domain"
)

var ErrNotFound = errors.New("not found")

type InMemoryVisualizationRepo struct {
	mu   sync.RWMutex
	data map[string]*domain.Visualization
}

func NewInMemoryVisualizationRepo() *InMemoryVisualizationRepo {
	return &InMemoryVisualizationRepo{
		data: make(map[string]*domain.Visualization),
	}
}

func (r *InMemoryVisualizationRepo) Create(viz *domain.Visualization) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.data[viz.ID] = viz
	return nil
}

func (r *InMemoryVisualizationRepo) GetByID(id string) (*domain.Visualization, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	viz, exists := r.data[id]
	if !exists {
		return nil, ErrNotFound
	}
	return viz, nil
}

func (r *InMemoryVisualizationRepo) GetBySimulationID(simulationID string) ([]*domain.Visualization, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*domain.Visualization
	for _, viz := range r.data {
		if viz.SimulationID == simulationID {
			result = append(result, viz)
		}
	}
	return result, nil
}

func (r *InMemoryVisualizationRepo) Update(viz *domain.Visualization) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.data[viz.ID]; !exists {
		return ErrNotFound
	}
	r.data[viz.ID] = viz
	return nil
}

func (r *InMemoryVisualizationRepo) Delete(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.data, id)
	return nil
}