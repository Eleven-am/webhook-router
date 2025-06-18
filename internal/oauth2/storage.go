package oauth2

import (
	"encoding/json"
	"fmt"
	"sync"
)

// SettingsStorage interface defines the contract for basic key-value storage operations.
// This interface abstracts the underlying storage mechanism used by DBTokenStorage,
// allowing it to work with different storage backends (SQLite, PostgreSQL, etc.).
type SettingsStorage interface {
	// GetSetting retrieves a value by key, returns empty string if not found
	GetSetting(key string) (string, error)
	// SetSetting stores a key-value pair, overwrites existing values
	SetSetting(key, value string) error
}

// DBTokenStorage implements TokenStorage using a database backend for persistent token storage.
// Tokens are serialized as JSON and stored in the database using the settings table.
// This implementation is suitable for single-instance deployments where persistence
// across service restarts is required.
type DBTokenStorage struct {
	// store provides the underlying database storage operations
	store SettingsStorage
}

// NewDBTokenStorage creates a new database-backed token storage instance.
// The storage uses the provided SettingsStorage interface to persist tokens
// as JSON in the database settings table with the prefix "oauth2_token_".
//
// Parameters:
//   - store: A SettingsStorage implementation (e.g., SQLite or PostgreSQL storage)
//
// Returns a configured DBTokenStorage ready for use.
func NewDBTokenStorage(store SettingsStorage) *DBTokenStorage {
	return &DBTokenStorage{store: store}
}

// SaveToken persists an OAuth2 token to the database using JSON serialization.
// The token is stored with a key prefix "oauth2_token_" followed by the service ID.
// If a token already exists for the service, it will be overwritten.
//
// Parameters:
//   - serviceID: Unique identifier for the OAuth2 service
//   - token: The OAuth2 token to persist
//
// Returns an error if serialization or database storage fails.
func (s *DBTokenStorage) SaveToken(serviceID string, token *Token) error {
	// Serialize token to JSON
	data, err := json.Marshal(token)
	if err != nil {
		return fmt.Errorf("failed to serialize token: %w", err)
	}
	
	// Save as a setting with a specific prefix
	key := fmt.Sprintf("oauth2_token_%s", serviceID)
	return s.store.SetSetting(key, string(data))
}

// LoadToken retrieves a previously saved OAuth2 token from the database.
// The method deserializes the JSON-stored token data and returns it as a Token struct.
// If no token exists for the service ID, returns nil without error.
//
// Parameters:
//   - serviceID: Unique identifier for the OAuth2 service
//
// Returns the token if found, nil if not found, or an error if deserialization fails.
func (s *DBTokenStorage) LoadToken(serviceID string) (*Token, error) {
	// Load from settings
	key := fmt.Sprintf("oauth2_token_%s", serviceID)
	data, err := s.store.GetSetting(key)
	if err != nil {
		return nil, err
	}
	
	if data == "" {
		return nil, nil
	}
	
	// Deserialize token
	var token Token
	if err := json.Unmarshal([]byte(data), &token); err != nil {
		return nil, fmt.Errorf("failed to deserialize token: %w", err)
	}
	
	return &token, nil
}

// DeleteToken removes an OAuth2 token from the database.
// The token is deleted by setting its value to an empty string in the settings table.
// This method is idempotent - deleting a non-existent token will not return an error.
//
// Parameters:
//   - serviceID: Unique identifier for the OAuth2 service
//
// Returns an error if the database operation fails.
func (s *DBTokenStorage) DeleteToken(serviceID string) error {
	key := fmt.Sprintf("oauth2_token_%s", serviceID)
	return s.store.SetSetting(key, "")
}

// MemoryTokenStorage implements TokenStorage using in-memory storage with thread safety.
// This implementation is suitable for testing, development, and single-instance deployments
// where persistence across service restarts is not required. All tokens are lost when
// the service stops.
//
// The implementation uses sync.RWMutex to ensure thread-safe concurrent access to the
// token map, allowing multiple readers but exclusive writers.
type MemoryTokenStorage struct {
	// tokens stores OAuth2 tokens in memory (service ID -> token)
	tokens map[string]*Token
	// mu provides thread-safe access to the tokens map
	mu sync.RWMutex
}

// NewMemoryTokenStorage creates a new thread-safe in-memory token storage instance.
// The storage initializes an empty token map and is ready for immediate use.
// This storage type is ideal for testing and development environments.
//
// Returns a configured MemoryTokenStorage ready for use.
func NewMemoryTokenStorage() *MemoryTokenStorage {
	return &MemoryTokenStorage{
		tokens: make(map[string]*Token),
	}
}

// SaveToken stores an OAuth2 token in memory with thread-safe access.
// If a token already exists for the service ID, it will be overwritten.
// This operation uses a write lock to ensure thread safety.
//
// Parameters:
//   - serviceID: Unique identifier for the OAuth2 service
//   - token: The OAuth2 token to store in memory
//
// Returns nil as in-memory operations don't typically fail.
func (s *MemoryTokenStorage) SaveToken(serviceID string, token *Token) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tokens[serviceID] = token
	return nil
}

// LoadToken retrieves an OAuth2 token from memory with thread-safe access.
// This operation uses a read lock to allow concurrent read access while ensuring
// consistency. If no token exists for the service ID, returns nil without error.
//
// Parameters:
//   - serviceID: Unique identifier for the OAuth2 service
//
// Returns the token if found, nil if not found, or an error (always nil for memory storage).
func (s *MemoryTokenStorage) LoadToken(serviceID string) (*Token, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	token, exists := s.tokens[serviceID]
	if !exists {
		return nil, nil
	}
	return token, nil
}

// DeleteToken removes an OAuth2 token from memory with thread-safe access.
// This operation uses a write lock to ensure thread safety during deletion.
// The method is idempotent - deleting a non-existent token will not return an error.
//
// Parameters:
//   - serviceID: Unique identifier for the OAuth2 service
//
// Returns nil as in-memory delete operations don't typically fail.
func (s *MemoryTokenStorage) DeleteToken(serviceID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.tokens, serviceID)
	return nil
}