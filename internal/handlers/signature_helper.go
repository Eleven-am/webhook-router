package handlers

import (
	"encoding/json"
	"fmt"

	"webhook-router/internal/crypto"
	"webhook-router/internal/signature"
	"webhook-router/internal/storage"
)

// PrepareSignatureConfig prepares the signature configuration for storage
// It validates the config and encrypts the secret if provided
func PrepareSignatureConfig(route *storage.Route, encryptor *crypto.ConfigEncryptor) error {
	if route.SignatureConfig == "" {
		// No signature configuration
		route.SignatureSecret = ""
		return nil
	}

	// Parse and validate the configuration
	config, err := signature.LoadConfig([]byte(route.SignatureConfig))
	if err != nil {
		return fmt.Errorf("invalid signature configuration: %v", err)
	}

	// Extract and encrypt secrets from the configuration
	secrets := make(map[string]string)
	for i, verification := range config.Verifications {
		// Check if secret is provided inline (static:secret format)
		if len(verification.SecretSource) > 7 && verification.SecretSource[:7] == "static:" {
			secret := verification.SecretSource[7:]
			if secret != "" {
				// Encrypt the secret
				encrypted, err := encryptor.Encrypt(secret)
				if err != nil {
					return fmt.Errorf("failed to encrypt secret: %v", err)
				}

				// Store encrypted secret reference
				secretKey := fmt.Sprintf("verification_%d_secret", i)
				secrets[secretKey] = encrypted

				// Update config to reference the encrypted secret
				config.Verifications[i].SecretSource = "db:" + secretKey
			}
		}
	}

	// Store encrypted secrets as JSON in SignatureSecret field
	if len(secrets) > 0 {
		secretsJSON, err := json.Marshal(secrets)
		if err != nil {
			return fmt.Errorf("failed to marshal secrets: %v", err)
		}
		route.SignatureSecret = string(secretsJSON)
	}

	// Update the configuration with modified secret sources
	configJSON, err := json.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal updated config: %v", err)
	}
	route.SignatureConfig = string(configJSON)

	return nil
}

// BuildSignatureAuthConfig builds authentication config for signature verification
// This is used when setting up HTTP triggers with signature verification
func BuildSignatureAuthConfig(route *storage.Route, encryptor *crypto.ConfigEncryptor) (*signature.Config, error) {
	if route.SignatureConfig == "" {
		return nil, nil
	}

	// Parse the configuration
	config, err := signature.LoadConfig([]byte(route.SignatureConfig))
	if err != nil {
		return nil, fmt.Errorf("failed to load signature config: %v", err)
	}

	// Decrypt secrets if they're stored in the database
	if route.SignatureSecret != "" {
		var secrets map[string]string
		if err := json.Unmarshal([]byte(route.SignatureSecret), &secrets); err != nil {
			return nil, fmt.Errorf("failed to unmarshal secrets: %v", err)
		}

		// Update config with decrypted secrets
		for i, verification := range config.Verifications {
			if len(verification.SecretSource) > 3 && verification.SecretSource[:3] == "db:" {
				secretKey := verification.SecretSource[3:]
				if encryptedSecret, ok := secrets[secretKey]; ok {
					// Decrypt the secret
					decrypted, err := encryptor.Decrypt(encryptedSecret)
					if err != nil {
						return nil, fmt.Errorf("failed to decrypt secret: %v", err)
					}
					// Update to static secret for runtime use
					config.Verifications[i].SecretSource = "static:" + decrypted
				}
			}
		}
	}

	return config, nil
}
