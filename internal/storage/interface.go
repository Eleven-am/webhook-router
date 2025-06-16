package storage

import (
	"time"
)

type Storage interface {
	// Connection management
	Connect(config StorageConfig) error
	Close() error
	Health() error

	// Routes (existing functionality)
	CreateRoute(route *Route) error
	GetRoute(id int) (*Route, error)
	GetRoutes() ([]*Route, error)
	UpdateRoute(route *Route) error
	DeleteRoute(id int) error
	FindMatchingRoutes(endpoint, method string) ([]*Route, error)

	// Users (existing functionality)
	ValidateUser(username, password string) (*User, error)
	UpdateUserCredentials(userID int, username, password string) error
	IsDefaultUser(userID int) (bool, error)

	// Settings (existing functionality)
	GetSetting(key string) (string, error)
	SetSetting(key, value string) error
	GetAllSettings() (map[string]string, error)

	// Webhook logs (existing functionality)
	LogWebhook(log *WebhookLog) error
	GetStats() (*Stats, error)
	GetRouteStats(routeID int) (map[string]interface{}, error)

	// New functionality for Phase 1+
	CreateTrigger(trigger *Trigger) error
	GetTrigger(id int) (*Trigger, error)
	GetTriggers(filters TriggerFilters) ([]*Trigger, error)
	UpdateTrigger(trigger *Trigger) error
	DeleteTrigger(id int) error

	CreatePipeline(pipeline *Pipeline) error
	GetPipeline(id int) (*Pipeline, error)
	GetPipelines() ([]*Pipeline, error)
	UpdatePipeline(pipeline *Pipeline) error
	DeletePipeline(id int) error

	CreateBroker(broker *BrokerConfig) error
	GetBroker(id int) (*BrokerConfig, error)
	GetBrokers() ([]*BrokerConfig, error)
	UpdateBroker(broker *BrokerConfig) error
	DeleteBroker(id int) error

	// Generic operations
	Query(query string, args ...interface{}) ([]map[string]interface{}, error)
	Transaction(fn func(tx Transaction) error) error
}

type StorageConfig interface {
	Validate() error
	GetType() string
	GetConnectionString() string
}

type Transaction interface {
	Commit() error
	Rollback() error
}

// Existing structures (from database.go)
type Route struct {
	ID                  int       `json:"id"`
	Name                string    `json:"name"`
	Endpoint            string    `json:"endpoint"`
	Method              string    `json:"method"`
	Queue               string    `json:"queue"`
	Exchange            string    `json:"exchange"`
	RoutingKey          string    `json:"routing_key"`
	Filters             string    `json:"filters"`
	Headers             string    `json:"headers"`
	Active              bool      `json:"active"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
	
	// New fields for Phase 1
	PipelineID          *int      `json:"pipeline_id,omitempty"`
	TriggerID           *int      `json:"trigger_id,omitempty"`
	DestinationBrokerID *int      `json:"destination_broker_id,omitempty"`
	Priority            int       `json:"priority"`
	ConditionExpression string    `json:"condition_expression"`
}

type User struct {
	ID           int       `json:"id"`
	Username     string    `json:"username"`
	PasswordHash string    `json:"-"`
	IsDefault    bool      `json:"is_default"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type WebhookLog struct {
	ID                      int       `json:"id"`
	RouteID                 int       `json:"route_id"`
	Method                  string    `json:"method"`
	Endpoint                string    `json:"endpoint"`
	Headers                 string    `json:"headers"`
	Body                    string    `json:"body"`
	StatusCode              int       `json:"status_code"`
	Error                   string    `json:"error,omitempty"`
	ProcessedAt             time.Time `json:"processed_at"`
	
	// New fields for Phase 1
	TriggerID               *int      `json:"trigger_id,omitempty"`
	PipelineID              *int      `json:"pipeline_id,omitempty"`
	TransformationTimeMS    int       `json:"transformation_time_ms"`
	BrokerPublishTimeMS     int       `json:"broker_publish_time_ms"`
}

type Stats struct {
	TotalRequests   int `json:"total_requests"`
	SuccessRequests int `json:"success_requests"`
	FailedRequests  int `json:"failed_requests"`
	ActiveRoutes    int `json:"active_routes"`
}

// New structures for Phase 1
type Trigger struct {
	ID            int                    `json:"id"`
	Name          string                 `json:"name"`
	Type          string                 `json:"type"`
	Config        map[string]interface{} `json:"config"`
	Status        string                 `json:"status"`
	Active        bool                   `json:"active"`
	ErrorMessage  string                 `json:"error_message,omitempty"`
	LastExecution *time.Time             `json:"last_execution,omitempty"`
	NextExecution *time.Time             `json:"next_execution,omitempty"`
	CreatedAt     time.Time              `json:"created_at"`
	UpdatedAt     time.Time              `json:"updated_at"`
}

type TriggerFilters struct {
	Type   string
	Status string
	Active *bool
}

type Pipeline struct {
	ID          int                      `json:"id"`
	Name        string                   `json:"name"`
	Description string                   `json:"description"`
	Stages      []map[string]interface{} `json:"stages"`
	Active      bool                     `json:"active"`
	CreatedAt   time.Time                `json:"created_at"`
	UpdatedAt   time.Time                `json:"updated_at"`
}

type BrokerConfig struct {
	ID           int                    `json:"id"`
	Name         string                 `json:"name"`
	Type         string                 `json:"type"`
	Config       map[string]interface{} `json:"config"`
	Active       bool                   `json:"active"`
	HealthStatus string                 `json:"health_status"`
	LastHealthCheck *time.Time          `json:"last_health_check,omitempty"`
	CreatedAt    time.Time              `json:"created_at"`
	UpdatedAt    time.Time              `json:"updated_at"`
}

type StorageFactory interface {
	Create(config StorageConfig) (Storage, error)
	GetType() string
}