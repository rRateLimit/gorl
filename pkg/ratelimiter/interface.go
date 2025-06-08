// Package ratelimiter provides a public interface for implementing custom rate limiting algorithms.
package ratelimiter

import (
	"context"
	"fmt"
	"sync"
)

// RateLimiter defines the interface that all rate limiting algorithms must implement.
// This is the public interface that users can implement to create custom rate limiters.
type RateLimiter interface {
	// Allow blocks until the rate limiter allows a request to proceed.
	// It returns an error if the context is cancelled or times out.
	Allow(ctx context.Context) error

	// String returns a human-readable description of the rate limiter.
	String() string
}

// Factory is a function type that creates a new instance of a custom rate limiter.
// The rate parameter specifies the target rate in requests per second.
type Factory func(rate float64) RateLimiter

// Registry manages custom rate limiting algorithm implementations.
type Registry struct {
	mu        sync.RWMutex
	factories map[string]Factory
}

// NewRegistry creates a new registry for custom rate limiting algorithms.
func NewRegistry() *Registry {
	return &Registry{
		factories: make(map[string]Factory),
	}
}

// Register registers a new custom rate limiting algorithm with the given name.
// The factory function will be called to create instances of the algorithm.
func (r *Registry) Register(name string, factory Factory) error {
	if name == "" {
		return fmt.Errorf("algorithm name cannot be empty")
	}
	if factory == nil {
		return fmt.Errorf("factory function cannot be nil")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.factories[name]; exists {
		return fmt.Errorf("algorithm '%s' is already registered", name)
	}

	r.factories[name] = factory
	return nil
}

// Unregister removes a custom rate limiting algorithm from the registry.
func (r *Registry) Unregister(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.factories[name]; !exists {
		return fmt.Errorf("algorithm '%s' is not registered", name)
	}

	delete(r.factories, name)
	return nil
}

// Create creates a new instance of the specified custom rate limiting algorithm.
func (r *Registry) Create(name string, rate float64) (RateLimiter, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	factory, exists := r.factories[name]
	if !exists {
		return nil, fmt.Errorf("algorithm '%s' is not registered", name)
	}

	return factory(rate), nil
}

// List returns a list of all registered custom algorithm names.
func (r *Registry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.factories))
	for name := range r.factories {
		names = append(names, name)
	}
	return names
}

// IsRegistered checks if an algorithm with the given name is registered.
func (r *Registry) IsRegistered(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	_, exists := r.factories[name]
	return exists
}

// DefaultRegistry is the global registry instance used by the application.
var DefaultRegistry = NewRegistry()

// Register registers a new custom rate limiting algorithm in the default registry.
func Register(name string, factory Factory) error {
	return DefaultRegistry.Register(name, factory)
}

// Unregister removes a custom rate limiting algorithm from the default registry.
func Unregister(name string) error {
	return DefaultRegistry.Unregister(name)
}

// Create creates a new instance of the specified custom rate limiting algorithm from the default registry.
func Create(name string, rate float64) (RateLimiter, error) {
	return DefaultRegistry.Create(name, rate)
}

// List returns a list of all registered custom algorithm names from the default registry.
func List() []string {
	return DefaultRegistry.List()
}

// IsRegistered checks if an algorithm with the given name is registered in the default registry.
func IsRegistered(name string) bool {
	return DefaultRegistry.IsRegistered(name)
}
