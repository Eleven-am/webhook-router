// Package utils provides utility functions for the webhook router.
//
// This package contains common utilities for ID generation, retry logic,
// time manipulation, and other helper functions used throughout the application.
//
// Features:
//   - Cryptographically secure ID generation
//   - UUID v4 generation with proper formatting
//   - Event and request ID generation with timestamps
//   - Exponential backoff retry mechanisms
//   - Extended duration parsing (days, weeks)
//   - Time manipulation and formatting utilities
package utils

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"
)

// GenerateEventID generates a unique event ID with a prefix and trigger ID.
//
// Creates an event ID in the format: "prefix-triggerID-timestamp"
// where timestamp is the current time in nanoseconds since Unix epoch.
//
// Parameters:
//   - prefix: String prefix to identify the event source (can be empty)
//   - triggerID: String identifier for the trigger that generated this event
//
// Returns a unique string ID suitable for event tracking and correlation.
//
// Note: If prefix contains hyphens, parsing the ID later may be ambiguous.
// Consider using prefixes without hyphens for easier parsing.
func GenerateEventID(prefix string, triggerID string) string {
	return fmt.Sprintf("%s-%s-%d", prefix, triggerID, time.Now().UnixNano())
}

// GenerateRandomID generates a cryptographically secure random hex ID.
//
// Creates a random ID of the specified length using crypto/rand.
// The resulting string will contain hexadecimal characters (0-9, a-f).
//
// Parameters:
//   - length: Desired length of the hex string (must be even for proper byte alignment)
//
// Returns:
//   - string: Hex-encoded random ID
//   - error: nil on success, error if random generation fails
//
// For odd lengths, the result will be 1 character shorter due to hex encoding.
// Each byte generates 2 hex characters, so length/2 bytes are generated.
func GenerateRandomID(length int) (string, error) {
	bytes := make([]byte, length/2)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

// GenerateUUID generates a cryptographically secure UUID v4.
//
// Creates a random UUID conforming to RFC 4122 version 4 specification.
// UUID v4 uses random or pseudo-random numbers for all bits except:
//   - Version field (4 bits): Set to 0100 (binary) = 4
//   - Variant field (2 bits): Set to 10 (binary)
//
// Returns:
//   - string: UUID in the format "xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx"
//   - error: nil on success, error if random generation fails
//
// The returned UUID is suitable for use as a unique identifier in
// distributed systems and databases.
func GenerateUUID() (string, error) {
	uuid := make([]byte, 16)
	if _, err := rand.Read(uuid); err != nil {
		return "", err
	}

	// Set version (4) and variant bits
	uuid[6] = (uuid[6] & 0x0f) | 0x40
	uuid[8] = (uuid[8] & 0x3f) | 0x80

	return fmt.Sprintf("%x-%x-%x-%x-%x",
		uuid[0:4], uuid[4:6], uuid[6:8], uuid[8:10], uuid[10:]), nil
}

// GenerateRequestID generates a unique request ID for tracing and correlation.
//
// Creates a request ID in the format: "req-{randomHex}-{timestamp}"
// where randomHex is a 16-character random hex string and timestamp
// is the current Unix timestamp.
//
// Returns:
//   - string: Request ID suitable for distributed tracing
//   - error: nil on success, error if random generation fails
//
// The request ID is designed to be:
//   - Unique across distributed systems
//   - Sortable by creation time (timestamp suffix)
//   - Easily identifiable as a request ID (req- prefix)
func GenerateRequestID() (string, error) {
	id, err := GenerateRandomID(16)
	if err != nil {
		return "", fmt.Errorf("failed to generate random ID: %w", err)
	}
	return fmt.Sprintf("req-%s-%d", id, time.Now().Unix()), nil
}

// MustGenerateRequestID generates a request ID or panics on failure.
//
// Convenience function that wraps GenerateRequestID() and panics if
// an error occurs. Use this when request ID generation failure is
// considered a fatal error that should stop program execution.
//
// Returns a request ID string in the same format as GenerateRequestID().
//
// Panics if random ID generation fails, which typically indicates
// system-level issues with the random number generator.
func MustGenerateRequestID() string {
	id, err := GenerateRequestID()
	if err != nil {
		panic(fmt.Sprintf("failed to generate request ID: %v", err))
	}
	return id
}
