package handlers

import (
	"net/http"
)

// Auth handlers

// ServeLogin serves the login page
// @Summary Serve login page
// @Description Serves the HTML login page
// @Tags auth
// @Produce html
// @Success 200 {string} string "Login page HTML"
// @Failure 404 {string} string "Login page not found"
// @Router /login [get]
func (h *Handlers) ServeLogin(w http.ResponseWriter, r *http.Request) {
	h.serveStaticFile(w, "web/login.html", "text/html", "Login page not found")
}

// HandleLogin processes login requests
// @Summary Process login
// @Description Authenticates user credentials and creates a session
// @Tags auth
// @Accept application/x-www-form-urlencoded
// @Produce json
// @Param username formData string true "Username"
// @Param password formData string true "Password"
// @Success 302 {string} string "Redirect to admin or change password page"
// @Failure 401 {string} string "Invalid credentials"
// @Failure 405 {string} string "Method not allowed"
// @Router /login [post]
func (h *Handlers) HandleLogin(w http.ResponseWriter, r *http.Request) {
	if !h.requirePOST(w, r) {
		return
	}

	username := r.FormValue("username")
	password := r.FormValue("password")

	token, session, err := h.auth.Login(username, password)
	if err != nil {
		h.handleError(w, err, "Login failed", "Invalid credentials", http.StatusUnauthorized)
		return
	}

	// Set JWT token cookie
	h.setTokenCookie(w, token, session.ExpiresAt)

	// If default user, redirect to change password page
	if session.IsDefault {
		http.Redirect(w, r, "/change-password", http.StatusFound)
		return
	}

	http.Redirect(w, r, "/admin", http.StatusFound)
}

// HandleLogout processes logout requests
// @Summary Process logout
// @Description Invalidates the current session and redirects to login
// @Tags auth
// @Success 302 {string} string "Redirect to login page"
// @Router /logout [post]
func (h *Handlers) HandleLogout(w http.ResponseWriter, r *http.Request) {
	// Get token from cookie or header
	token, _ := h.extractToken(r)
	
	// Blacklist the token if present
	if token != "" {
		if err := h.auth.Logout(token); err != nil {
			// Log error but continue with logout
			h.logger.Error("Failed to blacklist token", err)
		}
	}
	
	// Clear token cookie
	h.clearTokenCookie(w)

	// Return different responses based on request type
	if h.isAPIRequest(r) {
		h.sendJSONResponse(w, map[string]string{"message": "Logged out successfully"})
	} else {
		http.Redirect(w, r, "/login", http.StatusFound)
	}
}

// ServeChangePassword serves the change password page
// @Summary Serve change password page
// @Description Serves the HTML change password page for users with default credentials
// @Tags auth
// @Produce html
// @Success 200 {string} string "Change password page HTML"
// @Failure 302 {string} string "Redirect to login or admin page"
// @Failure 404 {string} string "Change password page not found"
// @Router /change-password [get]
func (h *Handlers) ServeChangePassword(w http.ResponseWriter, r *http.Request) {
	// Validate session
	session, valid := h.validateSession(w, r)
	if !valid {
		return // validateSession already handled the redirect
	}

	// Check if user is using default credentials
	if !session.IsDefault {
		// User already changed password, redirect to admin
		http.Redirect(w, r, "/admin", http.StatusFound)
		return
	}

	h.serveStaticFile(w, "web/change-password.html", "text/html", "Change password page not found")
}

// HandleChangePassword processes password change requests
// @Summary Process password change
// @Description Updates user password and invalidates current session
// @Tags auth
// @Accept application/x-www-form-urlencoded
// @Produce json
// @Param password formData string true "New password"
// @Param confirm_password formData string true "Password confirmation"
// @Success 302 {string} string "Redirect to login page with success message"
// @Failure 400 {string} string "Invalid password or passwords do not match"
// @Failure 401 {string} string "Invalid session"
// @Failure 405 {string} string "Method not allowed"
// @Failure 500 {string} string "Failed to change password"
// @Router /change-password [post]
func (h *Handlers) HandleChangePassword(w http.ResponseWriter, r *http.Request) {
	if !h.requirePOST(w, r) {
		return
	}

	// Validate session
	session, valid := h.validateSession(w, r)
	if !valid {
		return // validateSession already handled the redirect
	}

	newPassword := r.FormValue("password")
	confirmPassword := r.FormValue("confirm_password")

	// Validate password change request
	if err := h.validatePasswordChange(newPassword, confirmPassword); err != nil {
		h.handleError(w, err, "Password validation failed", err.Error(), http.StatusBadRequest)
		return
	}

	// Update password
	if err := h.auth.ChangePassword(session.Username, newPassword); err != nil {
		h.handleError(w, err, "Failed to change password", "Failed to change password", http.StatusInternalServerError)
		return
	}

	// Clear current token and cookie
	token, _ := h.extractToken(r)
	if token != "" {
		h.auth.Logout(token)
	}
	h.clearTokenCookie(w)

	// Redirect to login with success message
	http.Redirect(w, r, "/login?message=password_changed", http.StatusFound)
}
