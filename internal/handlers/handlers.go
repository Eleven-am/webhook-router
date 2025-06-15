package handlers

import (
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"webhook-router/internal/auth"
	"webhook-router/internal/config"
	"webhook-router/internal/database"
	"webhook-router/internal/rabbitmq"
)

type Handlers struct {
	db      *database.DB
	rmqPool *rabbitmq.ConnectionPool
	config  *config.Config
	webFS   embed.FS
	auth    *auth.Auth
}

type WebhookPayload struct {
	Method    string              `json:"method"`
	URL       *url.URL            `json:"url"`
	Headers   map[string][]string `json:"headers"`
	Body      string              `json:"body"`
	Timestamp time.Time           `json:"timestamp"`
	RouteID   int                 `json:"route_id,omitempty"`
	RouteName string              `json:"route_name,omitempty"`
}

func New(db *database.DB, rmqPool *rabbitmq.ConnectionPool, cfg *config.Config, webFS embed.FS, authHandler *auth.Auth) *Handlers {
	return &Handlers{
		db:      db,
		rmqPool: rmqPool,
		config:  cfg,
		webFS:   webFS,
		auth:    authHandler,
	}
}

// Auth handlers
func (h *Handlers) ServeLogin(w http.ResponseWriter, r *http.Request) {
	content, err := h.webFS.ReadFile("web/login.html")
	if err != nil {
		http.Error(w, "Login page not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "text/html")
	w.Write(content)
}

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

func (h *Handlers) HandleChangePassword(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get user ID from session cookie instead of header
	cookie, err := r.Cookie("session")
	if err != nil {
		http.Error(w, "Invalid session", http.StatusUnauthorized)
		return
	}

	session, valid := h.auth.ValidateSession(cookie.Value)
	if !valid {
		http.Error(w, "Invalid session", http.StatusUnauthorized)
		return
	}

	username := r.FormValue("username")
	password := r.FormValue("password")
	confirmPassword := r.FormValue("confirm_password")

	if password != confirmPassword {
		http.Error(w, "Passwords do not match", http.StatusBadRequest)
		return
	}

	if len(password) < 6 {
		http.Error(w, "Password must be at least 6 characters", http.StatusBadRequest)
		return
	}

	err = h.db.UpdateUserCredentials(session.UserID, username, password)
	if err != nil {
		http.Error(w, "Failed to update credentials", http.StatusInternalServerError)
		return
	}

	// Logout and redirect to login with new credentials
	h.auth.Logout(cookie.Value)

	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		MaxAge:   -1,
	})

	http.Redirect(w, r, "/login?message=credentials-updated", http.StatusFound)
}

// Settings handlers
func (h *Handlers) GetSettings(w http.ResponseWriter, r *http.Request) {
	settings, err := h.db.GetAllSettings()
	if err != nil {
		http.Error(w, "Failed to get settings", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(settings)
}

func (h *Handlers) UpdateSettings(w http.ResponseWriter, r *http.Request) {
	var settings map[string]string
	if err := json.NewDecoder(r.Body).Decode(&settings); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	// Check if RabbitMQ URL is being updated and if environment variable exists
	envRabbitMQ := os.Getenv("RABBITMQ_URL")
	if rmqURL, exists := settings["rabbitmq_url"]; exists {
		if envRabbitMQ != "" {
			// Environment variable takes precedence
			response := map[string]interface{}{
				"success": false,
				"error":   "RabbitMQ URL is set via environment variable and cannot be changed through the interface. Please update the RABBITMQ_URL environment variable instead.",
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(response)
			return
		}

		// Test RabbitMQ connection if URL is being updated
		if err := h.testRabbitMQConnection(rmqURL); err != nil {
			response := map[string]interface{}{
				"success": false,
				"error":   fmt.Sprintf("RabbitMQ connection test failed: %v", err),
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(response)
			return
		}
	}

	// Save settings to database
	for key, value := range settings {
		if key == "rabbitmq_source" {
			continue // Skip metadata field
		}
		if err := h.db.SetSetting(key, value); err != nil {
			http.Error(w, fmt.Sprintf("Failed to save setting %s", key), http.StatusInternalServerError)
			return
		}
	}

	// If RabbitMQ URL was updated and no env var exists, try to reinitialize the connection pool
	if rmqURL, exists := settings["rabbitmq_url"]; exists && envRabbitMQ == "" && rmqURL != "" {
		// Close existing pool if any
		if h.rmqPool != nil {
			h.rmqPool.Close()
		}

		// Try to create new pool
		newPool, err := rabbitmq.NewConnectionPool(rmqURL, 5)
		if err != nil {
			log.Printf("Failed to reinitialize RabbitMQ pool: %v", err)
			h.rmqPool = nil
		} else {
			h.rmqPool = newPool
			log.Println("RabbitMQ connection pool reinitialized")
		}
	}

	response := map[string]interface{}{
		"success": true,
		"message": "Settings updated successfully",
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (h *Handlers) testRabbitMQConnection(rmqURL string) error {
	// Create temporary connection pool to test
	testPool, err := rabbitmq.NewConnectionPool(rmqURL, 1)
	if err != nil {
		return err
	}
	defer testPool.Close()

	// Try to get a client
	client, err := testPool.NewClient()
	if err != nil {
		return err
	}
	defer client.Close()

	// Try to declare a test queue
	_, err = client.QueueDeclare("test-connection", false, true, false, false, nil)
	return err
}

func (h *Handlers) ServeSettings(w http.ResponseWriter, r *http.Request) {
	content, err := h.webFS.ReadFile("web/settings.html")
	if err != nil {
		http.Error(w, "Settings page not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "text/html")
	w.Write(content)
}

// Existing webhook handler (unchanged)
func (h *Handlers) HandleWebhook(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	endpoint := vars["endpoint"]
	if endpoint == "" {
		endpoint = "default"
	}

	// Read request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}

	// Find matching routes
	routes, err := h.db.FindMatchingRoutes(endpoint, r.Method)
	if err != nil {
		log.Printf("Error finding matching routes: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	if len(routes) == 0 {
		// No matching routes, use default if RabbitMQ is configured
		if h.rmqPool != nil {
			h.publishToDefault(r, body, endpoint)
			h.logWebhook(0, r, body, endpoint, 200, "")
		} else {
			h.logWebhook(0, r, body, endpoint, 500, "RabbitMQ not configured")
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
		return
	}

	// Process each matching route
	var lastError error
	successCount := 0

	for _, route := range routes {
		// Check filters if any
		if !h.matchesFilters(route, r, body) {
			continue
		}

		// Create webhook payload
		payload := WebhookPayload{
			Method:    r.Method,
			URL:       r.URL,
			Headers:   r.Header,
			Body:      string(body),
			Timestamp: time.Now(),
			RouteID:   route.ID,
			RouteName: route.Name,
		}

		payloadBytes, err := json.Marshal(payload)
		if err != nil {
			log.Printf("Error marshaling payload for route %s: %v", route.Name, err)
			lastError = err
			h.logWebhook(route.ID, r, body, endpoint, 500, err.Error())
			continue
		}

		// Publish to RabbitMQ if configured
		if h.rmqPool != nil {
			err = h.publishToQueue(route, payloadBytes)
			if err != nil {
				log.Printf("Error publishing to queue for route %s: %v", route.Name, err)
				lastError = err
				h.logWebhook(route.ID, r, body, endpoint, 500, err.Error())
				continue
			}
			successCount++
			h.logWebhook(route.ID, r, body, endpoint, 200, "")
		} else {
			h.logWebhook(route.ID, r, body, endpoint, 500, "RabbitMQ not configured")
		}
	}

	if successCount == 0 && lastError != nil {
		http.Error(w, "Failed to process webhook", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

func (h *Handlers) publishToDefault(r *http.Request, body []byte, endpoint string) {
	payload := WebhookPayload{
		Method:    r.Method,
		URL:       r.URL,
		Headers:   r.Header,
		Body:      string(body),
		Timestamp: time.Now(),
		RouteName: "default",
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		log.Printf("Error marshaling default payload: %v", err)
		return
	}

	client, err := h.rmqPool.NewClient()
	if err != nil {
		log.Printf("Error getting RabbitMQ client: %v", err)
		return
	}
	defer client.Close()

	err = client.PublishWebhook(h.config.DefaultQueue, "", h.config.DefaultQueue, payloadBytes)
	if err != nil {
		log.Printf("Error publishing to default queue: %v", err)
	}
}

func (h *Handlers) publishToQueue(route *database.Route, payload []byte) error {
	client, err := h.rmqPool.NewClient()
	if err != nil {
		return fmt.Errorf("failed to get RabbitMQ client: %w", err)
	}
	defer client.Close()

	return client.PublishWebhook(route.Queue, route.Exchange, route.RoutingKey, payload)
}

func (h *Handlers) matchesFilters(route *database.Route, r *http.Request, body []byte) bool {
	if route.Filters == "" || route.Filters == "{}" {
		return true
	}

	var filters map[string]interface{}
	if err := json.Unmarshal([]byte(route.Filters), &filters); err != nil {
		log.Printf("Error parsing filters for route %s: %v", route.Name, err)
		return true // If filters are invalid, allow the request
	}

	// Check header filters
	if headerFilters, ok := filters["headers"].(map[string]interface{}); ok {
		for key, expectedValue := range headerFilters {
			actualValue := r.Header.Get(key)
			if expectedStr, ok := expectedValue.(string); ok {
				if actualValue != expectedStr {
					return false
				}
			}
		}
	}

	// Check body content filters (simple string contains)
	if bodyFilters, ok := filters["body_contains"].([]interface{}); ok {
		bodyStr := string(body)
		for _, filter := range bodyFilters {
			if filterStr, ok := filter.(string); ok {
				if !strings.Contains(bodyStr, filterStr) {
					return false
				}
			}
		}
	}

	return true
}

func (h *Handlers) logWebhook(routeID int, r *http.Request, body []byte, endpoint string, statusCode int, errorMsg string) {
	headersJSON, _ := json.Marshal(r.Header)

	log := &database.WebhookLog{
		RouteID:    routeID,
		Method:     r.Method,
		Endpoint:   endpoint,
		Headers:    string(headersJSON),
		Body:       string(body),
		StatusCode: statusCode,
		Error:      errorMsg,
	}

	if err := h.db.LogWebhook(log); err != nil {
		log := fmt.Sprintf("Error logging webhook: %v", err)
		fmt.Println(log)
	}
}

// API Handlers for Route Management

func (h *Handlers) GetRoutes(w http.ResponseWriter, r *http.Request) {
	routes, err := h.db.GetRoutes()
	if err != nil {
		http.Error(w, "Failed to get routes", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(routes)
}

func (h *Handlers) CreateRoute(w http.ResponseWriter, r *http.Request) {
	var route database.Route
	if err := json.NewDecoder(r.Body).Decode(&route); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	// Set defaults
	if route.Method == "" {
		route.Method = "POST"
	}
	if route.Filters == "" {
		route.Filters = "{}"
	}
	if route.Headers == "" {
		route.Headers = "{}"
	}
	if route.RoutingKey == "" {
		route.RoutingKey = route.Queue
	}

	if err := h.db.CreateRoute(&route); err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			http.Error(w, "Route name already exists", http.StatusConflict)
			return
		}
		http.Error(w, "Failed to create route", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(route)
}

func (h *Handlers) GetRoute(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, err := strconv.Atoi(vars["id"])
	if err != nil {
		http.Error(w, "Invalid route ID", http.StatusBadRequest)
		return
	}

	route, err := h.db.GetRoute(id)
	if err != nil {
		if strings.Contains(err.Error(), "no rows") {
			http.Error(w, "Route not found", http.StatusNotFound)
			return
		}
		http.Error(w, "Failed to get route", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(route)
}

func (h *Handlers) UpdateRoute(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, err := strconv.Atoi(vars["id"])
	if err != nil {
		http.Error(w, "Invalid route ID", http.StatusBadRequest)
		return
	}

	var route database.Route
	if err := json.NewDecoder(r.Body).Decode(&route); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	route.ID = id
	if err := h.db.UpdateRoute(&route); err != nil {
		http.Error(w, "Failed to update route", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(route)
}

func (h *Handlers) DeleteRoute(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, err := strconv.Atoi(vars["id"])
	if err != nil {
		http.Error(w, "Invalid route ID", http.StatusBadRequest)
		return
	}

	if err := h.db.DeleteRoute(id); err != nil {
		http.Error(w, "Failed to delete route", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *Handlers) TestRoute(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, err := strconv.Atoi(vars["id"])
	if err != nil {
		http.Error(w, "Invalid route ID", http.StatusBadRequest)
		return
	}

	route, err := h.db.GetRoute(id)
	if err != nil {
		http.Error(w, "Route not found", http.StatusNotFound)
		return
	}

	// Create test payload
	testPayload := WebhookPayload{
		Method:    "POST",
		URL:       &url.URL{Path: "/test"},
		Headers:   map[string][]string{"Content-Type": {"application/json"}},
		Body:      `{"test": true}`,
		Timestamp: time.Now(),
		RouteID:   route.ID,
		RouteName: route.Name,
	}

	payloadBytes, err := json.Marshal(testPayload)
	if err != nil {
		http.Error(w, "Failed to create test payload", http.StatusInternalServerError)
		return
	}

	// Try to publish
	if h.rmqPool == nil {
		response := map[string]interface{}{
			"success": false,
			"error":   "RabbitMQ not configured",
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	err = h.publishToQueue(route, payloadBytes)
	if err != nil {
		response := map[string]interface{}{
			"success": false,
			"error":   err.Error(),
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	response := map[string]interface{}{
		"success": true,
		"message": "Test message sent successfully",
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (h *Handlers) GetStats(w http.ResponseWriter, r *http.Request) {
	stats, err := h.db.GetStats()
	if err != nil {
		http.Error(w, "Failed to get stats", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

func (h *Handlers) GetRouteStats(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, err := strconv.Atoi(vars["id"])
	if err != nil {
		http.Error(w, "Invalid route ID", http.StatusBadRequest)
		return
	}

	stats, err := h.db.GetRouteStats(id)
	if err != nil {
		http.Error(w, "Failed to get route stats", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

func (h *Handlers) HealthCheck(w http.ResponseWriter, r *http.Request) {
	health := map[string]interface{}{
		"status":    "healthy",
		"timestamp": time.Now(),
		"version":   "1.0.0",
	}

	// Check database connection
	if err := h.db.Ping(); err != nil {
		health["status"] = "unhealthy"
		health["database"] = "error: " + err.Error()
	} else {
		health["database"] = "connected"
	}

	// Check RabbitMQ connection if configured
	if h.rmqPool != nil {
		client, err := h.rmqPool.NewClient()
		if err != nil {
			health["rabbitmq"] = "error: " + err.Error()
			if health["status"] == "healthy" {
				health["status"] = "degraded"
			}
		} else {
			client.Close()
			health["rabbitmq"] = "connected"
		}
	} else {
		health["rabbitmq"] = "not configured"
		if health["status"] == "healthy" {
			health["status"] = "degraded"
		}
	}

	statusCode := http.StatusOK
	if health["status"] == "unhealthy" {
		statusCode = http.StatusServiceUnavailable
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(health)
}

func (h *Handlers) ServeFrontend(w http.ResponseWriter, r *http.Request) {
	// Read the index.html file from embedded filesystem
	content, err := h.webFS.ReadFile("web/index.html")
	if err != nil {
		http.Error(w, "Frontend not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "text/html")
	w.Write(content)
}
