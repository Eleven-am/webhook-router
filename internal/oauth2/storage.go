package oauth2

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"webhook-router/internal/common/errors"
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

// Encryptor defines the interface for encryption operations
type Encryptor interface {
	Encrypt(plaintext string) (string, error)
	Decrypt(ciphertext string) (string, error)
}

// DBTokenStorage implements TokenStorage using a database backend for persistent token storage.
// Tokens are serialized as JSON and stored in the database using the settings table.
// This implementation supports optional encryption for sensitive token data.
type DBTokenStorage struct {
	// store provides the underlying database storage operations
	store SettingsStorage
	// encryptor provides optional encryption for token data (nil = no encryption)
	encryptor Encryptor
}

// NewDBTokenStorage creates a new encrypted database-backed token storage instance.
// SECURITY: Encryption is mandatory for production deployments. Tokens contain sensitive
// OAuth2 credentials and must be encrypted at rest.
//
// Parameters:
//   - store: A SettingsStorage implementation (e.g., SQLite or PostgreSQL storage)
//   - encryptor: An Encryptor implementation for encrypting/decrypting token data (required)
//
// Returns a configured DBTokenStorage with encryption enabled, or an error if encryptor is nil.
func NewDBTokenStorage(store SettingsStorage, encryptor Encryptor) (*DBTokenStorage, error) {
	if encryptor == nil {
		return nil, errors.ValidationError("encryptor is required for DBTokenStorage - sensitive token data must be encrypted")
	}
	return &DBTokenStorage{store: store, encryptor: encryptor}, nil
}

// SaveToken persists an OAuth2 token to the database using JSON serialization and mandatory encryption.
// The token is stored with a key prefix "oauth2_token_" followed by the service ID.
// All token data is encrypted before storage for security.
// If a token already exists for the service, it will be overwritten.
//
// Parameters:
//   - ctx: Context for cancellation (unused in database storage)
//   - serviceID: Unique identifier for the OAuth2 service
//   - token: The OAuth2 token to persist
//
// Returns an error if serialization, encryption, or database storage fails.
func (s *DBTokenStorage) SaveToken(ctx context.Context, serviceID string, token *Token) error {
	// Serialize token to JSON
	data, err := json.Marshal(token)
	if err != nil {
		return fmt.Errorf("failed to serialize token: %w", err)
	}

	// Encrypt token data (mandatory for security)
	encryptedData, err := s.encryptor.Encrypt(string(data))
	if err != nil {
		return fmt.Errorf("failed to encrypt token: %w", err)
	}

	// Save as a setting with a specific prefix
	key := fmt.Sprintf("oauth2_token_%s", serviceID)
	return s.store.SetSetting(key, encryptedData)
}

// LoadToken retrieves a previously saved OAuth2 token from the database.
// The method decrypts and deserializes the token data (encryption is mandatory).
// If no token exists for the service ID, returns nil without error.
//
// Parameters:
//   - ctx: Context for cancellation (unused in database storage)
//   - serviceID: Unique identifier for the OAuth2 service
//
// Returns the token if found, nil if not found, or an error if decryption/deserialization fails.
func (s *DBTokenStorage) LoadToken(ctx context.Context, serviceID string) (*Token, error) {
	// Load from settings
	key := fmt.Sprintf("oauth2_token_%s", serviceID)
	data, err := s.store.GetSetting(key)
	if err != nil {
		return nil, err
	}

	if data == "" {
		return nil, nil
	}

	// Decrypt token data (mandatory)
	decryptedData, err := s.encryptor.Decrypt(data)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt token: %w", err)
	}

	// Deserialize token
	var token Token
	if err := json.Unmarshal([]byte(decryptedData), &token); err != nil {
		return nil, fmt.Errorf("failed to deserialize token: %w", err)
	}

	return &token, nil
}

// DeleteToken removes an OAuth2 token from the database.
// The token is deleted by setting its value to an empty string in the settings table.
// This method is idempotent - deleting a non-existent token will not return an error.
//
// Parameters:
//   - ctx: Context for cancellation (unused in database storage)
//   - serviceID: Unique identifier for the OAuth2 service
//
// Returns an error if the database operation fails.
func (s *DBTokenStorage) DeleteToken(ctx context.Context, serviceID string) error {
	key := fmt.Sprintf("oauth2_token_%s", serviceID)
	return s.store.SetSetting(key, "")
}

// GetExpiringTokens returns service IDs for tokens expiring before the specified time.
// This method is not supported by DBTokenStorage as it doesn't track expiry times.
//
// Parameters:
//   - ctx: Context for cancellation (unused)
//   - beforeTime: Time threshold (unused)
//
// Returns an empty slice as DBTokenStorage doesn't support proactive refresh.
func (s *DBTokenStorage) GetExpiringTokens(ctx context.Context, beforeTime time.Time) ([]string, error) {
	// DBTokenStorage doesn't support expiry tracking, so return empty slice
	// In a production environment, you would want to use a more sophisticated storage
	// like Redis with ZSET for proper expiry tracking
	return []string{}, nil
}

// RemoveFromExpiryIndex removes a service from the expiry tracking index.
// This method is not supported by DBTokenStorage as it doesn't track expiry times.
//
// Parameters:
//   - ctx: Context for cancellation (unused)
//   - serviceID: Unique identifier for the OAuth2 service (unused)
//
// Returns nil as DBTokenStorage doesn't maintain an expiry index.
func (s *DBTokenStorage) RemoveFromExpiryIndex(ctx context.Context, serviceID string) error {
	// DBTokenStorage doesn't support expiry tracking, so this is a no-op
	return nil
}

// LeaseExpiringTokens is not supported by DBTokenStorage as it doesn't track expiry times.
// This method returns an empty slice as DBTokenStorage doesn't support atomic leasing.
//
// Parameters:
//   - ctx: Context for cancellation (unused)
//   - lookahead: Time duration (unused)
//   - leaseDuration: Duration for lease (unused)
//   - maxCount: Maximum tokens to lease (unused)
//
// Returns an empty slice as DBTokenStorage doesn't support proactive refresh.
func (s *DBTokenStorage) LeaseExpiringTokens(ctx context.Context, lookahead time.Duration, leaseDuration time.Duration, maxCount int) ([]*LeasedToken, error) {
	// DBTokenStorage doesn't support atomic leasing, so return empty slice
	// In a production environment, you would want to use Redis with atomic operations
	return []*LeasedToken{}, nil
}

// ReleaseLease is not supported by DBTokenStorage as it doesn't track leases.
// This method is a no-op for DBTokenStorage.
//
// Parameters:
//   - ctx: Context for cancellation (unused)
//   - serviceID: Unique identifier for the OAuth2 service (unused)
//
// Returns nil as DBTokenStorage doesn't maintain lease tracking.
func (s *DBTokenStorage) ReleaseLease(ctx context.Context, serviceID string) error {
	// DBTokenStorage doesn't support leasing, so this is a no-op
	return nil
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
	// expiryIndex tracks tokens by expiry time for proactive refresh (expiry time -> []serviceID)
	expiryIndex map[time.Time][]string
	// leases tracks active leases on tokens (service ID -> lease expiry time)
	leases map[string]time.Time
	// mu provides thread-safe access to the tokens, expiryIndex, and leases maps
	mu sync.RWMutex
}

// NewMemoryTokenStorage creates a new thread-safe in-memory token storage instance.
// The storage initializes an empty token map and is ready for immediate use.
// This storage type is ideal for testing and development environments.
//
// Returns a configured MemoryTokenStorage ready for use.
func NewMemoryTokenStorage() *MemoryTokenStorage {
	return &MemoryTokenStorage{
		tokens:      make(map[string]*Token),
		expiryIndex: make(map[time.Time][]string),
		leases:      make(map[string]time.Time),
	}
}

// SaveToken stores an OAuth2 token in memory with thread-safe access.
// If a token already exists for the service ID, it will be overwritten.
// This operation also updates the expiry index for proactive refresh tracking.
//
// Parameters:
//   - ctx: Context for cancellation (unused in memory storage)
//   - serviceID: Unique identifier for the OAuth2 service
//   - token: The OAuth2 token to store in memory
//
// Returns nil as in-memory operations don't typically fail.
func (s *MemoryTokenStorage) SaveToken(ctx context.Context, serviceID string, token *Token) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Remove from old expiry index if token already exists
	if oldToken, exists := s.tokens[serviceID]; exists && !oldToken.Expiry.IsZero() {
		s.removeFromExpiryIndexUnsafe(serviceID, oldToken.Expiry)
	}

	// Store the new token
	s.tokens[serviceID] = token

	// Add to expiry index if token has expiry
	if !token.Expiry.IsZero() {
		s.addToExpiryIndexUnsafe(serviceID, token.Expiry)
	}

	return nil
}

// LoadToken retrieves an OAuth2 token from memory with thread-safe access.
// This operation uses a read lock to allow concurrent read access while ensuring
// consistency. If no token exists for the service ID, returns nil without error.
//
// Parameters:
//   - ctx: Context for cancellation (unused in memory storage)
//   - serviceID: Unique identifier for the OAuth2 service
//
// Returns the token if found, nil if not found, or an error (always nil for memory storage).
func (s *MemoryTokenStorage) LoadToken(ctx context.Context, serviceID string) (*Token, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	token, exists := s.tokens[serviceID]
	if !exists {
		return nil, nil
	}
	return token, nil
}

// DeleteToken removes an OAuth2 token from memory with thread-safe access.
// This operation also removes the token from the expiry index.
// The method is idempotent - deleting a non-existent token will not return an error.
//
// Parameters:
//   - ctx: Context for cancellation (unused in memory storage)
//   - serviceID: Unique identifier for the OAuth2 service
//
// Returns nil as in-memory delete operations don't typically fail.
func (s *MemoryTokenStorage) DeleteToken(ctx context.Context, serviceID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Remove from expiry index if token exists
	if token, exists := s.tokens[serviceID]; exists && !token.Expiry.IsZero() {
		s.removeFromExpiryIndexUnsafe(serviceID, token.Expiry)
	}

	delete(s.tokens, serviceID)
	return nil
}

// GetExpiringTokens returns service IDs for tokens expiring before the specified time.
// This method is used by the proactive refresh worker to identify tokens that need refreshing.
//
// Parameters:
//   - ctx: Context for cancellation (unused in memory storage)
//   - beforeTime: Time threshold - tokens expiring before this time will be returned
//
// Returns a slice of service IDs for tokens expiring before the threshold time.
func (s *MemoryTokenStorage) GetExpiringTokens(ctx context.Context, beforeTime time.Time) ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var expiringServiceIDs []string

	// Iterate through expiry index to find tokens expiring before the threshold
	for expiryTime, serviceIDs := range s.expiryIndex {
		if expiryTime.Before(beforeTime) {
			expiringServiceIDs = append(expiringServiceIDs, serviceIDs...)
		}
	}

	return expiringServiceIDs, nil
}

// RemoveFromExpiryIndex removes a service from the expiry tracking index.
// This method is used during token deletion and cleanup operations.
//
// Parameters:
//   - ctx: Context for cancellation (unused in memory storage)
//   - serviceID: Unique identifier for the OAuth2 service
//
// Returns nil as in-memory operations don't typically fail.
func (s *MemoryTokenStorage) RemoveFromExpiryIndex(ctx context.Context, serviceID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Find and remove the service from expiry index
	if token, exists := s.tokens[serviceID]; exists && !token.Expiry.IsZero() {
		s.removeFromExpiryIndexUnsafe(serviceID, token.Expiry)
	}

	return nil
}

// addToExpiryIndexUnsafe adds a service to the expiry index (must be called with write lock held)
func (s *MemoryTokenStorage) addToExpiryIndexUnsafe(serviceID string, expiry time.Time) {
	s.expiryIndex[expiry] = append(s.expiryIndex[expiry], serviceID)
}

// removeFromExpiryIndexUnsafe removes a service from the expiry index (must be called with write lock held)
func (s *MemoryTokenStorage) removeFromExpiryIndexUnsafe(serviceID string, expiry time.Time) {
	serviceIDs := s.expiryIndex[expiry]
	for i, id := range serviceIDs {
		if id == serviceID {
			// Remove this service ID from the slice
			s.expiryIndex[expiry] = append(serviceIDs[:i], serviceIDs[i+1:]...)
			break
		}
	}

	// Clean up empty entries
	if len(s.expiryIndex[expiry]) == 0 {
		delete(s.expiryIndex, expiry)
	}
}

// LeaseExpiringTokens atomically finds tokens expiring before lookahead time,
// leases them for the specified duration, and returns them for processing.
// This prevents race conditions where multiple workers process the same token.
func (s *MemoryTokenStorage) LeaseExpiringTokens(ctx context.Context, lookahead time.Duration, leaseDuration time.Duration, maxCount int) ([]*LeasedToken, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(lookahead)
	leaseExpiry := now.Add(leaseDuration)

	var leasedTokens []*LeasedToken

	// Clean up expired leases first
	for serviceID, expiry := range s.leases {
		if now.After(expiry) {
			delete(s.leases, serviceID)
		}
	}

	// Find tokens that are expiring and not currently leased
	for expiryTime, serviceIDs := range s.expiryIndex {
		if expiryTime.Before(cutoff) {
			for _, serviceID := range serviceIDs {
				// Skip if already leased
				if _, isLeased := s.leases[serviceID]; isLeased {
					continue
				}

				// Skip if we've reached max count
				if len(leasedTokens) >= maxCount {
					break
				}

				// Get the token
				token, exists := s.tokens[serviceID]
				if !exists || token == nil {
					continue
				}

				// Double-check the token is actually expiring
				if !token.IsImminentlyExpiring(lookahead) {
					continue
				}

				// Lease the token
				s.leases[serviceID] = leaseExpiry

				leasedTokens = append(leasedTokens, &LeasedToken{
					ServiceID:   serviceID,
					Token:       token,
					LeaseExpiry: leaseExpiry,
				})
			}
		}

		// Break if we've reached max count
		if len(leasedTokens) >= maxCount {
			break
		}
	}

	return leasedTokens, nil
}

// ReleaseLease releases a lease on a token, making it available for processing again.
func (s *MemoryTokenStorage) ReleaseLease(ctx context.Context, serviceID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.leases, serviceID)
	return nil
}
