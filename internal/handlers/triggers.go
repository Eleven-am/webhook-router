package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"webhook-router/internal/storage"

	"github.com/gorilla/mux"
)

// Trigger management handlers

// GetTriggers returns all configured triggers
// @Summary Get all triggers
// @Description Returns a list of all configured triggers with optional filtering
// @Tags triggers
// @Produce json
// @Security SessionAuth
// @Param type query string false "Filter by trigger type"
// @Param status query string false "Filter by trigger status"
// @Param active query boolean false "Filter by active status"
// @Success 200 {array} storage.Trigger "List of triggers"
// @Failure 500 {string} string "Internal server error"
// @Router /triggers [get]
func (h *Handlers) GetTriggers(w http.ResponseWriter, r *http.Request) {
	// Parse query parameters for filtering
	triggerType := r.URL.Query().Get("type")
	status := r.URL.Query().Get("status")
	activeParam := r.URL.Query().Get("active")

	filters := storage.TriggerFilters{
		Type:   triggerType,
		Status: status,
	}

	if activeParam != "" {
		active := activeParam == "true"
		filters.Active = &active
	}

	triggers, err := h.storage.GetTriggers(filters)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get triggers: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(triggers)
}

// GetTrigger returns a specific trigger configuration
// @Summary Get trigger
// @Description Returns a specific trigger configuration by ID
// @Tags triggers
// @Produce json
// @Security SessionAuth
// @Param id path int true "Trigger ID"
// @Success 200 {object} storage.Trigger "Trigger configuration"
// @Failure 400 {string} string "Invalid trigger ID"
// @Failure 404 {string} string "Trigger not found"
// @Router /triggers/{id} [get]
func (h *Handlers) GetTrigger(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, err := strconv.Atoi(vars["id"])
	if err != nil {
		http.Error(w, "Invalid trigger ID", http.StatusBadRequest)
		return
	}

	trigger, err := h.storage.GetTrigger(id)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get trigger: %v", err), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(trigger)
}

// CreateTrigger creates a new trigger configuration
// @Summary Create trigger
// @Description Creates a new trigger configuration for automated webhook processing
// @Tags triggers
// @Accept json
// @Produce json
// @Security SessionAuth
// @Param trigger body storage.Trigger true "Trigger configuration"
// @Success 201 {object} storage.Trigger "Created trigger"
// @Failure 400 {string} string "Invalid JSON"
// @Failure 500 {string} string "Internal server error"
// @Router /triggers [post]
func (h *Handlers) CreateTrigger(w http.ResponseWriter, r *http.Request) {
	var trigger storage.Trigger
	if err := json.NewDecoder(r.Body).Decode(&trigger); err != nil {
		http.Error(w, fmt.Sprintf("Invalid JSON: %v", err), http.StatusBadRequest)
		return
	}

	// Set current timestamp
	trigger.CreatedAt = time.Now()
	trigger.UpdatedAt = time.Now()

	if err := h.storage.CreateTrigger(&trigger); err != nil {
		http.Error(w, fmt.Sprintf("Failed to create trigger: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(trigger)
}

// UpdateTrigger updates an existing trigger configuration
// @Summary Update trigger
// @Description Updates an existing trigger configuration
// @Tags triggers
// @Accept json
// @Produce json
// @Security SessionAuth
// @Param id path int true "Trigger ID"
// @Param trigger body storage.Trigger true "Trigger configuration"
// @Success 200 {object} storage.Trigger "Updated trigger"
// @Failure 400 {string} string "Invalid JSON or trigger ID"
// @Failure 500 {string} string "Failed to update trigger"
// @Router /triggers/{id} [put]
func (h *Handlers) UpdateTrigger(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, err := strconv.Atoi(vars["id"])
	if err != nil {
		http.Error(w, "Invalid trigger ID", http.StatusBadRequest)
		return
	}

	var trigger storage.Trigger
	if err := json.NewDecoder(r.Body).Decode(&trigger); err != nil {
		http.Error(w, fmt.Sprintf("Invalid JSON: %v", err), http.StatusBadRequest)
		return
	}

	trigger.ID = id
	trigger.UpdatedAt = time.Now()

	if err := h.storage.UpdateTrigger(&trigger); err != nil {
		http.Error(w, fmt.Sprintf("Failed to update trigger: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(trigger)
}

// DeleteTrigger removes a trigger configuration
// @Summary Delete trigger
// @Description Removes a trigger configuration
// @Tags triggers
// @Security SessionAuth
// @Param id path int true "Trigger ID"
// @Success 204 "No Content"
// @Failure 400 {string} string "Invalid trigger ID"
// @Failure 500 {string} string "Failed to delete trigger"
// @Router /triggers/{id} [delete]
func (h *Handlers) DeleteTrigger(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, err := strconv.Atoi(vars["id"])
	if err != nil {
		http.Error(w, "Invalid trigger ID", http.StatusBadRequest)
		return
	}

	if err := h.storage.DeleteTrigger(id); err != nil {
		http.Error(w, fmt.Sprintf("Failed to delete trigger: %v", err), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// StartTrigger starts a specific trigger
// @Summary Start trigger
// @Description Starts a specific trigger
// @Tags triggers
// @Security SessionAuth
// @Param id path int true "Trigger ID"
// @Success 200 {object} map[string]interface{} "Start result"
// @Failure 400 {string} string "Invalid trigger ID"
// @Failure 404 {string} string "Trigger not found"
// @Failure 500 {string} string "Failed to start trigger"
// @Router /triggers/{id}/start [post]
func (h *Handlers) StartTrigger(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, err := strconv.Atoi(vars["id"])
	if err != nil {
		http.Error(w, "Invalid trigger ID", http.StatusBadRequest)
		return
	}

	// For now, just update the status in database
	// In a full implementation, this would interact with the trigger manager
	trigger, err := h.storage.GetTrigger(id)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get trigger: %v", err), http.StatusNotFound)
		return
	}

	trigger.Status = "running"
	trigger.Active = true
	trigger.UpdatedAt = time.Now()

	if err := h.storage.UpdateTrigger(trigger); err != nil {
		http.Error(w, fmt.Sprintf("Failed to start trigger: %v", err), http.StatusInternalServerError)
		return
	}

	result := map[string]interface{}{
		"status":  "started",
		"message": fmt.Sprintf("Trigger '%s' started successfully", trigger.Name),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// StopTrigger stops a specific trigger
// @Summary Stop trigger
// @Description Stops a specific trigger
// @Tags triggers
// @Security SessionAuth
// @Param id path int true "Trigger ID"
// @Success 200 {object} map[string]interface{} "Stop result"
// @Failure 400 {string} string "Invalid trigger ID"
// @Failure 404 {string} string "Trigger not found"
// @Failure 500 {string} string "Failed to stop trigger"
// @Router /triggers/{id}/stop [post]
func (h *Handlers) StopTrigger(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, err := strconv.Atoi(vars["id"])
	if err != nil {
		http.Error(w, "Invalid trigger ID", http.StatusBadRequest)
		return
	}

	// For now, just update the status in database
	// In a full implementation, this would interact with the trigger manager
	trigger, err := h.storage.GetTrigger(id)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get trigger: %v", err), http.StatusNotFound)
		return
	}

	trigger.Status = "stopped"
	trigger.Active = false
	trigger.UpdatedAt = time.Now()

	if err := h.storage.UpdateTrigger(trigger); err != nil {
		http.Error(w, fmt.Sprintf("Failed to stop trigger: %v", err), http.StatusInternalServerError)
		return
	}

	result := map[string]interface{}{
		"status":  "stopped",
		"message": fmt.Sprintf("Trigger '%s' stopped successfully", trigger.Name),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// TestTrigger tests a trigger configuration
// @Summary Test trigger
// @Description Tests a trigger configuration
// @Tags triggers
// @Security SessionAuth
// @Param id path int true "Trigger ID"
// @Success 200 {object} map[string]interface{} "Test result"
// @Failure 400 {string} string "Invalid trigger ID or configuration"
// @Failure 404 {string} string "Trigger not found"
// @Router /triggers/{id}/test [post]
func (h *Handlers) TestTrigger(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, err := strconv.Atoi(vars["id"])
	if err != nil {
		http.Error(w, "Invalid trigger ID", http.StatusBadRequest)
		return
	}

	trigger, err := h.storage.GetTrigger(id)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get trigger: %v", err), http.StatusNotFound)
		return
	}

	// Basic validation test
	if trigger.Type == "" {
		result := map[string]interface{}{
			"status": "error",
			"error":  "Trigger type is not specified",
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(result)
		return
	}

	// Check if configuration is valid for the trigger type
	validTypes := []string{"http", "schedule", "polling", "broker"}
	isValidType := false
	for _, validType := range validTypes {
		if trigger.Type == validType {
			isValidType = true
			break
		}
	}

	if !isValidType {
		result := map[string]interface{}{
			"status": "error",
			"error":  fmt.Sprintf("Unsupported trigger type: %s", trigger.Type),
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(result)
		return
	}

	result := map[string]interface{}{
		"status":  "success",
		"message": fmt.Sprintf("Trigger '%s' configuration is valid", trigger.Name),
		"type":    trigger.Type,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// GetAvailableTriggerTypes returns the list of supported trigger types
// @Summary Get trigger types
// @Description Returns a list of supported trigger types
// @Tags triggers
// @Produce json
// @Security SessionAuth
// @Success 200 {object} map[string]interface{} "List of trigger types"
// @Router /triggers/types [get]
func (h *Handlers) GetAvailableTriggerTypes(w http.ResponseWriter, r *http.Request) {
	types := []string{"http", "schedule", "polling", "broker"}

	result := map[string]interface{}{
		"types": types,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}
