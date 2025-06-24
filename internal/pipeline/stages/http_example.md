# HTTP Stage Configuration Examples

The HTTP stage now supports caching and rate limiting features. Here are examples of how to configure them:

## Basic HTTP Request (No Caching/Rate Limiting)
```json
{
  "id": "fetch-user",
  "type": "http",
  "target": "userData",
  "action": "GET https://api.example.com/users/123"
}
```

## HTTP Request with Caching
```json
{
  "id": "fetch-user-cached",
  "type": "http",
  "target": "userData",
  "action": {
    "method": "GET",
    "url": "https://api.example.com/users/${userId}",
    "cache": {
      "enabled": true,
      "ttl": "5m",
      "type": "local",
      "cacheMethods": ["GET"],
      "cacheStatuses": [200, 201]
    }
  }
}
```

## HTTP Request with Rate Limiting
```json
{
  "id": "fetch-user-limited",
  "type": "http",
  "target": "userData",
  "action": {
    "method": "GET",
    "url": "https://api.example.com/users/${userId}",
    "rateLimit": {
      "enabled": true,
      "requestsPerSecond": 10,
      "burstSize": 20,
      "type": "distributed"
    }
  }
}
```

## Full-Featured HTTP Request
```json
{
  "id": "fetch-user-full",
  "type": "http",
  "target": {
    "body": "userData",
    "status": "statusCode",
    "headers": "responseHeaders"
  },
  "action": {
    "method": "POST",
    "url": "https://api.example.com/users",
    "headers": {
      "Content-Type": "application/json",
      "X-Request-ID": "${requestId}"
    },
    "body": {
      "name": "${userName}",
      "email": "${userEmail}"
    },
    "auth": {
      "type": "oauth2",
      "oauth2ServiceId": "my-api-service"
    },
    "timeout": "30s",
    "retry": {
      "maxAttempts": 3,
      "initialDelay": "1s",
      "maxDelay": "10s",
      "backoffFactor": 2,
      "retryableStatusCodes": [429, 500, 502, 503, 504]
    },
    "circuitBreaker": {
      "enabled": true,
      "failureThreshold": 0.5,
      "resetTimeout": "30s",
      "halfOpenRequests": 3
    },
    "cache": {
      "enabled": true,
      "ttl": "10m",
      "type": "two_tier",
      "cacheMethods": ["GET", "POST"],
      "cacheStatuses": [200]
    },
    "rateLimit": {
      "enabled": true,
      "requestsPerSecond": 100,
      "burstSize": 150,
      "type": "distributed"
    },
    "connectionPool": {
      "maxIdleConns": 100,
      "maxIdleConnsPerHost": 10,
      "idleConnTimeout": "90s",
      "disableKeepAlives": false,
      "disableCompression": false,
      "insecureSkipVerify": false
    }
  }
}
```

## Batch HTTP Requests with Caching
```json
{
  "id": "fetch-users-batch",
  "type": "http",
  "target": "usersList",
  "action": {
    "method": "GET",
    "url": "https://api.example.com/users/${item.id}",
    "batch": {
      "items": "${userIds}",
      "itemVar": "item",
      "concurrency": 5,
      "continueOnError": true
    },
    "cache": {
      "enabled": true,
      "ttl": "15m",
      "type": "redis"
    }
  }
}
```

## Setting Up Cache and Rate Limiter

To use these features, you need to inject the cache and rate limiter into the HTTP executor:

```go
// Create cache
cacheConfig := cache.Config{
    Type:        cache.TypeTwoTier,
    TTL:         5 * time.Minute,
    RedisClient: redisClient,
}
cacheStore, _ := cache.New(cacheConfig)

// Create rate limiter
rlConfig := ratelimit.Config{
    Type:              ratelimit.BackendDistributed,
    RequestsPerSecond: 1000,
    BurstSize:         100,
    RedisClient:       redisClient,
}
rateLimiter, _ := ratelimit.NewLimiter(rlConfig)

// Get the HTTP executor from registry
registry := stages.NewRegistry()
if executor, found := registry.GetExecutor("http"); found {
    if httpExec, ok := executor.(*stages.HTTPExecutor); ok {
        httpExec.SetCache(cacheStore)
        httpExec.SetRateLimiter(rateLimiter)
    }
}
```

## Cache Configuration Options

- **type**: `"local"`, `"redis"`, or `"two_tier"`
  - `local`: In-memory LRU cache
  - `redis`: Distributed Redis cache
  - `two_tier`: L1 local + L2 Redis cache

- **ttl**: Duration string (e.g., "5m", "1h", "30s")
  - How long to cache responses

- **cacheMethods**: Array of HTTP methods to cache
  - Default: `["GET"]`

- **cacheStatuses**: Array of HTTP status codes to cache
  - Default: `[200]`

## Rate Limit Configuration Options

- **type**: `"local"` or `"distributed"`
  - `local`: Per-instance rate limiting
  - `distributed`: Shared rate limiting across instances

- **requestsPerSecond**: Maximum requests per second
- **burstSize**: Allow temporary bursts above the rate

## Feature Comparison with Old Enricher

| Feature | Old Enricher | Pipeline HTTP Stage |
|---------|--------------|-------------------|
| OAuth2 Auth | ✅ | ✅ |
| Basic/Bearer Auth | ✅ | ✅ |
| API Key Auth | ✅ | ✅ |
| Retry Logic | ✅ | ✅ |
| Circuit Breaker | ✅ | ✅ |
| Connection Pooling | ✅ | ✅ |
| Response Caching | ✅ | ✅ |
| Rate Limiting | ✅ | ✅ |
| Batch Requests | ❌ | ✅ |
| Template Support | ✅ | ✅ |
| Multi-field Response | ❌ | ✅ |

The Pipeline HTTP stage now has **all features from the old enricher plus additional capabilities** like batch processing and multi-field response extraction!