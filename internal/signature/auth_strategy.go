package signature

import (
	"encoding/json"
	"fmt"
	"net/http"
	
	"webhook-router/internal/common/errors"
	"webhook-router/internal/common/logging"
)

// AuthStrategy implements the auth.AuthStrategy interface for signature verification
// This allows signature verification to be used alongside other auth methods
type AuthStrategy struct {
	logger logging.Logger
}

// NewAuthStrategy creates a new signature auth strategy
func NewAuthStrategy(logger logging.Logger) *AuthStrategy {
	if logger == nil {
		logger = logging.NewDefaultLogger()
	}
	
	return &AuthStrategy{
		logger: logger,
	}
}

// GetType returns the authentication type identifier
func (s *AuthStrategy) GetType() string {
	return "signature"
}

// Authenticate validates the HTTP request using signature verification
func (s *AuthStrategy) Authenticate(r *http.Request, settings map[string]string) error {
	// Get configuration from settings
	configJSON, ok := settings["config"]
	if !ok {
		return errors.ConfigError("signature auth requires 'config' in settings")
	}
	
	// Parse configuration
	config, err := LoadConfig([]byte(configJSON))
	if err != nil {
		return errors.ConfigError(fmt.Sprintf("invalid signature configuration: %v", err))
	}
	
	// Get body from context (should be preserved by HTTP trigger)
	body, ok := r.Context().Value("body").([]byte)
	if !ok {
		// Try to read body if not in context
		body, err = PreserveRequestBody(r)
		if err != nil {
			return errors.InternalError("failed to read request body", err)
		}
	}
	
	// Create verifier and verify
	verifier := NewVerifier(config, s.logger)
	if err := verifier.Verify(r, body); err != nil {
		return errors.AuthError(err.Error())
	}
	
	return nil
}

// CreateAuthSettings creates auth settings for signature verification
// This is a helper to create the settings map from a Config
func CreateAuthSettings(config *Config) (map[string]string, error) {
	configJSON, err := json.Marshal(config)
	if err != nil {
		return nil, err
	}
	
	return map[string]string{
		"config": string(configJSON),
	}, nil
}