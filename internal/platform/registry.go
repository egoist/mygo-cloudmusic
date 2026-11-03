package platform

import (
	"errors"
	"fmt"
	"slices"
)

// Registry stores the providers known to an app. It owns no provider
// lifecycle; the app switches and closes sessions explicitly.
type Registry struct {
	providers map[ID]Provider
	order     []ID
}

// NewRegistry builds a registry and rejects duplicate or nil providers.
func NewRegistry(providers ...Provider) (*Registry, error) {
	r := &Registry{providers: make(map[ID]Provider, len(providers))}
	for _, p := range providers {
		if p == nil {
			return nil, errors.New("platform: nil provider")
		}
		if err := r.Register(p); err != nil {
			return nil, err
		}
	}
	return r, nil
}

// Register adds p to the registry. Registering an ID twice is an error so a
// build cannot silently replace the default platform.
func (r *Registry) Register(p Provider) error {
	if p == nil {
		return errors.New("platform: nil provider")
	}
	if r.providers == nil {
		r.providers = make(map[ID]Provider)
	}
	id := p.ID()
	if id == "" {
		return errors.New("platform: provider has no id")
	}
	if _, ok := r.providers[id]; ok {
		return fmt.Errorf("platform: provider %q already registered", id)
	}
	r.providers[id] = p
	r.order = append(r.order, id)
	return nil
}

// Get returns the provider for id, or nil when it is not registered.
func (r *Registry) Get(id ID) Provider {
	if r == nil {
		return nil
	}
	return r.providers[id]
}

// IDs returns the registered IDs in registration order.
func (r *Registry) IDs() []ID {
	if r == nil {
		return nil
	}
	return slices.Clone(r.order)
}
