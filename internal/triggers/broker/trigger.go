package broker

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"text/template"
	"time"
	"webhook-router/internal/brokers"
	"webhook-router/internal/triggers"
)

// Trigger implements the broker event trigger
type Trigger struct {
	config        *Config
	handler       triggers.TriggerHandler
	isRunning     bool
	mu            sync.RWMutex
	lastExecution *time.Time
	ctx           context.Context
	cancel        context.CancelFunc
	broker        brokers.Broker
	messageCount  int64
	errorCount    int64
}

func NewTrigger(config *Config, brokerRegistry *brokers.Registry) (*Trigger, error) {
	// Create broker instance from config
	brokerConfig, err := createBrokerConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create broker config: %w", err)
	}
	
	broker, err := brokerRegistry.Create(config.BrokerType, brokerConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create broker: %w", err)
	}
	
	return &Trigger{
		config:       config,
		isRunning:    false,
		broker:       broker,
		messageCount: 0,
		errorCount:   0,
	}, nil
}

func (t *Trigger) Name() string {
	return t.config.Name
}

func (t *Trigger) Type() string {
	return "broker"
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
	// Broker triggers don't have scheduled executions
	return nil
}

func (t *Trigger) Start(ctx context.Context, handler triggers.TriggerHandler) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	
	if t.isRunning {
		return triggers.ErrTriggerAlreadyRunning
	}
	
	t.handler = handler
	t.isRunning = true
	t.ctx, t.cancel = context.WithCancel(ctx)
	
	// Start subscribing to broker messages
	if err := t.startSubscription(); err != nil {
		t.isRunning = false
		return fmt.Errorf("failed to start subscription: %w", err)
	}
	
	log.Printf("Broker trigger '%s' started, listening to %s:%s", 
		t.config.Name, t.config.BrokerType, t.config.Topic)
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
	
	// Close broker connection
	if t.broker != nil {
		t.broker.Close()
	}
	
	log.Printf("Broker trigger '%s' stopped", t.config.Name)
	return nil
}

func (t *Trigger) Health() error {
	t.mu.RLock()
	defer t.mu.RUnlock()
	
	if !t.isRunning {
		return fmt.Errorf("trigger is not running")
	}
	
	// Check broker health
	if t.broker == nil {
		return fmt.Errorf("broker not initialized")
	}
	
	if err := t.broker.Health(); err != nil {
		return fmt.Errorf("broker health check failed: %w", err)
	}
	
	return nil
}

func (t *Trigger) startSubscription() error {
	// Create message handler for broker
	messageHandler := func(message *brokers.IncomingMessage) error {
		return t.handleBrokerMessage(message)
	}
	
	// Subscribe to the topic/queue
	topic := t.config.GetTopic()
	if t.config.GetRoutingKey() != "" {
		// For RabbitMQ with routing key
		topic = t.config.GetRoutingKey()
	}
	
	return t.broker.Subscribe(topic, messageHandler)
}

func (t *Trigger) handleBrokerMessage(message *brokers.IncomingMessage) error {
	// Update execution time
	t.mu.Lock()
	now := time.Now()
	t.lastExecution = &now
	t.messageCount++
	messageCount := t.messageCount
	t.mu.Unlock()
	
	// Check if message should be filtered
	if t.config.ShouldFilterMessage(message.Headers, string(message.Body), int64(len(message.Body))) {
		log.Printf("Broker trigger '%s': message filtered out", t.config.Name)
		return nil
	}
	
	// Process the message with retry logic
	return t.processMessageWithRetry(message, 1)
}

func (t *Trigger) processMessageWithRetry(message *brokers.IncomingMessage, attempt int) error {
	// Create trigger event from broker message
	event, err := t.createTriggerEvent(message)
	if err != nil {
		log.Printf("Error creating trigger event for '%s': %v", t.config.Name, err)
		return t.handleError(message, err, attempt)
	}
	
	// Handle the event
	if err := t.handler(event); err != nil {
		log.Printf("Error handling broker trigger event for '%s': %v", t.config.Name, err)
		return t.handleError(message, err, attempt)
	}
	
	log.Printf("Broker trigger '%s' processed message successfully", t.config.Name)
	return nil
}

func (t *Trigger) handleError(message *brokers.IncomingMessage, err error, attempt int) error {
	t.mu.Lock()
	t.errorCount++
	errorCount := t.errorCount
	t.mu.Unlock()
	
	log.Printf("Broker trigger '%s' error (attempt %d, total errors: %d): %v", 
		t.config.Name, attempt, errorCount, err)
	
	// Check if we should retry
	if t.config.ShouldRetry(attempt) {
		log.Printf("Retrying broker trigger '%s' in %v", t.config.Name, t.config.GetRetryDelay())
		
		// Retry after delay
		go func() {
			time.Sleep(t.config.GetRetryDelay())
			t.processMessageWithRetry(message, attempt+1)
		}()
		
		return nil // Don't return error to broker (we're handling retry)
	}
	
	// Send to dead letter queue if configured
	if t.config.ErrorHandling.DeadLetterQueue != "" {
		t.sendToDeadLetterQueue(message, err)
	}
	
	// Send alert if configured
	if t.config.ErrorHandling.AlertOnError {
		t.sendErrorAlert(message, err, attempt)
	}
	
	// Return error based on configuration
	if t.config.ErrorHandling.IgnoreErrors {
		return nil // Don't propagate error
	}
	
	return err
}

func (t *Trigger) createTriggerEvent(message *brokers.IncomingMessage) (*triggers.TriggerEvent, error) {
	// Create base event
	event := &triggers.TriggerEvent{
		ID:          t.generateEventID(),
		TriggerID:   t.config.ID,
		TriggerName: t.config.Name,
		Type:        "broker",
		Timestamp:   message.Timestamp,
		Data: map[string]interface{}{
			"message_id":     message.ID,
			"body":           string(message.Body),
			"broker_type":    t.config.BrokerType,
			"topic":          t.config.Topic,
			"routing_key":    t.config.RoutingKey,
			"consumer_group": t.config.ConsumerGroup,
		},
		Headers: make(map[string]string),
		Source: triggers.TriggerSource{
			Type: "broker",
			Name: t.config.Name,
			URL:  message.Source.URL,
		},
	}
	
	// Copy original headers
	for key, value := range message.Headers {
		event.Headers[key] = value
	}
	
	// Add broker metadata
	for key, value := range message.Metadata {
		event.Data[fmt.Sprintf("metadata_%s", key)] = value
	}
	
	// Apply transformations if enabled
	if t.config.Transformation.Enabled {
		if err := t.applyTransformations(event, message); err != nil {
			return nil, fmt.Errorf("transformation failed: %w", err)
		}
	}
	
	return event, nil
}

func (t *Trigger) applyTransformations(event *triggers.TriggerEvent, message *brokers.IncomingMessage) error {
	// Apply header mapping
	if len(t.config.Transformation.HeaderMapping) > 0 {
		newHeaders := make(map[string]string)
		for oldName, newName := range t.config.Transformation.HeaderMapping {
			if value, exists := event.Headers[oldName]; exists {
				newHeaders[newName] = value
				delete(event.Headers, oldName)
			}
		}
		for k, v := range newHeaders {
			event.Headers[k] = v
		}
	}
	
	// Add additional headers
	for key, value := range t.config.Transformation.AddHeaders {
		event.Headers[key] = value
	}
	
	// Apply body template transformation
	if t.config.Transformation.BodyTemplate != "" {
		transformedBody, err := t.processBodyTemplate(message)
		if err != nil {
			return fmt.Errorf("body template processing failed: %w", err)
		}
		event.Data["body"] = transformedBody
	}
	
	// Extract fields
	if len(t.config.Transformation.ExtractFields) > 0 {
		extracted := make(map[string]interface{})
		for _, field := range t.config.Transformation.ExtractFields {
			value, err := t.extractField(field, message)
			if err != nil && field.Required {
				return fmt.Errorf("failed to extract required field '%s': %w", field.Name, err)
			}
			if err == nil {
				extracted[field.Name] = value
			} else if field.DefaultValue != "" {
				extracted[field.Name] = field.DefaultValue
			}
		}
		event.Data["extracted_fields"] = extracted
	}
	
	return nil
}

func (t *Trigger) processBodyTemplate(message *brokers.IncomingMessage) (string, error) {
	// Create template variables
	vars := map[string]interface{}{
		"message_id":     message.ID,
		"body":           string(message.Body),
		"timestamp":      message.Timestamp,
		"source":         message.Source,
		"headers":        message.Headers,
		"metadata":       message.Metadata,
		"trigger_name":   t.config.Name,
		"trigger_id":     t.config.ID,
		"broker_type":    t.config.BrokerType,
		"topic":          t.config.Topic,
	}
	
	// Process template
	tmpl, err := template.New("body").Parse(t.config.Transformation.BodyTemplate)
	if err != nil {
		return "", fmt.Errorf("failed to parse template: %w", err)
	}
	
	var buf strings.Builder
	if err := tmpl.Execute(&buf, vars); err != nil {
		return "", fmt.Errorf("failed to execute template: %w", err)
	}
	
	return buf.String(), nil
}

func (t *Trigger) extractField(field ExtractFieldConfig, message *brokers.IncomingMessage) (interface{}, error) {
	switch field.Source {
	case "header":
		value, exists := message.Headers[field.Path]
		if !exists {
			return nil, fmt.Errorf("header '%s' not found", field.Path)
		}
		return value, nil
		
	case "metadata":
		value, exists := message.Metadata[field.Path]
		if !exists {
			return nil, fmt.Errorf("metadata '%s' not found", field.Path)
		}
		return value, nil
		
	case "body":
		// For body extraction, use JSONPath or simple field name
		if field.Path != "" {
			return t.extractFromJSON(string(message.Body), field.Path)
		}
		return string(message.Body), nil
		
	default:
		return nil, fmt.Errorf("unsupported source: %s", field.Source)
	}
}

func (t *Trigger) extractFromJSON(body, path string) (interface{}, error) {
	// Simple JSON extraction (would use a proper JSONPath library in production)
	var data interface{}
	if err := json.Unmarshal([]byte(body), &data); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w", err)
	}
	
	// Simple field extraction (path is field name)
	if m, ok := data.(map[string]interface{}); ok {
		if value, exists := m[path]; exists {
			return value, nil
		}
	}
	
	return nil, fmt.Errorf("field '%s' not found in JSON", path)
}

func (t *Trigger) sendToDeadLetterQueue(message *brokers.IncomingMessage, err error) {
	// Create dead letter message
	dlqMessage := &brokers.Message{
		Queue:      t.config.ErrorHandling.DeadLetterQueue,
		RoutingKey: "error",
		Headers: map[string]string{
			"original_trigger": t.config.Name,
			"error_message":    err.Error(),
			"error_time":       time.Now().Format(time.RFC3339),
			"original_topic":   t.config.Topic,
		},
		Body:      message.Body,
		Timestamp: time.Now(),
		MessageID: fmt.Sprintf("dlq-%s", message.ID),
	}
	
	// Copy original headers with prefix
	for key, value := range message.Headers {
		dlqMessage.Headers["original_"+key] = value
	}
	
	// Send to dead letter queue
	if err := t.broker.Publish(dlqMessage); err != nil {
		log.Printf("Failed to send message to dead letter queue: %v", err)
	} else {
		log.Printf("Message sent to dead letter queue: %s", t.config.ErrorHandling.DeadLetterQueue)
	}
}

func (t *Trigger) sendErrorAlert(message *brokers.IncomingMessage, err error, attempt int) {
	// Create alert (this would integrate with alerting system)
	alert := map[string]interface{}{
		"trigger_name":     t.config.Name,
		"trigger_id":       t.config.ID,
		"error_message":    err.Error(),
		"message_id":       message.ID,
		"attempt":          attempt,
		"timestamp":        time.Now(),
		"broker_type":      t.config.BrokerType,
		"topic":            t.config.Topic,
		"total_errors":     t.errorCount,
		"total_messages":   t.messageCount,
	}
	
	log.Printf("ALERT: Broker trigger error: %+v", alert)
	// In production, this would send to an alerting system
}

func (t *Trigger) generateEventID() string {
	return fmt.Sprintf("broker-%d-%d", t.config.ID, time.Now().UnixNano())
}

// Helper function to create broker config from trigger config
func createBrokerConfig(config *Config) (brokers.BrokerConfig, error) {
	switch config.BrokerType {
	case "rabbitmq":
		// Would import and use the appropriate broker config types
		return nil, fmt.Errorf("RabbitMQ broker config creation not implemented")
	case "kafka":
		return nil, fmt.Errorf("Kafka broker config creation not implemented")
	case "redis":
		return nil, fmt.Errorf("Redis broker config creation not implemented")
	case "aws":
		return nil, fmt.Errorf("AWS broker config creation not implemented")
	default:
		return nil, fmt.Errorf("unsupported broker type: %s", config.BrokerType)
	}
}

// Factory for creating broker triggers
type Factory struct {
	brokerRegistry *brokers.Registry
}

func NewFactory(brokerRegistry *brokers.Registry) *Factory {
	return &Factory{
		brokerRegistry: brokerRegistry,
	}
}

func (f *Factory) Create(config triggers.TriggerConfig) (triggers.Trigger, error) {
	brokerConfig, ok := config.(*Config)
	if !ok {
		return nil, fmt.Errorf("invalid config type for broker trigger")
	}
	
	if err := brokerConfig.Validate(); err != nil {
		return nil, err
	}
	
	return NewTrigger(brokerConfig, f.brokerRegistry)
}

func (f *Factory) GetType() string {
	return "broker"
}