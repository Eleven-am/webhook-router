package core

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"
	"webhook-router/internal/pipeline/cache"
)

// Executor is the main pipeline execution engine
type Executor struct {
	defaultTimeout time.Duration
	stageRegistry  StageRegistry
	cache          cache.Cache
}

// StageRegistry provides access to stage executors
type StageRegistry interface {
	GetExecutor(stageType string) (StageExecutor, bool)
}

// StageExecutor is the interface implemented by all stage types
type StageExecutor interface {
	Execute(ctx context.Context, runCtx *Context, stage *StageDefinition) (interface{}, error)
}

// NewExecutor creates a new pipeline executor
func NewExecutor(registry StageRegistry) *Executor {
	return &Executor{
		defaultTimeout: 30 * time.Second,
		stageRegistry:  registry,
		cache:          cache.NewInMemoryCache(), // Default to in-memory cache
	}
}

// SetCache sets the cache implementation
func (e *Executor) SetCache(c cache.Cache) {
	e.cache = c
}

// Execute runs a pipeline with the given input
func (e *Executor) Execute(ctx context.Context, pipeline *PipelineDefinition, input map[string]interface{}) (map[string]interface{}, error) {
	// Create execution context
	runCtx := NewContext()

	// Build DAG
	dag, err := NewDAG(pipeline.Stages)
	if err != nil {
		return nil, fmt.Errorf("invalid pipeline DAG: %w", err)
	}

	// Set resources in context (${api}, ${db}, etc.)
	for key, value := range pipeline.Resources {
		runCtx.Set(key, value)
	}

	// Find and execute input stage first
	if err := e.executeInputStage(ctx, dag, runCtx, input); err != nil {
		return nil, err
	}

	// Get execution plan
	plan, err := dag.GetExecutionPlan()
	if err != nil {
		return nil, err
	}

	// Execute stages according to plan
	for batchIdx, batch := range plan {
		if err := e.executeBatch(ctx, dag, runCtx, batch); err != nil {
			return nil, fmt.Errorf("batch %d failed: %w", batchIdx, err)
		}
	}

	// Build output from output stage
	return e.buildOutput(dag, runCtx)
}

// executeInputStage finds and executes the input stage
func (e *Executor) executeInputStage(ctx context.Context, dag *DAG, runCtx *Context, input map[string]interface{}) error {
	for _, stage := range dag.stages {
		if stage.Type == "input" {
			if targetName, ok := stage.GetTargetName(); ok {
				return runCtx.Set(targetName, input)
			}
			return fmt.Errorf("input stage must have string target")
		}
	}
	return nil // No input stage is valid
}

// executeBatch runs a batch of stages in parallel
func (e *Executor) executeBatch(ctx context.Context, dag *DAG, runCtx *Context, stageIDs []string) error {
	g, ctx := errgroup.WithContext(ctx)

	// Use mutex to protect context writes
	var mu sync.Mutex

	for _, stageID := range stageIDs {
		stageID := stageID // Capture for goroutine
		stage, _ := dag.GetStage(stageID)

		// Check if stage is nil (shouldn't happen, but defensive)
		if stage == nil {
			return fmt.Errorf("stage '%s' not found in DAG", stageID)
		}

		// Skip input stage (already processed)
		if stage.Type == "input" {
			continue
		}

		g.Go(func() error {
			// Execute stage
			result, err := e.executeStage(ctx, runCtx, stage)
			if err != nil {
				return fmt.Errorf("stage '%s' failed: %w", stageID, err)
			}

			// Store result in context (thread-safe)
			if stage.Type != "output" && stage.Target != nil {
				mu.Lock()
				defer mu.Unlock()

				if err := e.storeResult(runCtx, stage, result); err != nil {
					return fmt.Errorf("failed to store result for stage '%s': %w", stageID, err)
				}
			}

			return nil
		})
	}

	return g.Wait()
}

// executeStage executes a single stage
func (e *Executor) executeStage(ctx context.Context, runCtx *Context, stage *StageDefinition) (interface{}, error) {
	// Apply timeout
	timeout := stage.GetTimeout(e.defaultTimeout)
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	// Check cache if enabled
	if stage.Cache != nil && e.cache != nil {
		// Parse cache config
		var cacheConfig map[string]interface{}
		if err := json.Unmarshal(stage.Cache, &cacheConfig); err == nil {
			// Generate cache key
			cacheKey := e.generateCacheKey(stage, runCtx, cacheConfig)

			// Check cache
			if cached, found := e.cache.Get(ctx, cacheKey); found {
				return cached, nil
			}

			// Execute and cache result
			result, err := e.executeStageDirect(ctx, runCtx, stage)
			if err == nil && result != nil {
				// Parse cache TTL
				ttl := 5 * time.Minute // Default TTL
				if ttlStr, ok := cacheConfig["ttl"].(string); ok {
					if parsed, parseErr := time.ParseDuration(ttlStr); parseErr == nil {
						ttl = parsed
					}
				}

				// Cache the result
				e.cache.Set(ctx, cacheKey, result, ttl)
			}

			return result, err
		}
	}

	// No caching, execute directly
	return e.executeStageDirect(ctx, runCtx, stage)
}

// executeStageDirect executes a stage without caching
func (e *Executor) executeStageDirect(ctx context.Context, runCtx *Context, stage *StageDefinition) (interface{}, error) {
	// Get stage executor
	executor, ok := e.stageRegistry.GetExecutor(stage.Type)
	if !ok {
		return nil, fmt.Errorf("unknown stage type: %s", stage.Type)
	}

	// Execute stage
	result, err := executor.Execute(ctx, runCtx, stage)
	if err != nil {
		// Handle onError if present
		if stage.OnError != nil {
			return e.handleError(ctx, runCtx, stage, err)
		}
		return nil, err
	}

	return result, nil
}

// generateCacheKey generates a cache key for a stage
func (e *Executor) generateCacheKey(stage *StageDefinition, ctx *Context, cacheConfig map[string]interface{}) string {
	// Create a hash of stage ID + action + relevant context
	h := sha256.New()

	// Include stage ID and type
	h.Write([]byte(stage.ID))
	h.Write([]byte(stage.Type))

	// Include action
	h.Write(stage.Action)

	// Include cache key fields if specified
	if keyFields, ok := cacheConfig["key"].([]interface{}); ok {
		for _, field := range keyFields {
			if fieldStr, ok := field.(string); ok {
				if value, found := ctx.Get(fieldStr); found {
					// Convert value to string for hashing
					valStr := fmt.Sprint(value)
					h.Write([]byte(fieldStr + ":" + valStr))
				}
			}
		}
	}

	return "pipeline:" + hex.EncodeToString(h.Sum(nil))
}

// storeResult stores the stage result in the context
func (e *Executor) storeResult(runCtx *Context, stage *StageDefinition, result interface{}) error {
	// Handle string target
	if targetName, ok := stage.GetTargetName(); ok {
		return runCtx.Set(targetName, result)
	}

	// Handle map target (multi-field)
	if targetMap, ok := stage.GetTargetMap(); ok {
		resultMap, ok := result.(map[string]interface{})
		if !ok {
			return fmt.Errorf("multi-field target requires map result")
		}

		for key, fieldName := range targetMap {
			if value, exists := resultMap[key]; exists {
				if err := runCtx.Set(fieldName, value); err != nil {
					return err
				}
			}
		}
		return nil
	}

	return fmt.Errorf("invalid target type")
}

// handleError processes the onError handler
func (e *Executor) handleError(ctx context.Context, runCtx *Context, stage *StageDefinition, originalErr error) (interface{}, error) {
	// Create error context
	errorCtx := runCtx.Fork()
	errorCtx.Set("error", map[string]interface{}{
		"message": originalErr.Error(),
		"stage":   stage.ID,
		"type":    stage.Type,
	})

	// Parse onError as a mini-stage
	var onErrorStage StageDefinition
	onErrorStage.Type = "transform" // onError is always a transform
	onErrorStage.Action = stage.OnError

	// Execute error handler
	result, err := e.executeStage(ctx, errorCtx, &onErrorStage)
	if err != nil {
		// Error handler also failed, return original error
		return nil, originalErr
	}

	return result, nil
}

// buildOutput constructs the final output from the output stage
func (e *Executor) buildOutput(dag *DAG, runCtx *Context) (map[string]interface{}, error) {

	// Find output stage
	for _, stage := range dag.stages {
		if stage.Type == "output" {
			// Parse mapping from action
			var rawConfig map[string]interface{}
			if err := json.Unmarshal(stage.Action, &rawConfig); err != nil {
				return nil, fmt.Errorf("invalid output stage configuration: %w", err)
			}

			// Check if it has a "mapping" field (structured format)
			var mapping map[string]interface{}
			if mappingField, hasMappingField := rawConfig["mapping"]; hasMappingField {
				// Structured format with "mapping" field
				var ok bool
				mapping, ok = mappingField.(map[string]interface{})
				if !ok {
					return nil, fmt.Errorf("output mapping field must be an object")
				}
			} else {
				// Direct mapping format
				mapping = rawConfig
			}

			// Build output by resolving each mapping
			output := make(map[string]interface{})
			for key, templateValue := range mapping {
				// Convert template to string for resolution
				templateStr := fmt.Sprint(templateValue)

				// Resolve templates
				resolved, err := e.resolveOutputTemplate(templateStr, runCtx)
				if err != nil {
					return nil, fmt.Errorf("failed to resolve output mapping for '%s': %w", key, err)
				}

				output[key] = resolved
			}

			return output, nil
		}
	}

	// No output stage - return entire context
	return runCtx.GetAll(), nil
}

// resolveOutputTemplate resolves a template string for output mapping
func (e *Executor) resolveOutputTemplate(template string, runCtx *Context) (interface{}, error) {
	// This is a simplified version - in a full implementation,
	// we would import and use the expression package

	// Check if it's a simple variable reference
	if strings.HasPrefix(template, "${") && strings.HasSuffix(template, "}") {
		path := strings.TrimSuffix(strings.TrimPrefix(template, "${"), "}")
		value, found := runCtx.GetPath(path)
		if !found {
			return nil, fmt.Errorf("undefined variable: %s", path)
		}
		return value, nil
	}

	// Otherwise return as-is (literal value)
	return template, nil
}
