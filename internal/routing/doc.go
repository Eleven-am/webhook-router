// Package routing provides a comprehensive message routing engine for the webhook router system.
// It implements a rule-based routing system with support for complex conditions, multiple
// destinations, load balancing strategies, and health-aware routing decisions.
//
// # Overview
//
// The routing package is the core decision engine that determines how incoming webhook
// requests are processed and where they should be delivered. It provides:
//
//   - Rule-based routing with priority ordering
//   - Complex condition evaluation (path, headers, body content, etc.)
//   - Multiple destination support with load balancing
//   - Health-aware routing with circuit breaker patterns
//   - Metrics collection and performance monitoring
//   - Thread-safe concurrent request processing
//
// # Architecture
//
// ## Core Components
//
// ### Router
// The Router is the main entry point that orchestrates the routing process:
//   - Manages routing rules with priority ordering
//   - Evaluates rules in sequence to find matches
//   - Delegates to RuleEngine for condition evaluation
//   - Uses DestinationManager for destination selection
//
// ### RuleEngine
// The RuleEngine handles condition evaluation and rule compilation:
//   - Compiles rules for optimized evaluation
//   - Supports various condition types (path, method, headers, etc.)
//   - Implements comparison operators (eq, contains, regex, etc.)
//   - Provides numeric comparisons and CIDR matching
//
// ### DestinationManager
// The DestinationManager handles destination selection and health:
//   - Implements multiple load balancing strategies
//   - Tracks destination health and metrics
//   - Provides circuit breaker functionality
//   - Supports sticky sessions and connection limits
//
// ## Data Flow
//
//  1. Request arrives at Router.Route()
//  2. Router iterates through rules by priority
//  3. RuleEngine evaluates each rule's conditions
//  4. On match, DestinationManager selects destinations
//  5. Router returns selected destinations with metadata
//
// # Usage
//
// ## Basic Setup
//
//	// Create components
//	ruleEngine := routing.NewBasicRuleEngine()
//	destManager := routing.NewBasicDestinationManager()
//
//	// Create router
//	router := routing.NewBasicRouter(ruleEngine, destManager)
//	router.Start()
//	defer router.Stop()
//
//	// Add routing rule
//	rule := &routing.RouteRule{
//	    ID:       "api-v1-rule",
//	    Name:     "API v1 Traffic",
//	    Priority: 100,
//	    Enabled:  true,
//	    Conditions: []routing.RuleCondition{
//	        {Type: "path", Operator: "starts_with", Value: "/api/v1/"},
//	        {Type: "method", Operator: "in", Value: []string{"GET", "POST"}},
//	    },
//	    Destinations: []routing.RouteDestination{
//	        {ID: "api-server-1", Type: "http", Config: map[string]interface{}{
//	            "url": "http://api1.internal:8080",
//	        }},
//	        {ID: "api-server-2", Type: "http", Config: map[string]interface{}{
//	            "url": "http://api2.internal:8080",
//	        }},
//	    },
//	    LoadBalancing: &routing.LoadBalancingConfig{
//	        Strategy: "round_robin",
//	    },
//	}
//	router.AddRule(rule)
//
//	// Route request
//	request := &routing.RouteRequest{
//	    Path:   "/api/v1/users",
//	    Method: "GET",
//	    Headers: map[string]string{
//	        "User-Agent": "MyApp/1.0",
//	    },
//	}
//	result, err := router.Route(context.Background(), request)
//
// # Rule Conditions
//
// ## Condition Types
//
// The routing engine supports various condition types:
//
//   - **path**: URL path matching
//   - **method**: HTTP method matching
//   - **header**: HTTP header matching
//   - **query**: Query parameter matching
//   - **body**: Request body content matching
//   - **body_json**: JSON field extraction and matching
//   - **remote_addr**: Client IP address matching
//   - **user_agent**: User-Agent header matching
//   - **custom**: Custom field matching
//
// ## Operators
//
// Each condition type supports various operators:
//
//   - **eq**: Exact equality
//   - **neq**: Not equal
//   - **contains**: Substring match
//   - **not_contains**: Substring not present
//   - **starts_with**: Prefix match
//   - **ends_with**: Suffix match
//   - **regex**: Regular expression match
//   - **in**: Value in set
//   - **not_in**: Value not in set
//   - **gt**, **gte**, **lt**, **lte**: Numeric comparisons
//   - **cidr**: CIDR network matching (for IPs)
//
// ## Examples
//
//	// Path prefix matching
//	{Type: "path", Operator: "starts_with", Value: "/api/"}
//
//	// Header presence check
//	{Type: "header", Operator: "exists", Value: "X-API-Key"}
//
//	// JSON field extraction
//	{Type: "body_json", Operator: "eq", Value: 42, Field: "user.age"}
//
//	// CIDR matching for IPs
//	{Type: "remote_addr", Operator: "cidr", Value: "10.0.0.0/8"}
//
//	// Regex pattern matching
//	{Type: "path", Operator: "regex", Value: "^/users/[0-9]+$"}
//
// # Load Balancing
//
// ## Strategies
//
// The destination manager supports multiple load balancing strategies:
//
// ### Round Robin
// Distributes requests evenly across all healthy destinations:
//
//	LoadBalancing: &LoadBalancingConfig{
//	    Strategy: "round_robin",
//	}
//
// ### Weighted Round Robin
// Distributes based on destination weights:
//
//	LoadBalancing: &LoadBalancingConfig{
//	    Strategy: "weighted",
//	}
//	// Set weights in destination config
//	Destination.Config["weight"] = 10
//
// ### Random
// Randomly selects from healthy destinations:
//
//	LoadBalancing: &LoadBalancingConfig{
//	    Strategy: "random",
//	}
//
// ### Least Connections
// Routes to destination with fewest active connections:
//
//	LoadBalancing: &LoadBalancingConfig{
//	    Strategy: "least_connections",
//	}
//
// ### Consistent Hashing
// Routes based on request property hash:
//
//	LoadBalancing: &LoadBalancingConfig{
//	    Strategy: "hash",
//	    HashKey:  "remote_addr", // or header name
//	}
//
// ### Sticky Sessions
// Maintains session affinity:
//
//	LoadBalancing: &LoadBalancingConfig{
//	    Strategy:       "sticky",
//	    SessionCookie:  "JSESSIONID",
//	    SessionTimeout: 3600, // seconds
//	}
//
// # Health Management
//
// ## Health Tracking
//
// The destination manager tracks health for each destination:
//
//	// Update health status
//	destManager.UpdateDestinationHealth("dest-1", &DestinationHealth{
//	    Healthy:       false,
//	    LastCheck:     time.Now(),
//	    ConsecutiveFails: 3,
//	    Error:         "connection refused",
//	})
//
// ## Circuit Breaker
//
// Automatic circuit breaker protects unhealthy destinations:
//
//	rule.CircuitBreaker = &CircuitBreakerConfig{
//	    FailureThreshold:   5,      // failures before opening
//	    RecoveryTime:       60,     // seconds before half-open
//	    SampleSize:         10,     // requests to track
//	}
//
// # Metrics and Monitoring
//
// ## Metrics Collection
//
// The router collects various metrics:
//
//	metrics := router.GetMetrics()
//	// Returns RouterMetrics with:
//	// - Total requests processed
//	// - Success/failure counts
//	// - Rule evaluation times
//	// - Destination selection times
//
// ## Destination Metrics
//
//	destMetrics := destManager.GetDestinationMetrics("dest-1")
//	// Returns DestinationMetrics with:
//	// - Request count
//	// - Success/failure rates
//	// - Average latency
//	// - Active connections
//
// # Advanced Features
//
// ## Rule Compilation
//
// Rules can be pre-compiled for better performance:
//
//	err := ruleEngine.CompileRule(rule)
//	// Optimizes regex patterns, validates conditions
//
// ## Request Metadata
//
// Requests can carry metadata for routing decisions:
//
//	request := &RouteRequest{
//	    Path: "/api/users",
//	    Metadata: map[string]interface{}{
//	        "user_id":    12345,
//	        "tenant":     "acme-corp",
//	        "priority":   "high",
//	    },
//	}
//
// ## Custom Conditions
//
// Use custom conditions for application-specific routing:
//
//	{
//	    Type:     "custom",
//	    Field:    "metadata.tenant",
//	    Operator: "eq",
//	    Value:    "premium-customer",
//	}
//
// # Error Handling
//
// The routing package provides detailed error types:
//
//	result, err := router.Route(ctx, request)
//	if err != nil {
//	    switch err.(type) {
//	    case *NoMatchingRuleError:
//	        // No rules matched the request
//	    case *NoHealthyDestinationsError:
//	        // All destinations are unhealthy
//	    case *InvalidRuleError:
//	        // Rule configuration is invalid
//	    }
//	}
//
// # Best Practices
//
// ## Rule Design
//   - Order rules by priority (higher priority evaluated first)
//   - Use specific conditions before generic ones
//   - Combine conditions for precise matching
//   - Test rules thoroughly before deployment
//
// ## Performance
//   - Pre-compile rules with regex patterns
//   - Use appropriate load balancing strategies
//   - Monitor destination health actively
//   - Set reasonable circuit breaker thresholds
//
// ## Reliability
//   - Always configure health checks
//   - Use circuit breakers for external services
//   - Set appropriate timeouts
//   - Monitor metrics and adjust strategies
//
// # Thread Safety
//
// All components in the routing package are thread-safe and designed for
// concurrent use. The router, rule engine, and destination manager can
// handle multiple simultaneous routing requests without external synchronization.
//
// # Testing
//
// The package includes comprehensive test coverage (77.4%) with tests for:
//   - Rule evaluation logic
//   - Load balancing algorithms
//   - Health management
//   - Error conditions
//   - Concurrent access patterns
package routing
