package caldav

import (
	"fmt"
	"time"
	"webhook-router/internal/common/validation"
	"webhook-router/internal/triggers"
)

// Config represents the configuration for CalDAV polling triggers
type Config struct {
	triggers.BaseTriggerConfig

	// CalDAV connection settings
	URL      string `json:"url"`      // CalDAV server URL (e.g., https://caldav.example.com/calendars/user/)
	Username string `json:"username"` // Username for authentication
	Password string `json:"password"` // Password (will be encrypted)

	// OAuth2 settings
	UseOAuth2       bool   `json:"use_oauth2"`        // Use OAuth2 instead of password
	OAuth2ServiceID string `json:"oauth2_service_id"` // Reference to OAuth2 service

	// Polling settings
	PollInterval   time.Duration `json:"poll_interval"`    // How often to check for changes
	TimeRangeStart string        `json:"time_range_start"` // Start of time range (e.g., "-1d", "now")
	TimeRangeEnd   string        `json:"time_range_end"`   // End of time range (e.g., "+7d", "+1m")

	// Processing settings
	IncludeDescription bool     `json:"include_description"` // Include event description
	IncludeAttendees   bool     `json:"include_attendees"`   // Include attendee list
	IncludeAlarms      bool     `json:"include_alarms"`      // Include alarm/reminder info
	IncludeRecurrence  bool     `json:"include_recurrence"`  // Include recurrence rules
	CalendarFilter     []string `json:"calendar_filter"`     // Only process events from these calendars
	EventTypes         []string `json:"event_types"`         // Filter by event types (e.g., "VEVENT", "VTODO")

	// State tracking
	LastSync      time.Time            `json:"last_sync"`      // Last successful sync time
	SyncTokens    map[string]string    `json:"sync_tokens"`    // Sync tokens per calendar
	ProcessedUIDs map[string]time.Time `json:"processed_uids"` // Track processed events
}

// DefaultConfig returns default CalDAV configuration
func DefaultConfig() *Config {
	return &Config{
		PollInterval:       5 * time.Minute,
		TimeRangeStart:     "-1d",
		TimeRangeEnd:       "+30d",
		IncludeDescription: true,
		IncludeAttendees:   true,
		IncludeAlarms:      true,
		IncludeRecurrence:  true,
		EventTypes:         []string{"VEVENT"},
		SyncTokens:         make(map[string]string),
		ProcessedUIDs:      make(map[string]time.Time),
	}
}

// Validate validates the CalDAV configuration
func (c *Config) Validate() error {
	v := validation.NewValidatorWithPrefix("CalDAV config")

	// Required fields
	v.RequireURL(c.URL, "url")
	v.RequireString(c.Username, "username")

	// Password is required only if not using OAuth2
	if c.UseOAuth2 {
		v.RequireString(c.OAuth2ServiceID, "oauth2_service_id")
	} else {
		v.RequireString(c.Password, "password")
	}

	// Set defaults
	if c.PollInterval <= 0 {
		c.PollInterval = 5 * time.Minute
	}

	if c.TimeRangeStart == "" {
		c.TimeRangeStart = "-1d"
	}

	if c.TimeRangeEnd == "" {
		c.TimeRangeEnd = "+30d"
	}

	if len(c.EventTypes) == 0 {
		c.EventTypes = []string{"VEVENT"}
	}

	if c.SyncTokens == nil {
		c.SyncTokens = make(map[string]string)
	}

	if c.ProcessedUIDs == nil {
		c.ProcessedUIDs = make(map[string]time.Time)
	}

	// Validate poll interval
	if c.PollInterval < time.Minute {
		v.Validate(func() error {
			return fmt.Errorf("poll interval must be at least 1 minute")
		})
	}

	// Validate event types
	allowedEventTypes := []string{"VEVENT", "VTODO", "VJOURNAL", "VFREEBUSY"}
	for _, eventType := range c.EventTypes {
		v.RequireOneOf(eventType, allowedEventTypes, "event_type")
	}

	return v.Error()
}

// GetType returns the trigger type
func (c *Config) GetType() string {
	return "caldav"
}

// GetName returns the trigger name (implements triggers.TriggerConfig)
func (c *Config) GetName() string {
	return c.Name
}

// GetID returns the trigger ID (implements triggers.TriggerConfig)
func (c *Config) GetID() string {
	return c.ID
}

// parseTimeRange parses time range strings like "-1d", "+7d", "now"
func parseTimeRange(rangeStr string, from time.Time) (time.Time, error) {
	if rangeStr == "now" {
		return from, nil
	}

	if len(rangeStr) < 2 {
		return time.Time{}, fmt.Errorf("invalid time range: %s", rangeStr)
	}

	sign := rangeStr[0]
	if sign != '+' && sign != '-' {
		return time.Time{}, fmt.Errorf("time range must start with + or -: %s", rangeStr)
	}

	// Parse duration
	value := rangeStr[1 : len(rangeStr)-1]
	unit := rangeStr[len(rangeStr)-1]

	var duration time.Duration
	switch unit {
	case 'm': // minutes
		if d, err := time.ParseDuration(value + "m"); err == nil {
			duration = d
		} else {
			return time.Time{}, fmt.Errorf("invalid duration: %s", rangeStr)
		}
	case 'h': // hours
		if d, err := time.ParseDuration(value + "h"); err == nil {
			duration = d
		} else {
			return time.Time{}, fmt.Errorf("invalid duration: %s", rangeStr)
		}
	case 'd': // days
		days := 0
		if _, err := fmt.Sscanf(value, "%d", &days); err != nil {
			return time.Time{}, fmt.Errorf("invalid duration: %s", rangeStr)
		}
		duration = time.Duration(days) * 24 * time.Hour
	case 'w': // weeks
		weeks := 0
		if _, err := fmt.Sscanf(value, "%d", &weeks); err != nil {
			return time.Time{}, fmt.Errorf("invalid duration: %s", rangeStr)
		}
		duration = time.Duration(weeks) * 7 * 24 * time.Hour
	case 'M': // months (approximation)
		months := 0
		if _, err := fmt.Sscanf(value, "%d", &months); err != nil {
			return time.Time{}, fmt.Errorf("invalid duration: %s", rangeStr)
		}
		duration = time.Duration(months) * 30 * 24 * time.Hour
	default:
		return time.Time{}, fmt.Errorf("unknown time unit: %c", unit)
	}

	if sign == '+' {
		return from.Add(duration), nil
	}
	return from.Add(-duration), nil
}

// GetTimeRange returns the start and end times for the configured range
func (c *Config) GetTimeRange() (start, end time.Time, err error) {
	now := time.Now()

	start, err = parseTimeRange(c.TimeRangeStart, now)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("invalid start time range: %w", err)
	}

	end, err = parseTimeRange(c.TimeRangeEnd, now)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("invalid end time range: %w", err)
	}

	if end.Before(start) {
		return time.Time{}, time.Time{}, fmt.Errorf("end time must be after start time")
	}

	return start, end, nil
}
