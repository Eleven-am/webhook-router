package schedule

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"text/template"
	"time"
	
	"webhook-router/internal/common/base"
	"webhook-router/internal/common/errors"
	commonhttp "webhook-router/internal/common/http"
	"webhook-router/internal/common/logging"
	"webhook-router/internal/common/utils"
	"webhook-router/internal/triggers"
)

// Trigger implements the scheduled trigger using BaseTrigger
type Trigger struct {
	*base.BaseTrigger
	config        *Config
	nextExecution *time.Time
	runCount      int
	ticker        *time.Ticker
	mu            sync.RWMutex
	builder       *triggers.TriggerBuilder
	httpClient    *http.Client
	
	// Distributed coordination
	distributedExecutor func(triggerID int, taskID string, handler func() error) error
}

func NewTrigger(config *Config) *Trigger {
	builder := triggers.NewTriggerBuilder("schedule", config)
	
	trigger := &Trigger{
		config:     config,
		runCount:   0,
		builder:    builder,
		httpClient: commonhttp.NewDefaultHTTPClient(),
		// Default to direct execution (no distributed coordination)
		distributedExecutor: func(triggerID int, taskID string, handler func() error) error {
			return handler()
		},
	}
	
	// Initialize BaseTrigger
	trigger.BaseTrigger = base.NewBaseTrigger("schedule", config, nil)
	
	return trigger
}

// SetDistributedExecutor sets the distributed execution function
func (t *Trigger) SetDistributedExecutor(executor func(triggerID int, taskID string, handler func() error) error) {
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
	// Calculate initial delay
	now := time.Now()
	next := t.calculateNextExecution(now)
	
	t.mu.Lock()
	t.nextExecution = &next
	t.mu.Unlock()
	
	t.builder.Logger().Info("Schedule trigger started",
		logging.Field{"interval", t.config.GetInterval()},
		logging.Field{"next_execution", next.Format(time.RFC3339)},
	)
	
	// Create ticker with interval
	t.ticker = time.NewTicker(t.config.GetInterval())
	defer t.ticker.Stop()
	
	// Wait for initial delay
	select {
	case <-ctx.Done():
		return nil
	case <-time.After(time.Until(next)):
		// Execute first run
		t.execute(ctx)
	}
	
	// Continue with regular interval
	for {
		select {
		case <-ctx.Done():
			t.builder.Logger().Info("Schedule trigger stopped")
			return nil
			
		case tickTime := <-t.ticker.C:
			// Update next execution
			next := t.calculateNextExecution(tickTime)
			t.mu.Lock()
			t.nextExecution = &next
			t.mu.Unlock()
			
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
		
		// Create trigger event
		event := &triggers.TriggerEvent{
			ID:          taskID,
			TriggerID:   t.config.ID,
			TriggerName: t.config.Name,
			Type:        "schedule",
			Timestamp:   time.Now(),
			Data: map[string]interface{}{
				"run_count":    currentRun,
				"fetched_data": fetchedData,
			},
			Headers: make(map[string]string),
			Source: triggers.TriggerSource{
				Type: "schedule",
				Name: t.config.Name,
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
	
	req, err := http.NewRequestWithContext(ctx, config.Method, config.URL, nil)
	if err != nil {
		return nil, errors.InternalError("failed to create request", err)
	}
	
	// Add headers
	for name, value := range config.Headers {
		req.Header.Set(name, value)
	}
	
	// Perform request
	resp, err := t.httpClient.Do(req)
	if err != nil {
		return nil, errors.ConnectionError("failed to fetch data", err)
	}
	defer resp.Body.Close()
	
	// Check status code
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return nil, errors.InternalError(
			fmt.Sprintf("fetch failed with status %d: %s", resp.StatusCode, string(body)),
			nil,
		)
	}
	
	// Read response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.InternalError("failed to read response", err)
	}
	
	// Parse as JSON if content type indicates JSON
	contentType := resp.Header.Get("Content-Type")
	if strings.Contains(contentType, "application/json") {
		var data interface{}
		if err := json.Unmarshal(body, &data); err != nil {
			// Return raw string if JSON parsing fails
			return string(body), nil
		}
		return data, nil
	}
	
	return string(body), nil
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
	// For simple interval-based scheduling
	return from.Add(t.config.GetInterval())
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

