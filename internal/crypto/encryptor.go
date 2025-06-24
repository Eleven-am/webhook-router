// Package crypto provides AES-256-GCM encryption and decryption functionality
// for securing sensitive configuration data such as passwords, API keys, and tokens.
//
// The package uses AES-256-GCM (Galois/Counter Mode) which provides both
// confidentiality and authenticity. Each encryption operation uses a unique
// random nonce to ensure that encrypting the same plaintext multiple times
// produces different ciphertexts.
//
// Example usage:
//
//	encryptor, err := crypto.NewConfigEncryptor("my-32-byte-secret-key-here!!")
//	if err != nil {
//		log.Fatal(err)
//	}
//
//	// Encrypt sensitive data
//	encrypted, err := encryptor.Encrypt("database-password-123")
//	if err != nil {
//		log.Fatal(err)
//	}
//
//	// Decrypt when needed
//	decrypted, err := encryptor.Decrypt(encrypted)
//	if err != nil {
//		log.Fatal(err)
//	}
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"golang.org/x/crypto/pbkdf2"
	"io"
	"webhook-router/internal/common/errors"
)

// ConfigEncryptor handles encryption and decryption of sensitive configuration data
// using AES-256-GCM encryption. It provides authenticated encryption, which means
// both confidentiality and integrity protection for the encrypted data.
//
// The encryptor is safe for concurrent use by multiple goroutines.
type ConfigEncryptor struct {
	key []byte // 32-byte AES-256 encryption key
}

// NewConfigEncryptor creates a new ConfigEncryptor with the provided encryption key.
//
// The key parameter is processed using PBKDF2 key derivation to ensure it's
// exactly 32 bytes for AES-256 and cryptographically strong regardless of input length.
// This is more secure than simple padding or truncation.
//
// For security, use a strong passphrase or random key. The key should be stored
// securely (e.g., in environment variables) and never hardcoded in source code.
//
// Parameters:
//   - key: The encryption key as a string. Must not be empty.
//
// Returns:
//   - *ConfigEncryptor: A new encryptor instance
//   - error: An error if the key is empty
//
// Example:
//
//	encryptor, err := NewConfigEncryptor(os.Getenv("ENCRYPTION_KEY"))
//	if err != nil {
//		return fmt.Errorf("failed to create encryptor: %w", err)
//	}
func NewConfigEncryptor(key string) (*ConfigEncryptor, error) {
	if key == "" {
		return nil, errors.ValidationError("encryption key cannot be empty")
	}

	// Use PBKDF2 to derive a proper 32-byte key from the input
	// This is more secure than simple padding/truncation
	salt := []byte("webhook-router-salt") // Static salt for deterministic key derivation
	derivedKey := pbkdf2.Key([]byte(key), salt, 10000, 32, sha256.New)

	return &ConfigEncryptor{key: derivedKey}, nil
}

// Encrypt encrypts a plaintext string using AES-256-GCM and returns the result
// as a base64-encoded string suitable for storage or transmission.
//
// The encryption process:
// 1. Generates a cryptographically random nonce for each encryption
// 2. Encrypts the plaintext using AES-256-GCM with the nonce
// 3. Prepends the nonce to the ciphertext
// 4. Encodes the entire result (nonce + ciphertext) as base64
//
// Empty strings are returned as empty strings without encryption.
// Each call to Encrypt with the same plaintext will produce different
// ciphertexts due to the random nonce.
//
// Parameters:
//   - plaintext: The string to encrypt. Can be empty.
//
// Returns:
//   - string: Base64-encoded ciphertext, or empty string if input was empty
//   - error: An error if encryption fails
//
// Example:
//
//	encrypted, err := encryptor.Encrypt("secret-password")
//	if err != nil {
//		return fmt.Errorf("encryption failed: %w", err)
//	}
//	// Store encrypted string in database or config file
func (e *ConfigEncryptor) Encrypt(plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}

	block, err := aes.NewCipher(e.key)
	if err != nil {
		return "", errors.InternalError("failed to create cipher", err)
	}

	// Create GCM mode
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", errors.InternalError("failed to create GCM", err)
	}

	// Create nonce
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", errors.InternalError("failed to create nonce", err)
	}

	// Encrypt data
	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)

	// Encode to base64 for storage
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// Decrypt decrypts a base64-encoded ciphertext that was produced by the Encrypt method
// and returns the original plaintext string.
//
// The decryption process:
// 1. Decodes the base64-encoded ciphertext
// 2. Extracts the nonce from the beginning of the decoded data
// 3. Decrypts and authenticates the remaining ciphertext using AES-256-GCM
// 4. Returns the original plaintext
//
// The function performs integrity verification as part of the GCM decryption,
// so tampered or corrupted ciphertexts will result in an error.
//
// Empty strings are returned as empty strings without decryption.
//
// Parameters:
//   - ciphertext: Base64-encoded ciphertext produced by Encrypt. Can be empty.
//
// Returns:
//   - string: The decrypted plaintext, or empty string if input was empty
//   - error: An error if decryption fails due to invalid format, wrong key, or tampering
//
// Example:
//
//	plaintext, err := encryptor.Decrypt(storedEncryptedValue)
//	if err != nil {
//		return fmt.Errorf("decryption failed: %w", err)
//	}
//	// Use plaintext (e.g., database password)
func (e *ConfigEncryptor) Decrypt(ciphertext string) (string, error) {
	if ciphertext == "" {
		return "", nil
	}

	// Decode from base64
	data, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", errors.InternalError("failed to decode ciphertext", err)
	}

	block, err := aes.NewCipher(e.key)
	if err != nil {
		return "", errors.InternalError("failed to create cipher", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", errors.InternalError("failed to create GCM", err)
	}

	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return "", errors.ValidationError("ciphertext too short")
	}

	nonce, ciphertextBytes := data[:nonceSize], data[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertextBytes, nil)
	if err != nil {
		return "", errors.InternalError("failed to decrypt", err)
	}

	return string(plaintext), nil
}

// EncryptJSON encrypts a JSON-serializable object by marshaling it to JSON
// and then encrypting the resulting string.
//
// Parameters:
//   - v: The object to marshal to JSON and encrypt
//
// Returns:
//   - string: The encrypted JSON string
//   - error: Error if JSON marshaling or encryption fails
func (e *ConfigEncryptor) EncryptJSON(v interface{}) (string, error) {
	// Marshal to JSON first
	jsonBytes, err := json.Marshal(v)
	if err != nil {
		return "", errors.InternalError("failed to marshal JSON", err)
	}

	// Encrypt the JSON string
	return e.Encrypt(string(jsonBytes))
}
