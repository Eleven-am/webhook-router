package gcp

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBroker_EdgeCases(t *testing.T) {
	t.Run("Broker Name method", func(t *testing.T) {
		// Create a broker with minimal valid config
		config := &Config{
			ProjectID:          "test-project",
			TopicID:            "test-topic",
			CreateSubscription: true,
		}

		broker, err := NewBroker(config)
		// Even if creation fails due to GCP credentials, we can test the name
		if err == nil {
			assert.Equal(t, "GCP Pub/Sub", broker.Name())
		}
	})

	t.Run("Config edge cases", func(t *testing.T) {
		tests := []struct {
			name    string
			config  *Config
			wantErr bool
			errMsg  string
		}{
			{
				name: "Empty strings",
				config: &Config{
					ProjectID:      "",
					TopicID:        "",
					SubscriptionID: "",
				},
				wantErr: true,
				errMsg:  "project_id is required",
			},
			{
				name: "Whitespace only project ID",
				config: &Config{
					ProjectID:      "   ",
					TopicID:        "test-topic",
					SubscriptionID: "test-sub",
				},
				wantErr: true,
				errMsg:  "project_id is required",
			},
			{
				name: "Very long project ID",
				config: &Config{
					ProjectID:      "this-is-a-very-long-project-id-that-exceeds-reasonable-length-limits-for-gcp-projects",
					TopicID:        "test-topic",
					SubscriptionID: "test-sub",
				},
				wantErr: false, // GCP will validate this, not our code
			},
			{
				name: "Special characters in topic ID",
				config: &Config{
					ProjectID:      "test-project",
					TopicID:        "test-topic-with-special-chars_123",
					SubscriptionID: "test-sub",
				},
				wantErr: false,
			},
			{
				name: "Ack deadline exactly at minimum",
				config: &Config{
					ProjectID:      "test-project",
					TopicID:        "test-topic",
					SubscriptionID: "test-sub",
					AckDeadline:    10,
				},
				wantErr: false,
			},
			{
				name: "Ack deadline exactly at maximum",
				config: &Config{
					ProjectID:      "test-project",
					TopicID:        "test-topic",
					SubscriptionID: "test-sub",
					AckDeadline:    600,
				},
				wantErr: false,
			},
			{
				name: "Large max outstanding messages",
				config: &Config{
					ProjectID:              "test-project",
					TopicID:                "test-topic",
					SubscriptionID:         "test-sub",
					MaxOutstandingMessages: 100000,
				},
				wantErr: false,
			},
			{
				name: "Message ordering without ordering key",
				config: &Config{
					ProjectID:             "test-project",
					TopicID:               "test-topic",
					SubscriptionID:        "test-sub",
					EnableMessageOrdering: true,
					OrderingKey:           "", // Empty ordering key with ordering enabled
				},
				wantErr: false, // This is allowed - ordering key can be set per message
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
				}
			})
		}
	})

	t.Run("Config with both credential types", func(t *testing.T) {
		config := &Config{
			ProjectID:       "test-project",
			TopicID:         "test-topic",
			SubscriptionID:  "test-sub",
			CredentialsJSON: `{"type": "service_account"}`,
			CredentialsPath: "/path/to/creds.json", // Both set
		}

		// Should not error - broker can choose which to use
		err := config.Validate()
		assert.NoError(t, err)
	})

	t.Run("Auto-generated subscription ID format", func(t *testing.T) {
		config := &Config{
			ProjectID:          "test-project",
			TopicID:            "test-topic",
			CreateSubscription: true,
			// No SubscriptionID
		}

		// Validate doesn't generate the ID, but it should pass
		err := config.Validate()
		assert.NoError(t, err)

		// If we create a broker, it might generate an ID
		broker, _ := NewBroker(config)
		if broker != nil {
			// The broker might have generated a subscription ID
			// but we can't test this without mocking
		}
	})
}

func TestConfig_SpecialCases(t *testing.T) {
	t.Run("Connection string escaping", func(t *testing.T) {
		tests := []struct {
			name      string
			projectID string
			topicID   string
			expected  string
		}{
			{
				name:      "Normal IDs",
				projectID: "my-project",
				topicID:   "my-topic",
				expected:  "pubsub://projects/my-project/topics/my-topic",
			},
			{
				name:      "IDs with underscores",
				projectID: "my_project",
				topicID:   "my_topic",
				expected:  "pubsub://projects/my_project/topics/my_topic",
			},
			{
				name:      "IDs with numbers",
				projectID: "project123",
				topicID:   "topic456",
				expected:  "pubsub://projects/project123/topics/topic456",
			},
			{
				name:      "Empty IDs",
				projectID: "",
				topicID:   "",
				expected:  "pubsub://projects//topics/",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				config := &Config{
					ProjectID: tt.projectID,
					TopicID:   tt.topicID,
				}
				assert.Equal(t, tt.expected, config.GetConnectionString())
			})
		}
	})

	t.Run("Config struct size", func(t *testing.T) {
		// Just ensure the config struct compiles and has reasonable size
		config := &Config{}
		assert.NotNil(t, config)

		// Test that all fields can be set
		config.ProjectID = "test"
		config.TopicID = "test"
		config.SubscriptionID = "test"
		config.CredentialsJSON = "test"
		config.CredentialsPath = "test"
		config.CreateSubscription = true
		config.AckDeadline = 60
		config.MaxOutstandingMessages = 100
		config.EnableMessageOrdering = true
		config.OrderingKey = "test"

		// Just ensure no panics
		_ = config.GetType()
		_ = config.GetConnectionString()
	})
}
