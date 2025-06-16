package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"webhook-router/internal/brokers"
	"webhook-router/internal/storage"
)

// Webhook handlers

// HandleWebhook handles incoming webhook requests
// @Summary Process incoming webhook
// @Description Processes incoming webhook requests and routes them to configured destinations
// @Tags webhooks
// @Accept json,xml,plain
// @Produce json
// @Param endpoint path string false "Webhook endpoint name"
// @Param payload body object true "Webhook payload"
// @Success 200 {string} string "OK"
// @Router /webhook/{endpoint} [post]
// @Router /webhook/{endpoint} [put]
// @Router /webhook/{endpoint} [patch]
// @Router /webhook/{endpoint} [delete]
// @Router /webhook/{endpoint} [get]
// @Router /webhook [post]
// @Router /webhook [put]
// @Router /webhook [patch]
// @Router /webhook [delete]
// @Router /webhook [get]
func (h *Handlers) HandleWebhook(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	endpoint := vars["endpoint"]

	// Log webhook details for debugging
	log.Printf("Received %s request to endpoint: %s", r.Method, endpoint)

	// Read the body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		log.Printf("Error reading request body: %v", err)
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}

	// Create webhook payload
	payload := WebhookPayload{
		Method:    r.Method,
		URL:       r.URL,
		Headers:   r.Header,
		Body:      string(body),
		Timestamp: time.Now(),
	}

	// If there's an endpoint specified, try to find a matching route
	var matchedRoutes []storage.Route
	if endpoint != "" {
		routes, err := h.storage.GetRoutes()
		if err != nil {
			log.Printf("Error getting routes: %v", err)
		} else {
			for _, route := range routes {
				if strings.EqualFold(route.Name, endpoint) || route.Endpoint == endpoint {
					matchedRoutes = append(matchedRoutes, *route)
				}
			}
		}
	}

	// If no specific routes match, get all routes and match by method/path
	if len(matchedRoutes) == 0 {
		routes, err := h.storage.GetRoutes()
		if err != nil {
			log.Printf("Error getting routes: %v", err)
		} else {
			for _, route := range routes {
				// Match by method and path patterns
				if route.Method == r.Method || route.Method == "*" {
					matchedRoutes = append(matchedRoutes, *route)
				}
			}
		}
	}

	// Process each matching route
	if len(matchedRoutes) > 0 {
		for _, route := range matchedRoutes {
			payload.RouteID = route.ID
			payload.RouteName = route.Name

			go h.processWebhookRoute(payload, route)
		}
	} else {
		// No matching routes, send to default queue if broker is available
		if h.broker != nil {
			go h.publishToDefaultQueue(payload)
		} else {
			log.Printf("No broker configured and no matching routes for endpoint: %s", endpoint)
		}
	}

	// Always return 200 OK for webhooks
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

func (h *Handlers) processWebhookRoute(payload WebhookPayload, route storage.Route) {
	// Apply filters if they exist
	if route.Filters != "" && route.Filters != "{}" {
		var filters map[string]interface{}
		if err := json.Unmarshal([]byte(route.Filters), &filters); err != nil {
			log.Printf("Error parsing filters for route %s: %v", route.Name, err)
			return
		}

		// Simple filter matching (can be expanded)
		if !h.matchesFilters(payload, filters) {
			log.Printf("Payload does not match filters for route: %s", route.Name)
			return
		}
	}

	// Apply headers if they exist
	var headers map[string]string
	if route.Headers != "" && route.Headers != "{}" {
		if err := json.Unmarshal([]byte(route.Headers), &headers); err != nil {
			log.Printf("Error parsing headers for route %s: %v", route.Name, err)
		}
	}

	// Convert payload to JSON
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		log.Printf("Error marshaling payload: %v", err)
		return
	}

	// Publish to broker if available
	if h.broker != nil {
		routingKey := route.RoutingKey
		if routingKey == "" {
			routingKey = route.Queue
		}

		message := &brokers.Message{
			MessageID:  fmt.Sprintf("webhook-%d", time.Now().UnixNano()),
			Queue:      route.Queue,
			RoutingKey: routingKey,
			Body:       payloadJSON,
			Headers:    headers,
			Timestamp:  time.Now(),
		}

		if err := h.broker.Publish(message); err != nil {
			log.Printf("Error publishing to broker for route %s: %v", route.Name, err)
			return
		}

		log.Printf("Successfully published webhook to queue %s for route %s", route.Queue, route.Name)
	} else {
		log.Printf("No broker configured, cannot publish for route: %s", route.Name)
	}
}

func (h *Handlers) publishToDefaultQueue(payload WebhookPayload) {
	// Convert payload to JSON
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		log.Printf("Error marshaling payload for default queue: %v", err)
		return
	}

	defaultQueue := "webhooks"
	if h.config != nil && h.config.DefaultQueue != "" {
		defaultQueue = h.config.DefaultQueue
	}

	message := &brokers.Message{
		MessageID:  fmt.Sprintf("webhook-default-%d", time.Now().UnixNano()),
		Queue:      defaultQueue,
		RoutingKey: defaultQueue,
		Body:       payloadJSON,
		Timestamp:  time.Now(),
	}

	if err := h.broker.Publish(message); err != nil {
		log.Printf("Error publishing to default queue: %v", err)
		return
	}

	log.Printf("Successfully published webhook to default queue: %s", defaultQueue)
}

func (h *Handlers) matchesFilters(payload WebhookPayload, filters map[string]interface{}) bool {
	// Simple filter implementation
	// In a full implementation, this would support complex filter logic

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

// HealthCheck returns the health status of the application
// @Summary Health check
// @Description Returns the health status of the application and its dependencies
// @Tags system
// @Produce json
// @Success 200 {object} map[string]interface{} "Health status"
// @Router /health [get]
func (h *Handlers) HealthCheck(w http.ResponseWriter, r *http.Request) {
	status := map[string]interface{}{
		"status":    "healthy",
		"timestamp": time.Now(),
		"version":   "1.0.0",
	}

	// Check broker health if available
	if h.broker != nil {
		if err := h.broker.Health(); err != nil {
			status["broker_status"] = "unhealthy"
			status["broker_error"] = err.Error()
		} else {
			status["broker_status"] = "healthy"
		}
	} else {
		status["broker_status"] = "not_configured"
	}

	// Check storage health
	if err := h.storage.Health(); err != nil {
		status["storage_status"] = "unhealthy"
		status["storage_error"] = err.Error()
		w.WriteHeader(http.StatusServiceUnavailable)
	} else {
		status["storage_status"] = "healthy"
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}
