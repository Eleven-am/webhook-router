package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"

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
// @Router /stats [get]
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
// @Param id path int true "Route ID"
// @Success 200 {object} map[string]interface{} "Route statistics"
// @Failure 400 {string} string "Invalid route ID"
// @Failure 404 {string} string "Route not found"
// @Router /stats/routes/{id} [get]
func (h *Handlers) GetRouteStats(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, err := strconv.Atoi(vars["id"])
	if err != nil {
		http.Error(w, "Invalid route ID", http.StatusBadRequest)
		return
	}

	stats, err := h.storage.GetRouteStats(id)
	if err != nil {
		http.Error(w, "Failed to get route statistics", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}
