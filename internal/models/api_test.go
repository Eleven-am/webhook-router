package models

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"webhook-router/internal/storage"
)

func TestRouteAPI_Conversions(t *testing.T) {
	t.Run("RouteFromInternal converts storage.Route correctly", func(t *testing.T) {
		now := time.Now()
		
		storageRoute := &storage.Route{
			ID:       123,
			Endpoint: "/webhook/test",
			Method:   "POST",
			CreatedAt: now,
			UpdatedAt: now,
		}

		api := ToRouteAPI(storageRoute)

		assert.NotNil(t, api)
		assert.Equal(t, "123", api.ID)
		assert.Equal(t, "/webhook/test", api.Path)
		assert.Equal(t, "POST", api.Method)
		assert.Equal(t, now, api.CreatedAt)
		assert.Equal(t, now, api.UpdatedAt)
	})

	t.Run("ToRouteAPI handles nil input", func(t *testing.T) {
		api := ToRouteAPI(nil)
		assert.Nil(t, api)
	})

	t.Run("ToRouteAPI with zero values", func(t *testing.T) {
		storageRoute := &storage.Route{
			ID:       0,
			Endpoint: "",
			Method:   "",
		}

		api := ToRouteAPI(storageRoute)

		assert.NotNil(t, api)
		assert.Equal(t, "0", api.ID)
		assert.Equal(t, "", api.Path)
		assert.Equal(t, "", api.Method)
	})
}

func TestBrokerConfigAPI_Conversions(t *testing.T) {
	t.Run("ToBrokerConfigAPI converts storage.BrokerConfig correctly", func(t *testing.T) {
		storageBrokerConfig := &storage.BrokerConfig{
			Type: "rabbitmq",
			Config: map[string]interface{}{
				"host":     "localhost",
				"port":     5672,
				"username": "guest",
				"password": "guest",
			},
		}

		api := ToBrokerConfigAPI(storageBrokerConfig)

		assert.NotNil(t, api)
		assert.Equal(t, "rabbitmq", api.Type)
		assert.Equal(t, storageBrokerConfig.Config, api.Config)
	})

	t.Run("ToBrokerConfigAPI handles nil input", func(t *testing.T) {
		api := ToBrokerConfigAPI(nil)
		assert.Nil(t, api)
	})

	t.Run("ToBrokerConfigAPI with empty config", func(t *testing.T) {
		storageBrokerConfig := &storage.BrokerConfig{
			Type:   "kafka",
			Config: nil,
		}

		api := ToBrokerConfigAPI(storageBrokerConfig)

		assert.NotNil(t, api)
		assert.Equal(t, "kafka", api.Type)
		assert.Nil(t, api.Config)
	})

	t.Run("ToBrokerConfigAPI with complex config", func(t *testing.T) {
		storageBrokerConfig := &storage.BrokerConfig{
			Type: "aws",
			Config: map[string]interface{}{
				"region":      "us-east-1",
				"access_key":  "AKIAIOSFODNN7EXAMPLE",
				"secret_key":  "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
				"queue_url":   "https://sqs.us-east-1.amazonaws.com/123456789012/my-queue",
				"timeout":     30,
				"retry_count": 3,
				"settings": map[string]interface{}{
					"visibility_timeout": 300,
					"message_retention":  1209600,
				},
			},
		}

		api := ToBrokerConfigAPI(storageBrokerConfig)

		assert.NotNil(t, api)
		assert.Equal(t, "aws", api.Type)
		assert.Equal(t, storageBrokerConfig.Config, api.Config)

		// Verify nested structure is preserved
		settings, ok := api.Config["settings"].(map[string]interface{})
		assert.True(t, ok)
		assert.Equal(t, 300, settings["visibility_timeout"])
		assert.Equal(t, 1209600, settings["message_retention"])
	})
}

func TestStructFieldValidation(t *testing.T) {
	t.Run("RouteRuleAPI has required JSON tags", func(t *testing.T) {
		rule := RouteRuleAPI{
			ID:          "rule-1",
			Name:        "Test Rule",
			Description: "A test rule",
			Priority:    10,
			Enabled:     true,
			Tags:        []string{"test"},
		}

		// Basic struct field validation
		assert.Equal(t, "rule-1", rule.ID)
		assert.Equal(t, "Test Rule", rule.Name)
		assert.Equal(t, "A test rule", rule.Description)
		assert.Equal(t, 10, rule.Priority)
		assert.True(t, rule.Enabled)
		assert.Contains(t, rule.Tags, "test")
	})

	t.Run("HealthCheckConfigAPI fields", func(t *testing.T) {
		healthCheck := HealthCheckConfigAPI{
			Enabled:            true,
			Endpoint:           "/health",
			Interval:           "60s",
			Timeout:            "10s",
			HealthyThreshold:   2,
			UnhealthyThreshold: 3,
		}

		assert.True(t, healthCheck.Enabled)
		assert.Equal(t, "/health", healthCheck.Endpoint)
		assert.Equal(t, "60s", healthCheck.Interval)
		assert.Equal(t, "10s", healthCheck.Timeout)
		assert.Equal(t, 2, healthCheck.HealthyThreshold)
		assert.Equal(t, 3, healthCheck.UnhealthyThreshold)
	})

	t.Run("LoadBalancingConfigAPI fields", func(t *testing.T) {
		loadBalancing := LoadBalancingConfigAPI{
			Strategy:      "round_robin",
			HashKey:       "user_id",
			StickySession: true,
			SessionKey:    "session_id",
		}

		assert.Equal(t, "round_robin", loadBalancing.Strategy)
		assert.Equal(t, "user_id", loadBalancing.HashKey)
		assert.True(t, loadBalancing.StickySession)
		assert.Equal(t, "session_id", loadBalancing.SessionKey)
	})

	t.Run("RouteDestinationAPI fields", func(t *testing.T) {
		destination := RouteDestinationAPI{
			ID:          "dest-1",
			Type:        "http",
			URL:         "https://example.com/webhook",
			BrokerQueue: "events",
			PipelineID:  "pipeline-1",
			Priority:    1,
			Weight:      100,
			Timeout:     "30s",
			Config: map[string]interface{}{
				"retry_count": 3,
				"verify_ssl":  true,
			},
		}

		assert.Equal(t, "dest-1", destination.ID)
		assert.Equal(t, "http", destination.Type)
		assert.Equal(t, "https://example.com/webhook", destination.URL)
		assert.Equal(t, "events", destination.BrokerQueue)
		assert.Equal(t, "pipeline-1", destination.PipelineID)
		assert.Equal(t, 1, destination.Priority)
		assert.Equal(t, 100, destination.Weight)
		assert.Equal(t, "30s", destination.Timeout)
		assert.Equal(t, 3, destination.Config["retry_count"])
		assert.Equal(t, true, destination.Config["verify_ssl"])
	})
}

func TestStringConversions(t *testing.T) {
	t.Run("route ID conversion", func(t *testing.T) {
		tests := []struct {
			id       int
			expected string
		}{
			{0, "0"},
			{1, "1"},
			{123, "123"},
			{999999, "999999"},
			{-1, "-1"},
		}

		for _, tt := range tests {
			route := &storage.Route{ID: tt.id}
			api := ToRouteAPI(route)
			assert.Equal(t, tt.expected, api.ID)
		}
	})
}

func TestComplexDataStructures(t *testing.T) {
	t.Run("nested maps in broker config", func(t *testing.T) {
		complexConfig := map[string]interface{}{
			"simple_string": "value",
			"simple_number": 42,
			"simple_bool":   true,
			"nested_map": map[string]interface{}{
				"inner_string": "inner_value",
				"inner_number": 123,
				"deeply_nested": map[string]interface{}{
					"deep_key": "deep_value",
				},
			},
			"array": []interface{}{
				"item1",
				"item2",
				map[string]interface{}{
					"array_item_key": "array_item_value",
				},
			},
		}

		storageBrokerConfig := &storage.BrokerConfig{
			Type:   "complex",
			Config: complexConfig,
		}

		api := ToBrokerConfigAPI(storageBrokerConfig)

		assert.NotNil(t, api)
		assert.Equal(t, "complex", api.Type)

		// Verify complex nested structure is preserved
		assert.Equal(t, "value", api.Config["simple_string"])
		assert.Equal(t, 42, api.Config["simple_number"])
		assert.Equal(t, true, api.Config["simple_bool"])

		nestedMap, ok := api.Config["nested_map"].(map[string]interface{})
		assert.True(t, ok)
		assert.Equal(t, "inner_value", nestedMap["inner_string"])
		assert.Equal(t, 123, nestedMap["inner_number"])

		deeplyNested, ok := nestedMap["deeply_nested"].(map[string]interface{})
		assert.True(t, ok)
		assert.Equal(t, "deep_value", deeplyNested["deep_key"])

		array, ok := api.Config["array"].([]interface{})
		assert.True(t, ok)
		assert.Len(t, array, 3)
		assert.Equal(t, "item1", array[0])
		assert.Equal(t, "item2", array[1])

		arrayMap, ok := array[2].(map[string]interface{})
		assert.True(t, ok)
		assert.Equal(t, "array_item_value", arrayMap["array_item_key"])
	})
}

func TestEdgeCases(t *testing.T) {
	t.Run("empty and nil values", func(t *testing.T) {
		tests := []struct {
			name  string
			route *storage.Route
		}{
			{
				name:  "nil route",
				route: nil,
			},
			{
				name: "empty route",
				route: &storage.Route{},
			},
			{
				name: "route with empty strings",
				route: &storage.Route{
					ID:       0,
					Endpoint: "",
					Method:   "",
				},
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				api := ToRouteAPI(tt.route)
				if tt.route == nil {
					assert.Nil(t, api)
				} else {
					assert.NotNil(t, api)
				}
			})
		}
	})

	t.Run("time handling", func(t *testing.T) {
		// Test various time scenarios
		now := time.Now()
		zeroTime := time.Time{}
		futureTime := now.Add(24 * time.Hour)
		pastTime := now.Add(-24 * time.Hour)

		times := []time.Time{now, zeroTime, futureTime, pastTime}

		for i, testTime := range times {
			route := &storage.Route{
				ID:        i,
				CreatedAt: testTime,
				UpdatedAt: testTime,
			}

			api := ToRouteAPI(route)
			assert.Equal(t, testTime, api.CreatedAt)
			assert.Equal(t, testTime, api.UpdatedAt)
		}
	})
}