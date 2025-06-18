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
	ScheduleType    string            `json:"schedule_type"`     // cron, interval, once
	CronSpec        string            `json:"cron_spec"`         // Cron expression (for cron type)
	Interval        time.Duration     `json:"interval"`          // Interval duration (for interval type)
	StartTime       *time.Time        `json:"start_time"`        // When to start (for once type or delayed start)
	EndTime         *time.Time        `json:"end_time"`          // When to stop (optional)
	Timezone        string            `json:"timezone"`          // Timezone for cron expressions
	MaxRuns         int               `json:"max_runs"`          // Maximum number of runs (0 = unlimited)
	CustomData      map[string]interface{} `json:"custom_data"`  // Custom data to include in events
	DataSource      DataSourceConfig  `json:"data_source"`       // External data source
	DataTransform   TransformConfig   `json:"data_transform"`    // Data transformation
	ContinueOnError bool              `json:"continue_on_error"` // Continue if data fetch fails
}

// DataSourceConfig defines configuration for fetching external data
type DataSourceConfig struct {
	URL     string            `json:"url"`      // URL to fetch data from
	Method  string            `json:"method"`   // HTTP method
	Headers map[string]string `json:"headers"`  // HTTP headers
	Timeout time.Duration     `json:"timeout"`  // Request timeout
}

// TransformConfig defines data transformation settings
type TransformConfig struct {
	Enabled  bool   `json:"enabled"`   // Enable transformation
	Template string `json:"template"`  // Go template for transformation
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
		MaxRuns:         0, // Unlimited
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
	v := validation.NewValidator()
	
	// Basic validation
	v.RequireString(c.Name, "name")
	v.RequireOneOf(c.ScheduleType, []string{"cron", "interval", "once"}, "schedule_type")
	
	// Validate schedule-specific settings
	switch c.ScheduleType {
	case "cron":
		v.RequireString(c.CronSpec, "cron_spec")
		v.Validate(func() error {
			return c.validateCronSpec()
		})
		
	case "interval":
		if c.Interval <= 0 {
			v.Validate(func() error {
				return errors.ConfigError("interval must be positive")
			})
		}
		
	case "once":
		if c.StartTime == nil {
			v.Validate(func() error {
				return errors.ConfigError("start_time is required for 'once' schedule type")
			})
		}
	}
	
	// Validate timezone
	if c.Timezone != "" {
		v.Validate(func() error {
			_, err := time.LoadLocation(c.Timezone)
			if err != nil {
				return errors.ConfigError("invalid timezone")
			}
			return nil
		})
	}
	
	// Validate max runs
	v.RequireNonNegative(c.MaxRuns, "max_runs")
	
	// Validate time constraints
	if c.StartTime != nil && c.EndTime != nil {
		if c.StartTime.After(*c.EndTime) {
			v.Validate(func() error {
				return errors.ConfigError("start_time cannot be after end_time")
			})
		}
	}
	
	// Validate data source if configured
	if c.DataSource.URL != "" {
		v.RequireURL(c.DataSource.URL, "data_source.url")
		v.RequireOneOf(c.DataSource.Method, 
			[]string{"GET", "POST", "PUT", "DELETE", "PATCH"}, 
			"data_source.method",
		)
		
		if c.DataSource.Timeout <= 0 {
			c.DataSource.Timeout = 30 * time.Second
		}
	}
	
	return v.Error()
}

func (c *Config) validateCronSpec() error {
	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	_, err := parser.Parse(c.CronSpec)
	return err
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
		parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
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
func (c *Config) GetID() int {
	return c.ID
}

// GetType returns the trigger type (implements triggers.TriggerConfig)
func (c *Config) GetType() string {
	return "schedule"
}