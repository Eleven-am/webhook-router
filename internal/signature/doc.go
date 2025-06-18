// Package signature provides configurable webhook signature verification.
//
// This package implements a flexible signature verification system that can
// handle various webhook providers through configuration rather than
// provider-specific code.
//
// # Features
//
//   - Configurable signature formats (prefix, suffix, custom patterns)
//   - Multiple signature algorithms (HMAC-SHA1, SHA256, SHA512)
//   - Multiple encoding formats (hex, base64)
//   - Timestamp validation for replay attack prevention
//   - Multiple verification methods for key rotation
//   - Flexible signature input construction
//   - Integration with existing auth system
//
// # Configuration Examples
//
// GitHub webhook configuration:
//
//	{
//	  "enabled": true,
//	  "verifications": [{
//	    "header": "X-Hub-Signature-256",
//	    "format": "sha256=${signature}",
//	    "algorithm": "hmac-sha256",
//	    "encoding": "hex",
//	    "secret_source": "env:GITHUB_WEBHOOK_SECRET"
//	  }]
//	}
//
// Stripe webhook configuration:
//
//	{
//	  "enabled": true,
//	  "verifications": [{
//	    "header": "Stripe-Signature",
//	    "format": "t=${timestamp},v1=${signature}",
//	    "algorithm": "hmac-sha256",
//	    "encoding": "hex",
//	    "secret_source": "env:STRIPE_WEBHOOK_SECRET",
//	    "signature_inputs": {
//	      "template": "${timestamp}.${body}"
//	    }
//	  }],
//	  "timestamp_validation": {
//	    "enabled": true,
//	    "tolerance": 300
//	  }
//	}
//
// # Usage
//
// As a standalone verifier:
//
//	config, err := signature.LoadConfig(configJSON)
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	verifier := signature.NewVerifier(config, logger)
//	body, _ := signature.PreserveRequestBody(r)
//	
//	if err := verifier.Verify(r, body); err != nil {
//	    http.Error(w, "Invalid signature", http.StatusUnauthorized)
//	    return
//	}
//
// As part of the auth system:
//
//	authRegistry.Register(signature.NewAuthStrategy(logger))
//	
//	// In route configuration:
//	route.Authentication = config.AuthConfig{
//	    Type: "signature",
//	    Required: true,
//	    Settings: authSettings,
//	}
//
// # Security Considerations
//
//   - Always use HTTPS to prevent signature replay attacks
//   - Enable timestamp validation when supported by the provider
//   - Use environment variables for secrets, never hardcode
//   - Rotate secrets regularly
//   - Use constant-time comparison (built-in)
//
// # Performance Considerations
//
//   - Signatures are computed for each request
//   - Multiple verification methods are tried sequentially
//   - Body must be read and preserved for signature computation
//   - Consider caching for high-traffic endpoints
package signature