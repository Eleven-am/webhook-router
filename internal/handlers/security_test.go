package handlers

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFilterSensitiveSettings(t *testing.T) {
	settings := map[string]string{
		"app_name":              "Webhook Router",
		"oauth2_token_google":   "ya29.abc123...",
		"aws_secret_access_key": "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
		"redis_password":        "super-secret-password",
		"jwt_secret":            "my-jwt-secret-key",
		"api_key":               "api-key-123",
		"environment":           "production",
		"max_connections":       "100",
	}

	filtered := FilterSensitiveSettings(settings)

	// Should keep safe settings
	assert.Equal(t, "Webhook Router", filtered["app_name"])
	assert.Equal(t, "production", filtered["environment"])
	assert.Equal(t, "100", filtered["max_connections"])

	// Should redact sensitive settings
	assert.Equal(t, "[REDACTED]", filtered["oauth2_token_google"])
	assert.Equal(t, "[REDACTED]", filtered["aws_secret_access_key"])
	assert.Equal(t, "[REDACTED]", filtered["redis_password"])
	assert.Equal(t, "[REDACTED]", filtered["jwt_secret"])
	assert.Equal(t, "[REDACTED]", filtered["api_key"])
}

func TestGetSafeSettings(t *testing.T) {
	allSettings := map[string]string{
		"app_name":              "Webhook Router",
		"oauth2_token_google":   "ya29.abc123...",
		"aws_secret_access_key": "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
		"environment":           "production",
		"ui_theme":              "dark",
		"feature_analytics":     "enabled",
		"timeout_duration":      "30s",
		"secret_signing_key":    "secret-key-123",
	}

	safeSettings := GetSafeSettings(allSettings)

	// Should include explicitly safe settings
	assert.Equal(t, "Webhook Router", safeSettings["app_name"])
	assert.Equal(t, "production", safeSettings["environment"])
	assert.Equal(t, "dark", safeSettings["ui_theme"])
	assert.Equal(t, "enabled", safeSettings["feature_analytics"])
	assert.Equal(t, "30s", safeSettings["timeout_duration"])

	// Should exclude sensitive settings completely
	assert.NotContains(t, safeSettings, "oauth2_token_google")
	assert.NotContains(t, safeSettings, "aws_secret_access_key")
	assert.NotContains(t, safeSettings, "secret_signing_key")
}

func TestFilterSensitiveFields(t *testing.T) {
	brokerConfig := map[string]interface{}{
		"id":     1,
		"name":   "AWS Broker",
		"type":   "aws",
		"active": true,
		"config": map[string]interface{}{
			"region":            "us-east-1",
			"access_key_id":     "AKIAIOSFODNN7EXAMPLE",
			"secret_access_key": "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
			"queue_url":         "https://sqs.us-east-1.amazonaws.com/123456789012/test-queue",
		},
	}

	filtered := FilterSensitiveFields(brokerConfig)

	// Should keep safe fields
	assert.Equal(t, 1, filtered["id"])
	assert.Equal(t, "AWS Broker", filtered["name"])
	assert.Equal(t, "aws", filtered["type"])
	assert.Equal(t, true, filtered["active"])

	// Should filter nested config
	config := filtered["config"].(map[string]interface{})
	assert.Equal(t, "us-east-1", config["region"])
	assert.Equal(t, "https://sqs.us-east-1.amazonaws.com/123456789012/test-queue", config["queue_url"])

	// Should redact sensitive fields
	assert.Equal(t, "[REDACTED]", config["access_key_id"])
	assert.Equal(t, "[REDACTED]", config["secret_access_key"])
}

func TestIsSensitiveField(t *testing.T) {
	testCases := []struct {
		field     string
		sensitive bool
	}{
		{"password", true},
		{"secret", true},
		{"token", true},
		{"api_key", true},
		{"apikey", true},
		{"oauth2_token", true},
		{"aws_secret_access_key", true},
		{"client_secret", true},
		{"private_key", true},
		{"encryption_key", true},
		{"jwt_secret", true},
		{"signature_secret", true},

		// Safe fields
		{"name", false},
		{"type", false},
		{"id", false},
		{"region", false},
		{"url", false},
		{"environment", false},
		{"app_name", false},
		{"timeout", false},
	}

	for _, tc := range testCases {
		t.Run(tc.field, func(t *testing.T) {
			result := isSensitiveField(tc.field)
			assert.Equal(t, tc.sensitive, result, "Field %s should be sensitive=%t", tc.field, tc.sensitive)
		})
	}
}

func TestFilterBrokerConfig(t *testing.T) {
	// Test single broker
	broker := map[string]interface{}{
		"id":   1,
		"name": "Test Broker",
		"config": map[string]interface{}{
			"password": "secret123",
			"host":     "localhost",
		},
	}

	filtered := FilterBrokerConfig(broker)
	brokerMap := filtered.(map[string]interface{})
	configMap := brokerMap["config"].(map[string]interface{})

	assert.Equal(t, "[REDACTED]", configMap["password"])
	assert.Equal(t, "localhost", configMap["host"])

	// Test array of brokers
	brokers := []interface{}{broker}
	filteredArray := FilterBrokerConfig(brokers)
	brokerArray := filteredArray.([]interface{})

	assert.Len(t, brokerArray, 1)
	firstBroker := brokerArray[0].(map[string]interface{})
	firstConfig := firstBroker["config"].(map[string]interface{})
	assert.Equal(t, "[REDACTED]", firstConfig["password"])
}
