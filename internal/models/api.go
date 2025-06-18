package models

import (
	"fmt"
	"time"
	"webhook-router/internal/pipeline"
	"webhook-router/internal/routing"
	"webhook-router/internal/storage"
)

// API-friendly models for Swagger documentation
// These models use string representations for time.Duration fields

// RouteRuleAPI represents a routing rule for API responses
type RouteRuleAPI struct {
	ID            string                 `json:"id"`
	Name          string                 `json:"name"`
	Description   string                 `json:"description"`
	Priority      int                    `json:"priority"`
	Enabled       bool                   `json:"enabled"`
	Conditions    []RuleConditionAPI     `json:"conditions"`
	Destinations  []RouteDestinationAPI  `json:"destinations"`
	LoadBalancing LoadBalancingConfigAPI `json:"load_balancing"`
	Tags          []string               `json:"tags"`
	CreatedAt     time.Time              `json:"created_at"`
	UpdatedAt     time.Time              `json:"updated_at"`
}

// RuleConditionAPI represents a rule condition for API responses
type RuleConditionAPI struct {
	Type     string      `json:"type"`
	Field    string      `json:"field,omitempty"`
	Operator string      `json:"operator"`
	Value    interface{} `json:"value"`
	Negate   bool        `json:"negate,omitempty"`
}

// RouteDestinationAPI represents a route destination for API responses
type RouteDestinationAPI struct {
	ID          string                 `json:"id"`
	Type        string                 `json:"type"`
	URL         string                 `json:"url,omitempty"`
	BrokerQueue string                 `json:"broker_queue,omitempty"`
	PipelineID  string                 `json:"pipeline_id,omitempty"`
	Priority    int                    `json:"priority"`
	Weight      int                    `json:"weight"`
	Timeout     string                 `json:"timeout"` // Duration as string
	HealthCheck HealthCheckConfigAPI   `json:"health_check"`
	Config      map[string]interface{} `json:"config,omitempty"`
}

// HealthCheckConfigAPI represents health check configuration for API responses
type HealthCheckConfigAPI struct {
	Enabled            bool   `json:"enabled"`
	Endpoint           string `json:"endpoint"`
	Interval           string `json:"interval"` // Duration as string
	Timeout            string `json:"timeout"`  // Duration as string
	HealthyThreshold   int    `json:"healthy_threshold"`
	UnhealthyThreshold int    `json:"unhealthy_threshold"`
}

// LoadBalancingConfigAPI represents load balancing configuration for API responses
type LoadBalancingConfigAPI struct {
	Strategy      string `json:"strategy"`
	HashKey       string `json:"hash_key,omitempty"`
	StickySession bool   `json:"sticky_session"`
	SessionKey    string `json:"session_key,omitempty"`
}

// PipelineConfigAPI represents pipeline configuration for API responses
type PipelineConfigAPI struct {
	ID          string           `json:"id"`
	Name        string           `json:"name"`
	Description string           `json:"description"`
	Stages      []StageConfigAPI `json:"stages"`
	Enabled     bool             `json:"enabled"`
	Tags        []string         `json:"tags"`
	CreatedAt   time.Time        `json:"created_at"`
	UpdatedAt   time.Time        `json:"updated_at"`
}

// StageConfigAPI represents stage configuration for API responses
type StageConfigAPI struct {
	Name        string                 `json:"name"`
	Type        string                 `json:"type"`
	Config      map[string]interface{} `json:"config"`
	Enabled     bool                   `json:"enabled"`
	OnError     string                 `json:"on_error"`
	RetryConfig RetryConfigAPI         `json:"retry_config"`
	Timeout     string                 `json:"timeout"` // Duration as string
}

// RetryConfigAPI represents retry configuration for API responses
type RetryConfigAPI struct {
	Enabled     bool   `json:"enabled"`
	MaxAttempts int    `json:"max_attempts"`
	Delay       string `json:"delay"` // Duration as string
	BackoffType string `json:"backoff_type"`
	MaxDelay    string `json:"max_delay"` // Duration as string
}

// Conversion functions to convert between internal models and API models

// ToRouteRuleAPI converts routing.RouteRule to RouteRuleAPI
func ToRouteRuleAPI(rule *routing.RouteRule) *RouteRuleAPI {
	if rule == nil {
		return nil
	}

	api := &RouteRuleAPI{
		ID:          rule.ID,
		Name:        rule.Name,
		Description: rule.Description,
		Priority:    rule.Priority,
		Enabled:     rule.Enabled,
		Tags:        rule.Tags,
		CreatedAt:   rule.CreatedAt,
		UpdatedAt:   rule.UpdatedAt,
	}

	// Convert conditions
	api.Conditions = make([]RuleConditionAPI, len(rule.Conditions))
	for i, cond := range rule.Conditions {
		api.Conditions[i] = RuleConditionAPI{
			Type:     cond.Type,
			Field:    cond.Field,
			Operator: cond.Operator,
			Value:    cond.Value,
			Negate:   cond.Negate,
		}
	}

	// Convert destinations
	api.Destinations = make([]RouteDestinationAPI, len(rule.Destinations))
	for i, dest := range rule.Destinations {
		api.Destinations[i] = RouteDestinationAPI{
			ID:       dest.ID,
			Type:     dest.Type,
			Priority: dest.Priority,
			Weight:   dest.Weight,
			Config:   dest.Config,
			HealthCheck: HealthCheckConfigAPI{
				Enabled:            dest.HealthCheck.Enabled,
				Interval:           dest.HealthCheck.Interval.String(),
				Timeout:            dest.HealthCheck.Timeout.String(),
				HealthyThreshold:   dest.HealthCheck.HealthyThreshold,
				UnhealthyThreshold: dest.HealthCheck.UnhealthyThreshold,
			},
		}
	}

	// Convert load balancing
	api.LoadBalancing = LoadBalancingConfigAPI{
		Strategy:      rule.LoadBalancing.Strategy,
		HashKey:       rule.LoadBalancing.HashKey,
		StickySession: rule.LoadBalancing.StickySession,
		SessionKey:    rule.LoadBalancing.SessionKey,
	}

	return api
}

// ToPipelineConfigAPI converts pipeline.PipelineConfig to PipelineConfigAPI
func ToPipelineConfigAPI(config *pipeline.Config) *PipelineConfigAPI {
	if config == nil {
		return nil
	}

	api := &PipelineConfigAPI{
		ID:          config.ID,
		Name:        config.Name,
		Description: config.Description,
		Enabled:     config.Enabled,
		Tags:        config.Tags,
		CreatedAt:   config.CreatedAt,
		UpdatedAt:   config.UpdatedAt,
	}

	// Convert stages
	api.Stages = make([]StageConfigAPI, len(config.Stages))
	for i, stage := range config.Stages {
		api.Stages[i] = StageConfigAPI{
			Name:    stage.Name,
			Type:    stage.Type,
			Config:  stage.Config,
			Enabled: stage.Enabled,
			OnError: stage.OnError,
			Timeout: stage.Timeout.String(),
			RetryConfig: RetryConfigAPI{
				Enabled:     stage.RetryConfig.Enabled,
				MaxAttempts: stage.RetryConfig.MaxAttempts,
				Delay:       stage.RetryConfig.Delay.String(),
				BackoffType: stage.RetryConfig.BackoffType,
				MaxDelay:    stage.RetryConfig.MaxDelay.String(),
			},
		}
	}

	return api
}

// BrokerConfigAPI represents broker configuration for API responses
type BrokerConfigAPI struct {
	Type   string                 `json:"type"`
	Config map[string]interface{} `json:"config"`
}

// ToBrokerConfigAPI converts storage.BrokerConfig to BrokerConfigAPI
func ToBrokerConfigAPI(config *storage.BrokerConfig) *BrokerConfigAPI {
	if config == nil {
		return nil
	}

	return &BrokerConfigAPI{
		Type:   config.Type,
		Config: config.Config,
	}
}

// RouteAPI represents a webhook route for API responses
type RouteAPI struct {
	ID        string    `json:"id"`
	Path      string    `json:"path"`
	Method    string    `json:"method"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ToRouteAPI converts storage.Route to RouteAPI
func ToRouteAPI(route *storage.Route) *RouteAPI {
	if route == nil {
		return nil
	}

	return &RouteAPI{
		ID:        fmt.Sprintf("%d", route.ID),
		Path:      route.Endpoint,
		Method:    route.Method,
		CreatedAt: route.CreatedAt,
		UpdatedAt: route.UpdatedAt,
	}
}
