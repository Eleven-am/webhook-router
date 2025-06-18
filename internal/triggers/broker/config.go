package broker

import (
	"fmt"
	"time"
	"webhook-router/internal/common/errors"
	"webhook-router/internal/common/validation"
	"webhook-router/internal/triggers"
)

// Config represents the configuration for broker event triggers
type Config struct {
	triggers.BaseTriggerConfig
	
	// Broker-specific settings
	BrokerType     string                 `json:"broker_type"`     // rabbitmq, kafka, redis, aws
	BrokerConfig   map[string]interface{} `json:"broker_config"`   // Broker connection configuration
	Topic          string                 `json:"topic"`           // Topic/queue/stream to subscribe to
	ConsumerGroup  string                 `json:"consumer_group"`  // Consumer group (for Kafka/Redis)
	RoutingKey     string                 `json:"routing_key"`     // Routing key filter (for RabbitMQ)
	MessageFilter  MessageFilterConfig    `json:"message_filter"`  // Message filtering
	Transformation TransformConfig        `json:"transformation"`  // Message transformation
	ErrorHandling  ErrorHandlingConfig    `json:"error_handling"`  // Error handling
}

// MessageFilterConfig defines message filtering rules
type MessageFilterConfig struct {
	Enabled         bool                `json:"enabled"`           // Enable message filtering
	Headers         map[string]string   `json:"headers"`           // Required headers
	ContentType     string              `json:"content_type"`      // Required content type
	BodyContains    string              `json:"body_contains"`     // Text that must be in body
	BodyNotContains string              `json:"body_not_contains"` // Text that must not be in body
	JSONCondition   JSONConditionConfig `json:"json_condition"`    // JSON-based conditions
	MinSize         int64               `json:"min_size"`          // Minimum message size
	MaxSize         int64               `json:"max_size"`          // Maximum message size
}

// JSONConditionConfig defines JSON-based filtering conditions
type JSONConditionConfig struct {
	Enabled   bool   `json:"enabled"`    // Enable JSON condition checking
	Path      string `json:"path"`       // JSONPath expression
	Operator  string `json:"operator"`   // eq, ne, gt, lt, gte, lte, contains, exists
	Value     string `json:"value"`      // Expected value
	ValueType string `json:"value_type"` // string, number, boolean
}

// TransformConfig defines message transformation settings
type TransformConfig struct {
	Enabled       bool                 `json:"enabled"`        // Enable transformation
	HeaderMapping map[string]string    `json:"header_mapping"` // Map incoming headers to new names
	BodyTemplate  string               `json:"body_template"`  // Template for body transformation
	AddHeaders    map[string]string    `json:"add_headers"`    // Additional headers to add
	ExtractFields []ExtractFieldConfig `json:"extract_fields"` // Fields to extract from message
}

// ExtractFieldConfig defines field extraction from messages
type ExtractFieldConfig struct {
	Name         string `json:"name"`          // Name of the extracted field
	Source       string `json:"source"`        // header, body, metadata
	Path         string `json:"path"`          // JSONPath or header name
	DefaultValue string `json:"default_value"` // Default value if not found
	Required     bool   `json:"required"`      // Whether field is required
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
			Enabled: false,
			Headers: make(map[string]string),
			MinSize: 0,
			MaxSize: 10 * 1024 * 1024, // 10MB
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
	v := validation.NewValidator()
	
	// Basic validation
	v.RequireString(c.Name, "name")
	v.RequireOneOf(c.BrokerType, []string{"rabbitmq", "kafka", "redis", "aws", "gcp"}, "broker_type")
	v.RequireString(c.Topic, "topic")
	
	// Validate broker-specific configuration
	v.Validate(func() error {
		return c.validateBrokerConfig()
	})
	
	// Validate consumer group for brokers that require it
	if c.BrokerType == "kafka" || c.BrokerType == "redis" {
		v.RequireString(c.ConsumerGroup, "consumer_group")
	}
	
	// Validate message filter
	if c.MessageFilter.Enabled {
		if c.MessageFilter.MaxSize <= 0 {
			c.MessageFilter.MaxSize = 10 * 1024 * 1024 // 10MB default
		}
		
		v.RequireNonNegative(int(c.MessageFilter.MinSize), "message_filter.min_size")
		
		if c.MessageFilter.MinSize > c.MessageFilter.MaxSize {
			v.Validate(func() error {
				return errors.ValidationError("min_size cannot be greater than max_size")
			})
		}
		
		// Validate JSON condition if enabled
		if c.MessageFilter.JSONCondition.Enabled {
			v.RequireString(c.MessageFilter.JSONCondition.Path, "json_condition.path")
			v.RequireOneOf(c.MessageFilter.JSONCondition.Operator,
				[]string{"eq", "ne", "gt", "lt", "gte", "lte", "contains", "exists"},
				"json_condition.operator",
			)
			v.RequireOneOf(c.MessageFilter.JSONCondition.ValueType,
				[]string{"string", "number", "boolean"},
				"json_condition.value_type",
			)
		}
	}
	
	// Validate transformation
	if c.Transformation.Enabled {
		// Validate extract fields
		for i, field := range c.Transformation.ExtractFields {
			fieldValidator := validation.NewValidatorWithPrefix(fmt.Sprintf("extract_fields[%d]", i))
			fieldValidator.RequireString(field.Name, "name")
			fieldValidator.RequireOneOf(field.Source, []string{"header", "body", "metadata"}, "source")
			if field.Source != "body" {
				fieldValidator.RequireString(field.Path, "path")
			}
			v.Merge(fieldValidator)
		}
	}
	
	// Validate error handling
	v.RequireNonNegative(c.ErrorHandling.MaxRetries, "error_handling.max_retries")
	if c.ErrorHandling.RetryDelay <= 0 {
		c.ErrorHandling.RetryDelay = 30 * time.Second
	}
	
	return v.Error()
}

func (c *Config) validateBrokerConfig() error {
	if c.BrokerConfig == nil {
		return errors.ConfigError("broker_config is required")
	}
	
	switch c.BrokerType {
	case "rabbitmq":
		return c.validateRabbitMQConfig()
	case "kafka":
		return c.validateKafkaConfig()
	case "redis":
		return c.validateRedisConfig()
	case "aws":
		return c.validateAWSConfig()
	case "gcp":
		return c.validateGCPConfig()
	default:
		return errors.ConfigError(fmt.Sprintf("unsupported broker type: %s", c.BrokerType))
	}
}

func (c *Config) validateRabbitMQConfig() error {
	v := validation.NewValidatorWithPrefix("broker_config")
	
	if url, ok := c.BrokerConfig["url"].(string); ok {
		v.RequireString(url, "url")
	} else {
		v.RequireString("", "url")
	}
	
	// Exchange is optional but if provided, must be valid
	if exchange, ok := c.BrokerConfig["exchange"].(string); ok && exchange != "" {
		if exchangeType, ok := c.BrokerConfig["exchange_type"].(string); ok {
			v.RequireOneOf(exchangeType, []string{"direct", "topic", "fanout", "headers"}, "exchange_type")
		} else {
			// Default to topic if exchange is specified but type is not
			c.BrokerConfig["exchange_type"] = "topic"
		}
	}
	
	return v.Error()
}

func (c *Config) validateKafkaConfig() error {
	v := validation.NewValidatorWithPrefix("broker_config")
	
	// Check brokers
	brokers, ok := c.BrokerConfig["brokers"].([]interface{})
	if !ok || len(brokers) == 0 {
		v.Validate(func() error {
			return errors.ConfigError("brokers array is required")
		})
	} else {
		for i, broker := range brokers {
			if _, ok := broker.(string); !ok {
				v.Validate(func() error {
					return errors.ConfigError(fmt.Sprintf("broker[%d] must be a string", i))
				})
			}
		}
	}
	
	return v.Error()
}

func (c *Config) validateRedisConfig() error {
	v := validation.NewValidatorWithPrefix("broker_config")
	
	if address, ok := c.BrokerConfig["address"].(string); ok {
		v.RequireString(address, "address")
	} else {
		v.RequireString("", "address")
	}
	
	// DB must be a valid number if provided
	if db, ok := c.BrokerConfig["db"].(float64); ok {
		v.RequireNonNegative(int(db), "db")
	}
	
	return v.Error()
}

func (c *Config) validateAWSConfig() error {
	v := validation.NewValidatorWithPrefix("broker_config")
	
	if region, ok := c.BrokerConfig["region"].(string); ok {
		v.RequireString(region, "region")
	} else {
		v.RequireString("", "region")
	}
	
	// Must have either queue_url (SQS) or topic_arn (SNS)
	queueURL, hasQueue := c.BrokerConfig["queue_url"].(string)
	topicARN, hasTopic := c.BrokerConfig["topic_arn"].(string)
	
	if !hasQueue && !hasTopic {
		v.Validate(func() error {
			return errors.ConfigError("either queue_url or topic_arn is required")
		})
	}
	
	if hasQueue {
		v.RequireString(queueURL, "queue_url")
	}
	if hasTopic {
		v.RequireString(topicARN, "topic_arn")
	}
	
	return v.Error()
}

func (c *Config) validateGCPConfig() error {
	v := validation.NewValidatorWithPrefix("broker_config")

	if projectID, ok := c.BrokerConfig["project_id"].(string); ok {
		v.RequireString(projectID, "project_id")
	} else {
		v.RequireString("", "project_id")
	}

	// Must have either topic_id or subscription_id
	topicID, hasTopic := c.BrokerConfig["topic_id"].(string)
	subscriptionID, hasSubscription := c.BrokerConfig["subscription_id"].(string)

	if !hasTopic && !hasSubscription {
		v.Validate(func() error {
			return errors.ConfigError("either topic_id or subscription_id is required")
		})
	}

	if hasTopic {
		v.RequireString(topicID, "topic_id")
	}
	if hasSubscription {
		v.RequireString(subscriptionID, "subscription_id")
	}

	return v.Error()
}

// GetName returns the trigger name (implements base.TriggerConfig)
func (c *Config) GetName() string {
	return c.Name
}

// GetID returns the trigger ID (implements base.TriggerConfig)
func (c *Config) GetID() int {
	return c.ID
}