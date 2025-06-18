package storage

import (
	"encoding/json"
	"fmt"
	"webhook-router/internal/crypto"
)

// EncryptedConfig handles encryption/decryption of configuration data
type EncryptedConfig struct {
	encryptor *crypto.ConfigEncryptor
}

// NewEncryptedConfig creates a new encrypted config handler
func NewEncryptedConfig(encryptionKey string) (*EncryptedConfig, error) {
	if encryptionKey == "" {
		// No encryption if key not provided (development mode)
		return &EncryptedConfig{}, nil
	}
	
	encryptor, err := crypto.NewConfigEncryptor(encryptionKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create encryptor: %w", err)
	}
	
	return &EncryptedConfig{encryptor: encryptor}, nil
}

// EncryptConfig encrypts a configuration map
func (ec *EncryptedConfig) EncryptConfig(config map[string]interface{}) (string, error) {
	if ec.encryptor == nil {
		// No encryption, just marshal to JSON
		data, err := json.Marshal(config)
		return string(data), err
	}
	
	// Marshal to JSON
	data, err := json.Marshal(config)
	if err != nil {
		return "", fmt.Errorf("failed to marshal config: %w", err)
	}
	
	// Encrypt
	return ec.encryptor.Encrypt(string(data))
}

// DecryptConfig decrypts a configuration string
func (ec *EncryptedConfig) DecryptConfig(encrypted string) (map[string]interface{}, error) {
	var configData string
	var err error
	
	if ec.encryptor == nil {
		// No decryption needed
		configData = encrypted
	} else {
		// Decrypt
		configData, err = ec.encryptor.Decrypt(encrypted)
		if err != nil {
			return nil, fmt.Errorf("failed to decrypt config: %w", err)
		}
	}
	
	// Unmarshal JSON
	var config map[string]interface{}
	if err := json.Unmarshal([]byte(configData), &config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}
	
	return config, nil
}

// EncryptSensitiveFields encrypts only sensitive fields in a config
func (ec *EncryptedConfig) EncryptSensitiveFields(config map[string]interface{}, sensitiveFields []string) (map[string]interface{}, error) {
	if ec.encryptor == nil {
		return config, nil
	}
	
	result := make(map[string]interface{})
	for k, v := range config {
		result[k] = v
	}
	
	// Encrypt sensitive fields
	for _, field := range sensitiveFields {
		if val, exists := result[field]; exists {
			if strVal, ok := val.(string); ok && strVal != "" {
				encrypted, err := ec.encryptor.Encrypt(strVal)
				if err != nil {
					return nil, fmt.Errorf("failed to encrypt field %s: %w", field, err)
				}
				result[field] = encrypted
			}
		}
	}
	
	return result, nil
}

// DecryptSensitiveFields decrypts only sensitive fields in a config
func (ec *EncryptedConfig) DecryptSensitiveFields(config map[string]interface{}, sensitiveFields []string) (map[string]interface{}, error) {
	if ec.encryptor == nil {
		return config, nil
	}
	
	result := make(map[string]interface{})
	for k, v := range config {
		result[k] = v
	}
	
	// Decrypt sensitive fields
	for _, field := range sensitiveFields {
		if val, exists := result[field]; exists {
			if strVal, ok := val.(string); ok && strVal != "" {
				decrypted, err := ec.encryptor.Decrypt(strVal)
				if err != nil {
					return nil, fmt.Errorf("failed to decrypt field %s: %w", field, err)
				}
				result[field] = decrypted
			}
		}
	}
	
	return result, nil
}