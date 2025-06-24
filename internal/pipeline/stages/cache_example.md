# Cache Stage Examples

The cache stage allows you to cache the results of expensive pipeline operations to improve performance and reduce load on external systems.

## Basic Cache Usage

Cache the result of an API call:

```json
{
  "id": "get-user-profile",
  "type": "cache",
  "ttl": "10m",
  "target": "userProfile",
  "key": ["${userId}"],
  "execute": [
    {
      "id": "fetch-user",
      "type": "http",
      "action": "GET https://api.example.com/users/${userId}",
      "target": "userProfile"
    }
  ]
}
```

## Multi-Key Cache

Cache based on multiple parameters:

```json
{
  "id": "get-product-price",
  "type": "cache",
  "ttl": "1h",
  "target": "pricing",
  "key": ["${productId}", "${currency}", "${customerTier}"],
  "execute": [
    {
      "id": "calculate-price",
      "type": "http",
      "action": {
        "method": "POST",
        "url": "https://pricing-service.internal/calculate",
        "body": {
          "productId": "${productId}",
          "currency": "${currency}",
          "tier": "${customerTier}"
        }
      },
      "target": "pricing"
    }
  ]
}
```

## Complex Pipeline Caching

Cache the result of multiple operations:

```json
{
  "id": "get-customer-360",
  "type": "cache",
  "ttl": "30m",
  "target": "customer360",
  "key": ["${customerId}"],
  "execute": [
    {
      "id": "fetch-customer",
      "type": "http",
      "action": "GET https://api.example.com/customers/${customerId}",
      "target": "customer"
    },
    {
      "id": "fetch-orders",
      "type": "http",
      "action": "GET https://api.example.com/customers/${customerId}/orders",
      "target": "orders"
    },
    {
      "id": "fetch-preferences",
      "type": "http",
      "action": "GET https://api.example.com/customers/${customerId}/preferences",
      "target": "preferences"
    },
    {
      "id": "combine-data",
      "type": "transform",
      "target": "customer360",
      "expression": "merge(customer, {orders: orders, preferences: preferences})"
    }
  ]
}
```

## Cache Invalidation

Invalidate cached data when it's updated:

```json
{
  "stages": [
    {
      "id": "update-product",
      "type": "http",
      "action": {
        "method": "PUT",
        "url": "https://api.example.com/products/${productId}",
        "body": "${productData}"
      },
      "target": "updateResult"
    },
    {
      "id": "clear-product-cache",
      "type": "cache.invalidate",
      "cacheStageId": "get-product-price",
      "key": ["${productId}", "${currency}", "${customerTier}"],
      "dependsOn": ["update-product"]
    }
  ]
}
```

## Shared Cache Across Pipelines

Use the optional `pipelineId` to share cache between different pipelines:

### Pipeline A: Fetch and Cache Customer Data
```json
{
  "id": "cache-customer-360",
  "type": "cache",
  "ttl": "30m",
  "target": "customerData",
  "key": ["${customerId}"],
  "pipelineId": "shared-customer-cache",  // Explicit pipeline ID for sharing
  "execute": [
    {
      "id": "fetch-customer",
      "type": "http",
      "action": "GET https://api.example.com/customers/${customerId}",
      "target": "customer"
    },
    {
      "id": "fetch-orders",
      "type": "http", 
      "action": "GET https://api.example.com/orders?customerId=${customerId}",
      "target": "orders"
    },
    {
      "id": "combine",
      "type": "transform",
      "target": "customerData",
      "expression": "merge(customer, {orders: orders})"
    }
  ]
}
```

### Pipeline B: Reuse Customer Data from Pipeline A
```json
{
  "id": "cache-customer-360",
  "type": "cache",
  "ttl": "30m",
  "target": "customerData",
  "key": ["${customerId}"],
  "pipelineId": "shared-customer-cache",  // Same pipeline ID = shared cache!
  "execute": [
    // This will only execute if cache miss
    // Same expensive operations...
  ]
}
```

### Invalidate Shared Cache
```json
{
  "id": "clear-shared-customer-cache",
  "type": "cache.invalidate",
  "cacheStageId": "cache-customer-360",
  "key": ["${customerId}"],
  "pipelineId": "shared-customer-cache"  // Must match the cache's pipelineId
}
```

## Conditional Caching

Only cache under certain conditions:

```json
{
  "id": "get-weather",
  "type": "cache",
  "ttl": "15m",
  "target": "weather",
  "key": ["${city}"],
  "condition": "${!forceRefresh}",
  "execute": [
    {
      "id": "fetch-weather",
      "type": "http",
      "action": "GET https://weather-api.example.com/current?city=${city}",
      "target": "weather"
    }
  ]
}
```

## Key Design Patterns

### 1. User-Specific Cache
```json
{
  "key": ["${userId}", "${resource}"]
}
```

### 2. Time-Based Cache (Daily)
```json
{
  "key": ["${dateFormat(now(), 'YYYY-MM-DD')}", "${reportType}"]
}
```

### 3. Version-Aware Cache
```json
{
  "key": ["${apiVersion}", "${endpoint}", "${params}"]
}
```

### 4. Computed Keys
```json
{
  "key": ["${toLowerCase(country)}", "${toUpperCase(state)}", "${postalCode}"]
}
```

## Important Notes

1. **User Isolation**: Caches are automatically scoped to the pipeline owner. Different users' caches are completely isolated.

2. **Key Generation**: Keys are hashed using SHA256, so you can safely use any values including objects or special characters.

3. **TTL Format**: Use Go duration strings like "5m", "1h", "24h", "7d".

4. **Thundering Herd Protection**: The cache stage automatically handles concurrent requests for the same key - only one execution happens while others wait.

5. **Error Handling**: Errors are never cached. If the execute pipeline fails, the error bubbles up immediately.

6. **Cache Backends**: The cache stage works with whatever cache backend is configured (local memory, Redis, or two-tier).

## Performance Tips

1. **Choose appropriate TTLs**: Balance between freshness and performance
2. **Use specific keys**: More specific keys = better cache hit rates
3. **Cache at the right level**: Cache final results, not intermediate steps
4. **Monitor cache hit rates**: Adjust TTLs and keys based on usage patterns