package schedule

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"text/template"
	"time"

	"webhook-router/internal/common/base"
	"webhook-router/internal/common/errors"
	commonhttp "webhook-router/internal/common/http"
	"webhook-router/internal/common/logging"
	"webhook-router/internal/common/utils"
	"webhook-router/internal/oauth2"
	"webhook-router/internal/triggers"
)

// oauth2ManagerAdapter adapts oauth2.Manager to commonhttp.OAuth2ManagerInterface
type oauth2ManagerAdapter struct {
	manager *oauth2.Manager
}

func (a *oauth2ManagerAdapter) GetToken(ctx context.Context, serviceID string) (*commonhttp.OAuth2Token, error) {
	token, err := a.manager.GetToken(ctx, serviceID)
	if err != nil {
		return nil, err
	}

	return &commonhttp.OAuth2Token{
		AccessToken: token.AccessToken,
		TokenType:   token.TokenType,
		Expiry:      token.Expiry,
	}, nil
}

// Trigger implements the scheduled trigger using BaseTrigger
type Trigger struct {
	*base.BaseTrigger
	config        *Config
	nextExecution *time.Time
	runCount      int
	ticker        *time.Ticker
	mu            sync.RWMutex
	builder       *triggers.TriggerBuilder
	clientWrapper *commonhttp.HTTPClientWrapper

	// Distributed coordination
	distributedExecutor func(triggerID string, taskID string, handler func() error) error
}

func NewTrigger(config *Config) *Trigger {
	builder := triggers.NewTriggerBuilder("schedule", config)

	// Create HTTP client wrapper
	clientWrapper := commonhttp.NewHTTPClientWrapper(
		commonhttp.WithTimeout(30 * time.Second),
	).WithCircuitBreaker(fmt.Sprintf("schedule-trigger-%s", config.Name))

	trigger := &Trigger{
		config:        config,
		runCount:      0,
		builder:       builder,
		clientWrapper: clientWrapper,
		// Default to direct execution (no distributed coordination)
		distributedExecutor: func(triggerID string, taskID string, handler func() error) error {
			return handler()
		},
	}

	// Initialize BaseTrigger
	trigger.BaseTrigger = base.NewBaseTrigger("schedule", config, nil)

	return trigger
}

// SetOAuthManager sets the OAuth2 manager for authentication
func (t *Trigger) SetOAuthManager(manager *oauth2.Manager) {
	// Delegate to BaseTrigger
	t.BaseTrigger.SetOAuth2Manager(manager)

	// Also set on the HTTP client wrapper for automatic OAuth2 handling
	if t.clientWrapper != nil && manager != nil {
		// Create adapter to use with HTTPClientWrapper
		adapter := &oauth2ManagerAdapter{manager: manager}
		// Set a default service ID - it will be overridden per request if needed
		t.clientWrapper.SetOAuth2Manager(adapter, "")
	}
}

// SetDistributedExecutor sets the distributed execution function
func (t *Trigger) SetDistributedExecutor(executor func(triggerID string, taskID string, handler func() error) error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.distributedExecutor = executor
}

// NextExecution returns the next scheduled execution time
func (t *Trigger) NextExecution() *time.Time {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.nextExecution
}

// Start starts the scheduled trigger
func (t *Trigger) Start(ctx context.Context, handler triggers.TriggerHandler) error {
	// Check if trigger has expired
	if t.config.IsExpired() {
		return errors.ValidationError("trigger has expired")
	}

	// Check if max runs reached
	if t.config.MaxRuns > 0 && t.runCount >= t.config.MaxRuns {
		return errors.ValidationError(fmt.Sprintf("max runs (%d) already reached", t.config.MaxRuns))
	}

	// Update BaseTrigger with the adapted handler
	t.BaseTrigger = t.builder.BuildBaseTrigger(handler)

	// Use the BaseTrigger's Start method with our run function
	return t.BaseTrigger.Start(ctx, func(ctx context.Context) error {
		return t.run(ctx)
	})
}

func (t *Trigger) run(ctx context.Context) error {
	// For interval-based scheduling, use ticker
	if t.config.ScheduleType == "interval" {
		t.ticker = time.NewTicker(t.config.GetInterval())
		defer t.ticker.Stop()
	}

	// Execute immediately if configured
	if t.config.RunImmediately && t.config.ScheduleType != "once" {
		// Set next execution for display purposes
		now := time.Now()
		next := t.calculateNextExecution(now)
		t.mu.Lock()
		t.nextExecution = &next
		t.mu.Unlock()

		t.builder.Logger().Info("Schedule trigger started",
			logging.Field{"schedule_type", t.config.ScheduleType},
			logging.Field{"interval", t.config.GetInterval()},
			logging.Field{"next_execution", next.Format(time.RFC3339)},
		)

		t.execute(ctx)
	}

	// Main scheduling loop
	for {
		// Calculate next execution time
		now := time.Now()
		next := t.calculateNextExecution(now)

		// Check if "once" schedule has already been executed
		if t.config.ScheduleType == "once" && t.runCount > 0 {
			t.builder.Logger().Info("Once schedule completed")
			return nil
		}

		t.mu.Lock()
		t.nextExecution = &next
		t.mu.Unlock()

		// Wait until next execution time
		waitDuration := time.Until(next)
		if waitDuration <= 0 {
			// Should execute immediately
			waitDuration = 0
		}

		t.builder.Logger().Debug("Waiting for next execution",
			logging.Field{"wait_duration", waitDuration},
			logging.Field{"next_execution", next.Format(time.RFC3339)},
		)

		select {
		case <-ctx.Done():
			t.builder.Logger().Info("Schedule trigger stopped")
			return nil

		case <-time.After(waitDuration):
			// Check if trigger expired
			if t.config.IsExpired() {
				t.builder.Logger().Info("Schedule trigger expired")
				return nil
			}

			// Check max runs
			if t.config.MaxRuns > 0 && t.runCount >= t.config.MaxRuns {
				t.builder.Logger().Info("Schedule trigger reached max runs",
					logging.Field{"max_runs", t.config.MaxRuns},
				)
				return nil
			}

			// Execute
			t.execute(ctx)
		}
	}
}

func (t *Trigger) execute(ctx context.Context) {
	// Generate unique task ID for distributed coordination
	taskID := utils.GenerateEventID("schedule", t.config.ID)

	// Use distributed executor to ensure only one instance runs
	err := t.distributedExecutor(t.config.ID, taskID, func() error {
		t.mu.Lock()
		t.runCount++
		currentRun := t.runCount
		t.mu.Unlock()

		t.builder.Logger().Debug("Executing scheduled task",
			logging.Field{"run_count", currentRun},
		)

		// Fetch data if URL is configured
		var fetchedData interface{}
		if t.config.DataSource.URL != "" {
			data, err := t.fetchData(ctx)
			if err != nil {
				t.builder.Logger().Error("Failed to fetch data", err)
				if !t.config.ContinueOnError {
					return err
				}
			} else {
				fetchedData = data
			}
		}

		// Create trigger event with enhanced metadata
		event := &triggers.TriggerEvent{
			ID:          taskID,
			TriggerID:   t.config.ID,
			TriggerName: t.config.Name,
			Type:        "schedule",
			Timestamp:   time.Now(),
			Data: map[string]interface{}{
				"run_count":     currentRun,
				"fetched_data":  fetchedData,
				"schedule_type": t.config.ScheduleType,
			},
			Headers: make(map[string]string),
			Source: triggers.TriggerSource{
				Type:     "schedule",
				Name:     t.config.Name,
				Endpoint: t.config.ScheduleType,
				Metadata: t.buildSourceMetadata(),
			},
		}

		// Add custom data if configured
		if t.config.CustomData != nil {
			for k, v := range t.config.CustomData {
				event.Data[k] = v
			}
		}

		// Apply transformations
		if t.config.DataTransform.Enabled {
			t.applyTransformations(event)
		}

		// Handle the event
		return t.HandleEvent(event)
	})

	if err != nil {
		t.builder.Logger().Error("Failed to execute scheduled task", err,
			logging.Field{"task_id", taskID},
		)
	}
}

func (t *Trigger) fetchData(ctx context.Context) (interface{}, error) {
	config := t.config.DataSource

	// Prepare headers
	headers := make(map[string]string)
	for name, value := range config.Headers {
		headers[name] = value
	}

	// Use HTTPClientWrapper for the request
	opts := &commonhttp.RequestOptions{
		Method:    config.Method,
		URL:       config.URL,
		Headers:   headers,
		ParseJSON: true, // Let the wrapper handle JSON parsing
	}

	// Handle OAuth2 if configured with the wrapper
	if config.OAuth2ServiceID != "" {
		// If we have OAuth2 manager set on the wrapper, it will handle it automatically
		// Otherwise we need to get the token manually
		oauth2Manager := t.BaseTrigger.GetOAuth2Manager()
		if oauth2Manager != nil {
			// Create adapter if needed
			adapter := &oauth2ManagerAdapter{manager: oauth2Manager}
			t.clientWrapper.SetOAuth2Manager(adapter, config.OAuth2ServiceID)
		}
	}

	// Perform request using HTTPClientWrapper
	resp, err := t.clientWrapper.Request(ctx, opts)
	if err != nil {
		return nil, err // HTTPClientWrapper already wraps errors appropriately
	}

	// Return the parsed body (HTTPClientWrapper handles JSON parsing)
	return resp.Body, nil
}

func (t *Trigger) applyTransformations(event *triggers.TriggerEvent) {
	transform := t.config.DataTransform

	if transform.Template == "" {
		return
	}

	// Parse and execute template
	tmpl, err := template.New("transform").Parse(transform.Template)
	if err != nil {
		t.builder.Logger().Error("Failed to parse transformation template", err)
		return
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, event); err != nil {
		t.builder.Logger().Error("Failed to execute transformation template", err)
		return
	}

	// Try to parse result as JSON
	var transformedData interface{}
	if err := json.Unmarshal(buf.Bytes(), &transformedData); err == nil {
		event.Data["transformed"] = transformedData
	} else {
		event.Data["transformed"] = buf.String()
	}
}

func (t *Trigger) calculateNextExecution(from time.Time) time.Time {
	// Use config's GetNextExecutionTime method which handles all schedule types
	next, err := t.config.GetNextExecutionTime(from)
	if err != nil {
		// Fallback to interval-based scheduling
		return from.Add(t.config.GetInterval())
	}
	return next
}

// buildSourceMetadata builds enhanced metadata for schedule triggers
func (t *Trigger) buildSourceMetadata() map[string]interface{} {
	metadata := map[string]interface{}{
		"scheduled_time": time.Now().Format(time.RFC3339),
		"timezone":       t.config.Timezone,
		"schedule_type":  t.config.ScheduleType,
	}

	// Add schedule-specific metadata
	switch t.config.ScheduleType {
	case "cron":
		metadata["cron_spec"] = t.config.CronSpec
		// Calculate next run time
		if next, err := t.config.GetNextExecutionTime(time.Now()); err == nil && !next.IsZero() {
			metadata["next_run"] = next.Format(time.RFC3339)
		}
	case "interval":
		metadata["interval"] = t.config.Interval
		metadata["interval_seconds"] = int(t.config.GetInterval().Seconds())
	case "once":
		if t.config.StartTime != nil {
			metadata["start_time"] = t.config.StartTime.Format(time.RFC3339)
		}
	}

	// Add run information
	t.mu.RLock()
	metadata["run_count"] = t.runCount
	if t.config.MaxRuns > 0 {
		metadata["max_runs"] = t.config.MaxRuns
		metadata["runs_remaining"] = t.config.MaxRuns - t.runCount
	}
	t.mu.RUnlock()

	// Add validity period
	if t.config.StartTime != nil && !t.config.StartTime.IsZero() {
		metadata["start_time"] = t.config.StartTime.Format(time.RFC3339)
	}
	if t.config.EndTime != nil && !t.config.EndTime.IsZero() {
		metadata["end_time"] = t.config.EndTime.Format(time.RFC3339)
	}

	return metadata
}

// Health checks if the trigger is healthy
func (t *Trigger) Health() error {
	if !t.IsRunning() {
		return errors.InternalError("trigger is not running", nil)
	}

	// Check if we've missed too many executions
	if next := t.NextExecution(); next != nil {
		missedBy := time.Since(*next)
		if missedBy > t.config.GetInterval()*2 {
			return errors.InternalError(
				fmt.Sprintf("missed execution by %v", missedBy),
				nil,
			)
		}
	}

	return nil
}

// Config returns the trigger configuration
func (t *Trigger) Config() triggers.TriggerConfig {
	return t.config
}
