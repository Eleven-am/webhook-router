// Package handlers - isolated test of DRY helper functions
//
// This test file tests the DRY helper functions in isolation
// to verify the refactoring works correctly.
package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// Test the core DRY helper functions that don't depend on external packages

func TestExtractTokenIsolated(t *testing.T) {
	h := &Handlers{}

	t.Run("ExtractFromHeader", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/test", nil)
		req.Header.Set("Authorization", "Bearer test-token-123")

		token, source := h.extractToken(req)
		assert.Equal(t, "test-token-123", token)
		assert.Equal(t, "header", source)
	})

	t.Run("ExtractFromCookie", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/test", nil)
		req.AddCookie(&http.Cookie{Name: "token", Value: "cookie-token-456"})

		token, source := h.extractToken(req)
		assert.Equal(t, "cookie-token-456", token)
		assert.Equal(t, "cookie", source)
	})

	t.Run("NoToken", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/test", nil)

		token, source := h.extractToken(req)
		assert.Equal(t, "", token)
		assert.Equal(t, "", source)
	})
}

func TestCookieHelpers(t *testing.T) {
	h := &Handlers{}

	t.Run("SetTokenCookie", func(t *testing.T) {
		rr := httptest.NewRecorder()
		expiresAt := time.Now().Add(24 * time.Hour)

		h.setTokenCookie(rr, "test-token", expiresAt)

		cookies := rr.Result().Cookies()
		assert.Len(t, cookies, 1)

		cookie := cookies[0]
		assert.Equal(t, "token", cookie.Name)
		assert.Equal(t, "test-token", cookie.Value)
		assert.Equal(t, "/", cookie.Path)
		assert.True(t, cookie.HttpOnly)
	})

	t.Run("ClearTokenCookie", func(t *testing.T) {
		rr := httptest.NewRecorder()

		h.clearTokenCookie(rr)

		cookies := rr.Result().Cookies()
		assert.Len(t, cookies, 1)

		cookie := cookies[0]
		assert.Equal(t, "token", cookie.Name)
		assert.Equal(t, "", cookie.Value)
		assert.Equal(t, -1, cookie.MaxAge)
	})
}

func TestHTTPHelpers(t *testing.T) {
	h := &Handlers{}

	t.Run("RequirePOST_Valid", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/test", nil)
		rr := httptest.NewRecorder()

		result := h.requirePOST(rr, req)

		assert.True(t, result)
		assert.Equal(t, 200, rr.Code) // No error response
	})

	t.Run("RequirePOST_Invalid", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/test", nil)
		rr := httptest.NewRecorder()

		result := h.requirePOST(rr, req)

		assert.False(t, result)
		assert.Equal(t, 405, rr.Code) // Method not allowed
	})

	t.Run("IsAPIRequest", func(t *testing.T) {
		// API path
		req := httptest.NewRequest("GET", "/api/test", nil)
		assert.True(t, h.isAPIRequest(req))

		// JSON Accept header
		req = httptest.NewRequest("GET", "/test", nil)
		req.Header.Set("Accept", "application/json")
		assert.True(t, h.isAPIRequest(req))

		// Regular web request
		req = httptest.NewRequest("GET", "/dashboard", nil)
		assert.False(t, h.isAPIRequest(req))
	})
}

func TestPasswordValidation(t *testing.T) {
	h := &Handlers{}

	t.Run("ValidPassword", func(t *testing.T) {
		err := h.validatePasswordChange("password123", "password123")
		assert.NoError(t, err)
	})

	t.Run("PasswordMismatch", func(t *testing.T) {
		err := h.validatePasswordChange("password123", "different123")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "passwords do not match")
	})

	t.Run("PasswordTooShort", func(t *testing.T) {
		err := h.validatePasswordChange("short", "short")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "must be at least 8 characters")
	})
}

func TestJSONResponse(t *testing.T) {
	h := &Handlers{}

	t.Run("SendJSONResponse", func(t *testing.T) {
		rr := httptest.NewRecorder()
		data := map[string]string{"message": "success", "status": "ok"}

		h.sendJSONResponse(rr, data)

		assert.Equal(t, "application/json", rr.Header().Get("Content-Type"))
		body := rr.Body.String()
		assert.Contains(t, body, "success")
		assert.Contains(t, body, "message")
		assert.Contains(t, body, "status")
	})
}

// Test that our refactoring didn't break core patterns
func TestRefactoringIntegrity(t *testing.T) {
	h := &Handlers{}

	t.Run("TokenExtractionPriority", func(t *testing.T) {
		// Header should take priority over cookie
		req := httptest.NewRequest("GET", "/test", nil)
		req.Header.Set("Authorization", "Bearer header-token")
		req.AddCookie(&http.Cookie{Name: "token", Value: "cookie-token"})

		token, source := h.extractToken(req)
		assert.Equal(t, "header-token", token)
		assert.Equal(t, "header", source)
	})

	t.Run("CookieSecuritySettings", func(t *testing.T) {
		rr := httptest.NewRecorder()
		expiresAt := time.Now().Add(time.Hour)

		h.setTokenCookie(rr, "secure-token", expiresAt)

		cookie := rr.Result().Cookies()[0]
		assert.True(t, cookie.HttpOnly, "Cookie should be HttpOnly")
		assert.Equal(t, http.SameSiteStrictMode, cookie.SameSite, "Cookie should use SameSite strict")
		assert.Equal(t, "/", cookie.Path, "Cookie should have root path")
	})
}
