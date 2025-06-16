package pipeline

import (
	"context"
	"time"
)

// PipelineData represents data flowing through the pipeline
type PipelineData struct {
	ID          string                 `json:"id"`
	OriginalID  string                 `json:"original_id"`
	Headers     map[string]string      `json:"headers"`
	Body        []byte                 `json:"body"`
	ContentType string                 `json:"content_type"`
	Metadata    map[string]interface{} `json:"metadata"`
	Timestamp   time.Time              `json:"timestamp"`
	Context     map[string]interface{} `json:"context"`
	Error       string                 `json:"error,omitempty"`
}

// StageResult represents the result of a pipeline stage
type StageResult struct {
	Data     *PipelineData     `json:"data"`
	Success  bool              `json:"success"`
	Error    string            `json:"error,omitempty"`
	Duration time.Duration     `json:"duration"`
	Metadata map[string]interface{} `json:"metadata"`
}

// PipelineResult represents the result of pipeline execution
type PipelineResult struct {
	PipelineID    string                 `json:"pipeline_id"`
	InputData     *PipelineData          `json:"input_data"`
	OutputData    *PipelineData          `json:"output_data"`
	Success       bool                   `json:"success"`
	Error         string                 `json:"error,omitempty"`
	TotalDuration time.Duration          `json:"total_duration"`
	StageResults  []StageResult          `json:"stage_results"`
	Metadata      map[string]interface{} `json:"metadata"`
}

// Stage represents a single processing stage in a pipeline
type Stage interface {
	// Name returns the stage name
	Name() string
	
	// Type returns the stage type
	Type() string
	
	// Process processes the data
	Process(ctx context.Context, data *PipelineData) (*StageResult, error)
	
	// Configure configures the stage with given settings
	Configure(config map[string]interface{}) error
	
	// Validate validates the stage configuration
	Validate() error
	
	// Health returns the health status of the stage
	Health() error
}

// Pipeline represents a data processing pipeline
type Pipeline interface {
	// ID returns the pipeline ID
	ID() string
	
	// Name returns the pipeline name
	Name() string
	
	// Execute executes the pipeline with input data
	Execute(ctx context.Context, data *PipelineData) (*PipelineResult, error)
	
	// AddStage adds a stage to the pipeline
	AddStage(stage Stage) error
	
	// RemoveStage removes a stage from the pipeline
	RemoveStage(stageName string) error
	
	// GetStages returns all stages in the pipeline
	GetStages() []Stage
	
	// GetStage returns a specific stage
	GetStage(stageName string) (Stage, error)
	
	// Validate validates the entire pipeline
	Validate() error
	
	// Health returns the health status of the pipeline
	Health() error
}

// PipelineEngine manages multiple pipelines
type PipelineEngine interface {
	// RegisterPipeline registers a pipeline
	RegisterPipeline(pipeline Pipeline) error
	
	// UnregisterPipeline unregisters a pipeline
	UnregisterPipeline(pipelineID string) error
	
	// GetPipeline returns a pipeline by ID
	GetPipeline(pipelineID string) (Pipeline, error)
	
	// GetAllPipelines returns all registered pipelines
	GetAllPipelines() []Pipeline
	
	// ExecutePipeline executes a pipeline by ID
	ExecutePipeline(ctx context.Context, pipelineID string, data *PipelineData) (*PipelineResult, error)
	
	// RegisterStageFactory registers a stage factory
	RegisterStageFactory(stageType string, factory StageFactory) error
	
	// CreateStage creates a stage using registered factories
	CreateStage(stageType string, config map[string]interface{}) (Stage, error)
	
	// Start starts the pipeline engine
	Start(ctx context.Context) error
	
	// Stop stops the pipeline engine
	Stop() error
	
	// Health returns the health status of the engine
	Health() error
	
	// GetMetrics returns pipeline execution metrics
	GetMetrics() (*PipelineMetrics, error)
	
	// CreatePipelineFromConfig creates a pipeline from configuration
	CreatePipelineFromConfig(config *PipelineConfig) (Pipeline, error)
}

// StageFactory creates stages
type StageFactory interface {
	// Create creates a new stage instance
	Create(config map[string]interface{}) (Stage, error)
	
	// GetType returns the stage type this factory creates
	GetType() string
	
	// GetConfigSchema returns the configuration schema for this stage type
	GetConfigSchema() map[string]interface{}
}

// PipelineMetrics contains execution metrics
type PipelineMetrics struct {
	TotalExecutions    int64                          `json:"total_executions"`
	SuccessfulExecutions int64                        `json:"successful_executions"`
	FailedExecutions   int64                          `json:"failed_executions"`
	AverageLatency     time.Duration                  `json:"average_latency"`
	PipelineMetrics    map[string]PipelineMetric      `json:"pipeline_metrics"`
	StageMetrics       map[string]StageMetric         `json:"stage_metrics"`
}

// PipelineMetric contains metrics for a specific pipeline
type PipelineMetric struct {
	ExecutionCount     int64         `json:"execution_count"`
	SuccessCount       int64         `json:"success_count"`
	FailureCount       int64         `json:"failure_count"`
	AverageLatency     time.Duration `json:"average_latency"`
	LastExecution      time.Time     `json:"last_execution"`
}

// StageMetric contains metrics for a specific stage
type StageMetric struct {
	ExecutionCount     int64         `json:"execution_count"`
	SuccessCount       int64         `json:"success_count"`
	FailureCount       int64         `json:"failure_count"`
	AverageLatency     time.Duration `json:"average_latency"`
	LastExecution      time.Time     `json:"last_execution"`
}

// StageConfig represents stage configuration
type StageConfig struct {
	Name        string                 `json:"name"`
	Type        string                 `json:"type"`
	Config      map[string]interface{} `json:"config"`
	Enabled     bool                   `json:"enabled"`
	OnError     string                 `json:"on_error"`     // continue, stop, retry
	RetryConfig RetryConfig            `json:"retry_config"`
	Timeout     time.Duration          `json:"timeout"`
}

// RetryConfig defines retry behavior for stages
type RetryConfig struct {
	Enabled     bool          `json:"enabled"`
	MaxAttempts int           `json:"max_attempts"`
	Delay       time.Duration `json:"delay"`
	BackoffType string        `json:"backoff_type"` // fixed, exponential, linear
	MaxDelay    time.Duration `json:"max_delay"`
}

// PipelineConfig represents pipeline configuration
type PipelineConfig struct {
	ID          string        `json:"id"`
	Name        string        `json:"name"`
	Description string        `json:"description"`
	Stages      []StageConfig `json:"stages"`
	Enabled     bool          `json:"enabled"`
	Tags        []string      `json:"tags"`
	CreatedAt   time.Time     `json:"created_at"`
	UpdatedAt   time.Time     `json:"updated_at"`
}

// Common stage types
const (
	StageTypeTransform   = "transform"
	StageTypeValidate    = "validate"
	StageTypeFilter      = "filter"
	StageTypeEnrich      = "enrich"
	StageTypeAggregate   = "aggregate"
	StageTypeConvert     = "convert"
	StageTypeScript      = "script"
	StageTypeHTTPRequest = "http_request"
	StageTypeDelay       = "delay"
	StageTypeLog         = "log"
)

// Error handling strategies
const (
	OnErrorContinue = "continue"
	OnErrorStop     = "stop"
	OnErrorRetry    = "retry"
	OnErrorSkip     = "skip"
)

// Helper functions
func NewPipelineData(id string, body []byte, headers map[string]string) *PipelineData {
	return &PipelineData{
		ID:          id,
		OriginalID:  id,
		Headers:     headers,
		Body:        body,
		ContentType: headers["Content-Type"],
		Metadata:    make(map[string]interface{}),
		Context:     make(map[string]interface{}),
		Timestamp:   time.Now(),
	}
}

func (pd *PipelineData) Clone() *PipelineData {
	clone := &PipelineData{
		ID:          pd.ID,
		OriginalID:  pd.OriginalID,
		Body:        make([]byte, len(pd.Body)),
		ContentType: pd.ContentType,
		Timestamp:   pd.Timestamp,
		Error:       pd.Error,
	}
	
	copy(clone.Body, pd.Body)
	
	// Clone headers
	clone.Headers = make(map[string]string)
	for k, v := range pd.Headers {
		clone.Headers[k] = v
	}
	
	// Clone metadata
	clone.Metadata = make(map[string]interface{})
	for k, v := range pd.Metadata {
		clone.Metadata[k] = v
	}
	
	// Clone context
	clone.Context = make(map[string]interface{})
	for k, v := range pd.Context {
		clone.Context[k] = v
	}
	
	return clone
}

func (pd *PipelineData) SetHeader(key, value string) {
	if pd.Headers == nil {
		pd.Headers = make(map[string]string)
	}
	pd.Headers[key] = value
}

func (pd *PipelineData) GetHeader(key string) string {
	if pd.Headers == nil {
		return ""
	}
	return pd.Headers[key]
}

func (pd *PipelineData) SetMetadata(key string, value interface{}) {
	if pd.Metadata == nil {
		pd.Metadata = make(map[string]interface{})
	}
	pd.Metadata[key] = value
}

func (pd *PipelineData) GetMetadata(key string) interface{} {
	if pd.Metadata == nil {
		return nil
	}
	return pd.Metadata[key]
}

func (pd *PipelineData) SetContext(key string, value interface{}) {
	if pd.Context == nil {
		pd.Context = make(map[string]interface{})
	}
	pd.Context[key] = value
}

func (pd *PipelineData) GetContext(key string) interface{} {
	if pd.Context == nil {
		return nil
	}
	return pd.Context[key]
}