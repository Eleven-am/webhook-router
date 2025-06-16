package schedule

import (
	"fmt"
	"strings"
	"time"
	"webhook-router/internal/triggers"
)

// Config represents the configuration for scheduled triggers
type Config struct {
	triggers.BaseTriggerConfig
	
	// Schedule-specific settings
	ScheduleType string            `json:"schedule_type"` // cron, interval, once
	CronSpec     string            `json:"cron_spec"`     // Cron expression (for cron type)
	Interval     time.Duration     `json:"interval"`      // Interval duration (for interval type)
	StartTime    *time.Time        `json:"start_time"`    // When to start (for once type or delayed start)
	EndTime      *time.Time        `json:"end_time"`      // When to stop (optional)
	Timezone     string            `json:"timezone"`      // Timezone for cron expressions
	MaxRuns      int               `json:"max_runs"`      // Maximum number of runs (0 = unlimited)
	Payload      PayloadConfig     `json:"payload"`       // Data to send when triggered
	Retry        RetryConfig       `json:"retry"`         // Retry configuration
}

// PayloadConfig defines what data to send when the trigger fires
type PayloadConfig struct {
	Type     string                 `json:"type"`     // static, template, http_call
	Data     map[string]interface{} `json:"data"`     // Static data or template variables
	Template string                 `json:"template"` // Template string for dynamic payloads
	HTTPCall HTTPCallConfig         `json:"http_call"` // HTTP call configuration
}

// HTTPCallConfig for making HTTP calls to get payload data
type HTTPCallConfig struct {
	URL     string            `json:"url"`
	Method  string            `json:"method"`
	Headers map[string]string `json:"headers"`
	Body    string            `json:"body"`
	Timeout time.Duration     `json:"timeout"`
}

// RetryConfig defines retry behavior for failed executions
type RetryConfig struct {
	Enabled     bool          `json:"enabled"`      // Enable retries
	MaxRetries  int           `json:"max_retries"`  // Maximum retry attempts
	RetryDelay  time.Duration `json:"retry_delay"`  // Delay between retries
	BackoffType string        `json:"backoff_type"` // fixed, exponential, linear
}

func NewConfig(name string) *Config {
	return &Config{
		BaseTriggerConfig: triggers.BaseTriggerConfig{
			Name:     name,
			Type:     "schedule",
			Active:   true,
			Settings: make(map[string]interface{}),
		},
		ScheduleType: "interval",
		Interval:     time.Minute,
		Timezone:     "UTC",
		MaxRuns:      0, // Unlimited
		Payload: PayloadConfig{
			Type: "static",
			Data: make(map[string]interface{}),
		},
		Retry: RetryConfig{
			Enabled:     false,
			MaxRetries:  3,
			RetryDelay:  time.Second * 30,
			BackoffType: "exponential",
		},
	}
}

func (c *Config) Validate() error {
	if c.Name == "" {
		return fmt.Errorf("trigger name is required")
	}
	
	// Validate schedule type
	validTypes := map[string]bool{
		"cron": true, "interval": true, "once": true,
	}
	if !validTypes[c.ScheduleType] {
		return fmt.Errorf("invalid schedule type: %s", c.ScheduleType)
	}
	
	// Validate schedule-specific settings
	switch c.ScheduleType {
	case "cron":
		if c.CronSpec == "" {
			return fmt.Errorf("cron_spec is required for cron schedule type")
		}
		if err := c.validateCronSpec(); err != nil {
			return fmt.Errorf("invalid cron spec: %w", err)
		}
	case "interval":
		if c.Interval <= 0 {
			return fmt.Errorf("interval must be positive for interval schedule type")
		}
		if c.Interval < time.Second {
			return fmt.Errorf("minimum interval is 1 second")
		}
	case "once":
		if c.StartTime == nil {
			return fmt.Errorf("start_time is required for once schedule type")
		}
		if c.StartTime.Before(time.Now()) {
			return fmt.Errorf("start_time must be in the future for once schedule type")
		}
	}
	
	// Validate timezone
	if c.Timezone == "" {
		c.Timezone = "UTC"
	}
	if _, err := time.LoadLocation(c.Timezone); err != nil {
		return fmt.Errorf("invalid timezone: %s", c.Timezone)
	}
	
	// Validate end time
	if c.EndTime != nil && c.StartTime != nil {
		if c.EndTime.Before(*c.StartTime) {
			return fmt.Errorf("end_time cannot be before start_time")
		}
	}
	
	// Validate max runs
	if c.MaxRuns < 0 {
		c.MaxRuns = 0 // 0 means unlimited
	}
	
	// Validate payload config
	if err := c.validatePayload(); err != nil {
		return fmt.Errorf("payload config error: %w", err)
	}
	
	// Validate retry config
	if err := c.validateRetry(); err != nil {
		return fmt.Errorf("retry config error: %w", err)
	}
	
	return nil
}

func (c *Config) validateCronSpec() error {
	// Basic cron spec validation (5 or 6 fields)
	fields := strings.Fields(c.CronSpec)
	if len(fields) != 5 && len(fields) != 6 {
		return fmt.Errorf("cron spec must have 5 or 6 fields, got %d", len(fields))
	}
	
	// Additional validation could be added here
	// For now, we'll rely on the cron library for detailed validation
	
	return nil
}

func (c *Config) validatePayload() error {
	validTypes := map[string]bool{
		"static": true, "template": true, "http_call": true,
	}
	if !validTypes[c.Payload.Type] {
		return fmt.Errorf("invalid payload type: %s", c.Payload.Type)
	}
	
	switch c.Payload.Type {
	case "template":
		if c.Payload.Template == "" {
			return fmt.Errorf("template is required for template payload type")
		}
	case "http_call":
		if c.Payload.HTTPCall.URL == "" {
			return fmt.Errorf("URL is required for http_call payload type")
		}
		if c.Payload.HTTPCall.Method == "" {
			c.Payload.HTTPCall.Method = "GET"
		}
		if c.Payload.HTTPCall.Timeout <= 0 {
			c.Payload.HTTPCall.Timeout = 30 * time.Second
		}
	}
	
	return nil
}

func (c *Config) validateRetry() error {
	if c.Retry.Enabled {
		if c.Retry.MaxRetries <= 0 {
			c.Retry.MaxRetries = 3
		}
		if c.Retry.RetryDelay <= 0 {
			c.Retry.RetryDelay = 30 * time.Second
		}
		
		validBackoffs := map[string]bool{
			"fixed": true, "exponential": true, "linear": true,
		}
		if !validBackoffs[c.Retry.BackoffType] {
			c.Retry.BackoffType = "exponential"
		}
	}
	
	return nil
}

func (c *Config) GetNextExecution(from time.Time) (*time.Time, error) {
	location, err := time.LoadLocation(c.Timezone)
	if err != nil {
		return nil, err
	}
	
	fromInTz := from.In(location)
	
	switch c.ScheduleType {
	case "once":
		if c.StartTime != nil && c.StartTime.After(from) {
			next := c.StartTime.In(location)
			return &next, nil
		}
		return nil, nil // Already executed or past start time
		
	case "interval":
		var next time.Time
		if c.StartTime != nil {
			// Start from the specified start time
			next = c.StartTime.In(location)
			if next.Before(fromInTz) {
				// Calculate next interval from start time
				elapsed := fromInTz.Sub(next)
				intervals := int(elapsed / c.Interval)
				next = next.Add(time.Duration(intervals+1) * c.Interval)
			}
		} else {
			// Start from current time + interval
			next = fromInTz.Add(c.Interval)
		}
		
		// Check end time
		if c.EndTime != nil && next.After(*c.EndTime) {
			return nil, nil
		}
		
		return &next, nil
		
	case "cron":
		// This would require a cron parsing library
		// For now, return a placeholder implementation
		next := fromInTz.Add(time.Hour) // Placeholder
		if c.EndTime != nil && next.After(*c.EndTime) {
			return nil, nil
		}
		return &next, nil
		
	default:
		return nil, fmt.Errorf("unsupported schedule type: %s", c.ScheduleType)
	}
}

func (c *Config) IsExpired() bool {
	if c.EndTime == nil {
		return false
	}
	return time.Now().After(*c.EndTime)
}

func (c *Config) ShouldRetry(attempt int) bool {
	if !c.Retry.Enabled {
		return false
	}
	return attempt <= c.Retry.MaxRetries
}

func (c *Config) GetRetryDelay(attempt int) time.Duration {
	if !c.Retry.Enabled {
		return 0
	}
	
	baseDelay := c.Retry.RetryDelay
	
	switch c.Retry.BackoffType {
	case "fixed":
		return baseDelay
	case "linear":
		return time.Duration(attempt) * baseDelay
	case "exponential":
		factor := 1
		for i := 1; i < attempt; i++ {
			factor *= 2
		}
		return time.Duration(factor) * baseDelay
	default:
		return baseDelay
	}
}