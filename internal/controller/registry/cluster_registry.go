package registry

import (
	"sync"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

type ClusterClientRegistry struct {
	mu      sync.RWMutex
	clients map[string]client.Client
}

func NewClusterClientRegistry() *ClusterClientRegistry {
	return &ClusterClientRegistry{clients: make(map[string]client.Client)}
}

func (r *ClusterClientRegistry) Get(name string) (client.Client, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	c, ok := r.clients[name]
	return c, ok
}

func (r *ClusterClientRegistry) Set(name string, c client.Client) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.clients[name] = c
}

func (r *ClusterClientRegistry) Delete(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.clients, name)
}
