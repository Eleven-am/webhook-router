package handlers

import (
	"encoding/json"
	"net/http"
)

// Settings handlers

// GetSettings returns non-sensitive application settings
// @Summary Get application settings
// @Description Returns non-sensitive application configuration settings (sensitive fields are filtered)
// @Tags settings
// @Produce json
// @Security SessionAuth
// @Success 200 {object} map[string]string "Application settings (sensitive fields filtered)"
// @Failure 500 {string} string "Internal server error"
// @Router /api/settings [get]
func (h *Handlers) GetSettings(w http.ResponseWriter, r *http.Request) {
	allSettings, err := h.storage.GetAllSettings()
	if err != nil {
		http.Error(w, "Failed to get settings", http.StatusInternalServerError)
		return
	}

	// SECURITY FIX: Filter out sensitive settings before API response
	// This prevents exposure of OAuth2 tokens, passwords, API keys, etc.
	safeSettings := GetSafeSettings(allSettings)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(safeSettings)
}

// UpdateSettings updates application settings
// @Summary Update application settings
// @Description Updates multiple application configuration settings
// @Tags settings
// @Accept json
// @Produce json
// @Security SessionAuth
// @Param settings body map[string]string true "Settings to update"
// @Success 200 {object} map[string]string "Updated settings"
// @Failure 400 {string} string "Invalid JSON"
// @Failure 500 {string} string "Internal server error"
// @Router /api/settings [post]
func (h *Handlers) UpdateSettings(w http.ResponseWriter, r *http.Request) {
	var settings map[string]string
	if err := json.NewDecoder(r.Body).Decode(&settings); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	// Update each setting
	for key, value := range settings {
		if err := h.storage.SetSetting(key, value); err != nil {
			http.Error(w, "Failed to update settings", http.StatusInternalServerError)
			return
		}
	}

	// Return updated settings (filtered for security)
	allUpdatedSettings, err := h.storage.GetAllSettings()
	if err != nil {
		http.Error(w, "Failed to get updated settings", http.StatusInternalServerError)
		return
	}

	// SECURITY FIX: Filter sensitive settings from response
	safeUpdatedSettings := GetSafeSettings(allUpdatedSettings)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(safeUpdatedSettings)
}
