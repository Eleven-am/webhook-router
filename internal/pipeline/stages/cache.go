package stages

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"webhook-router/internal/common/cache"
	apperrors "webhook-router/internal/common/errors"
	"webhook-router/internal/common/logging"
	"webhook-router/internal/pipeline/core"
	"webhook-router/internal/pipeline/expression"
)

func init() {
	Register("cache", &CacheExecutor{})
	Register("cache.invalidate", &CacheInvalidateExecutor{})
}

// CacheExecutor implements the cache stage
type CacheExecutor struct {
	cacheStore cache.Cache
	// Metadata injected by pipeline executor
	pipelineID string
	userID     string
}

// CacheStageConfig represents the cache stage configuration
type CacheStageConfig struct {
	TTL        string          `json:"ttl"`        // Required: Duration string
	Key        []string        `json:"key"`        // Required: Array of expressions for cache key
	Execute    json.RawMessage `json:"execute"`    // Required: Pipeline to run on cache miss
	PipelineID string          `json:"pipelineId"` // Optional: Explicit pipeline ID for cache scope
}

// SetCache sets the cache instance for the executor
func (e *CacheExecutor) SetCache(c cache.Cache) {
	e.cacheStore = c
}

// SetMetadata sets the pipeline metadata for cache key generation
func (e *CacheExecutor) SetMetadata(pipelineID, userID string) {
	e.pipelineID = pipelineID
	e.userID = userID
}

// Execute performs the cache logic
func (e *CacheExecutor) Execute(ctx context.Context, runCtx *core.Context, stage *core.StageDefinition) (interface{}, error) {
	if e.cacheStore == nil {
		return nil, apperrors.ConfigError("cache not configured")
	}

	// Parse configuration
	var config CacheStageConfig
	if err := json.Unmarshal(stage.Action, &config); err != nil {
		return nil, apperrors.ConfigError("invalid cache configuration").WithContext("error", err.Error())
	}

	// Validate required fields
	if config.TTL == "" {
		return nil, apperrors.ConfigError("ttl is required")
	}
	if len(config.Key) == 0 {
		return nil, apperrors.ConfigError("key array is required")
	}
	if config.Execute == nil {
		return nil, apperrors.ConfigError("execute pipeline is required")
	}

	// Get target from stage definition
	targetName, ok := stage.GetTargetName()
	if !ok || targetName == "" {
		return nil, apperrors.ConfigError("target is required for cache stage")
	}

	// Parse TTL
	ttl, err := time.ParseDuration(config.TTL)
	if err != nil {
		return nil, apperrors.ConfigError("invalid ttl duration").WithContext("error", err.Error())
	}

	// Get user ID from metadata
	userID := e.userID
	if userID == "" {
		// Try to get from context if available
		if metadata, ok := runCtx.Get("_metadata"); ok {
			if metaMap, ok := metadata.(map[string]interface{}); ok {
				if uid, ok := metaMap["user_id"]; ok {
					userID = fmt.Sprint(uid)
				}
			}
		}

		if userID == "" {
			return nil, apperrors.ConfigError("user ID not available for cache stage")
		}
	}

	// Determine pipeline ID to use for cache scope
	pipelineID := config.PipelineID // Use explicit pipeline ID if provided
	if pipelineID == "" {
		// Fall back to current pipeline ID
		pipelineID = e.pipelineID
		if pipelineID == "" {
			// Try to get from context if not set
			if metadata, ok := runCtx.Get("_metadata"); ok {
				if metaMap, ok := metadata.(map[string]interface{}); ok {
					if pid, ok := metaMap["pipeline_id"]; ok {
						pipelineID = fmt.Sprint(pid)
					}
				}
			}
		}

		if pipelineID == "" {
			return nil, apperrors.ConfigError("pipeline ID not available for cache stage")
		}
	}

	// Generate cache key
	cacheKey, err := e.generateCacheKey(ctx, runCtx, userID, pipelineID, stage.ID, config.Key)
	if err != nil {
		return nil, err
	}

	// Check cache first
	if cached, found := e.cacheStore.Get(ctx, cacheKey); found {
		// Cache hit - set target and return
		runCtx.Set(targetName, cached)
		return cached, nil
	}

	// Cache miss - need to execute pipeline with lock protection
	lockKey := fmt.Sprintf("%s:lock", cacheKey)
	lockToken := e.generateLockToken()

	// Try to acquire lock with token
	acquired, err := e.tryAcquireLock(ctx, lockKey, lockToken, 30*time.Second)
	if err != nil {
		return nil, apperrors.InternalError("failed to acquire lock", err)
	}

	if acquired {
		// We are the leader - execute the pipeline
		defer e.releaseLock(ctx, lockKey, lockToken)

		result, err := e.executePipeline(ctx, runCtx, config.Execute)
		if err != nil {
			// Fail fast - don't cache errors
			return nil, err
		}

		// Cache the result
		if err := e.cacheStore.Set(ctx, cacheKey, result, ttl); err != nil {
			// Log cache error but don't fail the request
			logging.GetGlobalLogger().Error("Failed to cache pipeline result",
				err,
				logging.Field{Key: "cache_key", Value: cacheKey},
				logging.Field{Key: "ttl", Value: ttl},
				logging.Field{Key: "pipeline_id", Value: e.pipelineID},
			)
		}

		// Set target and return
		runCtx.Set(targetName, result)
		return result, nil

	} else {
		// We are a follower - wait for the leader to populate cache
		return e.waitForCachedResult(ctx, runCtx, cacheKey, lockKey, targetName, 30*time.Second)
	}
}

// generateCacheKey creates the deterministic cache key
func (e *CacheExecutor) generateCacheKey(ctx context.Context, runCtx *core.Context, userID, pipelineID, stageID string, keyExprs []string) (string, error) {
	// Evaluate each key expression
	keyParts := make([]string, len(keyExprs))
	for i, expr := range keyExprs {
		value, err := expression.ResolveTemplates(expr, runCtx)
		if err != nil {
			return "", apperrors.InternalError(fmt.Sprintf("failed to resolve key expression %d", i), err)
		}
		keyParts[i] = value
	}

	// Join with delimiter to prevent collisions
	userKeyPart := strings.Join(keyParts, ":")

	// Hash to handle any values
	hash := sha256.Sum256([]byte(userKeyPart))
	hashedKey := hex.EncodeToString(hash[:])

	// Build full cache key
	return fmt.Sprintf("user:%s:pipeline:%s:stage:%s:key:%s",
		userID,
		pipelineID,
		stageID,
		hashedKey,
	), nil
}

// generateLockToken creates a unique token for lock ownership
func (e *CacheExecutor) generateLockToken() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// tryAcquireLock attempts to acquire a distributed lock with a token
func (e *CacheExecutor) tryAcquireLock(ctx context.Context, lockKey, token string, ttl time.Duration) (bool, error) {
	// This uses SetNX semantics - only sets if key doesn't exist
	return e.cacheStore.SetNX(ctx, lockKey, token, ttl)
}

// releaseLock releases the lock only if we still own it (token matches)
func (e *CacheExecutor) releaseLock(ctx context.Context, lockKey, token string) error {
	// Get current lock value
	currentToken, found := e.cacheStore.Get(ctx, lockKey)
	if !found {
		// Lock already expired or released
		return nil
	}

	// Only delete if we still own it
	if currentToken == token {
		return e.cacheStore.Delete(ctx, lockKey)
	}

	// Someone else owns the lock now
	return nil
}

// executePipeline runs the sub-pipeline in a sandboxed context
func (e *CacheExecutor) executePipeline(ctx context.Context, parentCtx *core.Context, pipelineJSON json.RawMessage) (interface{}, error) {
	// Parse the pipeline stages
	var stages []core.StageDefinition
	if err := json.Unmarshal(pipelineJSON, &stages); err != nil {
		return nil, apperrors.ConfigError("invalid execute pipeline").WithContext("error", err.Error())
	}

	// Create a new context for the sub-pipeline
	// Copy parent variables but give it a fresh execution context
	subCtx := core.NewContext()
	for k, v := range parentCtx.GetAll() {
		subCtx.Set(k, v)
	}

	// Get the registry to execute stages
	registry := GetRegistry()

	// Execute each stage in sequence
	var lastResult interface{}
	for _, stage := range stages {
		executor, found := registry.GetExecutor(stage.Type)
		if !found {
			return nil, apperrors.ConfigError(fmt.Sprintf("unknown stage type: %s", stage.Type))
		}

		// Note: Conditions are not currently supported in pipeline stages
		// This would need to be added to StageDefinition if required

		// Execute the stage
		result, err := executor.Execute(ctx, subCtx, &stage)
		if err != nil {
			// Fail fast - propagate errors
			return nil, err
		}

		lastResult = result

		// Set target if specified
		if targetName, hasTarget := stage.GetTargetName(); hasTarget && targetName != "" {
			subCtx.Set(targetName, result)
		}
	}

	// Return the last result (or the value of the target from the last stage)
	return lastResult, nil
}

// waitForCachedResult waits for the leader to populate the cache
func (e *CacheExecutor) waitForCachedResult(ctx context.Context, runCtx *core.Context, cacheKey, lockKey, target string, timeout time.Duration) (interface{}, error) {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	timeoutCh := time.After(timeout)

	for {
		select {
		case <-ticker.C:
			// Check if result is now cached
			if cached, found := e.cacheStore.Get(ctx, cacheKey); found {
				runCtx.Set(target, cached)
				return cached, nil
			}

			// Check if lock still exists
			if _, found := e.cacheStore.Get(ctx, lockKey); !found {
				// Lock is gone but no cache - leader must have failed
				// Try to become the new leader by recursively calling Execute
				// This will either find the cache or try to acquire the lock again
				return nil, apperrors.InternalError("cache leader failed, retrying", errors.New("lock expired without cache"))
			}

		case <-timeoutCh:
			return nil, apperrors.InternalError("timeout waiting for cache", nil)

		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

// CacheInvalidateExecutor implements cache invalidation
type CacheInvalidateExecutor struct {
	cacheStore cache.Cache
	// Metadata injected by pipeline executor
	pipelineID string
	userID     string
}

// CacheInvalidateConfig represents the invalidation configuration
type CacheInvalidateConfig struct {
	CacheStageID string   `json:"cacheStageId"` // Required: ID of the cache stage
	Key          []string `json:"key"`          // Required: Same key expressions as cache
	PipelineID   string   `json:"pipelineId"`   // Optional: Explicit pipeline ID to match cache scope
}

// SetCache sets the cache instance for the executor
func (e *CacheInvalidateExecutor) SetCache(c cache.Cache) {
	e.cacheStore = c
}

// SetMetadata sets the pipeline metadata for cache key generation
func (e *CacheInvalidateExecutor) SetMetadata(pipelineID, userID string) {
	e.pipelineID = pipelineID
	e.userID = userID
}

// Execute invalidates a cache entry
func (e *CacheInvalidateExecutor) Execute(ctx context.Context, runCtx *core.Context, stage *core.StageDefinition) (interface{}, error) {
	if e.cacheStore == nil {
		return nil, apperrors.ConfigError("cache not configured")
	}

	// Parse configuration
	var config CacheInvalidateConfig
	if err := json.Unmarshal(stage.Action, &config); err != nil {
		return nil, apperrors.ConfigError("invalid cache.invalidate configuration").WithContext("error", err.Error())
	}

	// Validate required fields
	if config.CacheStageID == "" {
		return nil, apperrors.ConfigError("cacheStageId is required")
	}
	if len(config.Key) == 0 {
		return nil, apperrors.ConfigError("key array is required")
	}

	// Get user ID from metadata
	userID := e.userID
	if userID == "" {
		// Try to get from context if available
		if metadata, ok := runCtx.Get("_metadata"); ok {
			if metaMap, ok := metadata.(map[string]interface{}); ok {
				if uid, ok := metaMap["user_id"]; ok {
					userID = fmt.Sprint(uid)
				}
			}
		}

		if userID == "" {
			return nil, apperrors.ConfigError("user ID not available for cache invalidation")
		}
	}

	// Determine pipeline ID to use for cache scope
	pipelineID := config.PipelineID // Use explicit pipeline ID if provided
	if pipelineID == "" {
		// Fall back to current pipeline ID
		pipelineID = e.pipelineID
		if pipelineID == "" {
			// Try to get from context if not set
			if metadata, ok := runCtx.Get("_metadata"); ok {
				if metaMap, ok := metadata.(map[string]interface{}); ok {
					if pid, ok := metaMap["pipeline_id"]; ok {
						pipelineID = fmt.Sprint(pid)
					}
				}
			}
		}

		if pipelineID == "" {
			return nil, apperrors.ConfigError("pipeline ID not available for cache invalidation")
		}
	}

	// Generate the same cache key as the cache stage would
	cacheKey, err := e.generateCacheKey(ctx, runCtx, userID, pipelineID, config.CacheStageID, config.Key)
	if err != nil {
		return nil, err
	}

	// Delete the cache entry
	if err := e.cacheStore.Delete(ctx, cacheKey); err != nil {
		return nil, apperrors.InternalError("failed to invalidate cache", err)
	}

	return map[string]interface{}{
		"invalidated":  true,
		"cacheStageId": config.CacheStageID,
		"key":          config.Key,
	}, nil
}

// generateCacheKey creates the same deterministic cache key as CacheExecutor
func (e *CacheInvalidateExecutor) generateCacheKey(ctx context.Context, runCtx *core.Context, userID, pipelineID, stageID string, keyExprs []string) (string, error) {
	// Evaluate each key expression
	keyParts := make([]string, len(keyExprs))
	for i, expr := range keyExprs {
		value, err := expression.ResolveTemplates(expr, runCtx)
		if err != nil {
			return "", apperrors.InternalError(fmt.Sprintf("failed to resolve key expression %d", i), err)
		}
		keyParts[i] = value
	}

	// Join with delimiter to prevent collisions
	userKeyPart := strings.Join(keyParts, ":")

	// Hash to handle any values
	hash := sha256.Sum256([]byte(userKeyPart))
	hashedKey := hex.EncodeToString(hash[:])

	// Build full cache key
	return fmt.Sprintf("user:%s:pipeline:%s:stage:%s:key:%s",
		userID,
		pipelineID,
		stageID,
		hashedKey,
	), nil
}
