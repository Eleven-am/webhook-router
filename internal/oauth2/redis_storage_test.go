package oauth2

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

// Mock Redis client for testing
type mockRedisClient struct {
	data  map[string]string
	ttls  map[string]time.Time
	zsets map[string]map[string]float64 // zset_key -> member -> score
	mu    sync.RWMutex
}

// Ensure mockRedisClient implements RedisInterface
var _ RedisInterface = (*mockRedisClient)(nil)

func newMockRedisClient() *mockRedisClient {
	return &mockRedisClient{
		data:  make(map[string]string),
		ttls:  make(map[string]time.Time),
		zsets: make(map[string]map[string]float64),
	}
}

func (m *mockRedisClient) Get(ctx context.Context, key string) (string, error) {
	m.mu.Lock() // Use write lock for potential cleanup
	defer m.mu.Unlock()

	// Check TTL
	if ttl, exists := m.ttls[key]; exists && time.Now().After(ttl) {
		delete(m.data, key)
		delete(m.ttls, key)
		return "", errors.New("key not found")
	}

	value, exists := m.data[key]
	if !exists {
		return "", errors.New("key not found")
	}
	return value, nil
}

func (m *mockRedisClient) Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	var strValue string
	switch v := value.(type) {
	case string:
		strValue = v
	case []byte:
		strValue = string(v)
	default:
		b, _ := json.Marshal(v)
		strValue = string(b)
	}

	m.data[key] = strValue
	if ttl > 0 {
		m.ttls[key] = time.Now().Add(ttl)
	}
	return nil
}

func (m *mockRedisClient) Delete(ctx context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.data, key)
	delete(m.ttls, key)
	return nil
}

func (m *mockRedisClient) ZAdd(ctx context.Context, key string, members ...interface{}) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.zsets[key] == nil {
		m.zsets[key] = make(map[string]float64)
	}

	// Parse score, member pairs
	for i := 0; i < len(members); i += 2 {
		if i+1 >= len(members) {
			break
		}
		score, ok1 := members[i].(float64)
		member, ok2 := members[i+1].(string)
		if ok1 && ok2 {
			m.zsets[key][member] = score
		}
	}
	return nil
}

func (m *mockRedisClient) ZRangeByScore(ctx context.Context, key string, min, max string) ([]string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	zset, exists := m.zsets[key]
	if !exists {
		return []string{}, nil
	}

	var result []string
	for member, score := range zset {
		// Simple range check (assuming numeric values)
		if min == "-inf" || score >= parseFloat(min) {
			if max == "+inf" || score <= parseFloat(max) {
				result = append(result, member)
			}
		}
	}
	return result, nil
}

func (m *mockRedisClient) ZRem(ctx context.Context, key string, members ...interface{}) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	zset, exists := m.zsets[key]
	if !exists {
		return nil
	}

	for _, member := range members {
		if str, ok := member.(string); ok {
			delete(zset, str)
		}
	}
	return nil
}

func (m *mockRedisClient) SetNX(ctx context.Context, key string, value interface{}, ttl time.Duration) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if key already exists
	if _, exists := m.data[key]; exists {
		return false, nil
	}

	// Set the key
	var strValue string
	switch v := value.(type) {
	case string:
		strValue = v
	default:
		b, _ := json.Marshal(v)
		strValue = string(b)
	}

	m.data[key] = strValue
	if ttl > 0 {
		m.ttls[key] = time.Now().Add(ttl)
	}
	return true, nil
}

func (m *mockRedisClient) Eval(ctx context.Context, script string, keys []string, args ...interface{}) (interface{}, error) {
	// Simple mock implementation for the lease script
	if strings.Contains(script, "ZRANGEBYSCORE") && strings.Contains(script, "SETNX") {
		// Mock the lease acquisition script
		m.mu.Lock()
		defer m.mu.Unlock()

		if len(keys) < 2 {
			return []interface{}{}, nil
		}

		expiryKey := keys[0]
		leasePrefix := keys[1]

		if len(args) < 3 {
			return []interface{}{}, nil
		}

		maxScore := args[0].(string)
		maxCount := args[2].(int)

		// Get candidates from zset
		zset, exists := m.zsets[expiryKey]
		if !exists {
			return []interface{}{}, nil
		}

		var candidates []string
		for member, score := range zset {
			if score <= parseFloat(maxScore) {
				candidates = append(candidates, member)
				if len(candidates) >= maxCount {
					break
				}
			}
		}

		// Try to acquire leases
		var leased []interface{}
		for _, candidate := range candidates {
			leaseKey := leasePrefix + candidate
			if _, exists := m.data[leaseKey]; !exists {
				m.data[leaseKey] = "leased"
				m.ttls[leaseKey] = time.Now().Add(5 * time.Minute)
				leased = append(leased, candidate)
			}
		}

		return leased, nil
	}

	return nil, errors.New("script not supported")
}

// Helper function to parse float from string
func parseFloat(s string) float64 {
	if s == "-inf" {
		return -1e9
	}
	if s == "+inf" {
		return 1e9
	}
	// Simple parsing - in real Redis this would be more robust
	var f float64
	for _, r := range s {
		if r >= '0' && r <= '9' {
			f = f*10 + float64(r-'0')
		}
	}
	return f
}

func TestNewRedisTokenStorage(t *testing.T) {
	mockRedis := newMockRedisClient()
	storage := NewRedisTokenStorage(mockRedis)

	if storage == nil {
		t.Fatal("expected non-nil storage")
	}

	if storage.client == nil {
		t.Error("expected Redis client to be set")
	}

	if storage.prefix != "oauth2:token:" {
		t.Errorf("expected prefix 'oauth2:token:', got %q", storage.prefix)
	}

	expectedTTL := 30 * 24 * time.Hour
	if storage.ttl != expectedTTL {
		t.Errorf("expected TTL %v, got %v", expectedTTL, storage.ttl)
	}
}

func TestRedisTokenStorage_SaveAndLoadToken(t *testing.T) {
	mockRedis := newMockRedisClient()
	storage := NewRedisTokenStorage(mockRedis)

	// Test saving and loading token
	token := &Token{
		AccessToken:  "redis-test-token",
		TokenType:    "Bearer",
		RefreshToken: "redis-refresh-token",
		Expiry:       time.Now().Add(2 * time.Hour),
		Scope:        "read write admin",
		IDToken:      "redis-id-token",
	}

	serviceID := "redis-test-service"

	// Save token
	err := storage.SaveToken(context.Background(), serviceID, token)
	if err != nil {
		t.Fatalf("failed to save token: %v", err)
	}

	// Verify token was saved with correct key
	expectedKey := "oauth2:token:" + serviceID
	mockRedis.mu.RLock()
	_, exists := mockRedis.data[expectedKey]
	mockRedis.mu.RUnlock()

	if !exists {
		t.Error("token was not saved to Redis with correct key")
	}

	// Load token
	loadedToken, err := storage.LoadToken(context.Background(), serviceID)
	if err != nil {
		t.Fatalf("failed to load token: %v", err)
	}

	if loadedToken == nil {
		t.Fatal("loaded token is nil")
	}

	// Verify token fields
	if loadedToken.AccessToken != token.AccessToken {
		t.Errorf("expected access token %q, got %q", token.AccessToken, loadedToken.AccessToken)
	}

	if loadedToken.TokenType != token.TokenType {
		t.Errorf("expected token type %q, got %q", token.TokenType, loadedToken.TokenType)
	}

	if loadedToken.RefreshToken != token.RefreshToken {
		t.Errorf("expected refresh token %q, got %q", token.RefreshToken, loadedToken.RefreshToken)
	}

	if loadedToken.Scope != token.Scope {
		t.Errorf("expected scope %q, got %q", token.Scope, loadedToken.Scope)
	}

	if loadedToken.IDToken != token.IDToken {
		t.Errorf("expected ID token %q, got %q", token.IDToken, loadedToken.IDToken)
	}

	// Verify expiry is approximately correct (within 1 second)
	expiryDiff := loadedToken.Expiry.Sub(token.Expiry)
	if expiryDiff > time.Second || expiryDiff < -time.Second {
		t.Errorf("expiry time differs too much: expected %v, got %v", token.Expiry, loadedToken.Expiry)
	}
}

func TestRedisTokenStorage_LoadNonExistentToken(t *testing.T) {
	mockRedis := newMockRedisClient()
	storage := NewRedisTokenStorage(mockRedis)

	// Try to load non-existent token
	token, err := storage.LoadToken(context.Background(), "non-existent-service")
	if err != nil {
		t.Errorf("loading non-existent token should not return error: %v", err)
	}
	if token != nil {
		t.Error("loading non-existent token should return nil")
	}
}

func TestRedisTokenStorage_TTLCalculation(t *testing.T) {
	mockRedis := newMockRedisClient()
	storage := NewRedisTokenStorage(mockRedis)

	tests := []struct {
		name        string
		token       *Token
		expectedTTL time.Duration
		description string
	}{
		{
			name: "token with short expiry",
			token: &Token{
				AccessToken: "short-token",
				Expiry:      time.Now().Add(1 * time.Hour),
			},
			expectedTTL: 25 * time.Hour, // 1 hour + 24 hour buffer
			description: "should use token expiry + 24h buffer",
		},
		{
			name: "token with long expiry",
			token: &Token{
				AccessToken: "long-token",
				Expiry:      time.Now().Add(45 * 24 * time.Hour), // 45 days
			},
			expectedTTL: 30 * 24 * time.Hour, // Default TTL (30 days)
			description: "should use default TTL when token expiry is longer",
		},
		{
			name: "token with zero expiry",
			token: &Token{
				AccessToken: "no-expiry-token",
				Expiry:      time.Time{}, // Zero time
			},
			expectedTTL: 30 * 24 * time.Hour, // Default TTL
			description: "should use default TTL for zero expiry",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			serviceID := "ttl-test-" + tt.name

			err := storage.SaveToken(context.Background(), serviceID, tt.token)
			if err != nil {
				t.Fatalf("failed to save token: %v", err)
			}

			// Check TTL was set correctly in mock Redis
			expectedKey := "oauth2:token:" + serviceID
			mockRedis.mu.RLock()
			ttlTime, hasTTL := mockRedis.ttls[expectedKey]
			mockRedis.mu.RUnlock()

			if !hasTTL {
				t.Error("TTL should be set for token")
				return
			}

			actualTTL := time.Until(ttlTime)

			// Allow for some variance due to execution time
			variance := 10 * time.Second
			if actualTTL < tt.expectedTTL-variance || actualTTL > tt.expectedTTL+variance {
				t.Errorf("TTL variance too large: expected ~%v, got %v", tt.expectedTTL, actualTTL)
			}
		})
	}
}

func TestRedisTokenStorage_DeleteToken(t *testing.T) {
	mockRedis := newMockRedisClient()
	storage := NewRedisTokenStorage(mockRedis)

	token := &Token{
		AccessToken: "delete-test-token",
		TokenType:   "Bearer",
		Expiry:      time.Now().Add(time.Hour),
	}

	serviceID := "delete-test-service"

	// Save token
	err := storage.SaveToken(context.Background(), serviceID, token)
	if err != nil {
		t.Fatalf("failed to save token: %v", err)
	}

	// Verify token exists
	loadedToken, err := storage.LoadToken(context.Background(), serviceID)
	if err != nil {
		t.Fatalf("failed to load token: %v", err)
	}
	if loadedToken == nil {
		t.Fatal("token should exist before deletion")
	}

	// Delete token
	err = storage.DeleteToken(context.Background(), serviceID)
	if err != nil {
		t.Fatalf("failed to delete token: %v", err)
	}

	// Verify token is deleted
	deletedToken, err := storage.LoadToken(context.Background(), serviceID)
	if err != nil {
		t.Errorf("loading deleted token should not return error: %v", err)
	}
	if deletedToken != nil {
		t.Error("deleted token should be nil")
	}

	// Verify token was removed from mock Redis
	expectedKey := "oauth2:token:" + serviceID
	mockRedis.mu.RLock()
	_, exists := mockRedis.data[expectedKey]
	mockRedis.mu.RUnlock()

	if exists {
		t.Error("token should be removed from Redis")
	}
}

func TestRedisTokenStorage_TTLExpiration(t *testing.T) {
	mockRedis := newMockRedisClient()
	storage := NewRedisTokenStorage(mockRedis)

	// Override storage TTL to 50ms for testing
	storage.ttl = 50 * time.Millisecond

	token := &Token{
		AccessToken: "expiry-test-token",
		TokenType:   "Bearer",
		Expiry:      time.Now().Add(30 * 24 * time.Hour), // Long expiry, but storage TTL will be shorter
	}

	serviceID := "expiry-test-service"

	// Save token
	err := storage.SaveToken(context.Background(), serviceID, token)
	if err != nil {
		t.Fatalf("failed to save token: %v", err)
	}

	// Token should be available immediately
	loadedToken, err := storage.LoadToken(context.Background(), serviceID)
	if err != nil {
		t.Fatalf("failed to load token: %v", err)
	}
	if loadedToken == nil {
		t.Fatal("token should be available immediately")
	}

	// Wait for TTL to expire
	time.Sleep(100 * time.Millisecond)

	// Token should be expired and removed by mock Redis
	expiredToken, err := storage.LoadToken(context.Background(), serviceID)
	if err != nil {
		t.Errorf("loading expired token should not return error: %v", err)
	}
	if expiredToken != nil {
		t.Error("expired token should be nil")
	}
}

func TestRedisTokenStorage_MultipleTokens(t *testing.T) {
	mockRedis := newMockRedisClient()
	storage := NewRedisTokenStorage(mockRedis)

	tokens := map[string]*Token{
		"service-a": {
			AccessToken: "token-a",
			TokenType:   "Bearer",
			Expiry:      time.Now().Add(time.Hour),
		},
		"service-b": {
			AccessToken: "token-b",
			TokenType:   "Bearer",
			Expiry:      time.Now().Add(2 * time.Hour),
		},
		"service-c": {
			AccessToken: "token-c",
			TokenType:   "Bearer",
			Expiry:      time.Now().Add(3 * time.Hour),
		},
	}

	// Save all tokens
	for serviceID, token := range tokens {
		err := storage.SaveToken(context.Background(), serviceID, token)
		if err != nil {
			t.Fatalf("failed to save token for %s: %v", serviceID, err)
		}
	}

	// Load and verify all tokens
	for serviceID, expectedToken := range tokens {
		loadedToken, err := storage.LoadToken(context.Background(), serviceID)
		if err != nil {
			t.Fatalf("failed to load token for %s: %v", serviceID, err)
		}

		if loadedToken == nil {
			t.Fatalf("loaded token for %s is nil", serviceID)
		}

		if loadedToken.AccessToken != expectedToken.AccessToken {
			t.Errorf("service %s: expected access token %q, got %q",
				serviceID, expectedToken.AccessToken, loadedToken.AccessToken)
		}
	}

	// Delete one token
	err := storage.DeleteToken(context.Background(), "service-b")
	if err != nil {
		t.Fatalf("failed to delete token: %v", err)
	}

	// Verify service-b is deleted, others remain
	for serviceID, expectedToken := range tokens {
		loadedToken, err := storage.LoadToken(context.Background(), serviceID)
		if err != nil {
			t.Fatalf("failed to load token for %s: %v", serviceID, err)
		}

		if serviceID == "service-b" {
			if loadedToken != nil {
				t.Errorf("service-b token should be deleted")
			}
		} else {
			if loadedToken == nil {
				t.Errorf("service %s token should still exist", serviceID)
			} else if loadedToken.AccessToken != expectedToken.AccessToken {
				t.Errorf("service %s: expected access token %q, got %q",
					serviceID, expectedToken.AccessToken, loadedToken.AccessToken)
			}
		}
	}
}

func TestRedisTokenStorage_OverwriteToken(t *testing.T) {
	mockRedis := newMockRedisClient()
	storage := NewRedisTokenStorage(mockRedis)

	serviceID := "overwrite-service"

	// Save first token
	token1 := &Token{
		AccessToken: "first-redis-token",
		TokenType:   "Bearer",
		Expiry:      time.Now().Add(time.Hour),
	}

	err := storage.SaveToken(context.Background(), serviceID, token1)
	if err != nil {
		t.Fatalf("failed to save first token: %v", err)
	}

	// Save second token (overwrite)
	token2 := &Token{
		AccessToken: "second-redis-token",
		TokenType:   "Bearer",
		Expiry:      time.Now().Add(2 * time.Hour),
	}

	err = storage.SaveToken(context.Background(), serviceID, token2)
	if err != nil {
		t.Fatalf("failed to save second token: %v", err)
	}

	// Load token - should be the second one
	loadedToken, err := storage.LoadToken(context.Background(), serviceID)
	if err != nil {
		t.Fatalf("failed to load token: %v", err)
	}

	if loadedToken == nil {
		t.Fatal("loaded token is nil")
	}

	if loadedToken.AccessToken != token2.AccessToken {
		t.Errorf("expected overwritten token %q, got %q", token2.AccessToken, loadedToken.AccessToken)
	}
}

func TestRedisTokenStorage_JSONSerialization(t *testing.T) {
	mockRedis := newMockRedisClient()
	storage := NewRedisTokenStorage(mockRedis)

	// Test with complex token data
	token := &Token{
		AccessToken:  "test-serialization",
		TokenType:    "Bearer",
		RefreshToken: "refresh-serialization",
		Expiry:       time.Date(2024, 12, 31, 23, 59, 59, 0, time.UTC),
		Scope:        "read write admin",
		IDToken:      "jwt.token.here",
	}

	serviceID := "serialization-test"

	// Save and load token
	err := storage.SaveToken(context.Background(), serviceID, token)
	if err != nil {
		t.Fatalf("failed to save token: %v", err)
	}

	loadedToken, err := storage.LoadToken(context.Background(), serviceID)
	if err != nil {
		t.Fatalf("failed to load token: %v", err)
	}

	// Verify all fields are correctly serialized/deserialized
	if loadedToken.AccessToken != token.AccessToken {
		t.Errorf("access token serialization failed")
	}

	if loadedToken.TokenType != token.TokenType {
		t.Errorf("token type serialization failed")
	}

	if loadedToken.RefreshToken != token.RefreshToken {
		t.Errorf("refresh token serialization failed")
	}

	if loadedToken.Scope != token.Scope {
		t.Errorf("scope serialization failed")
	}

	if loadedToken.IDToken != token.IDToken {
		t.Errorf("ID token serialization failed")
	}

	// Check expiry time (should be exact for this specific time)
	if !loadedToken.Expiry.Equal(token.Expiry) {
		t.Errorf("expiry serialization failed: expected %v, got %v", token.Expiry, loadedToken.Expiry)
	}
}

func TestRedisTokenStorage_InvalidJSON(t *testing.T) {
	mockRedis := newMockRedisClient()
	storage := NewRedisTokenStorage(mockRedis)

	// Manually insert invalid JSON
	serviceID := "invalid-json-service"
	key := "oauth2:token:" + serviceID
	mockRedis.data[key] = "invalid json data"

	// Try to load - should return error
	_, err := storage.LoadToken(context.Background(), serviceID)
	if err == nil {
		t.Error("expected error when loading invalid JSON")
	}
	if err != nil && !strings.Contains(err.Error(), "failed to deserialize") {
		t.Errorf("expected deserialization error, got: %v", err)
	}
}

func TestRedisTokenStorage_EmptyValue(t *testing.T) {
	mockRedis := newMockRedisClient()
	storage := NewRedisTokenStorage(mockRedis)

	// Manually insert empty value
	serviceID := "empty-value-service"
	key := "oauth2:token:" + serviceID
	mockRedis.data[key] = ""

	// Try to load - should return nil
	token, err := storage.LoadToken(context.Background(), serviceID)
	if err != nil {
		t.Errorf("loading empty value should not return error: %v", err)
	}
	if token != nil {
		t.Error("loading empty value should return nil token")
	}
}

func TestRedisTokenStorage_Interface(t *testing.T) {
	// Test that RedisTokenStorage implements TokenStorage interface
	var _ TokenStorage = (*RedisTokenStorage)(nil)
}

func TestRedisTokenStorage_GetExpiringTokens(t *testing.T) {
	mockRedis := newMockRedisClient()
	storage := NewRedisTokenStorage(mockRedis)

	// Create tokens with different expiry times
	now := time.Now()
	tokens := map[string]*Token{
		"service-expiring-soon": {
			AccessToken: "token-1",
			TokenType:   "Bearer",
			Expiry:      now.Add(5 * time.Minute), // Expires soon
		},
		"service-expiring-later": {
			AccessToken: "token-2",
			TokenType:   "Bearer",
			Expiry:      now.Add(2 * time.Hour), // Expires later
		},
		"service-no-expiry": {
			AccessToken: "token-3",
			TokenType:   "Bearer",
			// No expiry set
		},
	}

	// Save tokens (this will add them to expiry tracking)
	for serviceID, token := range tokens {
		err := storage.SaveToken(context.Background(), serviceID, token)
		if err != nil {
			t.Fatalf("failed to save token for %s: %v", serviceID, err)
		}
	}

	// Get tokens expiring within 10 minutes
	threshold := now.Add(10 * time.Minute)
	expiring, err := storage.GetExpiringTokens(context.Background(), threshold)
	if err != nil {
		t.Fatalf("failed to get expiring tokens: %v", err)
	}

	// Should only return service-expiring-soon
	if len(expiring) != 1 {
		t.Errorf("expected 1 expiring token, got %d", len(expiring))
	}

	if len(expiring) > 0 && expiring[0] != "service-expiring-soon" {
		t.Errorf("expected service-expiring-soon, got %s", expiring[0])
	}
}

func TestRedisTokenStorage_LeaseExpiringTokens(t *testing.T) {
	mockRedis := newMockRedisClient()
	storage := NewRedisTokenStorage(mockRedis)

	// Create tokens with near expiry
	now := time.Now()
	tokens := map[string]*Token{
		"service-1": {
			AccessToken: "token-1",
			TokenType:   "Bearer",
			Expiry:      now.Add(5 * time.Minute),
		},
		"service-2": {
			AccessToken: "token-2",
			TokenType:   "Bearer",
			Expiry:      now.Add(8 * time.Minute),
		},
		"service-3": {
			AccessToken: "token-3",
			TokenType:   "Bearer",
			Expiry:      now.Add(15 * time.Minute), // Expires later
		},
	}

	// Save tokens
	for serviceID, token := range tokens {
		err := storage.SaveToken(context.Background(), serviceID, token)
		if err != nil {
			t.Fatalf("failed to save token for %s: %v", serviceID, err)
		}
	}

	// Lease tokens expiring within 10 minutes
	lookahead := 10 * time.Minute
	leaseDuration := 5 * time.Minute
	maxCount := 5

	leasedTokens, err := storage.LeaseExpiringTokens(context.Background(), lookahead, leaseDuration, maxCount)
	if err != nil {
		t.Fatalf("failed to lease expiring tokens: %v", err)
	}

	// Should lease service-1 and service-2
	if len(leasedTokens) != 2 {
		t.Errorf("expected 2 leased tokens, got %d", len(leasedTokens))
	}

	// Verify leased tokens have correct data
	leasedServiceIDs := make(map[string]bool)
	for _, leasedToken := range leasedTokens {
		leasedServiceIDs[leasedToken.ServiceID] = true

		// Verify token data is correct
		expectedToken := tokens[leasedToken.ServiceID]
		if leasedToken.Token.AccessToken != expectedToken.AccessToken {
			t.Errorf("leased token access token mismatch for %s", leasedToken.ServiceID)
		}

		// Verify lease expiry is set
		if leasedToken.LeaseExpiry.IsZero() {
			t.Errorf("lease expiry not set for %s", leasedToken.ServiceID)
		}
	}

	// Check that correct services were leased
	if !leasedServiceIDs["service-1"] || !leasedServiceIDs["service-2"] {
		t.Error("expected service-1 and service-2 to be leased")
	}
	if leasedServiceIDs["service-3"] {
		t.Error("service-3 should not be leased (expires later)")
	}
}

func TestRedisTokenStorage_LeaseExpiringTokens_Concurrent(t *testing.T) {
	mockRedis := newMockRedisClient()
	storage := NewRedisTokenStorage(mockRedis)

	// Create a token
	now := time.Now()
	token := &Token{
		AccessToken: "concurrent-token",
		TokenType:   "Bearer",
		Expiry:      now.Add(5 * time.Minute),
	}

	err := storage.SaveToken(context.Background(), "concurrent-service", token)
	if err != nil {
		t.Fatalf("failed to save token: %v", err)
	}

	// Try to lease the same token twice concurrently
	lookahead := 10 * time.Minute
	leaseDuration := 5 * time.Minute
	maxCount := 1

	// First lease should succeed
	leased1, err := storage.LeaseExpiringTokens(context.Background(), lookahead, leaseDuration, maxCount)
	if err != nil {
		t.Fatalf("first lease failed: %v", err)
	}

	if len(leased1) != 1 {
		t.Errorf("expected 1 leased token, got %d", len(leased1))
	}

	// Second lease should find no available tokens (already leased)
	leased2, err := storage.LeaseExpiringTokens(context.Background(), lookahead, leaseDuration, maxCount)
	if err != nil {
		t.Fatalf("second lease failed: %v", err)
	}

	if len(leased2) != 0 {
		t.Errorf("expected 0 leased tokens (already leased), got %d", len(leased2))
	}
}

func TestRedisTokenStorage_ReleaseLease(t *testing.T) {
	mockRedis := newMockRedisClient()
	storage := NewRedisTokenStorage(mockRedis)

	// Create a token
	now := time.Now()
	token := &Token{
		AccessToken: "release-token",
		TokenType:   "Bearer",
		Expiry:      now.Add(5 * time.Minute),
	}

	serviceID := "release-service"
	err := storage.SaveToken(context.Background(), serviceID, token)
	if err != nil {
		t.Fatalf("failed to save token: %v", err)
	}

	// Lease the token
	lookahead := 10 * time.Minute
	leaseDuration := 5 * time.Minute
	maxCount := 1

	leased, err := storage.LeaseExpiringTokens(context.Background(), lookahead, leaseDuration, maxCount)
	if err != nil {
		t.Fatalf("failed to lease token: %v", err)
	}

	if len(leased) != 1 {
		t.Fatalf("expected 1 leased token, got %d", len(leased))
	}

	// Release the lease
	err = storage.ReleaseLease(context.Background(), serviceID)
	if err != nil {
		t.Fatalf("failed to release lease: %v", err)
	}

	// Try to lease again - should succeed now
	leased2, err := storage.LeaseExpiringTokens(context.Background(), lookahead, leaseDuration, maxCount)
	if err != nil {
		t.Fatalf("failed to lease token after release: %v", err)
	}

	if len(leased2) != 1 {
		t.Errorf("expected 1 leased token after release, got %d", len(leased2))
	}
}

func TestRedisTokenStorage_RemoveFromExpiryIndex(t *testing.T) {
	mockRedis := newMockRedisClient()
	storage := NewRedisTokenStorage(mockRedis)

	// Create a token
	now := time.Now()
	token := &Token{
		AccessToken: "expiry-index-token",
		TokenType:   "Bearer",
		Expiry:      now.Add(5 * time.Minute),
	}

	serviceID := "expiry-index-service"
	err := storage.SaveToken(context.Background(), serviceID, token)
	if err != nil {
		t.Fatalf("failed to save token: %v", err)
	}

	// Verify token is in expiry index
	threshold := now.Add(10 * time.Minute)
	expiring, err := storage.GetExpiringTokens(context.Background(), threshold)
	if err != nil {
		t.Fatalf("failed to get expiring tokens: %v", err)
	}

	if len(expiring) != 1 || expiring[0] != serviceID {
		t.Errorf("expected token to be in expiry index")
	}

	// Remove from expiry index
	err = storage.RemoveFromExpiryIndex(context.Background(), serviceID)
	if err != nil {
		t.Fatalf("failed to remove from expiry index: %v", err)
	}

	// Verify token is no longer in expiry index
	expiring2, err := storage.GetExpiringTokens(context.Background(), threshold)
	if err != nil {
		t.Fatalf("failed to get expiring tokens after removal: %v", err)
	}

	if len(expiring2) != 0 {
		t.Errorf("expected token to be removed from expiry index, but found %d tokens", len(expiring2))
	}
}
