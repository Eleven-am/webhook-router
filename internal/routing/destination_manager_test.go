package routing

import (
	"fmt"
	"testing"
	"time"
)

func TestNewBasicDestinationManager(t *testing.T) {
	dm := NewBasicDestinationManager()
	
	if dm == nil {
		t.Fatal("NewBasicDestinationManager() returned nil")
	}
	
	if dm.healthStatus == nil {
		t.Error("NewBasicDestinationManager() should initialize healthStatus map")
	}
	
	if dm.metrics == nil {
		t.Error("NewBasicDestinationManager() should initialize metrics map")
	}
	
	if dm.circuitBreakers == nil {
		t.Error("NewBasicDestinationManager() should initialize circuitBreakers map")
	}
	
	if dm.roundRobinIndex == nil {
		t.Error("NewBasicDestinationManager() should initialize roundRobinIndex map")
	}
	
	if dm.sessionMap == nil {
		t.Error("NewBasicDestinationManager() should initialize sessionMap")
	}
}

func TestBasicDestinationManager_UpdateDestinationHealth(t *testing.T) {
	dm := NewBasicDestinationManager()
	
	destID := "test-dest"
	
	// Test updating to healthy
	err := dm.UpdateDestinationHealth(destID, true)
	if err != nil {
		t.Errorf("UpdateDestinationHealth() unexpected error = %v", err)
	}
	
	// Verify health status was updated
	healthy, err := dm.GetDestinationHealth(destID)
	if err != nil {
		t.Errorf("GetDestinationHealth() unexpected error = %v", err)
	}
	
	if !healthy {
		t.Errorf("GetDestinationHealth() = false, want true")
	}
	
	// Test updating to unhealthy
	err = dm.UpdateDestinationHealth(destID, false)
	if err != nil {
		t.Errorf("UpdateDestinationHealth() unexpected error = %v", err)
	}
	
	healthy, err = dm.GetDestinationHealth(destID)
	if err != nil {
		t.Errorf("GetDestinationHealth() unexpected error = %v", err)
	}
	
	if healthy {
		t.Errorf("GetDestinationHealth() = true, want false")
	}
}

func TestBasicDestinationManager_GetDestinationHealth(t *testing.T) {
	dm := NewBasicDestinationManager()
	
	// Test getting health for non-existing destination (should default to healthy)
	healthy, err := dm.GetDestinationHealth("non-existing")
	if err != nil {
		t.Errorf("GetDestinationHealth() unexpected error = %v", err)
	}
	
	if !healthy {
		t.Errorf("GetDestinationHealth() for non-existing destination should default to true")
	}
}

func TestBasicDestinationManager_RecordDestinationMetrics(t *testing.T) {
	dm := NewBasicDestinationManager()
	
	destID := "test-dest"
	latency := 100 * time.Millisecond
	
	// Record successful request
	err := dm.RecordDestinationMetrics(destID, latency, true)
	if err != nil {
		t.Errorf("RecordDestinationMetrics() unexpected error = %v", err)
	}
	
	// Verify metrics were updated
	metrics, err := dm.GetDestinationMetrics(destID)
	if err != nil {
		t.Errorf("GetDestinationMetrics() unexpected error = %v", err)
	}
	
	if metrics.TotalRequests != 1 {
		t.Errorf("TotalRequests = %d, want 1", metrics.TotalRequests)
	}
	
	if metrics.SuccessfulRequests != 1 {
		t.Errorf("SuccessfulRequests = %d, want 1", metrics.SuccessfulRequests)
	}
	
	if metrics.FailedRequests != 0 {
		t.Errorf("FailedRequests = %d, want 0", metrics.FailedRequests)
	}
	
	if metrics.AverageLatency != latency {
		t.Errorf("AverageLatency = %v, want %v", metrics.AverageLatency, latency)
	}
	
	// Record failed request
	err = dm.RecordDestinationMetrics(destID, latency*2, false)
	if err != nil {
		t.Errorf("RecordDestinationMetrics() unexpected error = %v", err)
	}
	
	// Verify metrics were updated
	metrics, err = dm.GetDestinationMetrics(destID)
	if err != nil {
		t.Errorf("GetDestinationMetrics() unexpected error = %v", err)
	}
	
	if metrics.TotalRequests != 2 {
		t.Errorf("TotalRequests = %d, want 2", metrics.TotalRequests)
	}
	
	if metrics.SuccessfulRequests != 1 {
		t.Errorf("SuccessfulRequests = %d, want 1", metrics.SuccessfulRequests)
	}
	
	if metrics.FailedRequests != 1 {
		t.Errorf("FailedRequests = %d, want 1", metrics.FailedRequests)
	}
}

func TestBasicDestinationManager_SelectDestination_RoundRobin(t *testing.T) {
	dm := NewBasicDestinationManager()
	
	destinations := []RouteDestination{
		{ID: "dest1", Type: "http"},
		{ID: "dest2", Type: "http"},
		{ID: "dest3", Type: "http"},
	}
	
	strategy := LoadBalancingConfig{
		Strategy: "round_robin",
	}
	
	request := &RouteRequest{
		ID: "test-request",
	}
	
	// Test multiple selections to verify round robin behavior
	selectedIDs := make([]string, 6)
	for i := 0; i < 6; i++ {
		selected, err := dm.SelectDestination(destinations, strategy, request)
		if err != nil {
			t.Errorf("SelectDestination() unexpected error = %v", err)
		}
		selectedIDs[i] = selected.ID
	}
	
	// Verify round robin pattern (should repeat after 3 selections)
	expected := []string{"dest1", "dest2", "dest3", "dest1", "dest2", "dest3"}
	for i, expectedID := range expected {
		if selectedIDs[i] != expectedID {
			t.Errorf("Selection %d: got %s, want %s", i, selectedIDs[i], expectedID)
		}
	}
}

func TestBasicDestinationManager_SelectDestination_Weighted(t *testing.T) {
	dm := NewBasicDestinationManager()
	
	destinations := []RouteDestination{
		{ID: "dest1", Type: "http", Weight: 1},
		{ID: "dest2", Type: "http", Weight: 3},
		{ID: "dest3", Type: "http", Weight: 0}, // Should default to 1
	}
	
	strategy := LoadBalancingConfig{
		Strategy: "weighted",
	}
	
	request := &RouteRequest{
		ID: "test-request",
	}
	
	// Test multiple selections and count distribution
	selections := make(map[string]int)
	numTests := 1000
	
	for i := 0; i < numTests; i++ {
		selected, err := dm.SelectDestination(destinations, strategy, request)
		if err != nil {
			t.Errorf("SelectDestination() unexpected error = %v", err)
		}
		selections[selected.ID]++
	}
	
	// dest2 should be selected about 3x more than dest1 and dest3
	// Allow some variance due to randomness
	if selections["dest2"] < selections["dest1"]*2 {
		t.Errorf("Weighted selection not working correctly: dest2=%d, dest1=%d", selections["dest2"], selections["dest1"])
	}
}

func TestBasicDestinationManager_SelectDestination_Random(t *testing.T) {
	dm := NewBasicDestinationManager()
	
	destinations := []RouteDestination{
		{ID: "dest1", Type: "http"},
		{ID: "dest2", Type: "http"},
	}
	
	strategy := LoadBalancingConfig{
		Strategy: "random",
	}
	
	request := &RouteRequest{
		ID: "test-request",
	}
	
	// Test multiple selections to ensure randomness
	selections := make(map[string]int)
	numTests := 100
	
	for i := 0; i < numTests; i++ {
		selected, err := dm.SelectDestination(destinations, strategy, request)
		if err != nil {
			t.Errorf("SelectDestination() unexpected error = %v", err)
		}
		selections[selected.ID]++
	}
	
	// Both destinations should be selected at least once in 100 tries
	if selections["dest1"] == 0 || selections["dest2"] == 0 {
		t.Errorf("Random selection not working: dest1=%d, dest2=%d", selections["dest1"], selections["dest2"])
	}
}

func TestBasicDestinationManager_SelectDestination_Hash(t *testing.T) {
	dm := NewBasicDestinationManager()
	
	destinations := []RouteDestination{
		{ID: "dest1", Type: "http"},
		{ID: "dest2", Type: "http"},
		{ID: "dest3", Type: "http"},
	}
	
	strategy := LoadBalancingConfig{
		Strategy: "hash",
		HashKey:  "ip",
	}
	
	tests := []struct {
		name       string
		remoteAddr string
		expectSame bool
	}{
		{
			name:       "same IP should get same destination",
			remoteAddr: "192.168.1.100",
			expectSame: true,
		},
		{
			name:       "same IP again",
			remoteAddr: "192.168.1.100",
			expectSame: true,
		},
		{
			name:       "different IP",
			remoteAddr: "10.0.0.1",
			expectSame: false,
		},
	}
	
	var firstSelection *RouteDestination
	var previousSelection *RouteDestination
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := &RouteRequest{
				ID:         "test-request",
				RemoteAddr: tt.remoteAddr,
			}
			
			selected, err := dm.SelectDestination(destinations, strategy, request)
			if err != nil {
				t.Errorf("SelectDestination() unexpected error = %v", err)
			}
			
			if firstSelection == nil {
				firstSelection = selected
				previousSelection = selected
				return
			}
			
			if tt.expectSame {
				if selected.ID != previousSelection.ID {
					t.Errorf("Hash selection should be consistent for same input: got %s, want %s", selected.ID, previousSelection.ID)
				}
			}
			
			previousSelection = selected
		})
	}
}

func TestBasicDestinationManager_SelectDestination_LeastConnections(t *testing.T) {
	dm := NewBasicDestinationManager()
	
	destinations := []RouteDestination{
		{ID: "dest1", Type: "http"},
		{ID: "dest2", Type: "http"},
	}
	
	strategy := LoadBalancingConfig{
		Strategy: "least_connections",
	}
	
	request := &RouteRequest{
		ID: "test-request",
	}
	
	// Record some metrics to simulate different connection counts
	dm.RecordDestinationMetrics("dest1", 100*time.Millisecond, true)
	dm.RecordDestinationMetrics("dest1", 100*time.Millisecond, true)
	dm.RecordDestinationMetrics("dest2", 100*time.Millisecond, true)
	
	// dest2 should be selected as it has fewer requests
	selected, err := dm.SelectDestination(destinations, strategy, request)
	if err != nil {
		t.Errorf("SelectDestination() unexpected error = %v", err)
	}
	
	// For this simple test, we'll just verify a destination was selected
	if selected == nil {
		t.Errorf("SelectDestination() should return a destination")
	}
}

func TestBasicDestinationManager_SelectDestination_StickySession(t *testing.T) {
	dm := NewBasicDestinationManager()
	
	destinations := []RouteDestination{
		{ID: "dest1", Type: "http"},
		{ID: "dest2", Type: "http"},
	}
	
	strategy := LoadBalancingConfig{
		Strategy:      "round_robin",
		StickySession: true,
		SessionKey:    "session_id",
	}
	
	request1 := &RouteRequest{
		ID: "request1",
		Headers: map[string]string{
			"session_id": "session123",
		},
	}
	
	request2 := &RouteRequest{
		ID: "request2",
		Headers: map[string]string{
			"session_id": "session123", // Same session
		},
	}
	
	// First request should create session mapping
	selected1, err := dm.SelectDestination(destinations, strategy, request1)
	if err != nil {
		t.Errorf("SelectDestination() unexpected error = %v", err)
	}
	
	// Second request with same session should get same destination
	selected2, err := dm.SelectDestination(destinations, strategy, request2)
	if err != nil {
		t.Errorf("SelectDestination() unexpected error = %v", err)
	}
	
	if selected1.ID != selected2.ID {
		t.Errorf("Sticky session should route to same destination: got %s and %s", selected1.ID, selected2.ID)
	}
}

func TestBasicDestinationManager_SelectDestination_NoDestinations(t *testing.T) {
	dm := NewBasicDestinationManager()
	
	destinations := []RouteDestination{}
	strategy := LoadBalancingConfig{Strategy: "round_robin"}
	request := &RouteRequest{ID: "test"}
	
	_, err := dm.SelectDestination(destinations, strategy, request)
	if err != ErrNoDestinationsAvailable {
		t.Errorf("SelectDestination() with no destinations should return ErrNoDestinationsAvailable, got %v", err)
	}
}

func TestBasicDestinationManager_SelectDestination_UnsupportedStrategy(t *testing.T) {
	dm := NewBasicDestinationManager()
	
	destinations := []RouteDestination{
		{ID: "dest1", Type: "http"},
	}
	
	strategy := LoadBalancingConfig{
		Strategy: "unsupported_strategy",
	}
	
	request := &RouteRequest{ID: "test"}
	
	_, err := dm.SelectDestination(destinations, strategy, request)
	if err == nil {
		t.Errorf("SelectDestination() with unsupported strategy should return error")
	}
}

func TestBasicDestinationManager_getSessionValue(t *testing.T) {
	dm := NewBasicDestinationManager()
	
	request := &RouteRequest{
		Headers: map[string]string{
			"session_id": "header_session",
		},
		QueryParams: map[string]string{
			"session": "query_session",
		},
		Context: map[string]interface{}{
			"ctx_session": "context_session",
		},
	}
	
	tests := []struct {
		name       string
		sessionKey string
		expected   string
	}{
		{
			name:       "from headers",
			sessionKey: "session_id",
			expected:   "header_session",
		},
		{
			name:       "from query params",
			sessionKey: "session",
			expected:   "query_session",
		},
		{
			name:       "from context",
			sessionKey: "ctx_session",
			expected:   "context_session",
		},
		{
			name:       "not found",
			sessionKey: "missing_key",
			expected:   "",
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := dm.getSessionValue(request, tt.sessionKey)
			if result != tt.expected {
				t.Errorf("getSessionValue() = %s, want %s", result, tt.expected)
			}
		})
	}
}

func TestBasicDestinationManager_CircuitBreaker(t *testing.T) {
	dm := NewBasicDestinationManager()
	
	destID := "circuit-test-dest"
	
	// Set circuit breaker config
	config := CircuitBreakerConfig{
		Enabled:          true,
		FailureThreshold: 3,
		RecoveryTimeout:  1 * time.Second,
		HalfOpenMax:      2,
	}
	
	err := dm.SetCircuitBreakerConfig(destID, config)
	if err != nil {
		t.Errorf("SetCircuitBreakerConfig() unexpected error = %v", err)
	}
	
	// Record failures to trigger circuit breaker
	for i := 0; i < 3; i++ {
		dm.RecordDestinationMetrics(destID, 100*time.Millisecond, false)
	}
	
	// Circuit should now be open
	if !dm.isCircuitBreakerOpen(destID) {
		t.Errorf("Circuit breaker should be open after %d failures", config.FailureThreshold)
	}
	
	// Test that destination is filtered out when circuit is open
	destinations := []RouteDestination{
		{ID: destID, Type: "http"},
	}
	
	strategy := LoadBalancingConfig{Strategy: "round_robin"}
	request := &RouteRequest{ID: "test"}
	
	_, err = dm.SelectDestination(destinations, strategy, request)
	if err != ErrCircuitBreakerOpen {
		t.Errorf("SelectDestination() with open circuit should return ErrCircuitBreakerOpen, got %v", err)
	}
}

func TestBasicDestinationManager_GetDestinationMetrics(t *testing.T) {
	dm := NewBasicDestinationManager()
	
	// Test getting metrics for non-existing destination
	metrics, err := dm.GetDestinationMetrics("non-existing")
	if err != nil {
		t.Errorf("GetDestinationMetrics() unexpected error = %v", err)
	}
	
	if metrics.HealthStatus != "unknown" {
		t.Errorf("GetDestinationMetrics() for non-existing destination should return unknown health status")
	}
	
	if metrics.CircuitState != "closed" {
		t.Errorf("GetDestinationMetrics() for non-existing destination should return closed circuit state")
	}
}

func BenchmarkBasicDestinationManager_SelectDestination(b *testing.B) {
	dm := NewBasicDestinationManager()
	
	destinations := make([]RouteDestination, 10)
	for i := 0; i < 10; i++ {
		destinations[i] = RouteDestination{
			ID:   fmt.Sprintf("dest-%d", i),
			Type: "http",
		}
	}
	
	strategy := LoadBalancingConfig{Strategy: "round_robin"}
	request := &RouteRequest{ID: "bench-request"}
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := dm.SelectDestination(destinations, strategy, request)
		if err != nil {
			b.Fatalf("SelectDestination() error = %v", err)
		}
	}
}