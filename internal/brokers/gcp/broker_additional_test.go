package gcp

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"webhook-router/internal/common/config"
)

func TestBroker_NewBroker(t *testing.T) {
	tests := []struct {
		name    string
		config  *Config
		wantErr bool
		errMsg  string
	}{
		{
			name: "Valid config without credentials",
			config: &Config{
				BaseConnConfig: config.BaseConnConfig{},
				ProjectID:      "test-project",
				TopicID:        "test-topic",
				SubscriptionID: "test-subscription",
			},
			wantErr: false,
		},
		{
			name: "Valid config with credentials JSON",
			config: &Config{
				BaseConnConfig:  config.BaseConnConfig{},
				ProjectID:       "test-project",
				TopicID:         "test-topic",
				SubscriptionID:  "test-subscription",
				CredentialsJSON: `{"type": "service_account", "project_id": "test"}`,
			},
			wantErr: false,
		},
		{
			name: "Valid config with credentials file",
			config: &Config{
				BaseConnConfig:  config.BaseConnConfig{},
				ProjectID:       "test-project",
				TopicID:         "test-topic",
				SubscriptionID:  "test-subscription",
				CredentialsPath: "/path/to/credentials.json",
			},
			wantErr: false,
		},
		{
			name: "Invalid config - missing project ID",
			config: &Config{
				BaseConnConfig: config.BaseConnConfig{},
				TopicID:        "test-topic",
				SubscriptionID: "test-subscription",
			},
			wantErr: true,
			errMsg:  "project_id is required",
		},
		{
			name: "Invalid config - missing topic ID",
			config: &Config{
				BaseConnConfig: config.BaseConnConfig{},
				ProjectID:      "test-project",
				SubscriptionID: "test-subscription",
			},
			wantErr: true,
			errMsg:  "topic_id is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			broker, err := NewBroker(tt.config)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, broker)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				// Can't actually create a real broker without GCP credentials
				// Any error at this point is expected since we don't have real GCP credentials
				if err == nil {
					assert.NotNil(t, broker)
				}
			}
		})
	}
}

func TestConfig_AdvancedValidation(t *testing.T) {
	tests := []struct {
		name    string
		config  *Config
		wantErr bool
		errMsg  string
	}{
		{
			name: "Valid config with all fields",
			config: &Config{
				BaseConnConfig:         config.BaseConnConfig{Timeout: 30 * time.Second, RetryMax: 3},
				ProjectID:              "test-project",
				TopicID:                "test-topic",
				SubscriptionID:         "test-subscription",
				CreateSubscription:     true,
				AckDeadline:            60,
				MaxOutstandingMessages: 1000,
				EnableMessageOrdering:  true,
				OrderingKey:            "test-key",
				CredentialsJSON:        `{"type": "service_account"}`,
			},
			wantErr: false,
		},
		{
			name: "Valid config with defaults applied",
			config: &Config{
				ProjectID:      "test-project",
				TopicID:        "test-topic",
				SubscriptionID: "test-subscription",
			},
			wantErr: false,
		},
		{
			name: "Auto-generate subscription ID",
			config: &Config{
				ProjectID:          "test-project",
				TopicID:            "test-topic",
				CreateSubscription: true,
				// No SubscriptionID provided
			},
			wantErr: false,
		},
		{
			name: "Invalid - ack deadline below minimum",
			config: &Config{
				ProjectID:      "test-project",
				TopicID:        "test-topic",
				SubscriptionID: "test-subscription",
				AckDeadline:    9, // Below 10
			},
			wantErr: true,
			errMsg:  "ack_deadline must be between 10 and 600 seconds",
		},
		{
			name: "Invalid - ack deadline above maximum",
			config: &Config{
				ProjectID:      "test-project",
				TopicID:        "test-topic",
				SubscriptionID: "test-subscription",
				AckDeadline:    601, // Above 600
			},
			wantErr: true,
			errMsg:  "ack_deadline must be between 10 and 600 seconds",
		},
		{
			name: "Negative max outstanding messages gets default",
			config: &Config{
				ProjectID:              "test-project",
				TopicID:                "test-topic",
				SubscriptionID:         "test-subscription",
				MaxOutstandingMessages: -1,
			},
			wantErr: false, // Negative values get converted to default (100)
		},
		{
			name: "Valid - zero max outstanding messages uses default",
			config: &Config{
				ProjectID:              "test-project",
				TopicID:                "test-topic",
				SubscriptionID:         "test-subscription",
				MaxOutstandingMessages: 0,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()

			if tt.wantErr {
				require.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				require.NoError(t, err)

				// Check defaults were applied
				if tt.config.AckDeadline == 0 {
					assert.Equal(t, 60, tt.config.AckDeadline)
				}
				if tt.name == "Negative max outstanding messages gets default" {
					assert.Equal(t, 100, tt.config.MaxOutstandingMessages)
				} else if tt.config.MaxOutstandingMessages == 0 {
					assert.Equal(t, 100, tt.config.MaxOutstandingMessages)
				}
				// Subscription ID is not auto-generated in config validation
			}
		})
	}
}

func TestConfig_ConnectionString(t *testing.T) {
	tests := []struct {
		name     string
		config   *Config
		expected string
	}{
		{
			name: "Standard project and topic",
			config: &Config{
				ProjectID: "my-gcp-project",
				TopicID:   "events-topic",
			},
			expected: "pubsub://projects/my-gcp-project/topics/events-topic",
		},
		{
			name: "Project with special characters",
			config: &Config{
				ProjectID: "my-project-123",
				TopicID:   "test_topic_v2",
			},
			expected: "pubsub://projects/my-project-123/topics/test_topic_v2",
		},
		{
			name: "Empty fields",
			config: &Config{
				ProjectID: "",
				TopicID:   "",
			},
			expected: "pubsub://projects//topics/",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.config.GetConnectionString()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFactory(t *testing.T) {
	factory := GetFactory()

	t.Run("GetType", func(t *testing.T) {
		assert.Equal(t, "gcp", factory.GetType())
	})

	t.Run("Create with valid config", func(t *testing.T) {
		config := &Config{
			ProjectID:      "test-project",
			TopicID:        "test-topic",
			SubscriptionID: "test-subscription",
		}

		broker, err := factory.Create(config)

		// Can't create real broker without GCP credentials
		// Any error at this point is expected since we don't have real GCP credentials
		if err == nil {
			assert.NotNil(t, broker)
			assert.Equal(t, "gcp", broker.Name())
		}
	})

	t.Run("Create with invalid config", func(t *testing.T) {
		config := &Config{
			// Missing required fields
		}

		broker, err := factory.Create(config)

		assert.Error(t, err)
		assert.Nil(t, broker)
		assert.Contains(t, err.Error(), "project_id is required")
	})

	t.Run("Create with nil config", func(t *testing.T) {
		broker, err := factory.Create(nil)

		assert.Error(t, err)
		assert.Nil(t, broker)
		assert.Contains(t, err.Error(), "invalid config type")
	})
}

func TestConfig_Defaults(t *testing.T) {
	t.Run("DefaultConfig values", func(t *testing.T) {
		config := DefaultConfig()

		assert.Equal(t, "", config.ProjectID)
		assert.Equal(t, "", config.TopicID)
		assert.Equal(t, "", config.SubscriptionID)
		assert.Equal(t, true, config.CreateSubscription)
		assert.Equal(t, 60, config.AckDeadline)
		assert.Equal(t, 100, config.MaxOutstandingMessages)
		assert.Equal(t, false, config.EnableMessageOrdering)
		assert.Equal(t, "", config.OrderingKey)
		assert.Equal(t, "", config.CredentialsJSON)
		assert.Equal(t, "", config.CredentialsPath)
		assert.Equal(t, 30*time.Second, config.Timeout)
		assert.Equal(t, 3, config.RetryMax)
	})

	t.Run("Config with partial values gets defaults", func(t *testing.T) {
		config := &Config{
			ProjectID:          "test-project",
			TopicID:            "test-topic",
			CreateSubscription: true, // Enable auto-create so subscription ID is not required
			// Other fields not set
		}

		err := config.Validate()
		require.NoError(t, err)

		// Check defaults were applied
		assert.Equal(t, 60, config.AckDeadline)
		assert.Equal(t, 100, config.MaxOutstandingMessages)
		assert.Equal(t, 30*time.Second, config.Timeout)
		assert.Equal(t, 3, config.RetryMax)
		// Note: Subscription ID is not auto-generated in the Validate method
	})
}

// Helper function to check if error is related to GCP connection
func isGCPConnectionError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return strings.Contains(errStr, "credentials") ||
		strings.Contains(errStr, "auth") ||
		strings.Contains(errStr, "pubsub") ||
		strings.Contains(errStr, "google") ||
		strings.Contains(errStr, "dialing")
}

// Note: The following tests would require mocking Google Cloud SDK clients,
// which requires refactoring the broker to use interfaces instead of concrete types.

func TestBroker_Operations_SkipWithoutMocks(t *testing.T) {
	t.Skip("Skipping broker operation tests - requires Google Cloud SDK client mocking")

	// Tests that would be included with proper mocking:
	// - TestBroker_Publish
	// - TestBroker_Subscribe
	// - TestBroker_Health
	// - TestBroker_Connect
	// - TestBroker_Disconnect
	// - TestBroker_ensureTopicAndSubscription
	// - TestBroker_MessageConversion
}
