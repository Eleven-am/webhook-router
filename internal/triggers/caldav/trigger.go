package caldav

import (
	"context"
	"fmt"
	"sync"
	"time"
	
	"webhook-router/internal/common/base"
	"webhook-router/internal/common/errors"
	"webhook-router/internal/common/logging"
	"webhook-router/internal/common/utils"
	"webhook-router/internal/models"
	"webhook-router/internal/triggers"
)

// Trigger implements the CalDAV polling trigger using BaseTrigger
type Trigger struct {
	*base.BaseTrigger
	config      *Config
	client      *SimpleClient
	eventCount  int64
	errorCount  int64
	mu          sync.RWMutex
	builder     *triggers.TriggerBuilder
}

func NewTrigger(config *Config) (*Trigger, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.ConfigError(fmt.Sprintf("invalid CalDAV config: %v", err))
	}
	
	builder := triggers.NewTriggerBuilder("caldav", config)
	
	// Create CalDAV client
	client := NewSimpleClient(config.URL, config.Username, config.Password)
	
	trigger := &Trigger{
		config:      config,
		client:      client,
		eventCount:  0,
		errorCount:  0,
		builder:     builder,
	}
	
	// Initialize BaseTrigger
	trigger.BaseTrigger = base.NewBaseTrigger("caldav", config, nil)
	
	return trigger, nil
}

// Start starts the CalDAV polling trigger
func (t *Trigger) Start(ctx context.Context, handler triggers.TriggerHandler) error {
	// Update BaseTrigger with the adapted handler
	t.BaseTrigger = t.builder.BuildBaseTrigger(handler)
	
	// Use the BaseTrigger's Start method with our run function
	return t.BaseTrigger.Start(ctx, func(ctx context.Context) error {
		return t.run(ctx, handler)
	})
}

// Stop stops the CalDAV polling trigger
func (t *Trigger) Stop() error {
	// First stop the base trigger
	if err := t.BaseTrigger.Stop(); err != nil {
		return err
	}
	
	t.builder.Logger().Info("CalDAV trigger stopped",
		logging.Field{"url", t.config.URL},
	)
	return nil
}

func (t *Trigger) run(ctx context.Context, handler triggers.TriggerHandler) error {
	t.builder.Logger().Info("CalDAV trigger started",
		logging.Field{"url", t.config.URL},
		logging.Field{"poll_interval", t.config.PollInterval},
	)
	
	// Start polling loop
	return t.pollLoop(ctx, handler)
}

// pollLoop runs the polling loop
func (t *Trigger) pollLoop(ctx context.Context, handler triggers.TriggerHandler) error {
	ticker := time.NewTicker(t.config.PollInterval)
	defer ticker.Stop()
	
	// Initial poll
	t.poll(ctx, handler)
	
	for {
		select {
		case <-ctx.Done():
			t.builder.Logger().Info("CalDAV trigger stopped")
			return nil
		case <-ticker.C:
			t.poll(ctx, handler)
		}
	}
}

// poll performs a single poll operation
func (t *Trigger) poll(ctx context.Context, handler triggers.TriggerHandler) {
	if err := t.checkCalendars(ctx, handler); err != nil {
		t.builder.Logger().Error("CalDAV check error", err)
		t.errorCount++
		return
	}
}

// checkCalendars checks for calendar events
func (t *Trigger) checkCalendars(ctx context.Context, handler triggers.TriggerHandler) error {
	// Try sync-collection first for efficient change detection
	syncToken := t.config.SyncTokens[t.config.URL]
	t.client.SetSyncToken(syncToken)
	
	events, newToken, err := t.client.SyncCollection(ctx)
	if err == nil && newToken != "" {
		// Sync successful, update token
		if t.config.SyncTokens == nil {
			t.config.SyncTokens = make(map[string]string)
		}
		t.config.SyncTokens[t.config.URL] = newToken
	} else if err != nil || newToken == "" {
		// Fallback to regular query
		start, end, err := t.config.GetTimeRange()
		if err != nil {
			return fmt.Errorf("failed to get time range: %w", err)
		}
		
		events, err = t.client.QueryEvents(ctx, start, end)
		if err != nil {
			return fmt.Errorf("failed to query events: %w", err)
		}
	}
	
	// Process events
	for _, event := range events {
		// Check if we've already processed this event
		if lastProcessed, exists := t.config.ProcessedUIDs[event.UID]; exists {
			// Check if event was updated
			if event.Updated.Before(lastProcessed) || event.Updated.Equal(lastProcessed) {
				continue
			}
		}
		
		if err := t.processEvent(event, handler); err != nil {
			t.builder.Logger().Error("Failed to process event", err,
				logging.Field{"uid", event.UID},
			)
		} else {
			// Mark as processed
			t.config.ProcessedUIDs[event.UID] = time.Now()
		}
	}
	
	// Update last sync time
	t.config.LastSync = time.Now()
	
	return nil
}


// processEvent processes a single calendar event
func (t *Trigger) processEvent(calEvent *models.CalendarEvent, handler triggers.TriggerHandler) error {
	// Convert to generic map for webhook data
	eventData := map[string]interface{}{
		"id":          calEvent.ID,
		"uid":         calEvent.UID,
		"title":       calEvent.Title,
		"description": calEvent.Description,
		"location":    calEvent.Location,
		"start":       calEvent.Start,
		"end":         calEvent.End,
		"all_day":     calEvent.AllDay,
		"status":      calEvent.Status,
		"created":     calEvent.Created,
		"updated":     calEvent.Updated,
		"metadata":    calEvent.Metadata,
	}
	
	// Add optional fields
	if calEvent.Organizer != nil {
		eventData["organizer"] = calEvent.Organizer
	}
	if len(calEvent.Attendees) > 0 {
		eventData["attendees"] = calEvent.Attendees
	}
	if calEvent.Recurrence != "" {
		eventData["recurrence"] = calEvent.Recurrence
	}
	if len(calEvent.Reminders) > 0 {
		eventData["reminders"] = calEvent.Reminders
	}
	if len(calEvent.Categories) > 0 {
		eventData["categories"] = calEvent.Categories
	}
	
	// Create trigger event
	event := &triggers.TriggerEvent{
		ID:          utils.GenerateEventID("caldav", t.config.ID),
		TriggerID:   t.config.ID,
		TriggerName: t.config.Name,
		Type:        "calendar",
		Timestamp:   time.Now(),
		Data:        eventData,
		Headers: map[string]string{
			"X-Trigger-Type": "caldav",
			"X-Event-UID":    calEvent.UID,
		},
		Source: triggers.TriggerSource{
			Type: "caldav",
			Name: t.config.Name,
			URL:  t.config.URL,
		},
	}
	
	// Call handler
	if err := handler(event); err != nil {
		return errors.InternalError("handler error", err)
	}
	
	t.eventCount++
	
	return nil
}

// NextExecution returns the next scheduled execution time
func (t *Trigger) NextExecution() *time.Time {
	if !t.IsRunning() || t.LastExecution() == nil {
		return nil
	}
	
	next := t.LastExecution().Add(t.config.PollInterval)
	return &next
}

// Health returns the trigger health status
func (t *Trigger) Health() error {
	if !t.IsRunning() {
		return errors.InternalError("trigger is not running", nil)
	}
	
	return nil
}

// Config returns the trigger configuration
func (t *Trigger) Config() triggers.TriggerConfig {
	return t.config
}


