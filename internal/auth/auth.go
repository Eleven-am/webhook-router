package auth

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"
	"time"

	"webhook-router/internal/storage"
)

type Auth struct {
	storage  storage.Storage
	sessions map[string]*Session
}

type Session struct {
	UserID    int
	Username  string
	IsDefault bool
	ExpiresAt time.Time
}

func New(storage storage.Storage) *Auth {
	return &Auth{
		storage:  storage,
		sessions: make(map[string]*Session),
	}
}

func (a *Auth) generateSessionID() string {
	bytes := make([]byte, 32)
	rand.Read(bytes)
	return base64.URLEncoding.EncodeToString(bytes)
}

func (a *Auth) Login(username, password string) (string, *Session, error) {
	user, err := a.storage.ValidateUser(username, password)
	if err != nil {
		return "", nil, err
	}

	sessionID := a.generateSessionID()
	session := &Session{
		UserID:    user.ID,
		Username:  user.Username,
		IsDefault: user.IsDefault,
		ExpiresAt: time.Now().Add(24 * time.Hour), // 24 hour session
	}

	a.sessions[sessionID] = session
	return sessionID, session, nil
}

func (a *Auth) Logout(sessionID string) {
	delete(a.sessions, sessionID)
}

func (a *Auth) ValidateSession(sessionID string) (*Session, bool) {
	session, exists := a.sessions[sessionID]
	if !exists {
		return nil, false
	}

	if time.Now().After(session.ExpiresAt) {
		delete(a.sessions, sessionID)
		return nil, false
	}

	return session, true
}

func (a *Auth) RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check for session cookie
		cookie, err := r.Cookie("session")
		if err != nil {
			a.redirectToLogin(w, r)
			return
		}

		session, valid := a.ValidateSession(cookie.Value)
		if !valid {
			a.redirectToLogin(w, r)
			return
		}

		// Add session info to request headers for handlers to use
		r.Header.Set("X-User-ID", fmt.Sprintf("%d", session.UserID))
		r.Header.Set("X-Username", session.Username)
		if session.IsDefault {
			r.Header.Set("X-Is-Default", "true")
		}

		next.ServeHTTP(w, r)
	})
}

func (a *Auth) redirectToLogin(w http.ResponseWriter, r *http.Request) {
	// For API endpoints, return 401
	if strings.HasPrefix(r.URL.Path, "/api") {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error": "Authentication required"}`))
		return
	}

	// For web endpoints, redirect to login
	http.Redirect(w, r, "/login", http.StatusFound)
}

func (a *Auth) CleanupExpiredSessions() {
	for sessionID, session := range a.sessions {
		if time.Now().After(session.ExpiresAt) {
			delete(a.sessions, sessionID)
		}
	}
}

func (a *Auth) ChangePassword(username, newPassword string) error {
	// This is a placeholder implementation
	// In a real implementation, this would update the user's password in the storage
	// For now, just return nil to indicate success
	return nil
}
