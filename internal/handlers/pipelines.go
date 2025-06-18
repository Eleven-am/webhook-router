package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"webhook-router/internal/pipeline"

	"github.com/gorilla/mux"
)

// Pipeline management handlers

// GetPipelines returns all configured pipelines
// @Summary Get all data pipelines
// @Description Returns a list of all configured data transformation pipelines
// @Tags pipelines
// @Produce json
// @Security SessionAuth
// @Success 200 {object} map[string]interface{} "List of pipelines"
// @Failure 503 {string} string "Pipeline engine not initialized"
// @Router /pipelines [get]
func (h *Handlers) GetPipelines(w http.ResponseWriter, r *http.Request) {
	if h.pipelineEngine == nil {
		http.Error(w, "Pipeline engine not initialized", http.StatusServiceUnavailable)
		return
	}

	pipelines := h.pipelineEngine.GetAllPipelines()

	// Convert to response format
	pipelineData := make([]map[string]interface{}, len(pipelines))
	for i, p := range pipelines {
		stages := p.GetStages()
		stageData := make([]map[string]interface{}, len(stages))
		for j, stage := range stages {
			stageData[j] = map[string]interface{}{
				"name": stage.Name(),
				"type": stage.Type(),
			}
		}

		pipelineData[i] = map[string]interface{}{
			"id":          p.ID(),
			"name":        p.Name(),
			"stages":      stageData,
			"stage_count": len(stages),
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"pipelines": pipelineData,
		"count":     len(pipelines),
	})
}

// GetPipeline returns a specific pipeline
// @Summary Get data pipeline
// @Description Returns a specific data transformation pipeline by ID
// @Tags pipelines
// @Produce json
// @Security SessionAuth
// @Param id path string true "Pipeline ID"
// @Success 200 {object} map[string]interface{} "Pipeline details"
// @Failure 404 {string} string "Pipeline not found"
// @Failure 503 {string} string "Pipeline engine not initialized"
// @Router /pipelines/{id} [get]
func (h *Handlers) GetPipeline(w http.ResponseWriter, r *http.Request) {
	if h.pipelineEngine == nil {
		http.Error(w, "Pipeline engine not initialized", http.StatusServiceUnavailable)
		return
	}

	vars := mux.Vars(r)
	pipelineID := vars["id"]

	p, err := h.pipelineEngine.GetPipeline(pipelineID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get pipeline: %v", err), http.StatusNotFound)
		return
	}

	stages := p.GetStages()
	stageData := make([]map[string]interface{}, len(stages))
	for i, stage := range stages {
		stageData[i] = map[string]interface{}{
			"name": stage.Name(),
			"type": stage.Type(),
		}
	}

	pipelineData := map[string]interface{}{
		"id":          p.ID(),
		"name":        p.Name(),
		"stages":      stageData,
		"stage_count": len(stages),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(pipelineData)
}

// CreatePipeline creates a new pipeline
// @Summary Create data pipeline
// @Description Creates a new data transformation pipeline with stages
// @Tags pipelines
// @Accept json
// @Produce json
// @Security SessionAuth
// @Param pipeline body pipeline.PipelineConfig true "Pipeline configuration"
// @Success 201 {object} map[string]interface{} "Created pipeline"
// @Failure 400 {string} string "Invalid JSON or pipeline validation failed"
// @Failure 500 {string} string "Failed to register pipeline"
// @Failure 503 {string} string "Pipeline engine not initialized"
// @Router /pipelines [post]
func (h *Handlers) CreatePipeline(w http.ResponseWriter, r *http.Request) {
	if h.pipelineEngine == nil {
		http.Error(w, "Pipeline engine not initialized", http.StatusServiceUnavailable)
		return
	}

	var config pipeline.Config
	if err := json.NewDecoder(r.Body).Decode(&config); err != nil {
		http.Error(w, fmt.Sprintf("Invalid JSON: %v", err), http.StatusBadRequest)
		return
	}

	// Set default values
	if config.ID == "" {
		config.ID = fmt.Sprintf("pipeline-%d", time.Now().Unix())
	}
	config.CreatedAt = time.Now()
	config.UpdatedAt = time.Now()

	p, err := h.pipelineEngine.CreatePipelineFromConfig(&config)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to create pipeline: %v", err), http.StatusBadRequest)
		return
	}

	if err := h.pipelineEngine.RegisterPipeline(p); err != nil {
		http.Error(w, fmt.Sprintf("Failed to register pipeline: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"id":   p.ID(),
		"name": p.Name(),
	})
}

// DeletePipeline removes a pipeline
// @Summary Delete data pipeline
// @Description Removes a data transformation pipeline
// @Tags pipelines
// @Security SessionAuth
// @Param id path string true "Pipeline ID"
// @Success 204 "No Content"
// @Failure 404 {string} string "Pipeline not found"
// @Failure 503 {string} string "Pipeline engine not initialized"
// @Router /pipelines/{id} [delete]
func (h *Handlers) DeletePipeline(w http.ResponseWriter, r *http.Request) {
	if h.pipelineEngine == nil {
		http.Error(w, "Pipeline engine not initialized", http.StatusServiceUnavailable)
		return
	}

	vars := mux.Vars(r)
	pipelineID := vars["id"]

	if err := h.pipelineEngine.UnregisterPipeline(pipelineID); err != nil {
		http.Error(w, fmt.Sprintf("Failed to delete pipeline: %v", err), http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// ExecutePipeline executes a pipeline with test data
// @Summary Execute pipeline
// @Description Executes a data transformation pipeline with test data
// @Tags pipelines
// @Accept json
// @Produce json
// @Security SessionAuth
// @Param id path string true "Pipeline ID"
// @Param testData body object true "Test data for pipeline execution"
// @Success 200 {object} map[string]interface{} "Pipeline execution result"
// @Failure 400 {string} string "Invalid JSON"
// @Failure 404 {string} string "Pipeline not found"
// @Failure 500 {string} string "Pipeline execution failed"
// @Failure 503 {string} string "Pipeline engine not initialized"
// @Router /pipelines/{id}/execute [post]
func (h *Handlers) ExecutePipeline(w http.ResponseWriter, r *http.Request) {
	if h.pipelineEngine == nil {
		http.Error(w, "Pipeline engine not initialized", http.StatusServiceUnavailable)
		return
	}

	vars := mux.Vars(r)
	pipelineID := vars["id"]

	var testData struct {
		Headers map[string]string `json:"headers"`
		Body    string            `json:"body"`
	}

	if err := json.NewDecoder(r.Body).Decode(&testData); err != nil {
		http.Error(w, fmt.Sprintf("Invalid JSON: %v", err), http.StatusBadRequest)
		return
	}

	// Create pipeline data
	pipelineData := pipeline.NewPipelineData(
		fmt.Sprintf("test-%d", time.Now().Unix()),
		[]byte(testData.Body),
		testData.Headers,
	)

	// Execute pipeline
	result, err := h.pipelineEngine.ExecutePipeline(context.Background(), pipelineID, pipelineData)
	if err != nil {
		http.Error(w, fmt.Sprintf("Pipeline execution failed: %v", err), http.StatusInternalServerError)
		return
	}

	// Convert result to response format
	stageResults := make([]map[string]interface{}, len(result.StageResults))
	for i, stageResult := range result.StageResults {
		stageResults[i] = map[string]interface{}{
			"success":  stageResult.Success,
			"error":    stageResult.Error,
			"duration": stageResult.Duration.String(),
			"metadata": stageResult.Metadata,
		}
	}

	response := map[string]interface{}{
		"pipeline_id":    result.PipelineID,
		"success":        result.Success,
		"error":          result.Error,
		"total_duration": result.TotalDuration.String(),
		"stage_results":  stageResults,
		"output_data":    result.OutputData,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// GetPipelineMetrics returns pipeline execution metrics
// @Summary Get pipeline metrics
// @Description Returns performance metrics for pipeline executions
// @Tags pipelines
// @Produce json
// @Security SessionAuth
// @Success 200 {object} map[string]interface{} "Pipeline metrics"
// @Failure 500 {string} string "Failed to get metrics"
// @Failure 503 {string} string "Pipeline engine not initialized"
// @Router /pipelines/metrics [get]
func (h *Handlers) GetPipelineMetrics(w http.ResponseWriter, r *http.Request) {
	if h.pipelineEngine == nil {
		http.Error(w, "Pipeline engine not initialized", http.StatusServiceUnavailable)
		return
	}

	metrics, err := h.pipelineEngine.GetMetrics()
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get metrics: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(metrics)
}

// GetAvailableStageTypes returns the list of supported stage types
// @Summary Get stage types
// @Description Returns a list of supported pipeline stage types
// @Tags pipelines
// @Produce json
// @Security SessionAuth
// @Success 200 {object} map[string]interface{} "List of stage types"
// @Router /pipelines/stages/types [get]
func (h *Handlers) GetAvailableStageTypes(w http.ResponseWriter, r *http.Request) {
	types := []string{
		"json_transform", "xml_transform", "template",
		"validate", "filter", "enrich", "aggregate",
	}

	result := map[string]interface{}{
		"types": types,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}
