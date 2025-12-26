package repository

import (
	"sync"

	"github.com/theweirdfulmurk/cfd-platform/internal/domain"
)

type InMemorySimulationRepo struct {
	mu   sync.RWMutex
	data map[string]*domain.Simulation
}

func NewInMemorySimulationRepo() *InMemorySimulationRepo {
	return &InMemorySimulationRepo{
		data: make(map[string]*domain.Simulation),
	}
}

func (r *InMemorySimulationRepo) Create(sim *domain.Simulation) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.data[sim.ID] = sim
	return nil
}

func (r *InMemorySimulationRepo) GetByID(id string) (*domain.Simulation, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	sim, exists := r.data[id]
	if !exists {
		return nil, ErrNotFound
	}
	return sim, nil
}

func (r *InMemorySimulationRepo) List() ([]*domain.Simulation, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]*domain.Simulation, 0, len(r.data))
	for _, sim := range r.data {
		result = append(result, sim)
	}
	return result, nil
}

func (r *InMemorySimulationRepo) Update(sim *domain.Simulation) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.data[sim.ID]; !exists {
		return ErrNotFound
	}
	r.data[sim.ID] = sim
	return nil
}

func (r *InMemorySimulationRepo) Delete(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.data, id)
	return nil
}