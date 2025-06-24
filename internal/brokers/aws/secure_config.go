package aws

import (
	"crypto/rand"
	"fmt"
	"sync"
	"time"
	"webhook-router/internal/common/validation"
	"webhook-router/internal/crypto"
)

// SecureConfig extends Config with encrypted credential storage
type SecureConfig struct {
	Region                   string
	EncryptedAccessKeyID     string // Base64-encoded encrypted AccessKeyID
	EncryptedSecretAccessKey string // Base64-encoded encrypted SecretAccessKey
	EncryptedSessionToken    string // Base64-encoded encrypted SessionToken (optional)
	QueueURL                 string // For SQS
	TopicArn                 string // For SNS
	Timeout                  time.Duration
	RetryMax                 int
	VisibilityTimeout        int64 // SQS message visibility timeout in seconds
	WaitTimeSeconds          int64 // SQS long polling wait time
	MaxMessages              int64 // SQS max messages per receive

	// Runtime fields (not persisted)
	encryptor        *crypto.ConfigEncryptor
	decryptedConfig  *Config       // Cache for decrypted credentials
	credentialExpiry time.Time     // When cached credentials expire
	credentialTTL    time.Duration // How long to cache credentials
	mu               sync.RWMutex  // Protects credential cache access
}

// NewSecureConfig creates a new SecureConfig with the provided encryptor
func NewSecureConfig(encryptor *crypto.ConfigEncryptor) *SecureConfig {
	return &SecureConfig{
		Region:            "us-east-1",
		Timeout:           30 * time.Second,
		RetryMax:          3,
		VisibilityTimeout: 30,
		WaitTimeSeconds:   20,
		MaxMessages:       1,
		encryptor:         encryptor,
		credentialTTL:     5 * time.Minute, // Default 5-minute credential cache TTL
	}
}

// SetCredentials encrypts and stores AWS credentials
func (sc *SecureConfig) SetCredentials(accessKeyID, secretAccessKey, sessionToken string) error {
	if sc.encryptor == nil {
		return fmt.Errorf("encryptor not initialized")
	}

	// Encrypt AccessKeyID
	if accessKeyID != "" {
		encrypted, err := sc.encryptor.Encrypt(accessKeyID)
		if err != nil {
			return fmt.Errorf("failed to encrypt access key ID: %w", err)
		}
		sc.EncryptedAccessKeyID = encrypted
	}

	// Encrypt SecretAccessKey
	if secretAccessKey != "" {
		encrypted, err := sc.encryptor.Encrypt(secretAccessKey)
		if err != nil {
			return fmt.Errorf("failed to encrypt secret access key: %w", err)
		}
		sc.EncryptedSecretAccessKey = encrypted
	}

	// Encrypt SessionToken (optional)
	if sessionToken != "" {
		encrypted, err := sc.encryptor.Encrypt(sessionToken)
		if err != nil {
			return fmt.Errorf("failed to encrypt session token: %w", err)
		}
		sc.EncryptedSessionToken = encrypted
	}

	// Clear cached decrypted config to force re-decryption
	sc.securelyWipeCredentialCache()

	return nil
}

// GetDecryptedConfig returns a Config with decrypted credentials
func (sc *SecureConfig) GetDecryptedConfig() (*Config, error) {
	if sc.encryptor == nil {
		return nil, fmt.Errorf("encryptor not initialized")
	}

	sc.mu.RLock()
	// Check if cached config is still valid (not expired)
	if sc.decryptedConfig != nil && time.Now().Before(sc.credentialExpiry) {
		config := sc.decryptedConfig
		sc.mu.RUnlock()
		return config, nil
	}
	sc.mu.RUnlock()

	// Need to decrypt credentials (cache expired or empty)
	sc.mu.Lock()
	defer sc.mu.Unlock()

	// Double-check after acquiring write lock
	if sc.decryptedConfig != nil && time.Now().Before(sc.credentialExpiry) {
		return sc.decryptedConfig, nil
	}

	// Clear any existing cached credentials securely
	sc.unsafeWipeCredentialCache()

	config := &Config{
		Region:            sc.Region,
		QueueURL:          sc.QueueURL,
		TopicArn:          sc.TopicArn,
		VisibilityTimeout: sc.VisibilityTimeout,
		WaitTimeSeconds:   sc.WaitTimeSeconds,
		MaxMessages:       sc.MaxMessages,
	}
	config.Timeout = sc.Timeout
	config.RetryMax = sc.RetryMax

	// Decrypt AccessKeyID
	if sc.EncryptedAccessKeyID != "" {
		decrypted, err := sc.encryptor.Decrypt(sc.EncryptedAccessKeyID)
		if err != nil {
			return nil, fmt.Errorf("failed to decrypt access key ID: %w", err)
		}
		config.AccessKeyID = decrypted
	}

	// Decrypt SecretAccessKey
	if sc.EncryptedSecretAccessKey != "" {
		decrypted, err := sc.encryptor.Decrypt(sc.EncryptedSecretAccessKey)
		if err != nil {
			return nil, fmt.Errorf("failed to decrypt secret access key: %w", err)
		}
		config.SecretAccessKey = decrypted
	}

	// Decrypt SessionToken
	if sc.EncryptedSessionToken != "" {
		decrypted, err := sc.encryptor.Decrypt(sc.EncryptedSessionToken)
		if err != nil {
			return nil, fmt.Errorf("failed to decrypt session token: %w", err)
		}
		config.SessionToken = decrypted
	}

	// Cache the decrypted config with expiry time
	sc.decryptedConfig = config
	sc.credentialExpiry = time.Now().Add(sc.credentialTTL)

	return config, nil
}

// Validate validates the secure configuration
func (sc *SecureConfig) Validate() error {
	v := validation.NewValidatorWithPrefix("AWS secure config")

	// Required fields
	v.RequireString(sc.Region, "region")
	v.RequireString(sc.EncryptedAccessKeyID, "encrypted_access_key_id")
	v.RequireString(sc.EncryptedSecretAccessKey, "encrypted_secret_access_key")

	// Either QueueURL or TopicArn is required
	if sc.QueueURL == "" && sc.TopicArn == "" {
		v.Validate(func() error {
			return fmt.Errorf("either QueueURL (for SQS) or TopicArn (for SNS) is required")
		})
	}

	// Set defaults
	if sc.Timeout <= 0 {
		sc.Timeout = 30 * time.Second
	}

	if sc.RetryMax <= 0 {
		sc.RetryMax = 3
	}

	if sc.VisibilityTimeout <= 0 {
		sc.VisibilityTimeout = 30
	}

	if sc.WaitTimeSeconds < 0 {
		sc.WaitTimeSeconds = 20
	}

	if sc.MaxMessages <= 0 {
		sc.MaxMessages = 1
	}

	// Validate ranges
	v.RequirePositive(sc.RetryMax, "retry_max")
	v.RequireNonNegative(int(sc.VisibilityTimeout), "visibility_timeout")
	v.RequireNonNegative(int(sc.WaitTimeSeconds), "wait_time_seconds")
	v.RequirePositive(int(sc.MaxMessages), "max_messages")

	// Validate that we can decrypt the credentials
	if sc.encryptor != nil {
		_, err := sc.GetDecryptedConfig()
		if err != nil {
			v.Validate(func() error {
				return fmt.Errorf("failed to decrypt credentials: %w", err)
			})
		}
	}

	return v.Error()
}

// GetType returns the broker type
func (sc *SecureConfig) GetType() string {
	return "aws"
}

// GetConnectionString returns a connection string without exposing credentials
func (sc *SecureConfig) GetConnectionString() string {
	if sc.QueueURL != "" {
		return fmt.Sprintf("sqs://%s/***", sc.Region)
	}
	if sc.TopicArn != "" {
		return fmt.Sprintf("sns://%s/***", sc.Region)
	}
	return fmt.Sprintf("aws://%s", sc.Region)
}

// ClearCredentials securely clears any cached decrypted credentials from memory
func (sc *SecureConfig) ClearCredentials() {
	sc.securelyWipeCredentialCache()
}

// SetCredentialTTL sets the time-to-live for cached credentials
func (sc *SecureConfig) SetCredentialTTL(ttl time.Duration) {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	sc.credentialTTL = ttl
	// If TTL is being reduced and current cache would exceed new TTL, clear it
	if sc.decryptedConfig != nil && time.Now().Add(ttl).Before(sc.credentialExpiry) {
		sc.unsafeWipeCredentialCache()
	}
}

// securelyWipeCredentialCache safely clears credentials with proper locking
func (sc *SecureConfig) securelyWipeCredentialCache() {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	sc.unsafeWipeCredentialCache()
}

// unsafeWipeCredentialCache clears credentials without locking (must hold write lock)
func (sc *SecureConfig) unsafeWipeCredentialCache() {
	if sc.decryptedConfig != nil {
		// Securely overwrite sensitive fields with random data first
		sc.secureOverwrite(&sc.decryptedConfig.AccessKeyID)
		sc.secureOverwrite(&sc.decryptedConfig.SecretAccessKey)
		sc.secureOverwrite(&sc.decryptedConfig.SessionToken)

		// Set to empty strings
		sc.decryptedConfig.AccessKeyID = ""
		sc.decryptedConfig.SecretAccessKey = ""
		sc.decryptedConfig.SessionToken = ""
		sc.decryptedConfig = nil
	}
	sc.credentialExpiry = time.Time{} // Reset expiry
}

// secureOverwrite overwrites a string with random data to prevent memory recovery
func (sc *SecureConfig) secureOverwrite(s *string) {
	if s == nil || len(*s) == 0 {
		return
	}

	// Create random data of the same length
	randomData := make([]byte, len(*s))
	if _, err := rand.Read(randomData); err != nil {
		// Fallback to zeros if random generation fails
		for i := range randomData {
			randomData[i] = 0
		}
	}

	// Overwrite the string data
	*s = string(randomData)
}

// FromEnvironment creates a SecureConfig from environment variables
// This is a helper method for development/testing purposes
func FromEnvironment(encryptor *crypto.ConfigEncryptor, region string) (*SecureConfig, error) {
	// In a real implementation, you would read from environment variables
	// and encrypt them. For now, this is a placeholder.
	return &SecureConfig{
		Region:            region,
		Timeout:           30 * time.Second,
		RetryMax:          3,
		VisibilityTimeout: 30,
		WaitTimeSeconds:   20,
		MaxMessages:       1,
		encryptor:         encryptor,
	}, nil
}
