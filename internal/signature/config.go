package signature

import (
	"encoding/json"
)

// Config represents the complete signature verification configuration
type Config struct {
	// Enabled determines if signature verification is active
	Enabled bool `json:"enabled"`
	
	// Verifications contains one or more verification methods to try
	// Multiple verifications allow for signature rotation or multiple providers
	Verifications []VerificationConfig `json:"verifications"`
	
	// TimestampValidation configures timestamp-based replay attack prevention
	TimestampValidation *TimestampConfig `json:"timestamp_validation,omitempty"`
	
	// BodyFormat specifies how to process the body for signing
	// Options: "raw" (default), "canonical_json", "base64"
	BodyFormat string `json:"body_format"`
	
	// FailureAction determines what to do on verification failure
	// Options: "reject" (default), "log", "allow"
	FailureAction string `json:"failure_action"`
}

// VerificationConfig defines a single signature verification method
type VerificationConfig struct {
	// Header is the HTTP header containing the signature
	Header string `json:"header"`
	
	// Format is a template for parsing the signature from the header value
	// Examples: "sha256=${signature}", "${signature}", "t=${timestamp},v1=${signature}"
	// Use ${signature} for the signature value, ${timestamp} for timestamp
	Format string `json:"format"`
	
	// Algorithm specifies the signing algorithm
	// Options: "hmac-sha1", "hmac-sha256" (default), "hmac-sha512"
	Algorithm string `json:"algorithm"`
	
	// Encoding specifies how the signature is encoded
	// Options: "hex" (default), "base64"
	Encoding string `json:"encoding"`
	
	// SecretSource specifies where to get the signing secret
	// Format: "type:value" where type can be:
	//   - "env:VAR_NAME" - from environment variable
	//   - "static:value" - direct value (will be encrypted in DB)
	//   - "header:Header-Name" - from request header
	SecretSource string `json:"secret_source"`
	
	// SignatureInputs defines what data to include in the signature
	SignatureInputs *SignatureInputConfig `json:"signature_inputs,omitempty"`
}

// SignatureInputConfig defines what data to include when computing the signature
type SignatureInputConfig struct {
	// IncludeBody whether to include request body (default: true)
	IncludeBody bool `json:"include_body"`
	
	// IncludeHeaders list of header names to include in signature
	IncludeHeaders []string `json:"include_headers,omitempty"`
	
	// IncludeMethod whether to include HTTP method
	IncludeMethod bool `json:"include_method"`
	
	// IncludePath whether to include request path
	IncludePath bool `json:"include_path"`
	
	// IncludeQuery whether to include query string
	IncludeQuery bool `json:"include_query"`
	
	// Separator used between elements when building signature input
	Separator string `json:"separator"`
	
	// Template for building the signature input string
	// Variables: ${method}, ${path}, ${query}, ${body}, ${header:Name}, ${timestamp}
	// Example: "${method}:${path}:${timestamp}:${body}"
	Template string `json:"template,omitempty"`
}

// TimestampConfig configures timestamp validation for replay attack prevention
type TimestampConfig struct {
	// Enabled activates timestamp validation
	Enabled bool `json:"enabled"`
	
	// Header containing the timestamp
	Header string `json:"header"`
	
	// Format of the timestamp
	// Options: "unix" (default), "unix_ms", "iso8601"
	Format string `json:"format"`
	
	// Tolerance is the maximum age of a valid request in seconds
	Tolerance int `json:"tolerance"`
	
	// IncludeInSignature whether timestamp is part of the signature
	IncludeInSignature bool `json:"include_in_signature"`
}

// SetDefaults applies default values to the configuration
func (c *Config) SetDefaults() {
	if c.BodyFormat == "" {
		c.BodyFormat = "raw"
	}
	
	if c.FailureAction == "" {
		c.FailureAction = "reject"
	}
	
	for i := range c.Verifications {
		c.Verifications[i].SetDefaults()
	}
	
	if c.TimestampValidation != nil {
		c.TimestampValidation.SetDefaults()
	}
}

// SetDefaults applies default values to verification config
func (v *VerificationConfig) SetDefaults() {
	if v.Algorithm == "" {
		v.Algorithm = "hmac-sha256"
	}
	
	if v.Encoding == "" {
		v.Encoding = "hex"
	}
	
	if v.Format == "" {
		v.Format = "${signature}"
	}
	
	if v.SignatureInputs == nil {
		v.SignatureInputs = &SignatureInputConfig{
			IncludeBody: true,
			Separator:   "",
		}
	} else {
		v.SignatureInputs.SetDefaults()
	}
}

// SetDefaults applies default values to signature input config
func (s *SignatureInputConfig) SetDefaults() {
	// Default to including body only
	if !s.IncludeMethod && !s.IncludePath && !s.IncludeQuery && len(s.IncludeHeaders) == 0 && s.Template == "" {
		s.IncludeBody = true
	}
	
	if s.Separator == "" && s.Template == "" {
		s.Separator = ""
	}
}

// SetDefaults applies default values to timestamp config
func (t *TimestampConfig) SetDefaults() {
	if t.Format == "" {
		t.Format = "unix"
	}
	
	if t.Tolerance <= 0 {
		t.Tolerance = 300 // 5 minutes default
	}
}

// Validate checks if the configuration is valid
func (c *Config) Validate() error {
	if c.Enabled && len(c.Verifications) == 0 {
		return NewValidationError("at least one verification method required when enabled")
	}
	
	for i, v := range c.Verifications {
		if err := v.Validate(); err != nil {
			return NewValidationError("verification[%d]: %v", i, err)
		}
	}
	
	if c.TimestampValidation != nil && c.TimestampValidation.Enabled {
		if err := c.TimestampValidation.Validate(); err != nil {
			return NewValidationError("timestamp validation: %v", err)
		}
	}
	
	return nil
}

// Validate checks if the verification config is valid
func (v *VerificationConfig) Validate() error {
	if v.Header == "" {
		return NewValidationError("header is required")
	}
	
	if v.SecretSource == "" {
		return NewValidationError("secret source is required")
	}
	
	// Validate algorithm
	switch v.Algorithm {
	case "hmac-sha1", "hmac-sha256", "hmac-sha512":
		// Valid
	default:
		return NewValidationError("unsupported algorithm: %s", v.Algorithm)
	}
	
	// Validate encoding
	switch v.Encoding {
	case "hex", "base64":
		// Valid
	default:
		return NewValidationError("unsupported encoding: %s", v.Encoding)
	}
	
	return nil
}

// Validate checks if the timestamp config is valid
func (t *TimestampConfig) Validate() error {
	if t.Header == "" {
		return NewValidationError("header is required")
	}
	
	switch t.Format {
	case "unix", "unix_ms", "iso8601":
		// Valid
	default:
		return NewValidationError("unsupported timestamp format: %s", t.Format)
	}
	
	return nil
}

// LoadConfig loads signature configuration from JSON
func LoadConfig(data []byte) (*Config, error) {
	var config Config
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, err
	}
	
	config.SetDefaults()
	
	if err := config.Validate(); err != nil {
		return nil, err
	}
	
	return &config, nil
}

// ExampleConfigs returns example configurations for common providers
func ExampleConfigs() map[string]*Config {
	return map[string]*Config{
		"github": {
			Enabled: true,
			Verifications: []VerificationConfig{
				{
					Header:       "X-Hub-Signature-256",
					Format:       "sha256=${signature}",
					Algorithm:    "hmac-sha256",
					Encoding:     "hex",
					SecretSource: "env:GITHUB_WEBHOOK_SECRET",
				},
			},
		},
		"stripe": {
			Enabled: true,
			Verifications: []VerificationConfig{
				{
					Header:       "Stripe-Signature",
					Format:       "t=${timestamp},v1=${signature}",
					Algorithm:    "hmac-sha256",
					Encoding:     "hex",
					SecretSource: "env:STRIPE_WEBHOOK_SECRET",
					SignatureInputs: &SignatureInputConfig{
						Template: "${timestamp}.${body}",
					},
				},
			},
			TimestampValidation: &TimestampConfig{
				Enabled:            true,
				Header:             "Stripe-Signature",
				Format:             "unix",
				Tolerance:          300,
				IncludeInSignature: true,
			},
		},
		"generic": {
			Enabled: true,
			Verifications: []VerificationConfig{
				{
					Header:       "X-Webhook-Signature",
					Format:       "${signature}",
					Algorithm:    "hmac-sha256",
					Encoding:     "hex",
					SecretSource: "env:WEBHOOK_SECRET",
				},
			},
		},
	}
}