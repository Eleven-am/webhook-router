// Package routing provides a comprehensive routing engine for webhook requests
// with support for complex rule-based routing, load balancing, health checking,
// circuit breakers, and retry mechanisms.
//
// The routing system consists of several key components:
//
// 1. Router: Manages routing rules and processes incoming requests
// 2. RuleEngine: Evaluates routing conditions and rules
// 3. DestinationManager: Handles load balancing and destination health
// 4. Executor: Executes the actual routing to destinations
//
// Key Features:
//
// - Rule-based routing with complex conditions
// - Multiple load balancing strategies (round-robin, weighted, least-connections, hash, random)
// - Health checking with configurable thresholds
// - Circuit breaker pattern for fault tolerance
// - Retry mechanisms with exponential backoff
// - Session stickiness support
// - Comprehensive metrics and monitoring
//
// Example usage:
//
//	// Create components
//	ruleEngine := routing.NewBasicRuleEngine()
//	destManager := routing.NewBasicDestinationManager()
//	router := routing.NewBasicRouter(ruleEngine, destManager)
//
//	// Define a routing rule
//	rule := &routing.RouteRule{
//		ID:   "api-users",
//		Name: "API Users Route",
//		Priority: 10,
//		Enabled:  true,
//		Conditions: []routing.RuleCondition{
//			{Type: "path", Operator: "starts_with", Value: "/api/users"},
//			{Type: "method", Operator: "in", Value: []string{"GET", "POST"}},
//		},
//		Destinations: []routing.RouteDestination{
//			{
//				ID:   "users-service",
//				Type: "http",
//				Config: map[string]interface{}{
//					"url": "http://users-service:8080",
//				},
//				Weight: 100,
//			},
//		},
//		LoadBalancing: routing.LoadBalancingConfig{
//			Strategy: "round_robin",
//		},
//	}
//
//	// Add rule to router
//	router.AddRule(rule)
//
//	// Start router
//	ctx := context.Background()
//	router.Start(ctx)
//
//	// Route a request
//	request := &routing.RouteRequest{
//		ID:     "req-123",
//		Method: "GET",
//		Path:   "/api/users/123",
//		Headers: map[string]string{
//			"Content-Type": "application/json",
//		},
//	}
//
//	result, err := router.Route(ctx, request)
//	if err != nil {
//		log.Printf("Routing failed: %v", err)
//	} else {
//		log.Printf("Routed to %d destinations", len(result.Destinations))
//	}
package routing

import (
	"context"
	"time"
	"webhook-router/internal/brokers"
)

// RouteRequest represents an incoming HTTP request that needs to be routed
// to one or more destinations based on configured routing rules.
//
// The request contains all relevant information needed for routing decisions
// including HTTP method, path, headers, query parameters, body content,
// and additional context data.
type RouteRequest struct {
	ID          string                 `json:"id"`           // Unique request identifier
	Method      string                 `json:"method"`       // HTTP method (GET, POST, etc.)
	Path        string                 `json:"path"`         // Request path (e.g., "/api/users")
	Headers     map[string]string      `json:"headers"`      // HTTP headers
	Body        []byte                 `json:"body"`         // Request body content
	QueryParams map[string]string      `json:"query_params"` // URL query parameters
	RemoteAddr  string                 `json:"remote_addr"`  // Client IP address
	UserAgent   string                 `json:"user_agent"`   // Client user agent string
	Timestamp   time.Time              `json:"timestamp"`    // When the request was received
	Context     map[string]interface{} `json:"context"`      // Additional context data for routing
}

// RouteDestination represents a target destination where routed requests
// should be sent. Destinations can be HTTP endpoints, message brokers,
// webhooks, or data processing pipelines.
//
// Each destination includes configuration for health checking, retry logic,
// and circuit breaker behavior to ensure reliable request delivery.
type RouteDestination struct {
	ID           string                 `json:"id"`             // Unique destination identifier
	Type         string                 `json:"type"`           // Destination type: broker, http, webhook, pipeline
	Name         string                 `json:"name"`           // Human-readable destination name
	Config       map[string]interface{} `json:"config"`         // Destination-specific configuration (URLs, credentials, etc.)
	Priority     int                    `json:"priority"`       // Routing priority (higher number = higher priority)
	Weight       int                    `json:"weight"`         // Weight for load balancing (higher = more traffic)
	HealthCheck  HealthCheckConfig      `json:"health_check"`   // Health check configuration
	Retry        RetryConfig            `json:"retry"`          // Retry behavior configuration
	CircuitBreaker CircuitBreakerConfig `json:"circuit_breaker"` // Circuit breaker configuration for fault tolerance
}

// HealthCheckConfig defines health monitoring settings for destinations.
// Health checks help determine if a destination is available and can
// handle requests reliably.
type HealthCheckConfig struct {
	Enabled            bool          `json:"enabled"`             // Whether health checking is enabled
	Interval           time.Duration `json:"interval"`            // How often to perform health checks
	Timeout            time.Duration `json:"timeout"`             // Request timeout for health checks
	HealthyThreshold   int           `json:"healthy_threshold"`   // Consecutive successes needed to mark as healthy
	UnhealthyThreshold int           `json:"unhealthy_threshold"` // Consecutive failures needed to mark as unhealthy
	Path               string        `json:"path"`                // Health check endpoint path (e.g., "/health")
	ExpectedStatus     int           `json:"expected_status"`     // Expected HTTP status code (e.g., 200)
}

// RetryConfig defines retry behavior for failed request deliveries.
// This configuration controls how the system handles temporary failures
// when delivering requests to destinations.
type RetryConfig struct {
	Enabled         bool          `json:"enabled"`          // Whether retry is enabled
	MaxAttempts     int           `json:"max_attempts"`     // Maximum number of retry attempts
	InitialDelay    time.Duration `json:"initial_delay"`    // Initial delay before first retry
	BackoffType     string        `json:"backoff_type"`     // Backoff strategy: fixed, exponential, linear
	MaxDelay        time.Duration `json:"max_delay"`        // Maximum delay between retries
	RetryableErrors []string      `json:"retryable_errors"` // Error patterns that should trigger retry (regex patterns)
}

// CircuitBreakerConfig defines circuit breaker settings for fault tolerance.
// Circuit breakers prevent cascading failures by temporarily stopping
// requests to failing destinations.
//
// States:
// - Closed: Normal operation, requests flow through
// - Open: Failures detected, requests are rejected immediately
// - Half-Open: Testing if destination has recovered
type CircuitBreakerConfig struct {
	Enabled          bool          `json:"enabled"`           // Whether circuit breaker is enabled
	FailureThreshold int           `json:"failure_threshold"` // Number of failures before opening circuit
	RecoveryTimeout  time.Duration `json:"recovery_timeout"`  // Time to wait before attempting recovery
	HalfOpenMax      int           `json:"half_open_max"`     // Maximum requests allowed in half-open state
}

// RouteRule defines the routing logic and configuration for matching and
// directing incoming requests to appropriate destinations.
//
// Rules are evaluated in priority order (higher numbers first) and consist
// of conditions that must be met for the rule to match a request.
// When a rule matches, its destinations become candidates for routing.
type RouteRule struct {
	ID            string            `json:"id"`             // Unique rule identifier
	Name          string            `json:"name"`           // Human-readable rule name
	Priority      int               `json:"priority"`       // Rule priority (higher = evaluated first)
	Enabled       bool              `json:"enabled"`        // Whether the rule is active
	Conditions    []RuleCondition   `json:"conditions"`     // Conditions that must be met for rule to match
	Destinations  []RouteDestination `json:"destinations"`   // Target destinations for matched requests
	LoadBalancing LoadBalancingConfig `json:"load_balancing"` // Load balancing strategy for multiple destinations
	Tags          []string          `json:"tags"`           // Tags for categorization and filtering
	Description   string            `json:"description"`    // Optional rule description
	CreatedAt     time.Time         `json:"created_at"`     // When the rule was created
	UpdatedAt     time.Time         `json:"updated_at"`     // When the rule was last modified
}

// RuleCondition defines a single condition that must be evaluated
// to determine if a request matches a routing rule.
//
// Conditions can test various aspects of the request including path,
// headers, query parameters, body content, and more. Multiple conditions
// in a rule are combined with AND logic (all must match).
type RuleCondition struct {
	Type     string      `json:"type"`     // Condition type: path, header, body, method, query, body_json, remote_addr, user_agent, size, context
	Field    string      `json:"field"`    // Field name (required for header, query, body_json, context conditions)
	Operator string      `json:"operator"` // Comparison operator: eq, ne, contains, regex, exists, gt, lt, gte, lte, in, starts_with, ends_with, cidr
	Value    interface{} `json:"value"`    // Expected value to compare against
	Negate   bool        `json:"negate"`   // Whether to negate the condition result (NOT operation)
}

// LoadBalancingConfig defines the strategy and parameters for distributing
// requests across multiple destinations when a rule has more than one target.
//
// Different strategies provide various distribution algorithms to optimize
// performance, handle failures, and maintain session consistency.
type LoadBalancingConfig struct {
	Strategy      string `json:"strategy"`       // Load balancing algorithm: round_robin, weighted, least_connections, random, hash
	HashKey       string `json:"hash_key"`       // Key for hash-based load balancing (header name, ip, etc.)
	StickySession bool   `json:"sticky_session"` // Whether to maintain session affinity
	SessionKey    string `json:"session_key"`    // Header/query/context key used for session identification
}

// RouteResult represents the outcome of processing a routing request,
// including which rules matched, selected destinations, and performance metrics.
//
// This structure provides comprehensive information about the routing decision
// process and can be used for monitoring, debugging, and analytics.
type RouteResult struct {
	RequestID      string                 `json:"request_id"`      // Original request identifier
	MatchedRules   []RouteRule            `json:"matched_rules"`   // Rules that matched the request
	Destinations   []RouteDestination     `json:"destinations"`    // Selected destinations for routing
	ProcessingTime time.Duration          `json:"processing_time"` // Time taken to process the routing decision
	Metadata       map[string]interface{} `json:"metadata"`        // Additional routing metadata and context
	Error          string                 `json:"error,omitempty"` // Error message if routing failed
}

// Router interface defines the contract for routing engines that process
// incoming requests and determine appropriate destinations based on configured rules.
//
// The router is the main entry point for the routing system and provides
// methods for rule management, request processing, and system lifecycle.
type Router interface {
	// Route processes a request and returns routing decisions based on configured rules
	Route(ctx context.Context, request *RouteRequest) (*RouteResult, error)
	
	// AddRule adds a new routing rule to the router
	AddRule(rule *RouteRule) error
	
	// UpdateRule updates an existing routing rule identified by its ID
	UpdateRule(rule *RouteRule) error
	
	// RemoveRule removes a routing rule by its ID
	RemoveRule(ruleID string) error
	
	// GetRule returns a specific routing rule by its ID
	GetRule(ruleID string) (*RouteRule, error)
	
	// GetRules returns all configured routing rules
	GetRules() ([]*RouteRule, error)
	
	// Health returns the health status of the router and its components
	Health() error
	
	// Start initializes and starts the router with the given context
	Start(ctx context.Context) error
	
	// Stop gracefully shuts down the router
	Stop() error
	
	// GetMetrics returns performance and operational metrics for the router
	GetMetrics() (*RouterMetrics, error)
}

// RouterMetrics contains comprehensive performance and operational metrics
// for the routing system, providing insights into request processing,
// rule effectiveness, and destination performance.
type RouterMetrics struct {
	TotalRequests      int64                         `json:"total_requests"`      // Total number of requests processed
	SuccessfulRoutes   int64                         `json:"successful_routes"`   // Number of successfully routed requests
	FailedRoutes       int64                         `json:"failed_routes"`       // Number of failed routing attempts
	AverageLatency     time.Duration                 `json:"average_latency"`     // Average time to process routing decisions
	RuleHitCounts      map[string]int64              `json:"rule_hit_counts"`     // Per-rule match statistics
	DestinationMetrics map[string]DestinationMetrics `json:"destination_metrics"` // Per-destination performance metrics
}

// DestinationMetrics contains detailed performance and health metrics
// for individual routing destinations, including request statistics,
// latency measurements, and circuit breaker status.
type DestinationMetrics struct {
	TotalRequests      int64         `json:"total_requests"`      // Total requests sent to this destination
	SuccessfulRequests int64         `json:"successful_requests"` // Number of successful requests
	FailedRequests     int64         `json:"failed_requests"`     // Number of failed requests
	AverageLatency     time.Duration `json:"average_latency"`     // Average response latency
	CurrentWeight      int           `json:"current_weight"`      // Current load balancing weight
	HealthStatus       string        `json:"health_status"`       // Health status: healthy, unhealthy, unknown
	CircuitState       string        `json:"circuit_state"`       // Circuit breaker state: closed, open, half-open
}

// DestinationManager manages destination health monitoring, load balancing,
// and performance tracking for routing destinations.
//
// The manager is responsible for selecting appropriate destinations based on
// load balancing strategies, maintaining health status, and collecting metrics.
type DestinationManager interface {
	// SelectDestination selects an appropriate destination from the available options
	// based on the configured load balancing strategy and current destination health
	SelectDestination(destinations []RouteDestination, strategy LoadBalancingConfig, request *RouteRequest) (*RouteDestination, error)
	
	// UpdateDestinationHealth updates the health status of a specific destination
	UpdateDestinationHealth(destinationID string, healthy bool) error
	
	// GetDestinationHealth returns the current health status of a destination
	GetDestinationHealth(destinationID string) (bool, error)
	
	// RecordDestinationMetrics records performance metrics for a destination
	// including request latency and success/failure status
	RecordDestinationMetrics(destinationID string, latency time.Duration, success bool) error
}

// RuleEngine processes routing rules and conditions to determine if incoming
// requests match configured routing criteria.
//
// The engine provides rule compilation for performance optimization and
// supports various condition types and comparison operators.
type RuleEngine interface {
	// EvaluateRule evaluates if a request matches all conditions in a routing rule
	EvaluateRule(rule *RouteRule, request *RouteRequest) (bool, error)
	
	// EvaluateCondition evaluates a single rule condition against a request
	EvaluateCondition(condition *RuleCondition, request *RouteRequest) (bool, error)
	
	// CompileRule pre-processes a rule for faster evaluation (e.g., regex compilation)
	CompileRule(rule *RouteRule) error
	
	// GetSupportedOperators returns a list of all supported comparison operators
	GetSupportedOperators() []string
	
	// GetSupportedConditionTypes returns a list of all supported condition types
	GetSupportedConditionTypes() []string
}

// Executor handles the actual delivery of routed requests to their selected
// destinations using appropriate protocols and methods.
//
// The executor supports multiple delivery mechanisms including HTTP requests,
// message brokers, and data processing pipelines.
type Executor interface {
	// Execute routes a request to all selected destinations using their configured protocols
	Execute(ctx context.Context, request *RouteRequest, destinations []RouteDestination) error
	
	// ExecuteWithBroker routes a request to a destination using a message broker
	ExecuteWithBroker(ctx context.Context, request *RouteRequest, broker brokers.Broker, destination RouteDestination) error
	
	// ExecuteWithHTTP routes a request to a destination using HTTP protocol
	ExecuteWithHTTP(ctx context.Context, request *RouteRequest, destination RouteDestination) error
	
	// ExecuteWithPipeline routes a request through a data processing pipeline
	ExecuteWithPipeline(ctx context.Context, request *RouteRequest, destination RouteDestination) error
}

// RoutingEngine is the main orchestrator that coordinates all routing components
// including the router, rule engine, destination manager, and executor.
//
// The engine provides a high-level interface for processing requests through
// the complete routing pipeline from rule evaluation to request delivery.
type RoutingEngine struct {
	router      Router             // Core router for rule management and request routing
	ruleEngine  RuleEngine         // Rule evaluation engine
	destManager DestinationManager // Destination health and load balancing manager
	executor    Executor           // Request execution and delivery handler
	metrics     *RouterMetrics     // Performance and operational metrics
	isRunning   bool               // Current running state of the engine
}

// NewRoutingEngine creates a new routing engine with the provided components.
// All components are required and the engine will coordinate their interactions
// to provide complete routing functionality.
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

// Start initializes and starts the routing engine and all its components.
// Returns an error if the engine is already running or if any component fails to start.
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

// Stop gracefully shuts down the routing engine and all its components.
// Returns an error if the engine is not currently running or if any component fails to stop.
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

// ProcessRequest processes a complete routing request through the entire pipeline
// including rule evaluation, destination selection, and request execution.
// Returns the routing result with performance metrics and matched rules.
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

// GetMetrics returns the current performance and operational metrics
// for the routing engine including request counts, latencies, and per-rule statistics.
func (re *RoutingEngine) GetMetrics() *RouterMetrics {
	return re.metrics
}

// Health checks the health status of the routing engine and its components.
// Returns an error if the engine is not running or if any component is unhealthy.
func (re *RoutingEngine) Health() error {
	if !re.isRunning {
		return ErrEngineNotRunning
	}
	return re.router.Health()
}