package handlers

import (
	"net/http"
	"strings"

	"github.com/gorilla/mux"
	"webhook-router/internal/common/logging"
	httpTrigger "webhook-router/internal/triggers/http"
)

// HandleHTTPTrigger handles incoming requests for username-based HTTP triggers
// @Summary Process HTTP trigger request
// @Description Processes incoming requests to user-defined HTTP trigger endpoints
// @Tags triggers
// @Accept json,xml,plain
// @Produce json
// @Param username path string true "Username"
// @Param path path string true "Trigger path"
// @Success 200 {string} string "Success response based on trigger configuration"
// @Router /{username}/{path} [get]
// @Router /{username}/{path} [post]
// @Router /{username}/{path} [put]
// @Router /{username}/{path} [patch]
// @Router /{username}/{path} [delete]
func (h *Handlers) HandleHTTPTrigger(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	username := vars["username"]
	path := "/" + vars["path"] // Re-add the leading slash

	// Log request details
	h.logger.Debug("Received HTTP trigger request",
		logging.Field{"method", r.Method},
		logging.Field{"username", username},
		logging.Field{"path", path},
	)

	// Get user by username
	user, err := h.storage.GetUserByUsername(username)
	if err != nil || user == nil {
		h.logger.Debug("User not found",
			logging.Field{"username", username},
		)
		if err != nil {
			h.logger.Debug("Error retrieving user",
				logging.Field{"error", err.Error()},
			)
		}
		http.NotFound(w, r)
		return
	}

	// Find HTTP trigger for this user, path, and method
	trigger, err := h.storage.GetHTTPTriggerByUserPathMethod(user.ID, path, r.Method)
	if err != nil {
		h.logger.Debug("HTTP trigger not found",
			logging.Field{"user_id", user.ID},
			logging.Field{"path", path},
			logging.Field{"method", r.Method},
			logging.Field{"error", err.Error()},
		)
		http.NotFound(w, r)
		return
	}

	// Check if trigger is active
	if !trigger.Active {
		h.logger.Debug("HTTP trigger is not active",
			logging.Field{"trigger_id", trigger.ID},
		)
		http.Error(w, "Trigger not active", http.StatusServiceUnavailable)
		return
	}

	// Get the actual trigger instance from trigger manager
	if h.triggerManager == nil {
		h.logger.Error("Trigger manager not initialized", nil)
		http.Error(w, "Service unavailable", http.StatusServiceUnavailable)
		return
	}

	// Get the running trigger instance
	runningTrigger, err := h.triggerManager.GetTrigger(trigger.ID)
	if err != nil {
		h.logger.Debug("Failed to get trigger",
			logging.Field{"trigger_id", trigger.ID},
			logging.Field{"error", err.Error()},
		)
		http.Error(w, "Trigger not available", http.StatusServiceUnavailable)
		return
	}
	if runningTrigger == nil {
		h.logger.Debug("Trigger not running",
			logging.Field{"trigger_id", trigger.ID},
		)
		http.Error(w, "Trigger not running", http.StatusServiceUnavailable)
		return
	}

	// Type assert to HTTP trigger
	httpTrig, ok := runningTrigger.(*httpTrigger.Trigger)
	if !ok {
		h.logger.Error("Trigger is not an HTTP trigger", nil,
			logging.Field{"trigger_id", trigger.ID},
			logging.Field{"trigger_type", trigger.Type},
		)
		http.Error(w, "Invalid trigger type", http.StatusInternalServerError)
		return
	}

	// Delegate to the trigger's handler
	httpTrig.HandleHTTPRequest(w, r)
}

// NormalizePath ensures the path is properly formatted
func NormalizePath(path string) string {
	// Ensure path starts with /
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	// Replace multiple slashes with single slash
	for strings.Contains(path, "//") {
		path = strings.ReplaceAll(path, "//", "/")
	}

	return path
}
