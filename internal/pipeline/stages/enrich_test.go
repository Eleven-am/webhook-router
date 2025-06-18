package stages

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"webhook-router/internal/pipeline"
)

func TestEnrichmentStage_Configure(t *testing.T) {
	tests := []struct {
		name        string
		config      map[string]interface{}
		expectError bool
		errorMsg    string
	}{
		{
			name: "static enrichment",
			config: map[string]interface{}{
				"enrichments": []map[string]interface{}{
					{
						"name":         "add_status",
						"type":         "static",
						"target_field": "status",
						"static_value": "processed",
					},
				},
			},
			expectError: false,
		},
		{
			name: "lookup enrichment",
			config: map[string]interface{}{
				"enrichments": []map[string]interface{}{
					{
						"name":         "country_lookup",
						"type":         "lookup",
						"source_field": "country_code",
						"target_field": "country_name",
						"lookup_table": map[string]interface{}{
							"US": "United States",
							"UK": "United Kingdom",
							"CA": "Canada",
						},
						"default_value": "Unknown",
					},
				},
			},
			expectError: false,
		},
		{
			name: "function enrichment",
			config: map[string]interface{}{
				"enrichments": []map[string]interface{}{
					{
						"name":         "timestamp",
						"type":         "function",
						"target_field": "processed_at",
					},
				},
			},
			expectError: false,
		},
		{
			name: "http enrichment",
			config: map[string]interface{}{
				"enrichments": []map[string]interface{}{
					{
						"name":         "user_data",
						"type":         "http",
						"source_field": "user_id",
						"target_field": "user_details",
						"http_config": map[string]interface{}{
							"url":     "https://api.example.com/users/{user_id}",
							"method":  "GET",
							"timeout": 5,
						},
					},
				},
			},
			expectError: false,
		},
		{
			name: "parallel enrichments",
			config: map[string]interface{}{
				"parallel": true,
				"enrichments": []map[string]interface{}{
					{
						"name":         "status",
						"type":         "static",
						"target_field": "status",
						"static_value": "processed",
					},
					{
						"name":         "timestamp",
						"type":         "function",
						"target_field": "processed_at",
					},
				},
			},
			expectError: false,
		},
		{
			name: "enrichment with condition",
			config: map[string]interface{}{
				"enrichments": []map[string]interface{}{
					{
						"name":         "premium_status",
						"type":         "static",
						"target_field": "is_premium",
						"static_value": true,
						"condition":    "premium", // Only apply if 'premium' field exists
					},
				},
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stage := &EnrichmentStage{
				name: "test-enrichment",
			}

			err := stage.Configure(tt.config)
			if tt.expectError {
				assert.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err)
				assert.True(t, stage.compiled)

				// Test validation after configuration
				err = stage.Validate()
				assert.NoError(t, err)
			}
		})
	}
}

func TestEnrichmentStage_Validate(t *testing.T) {
	tests := []struct {
		name        string
		setupStage  func() *EnrichmentStage
		expectError bool
		errorMsg    string
	}{
		{
			name: "not configured",
			setupStage: func() *EnrichmentStage {
				return &EnrichmentStage{name: "test-validation"}
			},
			expectError: true,
			errorMsg:    "stage not configured",
		},
		{
			name: "empty enrichments",
			setupStage: func() *EnrichmentStage {
				stage := &EnrichmentStage{name: "test-validation"}
				_ = stage.Configure(map[string]interface{}{
					"enrichments": []map[string]interface{}{},
				})
				return stage
			},
			expectError: true,
			errorMsg:    "enrichment stage requires at least one enrichment",
		},
		{
			name: "invalid enrichment type",
			setupStage: func() *EnrichmentStage {
				stage := &EnrichmentStage{name: "test-validation"}
				_ = stage.Configure(map[string]interface{}{
					"enrichments": []map[string]interface{}{
						{
							"name":         "invalid",
							"type":         "invalid_type",
							"target_field": "result",
						},
					},
				})
				return stage
			},
			expectError: true,
			errorMsg:    "invalid type",
		},
		{
			name: "missing target field",
			setupStage: func() *EnrichmentStage {
				stage := &EnrichmentStage{name: "test-validation"}
				_ = stage.Configure(map[string]interface{}{
					"enrichments": []map[string]interface{}{
						{
							"name": "missing_target",
							"type": "static",
						},
					},
				})
				return stage
			},
			expectError: true,
			errorMsg:    "requires target_field",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stage := tt.setupStage()
			err := stage.Validate()
			
			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestEnrichmentStage_Process(t *testing.T) {
	tests := []struct {
		name        string
		config      map[string]interface{}
		inputData   *pipeline.Data
		expectError bool
		errorMsg    string
		validate    func(t *testing.T, result *pipeline.StageResult)
	}{
		{
			name: "static enrichment",
			config: map[string]interface{}{
				"enrichments": []map[string]interface{}{
					{
						"name":         "add_status",
						"type":         "static",
						"target_field": "status",
						"static_value": "processed",
					},
				},
			},
			inputData: &pipeline.Data{
				ID:   "test-1",
				Body: []byte(`{"id": 123, "name": "John"}`),
			},
			expectError: false,
			validate: func(t *testing.T, result *pipeline.StageResult) {
				assert.True(t, result.Success)
				var output map[string]interface{}
				err := json.Unmarshal(result.Data.Body, &output)
				require.NoError(t, err)

				assert.Equal(t, float64(123), output["id"])
				assert.Equal(t, "John", output["name"])
				assert.Equal(t, "processed", output["status"])
			},
		},
		{
			name: "lookup enrichment with match",
			config: map[string]interface{}{
				"enrichments": []map[string]interface{}{
					{
						"name":         "country_lookup",
						"type":         "lookup",
						"source_field": "country_code",
						"target_field": "country_name",
						"lookup_table": map[string]interface{}{
							"US": "United States",
							"UK": "United Kingdom",
						},
						"default_value": "Unknown",
					},
				},
			},
			inputData: &pipeline.Data{
				ID:   "test-2",
				Body: []byte(`{"country_code": "US"}`),
			},
			expectError: false,
			validate: func(t *testing.T, result *pipeline.StageResult) {
				var output map[string]interface{}
				err := json.Unmarshal(result.Data.Body, &output)
				require.NoError(t, err)

				assert.Equal(t, "US", output["country_code"])
				assert.Equal(t, "United States", output["country_name"])
			},
		},
		{
			name: "lookup enrichment with default",
			config: map[string]interface{}{
				"enrichments": []map[string]interface{}{
					{
						"name":         "country_lookup",
						"type":         "lookup",
						"source_field": "country_code",
						"target_field": "country_name",
						"lookup_table": map[string]interface{}{
							"US": "United States",
						},
						"default_value": "Unknown Country",
					},
				},
			},
			inputData: &pipeline.Data{
				ID:   "test-3",
				Body: []byte(`{"country_code": "XX"}`),
			},
			expectError: false,
			validate: func(t *testing.T, result *pipeline.StageResult) {
				var output map[string]interface{}
				err := json.Unmarshal(result.Data.Body, &output)
				require.NoError(t, err)

				assert.Equal(t, "XX", output["country_code"])
				assert.Equal(t, "Unknown Country", output["country_name"])
			},
		},
		{
			name: "function enrichment - timestamp",
			config: map[string]interface{}{
				"enrichments": []map[string]interface{}{
					{
						"name":         "timestamp",
						"type":         "function",
						"target_field": "processed_at",
					},
				},
			},
			inputData: &pipeline.Data{
				ID:   "test-4",
				Body: []byte(`{"id": 123}`),
			},
			expectError: false,
			validate: func(t *testing.T, result *pipeline.StageResult) {
				var output map[string]interface{}
				err := json.Unmarshal(result.Data.Body, &output)
				require.NoError(t, err)

				assert.Equal(t, float64(123), output["id"])
				// Timestamp should be a number
				timestamp, ok := output["processed_at"].(float64)
				assert.True(t, ok)
				assert.Greater(t, timestamp, float64(0))
			},
		},
		{
			name: "conditional enrichment - condition met",
			config: map[string]interface{}{
				"enrichments": []map[string]interface{}{
					{
						"name":         "premium_status",
						"type":         "static",
						"target_field": "is_premium",
						"static_value": true,
						"condition":    "premium",
					},
				},
			},
			inputData: &pipeline.Data{
				ID:   "test-5",
				Body: []byte(`{"premium": true, "name": "John"}`),
			},
			expectError: false,
			validate: func(t *testing.T, result *pipeline.StageResult) {
				var output map[string]interface{}
				err := json.Unmarshal(result.Data.Body, &output)
				require.NoError(t, err)

				assert.Equal(t, true, output["premium"])
				assert.Equal(t, "John", output["name"])
				assert.Equal(t, true, output["is_premium"])
			},
		},
		{
			name: "conditional enrichment - condition not met",
			config: map[string]interface{}{
				"enrichments": []map[string]interface{}{
					{
						"name":         "premium_status",
						"type":         "static",
						"target_field": "is_premium",
						"static_value": true,
						"condition":    "premium",
					},
				},
			},
			inputData: &pipeline.Data{
				ID:   "test-6",
				Body: []byte(`{"name": "John"}`),
			},
			expectError: false,
			validate: func(t *testing.T, result *pipeline.StageResult) {
				var output map[string]interface{}
				err := json.Unmarshal(result.Data.Body, &output)
				require.NoError(t, err)

				assert.Equal(t, "John", output["name"])
				// is_premium should not be added
				_, exists := output["is_premium"]
				assert.False(t, exists)
			},
		},
		{
			name: "multiple enrichments sequential",
			config: map[string]interface{}{
				"parallel": false,
				"enrichments": []map[string]interface{}{
					{
						"name":         "add_status",
						"type":         "static",
						"target_field": "status",
						"static_value": "processed",
					},
					{
						"name":         "timestamp",
						"type":         "function",
						"target_field": "processed_at",
					},
				},
			},
			inputData: &pipeline.Data{
				ID:   "test-7",
				Body: []byte(`{"id": 123}`),
			},
			expectError: false,
			validate: func(t *testing.T, result *pipeline.StageResult) {
				var output map[string]interface{}
				err := json.Unmarshal(result.Data.Body, &output)
				require.NoError(t, err)

				assert.Equal(t, float64(123), output["id"])
				assert.Equal(t, "processed", output["status"])
				assert.NotNil(t, output["processed_at"])
				assert.Equal(t, 2, result.Metadata["enrichments_applied"])
			},
		},
		{
			name: "invalid JSON input",
			config: map[string]interface{}{
				"enrichments": []map[string]interface{}{
					{
						"name":         "add_status",
						"type":         "static",
						"target_field": "status",
						"static_value": "processed",
					},
				},
			},
			inputData: &pipeline.Data{
				ID:   "test-8",
				Body: []byte(`{invalid json`),
			},
			expectError: false, // Process doesn't return error, but result will have Success=false
			validate: func(t *testing.T, result *pipeline.StageResult) {
				assert.False(t, result.Success)
				assert.Contains(t, result.Error, "failed to parse JSON")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stage := &EnrichmentStage{
				name: "test-enrichment",
			}

			err := stage.Configure(tt.config)
			require.NoError(t, err)

			result, err := stage.Process(context.Background(), tt.inputData)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err)
				if tt.validate != nil {
					tt.validate(t, result)
				}
			}
		})
	}
}

func TestEnrichmentStage_ParallelExecution(t *testing.T) {
	// Test parallel enrichment execution
	config := map[string]interface{}{
		"parallel": true,
		"enrichments": []map[string]interface{}{
			{
				"name":         "status1",
				"type":         "static",
				"target_field": "status1",
				"static_value": "processed1",
			},
			{
				"name":         "status2",
				"type":         "static",
				"target_field": "status2",
				"static_value": "processed2",
			},
			{
				"name":         "timestamp",
				"type":         "function",
				"target_field": "processed_at",
			},
		},
	}

	stage := &EnrichmentStage{
		name: "test-parallel-enrichment",
	}

	err := stage.Configure(config)
	require.NoError(t, err)

	inputData := &pipeline.Data{
		ID:   "test-parallel",
		Body: []byte(`{"id": 123}`),
	}

	result, err := stage.Process(context.Background(), inputData)
	assert.NoError(t, err)
	assert.True(t, result.Success)

	var output map[string]interface{}
	err = json.Unmarshal(result.Data.Body, &output)
	require.NoError(t, err)

	// All enrichments should be applied
	assert.Equal(t, float64(123), output["id"])
	assert.Equal(t, "processed1", output["status1"])
	assert.Equal(t, "processed2", output["status2"])
	assert.NotNil(t, output["processed_at"])
	assert.Equal(t, 3, result.Metadata["enrichments_applied"])
}

func TestEnrichmentStage_ConcurrentAccess(t *testing.T) {
	// Test concurrent access to the same stage
	stage := &EnrichmentStage{
		name: "test-concurrent-enrichment",
	}

	config := map[string]interface{}{
		"enrichments": []map[string]interface{}{
			{
				"name":         "add_status",
				"type":         "static",
				"target_field": "status",
				"static_value": "processed",
			},
		},
	}

	err := stage.Configure(config)
	require.NoError(t, err)

	const numRoutines = 10
	var wg sync.WaitGroup
	results := make([]*pipeline.StageResult, numRoutines)
	errors := make([]error, numRoutines)

	// Run multiple goroutines concurrently
	for i := 0; i < numRoutines; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()

			inputData := &pipeline.Data{
				ID:   fmt.Sprintf("test-%d", index),
				Body: []byte(fmt.Sprintf(`{"id": %d}`, index)),
			}

			result, err := stage.Process(context.Background(), inputData)
			results[index] = result
			errors[index] = err
		}(i)
	}

	wg.Wait()

	// Verify all executions succeeded
	for i := 0; i < numRoutines; i++ {
		assert.NoError(t, errors[i], "Execution %d should not error", i)
		assert.True(t, results[i].Success, "Execution %d should succeed", i)

		var output map[string]interface{}
		err := json.Unmarshal(results[i].Data.Body, &output)
		require.NoError(t, err)

		assert.Equal(t, float64(i), output["id"])
		assert.Equal(t, "processed", output["status"])
	}
}

func TestAggregationStage_Configure(t *testing.T) {
	tests := []struct {
		name        string
		config      map[string]interface{}
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid count aggregation",
			config: map[string]interface{}{
				"window_size": 60,
				"window_type": "fixed",
				"aggregations": []map[string]interface{}{
					{
						"name":         "count",
						"target_field": "total_count",
					},
				},
			},
			expectError: false,
		},
		{
			name: "valid sum aggregation",
			config: map[string]interface{}{
				"window_size": 300,
				"window_type": "sliding",
				"group_by":    []string{"user_id"},
				"aggregations": []map[string]interface{}{
					{
						"name":         "sum",
						"source_field": "amount",
						"target_field": "total_amount",
					},
				},
			},
			expectError: false,
		},
		{
			name: "valid average aggregation",
			config: map[string]interface{}{
				"window_size": 120,
				"aggregations": []map[string]interface{}{
					{
						"name":         "avg",
						"source_field": "response_time",
						"target_field": "avg_response_time",
					},
				},
				"output_field":   "metrics",
				"flush_interval": 30,
			},
			expectError: false,
		},
		{
			name: "multiple aggregations",
			config: map[string]interface{}{
				"window_size": 60,
				"group_by":    []string{"endpoint", "status_code"},
				"aggregations": []map[string]interface{}{
					{
						"name":         "count",
						"target_field": "request_count",
					},
					{
						"name":         "avg",
						"source_field": "response_time",
						"target_field": "avg_response_time",
					},
				},
			},
			expectError: false,
		},
		{
			name: "invalid window size",
			config: map[string]interface{}{
				"window_size": 0,
				"aggregations": []map[string]interface{}{
					{
						"name":         "count",
						"target_field": "total_count",
					},
				},
			},
			expectError: true,
			errorMsg:    "window_size must be positive",
		},
		{
			name: "no aggregations",
			config: map[string]interface{}{
				"window_size":  60,
				"aggregations": []map[string]interface{}{},
			},
			expectError: true,
			errorMsg:    "requires at least one aggregation function",
		},
		{
			name: "invalid window type",
			config: map[string]interface{}{
				"window_size": 60,
				"window_type": "invalid",
				"aggregations": []map[string]interface{}{
					{
						"name":         "count",
						"target_field": "total_count",
					},
				},
			},
			expectError: true,
			errorMsg:    "invalid window_type",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stage := &AggregationStage{
				name: "test-aggregation",
			}

			err := stage.Configure(tt.config)
			if tt.expectError {
				assert.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err)
				assert.True(t, stage.compiled)

				// Test validation after configuration
				err = stage.Validate()
				assert.NoError(t, err)
			}
		})
	}
}

func TestAggregationStage_Process(t *testing.T) {
	tests := []struct {
		name        string
		config      map[string]interface{}
		inputData   []*pipeline.Data // Multiple data points for aggregation
		expectError bool
		errorMsg    string
		validate    func(t *testing.T, results []*pipeline.StageResult)
	}{
		{
			name: "count aggregation",
			config: map[string]interface{}{
				"window_size": 60,
				"aggregations": []map[string]interface{}{
					{
						"name":         "count",
						"target_field": "total_count",
					},
				},
			},
			inputData: []*pipeline.Data{
				{ID: "1", Body: []byte(`{"event": "login"}`)},
				{ID: "2", Body: []byte(`{"event": "logout"}`)},
				{ID: "3", Body: []byte(`{"event": "login"}`)},
			},
			expectError: false,
			validate: func(t *testing.T, results []*pipeline.StageResult) {
				// Check the last result which should have all aggregated data
				lastResult := results[len(results)-1]
				assert.True(t, lastResult.Success)

				var output map[string]interface{}
				err := json.Unmarshal(lastResult.Data.Body, &output)
				require.NoError(t, err)

				aggregated := output["aggregated"].(map[string]interface{})
				assert.Equal(t, float64(3), aggregated["total_count"])
			},
		},
		{
			name: "sum aggregation",
			config: map[string]interface{}{
				"window_size": 60,
				"aggregations": []map[string]interface{}{
					{
						"name":         "sum",
						"source_field": "amount",
						"target_field": "total_amount",
					},
				},
			},
			inputData: []*pipeline.Data{
				{ID: "1", Body: []byte(`{"amount": 10.5}`)},
				{ID: "2", Body: []byte(`{"amount": 20.3}`)},
				{ID: "3", Body: []byte(`{"amount": 5.2}`)},
			},
			expectError: false,
			validate: func(t *testing.T, results []*pipeline.StageResult) {
				lastResult := results[len(results)-1]
				var output map[string]interface{}
				err := json.Unmarshal(lastResult.Data.Body, &output)
				require.NoError(t, err)

				aggregated := output["aggregated"].(map[string]interface{})
				expectedSum := 10.5 + 20.3 + 5.2
				assert.InDelta(t, expectedSum, aggregated["total_amount"], 0.01)
			},
		},
		{
			name: "average aggregation",
			config: map[string]interface{}{
				"window_size": 60,
				"aggregations": []map[string]interface{}{
					{
						"name":         "avg",
						"source_field": "score",
						"target_field": "avg_score",
					},
				},
			},
			inputData: []*pipeline.Data{
				{ID: "1", Body: []byte(`{"score": 80}`)},
				{ID: "2", Body: []byte(`{"score": 90}`)},
				{ID: "3", Body: []byte(`{"score": 70}`)},
			},
			expectError: false,
			validate: func(t *testing.T, results []*pipeline.StageResult) {
				lastResult := results[len(results)-1]
				var output map[string]interface{}
				err := json.Unmarshal(lastResult.Data.Body, &output)
				require.NoError(t, err)

				aggregated := output["aggregated"].(map[string]interface{})
				expectedAvg := (80.0 + 90.0 + 70.0) / 3.0
				assert.InDelta(t, expectedAvg, aggregated["avg_score"], 0.01)
			},
		},
		{
			name: "grouped aggregation",
			config: map[string]interface{}{
				"window_size": 60,
				"group_by":    []string{"user_type"},
				"aggregations": []map[string]interface{}{
					{
						"name":         "count",
						"target_field": "user_count",
					},
				},
			},
			inputData: []*pipeline.Data{
				{ID: "1", Body: []byte(`{"user_type": "premium", "action": "login"}`)},
				{ID: "2", Body: []byte(`{"user_type": "free", "action": "login"}`)},
				{ID: "3", Body: []byte(`{"user_type": "premium", "action": "logout"}`)},
			},
			expectError: false,
			validate: func(t *testing.T, results []*pipeline.StageResult) {
				// Should have different group keys for different user types
				assert.Equal(t, 3, len(results))
				
				// Check that group keys are different for premium vs free users
				premiumGroupKey := results[0].Metadata["group_key"]
				freeGroupKey := results[1].Metadata["group_key"]
				assert.NotEqual(t, premiumGroupKey, freeGroupKey)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stage := &AggregationStage{
				name:   "test-aggregation",
				buffer: make(map[string][]interface{}),
			}

			err := stage.Configure(tt.config)
			require.NoError(t, err)

			var results []*pipeline.StageResult

			// Process all input data sequentially
			for _, inputData := range tt.inputData {
				result, err := stage.Process(context.Background(), inputData)
				if tt.expectError {
					assert.Error(t, err)
					if tt.errorMsg != "" {
						assert.Contains(t, err.Error(), tt.errorMsg)
					}
					return
				}

				assert.NoError(t, err)
				results = append(results, result)
			}

			if tt.validate != nil {
				tt.validate(t, results)
			}
		})
	}
}

func TestAggregationStage_ConcurrentAccess(t *testing.T) {
	// Test concurrent access to aggregation buffer
	stage := &AggregationStage{
		name:   "test-concurrent-aggregation",
		buffer: make(map[string][]interface{}),
	}

	config := map[string]interface{}{
		"window_size": 60,
		"aggregations": []map[string]interface{}{
			{
				"name":         "count",
				"target_field": "total_count",
			},
		},
	}

	err := stage.Configure(config)
	require.NoError(t, err)

	const numRoutines = 20
	var wg sync.WaitGroup
	results := make([]*pipeline.StageResult, numRoutines)
	errors := make([]error, numRoutines)

	// Run multiple goroutines concurrently
	for i := 0; i < numRoutines; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()

			inputData := &pipeline.Data{
				ID:   fmt.Sprintf("test-%d", index),
				Body: []byte(fmt.Sprintf(`{"id": %d, "value": %d}`, index, index*10)),
			}

			result, err := stage.Process(context.Background(), inputData)
			results[index] = result
			errors[index] = err
		}(i)
	}

	wg.Wait()

	// Verify all executions succeeded
	successCount := 0
	for i := 0; i < numRoutines; i++ {
		if errors[i] == nil {
			successCount++
			assert.True(t, results[i].Success, "Execution %d should succeed", i)
		}
	}

	assert.Equal(t, numRoutines, successCount, "All concurrent executions should succeed")

	// Check the final buffer size (should have all items for default group)
	stage.mu.RLock()
	bufferSize := len(stage.buffer["default"])
	stage.mu.RUnlock()
	assert.Equal(t, numRoutines, bufferSize, "Buffer should contain all processed items")
}

func TestEnrichmentStage_Health(t *testing.T) {
	stage := &EnrichmentStage{
		name: "test-enrichment",
	}

	// Should fail when not configured
	err := stage.Health()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "stage not configured")

	// Should pass after configuration
	config := map[string]interface{}{
		"enrichments": []map[string]interface{}{
			{
				"name":         "test",
				"type":         "static",
				"target_field": "test",
				"static_value": "test",
			},
		},
	}
	err = stage.Configure(config)
	require.NoError(t, err)

	err = stage.Health()
	assert.NoError(t, err)
}

func TestAggregationStage_Health(t *testing.T) {
	stage := &AggregationStage{
		name: "test-aggregation",
	}

	// Should fail when not configured
	err := stage.Health()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "stage not configured")

	// Should pass after configuration
	config := map[string]interface{}{
		"window_size": 60,
		"aggregations": []map[string]interface{}{
			{
				"name":         "count",
				"target_field": "total",
			},
		},
	}
	err = stage.Configure(config)
	require.NoError(t, err)

	err = stage.Health()
	assert.NoError(t, err)
}

func TestEnrichmentStage_Interface(t *testing.T) {
	// Ensure EnrichmentStage implements pipeline.Stage
	var _ pipeline.Stage = &EnrichmentStage{}

	stage := &EnrichmentStage{
		name: "test-enrichment-stage",
	}

	assert.Equal(t, "test-enrichment-stage", stage.Name())
	assert.Equal(t, "enrich", stage.Type())
}

func TestAggregationStage_Interface(t *testing.T) {
	// Ensure AggregationStage implements pipeline.Stage
	var _ pipeline.Stage = &AggregationStage{}

	stage := &AggregationStage{
		name: "test-aggregation-stage",
	}

	assert.Equal(t, "test-aggregation-stage", stage.Name())
	assert.Equal(t, "aggregate", stage.Type())
}

func BenchmarkEnrichmentStage_Process(b *testing.B) {
	stage := &EnrichmentStage{
		name: "bench-enrichment",
	}

	config := map[string]interface{}{
		"parallel": false,
		"enrichments": []map[string]interface{}{
			{
				"name":         "status",
				"type":         "static",
				"target_field": "status",
				"static_value": "processed",
			},
			{
				"name":         "country_lookup",
				"type":         "lookup",
				"source_field": "country_code",
				"target_field": "country_name",
				"lookup_table": map[string]interface{}{
					"US": "United States",
					"UK": "United Kingdom",
					"CA": "Canada",
				},
				"default_value": "Unknown",
			},
		},
	}

	err := stage.Configure(config)
	require.NoError(b, err)

	data := &pipeline.Data{
		ID:   "bench-test",
		Body: []byte(`{"id": 123, "country_code": "US", "name": "John"}`),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = stage.Process(context.Background(), data)
	}
}

func BenchmarkEnrichmentStage_ProcessParallel(b *testing.B) {
	stage := &EnrichmentStage{
		name: "bench-enrichment-parallel",
	}

	config := map[string]interface{}{
		"parallel": true,
		"enrichments": []map[string]interface{}{
			{
				"name":         "status1",
				"type":         "static",
				"target_field": "status1",
				"static_value": "processed1",
			},
			{
				"name":         "status2",
				"type":         "static",
				"target_field": "status2",
				"static_value": "processed2",
			},
			{
				"name":         "timestamp",
				"type":         "function",
				"target_field": "processed_at",
			},
		},
	}

	err := stage.Configure(config)
	require.NoError(b, err)

	data := &pipeline.Data{
		ID:   "bench-test",
		Body: []byte(`{"id": 123}`),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = stage.Process(context.Background(), data)
	}
}

func BenchmarkAggregationStage_Process(b *testing.B) {
	stage := &AggregationStage{
		name:   "bench-aggregation",
		buffer: make(map[string][]interface{}),
	}

	config := map[string]interface{}{
		"window_size": 60,
		"aggregations": []map[string]interface{}{
			{
				"name":         "count",
				"target_field": "total_count",
			},
			{
				"name":         "sum",
				"source_field": "amount",
				"target_field": "total_amount",
			},
			{
				"name":         "avg",
				"source_field": "amount",
				"target_field": "avg_amount",
			},
		},
	}

	err := stage.Configure(config)
	require.NoError(b, err)

	data := &pipeline.Data{
		ID:   "bench-test",
		Body: []byte(`{"id": 123, "amount": 50.5}`),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = stage.Process(context.Background(), data)
	}
}