package storage

import (
	"time"
)

type Storage interface {
	// Connection management
	Connect(config StorageConfig) error
	Close() error
	Health() error

	// Users (existing functionality)
	CreateUser(username, password string) (*User, error)
	GetUser(userID string) (*User, error)
	GetUserByUsername(username string) (*User, error)
	ValidateUser(username, password string) (*User, error)
	UpdateUserCredentials(userID string, username, password string) error
	IsDefaultUser(userID string) (bool, error)
	GetUserCount() (int, error)

	// Settings (existing functionality)
	GetSetting(key string) (string, error)
	SetSetting(key, value string) error
	GetAllSettings() (map[string]string, error)

	// Execution logs (comprehensive trigger and pipeline_old tracking)
	// CreateExecutionLog creates a new execution log entry
	CreateExecutionLog(log *ExecutionLog) error

	// UpdateExecutionLog updates an existing execution log with new data
	UpdateExecutionLog(log *ExecutionLog) error

	// GetExecutionLog retrieves a single execution log by ID
	GetExecutionLog(id string) (*ExecutionLog, error)

	// ListExecutionLogs retrieves paginated execution logs for a user
	ListExecutionLogs(userID string, limit, offset int) ([]*ExecutionLog, error)

	// ListExecutionLogsWithCount retrieves paginated execution logs with total count for a user
	ListExecutionLogsWithCount(userID string, limit, offset int) ([]*ExecutionLog, int, error)

	// GetExecutionLogsByTriggerID retrieves execution logs for a specific trigger
	GetExecutionLogsByTriggerID(triggerID string, userID string, limit, offset int) ([]*ExecutionLog, error)

	// GetExecutionLogsByTriggerType retrieves execution logs for a specific trigger type
	GetExecutionLogsByTriggerType(triggerType string, userID string, limit, offset int) ([]*ExecutionLog, error)

	// GetExecutionLogsByStatus retrieves execution logs by status (success/error/processing)
	GetExecutionLogsByStatus(status string, userID string, limit, offset int) ([]*ExecutionLog, error)

	// GetStats returns overall system statistics for the last 24 hours including:
	// - Total requests processed
	// - Successful requests (2xx status codes)
	// - Failed requests (4xx+ status codes)
	// - Number of active triggers
	GetStats() (*Stats, error)

	// GetTriggerStats returns detailed statistics for a specific trigger including:
	// - Total, successful, and failed request counts
	// - Average transformation time in milliseconds
	// - Average broker publish time in milliseconds
	// - Timestamp of last processed request
	GetTriggerStats(triggerID string) (map[string]interface{}, error)

	// GetDashboardStats returns comprehensive dashboard statistics including:
	// - Metrics with current and previous period comparisons
	// - Time series data for charts (optionally filtered by route)
	// - Top triggers with stats
	// - Recent activity feed
	// Pass empty string for triggerID to get stats for all triggers
	GetDashboardStats(userID string, currentStart, previousStart, now time.Time, triggerID string) (*DashboardStats, error)

	// New functionality for Phase 1+
	CreateTrigger(trigger *Trigger) error
	GetTrigger(id string) (*Trigger, error)
	GetHTTPTriggerByUserPathMethod(userID, path, method string) (*Trigger, error)
	GetTriggers(filters TriggerFilters) ([]*Trigger, error)
	GetTriggersPaginated(filters TriggerFilters, limit, offset int) ([]*Trigger, int, error) // returns triggers and total count
	UpdateTrigger(trigger *Trigger) error
	DeleteTrigger(id string) error

	CreatePipeline(pipeline *Pipeline) error
	GetPipeline(id string) (*Pipeline, error)
	GetPipelines() ([]*Pipeline, error)
	GetPipelinesPaginated(limit, offset int) ([]*Pipeline, int, error) // returns pipelines and total count
	UpdatePipeline(pipeline *Pipeline) error
	DeletePipeline(id string) error

	CreateBroker(broker *BrokerConfig) error
	GetBroker(id string) (*BrokerConfig, error)
	GetBrokers() ([]*BrokerConfig, error)
	GetBrokersPaginated(limit, offset int) ([]*BrokerConfig, int, error) // returns brokers and total count
	UpdateBroker(broker *BrokerConfig) error
	DeleteBroker(id string) error

	// Dead Letter Queue
	CreateDLQMessage(message *DLQMessage) error
	GetDLQMessage(id string) (*DLQMessage, error)
	GetDLQMessageByMessageID(messageID string) (*DLQMessage, error)
	ListPendingDLQMessages(limit int) ([]*DLQMessage, error)
	ListDLQMessages(limit, offset int) ([]*DLQMessage, error)
	ListDLQMessagesWithCount(limit, offset int) ([]*DLQMessage, int, error) // returns messages and total count
	ListDLQMessagesByTrigger(triggerID string, limit, offset int) ([]*DLQMessage, error)
	ListDLQMessagesByTriggerWithCount(triggerID string, limit, offset int) ([]*DLQMessage, int, error) // returns messages and total count
	ListDLQMessagesByStatus(status string, limit, offset int) ([]*DLQMessage, error)
	UpdateDLQMessage(message *DLQMessage) error
	UpdateDLQMessageStatus(id string, status string) error
	DeleteDLQMessage(id string) error
	DeleteOldDLQMessages(before time.Time) error
	GetDLQStats() (*DLQStats, error)
	GetDLQStatsByTrigger() ([]*DLQTriggerStats, error)
	GetDLQStatsByError() ([]*DLQErrorStats, error)

	// OAuth2 Services
	CreateOAuth2Service(service *OAuth2Service) error
	GetOAuth2Service(id string) (*OAuth2Service, error)
	GetOAuth2ServiceByName(name string, userID string) (*OAuth2Service, error)
	ListOAuth2Services(userID string) ([]*OAuth2Service, error)
	UpdateOAuth2Service(service *OAuth2Service) error
	DeleteOAuth2Service(id string) error

	// Generic operations
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

type User struct {
	ID           string    `json:"id"`
	Username     string    `json:"username"`
	PasswordHash string    `json:"-"`
	IsDefault    bool      `json:"is_default"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// ExecutionLog represents a comprehensive execution log for all trigger types
type ExecutionLog struct {
	// ID is the unique identifier for this execution log entry
	ID string `json:"id"`

	// Trigger information
	TriggerID     *string `json:"trigger_id,omitempty"`
	TriggerType   string  `json:"trigger_type"`   // http, schedule, polling, broker, imap, caldav, carddav
	TriggerConfig string  `json:"trigger_config"` // JSON config specific to trigger type

	// Input data
	InputMethod   string `json:"input_method"`   // HTTP method for HTTP triggers, or trigger-specific method
	InputEndpoint string `json:"input_endpoint"` // HTTP endpoint or trigger-specific source
	InputHeaders  string `json:"input_headers"`  // JSON headers or metadata
	InputBody     string `json:"input_body"`     // Request body or trigger payload

	// Pipeline processing
	PipelineID           *string `json:"pipeline_id,omitempty"`
	PipelineStages       string  `json:"pipeline_stages"`     // JSON array of stages executed
	TransformationData   string  `json:"transformation_data"` // JSON of transformation details
	TransformationTimeMS int     `json:"transformation_time_ms"`

	// Broker publishing
	BrokerID            *string `json:"broker_id,omitempty"`
	BrokerType          string  `json:"broker_type"` // rabbitmq, kafka, redis, aws, gcp
	BrokerQueue         string  `json:"broker_queue"`
	BrokerExchange      string  `json:"broker_exchange"`
	BrokerRoutingKey    string  `json:"broker_routing_key"`
	BrokerPublishTimeMS int     `json:"broker_publish_time_ms"`
	BrokerResponse      string  `json:"broker_response"` // Success/failure info from broker

	// Final results
	Status         string `json:"status"`      // processing, success, error, partial
	StatusCode     int    `json:"status_code"` // HTTP status for HTTP triggers, or custom status
	ErrorMessage   string `json:"error_message,omitempty"`
	OutputData     string `json:"output_data"` // Final processed data
	TotalLatencyMS int    `json:"total_latency_ms"`

	// Timestamps
	StartedAt   time.Time  `json:"started_at"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`

	// User association (for scoping)
	UserID string `json:"user_id"`
}

// Stats represents system-wide webhook processing statistics
type Stats struct {
	// TotalRequests is the total number of webhook requests processed
	TotalRequests int `json:"total_requests"`
	// SuccessRequests is the count of requests with 2xx status codes
	SuccessRequests int `json:"success_requests"`
	// FailedRequests is the count of requests with 4xx+ status codes
	FailedRequests int `json:"failed_requests"`
	// ActiveTriggers is the number of currently active triggers
	ActiveTriggers int `json:"active_triggers"`
}

// DashboardStats represents comprehensive dashboard statistics
type DashboardStats struct {
	// Metrics with current and previous period values
	Metrics DashboardMetrics `json:"metrics"`
	// Time series data for charts
	TimeSeries []TimeSeriesPoint `json:"time_series"`
	// Top triggers by request volume
	TopTriggers []TriggerStatsSummary `json:"top_triggers"`
	// Recent activity events
	RecentActivity []ActivityEvent `json:"recent_activity"`
}

// DashboardMetrics contains all dashboard metric cards data
type DashboardMetrics struct {
	ActiveTriggers MetricWithChange `json:"active_triggers"`
	TotalRequests  MetricWithChange `json:"total_requests"`
	SuccessRate    MetricWithChange `json:"success_rate"`
	AverageLatency MetricWithChange `json:"average_latency"`
	DLQMessages    MetricWithChange `json:"dlq_messages"`
}

// MetricWithChange represents a metric value with change information
type MetricWithChange struct {
	Current       float64 `json:"current"`
	Previous      float64 `json:"previous"`
	Change        float64 `json:"change"`
	ChangePercent float64 `json:"change_percent"`
}

// TimeSeriesPoint represents a data point in time series
type TimeSeriesPoint struct {
	Timestamp time.Time `json:"timestamp"`
	Requests  int       `json:"requests"`
	Errors    int       `json:"errors"`
}

// TriggerStatsSummary represents summarized stats for a trigger
type TriggerStatsSummary struct {
	ID            string     `json:"id"`
	Name          string     `json:"name"`
	Type          string     `json:"type"`
	Active        bool       `json:"active"`
	TotalRequests int        `json:"total_requests"`
	SuccessRate   float64    `json:"success_rate"`
	AvgLatencyMs  float64    `json:"avg_latency_ms"`
	LastProcessed *time.Time `json:"last_processed,omitempty"`
}

// ActivityEvent represents a recent activity event
type ActivityEvent struct {
	Type      string    `json:"type"` // success, error, info
	Trigger   string    `json:"trigger"`
	Message   string    `json:"message"`
	Timestamp time.Time `json:"timestamp"`
}

// New structures for Phase 1
type Trigger struct {
	ID            string                 `json:"id"`
	Name          string                 `json:"name"`
	Type          string                 `json:"type"`
	Config        map[string]interface{} `json:"config"` // For HTTP triggers, includes path, method, etc.
	Status        string                 `json:"status"`
	Active        bool                   `json:"active"`
	ErrorMessage  string                 `json:"error_message,omitempty"`
	LastExecution *time.Time             `json:"last_execution,omitempty"`
	NextExecution *time.Time             `json:"next_execution,omitempty"`
	CreatedAt     time.Time              `json:"created_at"`
	UpdatedAt     time.Time              `json:"updated_at"`

	// Routing and processing configuration
	PipelineID          *string `json:"pipeline_id,omitempty"`
	DestinationBrokerID *string `json:"destination_broker_id,omitempty"`

	// Signature configuration (for HTTP triggers)
	SignatureConfig string `json:"signature_config,omitempty"`
	SignatureSecret string `json:"signature_secret,omitempty"`

	// DLQ configuration
	DLQBrokerID *string `json:"dlq_broker_id,omitempty"`
	DLQEnabled  bool    `json:"dlq_enabled"`
	DLQRetryMax int     `json:"dlq_retry_max"`

	// User association
	UserID    string     `json:"user_id"`
	DeletedAt *time.Time `json:"deleted_at,omitempty"`
}

type TriggerFilters struct {
	Type   string
	Status string
	Active *bool
}

type Pipeline struct {
	ID          string                   `json:"id"`
	Name        string                   `json:"name"`
	Description string                   `json:"description"`
	Stages      []map[string]interface{} `json:"stages"`
	Active      bool                     `json:"active"`
	CreatedAt   time.Time                `json:"created_at"`
	UpdatedAt   time.Time                `json:"updated_at"`
}

type BrokerConfig struct {
	ID              string                 `json:"id"`
	Name            string                 `json:"name"`
	Type            string                 `json:"type"`
	Config          map[string]interface{} `json:"config"`
	Active          bool                   `json:"active"`
	HealthStatus    string                 `json:"health_status"`
	LastHealthCheck *time.Time             `json:"last_health_check,omitempty"`
	CreatedAt       time.Time              `json:"created_at"`
	UpdatedAt       time.Time              `json:"updated_at"`
	DlqEnabled      *bool                  `json:"dlq_enabled,omitempty"`
	DlqBrokerID     *string                `json:"dlq_broker_id,omitempty"`
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
	ID           string                 `json:"id"`
	MessageID    string                 `json:"message_id"`
	TriggerID    *string                `json:"trigger_id,omitempty"`
	PipelineID   *string                `json:"pipeline_id,omitempty"`
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
	TotalMessages     int64      `json:"total_messages"`
	PendingMessages   int64      `json:"pending_messages"`
	RetryingMessages  int64      `json:"retrying_messages"`
	AbandonedMessages int64      `json:"abandoned_messages"`
	OldestFailure     *time.Time `json:"oldest_failure,omitempty"`
}

// DLQTriggerStats represents DLQ statistics grouped by trigger
type DLQTriggerStats struct {
	TriggerID      string `json:"trigger_id"`
	MessageCount   int64  `json:"message_count"`
	PendingCount   int64  `json:"pending_count"`
	AbandonedCount int64  `json:"abandoned_count"`
}

// DLQErrorStats represents DLQ statistics grouped by error message
type DLQErrorStats struct {
	ErrorMessage string    `json:"error_message"`
	MessageCount int64     `json:"message_count"`
	FirstSeen    time.Time `json:"first_seen"`
	LastSeen     time.Time `json:"last_seen"`
}

// OAuth2Service represents an OAuth2 service configuration
type OAuth2Service struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	ClientID     string    `json:"client_id"`
	ClientSecret string    `json:"-"` // Encrypted
	TokenURL     string    `json:"token_url"`
	AuthURL      string    `json:"auth_url,omitempty"`
	RedirectURL  string    `json:"redirect_url,omitempty"`
	Scopes       []string  `json:"scopes,omitempty"`
	GrantType    string    `json:"grant_type"`
	UserID       string    `json:"user_id"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}
