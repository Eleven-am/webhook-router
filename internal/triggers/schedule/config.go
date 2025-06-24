package schedule

import (
	"fmt"
	"time"
	"webhook-router/internal/common/errors"
	"webhook-router/internal/common/validation"
	"webhook-router/internal/triggers"

	"github.com/robfig/cron/v3"
)

// Config represents the configuration for scheduled triggers
type Config struct {
	triggers.BaseTriggerConfig

	// Schedule-specific settings
	ScheduleType    string                 `json:"schedule_type" validate:"required,oneof=cron interval once"`
	CronSpec        string                 `json:"cron_spec" validate:"required_if=ScheduleType cron,omitempty,cron_expression"`
	Interval        time.Duration          `json:"interval" validate:"required_if=ScheduleType interval,omitempty,min=1s"`
	StartTime       *time.Time             `json:"start_time"` // When to start (for once type or delayed start)
	EndTime         *time.Time             `json:"end_time"`   // When to stop (optional)
	Timezone        string                 `json:"timezone" validate:"omitempty,timezone"`
	MaxRuns         int                    `json:"max_runs" validate:"min=0"`
	RunImmediately  bool                   `json:"run_immediately"` // Execute immediately on start (default: true)
	CustomData      map[string]interface{} `json:"custom_data"`     // Custom data to include in events
	DataSource      DataSourceConfig       `json:"data_source" validate:"dive"`
	DataTransform   TransformConfig        `json:"data_transform" validate:"dive"`
	ContinueOnError bool                   `json:"continue_on_error"` // Continue if data fetch fails
}

// DataSourceConfig defines configuration for fetching external data
type DataSourceConfig struct {
	URL             string            `json:"url" validate:"omitempty,url"`
	Method          string            `json:"method" validate:"omitempty,http_method"`
	Headers         map[string]string `json:"headers"`
	Timeout         time.Duration     `json:"timeout" validate:"omitempty,min=1s"`
	OAuth2ServiceID string            `json:"oauth2_service_id,omitempty"` // OAuth2 service for authentication
}

// TransformConfig defines data transformation settings
type TransformConfig struct {
	Enabled  bool   `json:"enabled"`
	Template string `json:"template" validate:"required_if=Enabled true"`
}

func NewConfig(name string) *Config {
	return &Config{
		BaseTriggerConfig: triggers.BaseTriggerConfig{
			Name:     name,
			Type:     "schedule",
			Active:   true,
			Settings: make(map[string]interface{}),
		},
		ScheduleType:    "interval",
		Interval:        time.Minute,
		Timezone:        "UTC",
		MaxRuns:         0,    // Unlimited
		RunImmediately:  true, // Default to immediate execution
		CustomData:      make(map[string]interface{}),
		ContinueOnError: true,
		DataSource: DataSourceConfig{
			Method:  "GET",
			Headers: make(map[string]string),
			Timeout: 30 * time.Second,
		},
		DataTransform: TransformConfig{
			Enabled: false,
		},
	}
}

func (c *Config) Validate() error {
	// Apply defaults first
	if c.DataSource.Timeout <= 0 && c.DataSource.URL != "" {
		c.DataSource.Timeout = 30 * time.Second
	}

	// Use centralized validation with struct tags
	if err := validation.ValidateStruct(c); err != nil {
		return err
	}

	// Additional custom validation that can't be done with tags
	v := validation.NewValidator()

	// Validate time constraints
	if c.StartTime != nil && c.EndTime != nil {
		if c.StartTime.After(*c.EndTime) {
			v.Validate(func() error {
				return errors.ConfigError("start_time cannot be after end_time")
			})
		}
	}

	// Validate start_time requirement for 'once' schedule type
	if c.ScheduleType == "once" && c.StartTime == nil {
		v.Validate(func() error {
			return errors.ConfigError("start_time is required for 'once' schedule type")
		})
	}

	return v.Error()
}

// GetInterval returns the interval for the schedule
func (c *Config) GetInterval() time.Duration {
	switch c.ScheduleType {
	case "interval":
		return c.Interval
	case "cron":
		// For cron, return a reasonable check interval
		return time.Minute
	case "once":
		// For once, return a large interval as it only runs once
		return time.Hour * 24 * 365
	default:
		return time.Minute
	}
}

// IsExpired checks if the trigger has expired based on EndTime
func (c *Config) IsExpired() bool {
	if c.EndTime == nil {
		return false
	}
	return time.Now().After(*c.EndTime)
}

// GetNextExecutionTime calculates the next execution time based on the schedule type
func (c *Config) GetNextExecutionTime(from time.Time) (time.Time, error) {
	switch c.ScheduleType {
	case "interval":
		return from.Add(c.Interval), nil

	case "cron":
		parser := cron.NewParser(cron.Second | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
		schedule, err := parser.Parse(c.CronSpec)
		if err != nil {
			return time.Time{}, err
		}

		// Get timezone
		loc, err := time.LoadLocation(c.Timezone)
		if err != nil {
			loc = time.UTC
		}

		// Convert to local time for cron calculation
		localFrom := from.In(loc)
		return schedule.Next(localFrom), nil

	case "once":
		if c.StartTime != nil && from.Before(*c.StartTime) {
			return *c.StartTime, nil
		}
		// If we've passed the start time, there's no next execution
		return time.Time{}, errors.ConfigError("'once' schedule has already executed")

	default:
		return time.Time{}, errors.ConfigError(fmt.Sprintf("unknown schedule type: %s", c.ScheduleType))
	}
}

// GetName returns the trigger name (implements base.TriggerConfig)
func (c *Config) GetName() string {
	return c.Name
}

// GetID returns the trigger ID (implements base.TriggerConfig)
func (c *Config) GetID() string {
	return c.ID
}

// GetType returns the trigger type (implements triggers.TriggerConfig)
func (c *Config) GetType() string {
	return "schedule"
}
