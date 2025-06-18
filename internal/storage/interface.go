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
	// LogWebhook stores a webhook processing log entry including timing metrics
	LogWebhook(log *WebhookLog) error
	
	// GetStats returns overall system statistics for the last 24 hours including:
	// - Total requests processed
	// - Successful requests (2xx status codes)
	// - Failed requests (4xx+ status codes)
	// - Number of active routes
	GetStats() (*Stats, error)
	
	// GetRouteStats returns detailed statistics for a specific route including:
	// - Total, successful, and failed request counts
	// - Average transformation time in milliseconds
	// - Average broker publish time in milliseconds
	// - Timestamp of last processed request
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

	// Dead Letter Queue
	CreateDLQMessage(message *DLQMessage) error
	GetDLQMessage(id int) (*DLQMessage, error)
	GetDLQMessageByMessageID(messageID string) (*DLQMessage, error)
	ListPendingDLQMessages(limit int) ([]*DLQMessage, error)
	ListDLQMessages(limit, offset int) ([]*DLQMessage, error)
	ListDLQMessagesByRoute(routeID int, limit, offset int) ([]*DLQMessage, error)
	ListDLQMessagesByStatus(status string, limit, offset int) ([]*DLQMessage, error)
	UpdateDLQMessage(message *DLQMessage) error
	UpdateDLQMessageStatus(id int, status string) error
	DeleteDLQMessage(id int) error
	DeleteOldDLQMessages(before time.Time) error
	GetDLQStats() (*DLQStats, error)
	GetDLQStatsByRoute() ([]*DLQRouteStats, error)

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
	
	// Signature verification fields
	SignatureConfig     string    `json:"signature_config,omitempty"`
	SignatureSecret     string    `json:"signature_secret,omitempty"`
}

type User struct {
	ID           int       `json:"id"`
	Username     string    `json:"username"`
	PasswordHash string    `json:"-"`
	IsDefault    bool      `json:"is_default"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// WebhookLog represents a single webhook processing event with performance metrics
type WebhookLog struct {
	// ID is the unique identifier for this log entry
	ID                      int       `json:"id"`
	// RouteID links this log to the route that processed it
	RouteID                 int       `json:"route_id"`
	// Method is the HTTP method of the webhook request
	Method                  string    `json:"method"`
	// Endpoint is the URL path that received the webhook
	Endpoint                string    `json:"endpoint"`
	// Headers contains the JSON-encoded request headers
	Headers                 string    `json:"headers"`
	// Body contains the raw request body
	Body                    string    `json:"body"`
	// StatusCode is the HTTP response code returned
	StatusCode              int       `json:"status_code"`
	// Error contains any error message if processing failed
	Error                   string    `json:"error,omitempty"`
	// ProcessedAt is when the webhook was processed
	ProcessedAt             time.Time `json:"processed_at"`
	
	// TriggerID links to the trigger that initiated this webhook (optional)
	TriggerID               *int      `json:"trigger_id,omitempty"`
	// PipelineID links to the pipeline used for transformation (optional)
	PipelineID              *int      `json:"pipeline_id,omitempty"`
	// TransformationTimeMS is the time spent in pipeline transformation (milliseconds)
	TransformationTimeMS    int       `json:"transformation_time_ms"`
	// BrokerPublishTimeMS is the time spent publishing to the broker (milliseconds)
	BrokerPublishTimeMS     int       `json:"broker_publish_time_ms"`
}

// Stats represents system-wide webhook processing statistics
type Stats struct {
	// TotalRequests is the total number of webhook requests processed
	TotalRequests   int `json:"total_requests"`
	// SuccessRequests is the count of requests with 2xx status codes
	SuccessRequests int `json:"success_requests"`
	// FailedRequests is the count of requests with 4xx+ status codes
	FailedRequests  int `json:"failed_requests"`
	// ActiveRoutes is the number of currently active webhook routes
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
	ID              int                    `json:"id"`
	Name            string                 `json:"name"`
	Type            string                 `json:"type"`
	Config          map[string]interface{} `json:"config"`
	Active          bool                   `json:"active"`
	HealthStatus    string                 `json:"health_status"`
	LastHealthCheck *time.Time             `json:"last_health_check,omitempty"`
	CreatedAt       time.Time              `json:"created_at"`
	UpdatedAt       time.Time              `json:"updated_at"`
	DlqEnabled      *bool                  `json:"dlq_enabled,omitempty"`
	DlqBrokerID     *int64                 `json:"dlq_broker_id,omitempty"`
}

type StorageFactory interface {
	Create(config StorageConfig) (Storage, error)
	GetType() string
}

// GenericConfig is a simple map-based implementation of StorageConfig
type GenericConfig map[string]interface{}

func (gc GenericConfig) Validate() error {
	return nil // Basic configs don't need validation
}

func (gc GenericConfig) GetType() string {
	if t, ok := gc["type"].(string); ok {
		return t
	}
	return "unknown"
}

func (gc GenericConfig) GetConnectionString() string {
	if cs, ok := gc["connection_string"].(string); ok {
		return cs
	}
	return ""
}

// Statistics represents usage statistics
type Statistics struct {
	TotalRequests      int
	SuccessfulRequests int
	FailedRequests     int
	Since              time.Time
	TopEndpoints       []EndpointStats
	ErrorBreakdown     map[string]int
}

// EndpointStats represents statistics for a specific endpoint
type EndpointStats struct {
	Endpoint    string
	Method      string
	Count       int
	SuccessRate float64
}

// DLQMessage represents a message in the Dead Letter Queue
type DLQMessage struct {
	ID           int                    `json:"id"`
	MessageID    string                 `json:"message_id"`
	RouteID      int                    `json:"route_id"`
	TriggerID    *int                   `json:"trigger_id,omitempty"`
	PipelineID   *int                   `json:"pipeline_id,omitempty"`
	BrokerName   string                 `json:"broker_name"`
	Queue        string                 `json:"queue"`
	Exchange     string                 `json:"exchange"`
	RoutingKey   string                 `json:"routing_key"`
	Headers      map[string]string      `json:"headers"`
	Body         string                 `json:"body"`
	ErrorMessage string                 `json:"error_message"`
	FailureCount int                    `json:"failure_count"`
	FirstFailure time.Time              `json:"first_failure"`
	LastFailure  time.Time              `json:"last_failure"`
	NextRetry    *time.Time             `json:"next_retry,omitempty"`
	Status       string                 `json:"status"` // pending, retrying, abandoned
	Metadata     map[string]interface{} `json:"metadata"`
	CreatedAt    time.Time              `json:"created_at"`
	UpdatedAt    time.Time              `json:"updated_at"`
}

// DLQStats represents statistics for the Dead Letter Queue
type DLQStats struct {
	TotalMessages      int64      `json:"total_messages"`
	PendingMessages    int64      `json:"pending_messages"`
	RetryingMessages   int64      `json:"retrying_messages"`
	AbandonedMessages  int64      `json:"abandoned_messages"`
	OldestFailure      *time.Time `json:"oldest_failure,omitempty"`
}

// DLQRouteStats represents DLQ statistics grouped by route
type DLQRouteStats struct {
	RouteID        int   `json:"route_id"`
	MessageCount   int64 `json:"message_count"`
	PendingCount   int64 `json:"pending_count"`
	AbandonedCount int64 `json:"abandoned_count"`
}