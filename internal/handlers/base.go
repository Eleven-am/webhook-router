package handlers

import (
	"embed"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"runtime"
	"strings"
	"time"

	"webhook-router/internal/auth"
	"webhook-router/internal/brokers"
	"webhook-router/internal/brokers/manager"
	"webhook-router/internal/common/logging"
	"webhook-router/internal/config"
	"webhook-router/internal/crypto"
	"webhook-router/internal/pipeline"
	"webhook-router/internal/routing"
	"webhook-router/internal/storage"
	"webhook-router/internal/triggers"
)

type Handlers struct {
	storage        storage.Storage
	broker         brokers.Broker
	brokerManager  *manager.Manager
	config         *config.Config
	webFS          embed.FS
	auth           *auth.Auth
	router         routing.Router
	pipelineEngine pipeline.Engine
	triggerManager *triggers.Manager
	logger         logging.Logger
	encryptor      *crypto.ConfigEncryptor
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

func New(storage storage.Storage, broker brokers.Broker, cfg *config.Config, webFS embed.FS, authHandler *auth.Auth, router routing.Router, pipelineEngine pipeline.Engine, triggerManager *triggers.Manager, encryptor *crypto.ConfigEncryptor) *Handlers {
	logger := logging.GetGlobalLogger().WithFields(
		logging.Field{"component", "handlers"},
	)

	// Create broker manager
	brokerManager := manager.NewManager(storage)

	// Create DLQ if broker is available

	return &Handlers{
		storage:        storage,
		broker:         broker,
		brokerManager:  brokerManager,
		config:         cfg,
		webFS:          webFS,
		auth:           authHandler,
		router:         router,
		pipelineEngine: pipelineEngine,
		triggerManager: triggerManager,
		logger:         logger,
		encryptor:      encryptor,
	}
}

// GetBrokerManager returns the broker manager instance
// GetBrokerManager returns the broker manager instance
func (h *Handlers) GetBrokerManager() *manager.Manager {
	return h.brokerManager
}

// Helper functions for DRY code

// extractToken extracts JWT token from Authorization header or cookie
// Returns the token string and the source ("header", "cookie", or "")
func (h *Handlers) extractToken(r *http.Request) (string, string) {
	// Try Authorization header first
	authHeader := r.Header.Get("Authorization")
	if authHeader != "" && strings.HasPrefix(authHeader, "Bearer ") {
		return strings.TrimPrefix(authHeader, "Bearer "), "header"
	}

	// Try cookie
	cookie, err := r.Cookie("token")
	if err == nil {
		return cookie.Value, "cookie"
	}

	return "", ""
}

// validateSession validates a session from request and redirects to login if invalid
// Returns the session and true if valid, or writes redirect and returns nil, false
func (h *Handlers) validateSession(w http.ResponseWriter, r *http.Request) (*auth.Session, bool) {
	token, _ := h.extractToken(r)
	if token == "" {
		http.Redirect(w, r, "/login", http.StatusFound)
		return nil, false
	}

	session, valid := h.auth.ValidateSession(token)
	if !valid {
		http.Redirect(w, r, "/login", http.StatusFound)
		return nil, false
	}

	return session, true
}

// setTokenCookie sets a JWT token cookie with appropriate security settings
func (h *Handlers) setTokenCookie(w http.ResponseWriter, token string, expiresAt time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     "token",
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   false, // Set to true in production with HTTPS
		SameSite: http.SameSiteStrictMode,
		Expires:  expiresAt,
	})
}

// clearTokenCookie clears the JWT token cookie
func (h *Handlers) clearTokenCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     "token",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		MaxAge:   -1,
	})
}

// handleError logs an error and sends an HTTP error response
func (h *Handlers) handleError(w http.ResponseWriter, err error, logMessage, httpMessage string, statusCode int) {
	if err != nil {
		h.logger.Error(logMessage, err)
	}
	http.Error(w, httpMessage, statusCode)
}

// requirePOST ensures the request method is POST
func (h *Handlers) requirePOST(w http.ResponseWriter, r *http.Request) bool {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return false
	}
	return true
}

// isAPIRequest determines if this is an API request that should get JSON responses
func (h *Handlers) isAPIRequest(r *http.Request) bool {
	return strings.HasPrefix(r.URL.Path, "/api") || r.Header.Get("Accept") == "application/json"
}

// sendJSONResponse sends a JSON response
func (h *Handlers) sendJSONResponse(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

// serveStaticFile serves a static file from the embedded filesystem
func (h *Handlers) serveStaticFile(w http.ResponseWriter, filename, contentType, notFoundMessage string) {
	content, err := h.webFS.ReadFile(filename)
	if err != nil {
		http.Error(w, notFoundMessage, http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", contentType)
	w.Write(content)
}

// validatePasswordChange validates password change form data
func (h *Handlers) validatePasswordChange(newPassword, confirmPassword string) error {
	if newPassword != confirmPassword {
		return fmt.Errorf("passwords do not match")
	}

	if len(newPassword) < 8 {
		return fmt.Errorf("password must be at least 8 characters")
	}

	return nil
}

// Webhook-specific helper functions

// createWebhookPayload creates a WebhookPayload from an HTTP request
func (h *Handlers) createWebhookPayload(r *http.Request, body []byte) WebhookPayload {
	return WebhookPayload{
		Method:    r.Method,
		URL:       r.URL,
		Headers:   r.Header,
		Body:      string(body),
		Timestamp: time.Now(),
	}
}

// findMatchingRoutes finds routes that match the given endpoint and method
func (h *Handlers) findMatchingRoutes(endpoint, method string) ([]*storage.Route, error) {
	routes, err := h.storage.GetRoutes()
	if err != nil {
		return nil, fmt.Errorf("failed to get routes: %w", err)
	}

	var matchedRoutes []*storage.Route

	// First try to match by endpoint name or path
	if endpoint != "" {
		for _, route := range routes {
			if strings.EqualFold(route.Name, endpoint) || route.Endpoint == endpoint {
				matchedRoutes = append(matchedRoutes, route)
			}
		}
	}

	// If no specific routes match, match by method
	if len(matchedRoutes) == 0 {
		for _, route := range routes {
			if route.Method == method || route.Method == "*" {
				matchedRoutes = append(matchedRoutes, route)
			}
		}
	}

	return matchedRoutes, nil
}

// createBrokerMessage creates a standardized broker message
func (h *Handlers) createBrokerMessage(prefix, queue, routingKey string, body []byte, headers map[string]string) *brokers.Message {
	if routingKey == "" {
		routingKey = queue
	}

	return &brokers.Message{
		MessageID:  fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano()),
		Queue:      queue,
		RoutingKey: routingKey,
		Body:       body,
		Headers:    headers,
		Timestamp:  time.Now(),
	}
}

// publishMessageWithDLQ publishes a message with DLQ fallback support
func (h *Handlers) publishMessageWithDLQ(routeID int, message *brokers.Message, brokerID *int) error {
	if brokerID == nil {
		// Use default broker
		if h.broker == nil {
			return fmt.Errorf("no broker configured")
		}

		if err := h.broker.Publish(message); err != nil {
			// Note: DLQ is now handled per-broker via broker manager
			return err
		}
		return nil
	}

	// Use broker manager for specific broker when available
	if brokerID != nil {
		return h.brokerManager.PublishWithFallback(*brokerID, routeID, message)
	}
	// Fallback to default broker
	return h.broker.Publish(message)
}

// processFilters checks if a payload matches the given filters
func (h *Handlers) processFilters(payload WebhookPayload, filtersJSON string) (bool, error) {
	if filtersJSON == "" || filtersJSON == "{}" {
		return true, nil
	}

	var filters map[string]interface{}
	if err := json.Unmarshal([]byte(filtersJSON), &filters); err != nil {
		return false, fmt.Errorf("failed to parse filters: %w", err)
	}

	return h.matchesFilters(payload, filters), nil
}

// matchesFilters checks if payload matches the filter criteria
func (h *Handlers) matchesFilters(payload WebhookPayload, filters map[string]interface{}) bool {
	for key, expectedValue := range filters {
		switch key {
		case "method":
			if payload.Method != expectedValue {
				return false
			}
		case "content_type":
			contentType := ""
			if ct, exists := payload.Headers["Content-Type"]; exists && len(ct) > 0 {
				contentType = ct[0]
			}
			if !strings.Contains(contentType, fmt.Sprintf("%v", expectedValue)) {
				return false
			}
		case "user_agent":
			userAgent := ""
			if ua, exists := payload.Headers["User-Agent"]; exists && len(ua) > 0 {
				userAgent = ua[0]
			}
			if !strings.Contains(userAgent, fmt.Sprintf("%v", expectedValue)) {
				return false
			}
		case "body_contains":
			if !strings.Contains(payload.Body, fmt.Sprintf("%v", expectedValue)) {
				return false
			}
		}
	}
	return true
}

// parseHeaders parses a JSON string into a headers map
func (h *Handlers) parseHeaders(headersJSON string) (map[string]string, error) {
	if headersJSON == "" || headersJSON == "{}" {
		return make(map[string]string), nil
	}

	var headers map[string]string
	if err := json.Unmarshal([]byte(headersJSON), &headers); err != nil {
		return nil, fmt.Errorf("failed to parse headers: %w", err)
	}

	return headers, nil
}

// Health check helper functions

// checkComponentHealth checks the health of a component and updates status
func (h *Handlers) checkComponentHealth(status map[string]interface{}, componentName string, healthCheck func() error) bool {
	if err := healthCheck(); err != nil {
		status[componentName+"_status"] = "unhealthy"
		status[componentName+"_error"] = err.Error()
		return false
	}
	status[componentName+"_status"] = "healthy"
	return true
}

// addSystemMetrics adds system runtime metrics to the status
func (h *Handlers) addSystemMetrics(status map[string]interface{}) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	status["system_metrics"] = map[string]interface{}{
		"goroutines":   runtime.NumGoroutine(),
		"memory_alloc": m.Alloc / 1024 / 1024,      // MB
		"memory_total": m.TotalAlloc / 1024 / 1024, // MB
		"memory_sys":   m.Sys / 1024 / 1024,        // MB
		"gc_runs":      m.NumGC,
		"cpu_count":    runtime.NumCPU(),
	}
}

// addStorageMetrics adds storage-specific metrics to the status
func (h *Handlers) addStorageMetrics(status map[string]interface{}) {
	if routes, err := h.storage.GetRoutes(); err == nil {
		status["route_count"] = len(routes)
	}
	if triggers, err := h.storage.GetTriggers(storage.TriggerFilters{}); err == nil {
		status["trigger_count"] = len(triggers)
	}
	if pipelines, err := h.storage.GetPipelines(); err == nil {
		status["pipeline_count"] = len(pipelines)
	}
}

// addBrokerMetrics adds broker-specific metrics to the status
func (h *Handlers) addBrokerMetrics(status map[string]interface{}) {
	if h.brokerManager != nil {
		brokerStats, _ := h.brokerManager.GetDLQStatistics()
		status["dlq_statistics"] = brokerStats
		status["configured_brokers"] = len(brokerStats)
	}

	// Get DLQ stats from storage
	if dlqStats, err := h.storage.GetDLQStats(); err == nil {
		status["dlq_stats"] = dlqStats
	}
}
