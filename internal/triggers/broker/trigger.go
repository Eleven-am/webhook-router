package broker

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"text/template"
	"time"

	"webhook-router/internal/brokers"
	"webhook-router/internal/brokers/aws"
	"webhook-router/internal/brokers/gcp"
	"webhook-router/internal/brokers/kafka"
	"webhook-router/internal/brokers/rabbitmq"
	redisbroker "webhook-router/internal/brokers/redis"
	"webhook-router/internal/common/base"
	"webhook-router/internal/common/errors"
	"webhook-router/internal/common/logging"
	commonutils "webhook-router/internal/common/utils"
	"webhook-router/internal/pipeline/utils"
	"webhook-router/internal/triggers"
)

// Trigger implements the broker event trigger using BaseTrigger
type Trigger struct {
	*base.BaseTrigger
	config       *Config
	broker       brokers.Broker
	messageCount int64
	errorCount   int64
	mu           sync.RWMutex
	builder      *triggers.TriggerBuilder
}

func NewTrigger(config *Config, brokerRegistry *brokers.Registry) (*Trigger, error) {
	builder := triggers.NewTriggerBuilder("broker", config)

	// Create broker instance from config
	brokerConfig, err := createBrokerConfig(config)
	if err != nil {
		return nil, errors.ConfigError(fmt.Sprintf("failed to create broker config: %v", err))
	}

	// Cast to BrokerConfig interface
	bConfig, ok := brokerConfig.(brokers.BrokerConfig)
	if !ok {
		return nil, errors.ConfigError("invalid broker config type")
	}

	broker, err := brokerRegistry.Create(config.BrokerType, bConfig)
	if err != nil {
		return nil, errors.InternalError("failed to create broker", err)
	}

	trigger := &Trigger{
		config:       config,
		broker:       broker,
		messageCount: 0,
		errorCount:   0,
		builder:      builder,
	}

	// Initialize BaseTrigger
	trigger.BaseTrigger = builder.BuildBaseTrigger(nil)

	return trigger, nil
}

// NextExecution returns nil as broker triggers are event-driven
func (t *Trigger) NextExecution() *time.Time {
	return nil
}

// Start starts the broker trigger
func (t *Trigger) Start(ctx context.Context, handler triggers.TriggerHandler) error {
	// Build BaseTrigger with the handler
	t.BaseTrigger = t.builder.BuildBaseTrigger(handler)

	// Use the BaseTrigger's Start method with our run function
	return t.BaseTrigger.Start(ctx, func(ctx context.Context) error {
		return t.run(ctx, handler)
	})
}

// Stop stops the broker trigger
func (t *Trigger) Stop() error {
	// First stop the base trigger
	if err := t.BaseTrigger.Stop(); err != nil {
		return err
	}

	// Then close broker connection
	if t.broker != nil {
		if err := t.broker.Close(); err != nil {
			t.builder.Logger().Error("Failed to close broker", err)
		}
	}

	return nil
}

func (t *Trigger) run(ctx context.Context, handler triggers.TriggerHandler) error {
	// Connect to broker with config
	brokerConfig, err := createBrokerConfig(t.config)
	if err != nil {
		return errors.ConfigError(fmt.Sprintf("failed to create broker config: %v", err))
	}

	bConfig, ok := brokerConfig.(brokers.BrokerConfig)
	if !ok {
		return errors.ConfigError("invalid broker config type")
	}

	if err := t.broker.Connect(bConfig); err != nil {
		return errors.ConnectionError("failed to connect to broker", err)
	}

	t.builder.Logger().Info("Broker trigger started",
		logging.Field{"topic", t.config.Topic},
		logging.Field{"consumer_group", t.config.ConsumerGroup},
	)

	// Create message handler
	messageHandler := func(msg *brokers.IncomingMessage) error {
		t.processMessage(ctx, msg, handler)
		return nil
	}

	// Subscribe to topic
	if err := t.broker.Subscribe(ctx, t.config.Topic, messageHandler); err != nil {
		return errors.ConnectionError("failed to subscribe to broker", err)
	}

	// Wait for context cancellation
	<-ctx.Done()
	t.builder.Logger().Info("Broker trigger stopped")
	return nil
}

func (t *Trigger) processMessage(ctx context.Context, msg *brokers.IncomingMessage, handler triggers.TriggerHandler) {
	atomic.AddInt64(&t.messageCount, 1)

	// Check if message passes filters
	if !t.passesFilters(msg) {
		t.builder.Logger().Debug("Message filtered out",
			logging.Field{"message_id", msg.ID},
		)
		return
	}

	// Create trigger event with enhanced metadata
	event := &triggers.TriggerEvent{
		ID:          commonutils.GenerateEventID("broker", t.config.ID),
		TriggerID:   t.config.ID,
		TriggerName: t.config.Name,
		Type:        "broker",
		Timestamp:   time.Now(),
		Data: map[string]interface{}{
			"message_id":  msg.ID,
			"body":        string(msg.Body),
			"broker_type": t.config.BrokerType,
			"topic":       t.config.Topic,
			"timestamp":   msg.Timestamp,
			"metadata":    msg.Metadata,
			// Preserve original message for DLQ
			"_original_message": map[string]interface{}{
				"id":        msg.ID,
				"headers":   msg.Headers,
				"body":      msg.Body,
				"timestamp": msg.Timestamp,
				"source":    msg.Source,
				"metadata":  msg.Metadata,
			},
		},
		Headers: msg.Headers,
		Source: triggers.TriggerSource{
			Type:     "broker",
			Name:     t.config.Name,
			Endpoint: t.config.BrokerType,
			Metadata: t.buildSourceMetadata(msg),
		},
	}

	// Apply transformations
	if t.config.Transformation.Enabled {
		t.applyTransformations(event, msg)
	}

	// Handle the event through the handler directly
	err := handler(event)

	if err != nil {
		atomic.AddInt64(&t.errorCount, 1)
		t.handleMessageError(msg, err)
	}
}

func (t *Trigger) passesFilters(msg *brokers.IncomingMessage) bool {
	if !t.config.MessageFilter.Enabled {
		return true
	}

	filter := t.config.MessageFilter

	// Check headers
	for name, value := range filter.Headers {
		if msg.Headers[name] != value {
			return false
		}
	}

	// Check content type
	if filter.ContentType != "" {
		contentType := msg.Headers["Content-Type"]
		if !strings.Contains(contentType, filter.ContentType) {
			return false
		}
	}

	// Check body contains/not contains
	bodyStr := string(msg.Body)
	if filter.BodyContains != "" && !strings.Contains(bodyStr, filter.BodyContains) {
		return false
	}
	if filter.BodyNotContains != "" && strings.Contains(bodyStr, filter.BodyNotContains) {
		return false
	}

	// Check size limits
	msgSize := int64(len(msg.Body))
	if filter.MinSize > 0 && msgSize < filter.MinSize {
		return false
	}
	if filter.MaxSize > 0 && msgSize > filter.MaxSize {
		return false
	}

	// Check JSON conditions
	if filter.JSONCondition.Enabled {
		if !t.checkJSONCondition(msg.Body, &filter.JSONCondition) {
			return false
		}
	}

	return true
}

func (t *Trigger) applyTransformations(event *triggers.TriggerEvent, msg *brokers.IncomingMessage) {
	transform := t.config.Transformation

	// Apply header mapping
	if len(transform.HeaderMapping) > 0 {
		newHeaders := make(map[string]string)
		for oldName, newName := range transform.HeaderMapping {
			if value, exists := event.Headers[oldName]; exists {
				newHeaders[newName] = value
			}
		}
		// Merge with existing headers
		for k, v := range newHeaders {
			event.Headers[k] = v
		}
	}

	// Add configured headers
	for name, value := range transform.AddHeaders {
		event.Headers[name] = value
	}

	// Extract fields
	for _, field := range transform.ExtractFields {
		value := t.extractField(msg, field)
		if value != "" || !field.Required {
			event.Data[field.Name] = value
		}
	}

	// Apply body template if configured
	if transform.BodyTemplate != "" {
		tmpl, err := template.New("body").Parse(transform.BodyTemplate)
		if err != nil {
			t.builder.Logger().Error("Failed to parse body template", err)
			return
		}

		var buf strings.Builder
		templateData := map[string]interface{}{
			"Event":   event,
			"Message": msg,
		}

		if err := tmpl.Execute(&buf, templateData); err != nil {
			t.builder.Logger().Error("Failed to execute body template", err)
			return
		}

		// Try to parse result as JSON
		var transformedBody interface{}
		if err := json.Unmarshal([]byte(buf.String()), &transformedBody); err == nil {
			event.Data["body"] = transformedBody
		} else {
			event.Data["body"] = buf.String()
		}
	}
}

func (t *Trigger) extractField(msg *brokers.IncomingMessage, field ExtractFieldConfig) string {
	switch field.Source {
	case "header":
		if value, exists := msg.Headers[field.Path]; exists {
			return value
		}

	case "metadata":
		// Extract from message metadata
		switch field.Path {
		case "id":
			return msg.ID
		case "timestamp":
			return msg.Timestamp.Format(time.RFC3339)
		case "source_name":
			return msg.Source.Name
		case "source_type":
			return msg.Source.Type
		default:
			// Check in metadata map
			if val, exists := msg.Metadata[field.Path]; exists {
				return fmt.Sprintf("%v", val)
			}
		}

	case "body":
		// Extract from JSON body using dot notation
		if field.Path == "" {
			return string(msg.Body)
		}

		// Parse JSON and extract field
		var data map[string]interface{}
		if err := json.Unmarshal(msg.Body, &data); err != nil {
			t.builder.Logger().Debug("Failed to parse JSON body for field extraction",
				logging.Field{"error", err.Error()},
				logging.Field{"field_path", field.Path},
			)
			return field.DefaultValue
		}

		value := utils.GetFieldValue(data, field.Path)
		if value != nil {
			return fmt.Sprintf("%v", value)
		}
	}

	return field.DefaultValue
}

func (t *Trigger) handleMessageError(msg *brokers.IncomingMessage, err error) {
	t.builder.Logger().Error("Failed to process message", err,
		logging.Field{"message_id", msg.ID},
	)

	// DLQ handling is implemented at the trigger manager level.
	// When this error is propagated up through the handler, the trigger manager
	// will check if the trigger has DLQ configured and use PublishWithFallback.
	if t.config.ErrorHandling.DeadLetterQueue != "" {
		t.builder.Logger().Debug("Message processing failed - DLQ handling delegated to trigger manager",
			logging.Field{"message_id", msg.ID},
			logging.Field{"configured_dlq", t.config.ErrorHandling.DeadLetterQueue},
			logging.Field{"error", err.Error()},
		)
	}

	// Alert if configured (skipping for now as requested)
	if t.config.ErrorHandling.AlertOnError {
		t.builder.Logger().Warn("Alert should be sent for message processing failure",
			logging.Field{"message_id", msg.ID},
		)
	}
}

// Health checks if the trigger is healthy
func (t *Trigger) Health() error {
	if !t.IsRunning() {
		return errors.InternalError("trigger is not running", nil)
	}

	// Check broker connection
	if t.broker != nil {
		if err := t.broker.Health(); err != nil {
			return errors.ConnectionError("broker unhealthy", err)
		}
	}

	// Check error rate
	msgCount := atomic.LoadInt64(&t.messageCount)
	errCount := atomic.LoadInt64(&t.errorCount)

	if msgCount > 100 && float64(errCount)/float64(msgCount) > 0.5 {
		return errors.InternalError(
			fmt.Sprintf("high error rate: %d errors out of %d messages", errCount, msgCount),
			nil,
		)
	}

	return nil
}

// GetStats returns trigger statistics
func (t *Trigger) GetStats() map[string]interface{} {
	return map[string]interface{}{
		"message_count": atomic.LoadInt64(&t.messageCount),
		"error_count":   atomic.LoadInt64(&t.errorCount),
		"broker_type":   t.config.BrokerType,
		"topic":         t.config.Topic,
	}
}

// createBrokerConfig creates broker-specific configuration
func createBrokerConfig(config *Config) (interface{}, error) {
	switch config.BrokerType {
	case "rabbitmq":
		return createRabbitMQConfig(config.BrokerConfig)
	case "kafka":
		return createKafkaConfig(config.BrokerConfig)
	case "redis":
		return createRedisConfig(config.BrokerConfig)
	case "aws":
		return createAWSConfig(config.BrokerConfig)
	case "gcp":
		return createGCPConfig(config.BrokerConfig)
	default:
		return nil, fmt.Errorf("unsupported broker type: %s", config.BrokerType)
	}
}

func createRabbitMQConfig(configMap map[string]interface{}) (*rabbitmq.Config, error) {
	cfg := &rabbitmq.Config{}

	if url, ok := configMap["url"].(string); ok {
		cfg.URL = url
	}
	if poolSize, ok := configMap["pool_size"].(float64); ok {
		cfg.PoolSize = int(poolSize)
	} else {
		cfg.PoolSize = 5 // default
	}

	return cfg, nil
}

func createKafkaConfig(configMap map[string]interface{}) (*kafka.Config, error) {
	cfg := &kafka.Config{}

	if brokers, ok := configMap["brokers"].([]interface{}); ok {
		for _, broker := range brokers {
			if b, ok := broker.(string); ok {
				cfg.Brokers = append(cfg.Brokers, b)
			}
		}
	}
	if clientID, ok := configMap["client_id"].(string); ok {
		cfg.ClientID = clientID
	}
	if groupID, ok := configMap["group_id"].(string); ok {
		cfg.GroupID = groupID
	}

	return cfg, nil
}

func createRedisConfig(configMap map[string]interface{}) (*redisbroker.Config, error) {
	cfg := &redisbroker.Config{}

	if address, ok := configMap["address"].(string); ok {
		cfg.Address = address
	}
	if password, ok := configMap["password"].(string); ok {
		cfg.Password = password
	}
	if db, ok := configMap["db"].(float64); ok {
		cfg.DB = int(db)
	}
	if poolSize, ok := configMap["pool_size"].(float64); ok {
		cfg.PoolSize = int(poolSize)
	} else {
		cfg.PoolSize = 10 // default
	}
	if consumerGroup, ok := configMap["consumer_group"].(string); ok {
		cfg.ConsumerGroup = consumerGroup
	}

	return cfg, nil
}

func createAWSConfig(configMap map[string]interface{}) (*aws.Config, error) {
	cfg := &aws.Config{}

	if region, ok := configMap["region"].(string); ok {
		cfg.Region = region
	}
	if accessKeyID, ok := configMap["access_key_id"].(string); ok {
		cfg.AccessKeyID = accessKeyID
	}
	if secretAccessKey, ok := configMap["secret_access_key"].(string); ok {
		cfg.SecretAccessKey = secretAccessKey
	}
	if queueURL, ok := configMap["queue_url"].(string); ok {
		cfg.QueueURL = queueURL
	}
	if topicArn, ok := configMap["topic_arn"].(string); ok {
		cfg.TopicArn = topicArn
	}

	return cfg, nil
}

// buildSourceMetadata builds enhanced metadata for broker messages
func (t *Trigger) buildSourceMetadata(msg *brokers.IncomingMessage) map[string]interface{} {
	metadata := map[string]interface{}{
		"broker_type": t.config.BrokerType,
		"message_id":  msg.ID,
		"timestamp":   msg.Timestamp.Format(time.RFC3339),
	}

	// Add generic metadata from the message
	if msg.Metadata != nil {
		for k, v := range msg.Metadata {
			metadata[k] = v
		}
	}

	// Add broker-specific configuration metadata from settings
	if t.config.Settings != nil {
		switch t.config.BrokerType {
		case "kafka":
			if group, ok := t.config.Settings["consumer_group"].(string); ok {
				metadata["consumer_group"] = group
			}
			if topics, ok := t.config.Settings["topics"]; ok {
				metadata["topics"] = topics
			}

		case "rabbitmq":
			if queue, ok := t.config.Settings["queue"].(string); ok {
				metadata["queue"] = queue
			}
			if exchange, ok := t.config.Settings["exchange"].(string); ok {
				metadata["exchange"] = exchange
			}

		case "redis":
			if channel, ok := t.config.Settings["channel"].(string); ok {
				metadata["pubsub_channel"] = channel
			}

		case "aws_sqs":
			if queueURL, ok := t.config.Settings["queue_url"].(string); ok {
				metadata["queue_url"] = queueURL
			}

		case "aws_sns":
			if topicArn, ok := t.config.Settings["topic_arn"].(string); ok {
				metadata["topic_arn"] = topicArn
			}

		case "gcp_pubsub":
			if topicID, ok := t.config.Settings["topic_id"].(string); ok {
				metadata["topic_id"] = topicID
			}
			if subID, ok := t.config.Settings["subscription_id"].(string); ok {
				metadata["subscription_id"] = subID
			}
		}
	}

	return metadata
}

// Config returns the trigger configuration
func (t *Trigger) Config() triggers.TriggerConfig {
	return t.config
}

// checkJSONCondition checks if the JSON message body matches the specified condition
func (t *Trigger) checkJSONCondition(body []byte, condition *JSONConditionConfig) bool {
	var data map[string]interface{}
	if err := json.Unmarshal(body, &data); err != nil {
		t.builder.Logger().Debug("Failed to parse JSON for condition check",
			logging.Field{"error", err.Error()},
		)
		return false
	}

	// Extract the value at the specified path
	var testValue interface{}
	if condition.Path == "" {
		// No path means check the entire body
		testValue = data
	} else {
		testValue = utils.GetFieldValue(data, condition.Path)
	}

	// Check "exists" operator first
	if condition.Operator == "exists" {
		return testValue != nil
	}

	// If value doesn't exist and operator is not "exists", condition fails
	if testValue == nil {
		return false
	}

	// Convert test value to string for comparison
	testValueStr := fmt.Sprintf("%v", testValue)

	// Handle numeric comparisons if value type is number
	if condition.ValueType == "number" {
		return t.checkNumericCondition(testValueStr, condition.Operator, condition.Value)
	}

	// Handle boolean comparisons
	if condition.ValueType == "boolean" {
		return t.checkBooleanCondition(testValueStr, condition.Operator, condition.Value)
	}

	// String comparisons
	switch condition.Operator {
	case "eq":
		return testValueStr == condition.Value
	case "ne":
		return testValueStr != condition.Value
	case "contains":
		return strings.Contains(testValueStr, condition.Value)
	case "starts_with":
		return strings.HasPrefix(testValueStr, condition.Value)
	case "ends_with":
		return strings.HasSuffix(testValueStr, condition.Value)
	case "regex":
		if regex, err := regexp.Compile(condition.Value); err == nil {
			return regex.MatchString(testValueStr)
		}
		return false
	case "in":
		// Value should be comma-separated list
		values := strings.Split(condition.Value, ",")
		for _, v := range values {
			if strings.TrimSpace(v) == testValueStr {
				return true
			}
		}
		return false
	default:
		t.builder.Logger().Warn("Unknown JSON condition operator",
			logging.Field{"operator", condition.Operator},
		)
		return false
	}
}

func (t *Trigger) checkNumericCondition(testValue, operator, compareValue string) bool {
	testNum, err := strconv.ParseFloat(testValue, 64)
	if err != nil {
		return false
	}

	compareNum, err := strconv.ParseFloat(compareValue, 64)
	if err != nil {
		return false
	}

	switch operator {
	case "eq":
		return testNum == compareNum
	case "ne":
		return testNum != compareNum
	case "gt":
		return testNum > compareNum
	case "lt":
		return testNum < compareNum
	case "gte":
		return testNum >= compareNum
	case "lte":
		return testNum <= compareNum
	default:
		return false
	}
}

func (t *Trigger) checkBooleanCondition(testValue, operator, compareValue string) bool {
	testBool := testValue == "true" || testValue == "1"
	compareBool := compareValue == "true" || compareValue == "1"

	switch operator {
	case "eq":
		return testBool == compareBool
	case "ne":
		return testBool != compareBool
	default:
		return false
	}
}

func createGCPConfig(configMap map[string]interface{}) (*gcp.Config, error) {
	cfg := &gcp.Config{}

	if projectID, ok := configMap["project_id"].(string); ok {
		cfg.ProjectID = projectID
	}
	if topicID, ok := configMap["topic_id"].(string); ok {
		cfg.TopicID = topicID
	}
	if subscriptionID, ok := configMap["subscription_id"].(string); ok {
		cfg.SubscriptionID = subscriptionID
	}
	if credentialsJSON, ok := configMap["credentials_json"].(string); ok {
		cfg.CredentialsJSON = credentialsJSON
	}
	if credentialsPath, ok := configMap["credentials_path"].(string); ok {
		cfg.CredentialsPath = credentialsPath
	}
	if createSubscription, ok := configMap["create_subscription"].(bool); ok {
		cfg.CreateSubscription = createSubscription
	}
	if ackDeadline, ok := configMap["ack_deadline"].(float64); ok {
		cfg.AckDeadline = int(ackDeadline)
	}
	if maxOutstandingMessages, ok := configMap["max_outstanding_messages"].(float64); ok {
		cfg.MaxOutstandingMessages = int(maxOutstandingMessages)
	}
	if enableOrdering, ok := configMap["enable_message_ordering"].(bool); ok {
		cfg.EnableMessageOrdering = enableOrdering
	}
	if orderingKey, ok := configMap["ordering_key"].(string); ok {
		cfg.OrderingKey = orderingKey
	}

	return cfg, nil
}
