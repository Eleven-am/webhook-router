package stages

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"webhook-router/internal/pipeline"
)

// TestConcurrentStageExecution tests concurrent execution across different stage types
func TestConcurrentStageExecution(t *testing.T) {
	// Create multiple stage types
	transformStage := &JSONTransformStage{name: "concurrent-transform"}
	err := transformStage.Configure(map[string]interface{}{
		"transformations": []interface{}{
			map[string]interface{}{
				"field":     "email",
				"operation": "lowercase",
			},
		},
	})
	require.NoError(t, err)

	validateStage := &ValidationStage{name: "concurrent-validate"}
	err = validateStage.Configure(map[string]interface{}{
		"rules": []map[string]interface{}{
			{
				"field":    "email",
				"required": true,
				"pattern":  "^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\\.[a-zA-Z]{2,}$",
			},
		},
	})
	require.NoError(t, err)

	enrichmentStage := &EnrichmentStage{name: "concurrent-enrich"}
	err = enrichmentStage.Configure(map[string]interface{}{
		"enrichments": []map[string]interface{}{
			{
				"name":         "add_status",
				"type":         "static",
				"target_field": "status",
				"static_value": "processed",
			},
		},
	})
	require.NoError(t, err)

	filterStage := &FilterStage{name: "concurrent-filter"}
	err = filterStage.Configure(map[string]interface{}{
		"conditions": []map[string]interface{}{
			{
				"field":    "status",
				"operator": "eq",
				"value":    "processed",
			},
		},
		"action": "include",
	})
	require.NoError(t, err)

	// Test data
	testData := []*pipeline.Data{
		{ID: "1", Body: []byte(`{"email": "JOHN@EXAMPLE.COM", "name": "John"}`)},
		{ID: "2", Body: []byte(`{"email": "JANE@EXAMPLE.COM", "name": "Jane"}`)},
		{ID: "3", Body: []byte(`{"email": "BOB@EXAMPLE.COM", "name": "Bob"}`)},
		{ID: "4", Body: []byte(`{"email": "ALICE@EXAMPLE.COM", "name": "Alice"}`)},
		{ID: "5", Body: []byte(`{"email": "CHARLIE@EXAMPLE.COM", "name": "Charlie"}`)},
	}

	const numRoutines = 10
	var wg sync.WaitGroup
	results := make([][]*pipeline.StageResult, numRoutines)
	errors := make([]error, numRoutines)

	// Run multiple pipeline executions concurrently
	for i := 0; i < numRoutines; i++ {
		wg.Add(1)
		go func(routineIndex int) {
			defer wg.Done()

			var stageResults []*pipeline.StageResult
			currentData := testData[routineIndex%len(testData)]

			// Execute stages in sequence: Transform -> Validate -> Enrich -> Filter
			// Transform
			result1, err := transformStage.Process(context.Background(), currentData)
			if err != nil || !result1.Success {
				errors[routineIndex] = fmt.Errorf("transform failed: %v", err)
				return
			}
			stageResults = append(stageResults, result1)

			// Validate
			result2, err := validateStage.Process(context.Background(), result1.Data)
			if err != nil || !result2.Success {
				errors[routineIndex] = fmt.Errorf("validate failed: %v", err)
				return
			}
			stageResults = append(stageResults, result2)

			// Enrich
			result3, err := enrichmentStage.Process(context.Background(), result2.Data)
			if err != nil || !result3.Success {
				errors[routineIndex] = fmt.Errorf("enrich failed: %v", err)
				return
			}
			stageResults = append(stageResults, result3)

			// Filter
			result4, err := filterStage.Process(context.Background(), result3.Data)
			if err != nil || !result4.Success {
				errors[routineIndex] = fmt.Errorf("filter failed: %v", err)
				return
			}
			stageResults = append(stageResults, result4)

			results[routineIndex] = stageResults
		}(i)
	}

	wg.Wait()

	// Verify all executions succeeded
	for i := 0; i < numRoutines; i++ {
		assert.NoError(t, errors[i], "Execution %d should not error", i)
		assert.Equal(t, 4, len(results[i]), "Should have 4 stage results")

		// Check final result
		finalResult := results[i][3] // Filter stage result
		assert.True(t, finalResult.Success, "Final result should be successful")

		var output map[string]interface{}
		err := json.Unmarshal(finalResult.Data.Body, &output)
		require.NoError(t, err)

		// Verify transformations were applied
		email := output["email"].(string)
		assert.True(t, email == "john@example.com" || email == "jane@example.com" || 
					 email == "bob@example.com" || email == "alice@example.com" || 
					 email == "charlie@example.com", "Email should be lowercase")
		
		// Verify enrichment was applied
		assert.Equal(t, "processed", output["status"])
	}
}

// TestStageFactory_ConcurrentCreation tests concurrent stage creation
func TestStageFactory_ConcurrentCreation(t *testing.T) {
	// Test concurrent stage factory usage
	const numRoutines = 20
	var wg sync.WaitGroup
	stages := make([]pipeline.Stage, numRoutines)
	errors := make([]error, numRoutines)

	factories := []func() (pipeline.Stage, error){
		func() (pipeline.Stage, error) {
			factory := &JSONTransformStageFactory{}
			return factory.Create(map[string]interface{}{
				"name": "test-transform",
				"transformations": []interface{}{
					map[string]interface{}{
						"field":     "test",
						"operation": "lowercase",
					},
				},
			})
		},
		func() (pipeline.Stage, error) {
			factory := &ValidationStageFactory{}
			return factory.Create(map[string]interface{}{
				"name": "test-validate",
				"rules": []map[string]interface{}{
					{
						"field":    "test",
						"required": true,
					},
				},
			})
		},
		func() (pipeline.Stage, error) {
			factory := &EnrichmentStageFactory{}
			return factory.Create(map[string]interface{}{
				"name": "test-enrich",
				"enrichments": []map[string]interface{}{
					{
						"name":         "test",
						"type":         "static",
						"target_field": "test",
						"static_value": "test",
					},
				},
			})
		},
		func() (pipeline.Stage, error) {
			factory := &FilterStageFactory{}
			return factory.Create(map[string]interface{}{
				"name": "test-filter",
				"conditions": []map[string]interface{}{
					{
						"field":    "test",
						"operator": "eq",
						"value":    "test",
					},
				},
				"action": "include",
			})
		},
	}

	// Create stages concurrently
	for i := 0; i < numRoutines; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()

			// Use different factory types in rotation
			factoryFunc := factories[index%len(factories)]
			stage, err := factoryFunc()
			
			stages[index] = stage
			errors[index] = err
		}(i)
	}

	wg.Wait()

	// Verify all stage creations succeeded
	successCount := 0
	for i := 0; i < numRoutines; i++ {
		if errors[i] == nil {
			successCount++
			assert.NotNil(t, stages[i], "Stage %d should not be nil", i)
			
			// Verify stage health
			err := stages[i].Health()
			assert.NoError(t, err, "Stage %d should be healthy", i)
		} else {
			t.Errorf("Stage creation %d failed: %v", i, errors[i])
		}
	}

	assert.Equal(t, numRoutines, successCount, "All stage creations should succeed")
}

// TestConcurrentStageProcessingWithSharedData tests concurrent processing with shared data structures
func TestConcurrentStageProcessingWithSharedData(t *testing.T) {
	// Test concurrent access to stages that might share data (like aggregation)
	stage := &AggregationStage{
		name:   "concurrent-shared-aggregation",
		buffer: make(map[string][]interface{}),
	}

	config := map[string]interface{}{
		"window_size": 60,
		"group_by":    []string{"user_id"}, // This will create shared buffer access
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
		},
	}

	err := stage.Configure(config)
	require.NoError(t, err)

	const numRoutines = 50
	const itemsPerRoutine = 10
	var wg sync.WaitGroup
	successCount := int64(0)
	var mu sync.Mutex

	// Run many concurrent operations that will access shared buffer
	for i := 0; i < numRoutines; i++ {
		wg.Add(1)
		go func(routineIndex int) {
			defer wg.Done()

			localSuccessCount := 0
			for j := 0; j < itemsPerRoutine; j++ {
				// Vary user_id to test different group keys
				userID := routineIndex % 5 // 5 different users
				inputData := &pipeline.Data{
					ID:   fmt.Sprintf("routine-%d-item-%d", routineIndex, j),
					Body: []byte(fmt.Sprintf(`{"user_id": %d, "amount": %d}`, userID, j+1)),
				}

				result, err := stage.Process(context.Background(), inputData)
				if err == nil && result.Success {
					localSuccessCount++
				}
			}

			mu.Lock()
			successCount += int64(localSuccessCount)
			mu.Unlock()
		}(i)
	}

	wg.Wait()

	expectedTotal := int64(numRoutines * itemsPerRoutine)
	assert.Equal(t, expectedTotal, successCount, "All concurrent operations should succeed")

	// Verify buffer integrity
	stage.mu.RLock()
	totalBufferItems := 0
	for groupKey, items := range stage.buffer {
		totalBufferItems += len(items)
		t.Logf("Group %s has %d items", groupKey, len(items))
	}
	stage.mu.RUnlock()

	assert.Equal(t, int(expectedTotal), totalBufferItems, "Buffer should contain all processed items")
}

// TestConcurrentHealthChecks tests concurrent health checks on stages
func TestConcurrentHealthChecks(t *testing.T) {
	// Create and configure multiple stages
	stages := []pipeline.Stage{
		func() pipeline.Stage {
			s := &JSONTransformStage{name: "health-transform"}
			_ = s.Configure(map[string]interface{}{
				"transformations": []interface{}{
					map[string]interface{}{
						"field":     "test",
						"operation": "lowercase",
					},
				},
			})
			return s
		}(),
		func() pipeline.Stage {
			s := &ValidationStage{name: "health-validate"}
			_ = s.Configure(map[string]interface{}{
				"rules": []map[string]interface{}{
					{
						"field":    "test",
						"required": true,
					},
				},
			})
			return s
		}(),
		func() pipeline.Stage {
			s := &EnrichmentStage{name: "health-enrich"}
			_ = s.Configure(map[string]interface{}{
				"enrichments": []map[string]interface{}{
					{
						"name":         "test",
						"type":         "static",
						"target_field": "test",
						"static_value": "test",
					},
				},
			})
			return s
		}(),
		func() pipeline.Stage {
			s := &FilterStage{name: "health-filter"}
			_ = s.Configure(map[string]interface{}{
				"conditions": []map[string]interface{}{
					{
						"field":    "test",
						"operator": "eq",
						"value":    "test",
					},
				},
				"action": "include",
			})
			return s
		}(),
	}

	const numHealthChecks = 100
	var wg sync.WaitGroup
	healthResults := make([][]error, len(stages))

	// Perform concurrent health checks on all stages
	for stageIndex, stage := range stages {
		healthResults[stageIndex] = make([]error, numHealthChecks)
		
		for i := 0; i < numHealthChecks; i++ {
			wg.Add(1)
			go func(sIndex, checkIndex int, s pipeline.Stage) {
				defer wg.Done()
				
				// Add some random delay to increase concurrency
				time.Sleep(time.Microsecond * time.Duration(checkIndex%10))
				
				err := s.Health()
				healthResults[sIndex][checkIndex] = err
			}(stageIndex, i, stage)
		}
	}

	wg.Wait()

	// Verify all health checks passed
	for stageIndex, results := range healthResults {
		for checkIndex, err := range results {
			assert.NoError(t, err, "Health check %d for stage %d should pass", checkIndex, stageIndex)
		}
	}
}

// TestStageMethodConcurrency tests concurrent access to individual stage methods
func TestStageMethodConcurrency(t *testing.T) {
	stage := &JSONTransformStage{name: "method-concurrency-test"}
	
	err := stage.Configure(map[string]interface{}{
		"transformations": []interface{}{
			map[string]interface{}{
				"field":     "email",
				"operation": "lowercase",
			},
		},
	})
	require.NoError(t, err)

	const numOperations = 50
	var wg sync.WaitGroup
	
	// Test concurrent access to read-only methods
	for i := 0; i < numOperations; i++ {
		wg.Add(3) // Name, Type, Health
		
		go func(index int) {
			defer wg.Done()
			name := stage.Name()
			assert.Equal(t, "method-concurrency-test", name)
		}(i)
		
		go func(index int) {
			defer wg.Done()
			stageType := stage.Type()
			assert.Equal(t, "transform", stageType)
		}(i)
		
		go func(index int) {
			defer wg.Done()
			err := stage.Health()
			assert.NoError(t, err)
		}(i)
	}

	// Test concurrent Process calls
	inputData := &pipeline.Data{
		ID:   "concurrent-test",
		Body: []byte(`{"email": "TEST@EXAMPLE.COM", "name": "Test"}`),
	}

	processResults := make([]*pipeline.StageResult, numOperations)
	processErrors := make([]error, numOperations)

	for i := 0; i < numOperations; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			result, err := stage.Process(context.Background(), inputData)
			processResults[index] = result
			processErrors[index] = err
		}(i)
	}

	wg.Wait()

	// Verify all process calls succeeded and produced consistent results
	for i := 0; i < numOperations; i++ {
		assert.NoError(t, processErrors[i], "Process call %d should not error", i)
		assert.True(t, processResults[i].Success, "Process call %d should succeed", i)

		var output map[string]interface{}
		err := json.Unmarshal(processResults[i].Data.Body, &output)
		require.NoError(t, err)

		// Verify transformation was applied consistently
		assert.Equal(t, "test@example.com", output["email"], "Email should be consistently transformed")
		assert.Equal(t, "Test", output["name"], "Name should be preserved")
	}
}

// BenchmarkConcurrentStageExecution benchmarks concurrent stage execution
func BenchmarkConcurrentStageExecution(b *testing.B) {
	// Setup stages
	transformStage := &JSONTransformStage{name: "bench-transform"}
	_ = transformStage.Configure(map[string]interface{}{
		"transformations": []interface{}{
			map[string]interface{}{
				"field":     "email",
				"operation": "lowercase",
			},
		},
	})

	enrichmentStage := &EnrichmentStage{name: "bench-enrich"}
	_ = enrichmentStage.Configure(map[string]interface{}{
		"enrichments": []map[string]interface{}{
			{
				"name":         "status",
				"type":         "static",
				"target_field": "status",
				"static_value": "processed",
			},
		},
	})

	inputData := &pipeline.Data{
		ID:   "bench-test",
		Body: []byte(`{"email": "JOHN@EXAMPLE.COM", "name": "John"}`),
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			// Transform
			result1, err := transformStage.Process(context.Background(), inputData)
			if err != nil || !result1.Success {
				b.Fatalf("Transform failed: %v", err)
			}

			// Enrich
			result2, err := enrichmentStage.Process(context.Background(), result1.Data)
			if err != nil || !result2.Success {
				b.Fatalf("Enrich failed: %v", err)
			}
		}
	})
}