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
	content, err := h.webFS.ReadFile("web/login.html")
	if err != nil {
		http.Error(w, "Login page not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "text/html")
	w.Write(content)
}

// HandleLogin processes login requests
// @Summary Process login
// @Description Authenticates user credentials and creates a session
// @Tags auth
// @Accept form
// @Produce json
// @Param username formData string true "Username"
// @Param password formData string true "Password"
// @Success 302 {string} string "Redirect to admin or change password page"
// @Failure 401 {string} string "Invalid credentials"
// @Failure 405 {string} string "Method not allowed"
// @Router /login [post]
func (h *Handlers) HandleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	username := r.FormValue("username")
	password := r.FormValue("password")

	sessionID, session, err := h.auth.Login(username, password)
	if err != nil {
		http.Error(w, "Invalid credentials", http.StatusUnauthorized)
		return
	}

	// Set session cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    sessionID,
		Path:     "/",
		HttpOnly: true,
		Secure:   false, // Set to true in production with HTTPS
		SameSite: http.SameSiteStrictMode,
		Expires:  session.ExpiresAt,
	})

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
	cookie, err := r.Cookie("session")
	if err == nil {
		h.auth.Logout(cookie.Value)
	}

	// Clear session cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		MaxAge:   -1,
	})

	http.Redirect(w, r, "/login", http.StatusFound)
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
	// Check if user has valid session
	cookie, err := r.Cookie("session")
	if err != nil {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}

	session, valid := h.auth.ValidateSession(cookie.Value)
	if !valid {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}

	// Check if user is using default credentials
	if !session.IsDefault {
		// User already changed password, redirect to admin
		http.Redirect(w, r, "/admin", http.StatusFound)
		return
	}

	content, err := h.webFS.ReadFile("web/change-password.html")
	if err != nil {
		http.Error(w, "Change password page not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "text/html")
	w.Write(content)
}

// HandleChangePassword processes password change requests
// @Summary Process password change
// @Description Updates user password and invalidates current session
// @Tags auth
// @Accept form
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
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Check session
	cookie, err := r.Cookie("session")
	if err != nil {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}

	session, valid := h.auth.ValidateSession(cookie.Value)
	if !valid {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}

	newPassword := r.FormValue("password")
	confirmPassword := r.FormValue("confirm_password")

	if newPassword != confirmPassword {
		http.Error(w, "Passwords do not match", http.StatusBadRequest)
		return
	}

	if len(newPassword) < 8 {
		http.Error(w, "Password must be at least 8 characters", http.StatusBadRequest)
		return
	}

	// Update password
	if err := h.auth.ChangePassword(session.Username, newPassword); err != nil {
		http.Error(w, "Failed to change password", http.StatusInternalServerError)
		return
	}

	// Clear current session
	h.auth.Logout(cookie.Value)

	// Clear session cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		MaxAge:   -1,
	})

	// Redirect to login with success message
	http.Redirect(w, r, "/login?message=password_changed", http.StatusFound)
}
