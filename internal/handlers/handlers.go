package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"webhook-router/internal/config"
	"webhook-router/internal/database"
	"webhook-router/internal/rabbitmq"
)

type Handlers struct {
	db     *database.DB
	rmqPool *rabbitmq.ConnectionPool
	config *config.Config
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

func New(db *database.DB, rmqPool *rabbitmq.ConnectionPool, cfg *config.Config) *Handlers {
	return &Handlers{
		db:      db,
		rmqPool: rmqPool,
		config:  cfg,
	}
}

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
		// No matching routes, use default
		h.publishToDefault(r, body, endpoint)
		h.logWebhook(0, r, body, endpoint, 200, "")
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

		// Publish to RabbitMQ
		err = h.publishToQueue(route, payloadBytes)
		if err != nil {
			log.Printf("Error publishing to queue for route %s: %v", route.Name, err)
			lastError = err
			h.logWebhook(route.ID, r, body, endpoint, 500, err.Error())
			continue
		}

		successCount++
		h.logWebhook(route.ID, r, body, endpoint, 200, "")
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
	// Check database connection
	if err := h.db.Ping(); err != nil {
		http.Error(w, "Database unhealthy", http.StatusServiceUnavailable)
		return
	}

	// Check RabbitMQ connection
	client, err := h.rmqPool.NewClient()
	if err != nil {
		http.Error(w, "RabbitMQ unhealthy", http.StatusServiceUnavailable)
		return
	}
	client.Close()

	health := map[string]interface{}{
		"status":    "healthy",
		"timestamp": time.Now(),
		"version":   "1.0.0",
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(health)
}

func (h *Handlers) ServeFrontend(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "./web/index.html")
}