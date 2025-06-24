package expression

import (
	"fmt"
	"sync"
	"time"

	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/vm"
	gocache "github.com/patrickmn/go-cache"
	"golang.org/x/time/rate"
)

// ExpressionCache with proper memory management using go-cache
type ExpressionCache struct {
	cache       *gocache.Cache
	mu          sync.RWMutex
	maxItems    int
	rateLimiter *rate.Limiter
}

// NewExpressionCache creates a new expression cache with expiration
func NewExpressionCache() *ExpressionCache {
	// 5 minute default expiration, 10 minute cleanup interval
	return &ExpressionCache{
		cache:       gocache.New(5*time.Minute, 10*time.Minute),
		maxItems:    1000,                                 // Limit cache to 1000 items to prevent unbounded growth
		rateLimiter: rate.NewLimiter(rate.Limit(100), 10), // 100 evaluations per second, burst of 10
	}
}

// Global expression cache with proper memory management
var exprCache = NewExpressionCache()

// Evaluate evaluates an expression with the given environment
func Evaluate(expression string, env map[string]interface{}) (interface{}, error) {
	// Apply rate limiting
	if !exprCache.rateLimiter.Allow() {
		return nil, fmt.Errorf("expression evaluation rate limit exceeded")
	}

	// Check cache first
	if cached, found := exprCache.cache.Get(expression); found {
		if program, ok := cached.(*vm.Program); ok {
			return expr.Run(program, env)
		}
	}

	// Compile expression with our custom functions
	options := GetExprOptions(env)
	program, err := expr.Compile(expression, options...)

	if err != nil {
		return nil, fmt.Errorf("failed to compile expression: %w", err)
	}

	// Cache the compiled program with size limit enforcement
	exprCache.mu.Lock()
	defer exprCache.mu.Unlock()

	// Check if we've reached the max items limit
	if exprCache.cache.ItemCount() >= exprCache.maxItems {
		// Remove expired items first
		exprCache.cache.DeleteExpired()

		// If still at limit, skip caching this item
		if exprCache.cache.ItemCount() >= exprCache.maxItems {
			// Don't cache, but still return the result
			result, err := expr.Run(program, env)
			if err != nil {
				return nil, fmt.Errorf("failed to evaluate expression: %w", err)
			}
			return result, nil
		}
	}

	exprCache.cache.Set(expression, program, gocache.DefaultExpiration)

	// Run the expression
	result, err := expr.Run(program, env)
	if err != nil {
		return nil, fmt.Errorf("failed to evaluate expression: %w", err)
	}

	return result, nil
}

// ClearCache clears the expression cache
func ClearCache() {
	exprCache.cache.Flush()
}

// SetCacheExpiration sets the default expiration time for cached expressions
func SetCacheExpiration(expiration time.Duration) {
	exprCache.mu.Lock()
	defer exprCache.mu.Unlock()

	// Create new cache with new expiration settings
	exprCache.cache = gocache.New(expiration, expiration*2)
}
