package gcp

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfigValidation(t *testing.T) {
	tests := []struct {
		name      string
		config    *Config
		wantError bool
		errorMsg  string
	}{
		{
			name: "valid config",
			config: &Config{
				ProjectID:      "test-project",
				TopicID:        "test-topic",
				SubscriptionID: "test-subscription",
				AckDeadline:    60,
			},
			wantError: false,
		},
		{
			name: "missing project ID",
			config: &Config{
				TopicID:        "test-topic",
				SubscriptionID: "test-subscription",
			},
			wantError: true,
			errorMsg:  "project_id",
		},
		{
			name: "missing topic ID",
			config: &Config{
				ProjectID:      "test-project",
				SubscriptionID: "test-subscription",
			},
			wantError: true,
			errorMsg:  "topic_id",
		},
		{
			name: "missing subscription ID with auto-create disabled",
			config: &Config{
				ProjectID:          "test-project",
				TopicID:            "test-topic",
				CreateSubscription: false,
			},
			wantError: true,
			errorMsg:  "subscription_id",
		},
		{
			name: "valid config with auto-create enabled",
			config: &Config{
				ProjectID:          "test-project",
				TopicID:            "test-topic",
				CreateSubscription: true,
			},
			wantError: false,
		},
		{
			name: "invalid ack deadline too low",
			config: &Config{
				ProjectID:      "test-project",
				TopicID:        "test-topic",
				SubscriptionID: "test-subscription",
				AckDeadline:    5, // Too low
			},
			wantError: true,
			errorMsg:  "ack_deadline must be between 10 and 600 seconds",
		},
		{
			name: "invalid ack deadline too high",
			config: &Config{
				ProjectID:      "test-project",
				TopicID:        "test-topic",
				SubscriptionID: "test-subscription",
				AckDeadline:    700, // Too high
			},
			wantError: true,
			errorMsg:  "ack_deadline must be between 10 and 600 seconds",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.wantError {
				require.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	// Check defaults
	assert.Equal(t, "", config.ProjectID)
	assert.Equal(t, "", config.TopicID)
	assert.Equal(t, true, config.CreateSubscription)
	assert.Equal(t, 60, config.AckDeadline)
	assert.Equal(t, 100, config.MaxOutstandingMessages)
	assert.Equal(t, false, config.EnableMessageOrdering)
	assert.Equal(t, 30*time.Second, config.Timeout)
	assert.Equal(t, 3, config.RetryMax)
}

func TestConfigGetType(t *testing.T) {
	config := &Config{}
	assert.Equal(t, "gcp", config.GetType())
}

func TestConfigGetConnectionString(t *testing.T) {
	config := &Config{
		ProjectID: "my-project",
		TopicID:   "my-topic",
	}

	expected := "pubsub://projects/my-project/topics/my-topic"
	assert.Equal(t, expected, config.GetConnectionString())
}
