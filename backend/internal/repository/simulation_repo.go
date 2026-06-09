package repository

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"

	"github.com/theweirdfulmurk/cfd-platform/internal/domain"
)

// InMemorySimulationRepo keeps simulations in a map for fast reads and, when
// given a non-empty indexPath, mirrors every mutation to a JSON file on the
// shared PVC. Without that file-backing a backend pod restart (rollout, crash,
// OOM) would silently drop the entire run history — the records live only in
// RAM. load-on-start + write-through makes the listing survive restarts.
type InMemorySimulationRepo struct {
	mu        sync.RWMutex
	data      map[string]*domain.Simulation
	indexPath string
}

// NewInMemorySimulationRepo loads any persisted index at indexPath (pass "" to
// disable persistence). A missing/unreadable index starts empty — never fatal.
func NewInMemorySimulationRepo(indexPath string) *InMemorySimulationRepo {
	r := &InMemorySimulationRepo{
		data:      make(map[string]*domain.Simulation),
		indexPath: indexPath,
	}
	r.load()
	return r
}

// load reads the persisted index into the map. Best-effort: any error leaves
// the repo empty rather than failing startup.
func (r *InMemorySimulationRepo) load() {
	if r.indexPath == "" {
		return
	}
	b, err := os.ReadFile(r.indexPath)
	if err != nil {
		return
	}
	var loaded map[string]*domain.Simulation
	if json.Unmarshal(b, &loaded) == nil && loaded != nil {
		r.data = loaded
	}
}

// persist writes the current map to indexPath atomically (temp file + rename so
// a crash mid-write can't truncate the index). Caller must hold the lock.
func (r *InMemorySimulationRepo) persist() {
	if r.indexPath == "" {
		return
	}
	b, err := json.MarshalIndent(r.data, "", "  ")
	if err != nil {
		return
	}
	tmp := r.indexPath + ".tmp"
	if os.WriteFile(tmp, b, 0o644) != nil {
		return
	}
	_ = os.MkdirAll(filepath.Dir(r.indexPath), 0o755)
	_ = os.Rename(tmp, r.indexPath)
}

func (r *InMemorySimulationRepo) Create(sim *domain.Simulation) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.data[sim.ID] = sim
	r.persist()
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
	r.persist()
	return nil
}

func (r *InMemorySimulationRepo) Delete(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.data, id)
	r.persist()
	return nil
}
