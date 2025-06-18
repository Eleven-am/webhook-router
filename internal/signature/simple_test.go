package signature

import (
	"testing"
	"encoding/json"
)

func TestConfigMarshalling(t *testing.T) {
	config := &Config{
		Enabled: true,
		Verifications: []VerificationConfig{
			{
				Header:       "X-Hub-Signature-256",
				Format:       "sha256=${signature}",
				Algorithm:    "hmac-sha256",
				Encoding:     "hex",
				SecretSource: "static:test-secret",
			},
		},
		BodyFormat:    "raw",
		FailureAction: "reject",
	}

	// Test marshalling
	data, err := json.Marshal(config)
	if err != nil {
		t.Fatalf("Failed to marshal config: %v", err)
	}

	// Test unmarshalling
	var decoded Config
	err = json.Unmarshal(data, &decoded)
	if err != nil {
		t.Fatalf("Failed to unmarshal config: %v", err)
	}

	// Basic validation
	if !decoded.Enabled {
		t.Error("Expected config to be enabled")
	}
	if len(decoded.Verifications) != 1 {
		t.Errorf("Expected 1 verification, got %d", len(decoded.Verifications))
	}
	if decoded.Verifications[0].Header != "X-Hub-Signature-256" {
		t.Errorf("Expected header X-Hub-Signature-256, got %s", decoded.Verifications[0].Header)
	}
}

func TestLoadConfig(t *testing.T) {
	configJSON := `{
		"enabled": true,
		"verifications": [{
			"header": "X-Signature",
			"format": "${signature}",
			"algorithm": "hmac-sha256",
			"encoding": "hex",
			"secret_source": "static:test"
		}],
		"body_format": "raw",
		"failure_action": "reject"
	}`

	config, err := LoadConfig([]byte(configJSON))
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	if !config.Enabled {
		t.Error("Expected config to be enabled")
	}
	if config.BodyFormat != "raw" {
		t.Errorf("Expected body format raw, got %s", config.BodyFormat)
	}
	if config.FailureAction != "reject" {
		t.Errorf("Expected failure action reject, got %s", config.FailureAction)
	}
}