package pipeline

import (
	"time"
)

// Metadata contains pipeline execution metadata that works across all trigger types
type Metadata struct {
	// Core identifiers
	UserID      string    `json:"user_id"`      // Pipeline owner
	PipelineID  string    `json:"pipeline_id"`  // Pipeline being executed
	ExecutionID string    `json:"execution_id"` // Unique execution identifier
	StartTime   time.Time `json:"start_time"`   // Execution start time

	// Trigger information (generic)
	TriggerID   string `json:"trigger_id"`   // ID of the trigger
	TriggerType string `json:"trigger_type"` // http, broker, schedule, polling, etc.

	// Source information (trigger-agnostic)
	Source SourceInfo `json:"source"`

	// Execution context
	Environment string `json:"environment"` // dev, staging, prod
	Region      string `json:"region"`      // Geographic region if applicable
	Instance    string `json:"instance"`    // Server/container instance ID

	// Tracing
	TraceID       string `json:"trace_id"`       // Distributed trace ID
	SpanID        string `json:"span_id"`        // Current span ID
	ParentSpanID  string `json:"parent_span_id"` // Parent span if nested
	CorrelationID string `json:"correlation_id"` // Business correlation ID

	// Resource management
	Priority   string        `json:"priority"`    // high, normal, low
	Timeout    time.Duration `json:"timeout"`     // Execution timeout
	MaxRetries int           `json:"max_retries"` // Retry limit
	RetryCount int           `json:"retry_count"` // Current retry attempt
	Partition  string        `json:"partition"`   // For partitioned processing

	// Feature flags and tags
	Features map[string]bool   `json:"features"` // Enabled feature flags
	Tags     []string          `json:"tags"`     // Custom tags
	Labels   map[string]string `json:"labels"`   // Key-value labels
}

// SourceInfo contains trigger-agnostic source information
type SourceInfo struct {
	// For HTTP: URL path, method
	// For Broker: topic/queue, message ID
	// For Schedule: cron expression
	// For Polling: resource being polled
	Type       string                 `json:"type"`       // Describes what initiated this
	Identifier string                 `json:"identifier"` // Unique identifier from source
	Metadata   map[string]interface{} `json:"metadata"`   // Additional source-specific data
}

// MetadataKey is the key used to store metadata in the context
const MetadataKey = "_metadata"

// NewMetadata creates a new metadata instance with defaults
func NewMetadata(userID, pipelineID, executionID string) *Metadata {
	return &Metadata{
		UserID:      userID,
		PipelineID:  pipelineID,
		ExecutionID: executionID,
		StartTime:   time.Now(),
		Features:    make(map[string]bool),
		Tags:        []string{},
		Labels:      make(map[string]string),
		Source: SourceInfo{
			Metadata: make(map[string]interface{}),
		},
	}
}

// SetTriggerInfo sets trigger-specific information
func (m *Metadata) SetTriggerInfo(triggerID, triggerType string) {
	m.TriggerID = triggerID
	m.TriggerType = triggerType
}

// SetSourceInfo sets source-specific information in a generic way
func (m *Metadata) SetSourceInfo(sourceType, identifier string, metadata map[string]interface{}) {
	m.Source.Type = sourceType
	m.Source.Identifier = identifier
	if metadata != nil {
		m.Source.Metadata = metadata
	} else {
		m.Source.Metadata = make(map[string]interface{})
	}
}

// SetEnvironment sets the environment context
func (m *Metadata) SetEnvironment(env, region, instance string) {
	m.Environment = env
	m.Region = region
	m.Instance = instance
}

// SetTracing sets distributed tracing information
func (m *Metadata) SetTracing(traceID, spanID, parentSpanID, correlationID string) {
	m.TraceID = traceID
	m.SpanID = spanID
	m.ParentSpanID = parentSpanID
	m.CorrelationID = correlationID
}

// AddTag adds a tag to the metadata
func (m *Metadata) AddTag(tag string) {
	m.Tags = append(m.Tags, tag)
}

// SetLabel sets a label key-value pair
func (m *Metadata) SetLabel(key, value string) {
	if m.Labels == nil {
		m.Labels = make(map[string]string)
	}
	m.Labels[key] = value
}

// SetFeature sets a feature flag
func (m *Metadata) SetFeature(feature string, enabled bool) {
	if m.Features == nil {
		m.Features = make(map[string]bool)
	}
	m.Features[feature] = enabled
}
