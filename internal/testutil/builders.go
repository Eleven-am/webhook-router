package testutil

import (
	"encoding/json"
	"time"

	"webhook-router/internal/brokers"
	"webhook-router/internal/storage"
	"webhook-router/internal/triggers"
)

// Helper function for string pointers
func stringPtr(s string) *string {
	return &s
}

// TriggerBuilder helps build test triggers
type TriggerBuilder struct {
	trigger *storage.Trigger
}

// NewTriggerBuilder creates a new trigger builder
func NewTriggerBuilder() *TriggerBuilder {
	return &TriggerBuilder{
		trigger: &storage.Trigger{
			ID:        "test-trigger-id",
			Name:      "test-trigger",
			Type:      "http",
			Active:    true,
			Status:    "active",
			UserID:    "test-user-id",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
			Config:    make(map[string]interface{}),
		},
	}
}

func (b *TriggerBuilder) WithID(id string) *TriggerBuilder {
	b.trigger.ID = id
	return b
}

func (b *TriggerBuilder) WithName(name string) *TriggerBuilder {
	b.trigger.Name = name
	return b
}

func (b *TriggerBuilder) WithType(triggerType string) *TriggerBuilder {
	b.trigger.Type = triggerType
	return b
}

func (b *TriggerBuilder) WithPipelineID(id string) *TriggerBuilder {
	b.trigger.PipelineID = &id
	return b
}

func (b *TriggerBuilder) WithBrokerID(id string) *TriggerBuilder {
	b.trigger.DestinationBrokerID = &id
	return b
}

func (b *TriggerBuilder) WithActive(active bool) *TriggerBuilder {
	b.trigger.Active = active
	return b
}

func (b *TriggerBuilder) WithSignatureConfig(config string) *TriggerBuilder {
	b.trigger.SignatureConfig = config
	return b
}

func (b *TriggerBuilder) WithUserID(userID string) *TriggerBuilder {
	b.trigger.UserID = userID
	return b
}

func (b *TriggerBuilder) WithConfig(config map[string]interface{}) *TriggerBuilder {
	b.trigger.Config = config
	return b
}

func (b *TriggerBuilder) Build() *storage.Trigger {
	return b.trigger
}

// UserBuilder helps build test users
type UserBuilder struct {
	user *storage.User
}

// NewUserBuilder creates a new user builder
func NewUserBuilder() *UserBuilder {
	return &UserBuilder{
		user: &storage.User{
			ID:           "test-user-id",
			Username:     "testuser",
			PasswordHash: "$2a$10$YourHashedPasswordHere",
			IsDefault:    false,
			CreatedAt:    time.Now(),
			UpdatedAt:    time.Now(),
		},
	}
}

func (b *UserBuilder) WithID(id string) *UserBuilder {
	b.user.ID = id
	return b
}

func (b *UserBuilder) WithUsername(username string) *UserBuilder {
	b.user.Username = username
	return b
}

func (b *UserBuilder) WithPasswordHash(hash string) *UserBuilder {
	b.user.PasswordHash = hash
	return b
}

func (b *UserBuilder) WithIsDefault(isDefault bool) *UserBuilder {
	b.user.IsDefault = isDefault
	return b
}

func (b *UserBuilder) Build() *storage.User {
	return b.user
}

// WebhookLogBuilder helps build test webhook logs
// TODO: Remove after migration to HTTP triggers
/*
type WebhookLogBuilder struct {
	log *storage.WebhookLog
}

// NewWebhookLogBuilder creates a new webhook log builder
func NewWebhookLogBuilder() *WebhookLogBuilder {
	routeID := "test-route-id"
	return &WebhookLogBuilder{
		log: &storage.WebhookLog{
			ID:           "test-log-id",
			RouteID:      routeID,
			Method:       "POST",
			Endpoint:     "/webhook/test",
			Headers:      `{"Content-Type": "application/json"}`,
			Body:         `{"test": "data"}`,
			StatusCode:   200,
			ProcessedAt:  time.Now(),
			TransformationTimeMS: 10,
			BrokerPublishTimeMS:  5,
		},
	}
}

func (b *WebhookLogBuilder) WithID(id string) *WebhookLogBuilder {
	b.log.ID = id
	return b
}

func (b *WebhookLogBuilder) WithRouteID(id string) *WebhookLogBuilder {
	b.log.RouteID = id
	return b
}

func (b *WebhookLogBuilder) WithStatusCode(code int) *WebhookLogBuilder {
	b.log.StatusCode = code
	return b
}

func (b *WebhookLogBuilder) WithError(err string) *WebhookLogBuilder {
	b.log.Error = err
	return b
}

func (b *WebhookLogBuilder) WithBody(body string) *WebhookLogBuilder {
	b.log.Body = body
	return b
}

func (b *WebhookLogBuilder) Build() *storage.WebhookLog {
	return b.log
}
*/

// TriggerConfigBuilder helps build test trigger configs
type TriggerConfigBuilder struct {
	config *triggers.BaseTriggerConfig
}

// NewTriggerConfigBuilder creates a new trigger config builder
func NewTriggerConfigBuilder() *TriggerConfigBuilder {
	return &TriggerConfigBuilder{
		config: &triggers.BaseTriggerConfig{
			ID:        "test-trigger-id",
			Name:      "test-trigger",
			Type:      "http",
			Active:    true,
			Settings:  make(map[string]interface{}),
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		},
	}
}

func (b *TriggerConfigBuilder) WithID(id string) *TriggerConfigBuilder {
	b.config.ID = id
	return b
}

func (b *TriggerConfigBuilder) WithName(name string) *TriggerConfigBuilder {
	b.config.Name = name
	return b
}

func (b *TriggerConfigBuilder) WithType(triggerType string) *TriggerConfigBuilder {
	b.config.Type = triggerType
	return b
}

func (b *TriggerConfigBuilder) WithActive(active bool) *TriggerConfigBuilder {
	b.config.Active = active
	return b
}

func (b *TriggerConfigBuilder) WithSetting(key string, value interface{}) *TriggerConfigBuilder {
	b.config.Settings[key] = value
	return b
}

func (b *TriggerConfigBuilder) Build() *triggers.BaseTriggerConfig {
	return b.config
}

// BrokerMessageBuilder helps build test broker messages
type BrokerMessageBuilder struct {
	message *brokers.Message
}

// NewBrokerMessageBuilder creates a new broker message builder
func NewBrokerMessageBuilder() *BrokerMessageBuilder {
	return &BrokerMessageBuilder{
		message: &brokers.Message{
			Queue:      "test-queue",
			Exchange:   "test-exchange",
			RoutingKey: "test.routing.key",
			Headers:    make(map[string]string),
			Body:       []byte(`{"test": "data"}`),
			Timestamp:  time.Now(),
			MessageID:  "test-message-id",
		},
	}
}

func (b *BrokerMessageBuilder) WithQueue(queue string) *BrokerMessageBuilder {
	b.message.Queue = queue
	return b
}

func (b *BrokerMessageBuilder) WithExchange(exchange string) *BrokerMessageBuilder {
	b.message.Exchange = exchange
	return b
}

func (b *BrokerMessageBuilder) WithRoutingKey(key string) *BrokerMessageBuilder {
	b.message.RoutingKey = key
	return b
}

func (b *BrokerMessageBuilder) WithHeader(key, value string) *BrokerMessageBuilder {
	b.message.Headers[key] = value
	return b
}

func (b *BrokerMessageBuilder) WithBody(body []byte) *BrokerMessageBuilder {
	b.message.Body = body
	return b
}

func (b *BrokerMessageBuilder) WithJSONBody(data interface{}) *BrokerMessageBuilder {
	body, _ := json.Marshal(data)
	b.message.Body = body
	return b
}

func (b *BrokerMessageBuilder) WithMessageID(id string) *BrokerMessageBuilder {
	b.message.MessageID = id
	return b
}

func (b *BrokerMessageBuilder) Build() *brokers.Message {
	return b.message
}

// IncomingMessageBuilder helps build test incoming messages
type IncomingMessageBuilder struct {
	message *brokers.IncomingMessage
}

// NewIncomingMessageBuilder creates a new incoming message builder
func NewIncomingMessageBuilder() *IncomingMessageBuilder {
	return &IncomingMessageBuilder{
		message: &brokers.IncomingMessage{
			ID:        "test-incoming-id",
			Headers:   make(map[string]string),
			Body:      []byte(`{"test": "data"}`),
			Timestamp: time.Now(),
			Source: brokers.BrokerInfo{
				Name: "test-broker",
				Type: "rabbitmq",
				URL:  "amqp://localhost:5672",
			},
			Metadata: make(map[string]interface{}),
		},
	}
}

func (b *IncomingMessageBuilder) WithID(id string) *IncomingMessageBuilder {
	b.message.ID = id
	return b
}

func (b *IncomingMessageBuilder) WithHeader(key, value string) *IncomingMessageBuilder {
	b.message.Headers[key] = value
	return b
}

func (b *IncomingMessageBuilder) WithBody(body []byte) *IncomingMessageBuilder {
	b.message.Body = body
	return b
}

func (b *IncomingMessageBuilder) WithJSONBody(data interface{}) *IncomingMessageBuilder {
	body, _ := json.Marshal(data)
	b.message.Body = body
	return b
}

func (b *IncomingMessageBuilder) WithSource(name, brokerType, url string) *IncomingMessageBuilder {
	b.message.Source = brokers.BrokerInfo{
		Name: name,
		Type: brokerType,
		URL:  url,
	}
	return b
}

func (b *IncomingMessageBuilder) Build() *brokers.IncomingMessage {
	return b.message
}

// TODO: PipelineDataBuilder needs to be updated for the new pipeline structure
// Commenting out until pipeline integration is complete
/*
// PipelineDataBuilder helps build test pipeline data
type PipelineDataBuilder struct {
	data *pipeline.Data
}
// ... rest of PipelineDataBuilder commented out ...
*/

// DLQMessageBuilder helps build test DLQ messages
type DLQMessageBuilder struct {
	message *storage.DLQMessage
}

// NewDLQMessageBuilder creates a new DLQ message builder
func NewDLQMessageBuilder() *DLQMessageBuilder {
	now := time.Now()
	return &DLQMessageBuilder{
		message: &storage.DLQMessage{
			ID:           "test-dlq-message-id",
			MessageID:    "test-message-id",
			TriggerID:    stringPtr("test-trigger-id"),
			BrokerName:   "test-broker",
			Queue:        "test-queue",
			Exchange:     "test-exchange",
			RoutingKey:   "test.route",
			Headers:      map[string]string{"Content-Type": "application/json"},
			Body:         `{"test": "data"}`,
			ErrorMessage: "Test error",
			FailureCount: 1,
			FirstFailure: now,
			LastFailure:  now,
			NextRetry:    &now,
			Status:       "pending",
			Metadata:     map[string]interface{}{"test": "metadata"},
			CreatedAt:    now,
			UpdatedAt:    now,
		},
	}
}

func (b *DLQMessageBuilder) WithID(id string) *DLQMessageBuilder {
	b.message.ID = id
	return b
}

func (b *DLQMessageBuilder) WithBrokerName(name string) *DLQMessageBuilder {
	b.message.BrokerName = name
	return b
}

func (b *DLQMessageBuilder) WithTriggerID(id string) *DLQMessageBuilder {
	b.message.TriggerID = stringPtr(id)
	return b
}

func (b *DLQMessageBuilder) WithError(err string) *DLQMessageBuilder {
	b.message.ErrorMessage = err
	return b
}

func (b *DLQMessageBuilder) WithStatus(status string) *DLQMessageBuilder {
	b.message.Status = status
	return b
}

func (b *DLQMessageBuilder) WithFailureCount(count int) *DLQMessageBuilder {
	b.message.FailureCount = count
	return b
}

func (b *DLQMessageBuilder) WithBody(body string) *DLQMessageBuilder {
	b.message.Body = body
	return b
}

func (b *DLQMessageBuilder) Build() *storage.DLQMessage {
	return b.message
}
