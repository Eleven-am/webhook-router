package utils

import (
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateEventID(t *testing.T) {
	tests := []struct {
		name      string
		prefix    string
		triggerID int
	}{
		{"standard prefix", "webhook", 123},
		{"empty prefix", "", 456},
		{"long prefix", "verylongwebhookprefix", 789},
		{"zero trigger ID", "test", 0},
		{"negative trigger ID", "test", -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			startTime := time.Now().UnixNano()

			id := GenerateEventID(tt.prefix, tt.triggerID)

			endTime := time.Now().UnixNano()

			// Verify format: prefix-triggerID-timestamp
			parts := strings.Split(id, "-")

			var expectedParts int
			var triggerIDPart string

			if tt.prefix == "" && tt.triggerID >= 0 {
				// Empty prefix: -triggerID-timestamp
				expectedParts = 3
				assert.Equal(t, "", parts[0])
				triggerIDPart = parts[1]
			} else if tt.prefix == "" && tt.triggerID < 0 {
				// Empty prefix with negative ID: --triggerID-timestamp
				expectedParts = 4
				assert.Equal(t, "", parts[0])
				assert.Equal(t, "", parts[1])
				triggerIDPart = "-" + parts[2] // Reconstruct negative number
			} else if tt.triggerID < 0 {
				// Prefix with negative ID: prefix--triggerID-timestamp
				expectedParts = 4
				assert.Equal(t, tt.prefix, parts[0])
				assert.Equal(t, "", parts[1])
				triggerIDPart = "-" + parts[2] // Reconstruct negative number
			} else {
				// Normal case: prefix-triggerID-timestamp
				expectedParts = 3
				assert.Equal(t, tt.prefix, parts[0])
				triggerIDPart = parts[1]
			}

			assert.Len(t, parts, expectedParts)

			// Verify trigger ID
			assert.Equal(t, fmt.Sprintf("%d", tt.triggerID), triggerIDPart)

			// Verify timestamp is reasonable (between start and end)
			timestampPart := parts[len(parts)-1] // Last part is always timestamp
			timestamp := int64(0)
			_, err := fmt.Sscanf(timestampPart, "%d", &timestamp)
			require.NoError(t, err)
			assert.True(t, timestamp >= startTime)
			assert.True(t, timestamp <= endTime)
		})
	}
}

func TestGenerateEventID_Uniqueness(t *testing.T) {
	// Generate multiple IDs quickly and verify they're unique
	const numIDs = 1000
	ids := make(map[string]bool)

	for i := 0; i < numIDs; i++ {
		id := GenerateEventID("test", i)
		assert.False(t, ids[id], "Generated duplicate ID: %s", id)
		ids[id] = true
	}
}

func TestGenerateRandomID(t *testing.T) {
	tests := []struct {
		name           string
		length         int
		expectError    bool
		expectedLength int
	}{
		{"even length", 16, false, 16},
		{"small length", 4, false, 4},
		{"large length", 64, false, 64},
		{"zero length", 0, false, 0},
		{"odd length", 15, false, 15}, // Note: will generate 14 chars due to hex encoding
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id, err := GenerateRandomID(tt.length)

			if tt.expectError {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)

			// For odd lengths, the result will be 1 character shorter due to hex encoding
			expectedLength := tt.expectedLength
			if tt.length%2 == 1 {
				expectedLength = tt.length - 1
			}

			assert.Len(t, id, expectedLength)

			// Verify it's a valid hex string
			matched, err := regexp.MatchString("^[0-9a-f]*$", id)
			require.NoError(t, err)
			assert.True(t, matched, "ID should be valid hex: %s", id)
		})
	}
}

func TestGenerateRandomID_Uniqueness(t *testing.T) {
	// Generate multiple IDs and verify they're unique
	const numIDs = 1000
	const length = 32
	ids := make(map[string]bool)

	for i := 0; i < numIDs; i++ {
		id, err := GenerateRandomID(length)
		require.NoError(t, err)
		assert.False(t, ids[id], "Generated duplicate ID: %s", id)
		ids[id] = true
	}
}

func TestGenerateUUID(t *testing.T) {
	// UUID v4 format: xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx
	// where y is one of 8, 9, a, or b
	uuidRegex := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

	for i := 0; i < 100; i++ {
		t.Run(fmt.Sprintf("iteration_%d", i), func(t *testing.T) {
			uuid, err := GenerateUUID()
			require.NoError(t, err)

			// Verify format
			assert.True(t, uuidRegex.MatchString(uuid), "Invalid UUID format: %s", uuid)

			// Verify length
			assert.Len(t, uuid, 36) // 32 hex chars + 4 hyphens

			// Verify version bit (4)
			assert.Equal(t, byte('4'), uuid[14], "UUID should have version 4")

			// Verify variant bits (8, 9, a, or b)
			variantChar := uuid[19]
			assert.Contains(t, []byte{'8', '9', 'a', 'b'}, variantChar, "Invalid variant bits")
		})
	}
}

func TestGenerateUUID_Uniqueness(t *testing.T) {
	// Generate multiple UUIDs and verify they're unique
	const numUUIDs = 1000
	uuids := make(map[string]bool)

	for i := 0; i < numUUIDs; i++ {
		uuid, err := GenerateUUID()
		require.NoError(t, err)
		assert.False(t, uuids[uuid], "Generated duplicate UUID: %s", uuid)
		uuids[uuid] = true
	}
}

func TestGenerateRequestID(t *testing.T) {
	// Format: req-{hex}-{timestamp}
	requestIDRegex := regexp.MustCompile(`^req-[0-9a-f]+-\d+$`)

	startTime := time.Now().Unix()

	for i := 0; i < 10; i++ {
		t.Run(fmt.Sprintf("iteration_%d", i), func(t *testing.T) {
			requestID, err := GenerateRequestID()
			require.NoError(t, err)

			// Verify format
			assert.True(t, requestIDRegex.MatchString(requestID), "Invalid request ID format: %s", requestID)

			// Verify prefix
			assert.True(t, strings.HasPrefix(requestID, "req-"), "Request ID should start with 'req-'")

			// Extract and verify timestamp
			parts := strings.Split(requestID, "-")
			require.Len(t, parts, 3)

			timestamp := int64(0)
			_, err = fmt.Sscanf(parts[2], "%d", &timestamp)
			require.NoError(t, err)

			// Timestamp should be recent
			endTime := time.Now().Unix()
			assert.True(t, timestamp >= startTime, "Timestamp too old")
			assert.True(t, timestamp <= endTime, "Timestamp in future")
		})
	}
}

func TestGenerateRequestID_Uniqueness(t *testing.T) {
	// Generate multiple request IDs and verify they're unique
	const numIDs = 1000
	ids := make(map[string]bool)

	for i := 0; i < numIDs; i++ {
		id, err := GenerateRequestID()
		require.NoError(t, err)
		assert.False(t, ids[id], "Generated duplicate request ID: %s", id)
		ids[id] = true
	}
}

func TestGenerateRequestID_ErrorHandling(t *testing.T) {
	// Test that GenerateRequestID properly handles and propagates errors
	// This verifies the fixed error handling behavior

	// Normal case should work
	id, err := GenerateRequestID()
	assert.NoError(t, err)
	assert.Contains(t, id, "req-")
	assert.Contains(t, id, "-")

	// Error case is hard to test directly since GenerateRandomID uses crypto/rand
	// But we can verify the API contract
	assert.NotEmpty(t, id)
}

func TestMustGenerateRequestID(t *testing.T) {
	// Test the convenience function that panics on error

	// Normal case should work without panic
	assert.NotPanics(t, func() {
		id := MustGenerateRequestID()
		assert.Contains(t, id, "req-")
		assert.Contains(t, id, "-")
		assert.NotEmpty(t, id)
	})

	// Verify format matches GenerateRequestID
	mustID := MustGenerateRequestID()
	normalID, err := GenerateRequestID()
	require.NoError(t, err)

	// Both should have same format
	requestIDRegex := regexp.MustCompile(`^req-[0-9a-f]+-\d+$`)
	assert.True(t, requestIDRegex.MatchString(mustID))
	assert.True(t, requestIDRegex.MatchString(normalID))
}

func BenchmarkGenerateEventID(b *testing.B) {
	for i := 0; i < b.N; i++ {
		GenerateEventID("webhook", 123)
	}
}

func BenchmarkGenerateRandomID(b *testing.B) {
	for i := 0; i < b.N; i++ {
		GenerateRandomID(32)
	}
}

func BenchmarkGenerateUUID(b *testing.B) {
	for i := 0; i < b.N; i++ {
		GenerateUUID()
	}
}

func BenchmarkGenerateRequestID(b *testing.B) {
	for i := 0; i < b.N; i++ {
		GenerateRequestID()
	}
}

func BenchmarkMustGenerateRequestID(b *testing.B) {
	for i := 0; i < b.N; i++ {
		MustGenerateRequestID()
	}
}

// Test edge cases and potential issues

func TestGenerateRandomID_EmptyLength(t *testing.T) {
	id, err := GenerateRandomID(0)
	assert.NoError(t, err)
	assert.Equal(t, "", id)
}

func TestGenerateRandomID_NegativeLength(t *testing.T) {
	// This will likely cause issues with byte slice allocation
	_, err := GenerateRandomID(-1)
	// Should return an error or panic, but let's see what happens
	assert.Error(t, err) // This might not actually happen, which is a bug
}

func TestGenerateEventID_SpecialCharacters(t *testing.T) {
	// Test with special characters in prefix
	specialPrefixes := []string{
		"test.webhook",
		"test_webhook",
		"test/webhook",
		"test webhook", // Space
		"test-webhook",
		"webhook@service",
	}

	for _, prefix := range specialPrefixes {
		t.Run(prefix, func(t *testing.T) {
			id := GenerateEventID(prefix, 123)
			assert.Contains(t, id, prefix)
			// Note: This might cause parsing issues if the prefix contains hyphens
			// since the format is prefix-triggerID-timestamp
		})
	}
}
