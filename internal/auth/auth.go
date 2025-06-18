package auth

import (
	"context"
	"fmt"
	"log"
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
	UserID    int    `json:"user_id"`
	Username  string `json:"username"`
	IsDefault bool   `json:"is_default"`
	jwt.RegisteredClaims
}

// Session struct kept for compatibility with existing handlers
type Session struct {
	UserID    int
	Username  string
	IsDefault bool
	ExpiresAt time.Time
}

func New(storage storage.Storage, cfg *config.Config, redisClient RedisClient) *Auth {
	if cfg.JWTSecret == "" {
		log.Fatal("JWT_SECRET environment variable is required")
	}
	jwtSecret := []byte(cfg.JWTSecret)

	return &Auth{
		storage:     storage,
		jwtSecret:   jwtSecret,
		redisClient: redisClient,
	}
}

func (a *Auth) GenerateJWT(userID int, username string, isDefault bool) (string, error) {
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

func (a *Auth) RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var token string

		// Try to get JWT from Authorization header first
		authHeader := r.Header.Get("Authorization")
		if authHeader != "" && strings.HasPrefix(authHeader, "Bearer ") {
			token = strings.TrimPrefix(authHeader, "Bearer ")
		} else {
			// Fallback to cookie for web interface
			cookie, err := r.Cookie("token")
			if err != nil {
				a.redirectToLogin(w, r)
				return
			}
			token = cookie.Value
		}

		session, valid := a.ValidateSession(token)
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

// CleanupExpiredSessions is no longer needed with stateless JWT
// Tokens expire automatically and don't need cleanup

func (a *Auth) ChangePassword(username, newPassword string) error {
	// Get user to find their ID
	rows, err := a.storage.Query("SELECT id FROM users WHERE username = ?", username)
	if err != nil {
		return errors.InternalError("failed to query user", err)
	}
	if len(rows) == 0 {
		return errors.AuthError("user not found")
	}

	userID, ok := rows[0]["id"].(int64)
	if !ok {
		return errors.InternalError("invalid user ID type", nil)
	}

	// Update password using the storage's UpdateUserCredentials method
	// The method will handle password hashing and setting is_default to false
	return a.storage.UpdateUserCredentials(int(userID), username, newPassword)
}
