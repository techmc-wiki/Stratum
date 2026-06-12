package runtimeprofile

import (
	"fmt"
	"sort"
	"sync"
)

type Registry struct {
	mu       sync.RWMutex
	profiles map[string]Profile
}

func NewRegistry(values ...Profile) (*Registry, error) {
	registry := &Registry{profiles: map[string]Profile{}}
	if err := registry.RegisterAll(values); err != nil {
		return nil, err
	}
	return registry, nil
}

func Builtins() *Registry {
	registry, err := NewRegistry(DummyProcess())
	if err != nil {
		panic(err)
	}
	return registry
}

func (r *Registry) Register(value Profile) error {
	return r.RegisterAll([]Profile{value})
}

func (r *Registry) RegisterAll(values []Profile) error {
	pending := make(map[string]Profile, len(values))
	for _, value := range values {
		if err := Validate(value); err != nil {
			return err
		}
		if _, exists := pending[value.ID]; exists {
			return fmt.Errorf("runtime profile %q is duplicated in registration batch", value.ID)
		}
		pending[value.ID] = value
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	for id := range pending {
		if _, exists := r.profiles[id]; exists {
			return fmt.Errorf("runtime profile %q already registered", id)
		}
	}
	for id, value := range pending {
		r.profiles[id] = value
	}
	return nil
}

func (r *Registry) Get(id string) (Profile, error) {
	if id == "" {
		id = DefaultProfileID
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	value, ok := r.profiles[id]
	if !ok {
		return Profile{}, fmt.Errorf("runtime profile %q is not registered", id)
	}
	if !value.Enabled {
		return Profile{}, fmt.Errorf("runtime profile %q is disabled", id)
	}
	return value, nil
}

func (r *Registry) ListEnabled() []Profile {
	r.mu.RLock()
	defer r.mu.RUnlock()
	values := make([]Profile, 0, len(r.profiles))
	for _, value := range r.profiles {
		if value.Enabled {
			values = append(values, value)
		}
	}
	sort.Slice(values, func(i, j int) bool { return values[i].ID < values[j].ID })
	return values
}
