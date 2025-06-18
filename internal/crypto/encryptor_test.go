package crypto

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestNewConfigEncryptor(t *testing.T) {
	tests := []struct {
		name      string
		key       string
		wantError bool
	}{
		{
			name:      "valid key",
			key:       "test-encryption-key-32-bytes!!",
			wantError: false,
		},
		{
			name:      "short key",
			key:       "short",
			wantError: false, // Should be padded to 32 bytes
		},
		{
			name:      "long key",
			key:       strings.Repeat("a", 64),
			wantError: false, // Should be truncated to 32 bytes
		},
		{
			name:      "empty key",
			key:       "",
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encryptor, err := NewConfigEncryptor(tt.key)
			
			if tt.wantError {
				if err == nil {
					t.Errorf("NewConfigEncryptor() expected error but got none")
				}
				if encryptor != nil {
					t.Errorf("NewConfigEncryptor() expected nil encryptor but got %v", encryptor)
				}
				return
			}
			
			if err != nil {
				t.Errorf("NewConfigEncryptor() unexpected error = %v", err)
				return
			}
			
			if encryptor == nil {
				t.Errorf("NewConfigEncryptor() returned nil encryptor")
				return
			}
			
			// Verify key is always 32 bytes
			if len(encryptor.key) != 32 {
				t.Errorf("NewConfigEncryptor() key length = %d, want 32", len(encryptor.key))
			}
		})
	}
}

func TestConfigEncryptor_Encrypt(t *testing.T) {
	encryptor, err := NewConfigEncryptor("test-encryption-key-32-bytes!!")
	if err != nil {
		t.Fatalf("Failed to create encryptor: %v", err)
	}

	tests := []struct {
		name      string
		plaintext string
		wantError bool
	}{
		{
			name:      "normal text",
			plaintext: "hello world",
			wantError: false,
		},
		{
			name:      "empty string",
			plaintext: "",
			wantError: false,
		},
		{
			name:      "unicode text",
			plaintext: "こんにちは世界",
			wantError: false,
		},
		{
			name:      "json string",
			plaintext: `{"key": "value", "number": 123}`,
			wantError: false,
		},
		{
			name:      "long text",
			plaintext: strings.Repeat("abcdefgh", 1000),
			wantError: false,
		},
		{
			name:      "special characters",
			plaintext: "!@#$%^&*()_+-=[]{}|;':\",./<>?",
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ciphertext, err := encryptor.Encrypt(tt.plaintext)
			
			if tt.wantError {
				if err == nil {
					t.Errorf("Encrypt() expected error but got none")
				}
				return
			}
			
			if err != nil {
				t.Errorf("Encrypt() unexpected error = %v", err)
				return
			}
			
			// For empty string, expect empty result
			if tt.plaintext == "" {
				if ciphertext != "" {
					t.Errorf("Encrypt() empty string should return empty string, got %q", ciphertext)
				}
				return
			}
			
			// Verify result is base64 encoded
			if _, err := base64.StdEncoding.DecodeString(ciphertext); err != nil {
				t.Errorf("Encrypt() result is not valid base64: %v", err)
			}
			
			// Verify ciphertext is different from plaintext
			if ciphertext == tt.plaintext {
				t.Errorf("Encrypt() ciphertext should be different from plaintext")
			}
			
			// Verify ciphertext is not empty for non-empty plaintext
			if ciphertext == "" {
				t.Errorf("Encrypt() returned empty ciphertext for non-empty plaintext")
			}
		})
	}
}

func TestConfigEncryptor_Decrypt(t *testing.T) {
	encryptor, err := NewConfigEncryptor("test-encryption-key-32-bytes!!")
	if err != nil {
		t.Fatalf("Failed to create encryptor: %v", err)
	}

	tests := []struct {
		name       string
		plaintext  string
		wantError  bool
	}{
		{
			name:      "normal text",
			plaintext: "hello world",
			wantError: false,
		},
		{
			name:      "empty string",
			plaintext: "",
			wantError: false,
		},
		{
			name:      "unicode text",
			plaintext: "こんにちは世界",
			wantError: false,
		},
		{
			name:      "json string",
			plaintext: `{"key": "value", "number": 123}`,
			wantError: false,
		},
		{
			name:      "long text",
			plaintext: strings.Repeat("abcdefgh", 1000),
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// First encrypt
			ciphertext, err := encryptor.Encrypt(tt.plaintext)
			if err != nil {
				t.Fatalf("Failed to encrypt: %v", err)
			}
			
			// Then decrypt
			decrypted, err := encryptor.Decrypt(ciphertext)
			
			if tt.wantError {
				if err == nil {
					t.Errorf("Decrypt() expected error but got none")
				}
				return
			}
			
			if err != nil {
				t.Errorf("Decrypt() unexpected error = %v", err)
				return
			}
			
			// Verify decrypted matches original
			if decrypted != tt.plaintext {
				t.Errorf("Decrypt() = %q, want %q", decrypted, tt.plaintext)
			}
		})
	}
}

func TestConfigEncryptor_DecryptInvalidData(t *testing.T) {
	encryptor, err := NewConfigEncryptor("test-encryption-key-32-bytes!!")
	if err != nil {
		t.Fatalf("Failed to create encryptor: %v", err)
	}

	tests := []struct {
		name       string
		ciphertext string
		wantError  bool
	}{
		{
			name:       "empty string",
			ciphertext: "",
			wantError:  false, // Should return empty string
		},
		{
			name:       "invalid base64",
			ciphertext: "not-base64!@#$",
			wantError:  true,
		},
		{
			name:       "valid base64 but invalid ciphertext",
			ciphertext: base64.StdEncoding.EncodeToString([]byte("invalid")),
			wantError:  true,
		},
		{
			name:       "too short ciphertext",
			ciphertext: base64.StdEncoding.EncodeToString([]byte("abc")),
			wantError:  true,
		},
		{
			name:       "corrupted ciphertext",
			ciphertext: base64.StdEncoding.EncodeToString(make([]byte, 50)), // Valid length but random data
			wantError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := encryptor.Decrypt(tt.ciphertext)
			
			if tt.wantError {
				if err == nil {
					t.Errorf("Decrypt() expected error but got none")
				}
				return
			}
			
			if err != nil {
				t.Errorf("Decrypt() unexpected error = %v", err)
				return
			}
			
			// For empty ciphertext, expect empty result
			if tt.ciphertext == "" && result != "" {
				t.Errorf("Decrypt() empty ciphertext should return empty string, got %q", result)
			}
		})
	}
}

func TestConfigEncryptor_EncryptDecryptRoundTrip(t *testing.T) {
	encryptor, err := NewConfigEncryptor("test-encryption-key-32-bytes!!")
	if err != nil {
		t.Fatalf("Failed to create encryptor: %v", err)
	}

	testCases := []string{
		"simple test",
		"",
		"password123!@#",
		`{"database_password": "secret123", "api_key": "abcdef"}`,
		"unicode: こんにちは世界 🌍",
		strings.Repeat("long string test ", 100),
		"newlines\nand\ttabs\rhere",
	}

	for i, plaintext := range testCases {
		t.Run(fmt.Sprintf("case_%d", i), func(t *testing.T) {
			// Encrypt
			ciphertext, err := encryptor.Encrypt(plaintext)
			if err != nil {
				t.Fatalf("Encrypt() error = %v", err)
			}
			
			// Decrypt
			decrypted, err := encryptor.Decrypt(ciphertext)
			if err != nil {
				t.Fatalf("Decrypt() error = %v", err)
			}
			
			// Verify round trip
			if decrypted != plaintext {
				t.Errorf("Round trip failed: got %q, want %q", decrypted, plaintext)
			}
		})
	}
}

func TestConfigEncryptor_DifferentEncryptors(t *testing.T) {
	encryptor1, err := NewConfigEncryptor("key1-32-bytes-long-for-testing!")
	if err != nil {
		t.Fatalf("Failed to create encryptor1: %v", err)
	}
	
	encryptor2, err := NewConfigEncryptor("key2-32-bytes-long-for-testing!")
	if err != nil {
		t.Fatalf("Failed to create encryptor2: %v", err)
	}

	plaintext := "secret data"
	
	// Encrypt with first encryptor
	ciphertext, err := encryptor1.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt() error = %v", err)
	}
	
	// Try to decrypt with second encryptor (different key)
	_, err = encryptor2.Decrypt(ciphertext)
	if err == nil {
		t.Errorf("Decrypt() with different key should fail but didn't")
	}
	
	// Verify first encryptor can still decrypt
	decrypted, err := encryptor1.Decrypt(ciphertext)
	if err != nil {
		t.Fatalf("Decrypt() with original key failed: %v", err)
	}
	
	if decrypted != plaintext {
		t.Errorf("Decrypt() = %q, want %q", decrypted, plaintext)
	}
}

func TestConfigEncryptor_EncryptionIsRandom(t *testing.T) {
	encryptor, err := NewConfigEncryptor("test-encryption-key-32-bytes!!")
	if err != nil {
		t.Fatalf("Failed to create encryptor: %v", err)
	}

	plaintext := "test data for randomness"
	
	// Encrypt same plaintext multiple times
	ciphertexts := make([]string, 10)
	for i := 0; i < 10; i++ {
		ciphertext, err := encryptor.Encrypt(plaintext)
		if err != nil {
			t.Fatalf("Encrypt() error = %v", err)
		}
		ciphertexts[i] = ciphertext
	}
	
	// Verify all ciphertexts are different (due to random nonce)
	for i := 0; i < len(ciphertexts); i++ {
		for j := i + 1; j < len(ciphertexts); j++ {
			if ciphertexts[i] == ciphertexts[j] {
				t.Errorf("Encryption should be random: ciphertexts[%d] == ciphertexts[%d]", i, j)
			}
		}
	}
	
	// Verify all can be decrypted to same plaintext
	for i, ciphertext := range ciphertexts {
		decrypted, err := encryptor.Decrypt(ciphertext)
		if err != nil {
			t.Fatalf("Decrypt() ciphertext[%d] error = %v", i, err)
		}
		if decrypted != plaintext {
			t.Errorf("Decrypt() ciphertext[%d] = %q, want %q", i, decrypted, plaintext)
		}
	}
}

func TestConfigEncryptor_EncryptJSON(t *testing.T) {
	encryptor, err := NewConfigEncryptor("test-encryption-key-32-bytes!!")
	if err != nil {
		t.Fatalf("Failed to create encryptor: %v", err)
	}

	tests := []struct {
		name string
		data interface{}
	}{
		{
			name: "simple map",
			data: map[string]interface{}{"test": "data"},
		},
		{
			name: "complex struct",
			data: struct {
				Name   string `json:"name"`
				Age    int    `json:"age"`
				Active bool   `json:"active"`
			}{
				Name:   "test user",
				Age:    25,
				Active: true,
			},
		},
		{
			name: "array",
			data: []string{"item1", "item2", "item3"},
		},
		{
			name: "nested data",
			data: map[string]interface{}{
				"user": map[string]interface{}{
					"name": "test",
					"settings": map[string]interface{}{
						"theme": "dark",
						"notifications": true,
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Encrypt JSON
			encrypted, err := encryptor.EncryptJSON(tt.data)
			if err != nil {
				t.Fatalf("EncryptJSON() failed: %v", err)
			}

			if encrypted == "" {
				t.Error("EncryptJSON() returned empty string")
			}

			// Decrypt and verify we can recover the JSON
			decrypted, err := encryptor.Decrypt(encrypted)
			if err != nil {
				t.Fatalf("Decrypt() failed: %v", err)
			}

			// Marshal original data to compare
			originalJSON, err := json.Marshal(tt.data)
			if err != nil {
				t.Fatalf("Failed to marshal original data: %v", err)
			}

			if decrypted != string(originalJSON) {
				t.Errorf("Decrypted JSON doesn't match original. Expected: %s, Got: %s", string(originalJSON), decrypted)
			}
		})
	}
}

func BenchmarkConfigEncryptor_Encrypt(b *testing.B) {
	encryptor, err := NewConfigEncryptor("test-encryption-key-32-bytes!!")
	if err != nil {
		b.Fatalf("Failed to create encryptor: %v", err)
	}
	
	plaintext := "benchmark test data for encryption performance"
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := encryptor.Encrypt(plaintext)
		if err != nil {
			b.Fatalf("Encrypt() error = %v", err)
		}
	}
}

func BenchmarkConfigEncryptor_Decrypt(b *testing.B) {
	encryptor, err := NewConfigEncryptor("test-encryption-key-32-bytes!!")
	if err != nil {
		b.Fatalf("Failed to create encryptor: %v", err)
	}
	
	plaintext := "benchmark test data for decryption performance"
	ciphertext, err := encryptor.Encrypt(plaintext)
	if err != nil {
		b.Fatalf("Failed to encrypt test data: %v", err)
	}
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := encryptor.Decrypt(ciphertext)
		if err != nil {
			b.Fatalf("Decrypt() error = %v", err)
		}
	}
}

func BenchmarkConfigEncryptor_EncryptDecrypt(b *testing.B) {
	encryptor, err := NewConfigEncryptor("test-encryption-key-32-bytes!!")
	if err != nil {
		b.Fatalf("Failed to create encryptor: %v", err)
	}
	
	plaintext := "benchmark test data for full round trip performance"
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ciphertext, err := encryptor.Encrypt(plaintext)
		if err != nil {
			b.Fatalf("Encrypt() error = %v", err)
		}
		
		_, err = encryptor.Decrypt(ciphertext)
		if err != nil {
			b.Fatalf("Decrypt() error = %v", err)
		}
	}
}