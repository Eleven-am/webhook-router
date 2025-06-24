package stages

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"webhook-router/internal/pipeline/core"
)

func TestCacheStage_SharedCache(t *testing.T) {
	ctx := context.Background()
	mockCache := NewMockCache()

	// Register transform stage for the test
	Register("transform", &TransformExecutor{})
	registry := NewRegistry()
	SetRegistry(registry)

	// Test shared cache between different pipelines
	// First pipeline with explicit pipelineId
	executor1 := &CacheExecutor{
		cacheStore: mockCache,
		userID:     "user123",
		pipelineID: "pipeline1",
	}

	runCtx1 := core.NewContext()
	runCtx1.Set("customerId", "CUST456")

	stage1 := &core.StageDefinition{
		ID:     "cache-customer",
		Type:   "cache",
		Target: "customer",
		Action: json.RawMessage(`{
			"ttl": "10m",
			"key": ["${customerId}"],
			"pipelineId": "shared-customer-cache",
			"execute": [
				{
					"id": "create-customer",
					"type": "transform",
					"target": "customer",
					"expression": "{id: customerId, name: 'John Doe', tier: 'Premium'}"
				}
			]
		}`),
	}

	// Execute first pipeline
	result1, err := executor1.Execute(ctx, runCtx1, stage1)
	require.NoError(t, err)

	expected := map[string]interface{}{
		"id":   "CUST456",
		"name": "John Doe",
		"tier": "Premium",
	}
	assert.Equal(t, expected, result1)

	// Second pipeline accessing the same cache
	executor2 := &CacheExecutor{
		cacheStore: mockCache,
		userID:     "user123",   // Same user
		pipelineID: "pipeline2", // Different pipeline
	}

	runCtx2 := core.NewContext()
	runCtx2.Set("customerId", "CUST456")

	stage2 := &core.StageDefinition{
		ID:     "cache-customer",
		Type:   "cache",
		Target: "customerData",
		Action: json.RawMessage(`{
			"ttl": "10m",
			"key": ["${customerId}"],
			"pipelineId": "shared-customer-cache",
			"execute": [
				{
					"id": "create-customer",
					"type": "transform",
					"target": "customer",
					"expression": "{id: customerId, name: 'Different Name', tier: 'Basic'}"
				}
			]
		}`),
	}

	// Execute second pipeline - should get cached result
	result2, err := executor2.Execute(ctx, runCtx2, stage2)
	require.NoError(t, err)

	// Should get the same cached data, not execute the transform
	assert.Equal(t, expected, result2)
}

func TestCacheStage_MetadataFromContext(t *testing.T) {
	ctx := context.Background()
	mockCache := NewMockCache()

	// Register transform stage for the test
	Register("transform", &TransformExecutor{})
	registry := NewRegistry()
	SetRegistry(registry)

	// Create cache executor without metadata
	executor := &CacheExecutor{
		cacheStore: mockCache,
	}

	// Create context with metadata
	runCtx := core.NewContext()
	runCtx.Set("productId", "PROD789")
	runCtx.Set("_metadata", map[string]interface{}{
		"user_id":     "user999",
		"pipeline_id": "pipeline888",
	})

	stage := &core.StageDefinition{
		ID:     "cache-with-metadata",
		Type:   "cache",
		Target: "product",
		Action: json.RawMessage(`{
			"ttl": "5m",
			"key": ["${productId}"],
			"execute": [
				{
					"id": "fetch-product",
					"type": "transform",
					"target": "product",
					"expression": "{id: productId, name: 'Test Product'}"
				}
			]
		}`),
	}

	// Execute stage
	result, err := executor.Execute(ctx, runCtx, stage)
	require.NoError(t, err)

	expected := map[string]interface{}{
		"id":   "PROD789",
		"name": "Test Product",
	}
	assert.Equal(t, expected, result)
}

func TestCacheStage_ErrorHandling(t *testing.T) {
	ctx := context.Background()
	mockCache := NewMockCache()

	tests := []struct {
		name   string
		stage  *core.StageDefinition
		setup  func(*CacheExecutor)
		errMsg string
	}{
		{
			name: "Missing TTL",
			stage: &core.StageDefinition{
				ID:     "cache-no-ttl",
				Type:   "cache",
				Target: "result",
				Action: json.RawMessage(`{
					"key": ["${id}"],
					"execute": []
				}`),
			},
			setup: func(e *CacheExecutor) {
				e.userID = "user123"
				e.pipelineID = "pipeline456"
			},
			errMsg: "ttl is required",
		},
		{
			name: "Missing key",
			stage: &core.StageDefinition{
				ID:     "cache-no-key",
				Type:   "cache",
				Target: "result",
				Action: json.RawMessage(`{
					"ttl": "5m",
					"execute": []
				}`),
			},
			setup: func(e *CacheExecutor) {
				e.userID = "user123"
				e.pipelineID = "pipeline456"
			},
			errMsg: "key array is required",
		},
		{
			name: "Missing execute",
			stage: &core.StageDefinition{
				ID:     "cache-no-execute",
				Type:   "cache",
				Target: "result",
				Action: json.RawMessage(`{
					"ttl": "5m",
					"key": ["${id}"]
				}`),
			},
			setup: func(e *CacheExecutor) {
				e.userID = "user123"
				e.pipelineID = "pipeline456"
			},
			errMsg: "execute pipeline is required",
		},
		{
			name: "Missing target",
			stage: &core.StageDefinition{
				ID:   "cache-no-target",
				Type: "cache",
				Action: json.RawMessage(`{
					"ttl": "5m",
					"key": ["${id}"],
					"execute": []
				}`),
			},
			setup: func(e *CacheExecutor) {
				e.userID = "user123"
				e.pipelineID = "pipeline456"
			},
			errMsg: "target is required for cache stage",
		},
		{
			name: "Invalid TTL",
			stage: &core.StageDefinition{
				ID:     "cache-bad-ttl",
				Type:   "cache",
				Target: "result",
				Action: json.RawMessage(`{
					"ttl": "invalid",
					"key": ["${id}"],
					"execute": []
				}`),
			},
			setup: func(e *CacheExecutor) {
				e.userID = "user123"
				e.pipelineID = "pipeline456"
			},
			errMsg: "invalid ttl duration",
		},
		{
			name: "Missing user ID",
			stage: &core.StageDefinition{
				ID:     "cache-no-user",
				Type:   "cache",
				Target: "result",
				Action: json.RawMessage(`{
					"ttl": "5m",
					"key": ["${id}"],
					"execute": []
				}`),
			},
			setup: func(e *CacheExecutor) {
				// No user ID set
			},
			errMsg: "user ID not available for cache stage",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			executor := &CacheExecutor{
				cacheStore: mockCache,
			}

			if tt.setup != nil {
				tt.setup(executor)
			}

			runCtx := core.NewContext()
			runCtx.Set("id", "123")

			_, err := executor.Execute(ctx, runCtx, tt.stage)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.errMsg)
		})
	}
}

func TestCacheStage_ComplexPipeline(t *testing.T) {
	ctx := context.Background()
	mockCache := NewMockCache()

	// Register stages for the test
	Register("transform", &TransformExecutor{})
	Register("http", &HTTPExecutor{})
	registry := NewRegistry()
	SetRegistry(registry)

	executor := &CacheExecutor{
		cacheStore: mockCache,
		userID:     "user123",
		pipelineID: "pipeline456",
	}

	runCtx := core.NewContext()
	runCtx.Set("customerId", "CUST789")
	runCtx.Set("includeOrders", true)

	stage := &core.StageDefinition{
		ID:     "cache-customer-360",
		Type:   "cache",
		Target: "customer360",
		Action: json.RawMessage(`{
			"ttl": "30m",
			"key": ["${customerId}", "${includeOrders}"],
			"execute": [
				{
					"id": "fetch-customer",
					"type": "transform",
					"target": "customer",
					"expression": "{id: customerId, name: 'Jane Smith', email: 'jane@example.com'}"
				},
				{
					"id": "fetch-orders",
					"type": "transform",
					"target": "orders",
					"expression": "[{id: 'ORD001', total: 99.99}, {id: 'ORD002', total: 149.99}]",
					"condition": "${includeOrders}"
				},
				{
					"id": "combine",
					"type": "transform",
					"target": "customer360",
					"expression": "merge(customer, {orders: orders})"
				}
			]
		}`),
	}

	// Execute stage
	result, err := executor.Execute(ctx, runCtx, stage)
	require.NoError(t, err)

	expected := map[string]interface{}{
		"id":    "CUST789",
		"name":  "Jane Smith",
		"email": "jane@example.com",
		"orders": []interface{}{
			map[string]interface{}{"id": "ORD001", "total": 99.99},
			map[string]interface{}{"id": "ORD002", "total": 149.99},
		},
	}
	assert.Equal(t, expected, result)

	// Execute again - should hit cache
	result2, err := executor.Execute(ctx, runCtx, stage)
	require.NoError(t, err)
	assert.Equal(t, expected, result2)
}

func TestCacheStage_ThunderingHerd(t *testing.T) {
	ctx := context.Background()
	mockCache := NewMockCache()

	// Register transform stage for the test
	Register("transform", &TransformExecutor{})
	registry := NewRegistry()
	SetRegistry(registry)

	executor := &CacheExecutor{
		cacheStore: mockCache,
		userID:     "user123",
		pipelineID: "pipeline456",
	}

	runCtx := core.NewContext()
	runCtx.Set("key", "test")

	stage := &core.StageDefinition{
		ID:     "cache-test",
		Type:   "cache",
		Target: "result",
		Action: json.RawMessage(`{
			"ttl": "5m",
			"key": ["${key}"],
			"execute": [
				{
					"id": "slow-operation",
					"type": "transform",
					"target": "result",
					"expression": "{value: 'computed'}"
				}
			]
		}`),
	}

	// Simulate first request acquiring lock
	result1, err := executor.Execute(ctx, runCtx, stage)
	require.NoError(t, err)
	assert.Equal(t, map[string]interface{}{"value": "computed"}, result1)

	// Verify lock was acquired and released
	lockKey := fmt.Sprintf("user:user123:pipeline:pipeline456:stage:cache-test:key:%s:lock",
		"9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08")
	_, lockExists := mockCache.Get(ctx, lockKey)
	assert.False(t, lockExists) // Lock should be released
}

func TestCacheInvalidation_SharedCache(t *testing.T) {
	ctx := context.Background()
	mockCache := NewMockCache()

	// Pre-populate shared cache
	cacheKey := "user:user123:pipeline:shared-cache:stage:cache-data:key:abc123"
	mockCache.Set(ctx, cacheKey, map[string]interface{}{"value": "cached"}, 10*time.Minute)

	// Create invalidation executor for different pipeline
	executor := &CacheInvalidateExecutor{
		cacheStore: mockCache,
		userID:     "user123",
		pipelineID: "pipeline999", // Different pipeline
	}

	runCtx := core.NewContext()
	runCtx.Set("dataId", "data123")

	stage := &core.StageDefinition{
		ID:   "clear-shared-cache",
		Type: "cache.invalidate",
		Action: json.RawMessage(`{
			"cacheStageId": "cache-data",
			"key": ["${dataId}"],
			"pipelineId": "shared-cache"
		}`),
	}

	// Execute invalidation
	result, err := executor.Execute(ctx, runCtx, stage)
	require.NoError(t, err)

	expected := map[string]interface{}{
		"invalidated":  true,
		"cacheStageId": "cache-data",
		"key":          []string{"${dataId}"},
	}
	assert.Equal(t, expected, result)
}

func TestCacheStage_NullValues(t *testing.T) {
	ctx := context.Background()
	mockCache := NewMockCache()

	// Register transform stage for the test
	Register("transform", &TransformExecutor{})
	registry := NewRegistry()
	SetRegistry(registry)

	executor := &CacheExecutor{
		cacheStore: mockCache,
		userID:     "user123",
		pipelineID: "pipeline456",
	}

	runCtx := core.NewContext()
	runCtx.Set("key", "null-test")

	stage := &core.StageDefinition{
		ID:     "cache-null",
		Type:   "cache",
		Target: "result",
		Action: json.RawMessage(`{
			"ttl": "5m",
			"key": ["${key}"],
			"execute": [
				{
					"id": "return-null",
					"type": "transform",
					"target": "result",
					"expression": "null"
				}
			]
		}`),
	}

	// Execute stage - should cache null
	result, err := executor.Execute(ctx, runCtx, stage)
	require.NoError(t, err)
	assert.Nil(t, result)

	// Execute again - should return cached null
	result2, err := executor.Execute(ctx, runCtx, stage)
	require.NoError(t, err)
	assert.Nil(t, result2)
}

func TestCacheStage_ExpressionErrors(t *testing.T) {
	ctx := context.Background()
	mockCache := NewMockCache()

	executor := &CacheExecutor{
		cacheStore: mockCache,
		userID:     "user123",
		pipelineID: "pipeline456",
	}

	runCtx := core.NewContext()
	// Missing required variable

	stage := &core.StageDefinition{
		ID:     "cache-error",
		Type:   "cache",
		Target: "result",
		Action: json.RawMessage(`{
			"ttl": "5m",
			"key": ["${missingVar}"],
			"execute": []
		}`),
	}

	// Should fail with expression error
	_, err := executor.Execute(ctx, runCtx, stage)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to resolve key expression")
}
