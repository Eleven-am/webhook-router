package schedule

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"text/template"
	"time"
	"webhook-router/internal/triggers"
)

// Trigger implements the scheduled trigger
type Trigger struct {
	config        *Config
	handler       triggers.TriggerHandler
	isRunning     bool
	mu            sync.RWMutex
	lastExecution *time.Time
	nextExecution *time.Time
	runCount      int
	ctx           context.Context
	cancel        context.CancelFunc
	ticker        *time.Ticker
}

func NewTrigger(config *Config) *Trigger {
	return &Trigger{
		config:    config,
		isRunning: false,
		runCount:  0,
	}
}

func (t *Trigger) Name() string {
	return t.config.Name
}

func (t *Trigger) Type() string {
	return "schedule"
}

func (t *Trigger) ID() int {
	return t.config.ID
}

func (t *Trigger) Config() triggers.TriggerConfig {
	return t.config
}

func (t *Trigger) IsRunning() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.isRunning
}

func (t *Trigger) LastExecution() *time.Time {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.lastExecution
}

func (t *Trigger) NextExecution() *time.Time {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.nextExecution
}

func (t *Trigger) Start(ctx context.Context, handler triggers.TriggerHandler) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	
	if t.isRunning {
		return triggers.ErrTriggerAlreadyRunning
	}
	
	// Check if trigger has expired
	if t.config.IsExpired() {
		return fmt.Errorf("trigger has expired")
	}
	
	// Check if max runs reached
	if t.config.MaxRuns > 0 && t.runCount >= t.config.MaxRuns {
		return fmt.Errorf("maximum runs (%d) reached", t.config.MaxRuns)
	}
	
	t.handler = handler
	t.isRunning = true
	t.ctx, t.cancel = context.WithCancel(ctx)
	
	// Calculate next execution
	next, err := t.config.GetNextExecution(time.Now())
	if err != nil {
		t.isRunning = false
		return fmt.Errorf("failed to calculate next execution: %w", err)
	}
	
	if next == nil {
		t.isRunning = false
		return fmt.Errorf("no more executions scheduled")
	}
	
	t.nextExecution = next
	
	// Start the scheduler
	go t.run()
	
	log.Printf("Schedule trigger '%s' started, next execution: %s", t.config.Name, next.Format(time.RFC3339))
	return nil
}

func (t *Trigger) Stop() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	
	if !t.isRunning {
		return triggers.ErrTriggerNotRunning
	}
	
	t.isRunning = false
	if t.cancel != nil {
		t.cancel()
	}
	if t.ticker != nil {
		t.ticker.Stop()
	}
	
	log.Printf("Schedule trigger '%s' stopped", t.config.Name)
	return nil
}

func (t *Trigger) Health() error {
	t.mu.RLock()
	defer t.mu.RUnlock()
	
	if !t.isRunning {
		return fmt.Errorf("trigger is not running")
	}
	
	if t.config.IsExpired() {
		return fmt.Errorf("trigger has expired")
	}
	
	if t.config.MaxRuns > 0 && t.runCount >= t.config.MaxRuns {
		return fmt.Errorf("maximum runs reached")
	}
	
	return nil
}

func (t *Trigger) run() {
	defer func() {
		t.mu.Lock()
		t.isRunning = false
		t.mu.Unlock()
	}()
	
	for {
		select {
		case <-t.ctx.Done():
			return
		default:
			// Calculate time until next execution
			t.mu.RLock()
			nextExec := t.nextExecution
			t.mu.RUnlock()
			
			if nextExec == nil {
				return // No more executions
			}
			
			now := time.Now()
			if nextExec.After(now) {
				// Wait until next execution time
				waitDuration := nextExec.Sub(now)
				timer := time.NewTimer(waitDuration)
				
				select {
				case <-timer.C:
					t.execute()
				case <-t.ctx.Done():
					timer.Stop()
					return
				}
			} else {
				// Execution time has passed, execute immediately
				t.execute()
			}
			
			// Check if we should continue
			t.mu.Lock()
			if t.config.IsExpired() || (t.config.MaxRuns > 0 && t.runCount >= t.config.MaxRuns) {
				t.isRunning = false
				t.mu.Unlock()
				return
			}
			
			// Calculate next execution for recurring schedules
			if t.config.ScheduleType != "once" {
				next, err := t.config.GetNextExecution(time.Now())
				if err != nil {
					log.Printf("Error calculating next execution for trigger '%s': %v", t.config.Name, err)
					t.isRunning = false
					t.mu.Unlock()
					return
				}
				t.nextExecution = next
				if next == nil {
					t.isRunning = false
					t.mu.Unlock()
					return
				}
			} else {
				// For "once" schedules, stop after execution
				t.nextExecution = nil
				t.isRunning = false
				t.mu.Unlock()
				return
			}
			t.mu.Unlock()
		}
	}
}

func (t *Trigger) execute() {
	t.executeWithRetry(1)
}

func (t *Trigger) executeWithRetry(attempt int) {
	// Generate payload
	payload, err := t.generatePayload()
	if err != nil {
		log.Printf("Error generating payload for trigger '%s': %v", t.config.Name, err)
		if t.config.ShouldRetry(attempt) {
			t.scheduleRetry(attempt)
		}
		return
	}
	
	// Create trigger event
	event := &triggers.TriggerEvent{
		ID:          t.generateEventID(),
		TriggerID:   t.config.ID,
		TriggerName: t.config.Name,
		Type:        "schedule",
		Timestamp:   time.Now(),
		Data:        payload,
		Headers:     make(map[string]string),
		Source: triggers.TriggerSource{
			Type: "schedule",
			Name: t.config.Name,
		},
	}
	
	// Add schedule-specific metadata
	event.Headers["schedule_type"] = t.config.ScheduleType
	event.Headers["run_count"] = fmt.Sprintf("%d", t.runCount+1)
	if t.config.ScheduleType == "cron" {
		event.Headers["cron_spec"] = t.config.CronSpec
	}
	
	// Update execution tracking
	t.mu.Lock()
	now := time.Now()
	t.lastExecution = &now
	t.runCount++
	runCount := t.runCount
	t.mu.Unlock()
	
	// Execute the trigger handler
	if err := t.handler(event); err != nil {
		log.Printf("Error executing schedule trigger '%s' (run %d): %v", t.config.Name, runCount, err)
		if t.config.ShouldRetry(attempt) {
			t.scheduleRetry(attempt)
		}
		return
	}
	
	log.Printf("Schedule trigger '%s' executed successfully (run %d)", t.config.Name, runCount)
}

func (t *Trigger) scheduleRetry(attempt int) {
	delay := t.config.GetRetryDelay(attempt)
	log.Printf("Scheduling retry %d for trigger '%s' in %v", attempt, t.config.Name, delay)
	
	go func() {
		timer := time.NewTimer(delay)
		defer timer.Stop()
		
		select {
		case <-timer.C:
			t.executeWithRetry(attempt + 1)
		case <-t.ctx.Done():
			return
		}
	}()
}

func (t *Trigger) generatePayload() (map[string]interface{}, error) {
	switch t.config.Payload.Type {
	case "static":
		// Return static data
		result := make(map[string]interface{})
		for k, v := range t.config.Payload.Data {
			result[k] = v
		}
		return result, nil
		
	case "template":
		// Process template
		return t.processTemplate()
		
	case "http_call":
		// Make HTTP call to get data
		return t.makeHTTPCall()
		
	default:
		return nil, fmt.Errorf("unsupported payload type: %s", t.config.Payload.Type)
	}
}

func (t *Trigger) processTemplate() (map[string]interface{}, error) {
	// Create template variables
	vars := map[string]interface{}{
		"trigger_name":    t.config.Name,
		"trigger_id":      t.config.ID,
		"timestamp":       time.Now(),
		"run_count":       t.runCount + 1,
		"schedule_type":   t.config.ScheduleType,
	}
	
	// Add custom variables
	for k, v := range t.config.Payload.Data {
		vars[k] = v
	}
	
	// Process template
	tmpl, err := template.New("payload").Parse(t.config.Payload.Template)
	if err != nil {
		return nil, fmt.Errorf("failed to parse template: %w", err)
	}
	
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, vars); err != nil {
		return nil, fmt.Errorf("failed to execute template: %w", err)
	}
	
	// Try to parse as JSON
	var result map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		// If not JSON, return as string
		return map[string]interface{}{
			"body": buf.String(),
		}, nil
	}
	
	return result, nil
}

func (t *Trigger) makeHTTPCall() (map[string]interface{}, error) {
	client := &http.Client{
		Timeout: t.config.Payload.HTTPCall.Timeout,
	}
	
	// Prepare request
	var body io.Reader
	if t.config.Payload.HTTPCall.Body != "" {
		body = strings.NewReader(t.config.Payload.HTTPCall.Body)
	}
	
	req, err := http.NewRequest(t.config.Payload.HTTPCall.Method, t.config.Payload.HTTPCall.URL, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}
	
	// Add headers
	for key, value := range t.config.Payload.HTTPCall.Headers {
		req.Header.Set(key, value)
	}
	
	// Make request
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP call failed: %w", err)
	}
	defer resp.Body.Close()
	
	// Read response
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}
	
	// Check status code
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("HTTP call returned status %d: %s", resp.StatusCode, string(respBody))
	}
	
	// Try to parse as JSON
	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		// If not JSON, return as string with metadata
		return map[string]interface{}{
			"body":        string(respBody),
			"status_code": resp.StatusCode,
			"headers":     resp.Header,
		}, nil
	}
	
	// Add metadata to JSON response
	result["_metadata"] = map[string]interface{}{
		"status_code": resp.StatusCode,
		"headers":     resp.Header,
	}
	
	return result, nil
}

func (t *Trigger) generateEventID() string {
	return fmt.Sprintf("schedule-%d-%d", t.config.ID, time.Now().UnixNano())
}

// Factory for creating scheduled triggers
type Factory struct{}

func (f *Factory) Create(config triggers.TriggerConfig) (triggers.Trigger, error) {
	scheduleConfig, ok := config.(*Config)
	if !ok {
		return nil, fmt.Errorf("invalid config type for schedule trigger")
	}
	
	if err := scheduleConfig.Validate(); err != nil {
		return nil, err
	}
	
	return NewTrigger(scheduleConfig), nil
}

func (f *Factory) GetType() string {
	return "schedule"
}