package enrichers

import (
	"container/list"
	"sync"
	"time"
)

// ResponseCache implements a thread-safe LRU cache with TTL for HTTP responses
type ResponseCache struct {
	config   *CacheConfig
	items    map[string]*cacheItem
	lruList  *list.List
	mu       sync.RWMutex
	stopChan chan struct{}
}

type cacheItem struct {
	key       string
	value     interface{}
	expiresAt time.Time
	element   *list.Element
}

// NewResponseCache creates a new response cache
func NewResponseCache(config *CacheConfig) *ResponseCache {
	if config.MaxSize <= 0 {
		config.MaxSize = 1000 // Default max size
	}
	if config.TTL <= 0 {
		config.TTL = 5 * time.Minute // Default TTL
	}

	cache := &ResponseCache{
		config:   config,
		items:    make(map[string]*cacheItem),
		lruList:  list.New(),
		stopChan: make(chan struct{}),
	}

	// Start cleanup goroutine
	go cache.cleanup()

	return cache
}

// Get retrieves a value from the cache
func (c *ResponseCache) Get(key string) interface{} {
	c.mu.Lock()
	defer c.mu.Unlock()

	item, exists := c.items[key]
	if !exists {
		return nil
	}

	// Check if expired
	if time.Now().After(item.expiresAt) {
		c.removeItem(item)
		return nil
	}

	// Move to front (most recently used)
	c.lruList.MoveToFront(item.element)

	return item.value
}

// Set stores a value in the cache
func (c *ResponseCache) Set(key string, value interface{}) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Check if item already exists
	if existingItem, exists := c.items[key]; exists {
		// Update existing item
		existingItem.value = value
		existingItem.expiresAt = time.Now().Add(c.config.TTL)
		c.lruList.MoveToFront(existingItem.element)
		return
	}

	// Create new item
	item := &cacheItem{
		key:       key,
		value:     value,
		expiresAt: time.Now().Add(c.config.TTL),
	}

	// Add to front of LRU list
	item.element = c.lruList.PushFront(item)
	c.items[key] = item

	// Check if we need to evict items
	if c.lruList.Len() > c.config.MaxSize {
		c.evictLRU()
	}
}

// Delete removes a value from the cache
func (c *ResponseCache) Delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if item, exists := c.items[key]; exists {
		c.removeItem(item)
	}
}

// Clear removes all items from the cache
func (c *ResponseCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.items = make(map[string]*cacheItem)
	c.lruList.Init()
}

// Size returns the current number of items in the cache
func (c *ResponseCache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.items)
}

// Stats returns cache statistics
func (c *ResponseCache) Stats() map[string]interface{} {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return map[string]interface{}{
		"size":     len(c.items),
		"max_size": c.config.MaxSize,
		"ttl":      c.config.TTL.String(),
	}
}

// Stop shuts down the cache cleanup goroutine
func (c *ResponseCache) Stop() {
	select {
	case <-c.stopChan:
		// Already stopped
		return
	default:
		close(c.stopChan)
	}
}

// Ensure ResponseCache implements CacheInterface
var _ CacheInterface = (*ResponseCache)(nil)

// removeItem removes an item from both the map and LRU list
func (c *ResponseCache) removeItem(item *cacheItem) {
	delete(c.items, item.key)
	c.lruList.Remove(item.element)
}

// evictLRU removes the least recently used item
func (c *ResponseCache) evictLRU() {
	if c.lruList.Len() == 0 {
		return
	}

	// Remove from back (least recently used)
	element := c.lruList.Back()
	if element != nil {
		item := element.Value.(*cacheItem)
		c.removeItem(item)
	}
}

// cleanup periodically removes expired items
func (c *ResponseCache) cleanup() {
	ticker := time.NewTicker(time.Minute) // Cleanup every minute
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.cleanupExpired()
		case <-c.stopChan:
			return
		}
	}
}

// cleanupExpired removes all expired items
func (c *ResponseCache) cleanupExpired() {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	var expiredItems []*cacheItem

	// Find expired items
	for _, item := range c.items {
		if now.After(item.expiresAt) {
			expiredItems = append(expiredItems, item)
		}
	}

	// Remove expired items
	for _, item := range expiredItems {
		c.removeItem(item)
	}
}