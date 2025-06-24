package expression

import (
	"bytes"
	"fmt"
	"net"
	"net/url"
	"strings"

	"github.com/expr-lang/expr"
)

// SafeExprOptions returns expression options with sandboxing enabled
func GetSafeExprOptions(env map[string]interface{}) []expr.Option {
	baseOptions := GetExprOptions(env)

	// Add sandboxing options
	safeOptions := append(baseOptions,
		// Disable access to built-in functions that could be dangerous
		// Note: 'len' is safe and commonly needed, so we don't disable it
		expr.DisableBuiltin("make"),
		expr.DisableBuiltin("new"),
		expr.DisableBuiltin("panic"),
		expr.DisableBuiltin("recover"),
		expr.DisableBuiltin("close"),
		expr.DisableBuiltin("delete"),

		// Only allow safe operations
		expr.AllowUndefinedVariables(),

		// Add type checking
		expr.AsBool(),
	)

	return safeOptions
}

// ValidateExpression checks if an expression is safe to execute
func ValidateExpression(expression string) error {
	// Check for potentially dangerous patterns
	dangerousPatterns := []string{
		"__",      // Double underscore (often used for internals)
		"import",  // Import statements
		"eval",    // Eval functions
		"exec",    // Exec functions
		"system",  // System calls
		"syscall", // System calls
		"unsafe",  // Unsafe operations
		"reflect", // Reflection
		"runtime", // Runtime manipulation
	}

	lowerExpr := strings.ToLower(expression)
	for _, pattern := range dangerousPatterns {
		if strings.Contains(lowerExpr, pattern) {
			return fmt.Errorf("expression contains potentially dangerous pattern: %s", pattern)
		}
	}

	// Try to compile with safe options to validate
	_, err := expr.Compile(expression, GetSafeExprOptions(nil)...)
	if err != nil {
		return fmt.Errorf("invalid expression: %w", err)
	}

	return nil
}

// ValidateURL validates a URL for safety
func ValidateURL(urlStr string) error {
	// Parse the URL
	u, err := url.Parse(urlStr)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}

	// Check scheme
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("only HTTP(S) URLs are allowed, got: %s", u.Scheme)
	}

	// Check for local/internal addresses (SSRF prevention)
	host := strings.ToLower(u.Hostname())

	// Block localhost and common internal hostnames
	blockedHosts := []string{
		"localhost",
		"127.0.0.1",
		"0.0.0.0",
		"::1",
		"169.254.169.254",          // AWS metadata endpoint
		"metadata.google.internal", // GCP metadata endpoint
		"metadata.azure.com",       // Azure metadata endpoint
	}

	for _, blocked := range blockedHosts {
		if host == blocked {
			return fmt.Errorf("URL points to blocked host: %s", host)
		}
	}

	// Resolve hostname to IP addresses for better validation
	ips, err := net.LookupIP(host)
	if err == nil {
		// Check each resolved IP
		for _, ip := range ips {
			if isPrivateOrReservedIP(ip) {
				return fmt.Errorf("URL resolves to private/reserved IP address: %s", ip.String())
			}
		}
	} else {
		// If DNS resolution fails, still check if it looks like an IP
		if ip := net.ParseIP(host); ip != nil {
			if isPrivateOrReservedIP(ip) {
				return fmt.Errorf("URL points to private/reserved IP address: %s", host)
			}
		}
	}

	// Block file:// and other dangerous schemes that might have been missed
	if u.Scheme == "file" || u.Scheme == "ftp" || u.Scheme == "gopher" {
		return fmt.Errorf("URL scheme not allowed: %s", u.Scheme)
	}

	// Additional validation for ports
	if u.Port() != "" {
		// You might want to restrict certain ports
		blockedPorts := []string{"22", "23", "25", "445", "3389", "5432", "3306", "6379", "27017"}
		for _, port := range blockedPorts {
			if u.Port() == port {
				return fmt.Errorf("URL points to blocked port: %s", port)
			}
		}
	}

	return nil
}

// isPrivateOrReservedIP checks if an IP address is private or reserved
func isPrivateOrReservedIP(ip net.IP) bool {
	// Check for IPv4 private ranges
	privateIPv4Ranges := []struct {
		start net.IP
		end   net.IP
	}{
		{net.IPv4(10, 0, 0, 0), net.IPv4(10, 255, 255, 255)},         // 10.0.0.0/8
		{net.IPv4(172, 16, 0, 0), net.IPv4(172, 31, 255, 255)},       // 172.16.0.0/12
		{net.IPv4(192, 168, 0, 0), net.IPv4(192, 168, 255, 255)},     // 192.168.0.0/16
		{net.IPv4(127, 0, 0, 0), net.IPv4(127, 255, 255, 255)},       // 127.0.0.0/8 (loopback)
		{net.IPv4(169, 254, 0, 0), net.IPv4(169, 254, 255, 255)},     // 169.254.0.0/16 (link-local)
		{net.IPv4(224, 0, 0, 0), net.IPv4(239, 255, 255, 255)},       // 224.0.0.0/4 (multicast)
		{net.IPv4(255, 255, 255, 255), net.IPv4(255, 255, 255, 255)}, // 255.255.255.255 (broadcast)
		{net.IPv4(0, 0, 0, 0), net.IPv4(0, 255, 255, 255)},           // 0.0.0.0/8
	}

	// Convert to IPv4 if it's IPv4-mapped IPv6
	if ip4 := ip.To4(); ip4 != nil {
		for _, r := range privateIPv4Ranges {
			if bytes.Compare(ip4, r.start) >= 0 && bytes.Compare(ip4, r.end) <= 0 {
				return true
			}
		}
		return false
	}

	// Check for IPv6 private/reserved ranges
	if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsInterfaceLocalMulticast() || ip.IsMulticast() || ip.IsUnspecified() {
		return true
	}

	// Check for IPv6 unique local addresses (fc00::/7)
	if len(ip) == net.IPv6len && (ip[0] == 0xfc || ip[0] == 0xfd) {
		return true
	}

	return false
}
