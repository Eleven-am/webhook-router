package signature

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/hex"
	"hash"
	"io"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
	
	"webhook-router/internal/common/errors"
	"webhook-router/internal/common/logging"
)

// Verifier handles webhook signature verification
type Verifier struct {
	config *Config
	logger logging.Logger
}

// NewVerifier creates a new signature verifier
func NewVerifier(config *Config, logger logging.Logger) *Verifier {
	if logger == nil {
		logger = logging.NewDefaultLogger()
	}
	
	return &Verifier{
		config: config,
		logger: logger,
	}
}

// Verify checks if the request has a valid signature
func (v *Verifier) Verify(r *http.Request, body []byte) error {
	if !v.config.Enabled {
		return nil
	}
	
	// Check timestamp if configured
	if v.config.TimestampValidation != nil && v.config.TimestampValidation.Enabled {
		if err := v.validateTimestamp(r); err != nil {
			v.handleFailure("timestamp validation failed", err)
			return err
		}
	}
	
	// Try each verification method
	var lastErr error
	for _, verification := range v.config.Verifications {
		if err := v.verifySignature(r, body, &verification); err == nil {
			v.logger.Debug("Signature verified successfully",
				logging.Field{"header", verification.Header},
			)
			return nil
		} else {
			lastErr = err
			v.logger.Debug("Signature verification failed",
				logging.Field{"header", verification.Header},
				logging.Field{"error", err.Error()},
			)
		}
	}
	
	// All verifications failed
	if lastErr != nil {
		v.handleFailure("all signature verifications failed", lastErr)
		return lastErr
	}
	
	return NewVerificationError("", "no verification methods configured")
}

// verifySignature verifies a single signature
func (v *Verifier) verifySignature(r *http.Request, body []byte, config *VerificationConfig) error {
	// Get signature from header
	headerValue := r.Header.Get(config.Header)
	if headerValue == "" {
		return NewVerificationError(config.Header, "missing signature header")
	}
	
	// Parse signature from header value
	signature, metadata, err := v.parseSignatureFormat(headerValue, config.Format)
	if err != nil {
		return NewVerificationError(config.Header, "failed to parse signature: %v", err)
	}
	
	// Get secret
	secret, err := v.getSecret(config.SecretSource, r)
	if err != nil {
		return NewVerificationError(config.Header, "failed to get secret: %v", err)
	}
	
	// Build signature input
	signatureInput, err := v.buildSignatureInput(r, body, config.SignatureInputs, metadata)
	if err != nil {
		return NewVerificationError(config.Header, "failed to build signature input: %v", err)
	}
	
	// Compute expected signature
	expectedSignature, err := v.computeSignature(signatureInput, secret, config.Algorithm, config.Encoding)
	if err != nil {
		return NewVerificationError(config.Header, "failed to compute signature: %v", err)
	}
	
	// Compare signatures (constant time)
	if !hmac.Equal([]byte(signature), []byte(expectedSignature)) {
		return NewVerificationError(config.Header, "signature mismatch")
	}
	
	return nil
}

// parseSignatureFormat extracts signature and metadata from header value
func (v *Verifier) parseSignatureFormat(headerValue, format string) (signature string, metadata map[string]string, err error) {
	metadata = make(map[string]string)
	
	// Build regex pattern from format
	pattern := regexp.QuoteMeta(format)
	
	// Replace ${var} with capture groups
	varPattern := regexp.MustCompile(`\\\$\\\{(\w+)\\\}`)
	captures := varPattern.FindAllStringSubmatch(pattern, -1)
	
	for _, capture := range captures {
		pattern = strings.Replace(pattern, capture[0], `([^,\s]+)`, 1)
	}
	
	// Match against header value
	re, err := regexp.Compile("^" + pattern + "$")
	if err != nil {
		return "", nil, errors.InternalError("invalid format pattern", err)
	}
	
	matches := re.FindStringSubmatch(headerValue)
	if matches == nil {
		return "", nil, errors.ValidationError("header value doesn't match format")
	}
	
	// Extract captured values
	for i, capture := range captures {
		if i+1 < len(matches) {
			varName := capture[1]
			value := matches[i+1]
			
			if varName == "signature" {
				signature = value
			} else {
				metadata[varName] = value
			}
		}
	}
	
	if signature == "" {
		return "", nil, errors.ValidationError("signature not found in header value")
	}
	
	return signature, metadata, nil
}

// getSecret retrieves the signing secret based on source configuration
func (v *Verifier) getSecret(source string, r *http.Request) (string, error) {
	parts := strings.SplitN(source, ":", 2)
	if len(parts) != 2 {
		return "", errors.ValidationError("invalid secret source format")
	}
	
	sourceType, sourceValue := parts[0], parts[1]
	
	switch sourceType {
	case "env":
		secret := os.Getenv(sourceValue)
		if secret == "" {
			return "", errors.ConfigError("environment variable " + sourceValue + " not set")
		}
		return secret, nil
		
	case "static":
		// In production, this would be decrypted
		return sourceValue, nil
		
	case "header":
		secret := r.Header.Get(sourceValue)
		if secret == "" {
			return "", errors.ValidationError("header " + sourceValue + " not found")
		}
		return secret, nil
		
	default:
		return "", errors.ValidationError("unsupported secret source type: " + sourceType)
	}
}

// buildSignatureInput constructs the data to be signed
func (v *Verifier) buildSignatureInput(r *http.Request, body []byte, config *SignatureInputConfig, metadata map[string]string) ([]byte, error) {
	// If template is specified, use it
	if config.Template != "" {
		return v.buildFromTemplate(r, body, config.Template, metadata)
	}
	
	// Otherwise, build from individual components
	var parts []string
	
	if config.IncludeMethod {
		parts = append(parts, r.Method)
	}
	
	if config.IncludePath {
		parts = append(parts, r.URL.Path)
	}
	
	if config.IncludeQuery {
		parts = append(parts, r.URL.RawQuery)
	}
	
	for _, header := range config.IncludeHeaders {
		parts = append(parts, r.Header.Get(header))
	}
	
	// Build result
	var result []byte
	
	if len(parts) > 0 {
		result = []byte(strings.Join(parts, config.Separator))
		if config.IncludeBody && len(body) > 0 {
			if config.Separator != "" {
				result = append(result, []byte(config.Separator)...)
			}
			result = append(result, body...)
		}
	} else if config.IncludeBody {
		result = body
	}
	
	return result, nil
}

// buildFromTemplate builds signature input using a template
func (v *Verifier) buildFromTemplate(r *http.Request, body []byte, template string, metadata map[string]string) ([]byte, error) {
	result := template
	
	// Replace variables
	replacements := map[string]string{
		"${method}": r.Method,
		"${path}":   r.URL.Path,
		"${query}":  r.URL.RawQuery,
		"${body}":   string(body),
	}
	
	// Add metadata values (e.g., timestamp)
	for key, value := range metadata {
		replacements["${"+key+"}"] = value
	}
	
	// Add headers
	headerPattern := regexp.MustCompile(`\$\{header:([^}]+)\}`)
	headerMatches := headerPattern.FindAllStringSubmatch(result, -1)
	for _, match := range headerMatches {
		headerName := match[1]
		headerValue := r.Header.Get(headerName)
		result = strings.Replace(result, match[0], headerValue, -1)
	}
	
	// Replace all other variables
	for placeholder, value := range replacements {
		result = strings.Replace(result, placeholder, value, -1)
	}
	
	return []byte(result), nil
}

// computeSignature calculates the HMAC signature
func (v *Verifier) computeSignature(data []byte, secret, algorithm, encoding string) (string, error) {
	var h hash.Hash
	
	switch algorithm {
	case "hmac-sha1":
		h = hmac.New(sha1.New, []byte(secret))
	case "hmac-sha256":
		h = hmac.New(sha256.New, []byte(secret))
	case "hmac-sha512":
		h = hmac.New(sha512.New, []byte(secret))
	default:
		return "", errors.ValidationError("unsupported algorithm: " + algorithm)
	}
	
	h.Write(data)
	signature := h.Sum(nil)
	
	switch encoding {
	case "hex":
		return hex.EncodeToString(signature), nil
	case "base64":
		return base64.StdEncoding.EncodeToString(signature), nil
	default:
		return "", errors.ValidationError("unsupported encoding: " + encoding)
	}
}

// validateTimestamp checks if the request timestamp is within tolerance
func (v *Verifier) validateTimestamp(r *http.Request) error {
	config := v.config.TimestampValidation
	
	timestampStr := r.Header.Get(config.Header)
	if timestampStr == "" {
		// Try to extract from signature header if it's included there
		for _, verification := range v.config.Verifications {
			headerValue := r.Header.Get(verification.Header)
			if headerValue != "" {
				_, metadata, _ := v.parseSignatureFormat(headerValue, verification.Format)
				if ts, ok := metadata["timestamp"]; ok {
					timestampStr = ts
					break
				}
			}
		}
		
		if timestampStr == "" {
			return NewVerificationError(config.Header, "missing timestamp")
		}
	}
	
	var timestamp time.Time
	var err error
	
	switch config.Format {
	case "unix":
		ts, err := strconv.ParseInt(timestampStr, 10, 64)
		if err != nil {
			return NewVerificationError(config.Header, "invalid unix timestamp: %v", err)
		}
		timestamp = time.Unix(ts, 0)
		
	case "unix_ms":
		ts, err := strconv.ParseInt(timestampStr, 10, 64)
		if err != nil {
			return NewVerificationError(config.Header, "invalid unix millisecond timestamp: %v", err)
		}
		timestamp = time.Unix(ts/1000, (ts%1000)*1000000)
		
	case "iso8601":
		timestamp, err = time.Parse(time.RFC3339, timestampStr)
		if err != nil {
			return NewVerificationError(config.Header, "invalid ISO8601 timestamp: %v", err)
		}
		
	default:
		return NewVerificationError(config.Header, "unsupported timestamp format: %s", config.Format)
	}
	
	// Check age
	age := time.Since(timestamp)
	if age < 0 {
		age = -age // Handle future timestamps
	}
	
	if age > time.Duration(config.Tolerance)*time.Second {
		return NewVerificationError(config.Header, "timestamp too old: %v", age)
	}
	
	return nil
}

// handleFailure processes verification failures based on configured action
func (v *Verifier) handleFailure(message string, err error) {
	v.logger.Warn("Signature verification failed",
		logging.Field{"message", message},
		logging.Field{"error", err.Error()},
		logging.Field{"action", v.config.FailureAction},
	)
	
	// Additional handling could be added here based on FailureAction
	switch v.config.FailureAction {
	case "reject":
		// Default behavior - return error
	case "log":
		// Log only, don't fail the request
	case "allow":
		// Allow the request to proceed
	}
}

// PreserveRequestBody reads and preserves the request body for signature verification
func PreserveRequestBody(r *http.Request) ([]byte, error) {
	if r.Body == nil {
		return nil, nil
	}
	
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}
	
	// Replace the body with a new reader
	r.Body = io.NopCloser(bytes.NewReader(body))
	
	return body, nil
}