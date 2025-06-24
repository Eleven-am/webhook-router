package handlers

import (
	"strings"
)

// SensitiveFieldPattern defines patterns for sensitive configuration fields
// These fields should never be exposed in API responses
var SensitiveFieldPatterns = []string{
	// OAuth2 and authentication tokens
	"oauth2_token",
	"access_token",
	"refresh_token",
	"id_token",
	"client_secret",
	"api_key",
	"apikey",

	// Database and service credentials
	"password",
	"passwd",
	"secret",
	"key",
	"token",
	"credential",

	// Cloud provider credentials
	"aws_secret_access_key",
	"secret_access_key",
	"private_key",
	"service_account_key",

	// Connection strings with embedded credentials
	"connection_string",
	"database_url",
	"redis_password",
	"rabbitmq_url",

	// Webhook and signature secrets
	"signature_secret",
	"webhook_secret",
	"signing_key",

	// Encryption keys
	"encryption_key",
	"config_encryption_key",
	"jwt_secret",
}

// FilterSensitiveSettings removes sensitive configuration from settings map
// This prevents API exposure of credentials, tokens, and secrets
func FilterSensitiveSettings(settings map[string]string) map[string]string {
	if settings == nil {
		return nil
	}

	filtered := make(map[string]string)

	for key, value := range settings {
		if !isSensitiveField(key) {
			filtered[key] = value
		} else {
			// For debugging, include the key but mask the value
			filtered[key] = "[REDACTED]"
		}
	}

	return filtered
}

// FilterSensitiveFields removes sensitive fields from any map
// Generic function for filtering any configuration map
func FilterSensitiveFields(data map[string]interface{}) map[string]interface{} {
	if data == nil {
		return nil
	}

	filtered := make(map[string]interface{})

	for key, value := range data {
		if !isSensitiveField(key) {
			// Recursively filter nested maps
			if nestedMap, ok := value.(map[string]interface{}); ok {
				filtered[key] = FilterSensitiveFields(nestedMap)
			} else {
				filtered[key] = value
			}
		} else {
			// For debugging, include the key but mask the value
			filtered[key] = "[REDACTED]"
		}
	}

	return filtered
}

// FilterSensitiveString removes sensitive fields from string maps
func FilterSensitiveString(data map[string]string) map[string]string {
	if data == nil {
		return nil
	}

	filtered := make(map[string]string)

	for key, value := range data {
		if !isSensitiveField(key) {
			filtered[key] = value
		} else {
			// For debugging, include the key but mask the value
			filtered[key] = "[REDACTED]"
		}
	}

	return filtered
}

// isSensitiveField checks if a field name contains sensitive patterns
func isSensitiveField(fieldName string) bool {
	fieldLower := strings.ToLower(fieldName)

	for _, pattern := range SensitiveFieldPatterns {
		if strings.Contains(fieldLower, pattern) {
			return true
		}
	}

	return false
}

// GetSafeSettings returns only non-sensitive settings for API responses
// This is the secure version of GetAllSettings for API exposure
func GetSafeSettings(allSettings map[string]string) map[string]string {
	safeSettings := make(map[string]string)

	// Define explicitly safe settings that can be exposed
	safeSettingPrefixes := []string{
		"app_name",
		"app_version",
		"environment",
		"timezone",
		"language",
		"theme",
		"ui_",
		"display_",
		"notification_",
		"email_from", // email FROM address is safe
		"smtp_host",  // SMTP host is safe (not password)
		"smtp_port",  // SMTP port is safe
		"default_",
		"max_",
		"min_",
		"timeout_",
		"interval_",
		"enabled_",
		"disabled_",
		"feature_",
		"debug_level",
		"log_level",
	}

	for key, value := range allSettings {
		keyLower := strings.ToLower(key)

		// Check if it's explicitly safe
		isSafe := false
		for _, prefix := range safeSettingPrefixes {
			if strings.HasPrefix(keyLower, prefix) {
				isSafe = true
				break
			}
		}

		// Double-check it's not sensitive
		if isSafe && !isSensitiveField(key) {
			safeSettings[key] = value
		}
	}

	return safeSettings
}

// FilterBrokerConfig removes sensitive fields from broker configuration
// This prevents API exposure of AWS keys, passwords, connection strings, etc.
func FilterBrokerConfig(broker interface{}) interface{} {
	// Handle different broker config types
	switch b := broker.(type) {
	case map[string]interface{}:
		return FilterSensitiveFields(b)
	case []interface{}:
		// Handle array of brokers
		filtered := make([]interface{}, len(b))
		for i, item := range b {
			filtered[i] = FilterBrokerConfig(item)
		}
		return filtered
	default:
		return broker
	}
}

// FilterBrokerConfigs removes sensitive fields from multiple broker configurations
func FilterBrokerConfigs(brokers interface{}) interface{} {
	return FilterBrokerConfig(brokers)
}
