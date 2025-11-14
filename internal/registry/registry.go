package registry

import (
	"fmt"
	"sync"

	"github.com/XavierBriggs/Mercury/pkg/contracts"
)

// SportRegistry manages registered sport modules
type SportRegistry struct {
	sports map[string]contracts.SportModule
	mu     sync.RWMutex
}

// NewSportRegistry creates a new sport registry
func NewSportRegistry() *SportRegistry {
	return &SportRegistry{
		sports: make(map[string]contracts.SportModule),
	}
}

// Register adds a sport module to the registry
func (r *SportRegistry) Register(sport contracts.SportModule) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	sportKey := sport.GetSportKey()
	if _, exists := r.sports[sportKey]; exists {
		return fmt.Errorf("sport %s is already registered", sportKey)
	}

	r.sports[sportKey] = sport
	return nil
}

// Get retrieves a sport module by key
func (r *SportRegistry) Get(sportKey string) (contracts.SportModule, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	sport, exists := r.sports[sportKey]
	return sport, exists
}

// GetAll returns all registered sports
func (r *SportRegistry) GetAll() []contracts.SportModule {
	r.mu.RLock()
	defer r.mu.RUnlock()

	sports := make([]contracts.SportModule, 0, len(r.sports))
	for _, sport := range r.sports {
		sports = append(sports, sport)
	}
	return sports
}

// Count returns the number of registered sports
func (r *SportRegistry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return len(r.sports)
}




