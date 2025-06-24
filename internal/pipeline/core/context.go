package core

import (
	"fmt"
	"strings"
	"sync"
)

// Context is the thread-safe execution context for pipelines
type Context struct {
	mu         sync.RWMutex
	values     map[string]interface{}
	parent     *Context               // For foreach sub-contexts
	readonly   bool                   // For immutable snapshots
	cache      map[string]interface{} // Cache for GetAll()
	cacheDirty bool                   // Whether cache needs rebuild
}

// NewContext creates a new execution context
func NewContext() *Context {
	return &Context{
		values:     make(map[string]interface{}),
		cache:      nil,
		cacheDirty: true,
	}
}

// Fork creates a sub-context for foreach iterations
func (c *Context) Fork() *Context {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return &Context{
		values:     make(map[string]interface{}),
		parent:     c,
		cacheDirty: true,
	}
}

// Set stores a value in the context
func (c *Context) Set(key string, value interface{}) error {
	if c.readonly {
		return fmt.Errorf("cannot modify readonly context")
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.values[key] = value
	c.cacheDirty = true // Invalidate cache
	return nil
}

// Get retrieves a value from the context
func (c *Context) Get(key string) (interface{}, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// Check local context first
	if val, ok := c.values[key]; ok {
		return val, true
	}

	// Check parent context if exists
	if c.parent != nil {
		return c.parent.Get(key)
	}

	return nil, false
}

// GetPath retrieves a nested value using dot notation (e.g., "user.profile.name")
func (c *Context) GetPath(path string) (interface{}, bool) {
	parts := strings.Split(path, ".")

	// Get the root object
	current, ok := c.Get(parts[0])
	if !ok {
		return nil, false
	}

	// Navigate through the path
	for i := 1; i < len(parts); i++ {
		switch v := current.(type) {
		case map[string]interface{}:
			current, ok = v[parts[i]]
			if !ok {
				return nil, false
			}
		case map[string]string:
			current, ok = v[parts[i]]
			if !ok {
				return nil, false
			}
		default:
			// Try array index access
			if idx := parseArrayIndex(parts[i]); idx >= 0 {
				if arr, ok := current.([]interface{}); ok && idx < len(arr) {
					current = arr[idx]
				} else {
					return nil, false
				}
			} else {
				return nil, false
			}
		}
	}

	return current, true
}

// GetAll returns all values in the context (including parent values)
// Uses caching to avoid recreating the map on every call
func (c *Context) GetAll() map[string]interface{} {
	c.mu.RLock()

	// Fast path: cache is valid
	if !c.cacheDirty && c.cache != nil {
		// Return cached copy
		result := make(map[string]interface{}, len(c.cache))
		for k, v := range c.cache {
			result[k] = v
		}
		c.mu.RUnlock()
		return result
	}

	c.mu.RUnlock()

	// Slow path: rebuild cache
	c.mu.Lock()
	defer c.mu.Unlock()

	// Double-check after acquiring write lock
	if !c.cacheDirty && c.cache != nil {
		result := make(map[string]interface{}, len(c.cache))
		for k, v := range c.cache {
			result[k] = v
		}
		return result
	}

	// Build new cache
	cache := make(map[string]interface{})

	// Start with parent values if exists
	if c.parent != nil {
		for k, v := range c.parent.GetAll() {
			cache[k] = v
		}
	}

	// Override with local values
	for k, v := range c.values {
		cache[k] = v
	}

	c.cache = cache
	c.cacheDirty = false

	// Return a copy of the cache
	result := make(map[string]interface{}, len(cache))
	for k, v := range cache {
		result[k] = v
	}

	return result
}

// Snapshot creates a read-only copy of the context
func (c *Context) Snapshot() *Context {
	c.mu.RLock()
	defer c.mu.RUnlock()

	valuesCopy := make(map[string]interface{})
	for k, v := range c.values {
		valuesCopy[k] = v
	}

	return &Context{
		values:   valuesCopy,
		parent:   c.parent,
		readonly: true,
	}
}

// parseArrayIndex extracts array index from strings like "[0]" or "0"
func parseArrayIndex(s string) int {
	s = strings.TrimPrefix(strings.TrimSuffix(s, "]"), "[")

	var idx int
	if _, err := fmt.Sscanf(s, "%d", &idx); err == nil {
		return idx
	}
	return -1
}
