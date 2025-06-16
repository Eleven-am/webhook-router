package routing

import (
	"fmt"
	"hash/fnv"
	"math/rand"
	"sync"
	"time"
)

// BasicDestinationManager implements the DestinationManager interface
type BasicDestinationManager struct {
	healthStatus    map[string]bool            // destination health status
	metrics         map[string]DestinationMetrics // destination metrics
	circuitBreakers map[string]*CircuitBreaker // circuit breakers per destination
	roundRobinIndex map[string]int             // round robin counters per rule
	sessionMap      map[string]string          // session to destination mapping
	mu              sync.RWMutex
}

// CircuitBreaker implements circuit breaker pattern
type CircuitBreaker struct {
	config        CircuitBreakerConfig
	state         string // closed, open, half_open
	failures      int
	lastFailTime  time.Time
	halfOpenCount int
	mu            sync.RWMutex
}

func NewBasicDestinationManager() *BasicDestinationManager {
	return &BasicDestinationManager{
		healthStatus:    make(map[string]bool),
		metrics:         make(map[string]DestinationMetrics),
		circuitBreakers: make(map[string]*CircuitBreaker),
		roundRobinIndex: make(map[string]int),
		sessionMap:      make(map[string]string),
	}
}

func (dm *BasicDestinationManager) SelectDestination(destinations []RouteDestination, strategy LoadBalancingConfig, request *RouteRequest) (*RouteDestination, error) {
	if len(destinations) == 0 {
		return nil, ErrNoDestinationsAvailable
	}
	
	// Filter out destinations with open circuit breakers
	availableDestinations := make([]RouteDestination, 0)
	for _, dest := range destinations {
		if !dm.isCircuitBreakerOpen(dest.ID) {
			availableDestinations = append(availableDestinations, dest)
		}
	}
	
	if len(availableDestinations) == 0 {
		return nil, ErrCircuitBreakerOpen
	}
	
	// Check for sticky session
	if strategy.StickySession && strategy.SessionKey != "" {
		sessionValue := dm.getSessionValue(request, strategy.SessionKey)
		if sessionValue != "" {
			dm.mu.RLock()
			destID, exists := dm.sessionMap[sessionValue]
			dm.mu.RUnlock()
			
			if exists {
				// Find the destination
				for _, dest := range availableDestinations {
					if dest.ID == destID {
						return &dest, nil
					}
				}
			}
		}
	}
	
	// Select destination based on strategy
	var selectedDest *RouteDestination
	var err error
	
	switch strategy.Strategy {
	case "round_robin", "":
		selectedDest, err = dm.selectRoundRobin(availableDestinations, request)
	case "weighted":
		selectedDest, err = dm.selectWeighted(availableDestinations)
	case "least_connections":
		selectedDest, err = dm.selectLeastConnections(availableDestinations)
	case "random":
		selectedDest, err = dm.selectRandom(availableDestinations)
	case "hash":
		selectedDest, err = dm.selectHash(availableDestinations, strategy.HashKey, request)
	default:
		return nil, fmt.Errorf("unsupported load balancing strategy: %s", strategy.Strategy)
	}
	
	if err != nil {
		return nil, err
	}
	
	// Update session mapping for sticky sessions
	if strategy.StickySession && strategy.SessionKey != "" {
		sessionValue := dm.getSessionValue(request, strategy.SessionKey)
		if sessionValue != "" {
			dm.mu.Lock()
			dm.sessionMap[sessionValue] = selectedDest.ID
			dm.mu.Unlock()
		}
	}
	
	return selectedDest, nil
}

func (dm *BasicDestinationManager) selectRoundRobin(destinations []RouteDestination, request *RouteRequest) (*RouteDestination, error) {
	if len(destinations) == 0 {
		return nil, ErrNoDestinationsAvailable
	}
	
	// Use request ID or path as key for round robin counter
	key := request.ID
	if key == "" {
		key = request.Path
	}
	
	dm.mu.Lock()
	index := dm.roundRobinIndex[key]
	dm.roundRobinIndex[key] = (index + 1) % len(destinations)
	dm.mu.Unlock()
	
	return &destinations[index], nil
}

func (dm *BasicDestinationManager) selectWeighted(destinations []RouteDestination) (*RouteDestination, error) {
	if len(destinations) == 0 {
		return nil, ErrNoDestinationsAvailable
	}
	
	// Calculate total weight
	totalWeight := 0
	for _, dest := range destinations {
		weight := dest.Weight
		if weight <= 0 {
			weight = 1 // Default weight
		}
		totalWeight += weight
	}
	
	if totalWeight == 0 {
		return &destinations[0], nil
	}
	
	// Select based on weight
	random := rand.Intn(totalWeight)
	currentWeight := 0
	
	for _, dest := range destinations {
		weight := dest.Weight
		if weight <= 0 {
			weight = 1
		}
		currentWeight += weight
		if random < currentWeight {
			return &dest, nil
		}
	}
	
	// Fallback to first destination
	return &destinations[0], nil
}

func (dm *BasicDestinationManager) selectLeastConnections(destinations []RouteDestination) (*RouteDestination, error) {
	if len(destinations) == 0 {
		return nil, ErrNoDestinationsAvailable
	}
	
	dm.mu.RLock()
	defer dm.mu.RUnlock()
	
	// Find destination with least active connections
	var selectedDest *RouteDestination
	minConnections := int64(-1)
	
	for _, dest := range destinations {
		metrics, exists := dm.metrics[dest.ID]
		if !exists {
			// No metrics means no connections, select this one
			destCopy := dest
			return &destCopy, nil
		}
		
		activeConnections := metrics.TotalRequests - metrics.SuccessfulRequests - metrics.FailedRequests
		if minConnections == -1 || activeConnections < minConnections {
			minConnections = activeConnections
			destCopy := dest
			selectedDest = &destCopy
		}
	}
	
	if selectedDest == nil {
		return &destinations[0], nil
	}
	
	return selectedDest, nil
}

func (dm *BasicDestinationManager) selectRandom(destinations []RouteDestination) (*RouteDestination, error) {
	if len(destinations) == 0 {
		return nil, ErrNoDestinationsAvailable
	}
	
	index := rand.Intn(len(destinations))
	return &destinations[index], nil
}

func (dm *BasicDestinationManager) selectHash(destinations []RouteDestination, hashKey string, request *RouteRequest) (*RouteDestination, error) {
	if len(destinations) == 0 {
		return nil, ErrNoDestinationsAvailable
	}
	
	// Get hash value based on hash key
	var hashValue string
	switch hashKey {
	case "ip":
		hashValue = request.RemoteAddr
	case "user_agent":
		hashValue = request.UserAgent
	case "path":
		hashValue = request.Path
	default:
		// Assume it's a header name
		hashValue = request.Headers[hashKey]
	}
	
	if hashValue == "" {
		// Fallback to request ID
		hashValue = request.ID
	}
	
	// Calculate hash
	hasher := fnv.New32a()
	hasher.Write([]byte(hashValue))
	hash := hasher.Sum32()
	
	// Select destination based on hash
	index := int(hash) % len(destinations)
	return &destinations[index], nil
}

func (dm *BasicDestinationManager) getSessionValue(request *RouteRequest, sessionKey string) string {
	// Try to get session value from headers first
	if value := request.Headers[sessionKey]; value != "" {
		return value
	}
	
	// Try to get from query parameters
	if value := request.QueryParams[sessionKey]; value != "" {
		return value
	}
	
	// Try to get from context
	if value, exists := request.Context[sessionKey]; exists {
		return fmt.Sprintf("%v", value)
	}
	
	return ""
}

func (dm *BasicDestinationManager) UpdateDestinationHealth(destinationID string, healthy bool) error {
	dm.mu.Lock()
	defer dm.mu.Unlock()
	
	dm.healthStatus[destinationID] = healthy
	
	// Update metrics
	if metrics, exists := dm.metrics[destinationID]; exists {
		metrics.HealthStatus = map[bool]string{true: "healthy", false: "unhealthy"}[healthy]
		dm.metrics[destinationID] = metrics
	}
	
	return nil
}

func (dm *BasicDestinationManager) GetDestinationHealth(destinationID string) (bool, error) {
	dm.mu.RLock()
	defer dm.mu.RUnlock()
	
	health, exists := dm.healthStatus[destinationID]
	if !exists {
		// Default to healthy if no health status recorded
		return true, nil
	}
	
	return health, nil
}

func (dm *BasicDestinationManager) RecordDestinationMetrics(destinationID string, latency time.Duration, success bool) error {
	dm.mu.Lock()
	defer dm.mu.Unlock()
	
	metrics, exists := dm.metrics[destinationID]
	if !exists {
		metrics = DestinationMetrics{
			HealthStatus: "healthy",
			CircuitState: "closed",
		}
	}
	
	metrics.TotalRequests++
	if success {
		metrics.SuccessfulRequests++
		dm.updateCircuitBreakerSuccess(destinationID)
	} else {
		metrics.FailedRequests++
		dm.updateCircuitBreakerFailure(destinationID)
	}
	
	// Update average latency (simple moving average)
	if metrics.TotalRequests == 1 {
		metrics.AverageLatency = latency
	} else {
		// Weighted average
		metrics.AverageLatency = time.Duration(
			(int64(metrics.AverageLatency)*(metrics.TotalRequests-1) + int64(latency)) / metrics.TotalRequests,
		)
	}
	
	dm.metrics[destinationID] = metrics
	return nil
}

func (dm *BasicDestinationManager) updateCircuitBreakerSuccess(destinationID string) {
	cb, exists := dm.circuitBreakers[destinationID]
	if !exists {
		return
	}
	
	cb.mu.Lock()
	defer cb.mu.Unlock()
	
	if cb.state == "half_open" {
		cb.halfOpenCount++
		if cb.halfOpenCount >= cb.config.HalfOpenMax {
			cb.state = "closed"
			cb.failures = 0
			cb.halfOpenCount = 0
		}
	} else if cb.state == "closed" {
		cb.failures = 0
	}
	
	// Update metrics
	if metrics, exists := dm.metrics[destinationID]; exists {
		metrics.CircuitState = cb.state
		dm.metrics[destinationID] = metrics
	}
}

func (dm *BasicDestinationManager) updateCircuitBreakerFailure(destinationID string) {
	cb, exists := dm.circuitBreakers[destinationID]
	if !exists {
		// Create circuit breaker with default config
		cb = &CircuitBreaker{
			config: CircuitBreakerConfig{
				Enabled:          true,
				FailureThreshold: 5,
				RecoveryTimeout:  30 * time.Second,
				HalfOpenMax:      3,
			},
			state: "closed",
		}
		dm.circuitBreakers[destinationID] = cb
	}
	
	cb.mu.Lock()
	defer cb.mu.Unlock()
	
	cb.failures++
	cb.lastFailTime = time.Now()
	
	if cb.state == "closed" && cb.failures >= cb.config.FailureThreshold {
		cb.state = "open"
	} else if cb.state == "half_open" {
		cb.state = "open"
		cb.halfOpenCount = 0
	}
	
	// Update metrics
	if metrics, exists := dm.metrics[destinationID]; exists {
		metrics.CircuitState = cb.state
		dm.metrics[destinationID] = metrics
	}
}

func (dm *BasicDestinationManager) isCircuitBreakerOpen(destinationID string) bool {
	dm.mu.RLock()
	cb, exists := dm.circuitBreakers[destinationID]
	dm.mu.RUnlock()
	
	if !exists {
		return false // No circuit breaker means it's closed
	}
	
	cb.mu.Lock()
	defer cb.mu.Unlock()
	
	if cb.state == "open" {
		// Check if recovery timeout has passed
		if time.Since(cb.lastFailTime) >= cb.config.RecoveryTimeout {
			cb.state = "half_open"
			cb.halfOpenCount = 0
			return false
		}
		return true
	}
	
	return false
}

func (dm *BasicDestinationManager) GetDestinationMetrics(destinationID string) (DestinationMetrics, error) {
	dm.mu.RLock()
	defer dm.mu.RUnlock()
	
	metrics, exists := dm.metrics[destinationID]
	if !exists {
		return DestinationMetrics{
			HealthStatus: "unknown",
			CircuitState: "closed",
		}, nil
	}
	
	return metrics, nil
}

func (dm *BasicDestinationManager) SetCircuitBreakerConfig(destinationID string, config CircuitBreakerConfig) error {
	dm.mu.Lock()
	defer dm.mu.Unlock()
	
	cb, exists := dm.circuitBreakers[destinationID]
	if !exists {
		cb = &CircuitBreaker{
			state: "closed",
		}
		dm.circuitBreakers[destinationID] = cb
	}
	
	cb.mu.Lock()
	cb.config = config
	cb.mu.Unlock()
	
	return nil
}