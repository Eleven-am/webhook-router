package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/gorilla/mux"
	"webhook-router/internal/storage"
)

// GetExecutionLogs returns paginated execution logs
// @Summary Get execution logs
// @Description Returns paginated execution logs with optional filtering
// @Tags logs
// @Produce json
// @Security SessionAuth
// @Param page query int false "Page number (default: 1)"
// @Param limit query int false "Items per page (default: 50, max: 100)"
// @Param trigger_type query string false "Filter by trigger type"
// @Param status query string false "Filter by status (success/error/processing)"
// @Param time_range query string false "Time range filter (1h/24h/7d/30d)"
// @Success 200 {object} map[string]interface{} "Paginated execution logs"
// @Failure 400 {string} string "Invalid parameters"
// @Failure 401 {string} string "Unauthorized"
// @Failure 500 {string} string "Internal server error"
// @Router /api/logs [get]
func (h *Handlers) GetExecutionLogs(w http.ResponseWriter, r *http.Request) {
	// Get current user from headers set by auth middleware
	userID := r.Header.Get("X-User-ID")
	if userID == "" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Parse query parameters
	page := 1
	if p := r.URL.Query().Get("page"); p != "" {
		if parsedPage, err := strconv.Atoi(p); err == nil && parsedPage > 0 {
			page = parsedPage
		}
	}

	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsedLimit, err := strconv.Atoi(l); err == nil && parsedLimit > 0 && parsedLimit <= 100 {
			limit = parsedLimit
		}
	}

	offset := (page - 1) * limit
	triggerType := r.URL.Query().Get("trigger_type")
	status := r.URL.Query().Get("status")
	timeRange := r.URL.Query().Get("time_range")

	// Get execution logs based on filters
	var logs []*storage.ExecutionLog
	var total int
	var err error

	if triggerType != "" {
		logs, err = h.storage.GetExecutionLogsByTriggerType(triggerType, userID, limit, offset)
		if err == nil {
			// Get total count - for simplicity, get all and count
			allLogs, countErr := h.storage.GetExecutionLogsByTriggerType(triggerType, userID, 1000, 0)
			if countErr == nil {
				total = len(allLogs)
			}
		}
	} else if status != "" {
		logs, err = h.storage.GetExecutionLogsByStatus(status, userID, limit, offset)
		if err == nil {
			// Get total count - for simplicity, get all and count
			allLogs, countErr := h.storage.GetExecutionLogsByStatus(status, userID, 1000, 0)
			if countErr == nil {
				total = len(allLogs)
			}
		}
	} else {
		// Get all execution logs for user
		logs, total, err = h.storage.ListExecutionLogsWithCount(userID, limit, offset)
	}

	if err != nil {
		h.logger.Error("Failed to get execution logs", err)
		http.Error(w, "Failed to get logs", http.StatusInternalServerError)
		return
	}

	// Filter by time range if requested
	if timeRange != "" {
		logs = h.filterExecutionLogsByTimeRange(logs, timeRange)
	}

	// Transform logs for response
	responseLogs := make([]map[string]interface{}, len(logs))
	for i, log := range logs {
		// Get trigger name if available
		triggerName := ""
		if log.TriggerID != nil && *log.TriggerID != "" {
			if trigger, err := h.storage.GetTrigger(*log.TriggerID); err == nil && trigger != nil {
				triggerName = trigger.Name
			}
		}

		// Format completed time
		completedAtStr := ""
		if log.CompletedAt != nil {
			completedAtStr = log.CompletedAt.Format(time.RFC3339)
		}

		responseLogs[i] = map[string]interface{}{
			"id":                     log.ID,
			"trigger_id":             log.TriggerID,
			"trigger_type":           log.TriggerType,
			"trigger_name":           triggerName,
			"input_method":           log.InputMethod,
			"input_endpoint":         log.InputEndpoint,
			"status":                 log.Status,
			"status_code":            log.StatusCode,
			"error_message":          log.ErrorMessage,
			"started_at":             log.StartedAt.Format(time.RFC3339),
			"completed_at":           completedAtStr,
			"total_latency_ms":       log.TotalLatencyMS,
			"transformation_time_ms": log.TransformationTimeMS,
			"broker_publish_time_ms": log.BrokerPublishTimeMS,
			"broker_type":            log.BrokerType,
			"broker_queue":           log.BrokerQueue,
			"input_headers":          log.InputHeaders,
			"input_body":             log.InputBody,
		}
	}

	// Prepare response
	response := map[string]interface{}{
		"logs":       responseLogs,
		"pagination": createPaginationResponse(page, limit, total),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// GetExecutionLog returns a single execution log by ID
// @Summary Get execution log by ID
// @Description Returns detailed information about a specific execution log
// @Tags logs
// @Produce json
// @Security SessionAuth
// @Param id path string true "Log ID"
// @Success 200 {object} map[string]interface{} "Execution log details"
// @Failure 401 {string} string "Unauthorized"
// @Failure 404 {string} string "Log not found"
// @Failure 500 {string} string "Internal server error"
// @Router /api/logs/{id} [get]
func (h *Handlers) GetExecutionLog(w http.ResponseWriter, r *http.Request) {
	// Get current user from headers set by auth middleware
	userID := r.Header.Get("X-User-ID")
	if userID == "" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Get log ID from URL
	logID := mux.Vars(r)["id"]
	if logID == "" {
		http.Error(w, "Log ID required", http.StatusBadRequest)
		return
	}

	// Get the execution log
	log, err := h.storage.GetExecutionLog(logID)
	if err != nil {
		h.logger.Error("Failed to get execution log", err)
		http.Error(w, "Log not found", http.StatusNotFound)
		return
	}

	// Verify user owns this log
	if log.UserID != userID {
		http.Error(w, "Log not found", http.StatusNotFound)
		return
	}

	// Get trigger name if available
	triggerName := ""
	if log.TriggerID != nil && *log.TriggerID != "" {
		if trigger, err := h.storage.GetTrigger(*log.TriggerID); err == nil && trigger != nil {
			triggerName = trigger.Name
		}
	}

	// Format completed time
	completedAtStr := ""
	if log.CompletedAt != nil {
		completedAtStr = log.CompletedAt.Format(time.RFC3339)
	}

	// Prepare response
	response := map[string]interface{}{
		"id":                     log.ID,
		"trigger_id":             log.TriggerID,
		"trigger_type":           log.TriggerType,
		"trigger_config":         log.TriggerConfig,
		"trigger_name":           triggerName,
		"input_method":           log.InputMethod,
		"input_endpoint":         log.InputEndpoint,
		"input_headers":          log.InputHeaders,
		"input_body":             log.InputBody,
		"pipeline_id":            log.PipelineID,
		"pipeline_stages":        log.PipelineStages,
		"transformation_data":    log.TransformationData,
		"transformation_time_ms": log.TransformationTimeMS,
		"broker_id":              log.BrokerID,
		"broker_type":            log.BrokerType,
		"broker_queue":           log.BrokerQueue,
		"broker_exchange":        log.BrokerExchange,
		"broker_routing_key":     log.BrokerRoutingKey,
		"broker_publish_time_ms": log.BrokerPublishTimeMS,
		"broker_response":        log.BrokerResponse,
		"status":                 log.Status,
		"status_code":            log.StatusCode,
		"error_message":          log.ErrorMessage,
		"output_data":            log.OutputData,
		"total_latency_ms":       log.TotalLatencyMS,
		"started_at":             log.StartedAt.Format(time.RFC3339),
		"completed_at":           completedAtStr,
		"user_id":                log.UserID,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// Helper functions

func (h *Handlers) filterExecutionLogsByTimeRange(logs []*storage.ExecutionLog, timeRange string) []*storage.ExecutionLog {
	var duration time.Duration
	switch timeRange {
	case "1h":
		duration = time.Hour
	case "24h":
		duration = 24 * time.Hour
	case "7d":
		duration = 7 * 24 * time.Hour
	case "30d":
		duration = 30 * 24 * time.Hour
	default:
		return logs
	}

	cutoff := time.Now().Add(-duration)
	var filtered []*storage.ExecutionLog
	for _, log := range logs {
		if log.StartedAt.After(cutoff) {
			filtered = append(filtered, log)
		}
	}
	return filtered
}

func createPaginationResponse(page, limit, total int) map[string]interface{} {
	totalPages := (total + limit - 1) / limit
	if totalPages == 0 {
		totalPages = 1
	}

	return map[string]interface{}{
		"page":        page,
		"limit":       limit,
		"total":       total,
		"total_pages": totalPages,
		"has_next":    page < totalPages,
		"has_prev":    page > 1,
	}
}
