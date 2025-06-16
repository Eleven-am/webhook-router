package broker

import (
	"fmt"
	"strings"
	"time"
	"webhook-router/internal/triggers"
)

// Config represents the configuration for broker event triggers
type Config struct {
	triggers.BaseTriggerConfig
	
	// Broker-specific settings
	BrokerType     string            `json:"broker_type"`     // rabbitmq, kafka, redis, aws
	BrokerConfig   map[string]interface{} `json:"broker_config"`   // Broker connection configuration
	Topic          string            `json:"topic"`           // Topic/queue/stream to subscribe to
	ConsumerGroup  string            `json:"consumer_group"`  // Consumer group (for Kafka/Redis)
	RoutingKey     string            `json:"routing_key"`     // Routing key filter (for RabbitMQ)
	MessageFilter  MessageFilterConfig `json:"message_filter"`  // Message filtering
	Transformation TransformConfig   `json:"transformation"`  // Message transformation
	ErrorHandling  ErrorHandlingConfig `json:"error_handling"`  // Error handling
}

// MessageFilterConfig defines message filtering rules
type MessageFilterConfig struct {
	Enabled        bool              `json:"enabled"`         // Enable message filtering
	Headers        map[string]string `json:"headers"`         // Required headers
	ContentType    string            `json:"content_type"`    // Required content type
	BodyContains   string            `json:"body_contains"`   // Text that must be in body
	BodyNotContains string           `json:"body_not_contains"` // Text that must not be in body
	JSONCondition  JSONConditionConfig `json:"json_condition"`  // JSON-based conditions
	MinSize        int64             `json:"min_size"`        // Minimum message size
	MaxSize        int64             `json:"max_size"`        // Maximum message size
}

// JSONConditionConfig defines JSON-based filtering conditions
type JSONConditionConfig struct {
	Enabled    bool   `json:"enabled"`     // Enable JSON condition checking
	Path       string `json:"path"`        // JSONPath expression
	Operator   string `json:"operator"`    // eq, ne, gt, lt, gte, lte, contains, exists
	Value      string `json:"value"`       // Expected value
	ValueType  string `json:"value_type"`  // string, number, boolean
}

// TransformConfig defines message transformation settings
type TransformConfig struct {
	Enabled       bool              `json:"enabled"`        // Enable transformation
	HeaderMapping map[string]string `json:"header_mapping"` // Map incoming headers to new names
	BodyTemplate  string            `json:"body_template"`  // Template for body transformation
	AddHeaders    map[string]string `json:"add_headers"`    // Additional headers to add
	ExtractFields []ExtractFieldConfig `json:"extract_fields"` // Fields to extract from message
}

// ExtractFieldConfig defines field extraction from messages
type ExtractFieldConfig struct {
	Name       string `json:"name"`        // Name of the extracted field
	Source     string `json:"source"`      // header, body, metadata
	Path       string `json:"path"`        // JSONPath or header name
	DefaultValue string `json:"default_value"` // Default value if not found
	Required   bool   `json:"required"`    // Whether field is required
}

// ErrorHandlingConfig defines error handling behavior
type ErrorHandlingConfig struct {
	RetryEnabled    bool          `json:"retry_enabled"`     // Enable message retry
	MaxRetries      int           `json:"max_retries"`       // Maximum retry attempts
	RetryDelay      time.Duration `json:"retry_delay"`       // Delay between retries
	DeadLetterQueue string        `json:"dead_letter_queue"` // Queue for failed messages
	AlertOnError    bool          `json:"alert_on_error"`    // Send alert on persistent errors
	IgnoreErrors    bool          `json:"ignore_errors"`     // Continue processing on errors
}

func NewConfig(name string) *Config {
	return &Config{
		BaseTriggerConfig: triggers.BaseTriggerConfig{
			Name:     name,
			Type:     "broker",
			Active:   true,
			Settings: make(map[string]interface{}),
		},
		BrokerType:   "rabbitmq",
		BrokerConfig: make(map[string]interface{}),
		MessageFilter: MessageFilterConfig{
			Enabled:  false,
			Headers:  make(map[string]string),
			MinSize:  0,
			MaxSize:  10 * 1024 * 1024, // 10MB
		},
		Transformation: TransformConfig{
			Enabled:       false,
			HeaderMapping: make(map[string]string),
			AddHeaders:    make(map[string]string),
			ExtractFields: []ExtractFieldConfig{},
		},
		ErrorHandling: ErrorHandlingConfig{
			RetryEnabled: true,
			MaxRetries:   3,
			RetryDelay:   30 * time.Second,
			AlertOnError: true,
			IgnoreErrors: false,
		},
	}
}

func (c *Config) Validate() error {
	if c.Name == "" {
		return fmt.Errorf("trigger name is required")
	}
	
	// Validate broker type
	validBrokerTypes := map[string]bool{
		"rabbitmq": true, "kafka": true, "redis": true, "aws": true,
	}
	if !validBrokerTypes[c.BrokerType] {
		return fmt.Errorf("invalid broker type: %s", c.BrokerType)
	}
	
	// Validate topic/queue
	if c.Topic == "" {
		return fmt.Errorf("topic/queue is required")
	}
	
	// Validate broker-specific settings
	if err := c.validateBrokerConfig(); err != nil {
		return fmt.Errorf("broker config error: %w", err)
	}
	
	// Validate message filter
	if err := c.validateMessageFilter(); err != nil {
		return fmt.Errorf("message filter error: %w", err)
	}
	
	// Validate transformation
	if err := c.validateTransformation(); err != nil {
		return fmt.Errorf("transformation error: %w", err)
	}
	
	// Validate error handling
	if err := c.validateErrorHandling(); err != nil {
		return fmt.Errorf("error handling error: %w", err)
	}
	
	return nil
}

func (c *Config) validateBrokerConfig() error {
	switch c.BrokerType {
	case "rabbitmq":
		if c.BrokerConfig["url"] == nil {
			return fmt.Errorf("RabbitMQ URL is required")
		}
	case "kafka":
		if c.BrokerConfig["brokers"] == nil {
			return fmt.Errorf("Kafka brokers are required")
		}
		if c.ConsumerGroup == "" {
			c.ConsumerGroup = "webhook-router-trigger"
		}
	case "redis":
		if c.BrokerConfig["address"] == nil {
			return fmt.Errorf("Redis address is required")
		}
		if c.ConsumerGroup == "" {
			c.ConsumerGroup = "webhook-router-trigger"
		}
	case "aws":
		if c.BrokerConfig["region"] == nil {
			return fmt.Errorf("AWS region is required")
		}
		if c.BrokerConfig["queue_url"] == nil && c.BrokerConfig["topic_arn"] == nil {
			return fmt.Errorf("AWS queue URL or topic ARN is required")
		}
	}
	
	return nil
}

func (c *Config) validateMessageFilter() error {
	if !c.MessageFilter.Enabled {
		return nil
	}
	
	// Validate size limits
	if c.MessageFilter.MinSize < 0 {
		c.MessageFilter.MinSize = 0
	}
	if c.MessageFilter.MaxSize <= 0 {
		c.MessageFilter.MaxSize = 10 * 1024 * 1024 // 10MB default
	}
	if c.MessageFilter.MinSize > c.MessageFilter.MaxSize {
		return fmt.Errorf("min_size cannot be greater than max_size")
	}
	
	// Validate JSON condition
	if c.MessageFilter.JSONCondition.Enabled {
		if c.MessageFilter.JSONCondition.Path == "" {
			return fmt.Errorf("JSON condition path is required")
		}
		
		validOperators := map[string]bool{
			"eq": true, "ne": true, "gt": true, "lt": true,
			"gte": true, "lte": true, "contains": true, "exists": true,
		}
		if !validOperators[c.MessageFilter.JSONCondition.Operator] {
			return fmt.Errorf("invalid JSON condition operator: %s", c.MessageFilter.JSONCondition.Operator)
		}
		
		validValueTypes := map[string]bool{
			"string": true, "number": true, "boolean": true,
		}
		if c.MessageFilter.JSONCondition.ValueType != "" &&
		   !validValueTypes[c.MessageFilter.JSONCondition.ValueType] {
			return fmt.Errorf("invalid JSON condition value type: %s", c.MessageFilter.JSONCondition.ValueType)
		}
	}
	
	return nil
}

func (c *Config) validateTransformation() error {
	if !c.Transformation.Enabled {
		return nil
	}
	
	// Validate extract fields
	for i, field := range c.Transformation.ExtractFields {
		if field.Name == "" {
			return fmt.Errorf("extract field %d: name is required", i)
		}
		
		validSources := map[string]bool{
			"header": true, "body": true, "metadata": true,
		}
		if !validSources[field.Source] {
			return fmt.Errorf("extract field %d: invalid source '%s'", i, field.Source)
		}
		
		if field.Source != "body" && field.Path == "" {
			return fmt.Errorf("extract field %d: path is required for source '%s'", i, field.Source)
		}
	}
	
	return nil
}

func (c *Config) validateErrorHandling() error {
	if c.ErrorHandling.MaxRetries < 0 {
		c.ErrorHandling.MaxRetries = 0
	}
	if c.ErrorHandling.MaxRetries > 10 {
		return fmt.Errorf("maximum retry count is 10")
	}
	
	if c.ErrorHandling.RetryDelay <= 0 {
		c.ErrorHandling.RetryDelay = 30 * time.Second
	}
	
	return nil
}

func (c *Config) ShouldFilterMessage(headers map[string]string, body string, size int64) bool {
	if !c.MessageFilter.Enabled {
		return false // Don't filter if filtering is disabled
	}
	
	// Check required headers
	for key, expectedValue := range c.MessageFilter.Headers {
		if actualValue, exists := headers[key]; !exists || actualValue != expectedValue {
			return true // Filter out
		}
	}
	
	// Check content type
	if c.MessageFilter.ContentType != "" {
		contentType := headers["Content-Type"]
		if !strings.Contains(contentType, c.MessageFilter.ContentType) {
			return true // Filter out
		}
	}
	
	// Check size
	if size < c.MessageFilter.MinSize || size > c.MessageFilter.MaxSize {
		return true // Filter out
	}
	
	// Check body contains
	if c.MessageFilter.BodyContains != "" {
		if !strings.Contains(body, c.MessageFilter.BodyContains) {
			return true // Filter out
		}
	}
	
	// Check body not contains
	if c.MessageFilter.BodyNotContains != "" {
		if strings.Contains(body, c.MessageFilter.BodyNotContains) {
			return true // Filter out
		}
	}
	
	return false // Don't filter (allow trigger)
}

func (c *Config) GetBrokerType() string {
	return c.BrokerType
}

func (c *Config) GetTopic() string {
	return c.Topic
}

func (c *Config) GetConsumerGroup() string {
	if c.ConsumerGroup == "" {
		return "webhook-router-trigger"
	}
	return c.ConsumerGroup
}

func (c *Config) GetRoutingKey() string {
	return c.RoutingKey
}

func (c *Config) ShouldRetry(attempt int) bool {
	if !c.ErrorHandling.RetryEnabled {
		return false
	}
	return attempt <= c.ErrorHandling.MaxRetries
}

func (c *Config) GetRetryDelay() time.Duration {
	return c.ErrorHandling.RetryDelay
}