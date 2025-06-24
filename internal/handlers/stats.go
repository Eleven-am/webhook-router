package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/gorilla/mux"
)

// Statistics handlers

// GetStats returns application statistics
// @Summary Get application statistics
// @Description Returns overall application statistics and metrics
// @Tags statistics
// @Produce json
// @Security SessionAuth
// @Success 200 {object} map[string]interface{} "Application statistics"
// @Failure 500 {string} string "Failed to get statistics"
// @Router /api/stats [get]
func (h *Handlers) GetStats(w http.ResponseWriter, r *http.Request) {
	stats, err := h.storage.GetStats()
	if err != nil {
		http.Error(w, "Failed to get statistics", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

// GetRouteStats returns statistics for a specific route
// @Summary Get route statistics
// @Description Returns statistics and metrics for a specific webhook route
// @Tags statistics
// @Produce json
// @Security SessionAuth
// @Param id path string true "Route ID"
// @Success 200 {object} map[string]interface{} "Route statistics"
// @Failure 400 {string} string "Invalid route ID"
// @Failure 404 {string} string "Route not found"
// @Router /api/stats/routes/{id} [get]
func (h *Handlers) GetTriggerStats(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	stats, err := h.storage.GetTriggerStats(id)
	if err != nil {
		http.Error(w, "Failed to get trigger statistics", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

// GetDashboardStats returns comprehensive dashboard statistics
// @Summary Get dashboard statistics
// @Description Returns comprehensive statistics for the dashboard including metrics with changes, time series data, and top routes
// @Tags statistics
// @Produce json
// @Security SessionAuth
// @Param period query string false "Time period: 1h, 24h, 7d, 30d (default: 24h)"
// @Param compare_previous query bool false "Include comparison with previous period (default: true)"
// @Param route_id query string false "Filter by specific route ID"
// @Success 200 {object} map[string]interface{} "Dashboard statistics"
// @Failure 500 {string} string "Failed to get dashboard statistics"
// @Router /api/stats/dashboard [get]
func (h *Handlers) GetDashboardStats(w http.ResponseWriter, r *http.Request) {
	// Parse query parameters
	period := r.URL.Query().Get("period")
	if period == "" {
		period = "24h"
	}

	routeID := r.URL.Query().Get("route_id")

	// Calculate time ranges based on period
	var currentStart, previousStart time.Time
	now := time.Now()

	switch period {
	case "1h":
		currentStart = now.Add(-1 * time.Hour)
		previousStart = now.Add(-2 * time.Hour)
	case "7d":
		currentStart = now.AddDate(0, 0, -7)
		previousStart = now.AddDate(0, 0, -14)
	case "30d":
		currentStart = now.AddDate(0, 0, -30)
		previousStart = now.AddDate(0, 0, -60)
	case "90d":
		currentStart = now.AddDate(0, 0, -90)
		previousStart = now.AddDate(0, 0, -180)
	default: // 24h
		currentStart = now.Add(-24 * time.Hour)
		previousStart = now.Add(-48 * time.Hour)
	}

	// Extract user ID from request
	userID, err := h.getUserIDFromRequest(r)
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Get comprehensive dashboard stats for the user with optional route filtering
	stats, err := h.storage.GetDashboardStats(userID, currentStart, previousStart, now, routeID)
	if err != nil {
		http.Error(w, "Failed to get dashboard statistics", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}
