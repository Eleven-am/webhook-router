// Package enrichers provides HTTP-based data enrichment capabilities with OAuth2 support,
// caching, rate limiting, and distributed backends for the webhook router.
//
// The enrichers package enables webhook payloads to be enhanced with additional data
// from external HTTP APIs. It supports various authentication methods, implements
// intelligent caching and rate limiting, and can work in both local and distributed
// (Redis-backed) configurations.
//
// # Core Components
//
// HTTPEnricher: The main enrichment engine that performs HTTP requests to external
// services to fetch additional data. Supports GET/POST methods, multiple authentication
// types, template-based URL and body construction, and configurable retry logic.
//
// TokenManager: Handles OAuth 2.0 token lifecycle management including automatic
// token refresh, concurrent request handling, and rate limiting of token requests.
// Supports both client_credentials and refresh_token grant types.
//
// Caching: Provides both local (LRU with TTL) and distributed (Redis-backed) response
// caching to reduce API calls and improve performance. The local cache uses an efficient
// LRU eviction policy with background cleanup of expired entries.
//
// Rate Limiting: Implements token bucket rate limiting for both local and distributed
// scenarios. The local rate limiter uses a token bucket algorithm while the distributed
// version leverages Redis sliding window counters.
//
// # Authentication Support
//
// The package supports multiple authentication methods:
//   - OAuth 2.0 (client credentials and refresh token flows)
//   - Basic Authentication
//   - Bearer Token
//   - API Key (with configurable header)
//   - Custom headers
//
// # Template Support
//
// Dynamic URL construction and request body templating using Go template syntax:
//
//	config := &HTTPConfig{
//		URLTemplate: "https://api.example.com/users/{{.user_id}}/profile",
//		BodyTemplate: `{"query": "{{.search_term}}", "limit": 10}`,
//		HeaderTemplate: map[string]string{
//			"X-User-ID": "{{.user_id}}",
//		},
//	}
//
// # Caching Configuration
//
// Local caching with LRU eviction:
//
//	cache := &CacheConfig{
//		Enabled: true,
//		TTL:     5 * time.Minute,
//		MaxSize: 1000,
//	}
//
// Distributed caching with Redis:
//
//	config := &HTTPConfig{
//		Cache:          cache,
//		UseDistributed: true,
//		RedisClient:    redisClient,
//	}
//
// # Rate Limiting Configuration
//
// Local rate limiting:
//
//	rateLimit := &RateLimitConfig{
//		RequestsPerSecond: 10,
//		BurstSize:         5,
//	}
//
// Distributed rate limiting with Redis:
//
//	config := &HTTPConfig{
//		RateLimit:      rateLimit,
//		UseDistributed: true,
//		RedisClient:    redisClient,
//	}
//
// # Usage Example
//
//	// Configure enricher with OAuth2 and caching
//	config := &HTTPConfig{
//		URL:    "https://api.example.com/enrich",
//		Method: "POST",
//		Auth: &AuthConfig{
//			Type: "oauth2",
//			OAuth2: &OAuth2Config{
//				TokenURL:     "https://oauth.example.com/token",
//				ClientID:     "your-client-id",
//				ClientSecret: "your-client-secret",
//				GrantType:    "client_credentials",
//				Scope:        []string{"read", "write"},
//			},
//		},
//		Cache: &CacheConfig{
//			Enabled: true,
//			TTL:     10 * time.Minute,
//			MaxSize: 500,
//		},
//		RateLimit: &RateLimitConfig{
//			RequestsPerSecond: 20,
//			BurstSize:         10,
//		},
//		Retry: &RetryConfig{
//			MaxRetries:  3,
//			RetryDelay:  1 * time.Second,
//			BackoffType: "exponential",
//		},
//	}
//
//	enricher, err := NewHTTPEnricher(config)
//	if err != nil {
//		log.Fatal(err)
//	}
//
//	// Enrich webhook data
//	data := map[string]interface{}{
//		"user_id": "12345",
//		"action":  "purchase",
//	}
//
//	enrichedData, err := enricher.Enrich(context.Background(), data)
//	if err != nil {
//		log.Printf("Enrichment failed: %v", err)
//		return
//	}
//
//	// Use enriched data...
//
// # Distributed Architecture
//
// The package is designed to work in distributed environments where multiple
// webhook router instances need to coordinate caching and rate limiting:
//
//   - Distributed cache uses Redis as a shared backend with JSON serialization
//   - Distributed rate limiting uses Redis sliding window counters
//   - OAuth2 tokens can be shared across instances (when configured)
//   - Health checks ensure Redis connectivity
//
// # Error Handling
//
// The package provides comprehensive error handling:
//   - Network timeouts and retries with configurable backoff
//   - OAuth2 token refresh failures with rate limiting
//   - Cache and rate limiter failures gracefully degrade
//   - Context cancellation support throughout
//
// # Performance Characteristics
//
//   - Local cache: Sub-millisecond lookups with O(1) operations
//   - Distributed cache: ~1ms Redis roundtrip for cache hits
//   - Rate limiting: Minimal overhead, ~microseconds for local, ~1ms for distributed
//   - OAuth2: Automatic token refresh with concurrent request batching
//   - Memory efficient: LRU eviction prevents unbounded growth
//
// # Thread Safety
//
// All components are thread-safe and designed for concurrent use:
//   - HTTPEnricher can handle concurrent enrichment requests
//   - TokenManager uses read-write mutexes for efficient token access
//   - Caches use appropriate locking mechanisms
//   - Rate limiters are safe for concurrent token acquisition
package enrichers