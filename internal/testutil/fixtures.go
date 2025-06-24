package testutil

import (
	"time"
	"webhook-router/internal/storage"
)

// TestFixtures provides common test data
type TestFixtures struct {
	Users         []*storage.User
	Triggers      []*storage.Trigger
	Pipelines     []*storage.Pipeline
	BrokerConfigs []*storage.BrokerConfig
	DLQMessages   []*storage.DLQMessage
}

// NewTestFixtures creates a new set of test fixtures
func NewTestFixtures() *TestFixtures {
	now := time.Now()

	return &TestFixtures{
		Triggers: []*storage.Trigger{
			{
				ID:   "trigger-1",
				Name: "webhook-trigger-1",
				Type: "http",
				Config: map[string]interface{}{
					"path":   "/webhook/orders",
					"method": "POST",
				},
				Active:    true,
				Status:    "active",
				UserID:    "test-user-id",
				CreatedAt: now,
				UpdatedAt: now,
			},
			{
				ID:   "trigger-2",
				Name: "webhook-trigger-2",
				Type: "http",
				Config: map[string]interface{}{
					"path":   "/webhook/payments",
					"method": "POST",
				},
				Active:    true,
				Status:    "active",
				UserID:    "test-user-id",
				CreatedAt: now,
				UpdatedAt: now,
			},
			{
				ID:   "trigger-3",
				Name: "inactive-trigger",
				Type: "http",
				Config: map[string]interface{}{
					"path":   "/webhook/inactive",
					"method": "POST",
				},
				Active:    false,
				Status:    "stopped",
				UserID:    "test-user-id",
				CreatedAt: now,
				UpdatedAt: now,
			},
		},
		Users: []*storage.User{
			{
				ID:           "user-1",
				Username:     "admin",
				PasswordHash: "$2a$10$YourHashedAdminPasswordHere",
				IsDefault:    true,
				CreatedAt:    now,
				UpdatedAt:    now,
			},
			{
				ID:           "user-2",
				Username:     "user1",
				PasswordHash: "$2a$10$YourHashedUser1PasswordHere",
				IsDefault:    false,
				CreatedAt:    now,
				UpdatedAt:    now,
			},
			{
				ID:           "user-3",
				Username:     "user2",
				PasswordHash: "$2a$10$YourHashedUser2PasswordHere",
				IsDefault:    false,
				CreatedAt:    now,
				UpdatedAt:    now,
			},
		},
		Pipelines: []*storage.Pipeline{
			{
				ID:          "pipeline-1",
				Name:        "transform-pipeline_old",
				Description: "Transform JSON data",
				Stages: []map[string]interface{}{
					{
						"name": "validate-json",
						"type": "validate",
						"config": map[string]interface{}{
							"schema": "json",
						},
					},
					{
						"name": "transform-data",
						"type": "transform",
						"config": map[string]interface{}{
							"template":   "jq",
							"expression": ".data | {processed: .}",
						},
					},
				},
				Active:    true,
				CreatedAt: now,
				UpdatedAt: now,
			},
			{
				ID:          "pipeline_old-2",
				Name:        "enrich-pipeline_old",
				Description: "Enrich data with external API",
				Stages: []map[string]interface{}{
					{
						"name": "enrich-customer",
						"type": "enrich",
						"config": map[string]interface{}{
							"url":    "https://api.example.com/customers/{id}",
							"method": "GET",
						},
					},
				},
				Active:    true,
				CreatedAt: now,
				UpdatedAt: now,
			},
		},
		BrokerConfigs: []*storage.BrokerConfig{
			{
				ID:   "trigger-1",
				Name: "rabbitmq-main",
				Type: "rabbitmq",
				Config: map[string]interface{}{
					"url":      "amqp://localhost:5672",
					"exchange": "main-exchange",
				},
				Active:       true,
				HealthStatus: "healthy",
				CreatedAt:    now,
				UpdatedAt:    now,
			},
			{
				ID:   "trigger-2",
				Name: "kafka-main",
				Type: "kafka",
				Config: map[string]interface{}{
					"brokers": []string{"localhost:9092"},
					"topic":   "main-topic",
				},
				Active:       true,
				HealthStatus: "healthy",
				CreatedAt:    now,
				UpdatedAt:    now,
			},
			{
				ID:   "trigger-3",
				Name: "redis-streams",
				Type: "redis",
				Config: map[string]interface{}{
					"url":    "redis://localhost:6379",
					"stream": "main-stream",
				},
				Active:       false,
				HealthStatus: "unhealthy",
				CreatedAt:    now,
				UpdatedAt:    now,
			},
		},
		DLQMessages: []*storage.DLQMessage{
			{
				ID:           "dlq-1",
				MessageID:    "dlq-msg-001",
				TriggerID:    stringPtr("trigger-1"),
				BrokerName:   "rabbitmq-main",
				Queue:        "orders-queue",
				Exchange:     "orders-exchange",
				RoutingKey:   "orders.created",
				Headers:      map[string]string{"Content-Type": "application/json"},
				Body:         `{"order_id": 123, "status": "pending"}`,
				ErrorMessage: "Connection timeout",
				FailureCount: 3,
				FirstFailure: now.Add(-time.Hour),
				LastFailure:  now.Add(-time.Minute * 10),
				Status:       "pending",
				Metadata:     map[string]interface{}{"retry_count": 3},
				CreatedAt:    now.Add(-time.Hour),
				UpdatedAt:    now.Add(-time.Minute * 10),
			},
			{
				ID:           "dlq-2",
				MessageID:    "dlq-msg-002",
				TriggerID:    stringPtr("trigger-2"),
				BrokerName:   "kafka-main",
				Queue:        "payments-queue",
				Exchange:     "payments-exchange",
				RoutingKey:   "payments.processed",
				Headers:      map[string]string{"Content-Type": "application/json"},
				Body:         `{"payment_id": 456, "amount": 100.50}`,
				ErrorMessage: "Invalid message format",
				FailureCount: 5,
				FirstFailure: now.Add(-time.Hour * 2),
				LastFailure:  now.Add(-time.Minute * 5),
				Status:       "abandoned",
				Metadata:     map[string]interface{}{"retry_count": 5},
				CreatedAt:    now.Add(-time.Hour * 2),
				UpdatedAt:    now.Add(-time.Minute * 5),
			},
		},
	}
}

// SeedStorage populates a storage instance with test fixtures
func SeedStorage(storage storage.Storage, fixtures *TestFixtures) error {
	// Add triggers
	for _, trigger := range fixtures.Triggers {
		if err := storage.CreateTrigger(trigger); err != nil {
			return err
		}
	}

	// Add users
	for range fixtures.Users {
		// Note: In real implementation, you'd need a CreateUser method
		// For now, we'll assume the mock storage handles this via other methods
	}

	// Add triggers
	for _, trigger := range fixtures.Triggers {
		if err := storage.CreateTrigger(trigger); err != nil {
			return err
		}
	}

	// Add pipelines
	for _, pipeline := range fixtures.Pipelines {
		if err := storage.CreatePipeline(pipeline); err != nil {
			return err
		}
	}

	// Add broker configs
	for _, broker := range fixtures.BrokerConfigs {
		if err := storage.CreateBroker(broker); err != nil {
			return err
		}
	}

	// Add DLQ messages
	for _, msg := range fixtures.DLQMessages {
		if err := storage.CreateDLQMessage(msg); err != nil {
			return err
		}
	}

	return nil
}

// Common test constants
const (
	TestJWTSecret       = "test-jwt-secret-minimum-32-characters-long"
	TestEncryptionKey   = "test-encryption-key-32-bytes-long!"
	TestAdminUsername   = "admin"
	TestAdminPassword   = "admin123"
	TestUserUsername    = "testuser"
	TestUserPassword    = "testpass123"
	TestBrokerURL       = "amqp://localhost:5672"
	TestRedisURL        = "redis://localhost:6379"
	TestKafkaBroker     = "localhost:9092"
	TestWebhookEndpoint = "/webhook/test"
	TestQueueName       = "test-queue"
	TestExchangeName    = "test-exchange"
	TestRoutingKey      = "test.routing.key"
)

// Sample JSON payloads
var (
	SampleOrderPayload = []byte(`{
		"order_id": 12345,
		"customer_id": 67890,
		"items": [
			{"product_id": "ABC123", "quantity": 2, "price": 29.99},
			{"product_id": "DEF456", "quantity": 1, "price": 49.99}
		],
		"total": 109.97,
		"status": "pending"
	}`)

	SamplePaymentPayload = []byte(`{
		"payment_id": "PAY-123456",
		"order_id": 12345,
		"amount": 109.97,
		"currency": "USD",
		"method": "credit_card",
		"status": "completed"
	}`)

	SampleWebhookHeaders = map[string]string{
		"Content-Type":     "application/json",
		"X-Webhook-ID":     "webhook-123",
		"X-Webhook-Source": "test-system",
		"User-Agent":       "TestWebhook/1.0",
	}
)
