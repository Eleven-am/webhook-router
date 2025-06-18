package pipeline

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// BasicPipelineEngine implements the Engine interface
type BasicPipelineEngine struct {
	pipelines      map[string]Pipeline
	stageFactories map[string]StageFactory
	metrics        *Metrics
	isRunning      bool
	mu             sync.RWMutex
}

// BasicPipeline implements the Pipeline interface
type BasicPipeline struct {
	id        string
	name      string
	stages    []Stage
	config    *Config
	isRunning bool
	metrics   *Metric
	mu        sync.RWMutex
}

func NewBasicPipelineEngine() *BasicPipelineEngine {
	return &BasicPipelineEngine{
		pipelines:      make(map[string]Pipeline),
		stageFactories: make(map[string]StageFactory),
		metrics: &Metrics{
			PipelineMetrics: make(map[string]Metric),
			StageMetrics:    make(map[string]StageMetric),
		},
		isRunning: false,
	}
}

func NewBasicPipeline(id, name string) *BasicPipeline {
	return &BasicPipeline{
		id:     id,
		name:   name,
		stages: make([]Stage, 0),
		metrics: &Metric{
			LastExecution: time.Now(),
		},
		isRunning: false,
	}
}

// BasicPipelineEngine methods
func (e *BasicPipelineEngine) RegisterPipeline(pipeline Pipeline) error {
	if pipeline == nil {
		return fmt.Errorf("pipeline cannot be nil")
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	if _, exists := e.pipelines[pipeline.ID()]; exists {
		return fmt.Errorf("pipeline with ID %s already exists", pipeline.ID())
	}

	e.pipelines[pipeline.ID()] = pipeline

	// Initialize metrics for the pipeline
	e.metrics.PipelineMetrics[pipeline.ID()] = Metric{
		LastExecution: time.Now(),
	}

	return nil
}

func (e *BasicPipelineEngine) UnregisterPipeline(pipelineID string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if _, exists := e.pipelines[pipelineID]; !exists {
		return fmt.Errorf("pipeline with ID %s not found", pipelineID)
	}

	delete(e.pipelines, pipelineID)
	delete(e.metrics.PipelineMetrics, pipelineID)

	return nil
}

func (e *BasicPipelineEngine) GetPipeline(pipelineID string) (Pipeline, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	pipeline, exists := e.pipelines[pipelineID]
	if !exists {
		return nil, fmt.Errorf("pipeline with ID %s not found", pipelineID)
	}

	return pipeline, nil
}

func (e *BasicPipelineEngine) GetAllPipelines() []Pipeline {
	e.mu.RLock()
	defer e.mu.RUnlock()

	pipelines := make([]Pipeline, 0, len(e.pipelines))
	for _, pipeline := range e.pipelines {
		pipelines = append(pipelines, pipeline)
	}

	return pipelines
}

func (e *BasicPipelineEngine) ExecutePipeline(ctx context.Context, pipelineID string, data *Data) (*Result, error) {
	pipeline, err := e.GetPipeline(pipelineID)
	if err != nil {
		return nil, err
	}

	start := time.Now()
	result, err := pipeline.Execute(ctx, data)
	duration := time.Since(start)

	// Update metrics
	e.updateMetrics(pipelineID, duration, err == nil)

	return result, err
}

func (e *BasicPipelineEngine) RegisterStageFactory(stageType string, factory StageFactory) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.stageFactories[stageType] = factory
}

func (e *BasicPipelineEngine) CreateStage(stageType string, config map[string]interface{}) (Stage, error) {
	e.mu.RLock()
	factory, exists := e.stageFactories[stageType]
	e.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("no factory registered for stage type: %s", stageType)
	}

	return factory.Create(config)
}

func (e *BasicPipelineEngine) Start(ctx context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.isRunning {
		return fmt.Errorf("pipeline engine is already running")
	}

	// Start all pipelines
	for _, pipeline := range e.pipelines {
		if err := pipeline.Health(); err != nil {
			return fmt.Errorf("pipeline %s is not healthy: %w", pipeline.ID(), err)
		}
	}

	e.isRunning = true
	return nil
}

func (e *BasicPipelineEngine) Stop() error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if !e.isRunning {
		return fmt.Errorf("pipeline engine is not running")
	}

	e.isRunning = false
	return nil
}

func (e *BasicPipelineEngine) Health() error {
	e.mu.RLock()
	defer e.mu.RUnlock()

	if !e.isRunning {
		return fmt.Errorf("pipeline engine is not running")
	}

	// Check health of all pipelines
	for _, pipeline := range e.pipelines {
		if err := pipeline.Health(); err != nil {
			return fmt.Errorf("pipeline %s is unhealthy: %w", pipeline.ID(), err)
		}
	}

	return nil
}

func (e *BasicPipelineEngine) GetMetrics() (*Metrics, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	// Create a copy of metrics
	metricsCopy := &Metrics{
		TotalExecutions:      e.metrics.TotalExecutions,
		SuccessfulExecutions: e.metrics.SuccessfulExecutions,
		FailedExecutions:     e.metrics.FailedExecutions,
		AverageLatency:       e.metrics.AverageLatency,
		PipelineMetrics:      make(map[string]Metric),
		StageMetrics:         make(map[string]StageMetric),
	}

	for k, v := range e.metrics.PipelineMetrics {
		metricsCopy.PipelineMetrics[k] = v
	}

	for k, v := range e.metrics.StageMetrics {
		metricsCopy.StageMetrics[k] = v
	}

	return metricsCopy, nil
}

func (e *BasicPipelineEngine) updateMetrics(pipelineID string, duration time.Duration, success bool) {
	e.mu.Lock()
	defer e.mu.Unlock()

	// Update global metrics
	e.metrics.TotalExecutions++
	if success {
		e.metrics.SuccessfulExecutions++
	} else {
		e.metrics.FailedExecutions++
	}

	// Update average latency
	if e.metrics.TotalExecutions == 1 {
		e.metrics.AverageLatency = duration
	} else {
		// Weighted average
		e.metrics.AverageLatency = time.Duration(
			(int64(e.metrics.AverageLatency)*(e.metrics.TotalExecutions-1) + int64(duration)) / e.metrics.TotalExecutions,
		)
	}

	// Update pipeline-specific metrics
	pipelineMetric := e.metrics.PipelineMetrics[pipelineID]
	pipelineMetric.ExecutionCount++
	if success {
		pipelineMetric.SuccessCount++
	} else {
		pipelineMetric.FailureCount++
	}
	pipelineMetric.LastExecution = time.Now()

	// Update pipeline average latency
	if pipelineMetric.ExecutionCount == 1 {
		pipelineMetric.AverageLatency = duration
	} else {
		pipelineMetric.AverageLatency = time.Duration(
			(int64(pipelineMetric.AverageLatency)*(pipelineMetric.ExecutionCount-1) + int64(duration)) / pipelineMetric.ExecutionCount,
		)
	}

	e.metrics.PipelineMetrics[pipelineID] = pipelineMetric
}

// BasicPipeline methods
func (p *BasicPipeline) ID() string {
	return p.id
}

func (p *BasicPipeline) Name() string {
	return p.name
}

func (p *BasicPipeline) Execute(ctx context.Context, data *Data) (*Result, error) {
	start := time.Now()

	result := &Result{
		PipelineID:   p.id,
		InputData:    data,
		StageResults: make([]StageResult, 0),
		Metadata:     make(map[string]interface{}),
		Success:      false,
	}

	// Clone input data for processing
	currentData := data.Clone()

	p.mu.RLock()
	stages := make([]Stage, len(p.stages))
	copy(stages, p.stages)
	p.mu.RUnlock()

	// Execute stages sequentially
	for i, stage := range stages {
		stageStart := time.Now()

		// Check context cancellation
		select {
		case <-ctx.Done():
			result.Error = "pipeline execution cancelled"
			result.TotalDuration = time.Since(start)
			return result, ctx.Err()
		default:
		}

		// Execute stage
		stageResult, err := stage.Process(ctx, currentData)
		if err != nil {
			result.Error = fmt.Sprintf("stage %s failed: %v", stage.Name(), err)
			result.TotalDuration = time.Since(start)
			return result, err
		}

		// Handle stage result
		if stageResult != nil {
			result.StageResults = append(result.StageResults, *stageResult)

			if !stageResult.Success {
				// Handle stage failure based on configuration
				if p.config != nil && i < len(p.config.Stages) {
					stageConfig := p.config.Stages[i]

					switch stageConfig.OnError {
					case OnErrorStop:
						result.Error = fmt.Sprintf("stage %s failed: %s", stage.Name(), stageResult.Error)
						result.TotalDuration = time.Since(start)
						return result, nil

					case OnErrorContinue:
						// Continue to next stage
						continue

					case OnErrorRetry:
						// Implement retry logic
						if err := p.retryStage(ctx, stage, currentData, stageConfig.RetryConfig); err != nil {
							result.Error = fmt.Sprintf("stage %s failed after retries: %v", stage.Name(), err)
							result.TotalDuration = time.Since(start)
							return result, nil
						}

					case OnErrorSkip:
						// Skip this stage and continue
						continue
					}
				} else {
					// Default behavior: stop on error
					result.Error = fmt.Sprintf("stage %s failed: %s", stage.Name(), stageResult.Error)
					result.TotalDuration = time.Since(start)
					return result, nil
				}
			}

			// Update current data for next stage
			if stageResult.Data != nil {
				currentData = stageResult.Data
			}
		}

		// Update stage metrics
		p.updateStageMetrics(stage.Name(), time.Since(stageStart), stageResult != nil && stageResult.Success)
	}

	result.OutputData = currentData
	result.Success = true
	result.TotalDuration = time.Since(start)

	// Update pipeline metrics
	p.updatePipelineMetrics(time.Since(start), true)

	return result, nil
}

func (p *BasicPipeline) retryStage(ctx context.Context, stage Stage, data *Data, retryConfig RetryConfig) error {
	if !retryConfig.Enabled {
		return fmt.Errorf("retry not enabled")
	}

	maxAttempts := retryConfig.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = 3 // Default
	}

	delay := retryConfig.Delay
	if delay <= 0 {
		delay = time.Second // Default 1 second
	}

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		// Wait before retry (except for first attempt)
		if attempt > 1 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(p.calculateRetryDelay(delay, attempt, retryConfig.BackoffType, retryConfig.MaxDelay)):
			}
		}

		stageResult, err := stage.Process(ctx, data)
		if err == nil && stageResult != nil && stageResult.Success {
			return nil // Success
		}
	}

	return fmt.Errorf("stage failed after %d attempts", maxAttempts)
}

func (p *BasicPipeline) calculateRetryDelay(baseDelay time.Duration, attempt int, backoffType string, maxDelay time.Duration) time.Duration {
	var delay time.Duration

	switch backoffType {
	case "exponential":
		delay = baseDelay * time.Duration(1<<(attempt-1)) // 2^(attempt-1)
	case "linear":
		delay = baseDelay * time.Duration(attempt)
	default: // "fixed"
		delay = baseDelay
	}

	if maxDelay > 0 && delay > maxDelay {
		delay = maxDelay
	}

	return delay
}

func (p *BasicPipeline) AddStage(stage Stage) error {
	if stage == nil {
		return fmt.Errorf("stage cannot be nil")
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	// Validate stage
	if err := stage.Validate(); err != nil {
		return fmt.Errorf("stage validation failed: %w", err)
	}

	p.stages = append(p.stages, stage)
	return nil
}

func (p *BasicPipeline) RemoveStage(stageName string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	for i, stage := range p.stages {
		if stage.Name() == stageName {
			// Remove stage from slice
			p.stages = append(p.stages[:i], p.stages[i+1:]...)
			return nil
		}
	}

	return fmt.Errorf("stage %s not found", stageName)
}

func (p *BasicPipeline) GetStages() []Stage {
	p.mu.RLock()
	defer p.mu.RUnlock()

	// Return copies to avoid external modifications
	stages := make([]Stage, len(p.stages))
	copy(stages, p.stages)

	return stages
}

func (p *BasicPipeline) GetStage(stageName string) (Stage, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	for _, stage := range p.stages {
		if stage.Name() == stageName {
			return stage, nil
		}
	}

	return nil, fmt.Errorf("stage %s not found", stageName)
}

func (p *BasicPipeline) Validate() error {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if len(p.stages) == 0 {
		return fmt.Errorf("pipeline must have at least one stage")
	}

	// Validate all stages
	for i, stage := range p.stages {
		if err := stage.Validate(); err != nil {
			return fmt.Errorf("stage %d (%s) validation failed: %w", i, stage.Name(), err)
		}
	}

	return nil
}

func (p *BasicPipeline) Health() error {
	p.mu.RLock()
	defer p.mu.RUnlock()

	// Check health of all stages
	for _, stage := range p.stages {
		if err := stage.Health(); err != nil {
			return fmt.Errorf("stage %s is unhealthy: %w", stage.Name(), err)
		}
	}

	return nil
}

func (p *BasicPipeline) updatePipelineMetrics(duration time.Duration, success bool) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.metrics.ExecutionCount++
	if success {
		p.metrics.SuccessCount++
	} else {
		p.metrics.FailureCount++
	}
	p.metrics.LastExecution = time.Now()

	// Update average latency
	if p.metrics.ExecutionCount == 1 {
		p.metrics.AverageLatency = duration
	} else {
		p.metrics.AverageLatency = time.Duration(
			(int64(p.metrics.AverageLatency)*(p.metrics.ExecutionCount-1) + int64(duration)) / p.metrics.ExecutionCount,
		)
	}
}

func (p *BasicPipeline) updateStageMetrics(stageName string, duration time.Duration, success bool) {
	// This would typically be handled by the pipeline engine
	// For simplicity, we'll skip this implementation here
}

// CreatePipelineFromConfig creates a pipeline from configuration
func (e *BasicPipelineEngine) CreatePipelineFromConfig(config *Config) (Pipeline, error) {
	pipeline := NewBasicPipeline(config.ID, config.Name)
	pipeline.config = config

	// Create and add stages
	for _, stageConfig := range config.Stages {
		if !stageConfig.Enabled {
			continue // Skip disabled stages
		}

		stage, err := e.CreateStage(stageConfig.Type, stageConfig.Config)
		if err != nil {
			return nil, fmt.Errorf("failed to create stage %s: %w", stageConfig.Name, err)
		}

		if err := pipeline.AddStage(stage); err != nil {
			return nil, fmt.Errorf("failed to add stage %s: %w", stageConfig.Name, err)
		}
	}

	// Validate pipeline
	if err := pipeline.Validate(); err != nil {
		return nil, fmt.Errorf("pipeline validation failed: %w", err)
	}

	return pipeline, nil
}
