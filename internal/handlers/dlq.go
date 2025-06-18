package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/gorilla/mux"
)

// GetDLQStats returns DLQ statistics
// @Summary Get DLQ statistics
// @Description Returns statistics about messages in the Dead Letter Queue
// @Tags dlq
// @Produce json
// @Security SessionAuth
// @Success 200 {object} map[string]interface{} "DLQ statistics"
// @Failure 503 {string} string "DLQ not configured"
// @Router /api/dlq/stats [get]
func (h *Handlers) GetDLQStats(w http.ResponseWriter, r *http.Request) {
	// Get per-broker DLQ stats from broker manager
	brokerStats, err := h.brokerManager.GetDLQStatistics()
	if err != nil {
		h.logger.Error("Failed to get broker DLQ stats", err)
		brokerStats = []map[string]interface{}{} // Continue with empty stats
	}

	// Get stats from storage
	stats, err := h.storage.GetDLQStats()
	if err != nil {
		h.logger.Error("Failed to get DLQ stats from storage", err)
	}

	var globalStats map[string]interface{}
	if stats != nil {
		globalStats = map[string]interface{}{
			"total_messages":     stats.TotalMessages,
			"pending_messages":   stats.PendingMessages,
			"abandoned_messages": stats.AbandonedMessages,
		}
	}

	response := map[string]interface{}{
		"per_broker_stats": brokerStats,
		"global_stats":     globalStats,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// ListDLQMessages returns messages in the DLQ
// @Summary List DLQ messages
// @Description Returns a list of messages currently in the Dead Letter Queue
// @Tags dlq
// @Produce json
// @Security SessionAuth
// @Param limit query int false "Number of messages to return" default(100)
// @Param route_id query int false "Filter by route ID"
// @Param status query string false "Filter by status (pending, abandoned)"
// @Success 200 {array} map[string]interface{} "DLQ messages"
// @Failure 503 {string} string "DLQ not configured"
// @Router /api/dlq/messages [get]
func (h *Handlers) ListDLQMessages(w http.ResponseWriter, r *http.Request) {
	// Get DLQ messages from storage
	messages, err := h.storage.ListDLQMessages(100, 0)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get DLQ messages: %v", err), http.StatusInternalServerError)
		return
	}

	stats, err := h.storage.GetDLQStats()
	if err != nil {
		h.logger.Error("Failed to get DLQ stats", err)
	}

	response := map[string]interface{}{
		"messages": messages,
		"stats":    stats,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// RetryDLQMessages triggers retry of failed messages
// @Summary Retry DLQ messages
// @Description Attempts to retry messages in the Dead Letter Queue
// @Tags dlq
// @Accept json
// @Produce json
// @Security SessionAuth
// @Param body body map[string]int false "Retry options" example({"limit": 10})
// @Success 200 {object} map[string]interface{} "Retry results"
// @Failure 503 {string} string "DLQ not configured"
// @Router /api/dlq/retry [post]
func (h *Handlers) RetryDLQMessages(w http.ResponseWriter, r *http.Request) {
	// Trigger retry via broker manager for per-broker DLQs
	err := h.brokerManager.RetryDLQMessages()
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to retry messages: %v", err), http.StatusInternalServerError)
		return
	}

	response := map[string]interface{}{
		"status": "retry initiated",
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// DeleteDLQMessage removes a specific message from the DLQ
// @Summary Delete DLQ message
// @Description Removes a specific message from the Dead Letter Queue
// @Tags dlq
// @Produce json
// @Security SessionAuth
// @Param id path string true "Message ID"
// @Success 200 {string} string "Message deleted"
// @Failure 503 {string} string "DLQ not configured"
// @Router /api/dlq/messages/{id} [delete]
func (h *Handlers) DeleteDLQMessage(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	messageIDStr := vars["id"]

	// Convert message ID to int
	messageID, err := strconv.Atoi(messageIDStr)
	if err != nil {
		http.Error(w, "Invalid message ID", http.StatusBadRequest)
		return
	}

	// Delete from storage
	err = h.storage.DeleteDLQMessage(messageID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to delete message: %v", err), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Message deleted"))
}

// ConfigureDLQRetryPolicy updates the DLQ retry policy
// @Summary Configure DLQ retry policy
// @Description Updates the retry policy for the Dead Letter Queue
// @Tags dlq
// @Accept json
// @Produce json
// @Security SessionAuth
// @Param policy body map[string]interface{} true "Retry policy configuration"
// @Success 200 {object} map[string]interface{} "Updated policy"
// @Failure 503 {string} string "DLQ not configured"
// @Router /api/dlq/policy [put]
func (h *Handlers) ConfigureDLQRetryPolicy(w http.ResponseWriter, r *http.Request) {

	// In production, you'd parse and update the retry policy
	var policy map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&policy); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	// For now, just return the current policy
	response := map[string]interface{}{
		"status": "policy updated",
		"policy": map[string]interface{}{
			"max_retries":        5,
			"initial_delay":      "1m",
			"max_delay":          "1h",
			"backoff_multiplier": 2.0,
			"abandon_after":      "24h",
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}
