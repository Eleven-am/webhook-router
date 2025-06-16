package routing

import (
	"context"
	"time"
	"webhook-router/internal/brokers"
)

// RouteRequest represents an incoming request to be routed
type RouteRequest struct {
	ID          string                 `json:"id"`
	Method      string                 `json:"method"`
	Path        string                 `json:"path"`
	Headers     map[string]string      `json:"headers"`
	Body        []byte                 `json:"body"`
	QueryParams map[string]string      `json:"query_params"`
	RemoteAddr  string                 `json:"remote_addr"`
	UserAgent   string                 `json:"user_agent"`
	Timestamp   time.Time              `json:"timestamp"`
	Context     map[string]interface{} `json:"context"` // Additional context data
}

// RouteDestination represents where a request should be routed
type RouteDestination struct {
	ID           string                 `json:"id"`
	Type         string                 `json:"type"`          // broker, http, webhook, pipeline
	Name         string                 `json:"name"`
	Config       map[string]interface{} `json:"config"`        // Destination-specific configuration
	Priority     int                    `json:"priority"`      // Higher number = higher priority
	Weight       int                    `json:"weight"`        // For load balancing
	HealthCheck  HealthCheckConfig      `json:"health_check"`  // Health check configuration
	Retry        RetryConfig            `json:"retry"`         // Retry configuration
	CircuitBreaker CircuitBreakerConfig `json:"circuit_breaker"` // Circuit breaker configuration
}

// HealthCheckConfig defines health check settings for destinations
type HealthCheckConfig struct {
	Enabled          bool          `json:"enabled"`
	Interval         time.Duration `json:"interval"`
	Timeout          time.Duration `json:"timeout"`
	HealthyThreshold int           `json:"healthy_threshold"`   // Consecutive successes to mark healthy
	UnhealthyThreshold int         `json:"unhealthy_threshold"` // Consecutive failures to mark unhealthy
	Path             string        `json:"path"`                // Health check endpoint path
	ExpectedStatus   int           `json:"expected_status"`     // Expected HTTP status code
}

// RetryConfig defines retry behavior for failed requests
type RetryConfig struct {
	Enabled        bool          `json:"enabled"`
	MaxAttempts    int           `json:"max_attempts"`
	InitialDelay   time.Duration `json:"initial_delay"`
	BackoffType    string        `json:"backoff_type"`    // fixed, exponential, linear
	MaxDelay       time.Duration `json:"max_delay"`
	RetryableErrors []string      `json:"retryable_errors"` // Error patterns that should trigger retry
}

// CircuitBreakerConfig defines circuit breaker settings
type CircuitBreakerConfig struct {
	Enabled        bool          `json:"enabled"`
	FailureThreshold int         `json:"failure_threshold"` // Failures before opening circuit
	RecoveryTimeout  time.Duration `json:"recovery_timeout"`  // Time before attempting recovery
	HalfOpenMax     int           `json:"half_open_max"`     // Max requests in half-open state
}

// RouteRule defines routing logic
type RouteRule struct {
	ID            string            `json:"id"`
	Name          string            `json:"name"`
	Priority      int               `json:"priority"`
	Enabled       bool              `json:"enabled"`
	Conditions    []RuleCondition   `json:"conditions"`
	Destinations  []RouteDestination `json:"destinations"`
	LoadBalancing LoadBalancingConfig `json:"load_balancing"`
	Tags          []string          `json:"tags"`
	Description   string            `json:"description"`
	CreatedAt     time.Time         `json:"created_at"`
	UpdatedAt     time.Time         `json:"updated_at"`
}

// RuleCondition defines a condition for route matching
type RuleCondition struct {
	Type     string      `json:"type"`     // path, header, body, method, query, script
	Field    string      `json:"field"`    // Field name (for header, query conditions)
	Operator string      `json:"operator"` // eq, ne, contains, regex, exists, gt, lt, in
	Value    interface{} `json:"value"`    // Expected value
	Negate   bool        `json:"negate"`   // Negate the condition result
}

// LoadBalancingConfig defines load balancing strategy
type LoadBalancingConfig struct {
	Strategy    string `json:"strategy"`     // round_robin, weighted, least_connections, random, hash
	HashKey     string `json:"hash_key"`     // For hash-based load balancing (header name, ip, etc.)
	StickySession bool `json:"sticky_session"` // Enable session stickiness
	SessionKey   string `json:"session_key"`  // Key for session stickiness
}

// RouteResult represents the result of routing a request
type RouteResult struct {
	RequestID     string             `json:"request_id"`
	MatchedRules  []RouteRule        `json:"matched_rules"`
	Destinations  []RouteDestination `json:"destinations"`
	ProcessingTime time.Duration     `json:"processing_time"`
	Metadata      map[string]interface{} `json:"metadata"`
	Error         string             `json:"error,omitempty"`
}

// Router interface defines the contract for routing engines
type Router interface {
	// Route processes a request and returns routing decisions
	Route(ctx context.Context, request *RouteRequest) (*RouteResult, error)
	
	// AddRule adds a new routing rule
	AddRule(rule *RouteRule) error
	
	// UpdateRule updates an existing routing rule
	UpdateRule(rule *RouteRule) error
	
	// RemoveRule removes a routing rule
	RemoveRule(ruleID string) error
	
	// GetRule returns a specific routing rule
	GetRule(ruleID string) (*RouteRule, error)
	
	// GetRules returns all routing rules
	GetRules() ([]*RouteRule, error)
	
	// Health returns the health status of the router
	Health() error
	
	// Start starts the router
	Start(ctx context.Context) error
	
	// Stop stops the router
	Stop() error
	
	// GetMetrics returns routing metrics
	GetMetrics() (*RouterMetrics, error)
}

// RouterMetrics contains routing performance metrics
type RouterMetrics struct {
	TotalRequests     int64             `json:"total_requests"`
	SuccessfulRoutes  int64             `json:"successful_routes"`
	FailedRoutes      int64             `json:"failed_routes"`
	AverageLatency    time.Duration     `json:"average_latency"`
	RuleHitCounts     map[string]int64  `json:"rule_hit_counts"`
	DestinationMetrics map[string]DestinationMetrics `json:"destination_metrics"`
}

// DestinationMetrics contains metrics for individual destinations
type DestinationMetrics struct {
	TotalRequests    int64         `json:"total_requests"`
	SuccessfulRequests int64       `json:"successful_requests"`
	FailedRequests   int64         `json:"failed_requests"`
	AverageLatency   time.Duration `json:"average_latency"`
	CurrentWeight    int           `json:"current_weight"`
	HealthStatus     string        `json:"health_status"`
	CircuitState     string        `json:"circuit_state"`
}

// DestinationManager manages destination health and load balancing
type DestinationManager interface {
	// SelectDestination selects a destination based on load balancing strategy
	SelectDestination(destinations []RouteDestination, strategy LoadBalancingConfig, request *RouteRequest) (*RouteDestination, error)
	
	// UpdateDestinationHealth updates the health status of a destination
	UpdateDestinationHealth(destinationID string, healthy bool) error
	
	// GetDestinationHealth returns the health status of a destination
	GetDestinationHealth(destinationID string) (bool, error)
	
	// RecordDestinationMetrics records metrics for a destination
	RecordDestinationMetrics(destinationID string, latency time.Duration, success bool) error
}

// RuleEngine processes routing rules and conditions
type RuleEngine interface {
	// EvaluateRule evaluates if a request matches a routing rule
	EvaluateRule(rule *RouteRule, request *RouteRequest) (bool, error)
	
	// EvaluateCondition evaluates a single condition
	EvaluateCondition(condition *RuleCondition, request *RouteRequest) (bool, error)
	
	// CompileRule pre-processes a rule for faster evaluation
	CompileRule(rule *RouteRule) error
	
	// GetSupportedOperators returns list of supported operators
	GetSupportedOperators() []string
	
	// GetSupportedConditionTypes returns list of supported condition types
	GetSupportedConditionTypes() []string
}

// Executor handles the actual routing of requests to destinations
type Executor interface {
	// Execute routes a request to the selected destinations
	Execute(ctx context.Context, request *RouteRequest, destinations []RouteDestination) error
	
	// ExecuteWithBroker routes a request using a message broker
	ExecuteWithBroker(ctx context.Context, request *RouteRequest, broker brokers.Broker, destination RouteDestination) error
	
	// ExecuteWithHTTP routes a request using HTTP
	ExecuteWithHTTP(ctx context.Context, request *RouteRequest, destination RouteDestination) error
	
	// ExecuteWithPipeline routes a request through a data pipeline
	ExecuteWithPipeline(ctx context.Context, request *RouteRequest, destination RouteDestination) error
}

// RoutingEngine is the main routing engine that orchestrates all components
type RoutingEngine struct {
	router      Router
	ruleEngine  RuleEngine
	destManager DestinationManager
	executor    Executor
	metrics     *RouterMetrics
	isRunning   bool
}

func NewRoutingEngine(router Router, ruleEngine RuleEngine, destManager DestinationManager, executor Executor) *RoutingEngine {
	return &RoutingEngine{
		router:      router,
		ruleEngine:  ruleEngine,
		destManager: destManager,
		executor:    executor,
		metrics: &RouterMetrics{
			RuleHitCounts:      make(map[string]int64),
			DestinationMetrics: make(map[string]DestinationMetrics),
		},
		isRunning: false,
	}
}

func (re *RoutingEngine) Start(ctx context.Context) error {
	if re.isRunning {
		return ErrEngineAlreadyRunning
	}
	
	if err := re.router.Start(ctx); err != nil {
		return err
	}
	
	re.isRunning = true
	return nil
}

func (re *RoutingEngine) Stop() error {
	if !re.isRunning {
		return ErrEngineNotRunning
	}
	
	if err := re.router.Stop(); err != nil {
		return err
	}
	
	re.isRunning = false
	return nil
}

func (re *RoutingEngine) ProcessRequest(ctx context.Context, request *RouteRequest) (*RouteResult, error) {
	if !re.isRunning {
		return nil, ErrEngineNotRunning
	}
	
	start := time.Now()
	
	// Route the request
	result, err := re.router.Route(ctx, request)
	if err != nil {
		re.metrics.FailedRoutes++
		return nil, err
	}
	
	// Execute routing to destinations
	if len(result.Destinations) > 0 {
		if err := re.executor.Execute(ctx, request, result.Destinations); err != nil {
			re.metrics.FailedRoutes++
			return nil, err
		}
	}
	
	// Update metrics
	re.metrics.TotalRequests++
	re.metrics.SuccessfulRoutes++
	result.ProcessingTime = time.Since(start)
	
	// Update rule hit counts
	for _, rule := range result.MatchedRules {
		re.metrics.RuleHitCounts[rule.ID]++
	}
	
	return result, nil
}

func (re *RoutingEngine) GetMetrics() *RouterMetrics {
	return re.metrics
}

func (re *RoutingEngine) Health() error {
	if !re.isRunning {
		return ErrEngineNotRunning
	}
	return re.router.Health()
}