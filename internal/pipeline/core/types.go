package core

import (
	"encoding/json"
	"time"
)

// StageDefinition represents a single stage from the JSON config
type StageDefinition struct {
	ID        string          `json:"id"`
	Type      string          `json:"type"`
	Target    interface{}     `json:"target"` // string or map[string]string for multi-field
	Action    json.RawMessage `json:"action"` // Defer parsing to stage implementations
	DependsOn []string        `json:"dependsOn"`
	Timeout   string          `json:"timeout,omitempty"`
	OnError   json.RawMessage `json:"onError,omitempty"`
	Cache     json.RawMessage `json:"cache,omitempty"`

	// Special fields for specific stage types
	Items  json.RawMessage `json:"items,omitempty"`  // For foreach
	Filter json.RawMessage `json:"filter,omitempty"` // For filter stage
}

// PipelineDefinition represents the entire pipeline configuration
type PipelineDefinition struct {
	ID      string            `json:"id"`
	Name    string            `json:"name"`
	Version string            `json:"version,omitempty"`
	Stages  []StageDefinition `json:"stages"`

	// Global settings
	DefaultTimeout string                 `json:"defaultTimeout,omitempty"`
	Resources      map[string]interface{} `json:"resources,omitempty"` // ${api}, ${db}, etc.
}

// GetTimeout returns the parsed timeout duration for a stage
func (s *StageDefinition) GetTimeout(defaultTimeout time.Duration) time.Duration {
	if s.Timeout == "" {
		return defaultTimeout
	}

	duration, err := time.ParseDuration(s.Timeout)
	if err != nil {
		return defaultTimeout
	}

	return duration
}

// IsSpecialStage returns true for input/output stages that don't follow standard pattern
func (s *StageDefinition) IsSpecialStage() bool {
	return s.Type == "input" || s.Type == "output"
}

// GetTargetName returns the target as a string (for simple targets)
func (s *StageDefinition) GetTargetName() (string, bool) {
	if str, ok := s.Target.(string); ok {
		return str, true
	}
	return "", false
}

// GetTargetMap returns the target as a map (for multi-field targets)
func (s *StageDefinition) GetTargetMap() (map[string]string, bool) {
	if m, ok := s.Target.(map[string]interface{}); ok {
		result := make(map[string]string)
		for k, v := range m {
			if str, ok := v.(string); ok {
				result[k] = str
			}
		}
		return result, true
	}
	return nil, false
}
