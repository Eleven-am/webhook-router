package handlers

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"webhook-router/internal/common/errors"
	"webhook-router/internal/models"
)

// Pipeline management handlers

// Ensure models is used (for swagger documentation)
var _ models.PipelineConfig

// GetPipelines returns all configured pipelines
// @Summary Get all data pipelines
// @Description Returns a list of all configured data transformation pipelines
// @Tags pipelines
// @Produce json
// @Security SessionAuth
// @Success 200 {array} models.PipelineConfig "List of pipelines"
// @Failure 503 {string} string "Pipeline engine not initialized"
// @Router /api/pipelines [get]
func (h *Handlers) GetPipelines(w http.ResponseWriter, r *http.Request) {
	// Pipeline storage not yet implemented - return empty list for now
	h.sendJSONResponse(w, []interface{}{})
}

// GetPipeline returns a specific pipeline
// @Summary Get a specific pipeline
// @Description Returns the configuration of a specific pipeline
// @Tags pipelines
// @Produce json
// @Security SessionAuth
// @Param id path string true "Pipeline ID"
// @Success 200 {object} models.PipelineConfig "Pipeline configuration"
// @Failure 404 {string} string "Pipeline not found"
// @Failure 503 {string} string "Pipeline engine not initialized"
// @Router /api/pipelines/{id} [get]
func (h *Handlers) GetPipeline(w http.ResponseWriter, r *http.Request) {
	// Pipeline storage not yet implemented
	h.sendJSONError(w, errors.NotFoundError("pipeline"), "Pipeline storage not implemented", "Pipeline not found", http.StatusNotFound)
}

// CreatePipeline creates a new pipeline
// @Summary Create a new pipeline
// @Description Creates a new data transformation pipeline
// @Tags pipelines
// @Accept json
// @Produce json
// @Security SessionAuth
// @Param pipeline body models.PipelineConfig true "Pipeline configuration"
// @Success 201 {object} models.PipelineConfig "Created pipeline"
// @Failure 400 {string} string "Invalid pipeline configuration"
// @Failure 503 {string} string "Pipeline engine not initialized"
// @Router /api/pipelines [post]
func (h *Handlers) CreatePipeline(w http.ResponseWriter, r *http.Request) {
	// Pipeline storage not yet implemented
	h.sendJSONError(w, errors.InternalError("Pipeline storage not implemented", nil), "Pipeline storage not implemented", "Feature not available", http.StatusNotImplemented)
}

// UpdatePipeline updates an existing pipeline
// @Summary Update a pipeline
// @Description Updates an existing pipeline configuration
// @Tags pipelines
// @Accept json
// @Produce json
// @Security SessionAuth
// @Param id path string true "Pipeline ID"
// @Param pipeline body models.PipelineConfig true "Pipeline configuration"
// @Success 200 {object} models.PipelineConfig "Updated pipeline"
// @Failure 400 {string} string "Invalid pipeline configuration"
// @Failure 404 {string} string "Pipeline not found"
// @Failure 503 {string} string "Pipeline engine not initialized"
// @Router /api/pipelines/{id} [put]
func (h *Handlers) UpdatePipeline(w http.ResponseWriter, r *http.Request) {
	// Pipeline storage not yet implemented
	h.sendJSONError(w, errors.InternalError("Pipeline storage not implemented", nil), "Pipeline storage not implemented", "Feature not available", http.StatusNotImplemented)
}

// DeletePipeline deletes a pipeline
// @Summary Delete a pipeline
// @Description Deletes an existing pipeline
// @Tags pipelines
// @Security SessionAuth
// @Param id path string true "Pipeline ID"
// @Success 204 "Pipeline deleted"
// @Failure 404 {string} string "Pipeline not found"
// @Failure 503 {string} string "Pipeline engine not initialized"
// @Router /api/pipelines/{id} [delete]
func (h *Handlers) DeletePipeline(w http.ResponseWriter, r *http.Request) {
	// Pipeline storage not yet implemented
	h.sendJSONError(w, errors.InternalError("Pipeline storage not implemented", nil), "Pipeline storage not implemented", "Feature not available", http.StatusNotImplemented)
}

// TestPipeline tests a pipeline with sample data
// @Summary Test a pipeline
// @Description Tests a pipeline with sample data
// @Tags pipelines
// @Accept json
// @Produce json
// @Security SessionAuth
// @Param id path string true "Pipeline ID"
// @Param data body object true "Test data"
// @Success 200 {object} map[string]interface{} "Test result"
// @Failure 400 {string} string "Invalid test data"
// @Failure 404 {string} string "Pipeline not found"
// @Failure 503 {string} string "Pipeline engine not initialized"
// @Router /api/pipelines/{id}/test [post]
func (h *Handlers) TestPipeline(w http.ResponseWriter, r *http.Request) {
	// Pipeline storage not yet implemented
	h.sendJSONError(w, errors.InternalError("Pipeline storage not implemented", nil), "Pipeline storage not implemented", "Feature not available", http.StatusNotImplemented)
}

// Note: This is kept for backward compatibility
// New code should use models.PipelineExecutionRequest
// ExecutePipelineRequest represents a pipeline execution request
type ExecutePipelineRequest struct {
	PipelineConfig map[string]interface{} `json:"pipeline_config"`
	Data           map[string]interface{} `json:"data"`
}

const (
	maxRequestBodySize = 10 * 1024 * 1024 // 10MB
	maxPipelineStages  = 100              // Maximum stages per pipeline
	maxStageNameLength = 100              // Maximum stage name length
	pipelineTimeout    = 30 * time.Second // Pipeline execution timeout
)

// ExecutePipeline executes a pipeline with provided data
// @Summary Execute a pipeline
// @Description Executes a pipeline with the provided data
// @Tags pipelines
// @Accept json
// @Produce json
// @Security SessionAuth
// @Param request body models.PipelineExecutionRequest true "Execution request"
// @Success 200 {object} models.PipelineExecutionResponse "Execution result"
// @Failure 400 {string} string "Invalid request"
// @Failure 503 {string} string "Pipeline engine not initialized"
// @Router /api/pipelines/execute [post]
func (h *Handlers) ExecutePipeline(w http.ResponseWriter, r *http.Request) {
	if h.pipelineEngine == nil {
		h.sendJSONError(w, errors.InternalError("Pipeline engine not initialized", nil), "Pipeline engine not initialized", "Pipeline service unavailable", http.StatusServiceUnavailable)
		return
	}

	// Limit request body size to prevent memory exhaustion
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodySize)

	// Read request body with size validation
	body, err := io.ReadAll(r.Body)
	if err != nil {
		if strings.Contains(err.Error(), "request body too large") {
			h.sendJSONError(w, nil, "Request body too large", "Request size exceeds limit", http.StatusRequestEntityTooLarge)
		} else {
			h.sendJSONError(w, nil, "Failed to read request body", "Invalid request", http.StatusBadRequest)
		}
		return
	}

	// Parse execution request
	var req ExecutePipelineRequest
	if err := json.Unmarshal(body, &req); err != nil {
		h.sendJSONError(w, nil, "Failed to parse execution request", "Invalid JSON", http.StatusBadRequest)
		return
	}

	// Comprehensive validation
	if err := h.validatePipelineExecuteRequest(&req); err != nil {
		h.sendJSONError(w, nil, "Pipeline validation failed", err.Error(), http.StatusBadRequest)
		return
	}

	// Create timeout context for execution
	ctx, cancel := context.WithTimeout(r.Context(), pipelineTimeout)
	defer cancel()

	// Execute pipeline with timeout and validation
	result, err := h.pipelineEngine.Execute(ctx, req.PipelineConfig, req.Data)
	if err != nil {
		// Sanitize error message to prevent information disclosure
		sanitizedMsg := h.sanitizeErrorMessage(err.Error())
		h.sendJSONError(w, nil, "Pipeline execution failed", sanitizedMsg, http.StatusInternalServerError)
		return
	}

	// Return result
	w.WriteHeader(http.StatusOK)
	h.sendJSONResponse(w, result)
}

// validatePipelineExecuteRequest validates the pipeline execution request
func (h *Handlers) validatePipelineExecuteRequest(req *ExecutePipelineRequest) error {
	if req.PipelineConfig == nil {
		return errors.ValidationError("Pipeline configuration is required")
	}

	// Validate pipeline structure
	if err := h.validatePipelineConfig(req.PipelineConfig); err != nil {
		return err
	}

	// Initialize data if nil
	if req.Data == nil {
		req.Data = make(map[string]interface{})
	}

	return nil
}

// validatePipelineConfig validates the pipeline configuration structure
func (h *Handlers) validatePipelineConfig(config map[string]interface{}) error {
	// Check required fields
	id, hasID := config["id"].(string)
	if !hasID || id == "" {
		return errors.ValidationError("Pipeline ID is required")
	}

	// Validate ID format (alphanumeric + hyphens/underscores only)
	if !isValidPipelineID(id) {
		return errors.ValidationError("Pipeline ID contains invalid characters")
	}

	// Validate stages
	stagesData, hasStages := config["stages"]
	if !hasStages {
		return errors.ValidationError("Pipeline stages are required")
	}

	stagesList, ok := stagesData.([]interface{})
	if !ok {
		return errors.ValidationError("Pipeline stages must be an array")
	}

	if len(stagesList) == 0 {
		return errors.ValidationError("Pipeline must have at least one stage")
	}

	if len(stagesList) > maxPipelineStages {
		return errors.ValidationError("Pipeline exceeds maximum number of stages")
	}

	// Validate each stage
	for i, stageData := range stagesList {
		stageMap, ok := stageData.(map[string]interface{})
		if !ok {
			return errors.ValidationError("Each stage must be an object")
		}

		if err := h.validateStageConfig(stageMap, i); err != nil {
			return err
		}
	}

	return nil
}

// validateStageConfig validates a single stage configuration
func (h *Handlers) validateStageConfig(stage map[string]interface{}, index int) error {
	// Validate stage ID
	stageID, hasID := stage["id"].(string)
	if !hasID || stageID == "" {
		return errors.ValidationError("Stage ID is required")
	}

	if len(stageID) > maxStageNameLength {
		return errors.ValidationError("Stage ID is too long")
	}

	if !isValidStageID(stageID) {
		return errors.ValidationError("Stage ID contains invalid characters")
	}

	// Validate stage type
	stageType, hasType := stage["type"].(string)
	if !hasType || stageType == "" {
		return errors.ValidationError("Stage type is required")
	}

	// Check for known dangerous stage types
	if isDangerousStageType(stageType) {
		return errors.ValidationError("Stage type is not allowed")
	}

	return nil
}

// isValidPipelineID checks if pipeline ID is safe
func isValidPipelineID(id string) bool {
	if len(id) == 0 || len(id) > 100 {
		return false
	}
	for _, char := range id {
		if !((char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') || char == '-' || char == '_') {
			return false
		}
	}
	return true
}

// isValidStageID checks if stage ID is safe
func isValidStageID(id string) bool {
	return isValidPipelineID(id) // Same rules for now
}

// isDangerousStageType checks for potentially dangerous stage types
func isDangerousStageType(stageType string) bool {
	dangerousTypes := []string{
		"system", "exec", "shell", "cmd", "eval", "script",
		"file", "filesystem", "network", "socket", "process",
	}

	lowerType := strings.ToLower(stageType)
	for _, dangerous := range dangerousTypes {
		if strings.Contains(lowerType, dangerous) {
			return true
		}
	}
	return false
}

// sanitizeErrorMessage removes sensitive information from error messages
func (h *Handlers) sanitizeErrorMessage(errMsg string) string {
	// Remove file paths
	sanitized := strings.ReplaceAll(errMsg, "/Users/", "[path]/")
	sanitized = strings.ReplaceAll(sanitized, "/home/", "[path]/")
	sanitized = strings.ReplaceAll(sanitized, "\\Users\\", "[path]\\")

	// Remove sensitive keywords
	sensitivePatterns := []string{
		"panic", "runtime error", "segmentation fault",
		"permission denied", "access denied", "authentication failed",
		"database", "sql", "redis", "connection string",
	}

	for _, pattern := range sensitivePatterns {
		if strings.Contains(strings.ToLower(sanitized), pattern) {
			return "Internal processing error"
		}
	}

	// Truncate very long messages
	if len(sanitized) > 200 {
		return "Processing error occurred"
	}

	return sanitized
}

// Helper function to get pipeline ID from URL
func getPipelineID(r *http.Request) string {
	vars := mux.Vars(r)
	return vars["id"]
}
