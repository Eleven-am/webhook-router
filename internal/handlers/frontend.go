package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"webhook-router/internal/storage"

	"github.com/gorilla/mux"
)

// Frontend handlers

// ServeFrontend serves the main frontend page
// @Summary Serve main frontend
// @Description Serves the main HTML frontend page
// @Tags frontend
// @Produce html
// @Success 200 {string} string "Frontend page HTML"
// @Failure 404 {string} string "Frontend not found"
// @Router / [get]
func (h *Handlers) ServeFrontend(w http.ResponseWriter, r *http.Request) {
	content, err := h.webFS.ReadFile("web/index.html")
	if err != nil {
		http.Error(w, "Frontend not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "text/html")
	w.Write(content)
}

// ServeSettings serves the settings page
// @Summary Serve settings page
// @Description Serves the HTML settings page
// @Tags frontend
// @Produce html
// @Success 200 {string} string "Settings page HTML"
// @Failure 404 {string} string "Settings page not found"
// @Router /settings [get]
func (h *Handlers) ServeSettings(w http.ResponseWriter, r *http.Request) {
	content, err := h.webFS.ReadFile("web/settings.html")
	if err != nil {
		http.Error(w, "Settings page not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "text/html")
	w.Write(content)
}

// API Handlers for Route Management

// GetRoutes returns all webhook routes
// @Summary Get all webhook routes
// @Description Returns a list of all configured webhook routes
// @Tags routes
// @Produce json
// @Security SessionAuth
// @Success 200 {array} storage.Route "List of routes"
// @Failure 500 {string} string "Internal server error"
// @Router /routes [get]
func (h *Handlers) GetRoutes(w http.ResponseWriter, r *http.Request) {
	routes, err := h.storage.GetRoutes()
	if err != nil {
		http.Error(w, "Failed to get routes", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(routes)
}

// CreateRoute creates a new webhook route
// @Summary Create webhook route
// @Description Creates a new webhook route configuration
// @Tags routes
// @Accept json
// @Produce json
// @Security SessionAuth
// @Param route body storage.Route true "Route configuration"
// @Success 201 {object} storage.Route "Created route"
// @Failure 400 {string} string "Invalid JSON"
// @Failure 409 {string} string "Route name already exists"
// @Failure 500 {string} string "Internal server error"
// @Router /routes [post]
func (h *Handlers) CreateRoute(w http.ResponseWriter, r *http.Request) {
	var route storage.Route
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

	// Process signature configuration if provided
	if route.SignatureConfig != "" && h.encryptor != nil {
		if err := PrepareSignatureConfig(&route, h.encryptor); err != nil {
			http.Error(w, fmt.Sprintf("Invalid signature configuration: %v", err), http.StatusBadRequest)
			return
		}
	}
	
	if err := h.storage.CreateRoute(&route); err != nil {
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

// GetRoute returns a specific webhook route
// @Summary Get webhook route
// @Description Returns a specific webhook route by ID
// @Tags routes
// @Produce json
// @Security SessionAuth
// @Param id path int true "Route ID"
// @Success 200 {object} storage.Route "Route details"
// @Failure 400 {string} string "Invalid route ID"
// @Failure 404 {string} string "Route not found"
// @Router /routes/{id} [get]
func (h *Handlers) GetRoute(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, err := strconv.Atoi(vars["id"])
	if err != nil {
		http.Error(w, "Invalid route ID", http.StatusBadRequest)
		return
	}

	route, err := h.storage.GetRoute(id)
	if err != nil {
		http.Error(w, "Route not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(route)
}

// UpdateRoute updates an existing webhook route
// @Summary Update webhook route
// @Description Updates an existing webhook route configuration
// @Tags routes
// @Accept json
// @Produce json
// @Security SessionAuth
// @Param id path int true "Route ID"
// @Param route body storage.Route true "Route configuration"
// @Success 200 {object} storage.Route "Updated route"
// @Failure 400 {string} string "Invalid JSON or route ID"
// @Failure 500 {string} string "Failed to update route"
// @Router /routes/{id} [put]
func (h *Handlers) UpdateRoute(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, err := strconv.Atoi(vars["id"])
	if err != nil {
		http.Error(w, "Invalid route ID", http.StatusBadRequest)
		return
	}

	var route storage.Route
	if err := json.NewDecoder(r.Body).Decode(&route); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	route.ID = id
	route.UpdatedAt = time.Now()

	// Process signature configuration if provided
	if route.SignatureConfig != "" && h.encryptor != nil {
		if err := PrepareSignatureConfig(&route, h.encryptor); err != nil {
			http.Error(w, fmt.Sprintf("Invalid signature configuration: %v", err), http.StatusBadRequest)
			return
		}
	}
	
	if err := h.storage.UpdateRoute(&route); err != nil {
		http.Error(w, "Failed to update route", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(route)
}

// DeleteRoute removes a webhook route
// @Summary Delete webhook route
// @Description Removes a webhook route configuration
// @Tags routes
// @Security SessionAuth
// @Param id path int true "Route ID"
// @Success 204 "No Content"
// @Failure 400 {string} string "Invalid route ID"
// @Failure 500 {string} string "Failed to delete route"
// @Router /routes/{id} [delete]
func (h *Handlers) DeleteRoute(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, err := strconv.Atoi(vars["id"])
	if err != nil {
		http.Error(w, "Invalid route ID", http.StatusBadRequest)
		return
	}

	if err := h.storage.DeleteRoute(id); err != nil {
		http.Error(w, "Failed to delete route", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// TestRoute tests a webhook route with sample data
// @Summary Test webhook route
// @Description Tests a webhook route by sending a sample payload
// @Tags routes
// @Security SessionAuth
// @Param id path int true "Route ID"
// @Success 200 {object} map[string]interface{} "Test result"
// @Failure 400 {string} string "Invalid route ID"
// @Failure 404 {string} string "Route not found"
// @Failure 503 {string} string "No broker configured"
// @Router /routes/{id}/test [post]
func (h *Handlers) TestRoute(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, err := strconv.Atoi(vars["id"])
	if err != nil {
		http.Error(w, "Invalid route ID", http.StatusBadRequest)
		return
	}

	route, err := h.storage.GetRoute(id)
	if err != nil {
		http.Error(w, "Route not found", http.StatusNotFound)
		return
	}

	// Create test payload
	testPayload := WebhookPayload{
		Method:    "POST",
		Body:      `{"test": true, "message": "This is a test payload"}`,
		Timestamp: time.Now(),
		RouteID:   route.ID,
		RouteName: route.Name,
	}

	// Test the route
	if h.broker != nil {
		go h.processWebhookRoute(testPayload, *route)
		result := map[string]interface{}{
			"status":  "success",
			"message": "Test payload sent to route",
			"route":   route.Name,
			"queue":   route.Queue,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(result)
	} else {
		result := map[string]interface{}{
			"status": "error",
			"error":  "No broker configured",
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(result)
	}
}
