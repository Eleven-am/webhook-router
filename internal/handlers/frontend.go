package handlers

import (
	"io"
	"io/fs"
	"net/http"
	"path"
	"strings"

	"webhook-router/internal/common/errors"
)

// ServeSPA serves the React SPA with proper routing support
func (h *Handlers) ServeSPA() http.HandlerFunc {
	// Get the embedded filesystem
	distFS, err := fs.Sub(h.webFS, "frontend/dist")
	if err != nil {
		return func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "Frontend not available", http.StatusNotFound)
		}
	}

	fileServer := http.FileServer(http.FS(distFS))

	return func(w http.ResponseWriter, r *http.Request) {
		// Clean the path
		p := strings.TrimPrefix(path.Clean(r.URL.Path), "/")

		// Try to serve the file
		file, err := distFS.Open(p)
		if err == nil {
			defer file.Close()

			// Check if it's a file or directory
			stat, err := file.Stat()
			if err == nil && !stat.IsDir() {
				// Serve the file
				fileServer.ServeHTTP(w, r)
				return
			}
		}

		// For all other paths (including client-side routes), serve index.html
		indexFile, err := distFS.Open("index.html")
		if err != nil {
			http.Error(w, "index.html not found", http.StatusNotFound)
			return
		}
		defer indexFile.Close()

		// Read and serve index.html
		stat, _ := indexFile.Stat()
		http.ServeContent(w, r, "index.html", stat.ModTime(), indexFile.(io.ReadSeeker))
	}
}

// Route management has been removed - routing is now handled by triggers
// See triggers.go for trigger management functionality

// getUserIDFromRequest extracts the user ID from JWT token in the request
func (h *Handlers) getUserIDFromRequest(r *http.Request) (string, error) {
	// Get JWT token from cookie or Authorization header
	var tokenString string

	// Try cookie first
	if cookie, err := r.Cookie("token"); err == nil {
		tokenString = cookie.Value
	} else {
		// Try Authorization header
		authHeader := r.Header.Get("Authorization")
		if strings.HasPrefix(authHeader, "Bearer ") {
			tokenString = strings.TrimPrefix(authHeader, "Bearer ")
		} else {
			return "", errors.AuthError("no JWT token found")
		}
	}

	// Validate JWT and extract claims
	claims, err := h.auth.ValidateJWT(tokenString)
	if err != nil {
		return "", errors.AuthError("invalid JWT token")
	}

	return claims.UserID, nil
}
