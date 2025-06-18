package stages

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"
	"webhook-router/internal/common/errors"
	"webhook-router/internal/enrichers"
	"webhook-router/internal/pipeline"
	"webhook-router/internal/pipeline/common"
	"webhook-router/internal/redis"
)

// EnrichmentStage enriches data with additional information
type EnrichmentStage struct {
	name        string
	config      EnrichmentConfig
	compiled    bool
	redisClient *redis.Client // Optional Redis client for distributed cache/rate limiting
}

type EnrichmentConfig struct {
	Enrichments []Enrichment `json:"enrichments"`
	Parallel    bool         `json:"parallel"` // whether to run enrichments in parallel
}

type Enrichment struct {
	Name         string                 `json:"name"`
	Type         string                 `json:"type"`         // static, lookup, function, http
	TargetField  string                 `json:"target_field"` // where to store the enriched data
	SourceField  string                 `json:"source_field"` // source field for enrichment
	StaticValue  interface{}            `json:"static_value"`
	LookupTable  map[string]interface{} `json:"lookup_table"`
	DefaultValue interface{}            `json:"default_value"`
	HttpConfig   HttpEnrichmentConfig   `json:"http_config"`
	Condition    string                 `json:"condition"` // condition for when to apply enrichment
}

type HttpEnrichmentConfig struct {
	URL            string                     `json:"url"`
	Method         string                     `json:"method"`
	Headers        map[string]string          `json:"headers"`
	Timeout        int                        `json:"timeout"` // timeout in seconds
	Auth           *enrichers.AuthConfig      `json:"auth"`
	Retry          *enrichers.RetryConfig     `json:"retry"`
	Cache          *enrichers.CacheConfig     `json:"cache"`
	RateLimit      *enrichers.RateLimitConfig `json:"rate_limit"`
	RequestBody    map[string]interface{}     `json:"request_body"`
	URLTemplate    string                     `json:"url_template"`
	BodyTemplate   string                     `json:"body_template"`
	HeaderTemplate map[string]string          `json:"header_template"`
	UseDistributed bool                       `json:"use_distributed"` // Whether to use distributed cache/rate limiting
}

func NewEnrichmentStage(name string) *EnrichmentStage {
	return &EnrichmentStage{
		name:     name,
		compiled: false,
	}
}

// SetRedisClient sets the Redis client for distributed operations
func (s *EnrichmentStage) SetRedisClient(client *redis.Client) {
	s.redisClient = client
}

func (s *EnrichmentStage) Name() string {
	return s.name
}

func (s *EnrichmentStage) Type() string {
	return "enrich"
}

func (s *EnrichmentStage) Configure(config map[string]interface{}) error {
	configData, err := json.Marshal(config)
	if err != nil {
		return errors.InternalError("failed to marshal config", err)
	}

	if err := json.Unmarshal(configData, &s.config); err != nil {
		return errors.InternalError("failed to unmarshal config", err)
	}

	s.compiled = true
	return nil
}

func (s *EnrichmentStage) Validate() error {
	if !s.compiled {
		return errors.ConfigError("stage not configured")
	}

	if len(s.config.Enrichments) == 0 {
		return errors.ConfigError("enrichment stage requires at least one enrichment")
	}

	validTypes := map[string]bool{
		"static": true, "lookup": true, "function": true, "http": true,
	}

	for i, enrichment := range s.config.Enrichments {
		if !validTypes[enrichment.Type] {
			return errors.ConfigError(fmt.Sprintf("enrichment %d has invalid type: %s", i, enrichment.Type))
		}

		if enrichment.TargetField == "" {
			return errors.ConfigError(fmt.Sprintf("enrichment %d requires target_field", i))
		}
	}

	return nil
}

func (s *EnrichmentStage) Process(ctx context.Context, data *pipeline.Data) (*pipeline.StageResult, error) {
	start := time.Now()

	result := &pipeline.StageResult{
		Success:  false,
		Metadata: make(map[string]interface{}),
	}

	// Parse input JSON
	var inputData map[string]interface{}
	if err := json.Unmarshal(data.Body, &inputData); err != nil {
		result.Error = fmt.Sprintf("failed to parse JSON: %v", err)
		result.Duration = time.Since(start)
		return result, nil
	}

	// Apply enrichments
	if s.config.Parallel {
		if err := s.applyEnrichmentsParallel(ctx, inputData); err != nil {
			result.Error = fmt.Sprintf("enrichment failed: %v", err)
			result.Duration = time.Since(start)
			return result, nil
		}
	} else {
		if err := s.applyEnrichmentsSequential(ctx, inputData); err != nil {
			result.Error = fmt.Sprintf("enrichment failed: %v", err)
			result.Duration = time.Since(start)
			return result, nil
		}
	}

	// Marshal output JSON
	outputBytes, err := json.Marshal(inputData)
	if err != nil {
		result.Error = fmt.Sprintf("failed to marshal output JSON: %v", err)
		result.Duration = time.Since(start)
		return result, nil
	}

	// Update pipeline data
	resultData := data.Clone()
	resultData.Body = outputBytes
	resultData.SetHeader("Content-Type", "application/json")

	result.Data = resultData
	result.Success = true
	result.Duration = time.Since(start)
	result.Metadata["enrichments_applied"] = len(s.config.Enrichments)

	return result, nil
}

func (s *EnrichmentStage) applyEnrichmentsSequential(ctx context.Context, data map[string]interface{}) error {
	for _, enrichment := range s.config.Enrichments {
		if err := s.applyEnrichment(ctx, enrichment, data); err != nil {
			return errors.InternalError(fmt.Sprintf("enrichment '%s' failed", enrichment.Name), err)
		}
	}
	return nil
}

func (s *EnrichmentStage) applyEnrichmentsParallel(ctx context.Context, data map[string]interface{}) error {
	var wg sync.WaitGroup
	errChan := make(chan error, len(s.config.Enrichments))

	for _, enrichment := range s.config.Enrichments {
		wg.Add(1)
		go func(e Enrichment) {
			defer wg.Done()
			if err := s.applyEnrichment(ctx, e, data); err != nil {
				errChan <- errors.InternalError(fmt.Sprintf("enrichment '%s' failed", e.Name), err)
			}
		}(enrichment)
	}

	wg.Wait()
	close(errChan)

	// Check for errors
	for err := range errChan {
		return err
	}

	return nil
}

func (s *EnrichmentStage) applyEnrichment(ctx context.Context, enrichment Enrichment, data map[string]interface{}) error {
	// Check condition if specified
	if enrichment.Condition != "" {
		// Simple condition checking (could be expanded)
		if !s.evaluateCondition(enrichment.Condition, data) {
			return nil // Skip this enrichment
		}
	}

	var enrichedValue interface{}
	var err error

	switch enrichment.Type {
	case "static":
		enrichedValue = enrichment.StaticValue

	case "lookup":
		sourceValue := s.getFieldValue(data, enrichment.SourceField)
		sourceStr := fmt.Sprintf("%v", sourceValue)

		if value, exists := enrichment.LookupTable[sourceStr]; exists {
			enrichedValue = value
		} else {
			enrichedValue = enrichment.DefaultValue
		}

	case "function":
		enrichedValue, err = s.applyFunction(enrichment, data)
		if err != nil {
			return err
		}

	case "http":
		enrichedValue, err = s.applyHttpEnrichment(ctx, enrichment, data)
		if err != nil {
			return err
		}

	default:
		return errors.ConfigError(fmt.Sprintf("unsupported enrichment type: %s", enrichment.Type))
	}

	// Set the enriched value
	s.setFieldValue(data, enrichment.TargetField, enrichedValue)

	return nil
}

func (s *EnrichmentStage) getFieldValue(data map[string]interface{}, field string) interface{} {
	if field == "" {
		return nil
	}

	// Simple field access (could be expanded to support JSONPath)
	return data[field]
}

func (s *EnrichmentStage) setFieldValue(data map[string]interface{}, field string, value interface{}) {
	if field == "" {
		return
	}

	// Simple field setting (could be expanded to support nested paths)
	data[field] = value
}

func (s *EnrichmentStage) evaluateCondition(condition string, data map[string]interface{}) bool {
	// Simple condition evaluation - could be expanded with a proper expression engine
	// For now, just check if a field exists
	if _, exists := data[condition]; exists {
		return true
	}
	return false
}

func (s *EnrichmentStage) applyFunction(enrichment Enrichment, data map[string]interface{}) (interface{}, error) {
	// Built-in functions for enrichment
	sourceValue := s.getFieldValue(data, enrichment.SourceField)

	switch enrichment.Name {
	case "timestamp":
		return time.Now().Unix(), nil

	case "uuid":
		// Simple UUID generation (in production, use proper UUID library)
		return fmt.Sprintf("uuid-%d", time.Now().UnixNano()), nil

	case "uppercase":
		return fmt.Sprintf("%v", sourceValue), nil

	case "lowercase":
		return fmt.Sprintf("%v", sourceValue), nil

	default:
		return enrichment.DefaultValue, nil
	}
}

func (s *EnrichmentStage) applyHttpEnrichment(ctx context.Context, enrichment Enrichment, data map[string]interface{}) (interface{}, error) {
	// Convert HttpEnrichmentConfig to enrichers.HTTPConfig
	httpConfig := &enrichers.HTTPConfig{
		URL:            enrichment.HttpConfig.URL,
		Method:         enrichment.HttpConfig.Method,
		Headers:        enrichment.HttpConfig.Headers,
		Auth:           enrichment.HttpConfig.Auth,
		Retry:          enrichment.HttpConfig.Retry,
		Cache:          enrichment.HttpConfig.Cache,
		RateLimit:      enrichment.HttpConfig.RateLimit,
		RequestBody:    enrichment.HttpConfig.RequestBody,
		URLTemplate:    enrichment.HttpConfig.URLTemplate,
		BodyTemplate:   enrichment.HttpConfig.BodyTemplate,
		HeaderTemplate: enrichment.HttpConfig.HeaderTemplate,
		UseDistributed: enrichment.HttpConfig.UseDistributed,
		RedisClient:    s.redisClient,
	}

	// Set timeout from config (convert seconds to duration)
	if enrichment.HttpConfig.Timeout > 0 {
		httpConfig.Timeout = time.Duration(enrichment.HttpConfig.Timeout) * time.Second
	}

	// Create HTTP enricher
	httpEnricher, err := enrichers.NewHTTPEnricher(httpConfig)
	if err != nil {
		return enrichment.DefaultValue, errors.InternalError("failed to create HTTP enricher", err)
	}

	// Perform enrichment
	result, err := httpEnricher.Enrich(ctx, data)
	if err != nil {
		// Return default value on error (could be configured to fail instead)
		return enrichment.DefaultValue, errors.InternalError("HTTP enrichment failed", err)
	}

	return result, nil
}

func (s *EnrichmentStage) Health() error {
	if !s.compiled {
		return errors.ConfigError("stage not configured")
	}
	return nil
}

// AggregationStage aggregates data over time windows
type AggregationStage struct {
	name     string
	config   AggregationConfig
	compiled bool
	buffer   map[string][]interface{} // aggregation buffer
	mu       sync.RWMutex
}

type AggregationConfig struct {
	WindowSize    int                   `json:"window_size"` // in seconds
	WindowType    string                `json:"window_type"` // fixed, sliding
	GroupBy       []string              `json:"group_by"`    // fields to group by
	Aggregations  []AggregationFunction `json:"aggregations"`
	OutputField   string                `json:"output_field"`   // where to store aggregated results
	FlushInterval int                   `json:"flush_interval"` // how often to flush results
}

type AggregationFunction struct {
	Name        string `json:"name"` // count, sum, avg, min, max
	SourceField string `json:"source_field"`
	TargetField string `json:"target_field"`
}

func NewAggregationStage(name string) *AggregationStage {
	return &AggregationStage{
		name:     name,
		compiled: false,
		buffer:   make(map[string][]interface{}),
	}
}

func (s *AggregationStage) Name() string {
	return s.name
}

func (s *AggregationStage) Type() string {
	return "aggregate"
}

func (s *AggregationStage) Configure(config map[string]interface{}) error {
	configData, err := json.Marshal(config)
	if err != nil {
		return errors.InternalError("failed to marshal config", err)
	}

	if err := json.Unmarshal(configData, &s.config); err != nil {
		return errors.InternalError("failed to unmarshal config", err)
	}

	// Set defaults
	if s.config.WindowType == "" {
		s.config.WindowType = "fixed"
	}
	if s.config.FlushInterval == 0 {
		s.config.FlushInterval = 60 // 1 minute default
	}

	s.compiled = true
	return nil
}

func (s *AggregationStage) Validate() error {
	if !s.compiled {
		return errors.ConfigError("stage not configured")
	}

	if s.config.WindowSize <= 0 {
		return errors.ConfigError("window_size must be positive")
	}

	if len(s.config.Aggregations) == 0 {
		return errors.ConfigError("aggregation stage requires at least one aggregation function")
	}

	validWindowTypes := map[string]bool{"fixed": true, "sliding": true}
	if !validWindowTypes[s.config.WindowType] {
		return errors.ConfigError(fmt.Sprintf("invalid window_type: %s", s.config.WindowType))
	}

	return nil
}

func (s *AggregationStage) Process(ctx context.Context, data *pipeline.Data) (*pipeline.StageResult, error) {
	start := time.Now()

	result := &pipeline.StageResult{
		Success:  false,
		Metadata: make(map[string]interface{}),
	}

	// Parse input JSON
	var inputData map[string]interface{}
	if err := json.Unmarshal(data.Body, &inputData); err != nil {
		result.Error = fmt.Sprintf("failed to parse JSON: %v", err)
		result.Duration = time.Since(start)
		return result, nil
	}

	// Generate group key
	groupKey := s.generateGroupKey(inputData)

	// Add data to buffer
	s.mu.Lock()
	if s.buffer[groupKey] == nil {
		s.buffer[groupKey] = make([]interface{}, 0)
	}
	s.buffer[groupKey] = append(s.buffer[groupKey], inputData)
	s.mu.Unlock()

	// For simplicity, always return aggregated results
	// In production, this would be triggered by timers
	aggregatedData := s.computeAggregations(groupKey)

	// Add aggregated data to input
	if s.config.OutputField != "" {
		inputData[s.config.OutputField] = aggregatedData
	} else {
		inputData["aggregated"] = aggregatedData
	}

	// Marshal output JSON
	outputBytes, err := json.Marshal(inputData)
	if err != nil {
		result.Error = fmt.Sprintf("failed to marshal output JSON: %v", err)
		result.Duration = time.Since(start)
		return result, nil
	}

	// Update pipeline data
	resultData := data.Clone()
	resultData.Body = outputBytes
	resultData.SetHeader("Content-Type", "application/json")

	result.Data = resultData
	result.Success = true
	result.Duration = time.Since(start)
	result.Metadata["group_key"] = groupKey
	result.Metadata["buffer_size"] = len(s.buffer[groupKey])

	return result, nil
}

func (s *AggregationStage) generateGroupKey(data map[string]interface{}) string {
	if len(s.config.GroupBy) == 0 {
		return "default"
	}

	key := ""
	for _, field := range s.config.GroupBy {
		if value, exists := data[field]; exists {
			key += fmt.Sprintf("%s:%v;", field, value)
		}
	}

	if key == "" {
		return "default"
	}

	return key
}

func (s *AggregationStage) computeAggregations(groupKey string) map[string]interface{} {
	s.mu.RLock()
	buffer := s.buffer[groupKey]
	s.mu.RUnlock()

	if len(buffer) == 0 {
		return make(map[string]interface{})
	}

	results := make(map[string]interface{})

	for _, aggFunc := range s.config.Aggregations {
		switch aggFunc.Name {
		case "count":
			results[aggFunc.TargetField] = len(buffer)

		case "sum":
			sum := 0.0
			for _, item := range buffer {
				if data, ok := item.(map[string]interface{}); ok {
					if value, exists := data[aggFunc.SourceField]; exists {
						if numValue, err := s.toFloat64(value); err == nil {
							sum += numValue
						}
					}
				}
			}
			results[aggFunc.TargetField] = sum

		case "avg":
			sum := 0.0
			count := 0
			for _, item := range buffer {
				if data, ok := item.(map[string]interface{}); ok {
					if value, exists := data[aggFunc.SourceField]; exists {
						if numValue, err := s.toFloat64(value); err == nil {
							sum += numValue
							count++
						}
					}
				}
			}
			if count > 0 {
				results[aggFunc.TargetField] = sum / float64(count)
			} else {
				results[aggFunc.TargetField] = 0
			}
		}
	}

	return results
}

func (s *AggregationStage) toFloat64(value interface{}) (float64, error) {
	return common.ToFloat64Simple(value)
}

func (s *AggregationStage) Health() error {
	if !s.compiled {
		return errors.ConfigError("stage not configured")
	}
	return nil
}

// Stage factories
type EnrichmentStageFactory struct {
	redisClient *redis.Client // Optional Redis client for distributed operations
}

func (f *EnrichmentStageFactory) Create(config map[string]interface{}) (pipeline.Stage, error) {
	name, ok := config["name"].(string)
	if !ok {
		name = "enrich"
	}

	stage := NewEnrichmentStage(name)

	// Set Redis client if available
	if f.redisClient != nil {
		stage.SetRedisClient(f.redisClient)
	}

	if err := stage.Configure(config); err != nil {
		return nil, err
	}

	return stage, nil
}

// NewEnrichmentStageFactory creates a factory with optional Redis client
func NewEnrichmentStageFactory(redisClient *redis.Client) *EnrichmentStageFactory {
	return &EnrichmentStageFactory{
		redisClient: redisClient,
	}
}

func (f *EnrichmentStageFactory) GetType() string {
	return "enrich"
}

func (f *EnrichmentStageFactory) GetConfigSchema() map[string]interface{} {
	return map[string]interface{}{
		"enrichments": map[string]interface{}{
			"type":        "array",
			"description": "List of enrichments to apply",
			"required":    true,
		},
		"parallel": map[string]interface{}{
			"type":        "boolean",
			"description": "Whether to run enrichments in parallel",
		},
	}
}

type AggregationStageFactory struct{}

func (f *AggregationStageFactory) Create(config map[string]interface{}) (pipeline.Stage, error) {
	name, ok := config["name"].(string)
	if !ok {
		name = "aggregate"
	}

	stage := NewAggregationStage(name)
	if err := stage.Configure(config); err != nil {
		return nil, err
	}

	return stage, nil
}

func (f *AggregationStageFactory) GetType() string {
	return "aggregate"
}

func (f *AggregationStageFactory) GetConfigSchema() map[string]interface{} {
	return map[string]interface{}{
		"window_size": map[string]interface{}{
			"type":        "integer",
			"description": "Aggregation window size in seconds",
			"required":    true,
		},
		"aggregations": map[string]interface{}{
			"type":        "array",
			"description": "List of aggregation functions",
			"required":    true,
		},
	}
}
