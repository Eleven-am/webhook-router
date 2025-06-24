package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"webhook-router/internal/common/logging"
	"webhook-router/internal/storage"
)

// Auth handlers

// HandleCreateAccount processes account creation requests
// @Summary Create new account
// @Description Creates a new user account with username and password
// @Tags auth
// @Accept json
// @Produce json
// @Param credentials body object{username=string,password=string,confirm_password=string} true "Account creation credentials"
// @Success 201 {object} object{token=string,user=object{id=string,username=string,is_default=boolean}} "Account created successfully"
// @Failure 400 {string} string "Invalid request or username already exists"
// @Failure 405 {string} string "Method not allowed"
// @Router /api/auth/create [post]
func (h *Handlers) HandleCreateAccount(w http.ResponseWriter, r *http.Request) {
	if !h.requirePOST(w, r) {
		return
	}

	// Parse JSON body
	var createReq struct {
		Username        string `json:"username"`
		Password        string `json:"password"`
		ConfirmPassword string `json:"confirm_password"`
	}

	if err := json.NewDecoder(r.Body).Decode(&createReq); err != nil {
		h.sendJSONError(w, err, "Invalid request", "Invalid JSON", http.StatusBadRequest)
		return
	}

	username := createReq.Username
	password := createReq.Password
	confirmPassword := createReq.ConfirmPassword

	// Validate input
	if username == "" || password == "" {
		h.sendJSONError(w, nil, "Missing credentials", "Username and password are required", http.StatusBadRequest)
		return
	}

	if password != confirmPassword {
		h.sendJSONError(w, nil, "Password mismatch", "Passwords do not match", http.StatusBadRequest)
		return
	}

	// Validate password requirements
	if err := h.validatePasswordRequirements(password); err != nil {
		h.sendJSONError(w, err, "Password validation failed", err.Error(), http.StatusBadRequest)
		return
	}

	// Create the account
	token, session, err := h.auth.CreateAccount(username, password)
	if err != nil {
		h.sendJSONError(w, err, "Account creation failed", "Username already exists or creation failed", http.StatusBadRequest)
		return
	}

	// Set JWT token cookie for web clients
	h.setTokenCookie(w, token, session.ExpiresAt)

	// Return JSON response with token and user info
	w.WriteHeader(http.StatusCreated)
	h.sendJSONResponse(w, map[string]interface{}{
		"token": token,
		"user": map[string]interface{}{
			"id":         session.UserID,
			"username":   session.Username,
			"is_default": session.IsDefault,
		},
	})
}

// HandleLogin processes login requests
// @Summary Process login
// @Description Authenticates user credentials and creates a session
// @Tags auth
// @Accept json
// @Produce json
// @Param credentials body object{username=string,password=string} true "Login credentials"
// @Success 200 {object} object{token=string,user=object{id=string,username=string,is_default=boolean}} "Login successful"
// @Failure 401 {string} string "Invalid credentials"
// @Failure 405 {string} string "Method not allowed"
// @Router /api/auth/login [post]
func (h *Handlers) HandleLogin(w http.ResponseWriter, r *http.Request) {
	if !h.requirePOST(w, r) {
		return
	}

	// Parse JSON body
	var loginReq struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}

	if err := json.NewDecoder(r.Body).Decode(&loginReq); err != nil {
		h.sendJSONError(w, err, "Invalid request", "Invalid JSON", http.StatusBadRequest)
		return
	}

	username := loginReq.Username
	password := loginReq.Password

	token, session, err := h.auth.Login(username, password)
	if err != nil {
		h.sendJSONError(w, err, "Login failed", "Invalid credentials", http.StatusUnauthorized)
		return
	}

	// Set JWT token cookie for web clients
	h.setTokenCookie(w, token, session.ExpiresAt)

	// Return JSON response with token and user info
	h.sendJSONResponse(w, map[string]interface{}{
		"token": token,
		"user": map[string]interface{}{
			"id":         session.UserID,
			"username":   session.Username,
			"is_default": session.IsDefault,
		},
	})
}

// HandleLogout processes logout requests
// @Summary Process logout
// @Description Invalidates the current session and redirects to login
// @Tags auth
// @Success 302 {string} string "Redirect to login page"
// @Router /api/auth/logout [post]
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

	// Always return JSON response
	h.sendJSONResponse(w, map[string]string{"message": "Logged out successfully"})
}

// HandleChangePassword processes password change requests
// @Summary Process password change
// @Description Updates user password and invalidates current session
// @Tags auth
// @Accept json
// @Produce json
// @Param credentials body object{password=string,confirm_password=string} true "Password change credentials"
// @Success 200 {object} object{message=string} "Password changed successfully"
// @Failure 400 {string} string "Invalid password or passwords do not match"
// @Failure 401 {string} string "Invalid session"
// @Failure 405 {string} string "Method not allowed"
// @Failure 500 {string} string "Failed to change password"
// @Router /api/auth/change-password [post]
func (h *Handlers) HandleChangePassword(w http.ResponseWriter, r *http.Request) {
	if !h.requirePOST(w, r) {
		return
	}

	// Get user info from request headers (set by auth middleware)
	userID := r.Header.Get("X-User-ID")
	username := r.Header.Get("X-Username")

	if userID == "" || username == "" {
		h.handleError(w, nil, "Missing user info", "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Parse JSON body
	var changePasswordReq struct {
		Password        string `json:"password"`
		ConfirmPassword string `json:"confirm_password"`
	}

	if err := json.NewDecoder(r.Body).Decode(&changePasswordReq); err != nil {
		h.handleError(w, err, "Invalid request", "Invalid JSON", http.StatusBadRequest)
		return
	}

	newPassword := changePasswordReq.Password
	confirmPassword := changePasswordReq.ConfirmPassword

	// Validate password change request
	if err := h.validatePasswordChange(newPassword, confirmPassword); err != nil {
		h.handleError(w, err, "Password validation failed", err.Error(), http.StatusBadRequest)
		return
	}

	// Update password
	if err := h.auth.ChangePassword(username, newPassword); err != nil {
		h.handleError(w, err, "Failed to change password", "Failed to change password", http.StatusInternalServerError)
		return
	}

	// Clear current token and cookie
	token, _ := h.extractToken(r)
	if token != "" {
		h.auth.Logout(token)
	}
	h.clearTokenCookie(w)

	// Return success response
	h.sendJSONResponse(w, map[string]string{"message": "Password changed successfully"})
}

// HandleGetCurrentUser returns the current authenticated user's detailed information
// @Summary Get current user
// @Description Returns the current authenticated user's profile information with statistics
// @Tags auth
// @Produce json
// @Security SessionAuth
// @Success 200 {object} object{user=object{id=string,username=string,is_default=boolean,created_at=string,updated_at=string},stats=object{routes_count=integer,recent_activity=string}} "Current user info with stats"
// @Failure 401 {string} string "Unauthorized"
// @Failure 500 {string} string "Failed to fetch user details"
// @Router /api/auth/me [get]
func (h *Handlers) HandleGetCurrentUser(w http.ResponseWriter, r *http.Request) {
	// Get user ID from request headers (set by auth middleware)
	userID := r.Header.Get("X-User-ID")

	if userID == "" {
		h.handleError(w, nil, "Missing user info", "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Fetch full user details from database
	user, err := h.storage.GetUser(userID)
	if err != nil {
		h.handleError(w, err, "Failed to fetch user details", "User not found", http.StatusNotFound)
		return
	}

	// Get user statistics (triggers count, recent activity)
	allTriggers, err := h.storage.GetTriggers(storage.TriggerFilters{})
	if err != nil {
		h.handleError(w, err, "Failed to fetch user statistics", "Internal server error", http.StatusInternalServerError)
		return
	}

	// Filter triggers by user ID
	var userTriggers []*storage.Trigger
	for _, trigger := range allTriggers {
		if trigger.UserID == user.ID {
			userTriggers = append(userTriggers, trigger)
		}
	}

	// Count user's triggers
	userTriggersCount := len(userTriggers)

	// Get recent activity (last trigger creation or update)
	var lastActivity string
	if len(userTriggers) > 0 {
		// Find most recent trigger update
		var mostRecent = userTriggers[0].UpdatedAt
		for _, trigger := range userTriggers {
			if trigger.UpdatedAt.After(mostRecent) {
				mostRecent = trigger.UpdatedAt
			}
		}
		lastActivity = mostRecent.Format("2006-01-02T15:04:05Z")
	} else {
		lastActivity = "No recent activity"
	}

	// Return comprehensive user information
	h.sendJSONResponse(w, map[string]interface{}{
		"user": map[string]interface{}{
			"id":         user.ID,
			"username":   user.Username,
			"is_default": user.IsDefault,
			"created_at": user.CreatedAt.Format("2006-01-02T15:04:05Z"),
			"updated_at": user.UpdatedAt.Format("2006-01-02T15:04:05Z"),
		},
		"stats": map[string]interface{}{
			"triggers_count":   userTriggersCount,
			"last_activity":    lastActivity,
			"account_age_days": int(time.Since(user.CreatedAt).Hours() / 24),
		},
	})
}

// HandleForgotPassword processes forgot password requests
// @Summary Request password reset
// @Description Sends a password reset email to the user
// @Tags auth
// @Accept json
// @Produce json
// @Param request body object{email=string} true "Email address"
// @Success 200 {object} object{message=string} "Reset email sent"
// @Failure 400 {string} string "Invalid request"
// @Failure 404 {string} string "User not found"
// @Failure 500 {string} string "Failed to send email"
// @Router /api/auth/forgot-password [post]
func (h *Handlers) HandleForgotPassword(w http.ResponseWriter, r *http.Request) {
	if !h.requirePOST(w, r) {
		return
	}

	// Parse JSON body
	var forgotReq struct {
		Email string `json:"email"`
	}

	if err := json.NewDecoder(r.Body).Decode(&forgotReq); err != nil {
		h.sendJSONError(w, err, "Invalid request", "Invalid JSON", http.StatusBadRequest)
		return
	}

	email := strings.TrimSpace(forgotReq.Email)
	if email == "" {
		h.sendJSONError(w, nil, "Missing email", "Email is required", http.StatusBadRequest)
		return
	}

	// Generate reset token
	token, err := h.auth.GeneratePasswordResetToken(email)
	if err != nil {
		// Don't reveal if user exists or not for security
		h.logger.Warn("Failed to generate reset token", logging.Field{"error", err.Error()})
		// Always return success to prevent email enumeration
		h.sendJSONResponse(w, map[string]string{"message": "If the email exists, a reset link has been sent"})
		return
	}

	// Send email with reset link
	if err := h.emailService.SendPasswordResetEmail(email, token); err != nil {
		h.logger.Error("Failed to send password reset email", err,
			logging.Field{"email", email})
		// Don't reveal the error to the user
		h.sendJSONResponse(w, map[string]string{"message": "If the email exists, a reset link has been sent"})
		return
	}

	h.logger.Info("Password reset email sent",
		logging.Field{"email", email})

	h.sendJSONResponse(w, map[string]string{"message": "If the email exists, a reset link has been sent"})
}

// HandleResetPassword processes password reset requests
// @Summary Reset password
// @Description Resets user password using a valid reset token
// @Tags auth
// @Accept json
// @Produce json
// @Param request body object{token=string,password=string} true "Reset token and new password"
// @Success 200 {object} object{message=string} "Password reset successful"
// @Failure 400 {string} string "Invalid token or password"
// @Failure 500 {string} string "Failed to reset password"
// @Router /api/auth/reset-password [post]
func (h *Handlers) HandleResetPassword(w http.ResponseWriter, r *http.Request) {
	if !h.requirePOST(w, r) {
		return
	}

	// Parse JSON body
	var resetReq struct {
		Token    string `json:"token"`
		Password string `json:"password"`
	}

	if err := json.NewDecoder(r.Body).Decode(&resetReq); err != nil {
		h.sendJSONError(w, err, "Invalid request", "Invalid JSON", http.StatusBadRequest)
		return
	}

	if resetReq.Token == "" || resetReq.Password == "" {
		h.sendJSONError(w, nil, "Missing fields", "Token and password are required", http.StatusBadRequest)
		return
	}

	// Validate password requirements
	if err := h.validatePasswordRequirements(resetReq.Password); err != nil {
		h.sendJSONError(w, err, "Password validation failed", err.Error(), http.StatusBadRequest)
		return
	}

	// Reset password using token
	if err := h.auth.ResetPassword(resetReq.Token, resetReq.Password); err != nil {
		h.sendJSONError(w, err, "Reset failed", "Invalid or expired token", http.StatusBadRequest)
		return
	}

	h.sendJSONResponse(w, map[string]string{"message": "Password reset successful"})
}

// validatePasswordRequirements validates password strength requirements
func (h *Handlers) validatePasswordRequirements(password string) error {
	if len(password) < 8 {
		return fmt.Errorf("password must be at least 8 characters long")
	}

	hasUpper := false
	hasLower := false
	hasDigit := false

	for _, char := range password {
		switch {
		case char >= 'A' && char <= 'Z':
			hasUpper = true
		case char >= 'a' && char <= 'z':
			hasLower = true
		case char >= '0' && char <= '9':
			hasDigit = true
		}
	}

	if !hasUpper {
		return fmt.Errorf("password must contain at least one uppercase letter")
	}
	if !hasLower {
		return fmt.Errorf("password must contain at least one lowercase letter")
	}
	if !hasDigit {
		return fmt.Errorf("password must contain at least one digit")
	}

	// Check for common weak passwords
	weakPasswords := []string{"password", "123456", "password123", "admin", "user"}
	lowerPassword := strings.ToLower(password)
	for _, weak := range weakPasswords {
		if lowerPassword == weak {
			return fmt.Errorf("password is too common, please choose a stronger password")
		}
	}

	return nil
}
