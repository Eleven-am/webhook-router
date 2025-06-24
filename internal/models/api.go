package models

import (
	"time"
	"webhook-router/internal/pipeline/core"
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

// PipelineConfigAPI represents pipeline_old configuration for API responses
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
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	Type        string                 `json:"type"`
	DependsOn   []string               `json:"depends_on,omitempty"`
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

// ToPipelineConfigAPI converts a pipeline definition to API format
func ToPipelineConfigAPI(config interface{}) *PipelineConfigAPI {
	if config == nil {
		return nil
	}

	switch p := config.(type) {
	case *core.PipelineDefinition:
		stages := make([]StageConfigAPI, len(p.Stages))
		for i, stage := range p.Stages {
			stages[i] = StageConfigAPI{
				ID:        stage.ID,
				Type:      stage.Type,
				DependsOn: stage.DependsOn,
				Config: map[string]interface{}{
					"target":  stage.Target,
					"action":  stage.Action,
					"timeout": stage.Timeout,
				},
			}
		}

		return &PipelineConfigAPI{
			ID:          p.ID,
			Name:        p.Name,
			Description: "", // Not in core.PipelineDefinition
			Stages:      stages,
			Enabled:     true,       // Default to enabled
			Tags:        []string{}, // Not in core.PipelineDefinition
			CreatedAt:   time.Now(), // Default timestamps
			UpdatedAt:   time.Now(),
		}
	case map[string]interface{}:
		// Handle map format
		result := &PipelineConfigAPI{
			Enabled:   true,
			Tags:      []string{},
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		if id, ok := p["id"].(string); ok {
			result.ID = id
		}
		if name, ok := p["name"].(string); ok {
			result.Name = name
		}
		if desc, ok := p["description"].(string); ok {
			result.Description = desc
		}

		// Handle stages if present
		if stages, ok := p["stages"].([]interface{}); ok {
			result.Stages = make([]StageConfigAPI, len(stages))
			for i, stage := range stages {
				if stageMap, ok := stage.(map[string]interface{}); ok {
					result.Stages[i] = StageConfigAPI{
						Config: stageMap,
					}
					if id, ok := stageMap["id"].(string); ok {
						result.Stages[i].ID = id
					}
					if stageType, ok := stageMap["type"].(string); ok {
						result.Stages[i].Type = stageType
					}
				}
			}
		}

		return result
	default:
		return nil
	}
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

// RouteAPI removed - routing is now handled by triggers
