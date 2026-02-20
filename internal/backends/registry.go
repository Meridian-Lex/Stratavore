package backends

import (
	"fmt"
	"sync"
)

// BackendRegistry maps backend name to ModelBackend implementation.
type BackendRegistry struct {
	mu       sync.RWMutex
	backends map[string]ModelBackend
}

func NewBackendRegistry() *BackendRegistry {
	return &BackendRegistry{backends: make(map[string]ModelBackend)}
}

func (r *BackendRegistry) Register(b ModelBackend) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.backends[b.Name()] = b
}

func (r *BackendRegistry) Get(name string) (ModelBackend, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	b, ok := r.backends[name]
	if !ok {
		return nil, fmt.Errorf("backend not registered: %s", name)
	}
	return b, nil
}

func (r *BackendRegistry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.backends))
	for n := range r.backends {
		names = append(names, n)
	}
	return names
}
