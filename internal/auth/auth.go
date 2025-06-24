package auth

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"webhook-router/internal/common/errors"
	"webhook-router/internal/config"
	"webhook-router/internal/storage"
)

// RedisClient interface for token blacklisting
type RedisClient interface {
	Set(ctx context.Context, key string, value interface{}, expiration time.Duration) error
	Get(ctx context.Context, key string) (string, error)
}

type Auth struct {
	storage     storage.Storage
	jwtSecret   []byte
	redisClient RedisClient
}

type Claims struct {
	UserID    string `json:"user_id"`
	Username  string `json:"username"`
	IsDefault bool   `json:"is_default"`
	jwt.RegisteredClaims
}

// Session struct kept for compatibility with existing handlers
type Session struct {
	UserID    string
	Username  string
	IsDefault bool
	ExpiresAt time.Time
}

func New(storage storage.Storage, cfg *config.Config, redisClient RedisClient) (*Auth, error) {
	if cfg.JWTSecret == "" {
		return nil, errors.ConfigError("JWT_SECRET environment variable is required")
	}
	jwtSecret := []byte(cfg.JWTSecret)

	return &Auth{
		storage:     storage,
		jwtSecret:   jwtSecret,
		redisClient: redisClient,
	}, nil
}

func (a *Auth) GenerateJWT(userID string, username string, isDefault bool) (string, error) {
	claims := &Claims{
		UserID:    userID,
		Username:  username,
		IsDefault: isDefault,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "webhook-router",
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(a.jwtSecret)
}

func (a *Auth) ValidateJWT(tokenString string) (*Claims, error) {
	// Check the blacklist first if Redis is available
	if a.redisClient != nil {
		ctx := context.Background()
		blacklistKey := fmt.Sprintf("jwt:blacklist:%s", tokenString)

		val, err := a.redisClient.Get(ctx, blacklistKey)
		if err == nil && val != "" {
			return nil, errors.AuthError("token has been revoked")
		}
		// Ignore Redis errors and continue with validation
	}

	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.AuthError(fmt.Sprintf("unexpected signing method: %v", token.Header["alg"]))
		}
		return a.jwtSecret, nil
	})

	if err != nil {
		return nil, err
	}

	if claims, ok := token.Claims.(*Claims); ok && token.Valid {
		return claims, nil
	}

	return nil, errors.AuthError("invalid token")
}

func (a *Auth) CreateAccount(username, password string) (string, *Session, error) {
	// Create the user account
	user, err := a.storage.CreateUser(username, password)
	if err != nil {
		return "", nil, err
	}

	// Generate JWT token
	tokenString, err := a.GenerateJWT(user.ID, user.Username, user.IsDefault)
	if err != nil {
		return "", nil, err
	}

	// Create session struct for compatibility
	session := &Session{
		UserID:    user.ID,
		Username:  user.Username,
		IsDefault: user.IsDefault,
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}

	return tokenString, session, nil
}

func (a *Auth) Login(username, password string) (string, *Session, error) {
	user, err := a.storage.ValidateUser(username, password)
	if err != nil {
		return "", nil, err
	}

	// Generate JWT token
	tokenString, err := a.GenerateJWT(user.ID, user.Username, user.IsDefault)
	if err != nil {
		return "", nil, err
	}

	// Create session struct for compatibility
	session := &Session{
		UserID:    user.ID,
		Username:  user.Username,
		IsDefault: user.IsDefault,
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}

	return tokenString, session, nil
}

func (a *Auth) Logout(token string) error {
	// Validate the token first
	claims, err := a.ValidateJWT(token)
	if err != nil {
		return errors.AuthError("invalid token")
	}

	// If no Redis client, just return (stateless logout)
	if a.redisClient == nil {
		return nil
	}

	// Calculate remaining time until token expiration
	remaining := time.Until(claims.ExpiresAt.Time)
	if remaining <= 0 {
		// Token already expired
		return nil
	}

	// Add token to blacklist with expiration
	ctx := context.Background()
	blacklistKey := fmt.Sprintf("jwt:blacklist:%s", token)

	err = a.redisClient.Set(ctx, blacklistKey, "1", remaining)
	if err != nil {
		return errors.InternalError("failed to blacklist token", err)
	}

	return nil
}

func (a *Auth) ValidateSession(token string) (*Session, bool) {
	claims, err := a.ValidateJWT(token)
	if err != nil {
		return nil, false
	}

	// Convert claims back to session for compatibility
	session := &Session{
		UserID:    claims.UserID,
		Username:  claims.Username,
		IsDefault: claims.IsDefault,
		ExpiresAt: claims.ExpiresAt.Time,
	}

	return session, true
}

// RequireAuth middleware supports both Bearer token and HTTP-only cookie authentication
func (a *Auth) RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var token string

		// First, try to get JWT from Authorization header
		authHeader := r.Header.Get("Authorization")
		if authHeader != "" && strings.HasPrefix(authHeader, "Bearer ") {
			token = strings.TrimPrefix(authHeader, "Bearer ")
		} else {
			// Fallback to HTTP-only cookie for web interface
			cookie, err := r.Cookie("token")
			if err != nil {
				a.handleUnauthorized(w, r)
				return
			}
			token = cookie.Value
		}

		session, valid := a.ValidateSession(token)
		if !valid {
			a.handleUnauthorized(w, r)
			return
		}

		// Add session info to request headers for handlers to use
		r.Header.Set("X-User-ID", session.UserID)
		r.Header.Set("X-Username", session.Username)
		if session.IsDefault {
			r.Header.Set("X-Is-Default", "true")
		}

		next.ServeHTTP(w, r)
	})
}

func (a *Auth) handleUnauthorized(w http.ResponseWriter, r *http.Request) {
	// Always return 401 JSON response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	w.Write([]byte(`{"error": "Authentication required"}`))
}

// CleanupExpiredSessions is no longer needed with stateless JWT
// Tokens expire automatically and don't need cleanup

func (a *Auth) ChangePassword(username, newPassword string) error {
	// Get user by username
	user, err := a.storage.GetUserByUsername(username)
	if err != nil {
		return errors.AuthError("user not found")
	}

	// Update password using the storage's UpdateUserCredentials method
	// The method will handle password hashing and setting is_default to false
	return a.storage.UpdateUserCredentials(user.ID, username, newPassword)
}

// GeneratePasswordResetToken generates a password reset token for the given email
func (a *Auth) GeneratePasswordResetToken(email string) (string, error) {
	// Find user by email (we'll need to update the schema to include email)
	// For now, we'll use username as email
	user, err := a.storage.GetUserByUsername(email)
	if err != nil {
		return "", errors.AuthError("user not found")
	}

	// Create reset token with 1-hour expiration
	claims := &Claims{
		UserID:   user.ID,
		Username: email,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Subject:   "password-reset",
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString(a.jwtSecret)
	if err != nil {
		return "", errors.InternalError("failed to generate reset token", err)
	}

	// Store the token in Redis with 1 hour expiration for validation
	ctx := context.Background()
	key := fmt.Sprintf("reset_token:%s", user.ID)
	if err := a.redisClient.Set(ctx, key, tokenString, time.Hour); err != nil {
		return "", errors.InternalError("failed to store reset token", err)
	}

	return tokenString, nil
}

// ResetPassword resets the user's password using a valid reset token
func (a *Auth) ResetPassword(tokenString, newPassword string) error {
	// Parse and validate the token
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return a.jwtSecret, nil
	})

	if err != nil || !token.Valid {
		return errors.AuthError("invalid or expired reset token")
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || claims.Subject != "password-reset" {
		return errors.AuthError("invalid reset token type")
	}

	// Verify the token hasn't been used
	ctx := context.Background()
	key := fmt.Sprintf("reset_token:%s", claims.UserID)
	storedToken, err := a.redisClient.Get(ctx, key)
	if err != nil || storedToken != tokenString {
		return errors.AuthError("reset token has already been used or is invalid")
	}

	// Update the password
	if err := a.storage.UpdateUserCredentials(claims.UserID, claims.Username, newPassword); err != nil {
		return errors.InternalError("failed to update password", err)
	}

	// Delete the reset token so it can't be reused
	a.redisClient.Set(ctx, key, "", 0) // Set with 0 expiration to delete

	return nil
}
