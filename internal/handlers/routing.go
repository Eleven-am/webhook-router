package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"webhook-router/internal/models"
	"webhook-router/internal/routing"

	"github.com/gorilla/mux"
)

// Routing management handlers

// GetRoutingRules returns all routing rules
// @Summary Get all routing rules
// @Description Returns a list of all configured advanced routing rules
// @Tags routing
// @Produce json
// @Security SessionAuth
// @Success 200 {array} models.RouteRuleAPI "List of routing rules"
// @Failure 503 {string} string "Router not initialized"
// @Failure 500 {string} string "Internal server error"
// @Router /routing/rules [get]
func (h *Handlers) GetRoutingRules(w http.ResponseWriter, r *http.Request) {
	if h.router == nil {
		http.Error(w, "Router not initialized", http.StatusServiceUnavailable)
		return
	}

	rules, err := h.router.GetRules()
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get routes: %v", err), http.StatusInternalServerError)
		return
	}

	// Convert to API models
	apiRules := make([]*models.RouteRuleAPI, len(rules))
	for i, rule := range rules {
		apiRules[i] = models.ToRouteRuleAPI(rule)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(apiRules)
}

// GetRoutingRule returns a specific routing rule
// @Summary Get routing rule
// @Description Returns a specific routing rule by ID
// @Tags routing
// @Produce json
// @Security SessionAuth
// @Param id path string true "Rule ID"
// @Success 200 {object} models.RouteRuleAPI "Routing rule"
// @Failure 404 {string} string "Rule not found"
// @Failure 503 {string} string "Router not initialized"
// @Router /routing/rules/{id} [get]
func (h *Handlers) GetRoutingRule(w http.ResponseWriter, r *http.Request) {
	if h.router == nil {
		http.Error(w, "Router not initialized", http.StatusServiceUnavailable)
		return
	}

	vars := mux.Vars(r)
	ruleID := vars["id"]

	rule, err := h.router.GetRule(ruleID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get route: %v", err), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(rule)
}

// CreateRoutingRule creates a new routing rule
// @Summary Create routing rule
// @Description Creates a new advanced routing rule with conditions and destinations
// @Tags routing
// @Accept json
// @Produce json
// @Security SessionAuth
// @Param rule body routing.RouteRule true "Routing rule configuration"
// @Success 201 {object} routing.RouteRule "Created routing rule"
// @Failure 400 {string} string "Invalid JSON or rule validation failed"
// @Failure 503 {string} string "Router not initialized"
// @Router /routing/rules [post]
func (h *Handlers) CreateRoutingRule(w http.ResponseWriter, r *http.Request) {
	if h.router == nil {
		http.Error(w, "Router not initialized", http.StatusServiceUnavailable)
		return
	}

	var rule routing.RouteRule
	if err := json.NewDecoder(r.Body).Decode(&rule); err != nil {
		http.Error(w, fmt.Sprintf("Invalid JSON: %v", err), http.StatusBadRequest)
		return
	}

	// Set default values
	if rule.ID == "" {
		rule.ID = fmt.Sprintf("rule-%d", time.Now().Unix())
	}
	rule.CreatedAt = time.Now()
	rule.UpdatedAt = time.Now()

	if err := h.router.AddRule(&rule); err != nil {
		http.Error(w, fmt.Sprintf("Failed to create route: %v", err), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(rule)
}

// UpdateRoutingRule updates an existing routing rule
// @Summary Update routing rule
// @Description Updates an existing routing rule configuration
// @Tags routing
// @Accept json
// @Produce json
// @Security SessionAuth
// @Param id path string true "Rule ID"
// @Param rule body routing.RouteRule true "Routing rule configuration"
// @Success 200 {object} routing.RouteRule "Updated routing rule"
// @Failure 400 {string} string "Invalid JSON or rule validation failed"
// @Failure 503 {string} string "Router not initialized"
// @Router /routing/rules/{id} [put]
func (h *Handlers) UpdateRoutingRule(w http.ResponseWriter, r *http.Request) {
	if h.router == nil {
		http.Error(w, "Router not initialized", http.StatusServiceUnavailable)
		return
	}

	vars := mux.Vars(r)
	ruleID := vars["id"]

	var rule routing.RouteRule
	if err := json.NewDecoder(r.Body).Decode(&rule); err != nil {
		http.Error(w, fmt.Sprintf("Invalid JSON: %v", err), http.StatusBadRequest)
		return
	}

	rule.ID = ruleID
	rule.UpdatedAt = time.Now()

	if err := h.router.UpdateRule(&rule); err != nil {
		http.Error(w, fmt.Sprintf("Failed to update route: %v", err), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(rule)
}

// DeleteRoutingRule removes a routing rule
// @Summary Delete routing rule
// @Description Removes a routing rule configuration
// @Tags routing
// @Security SessionAuth
// @Param id path string true "Rule ID"
// @Success 204 "No Content"
// @Failure 404 {string} string "Rule not found"
// @Failure 503 {string} string "Router not initialized"
// @Router /routing/rules/{id} [delete]
func (h *Handlers) DeleteRoutingRule(w http.ResponseWriter, r *http.Request) {
	if h.router == nil {
		http.Error(w, "Router not initialized", http.StatusServiceUnavailable)
		return
	}

	vars := mux.Vars(r)
	ruleID := vars["id"]

	if err := h.router.RemoveRule(ruleID); err != nil {
		http.Error(w, fmt.Sprintf("Failed to delete route: %v", err), http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// TestRoutingRule tests a routing rule against a sample request
// @Summary Test routing rule
// @Description Tests a routing rule with sample request data
// @Tags routing
// @Accept json
// @Produce json
// @Security SessionAuth
// @Param id path string true "Rule ID"
// @Param request body object true "Test request data"
// @Success 200 {object} map[string]interface{} "Test result"
// @Failure 400 {string} string "Invalid JSON"
// @Failure 404 {string} string "Rule not found"
// @Failure 500 {string} string "Route test failed"
// @Failure 503 {string} string "Router not initialized"
// @Router /routing/rules/{id}/test [post]
func (h *Handlers) TestRoutingRule(w http.ResponseWriter, r *http.Request) {
	if h.router == nil {
		http.Error(w, "Router not initialized", http.StatusServiceUnavailable)
		return
	}

	var testRequest struct {
		RouteID string               `json:"route_id"`
		Request routing.RouteRequest `json:"request"`
	}

	if err := json.NewDecoder(r.Body).Decode(&testRequest); err != nil {
		http.Error(w, fmt.Sprintf("Invalid JSON: %v", err), http.StatusBadRequest)
		return
	}

	// Get the specific rule
	rule, err := h.router.GetRule(testRequest.RouteID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Route not found: %v", err), http.StatusNotFound)
		return
	}

	// Test the route
	result, err := h.router.Route(context.Background(), &testRequest.Request)
	if err != nil {
		http.Error(w, fmt.Sprintf("Route test failed: %v", err), http.StatusInternalServerError)
		return
	}

	response := map[string]interface{}{
		"rule_id":         rule.ID,
		"rule_name":       rule.Name,
		"matched":         len(result.MatchedRules) > 0,
		"destinations":    result.Destinations,
		"error":           result.Error,
		"processing_time": result.ProcessingTime.String(),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// GetRouterMetrics returns routing performance metrics
// @Summary Get router metrics
// @Description Returns performance metrics for the routing system
// @Tags routing
// @Produce json
// @Security SessionAuth
// @Success 200 {object} map[string]interface{} "Router metrics"
// @Failure 500 {string} string "Failed to get metrics"
// @Failure 503 {string} string "Router not initialized"
// @Router /routing/metrics [get]
func (h *Handlers) GetRouterMetrics(w http.ResponseWriter, r *http.Request) {
	if h.router == nil {
		http.Error(w, "Router not initialized", http.StatusServiceUnavailable)
		return
	}

	metrics, err := h.router.GetMetrics()
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get metrics: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(metrics)
}
