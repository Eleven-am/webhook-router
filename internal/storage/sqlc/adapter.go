// Package sqlc provides type-safe database adapters using SQLC code generation.
// It implements the storage.Storage interface for both SQLite and PostgreSQL databases.
//
// The package uses SQLC to generate Go code from SQL queries, providing:
// - Type safety for all database operations
// - Automatic NULL handling for nullable columns
// - Compile-time query validation
// - High performance with minimal overhead
//
// Usage:
//
//	db, err := sql.Open("sqlite3", "webhook.db")
//	if err != nil {
//		return err
//	}
//	adapter := sqlc.NewSQLCAdapter(db)
//
// The adapter handles all storage operations including routes, webhooks, triggers,
// pipelines, brokers, and statistics.
package sqlc

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"time"

	"github.com/lucsky/cuid"
	"golang.org/x/crypto/bcrypt"
	"webhook-router/internal/common/errors"
	"webhook-router/internal/crypto"
	"webhook-router/internal/storage"
	sqlite "webhook-router/internal/storage/generated/sqlite"
)

// SQLCAdapter implements Storage interface using SQLC generated code
type SQLCAdapter struct {
	BaseAdapter
	db        *sql.DB
	queries   *sqlite.Queries
	encryptor *crypto.ConfigEncryptor // Optional encryptor for sensitive data
}

// NewSQLCAdapter creates a new SQLC-based storage adapter
func NewSQLCAdapter(db *sql.DB) *SQLCAdapter {
	return &SQLCAdapter{
		db:        db,
		queries:   sqlite.New(db),
		encryptor: nil,
	}
}

// NewSecureSQLCAdapter creates a new SQLC-based storage adapter with encryption
func NewSecureSQLCAdapter(db *sql.DB, encryptor *crypto.ConfigEncryptor) *SQLCAdapter {
	return &SQLCAdapter{
		db:        db,
		queries:   sqlite.New(db),
		encryptor: encryptor,
	}
}

// User operations

func (s *SQLCAdapter) CreateUser(username, passwordHash string) error {
	ctx := context.Background()
	isDefault := false
	_, err := s.queries.CreateUser(ctx, sqlite.CreateUserParams{
		ID:           cuid.New(),
		Username:     username,
		PasswordHash: passwordHash,
		IsDefault:    &isDefault,
	})
	return err
}

func (s *SQLCAdapter) GetUser(username string) (*storage.User, error) {
	ctx := context.Background()
	user, err := s.queries.GetUserByUsername(ctx, username)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	// Handle nullable fields
	isDefault := false
	if user.IsDefault != nil {
		isDefault = *user.IsDefault
	}

	createdAt := time.Now()
	if user.CreatedAt != nil {
		createdAt = *user.CreatedAt
	}

	return &storage.User{
		ID:           user.ID,
		Username:     user.Username,
		PasswordHash: user.PasswordHash,
		IsDefault:    isDefault,
		CreatedAt:    createdAt,
	}, nil
}

// Settings operations

func (s *SQLCAdapter) SetSetting(key, value string) error {
	ctx := context.Background()
	return s.queries.SetSetting(ctx, sqlite.SetSettingParams{
		Key:   key,
		Value: value,
	})
}

func (s *SQLCAdapter) GetSetting(key string) (string, error) {
	ctx := context.Background()
	setting, err := s.queries.GetSetting(ctx, key)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", nil
		}
		return "", err
	}
	return setting.Value, nil
}

func (s *SQLCAdapter) DeleteSetting(key string) error {
	ctx := context.Background()
	return s.queries.DeleteSetting(ctx, key)
}

// Route operations

func (s *SQLCAdapter) CreateRoute(route *storage.Route) error {
	ctx := context.Background()

	// Convert non-nullable fields to pointers
	exchange := &route.Exchange
	filters := &route.Filters
	headers := &route.Headers
	active := &route.Active
	priority := int64(route.Priority)

	// Convert signature fields
	var signatureConfig, signatureSecret *string
	if route.SignatureConfig != "" {
		signatureConfig = &route.SignatureConfig
	}
	if route.SignatureSecret != "" {
		signatureSecret = &route.SignatureSecret
	}

	params := sqlite.CreateRouteParams{
		ID:                  cuid.New(),
		Name:                route.Name,
		Endpoint:            route.Endpoint,
		Method:              route.Method,
		Queue:               route.Queue,
		Exchange:            exchange,
		RoutingKey:          route.RoutingKey,
		Filters:             filters,
		Headers:             headers,
		Active:              active,
		Priority:            &priority,
		ConditionExpression: &route.ConditionExpression,
		SignatureConfig:     signatureConfig,
		SignatureSecret:     signatureSecret,
		UserID:              route.UserID,
	}

	// Handle nullable fields
	params.PipelineID = route.PipelineID
	params.TriggerID = route.TriggerID
	params.DestinationBrokerID = route.DestinationBrokerID

	result, err := s.queries.CreateRoute(ctx, params)
	if err != nil {
		return err
	}

	route.ID = result.ID
	if result.CreatedAt != nil {
		route.CreatedAt = *result.CreatedAt
	}
	if result.UpdatedAt != nil {
		route.UpdatedAt = *result.UpdatedAt
	}
	return nil
}


// GetRouteByEndpoint finds a route by its endpoint path (interface method - checks any method).
// Returns nil if no matching route is found.
func (s *SQLCAdapter) GetRouteByEndpoint(endpoint string) (*storage.Route, error) {
	// Try common HTTP methods in order of likelihood
	methods := []string{"POST", "GET", "PUT", "DELETE", "PATCH"}

	for _, method := range methods {
		route, err := s.GetRouteByEndpointAndMethod(endpoint, method)
		if err != nil {
			return nil, err
		}
		if route != nil {
			return route, nil
		}
	}

	return nil, nil
}

// GetRouteByEndpointAndMethod finds a route by its endpoint path and HTTP method.
// Returns nil if no matching route is found.
// This method uses an optimized database query instead of loading all routes.
func (s *SQLCAdapter) GetRouteByEndpointAndMethod(endpoint, method string) (*storage.Route, error) {
	ctx := context.Background()

	route, err := s.queries.GetRouteByEndpoint(ctx, sqlite.GetRouteByEndpointParams{
		Endpoint: endpoint,
		Method:   method,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil // Return nil when no route found
		}
		return nil, err
	}

	return s.routeFromDB(route)
}

func (s *SQLCAdapter) ListRoutes() ([]*storage.Route, error) {
	ctx := context.Background()
	routes, err := s.queries.ListRoutes(ctx)
	if err != nil {
		return nil, err
	}

	result := make([]*storage.Route, len(routes))
	for i, route := range routes {
		result[i], err = s.routeFromDB(route)
		if err != nil {
			return nil, err
		}
	}

	return result, nil
}



// User-scoped route methods (new interface implementation)

func (s *SQLCAdapter) GetRoute(id string, userID string) (*storage.Route, error) {
	ctx := context.Background()
	route, err := s.queries.GetRouteByUser(ctx, sqlite.GetRouteByUserParams{
		ID:     id,
		UserID: userID,
	})
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	return s.routeFromDB(route)
}

func (s *SQLCAdapter) GetRoutesByUser(userID string) ([]*storage.Route, error) {
	ctx := context.Background()
	routes, err := s.queries.GetRoutesByUser(ctx, userID)
	if err != nil {
		return nil, err
	}

	result := make([]*storage.Route, len(routes))
	for i, route := range routes {
		result[i], err = s.routeFromDB(route)
		if err != nil {
			return nil, err
		}
	}

	return result, nil
}

func (s *SQLCAdapter) UpdateRoute(route *storage.Route, userID string) error {
	ctx := context.Background()

	// Convert non-nullable fields to pointers
	exchange := &route.Exchange
	filters := &route.Filters
	headers := &route.Headers
	active := &route.Active
	priority := int64(route.Priority)

	// Convert signature fields
	var signatureConfig, signatureSecret *string
	if route.SignatureConfig != "" {
		signatureConfig = &route.SignatureConfig
	}
	if route.SignatureSecret != "" {
		signatureSecret = &route.SignatureSecret
	}

	params := sqlite.UpdateRouteByUserParams{
		ID:                  route.ID,
		UserID:              userID,
		Name:                route.Name,
		Endpoint:            route.Endpoint,
		Method:              route.Method,
		Queue:               route.Queue,
		Exchange:            exchange,
		RoutingKey:          route.RoutingKey,
		Filters:             filters,
		Headers:             headers,
		Active:              active,
		Priority:            &priority,
		ConditionExpression: &route.ConditionExpression,
		SignatureConfig:     signatureConfig,
		SignatureSecret:     signatureSecret,
	}

	// Handle nullable fields
	params.PipelineID = route.PipelineID
	params.TriggerID = route.TriggerID
	params.DestinationBrokerID = route.DestinationBrokerID

	_, err := s.queries.UpdateRouteByUser(ctx, params)
	return err
}

func (s *SQLCAdapter) DeleteRoute(id string, userID string) error {
	ctx := context.Background()
	return s.queries.DeleteRouteByUser(ctx, sqlite.DeleteRouteByUserParams{
		ID:     id,
		UserID: userID,
	})
}

func (s *SQLCAdapter) CheckEndpointExists(endpoint string) (bool, error) {
	ctx := context.Background()
	count, err := s.queries.CheckEndpointExists(ctx, endpoint)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}


// Trigger operations

func (s *SQLCAdapter) CreateTrigger(trigger *storage.Trigger) error {
	ctx := context.Background()

	// Encrypt sensitive fields in config before storing
	encryptedConfig, err := s.encryptSensitiveConfig(trigger.Config)
	if err != nil {
		return err
	}

	configJSON, err := s.MarshalJSON(encryptedConfig)
	if err != nil {
		return err
	}

	params := sqlite.CreateTriggerParams{
		ID:          cuid.New(),
		Name:        trigger.Name,
		Type:        trigger.Type,
		Config:      string(configJSON),
		Status:      &trigger.Status,
		Active:      &trigger.Active,
		DlqEnabled:  &trigger.DLQEnabled,
		DlqRetryMax: func() *int64 { v := int64(trigger.DLQRetryMax); return &v }(),
	}

	// Handle optional DLQ broker ID
	if trigger.DLQBrokerID != nil {
		params.DlqBrokerID = trigger.DLQBrokerID
	}

	result, err := s.queries.CreateTrigger(ctx, params)
	if err != nil {
		return err
	}

	trigger.ID = result.ID
	if result.CreatedAt != nil {
		trigger.CreatedAt = *result.CreatedAt
	}
	if result.UpdatedAt != nil {
		trigger.UpdatedAt = *result.UpdatedAt
	}
	return nil
}

func (s *SQLCAdapter) GetTrigger(id string) (*storage.Trigger, error) {
	ctx := context.Background()
	trigger, err := s.queries.GetTrigger(ctx, id)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	return s.triggerFromDB(trigger)
}

func (s *SQLCAdapter) ListTriggers() ([]*storage.Trigger, error) {
	ctx := context.Background()
	triggers, err := s.queries.ListTriggers(ctx)
	if err != nil {
		return nil, err
	}

	result := make([]*storage.Trigger, len(triggers))
	for i, trigger := range triggers {
		result[i], err = s.triggerFromDB(trigger)
		if err != nil {
			return nil, err
		}
	}

	return result, nil
}

func (s *SQLCAdapter) UpdateTrigger(trigger *storage.Trigger) error {
	ctx := context.Background()

	// Encrypt sensitive fields in config before storing
	encryptedConfig, err := s.encryptSensitiveConfig(trigger.Config)
	if err != nil {
		return err
	}

	configJSON, err := s.MarshalJSON(encryptedConfig)
	if err != nil {
		return err
	}

	params := sqlite.UpdateTriggerParams{
		ID:          trigger.ID,
		Name:        trigger.Name,
		Type:        trigger.Type,
		Config:      string(configJSON),
		Status:      &trigger.Status,
		Active:      &trigger.Active,
		DlqEnabled:  &trigger.DLQEnabled,
		DlqRetryMax: func() *int64 { v := int64(trigger.DLQRetryMax); return &v }(),
	}

	// Handle optional DLQ broker ID
	if trigger.DLQBrokerID != nil {
		params.DlqBrokerID = trigger.DLQBrokerID
	}

	_, err = s.queries.UpdateTrigger(ctx, params)
	return err
}

func (s *SQLCAdapter) DeleteTrigger(id string) error {
	ctx := context.Background()
	return s.queries.DeleteTrigger(ctx, id)
}

// Pipeline operations

func (s *SQLCAdapter) CreatePipeline(pipeline *storage.Pipeline) error {
	ctx := context.Background()

	stagesJSON, err := s.MarshalJSON(pipeline.Stages)
	if err != nil {
		return err
	}

	description := &pipeline.Description
	if pipeline.Description == "" {
		description = nil
	}

	params := sqlite.CreatePipelineParams{
		ID:          cuid.New(),
		Name:        pipeline.Name,
		Description: description,
		Stages:      string(stagesJSON),
		Active:      &pipeline.Active,
	}

	result, err := s.queries.CreatePipeline(ctx, params)
	if err != nil {
		return err
	}

	pipeline.ID = result.ID
	if result.CreatedAt != nil {
		pipeline.CreatedAt = *result.CreatedAt
	}
	if result.UpdatedAt != nil {
		pipeline.UpdatedAt = *result.UpdatedAt
	}
	return nil
}

func (s *SQLCAdapter) GetPipeline(id string) (*storage.Pipeline, error) {
	ctx := context.Background()
	pipeline, err := s.queries.GetPipeline(ctx, id)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	return s.pipelineFromDB(pipeline)
}

func (s *SQLCAdapter) ListPipelines() ([]*storage.Pipeline, error) {
	ctx := context.Background()
	pipelines, err := s.queries.ListPipelines(ctx)
	if err != nil {
		return nil, err
	}

	result := make([]*storage.Pipeline, len(pipelines))
	for i, pipeline := range pipelines {
		result[i], err = s.pipelineFromDB(pipeline)
		if err != nil {
			return nil, err
		}
	}

	return result, nil
}

func (s *SQLCAdapter) UpdatePipeline(pipeline *storage.Pipeline) error {
	ctx := context.Background()

	stagesJSON, err := s.MarshalJSON(pipeline.Stages)
	if err != nil {
		return err
	}

	description := &pipeline.Description
	if pipeline.Description == "" {
		description = nil
	}

	params := sqlite.UpdatePipelineParams{
		ID:          pipeline.ID,
		Name:        pipeline.Name,
		Description: description,
		Stages:      string(stagesJSON),
		Active:      &pipeline.Active,
	}

	_, err = s.queries.UpdatePipeline(ctx, params)
	return err
}

func (s *SQLCAdapter) DeletePipeline(id string) error {
	ctx := context.Background()
	return s.queries.DeletePipeline(ctx, id)
}

// BrokerConfig operations

func (s *SQLCAdapter) CreateBroker(config *storage.BrokerConfig) error {
	ctx := context.Background()

	// Encrypt sensitive fields in config before storing
	encryptedConfig, err := s.encryptSensitiveConfig(config.Config)
	if err != nil {
		return err
	}

	configJSON, err := s.MarshalJSON(encryptedConfig)
	if err != nil {
		return err
	}

	params := sqlite.CreateBrokerConfigParams{
		ID:           cuid.New(),
		Name:         config.Name,
		Type:         config.Type,
		Config:       string(configJSON),
		Active:       &config.Active,
		HealthStatus: &config.HealthStatus,
		DlqEnabled:   config.DlqEnabled,
		DlqBrokerID:  config.DlqBrokerID,
	}

	result, err := s.queries.CreateBrokerConfig(ctx, params)
	if err != nil {
		return errors.InternalError("CreateBrokerConfig failed", err)
	}

	config.ID = result.ID
	if result.CreatedAt != nil {
		config.CreatedAt = *result.CreatedAt
	}
	if result.UpdatedAt != nil {
		config.UpdatedAt = *result.UpdatedAt
	}
	return nil
}

func (s *SQLCAdapter) GetBroker(id string) (*storage.BrokerConfig, error) {
	ctx := context.Background()
	config, err := s.queries.GetBrokerConfig(ctx, id)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	return s.brokerConfigFromDB(config)
}

func (s *SQLCAdapter) GetBrokers() ([]*storage.BrokerConfig, error) {
	ctx := context.Background()
	configs, err := s.queries.ListBrokerConfigs(ctx)
	if err != nil {
		return nil, err
	}

	result := make([]*storage.BrokerConfig, len(configs))
	for i, config := range configs {
		result[i], err = s.brokerConfigFromDB(config)
		if err != nil {
			return nil, err
		}
	}

	return result, nil
}

func (s *SQLCAdapter) UpdateBroker(config *storage.BrokerConfig) error {
	ctx := context.Background()

	// Encrypt sensitive fields in config before storing
	encryptedConfig, err := s.encryptSensitiveConfig(config.Config)
	if err != nil {
		return err
	}

	configJSON, err := s.MarshalJSON(encryptedConfig)
	if err != nil {
		return err
	}

	params := sqlite.UpdateBrokerConfigParams{
		ID:     config.ID,
		Name:   config.Name,
		Type:   config.Type,
		Config: string(configJSON),
		Active: &config.Active,
	}

	_, err = s.queries.UpdateBrokerConfig(ctx, params)
	return err
}

func (s *SQLCAdapter) DeleteBroker(id string) error {
	ctx := context.Background()
	return s.queries.DeleteBrokerConfig(ctx, id)
}

// WebhookLog operations

func (s *SQLCAdapter) LogWebhook(log *storage.WebhookLog) error {
	ctx := context.Background()

	// Convert non-nullable fields to pointers
	var routeID *string
	if log.RouteID != "" {
		routeID = &log.RouteID
	}

	headers := &log.Headers
	if log.Headers == "" {
		headers = nil
	}

	body := &log.Body
	if log.Body == "" {
		body = nil
	}

	statusCode := int64(log.StatusCode)
	errorStr := &log.Error
	if log.Error == "" {
		errorStr = nil
	}

	transformTime := int64(log.TransformationTimeMS)
	brokerTime := int64(log.BrokerPublishTimeMS)

	params := sqlite.CreateWebhookLogParams{
		ID:                   cuid.New(),
		RouteID:              routeID,
		Method:               log.Method,
		Endpoint:             log.Endpoint,
		Headers:              headers,
		Body:                 body,
		StatusCode:           &statusCode,
		Error:                errorStr,
		TransformationTimeMs: &transformTime,
		BrokerPublishTimeMs:  &brokerTime,
	}

	// Handle nullable fields
	if log.TriggerID != nil {
		params.TriggerID = log.TriggerID
	}
	if log.PipelineID != nil {
		params.PipelineID = log.PipelineID
	}

	_, err := s.queries.CreateWebhookLog(ctx, params)
	return err
}

func (s *SQLCAdapter) GetWebhookLogs(limit, offset int) ([]*storage.WebhookLog, error) {
	ctx := context.Background()
	logs, err := s.queries.ListWebhookLogs(ctx, sqlite.ListWebhookLogsParams{
		Limit:  int64(limit),
		Offset: int64(offset),
	})
	if err != nil {
		return nil, err
	}

	result := make([]*storage.WebhookLog, len(logs))
	for i, log := range logs {
		result[i], err = s.webhookLogFromDB(log)
		if err != nil {
			return nil, err
		}
	}

	return result, nil
}

// storage.Transaction support

func (s *SQLCAdapter) BeginTx() (storage.Transaction, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	return &SQLCTransaction{
		tx:      tx,
		queries: s.queries.WithTx(tx),
	}, nil
}

// Helper methods

func (s *SQLCAdapter) routeFromDB(route sqlite.Route) (*storage.Route, error) {
	// Convert nullable fields with defaults
	exchange := s.ConvertNullableString(route.Exchange)
	filters := s.ConvertNullableString(route.Filters)
	headers := s.ConvertNullableString(route.Headers)
	active := s.ConvertNullableBool(route.Active)
	priority := s.ConvertNullableInt64(route.Priority)
	conditionExpression := s.ConvertNullableString(route.ConditionExpression)
	createdAt := s.ConvertNullableTime(route.CreatedAt)
	updatedAt := s.ConvertNullableTime(route.UpdatedAt)

	// Convert signature fields
	signatureConfig := s.ConvertNullableString(route.SignatureConfig)
	signatureSecret := s.ConvertNullableString(route.SignatureSecret)

	r := &storage.Route{
		ID:                  route.ID,
		Name:                route.Name,
		Endpoint:            route.Endpoint,
		Method:              route.Method,
		Queue:               route.Queue,
		Exchange:            exchange,
		RoutingKey:          route.RoutingKey,
		Filters:             filters,
		Headers:             headers,
		Active:              active,
		Priority:            priority,
		ConditionExpression: conditionExpression,
		SignatureConfig:     signatureConfig,
		SignatureSecret:     signatureSecret,
		CreatedAt:           createdAt,
		UpdatedAt:           updatedAt,
	}

	// Handle nullable fields
	if route.PipelineID != nil {
		r.PipelineID = route.PipelineID
	}
	if route.TriggerID != nil {
		r.TriggerID = route.TriggerID
	}
	if route.DestinationBrokerID != nil {
		r.DestinationBrokerID = route.DestinationBrokerID
	}

	return r, nil
}

func (s *SQLCAdapter) triggerFromDB(trigger sqlite.Trigger) (*storage.Trigger, error) {
	var config map[string]interface{}
	if err := s.UnmarshalJSON(trigger.Config, &config); err != nil {
		return nil, err
	}

	// Decrypt sensitive fields in config after loading
	decryptedConfig, err := s.decryptSensitiveConfig(config)
	if err != nil {
		return nil, err
	}

	// Handle nullable fields with defaults
	status := s.ConvertNullableString(trigger.Status)
	active := s.ConvertNullableBool(trigger.Active)
	errorMessage := s.ConvertNullableString(trigger.ErrorMessage)
	createdAt := s.ConvertNullableTime(trigger.CreatedAt)
	updatedAt := s.ConvertNullableTime(trigger.UpdatedAt)

	t := &storage.Trigger{
		ID:           trigger.ID,
		Name:         trigger.Name,
		Type:         trigger.Type,
		Config:       decryptedConfig,
		Status:       status,
		Active:       active,
		ErrorMessage: errorMessage,
		CreatedAt:    createdAt,
		UpdatedAt:    updatedAt,
		DLQEnabled:   s.ConvertNullableBool(trigger.DlqEnabled),
		DLQRetryMax: func() int {
			if trigger.DlqRetryMax != nil {
				return int(*trigger.DlqRetryMax)
			}
			return 3
		}(),
	}

	// Handle nullable time fields
	if trigger.LastExecution != nil {
		t.LastExecution = trigger.LastExecution
	}
	if trigger.NextExecution != nil {
		t.NextExecution = trigger.NextExecution
	}

	// Handle DLQ broker ID
	if trigger.DlqBrokerID != nil {
		t.DLQBrokerID = trigger.DlqBrokerID
	}

	return t, nil
}

func (s *SQLCAdapter) pipelineFromDB(pipeline sqlite.Pipeline) (*storage.Pipeline, error) {
	var stages []map[string]interface{}
	if err := s.UnmarshalJSON(pipeline.Stages, &stages); err != nil {
		return nil, err
	}

	// Handle nullable fields with defaults
	description := s.ConvertNullableString(pipeline.Description)
	active := s.ConvertNullableBool(pipeline.Active)
	createdAt := s.ConvertNullableTime(pipeline.CreatedAt)
	updatedAt := s.ConvertNullableTime(pipeline.UpdatedAt)

	return &storage.Pipeline{
		ID:          pipeline.ID,
		Name:        pipeline.Name,
		Description: description,
		Stages:      stages,
		Active:      active,
		CreatedAt:   createdAt,
		UpdatedAt:   updatedAt,
	}, nil
}

func (s *SQLCAdapter) brokerConfigFromDB(config sqlite.BrokerConfig) (*storage.BrokerConfig, error) {
	var configData map[string]interface{}
	if err := s.UnmarshalJSON(config.Config, &configData); err != nil {
		return nil, err
	}

	// Decrypt sensitive fields in config after loading
	decryptedConfig, err := s.decryptSensitiveConfig(configData)
	if err != nil {
		return nil, err
	}

	// Handle nullable fields with defaults
	active := s.ConvertNullableBool(config.Active)
	healthStatus := s.ConvertNullableString(config.HealthStatus)
	createdAt := s.ConvertNullableTime(config.CreatedAt)
	updatedAt := s.ConvertNullableTime(config.UpdatedAt)

	bc := &storage.BrokerConfig{
		ID:           config.ID,
		Name:         config.Name,
		Type:         config.Type,
		Config:       decryptedConfig,
		Active:       active,
		HealthStatus: healthStatus,
		CreatedAt:    createdAt,
		UpdatedAt:    updatedAt,
	}

	// Handle nullable time field
	if config.LastHealthCheck != nil {
		bc.LastHealthCheck = config.LastHealthCheck
	}

	return bc, nil
}

func (s *SQLCAdapter) webhookLogFromDB(log sqlite.WebhookLog) (*storage.WebhookLog, error) {
	// Handle nullable fields with defaults
	routeID := ""
	if log.RouteID != nil {
		routeID = *log.RouteID
	}

	headers := ""
	if log.Headers != nil {
		headers = *log.Headers
	}

	body := ""
	if log.Body != nil {
		body = *log.Body
	}

	statusCode := 0
	if log.StatusCode != nil {
		statusCode = int(*log.StatusCode)
	}

	errorStr := ""
	if log.Error != nil {
		errorStr = *log.Error
	}

	processedAt := time.Now()
	if log.ProcessedAt != nil {
		processedAt = *log.ProcessedAt
	}

	transformTime := 0
	if log.TransformationTimeMs != nil {
		transformTime = int(*log.TransformationTimeMs)
	}

	brokerTime := 0
	if log.BrokerPublishTimeMs != nil {
		brokerTime = int(*log.BrokerPublishTimeMs)
	}

	wl := &storage.WebhookLog{
		ID:                   log.ID,
		RouteID:              routeID,
		Method:               log.Method,
		Endpoint:             log.Endpoint,
		Headers:              headers,
		Body:                 body,
		StatusCode:           statusCode,
		Error:                errorStr,
		ProcessedAt:          processedAt,
		TransformationTimeMS: transformTime,
		BrokerPublishTimeMS:  brokerTime,
	}

	// Handle nullable fields
	if log.TriggerID != nil {
		wl.TriggerID = log.TriggerID
	}
	if log.PipelineID != nil {
		wl.PipelineID = log.PipelineID
	}

	return wl, nil
}

// SQLCstorage.Transaction implements storage.Transaction interface
type SQLCTransaction struct {
	tx      *sql.Tx
	queries *sqlite.Queries
}

func (t *SQLCTransaction) Commit() error {
	return t.tx.Commit()
}

func (t *SQLCTransaction) Rollback() error {
	return t.tx.Rollback()
}

// Other required methods

func (s *SQLCAdapter) Close() error {
	return s.db.Close()
}

func (s *SQLCAdapter) Ping() error {
	return s.db.Ping()
}

// GetStatistics is not part of the Storage interface and is not used anywhere.
// The actual statistics methods are GetStats() and GetRouteStats() which are already implemented.

// Connect implements Storage interface
func (s *SQLCAdapter) Connect(config storage.StorageConfig) error {
	// Connection is already established through db parameter in constructor
	return nil
}

// Health implements Storage interface
func (s *SQLCAdapter) Health() error {
	return s.db.Ping()
}

// GetRoutes implements Storage interface
func (s *SQLCAdapter) GetRoutes() ([]*storage.Route, error) {
	return s.ListRoutes()
}

// FindMatchingRoutes implements Storage interface
func (s *SQLCAdapter) FindMatchingRoutes(endpoint, method string) ([]*storage.Route, error) {
	ctx := context.Background()
	routes, err := s.queries.ListRoutes(ctx)
	if err != nil {
		return nil, err
	}

	var matching []*storage.Route
	for _, route := range routes {
		if route.Endpoint == endpoint && route.Method == method {
			r, err := s.routeFromDB(route)
			if err != nil {
				return nil, err
			}
			matching = append(matching, r)
		}
	}

	return matching, nil
}

// ValidateUser implements Storage interface
func (s *SQLCAdapter) ValidateUser(username, password string) (*storage.User, error) {
	user, err := s.GetUser(username)
	if err != nil || user == nil {
		return nil, errors.AuthError("invalid credentials")
	}

	// Validate password using bcrypt
	err = bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password))
	if err != nil {
		return nil, errors.AuthError("invalid credentials")
	}

	return user, nil
}

// UpdateUserCredentials implements Storage interface
func (s *SQLCAdapter) UpdateUserCredentials(userID string, username, password string) error {
	ctx := context.Background()

	// Hash the new password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return errors.InternalError("failed to hash password", err)
	}

	return s.queries.UpdateUserCredentials(ctx, sqlite.UpdateUserCredentialsParams{
		ID:           userID,
		Username:     username,
		PasswordHash: string(hashedPassword),
	})
}

// IsDefaultUser implements Storage interface
func (s *SQLCAdapter) IsDefaultUser(userID string) (bool, error) {
	ctx := context.Background()
	user, err := s.queries.GetUser(ctx, userID)
	if err != nil {
		return false, err
	}

	if user.IsDefault != nil {
		return *user.IsDefault, nil
	}
	return false, nil
}

// GetSetting implements Storage interface (already implemented above)

// GetAllSettings implements Storage interface
func (s *SQLCAdapter) GetAllSettings() (map[string]string, error) {
	ctx := context.Background()
	settings, err := s.queries.ListSettings(ctx)
	if err != nil {
		return nil, err
	}

	result := make(map[string]string)
	for _, setting := range settings {
		result[setting.Key] = setting.Value
	}

	return result, nil
}

// GetStats implements Storage interface
func (s *SQLCAdapter) GetStats() (*storage.Stats, error) {
	ctx := context.Background()

	// Get counts from database
	routes, err := s.queries.ListRoutes(ctx)
	if err != nil {
		return nil, err
	}

	activeRoutes := 0
	for _, route := range routes {
		if route.Active != nil && *route.Active {
			activeRoutes++
		}
	}

	// Get webhook log statistics for the last 24 hours
	since := time.Now().Add(-24 * time.Hour)

	stats, err := s.queries.GetWebhookLogStats(ctx, &since)
	if err != nil {
		return nil, errors.InternalError("failed to get webhook log stats", err)
	}

	return &storage.Stats{
		TotalRequests:   int(stats.TotalCount),
		SuccessRequests: int(stats.SuccessCount),
		FailedRequests:  int(stats.ErrorCount),
		ActiveRoutes:    activeRoutes,
	}, nil
}

// GetRouteStats implements Storage interface
func (s *SQLCAdapter) GetRouteStats(routeID string) (map[string]interface{}, error) {
	ctx := context.Background()

	stats, err := s.queries.GetRouteStatistics(ctx, routeID)
	if err != nil {
		return nil, errors.InternalError("failed to get route statistics", err)
	}

	// Convert to sql.Null types for BuildStatsResult
	var avgTransformTime sql.NullFloat64
	if stats.AvgTransformationTime != nil {
		avgTransformTime = sql.NullFloat64{Float64: *stats.AvgTransformationTime, Valid: true}
	}

	var avgPublishTime sql.NullFloat64
	if stats.AvgPublishTime != nil {
		avgPublishTime = sql.NullFloat64{Float64: *stats.AvgPublishTime, Valid: true}
	}

	var lastProcessed sql.NullTime
	if stats.LastProcessed != nil {
		if t, ok := stats.LastProcessed.(time.Time); ok {
			lastProcessed = sql.NullTime{Time: t, Valid: true}
		}
	}

	return s.BuildStatsResult(
		routeID,
		stats.Name,
		stats.TotalRequests,
		stats.SuccessfulRequests,
		stats.FailedRequests,
		avgTransformTime,
		avgPublishTime,
		lastProcessed,
	), nil
}

// GetTriggers implements Storage interface
func (s *SQLCAdapter) GetTriggers(filters storage.TriggerFilters) ([]*storage.Trigger, error) {
	// For now, return all triggers and filter in memory
	triggers, err := s.ListTriggers()
	if err != nil {
		return nil, err
	}

	return s.ApplyTriggerFilters(triggers, filters), nil
}

// GetPipelines implements Storage interface
func (s *SQLCAdapter) GetPipelines() ([]*storage.Pipeline, error) {
	return s.ListPipelines()
}


// Query executes a raw SQL query and returns results as a slice of maps.
// Each map represents a row with column names as keys and values as interface{}.
// This method should be used sparingly; prefer type-safe SQLC-generated methods.
func (s *SQLCAdapter) Query(query string, args ...interface{}) ([]map[string]interface{}, error) {
	return s.ExecuteQuery(s.db, query, args...)
}

// Transaction executes a function within a database transaction.
// If the function returns an error, the transaction is rolled back.
// Otherwise, the transaction is committed.
// This ensures atomic operations across multiple database calls.
func (s *SQLCAdapter) Transaction(fn func(storage.Transaction) error) error {
	tx, err := s.BeginTx()
	if err != nil {
		return err
	}

	if err := fn(tx); err != nil {
		tx.Rollback()
		return err
	}

	return tx.Commit()
}

// DLQ Implementation Methods

// CreateDLQMessage implements Storage interface
func (s *SQLCAdapter) CreateDLQMessage(message *storage.DLQMessage) error {
	ctx := context.Background()

	headers, err := json.Marshal(message.Headers)
	if err != nil {
		return errors.InternalError("failed to marshal headers", err)
	}

	metadata, err := json.Marshal(message.Metadata)
	if err != nil {
		return errors.InternalError("failed to marshal metadata", err)
	}

	// Extract broker IDs from metadata if available
	sourceBrokerID := "1" // Default
	dlqBrokerID := "1"    // Default

	if message.Metadata != nil {
		if srcID, ok := message.Metadata["source_broker_id"].(string); ok {
			sourceBrokerID = srcID
		}
		if dlqID, ok := message.Metadata["dlq_broker_id"].(string); ok {
			dlqBrokerID = dlqID
		}
	}

	params := sqlite.CreateDLQMessageParams{
		ID:             cuid.New(),
		MessageID:      message.MessageID,
		RouteID:        message.RouteID,
		TriggerID:      message.TriggerID,
		PipelineID:     message.PipelineID,
		SourceBrokerID: sourceBrokerID,
		DlqBrokerID:    dlqBrokerID,
		BrokerName:     message.BrokerName,
		Queue:          message.Queue,
		Exchange:       s.ConvertStringToNullable(message.Exchange),
		RoutingKey:     message.RoutingKey,
		Headers:        s.ConvertStringToNullable(string(headers)),
		Body:           message.Body,
		ErrorMessage:   message.ErrorMessage,
		FailureCount:   s.ConvertIntToNullableInt64(message.FailureCount),
		FirstFailure:   message.FirstFailure,
		LastFailure:    message.LastFailure,
		NextRetry:      s.ConvertNullableTimePtrToTimePtr(message.NextRetry),
		Status:         s.ConvertStringToNullable(message.Status),
		Metadata:       s.ConvertStringToNullable(string(metadata)),
	}

	result, err := s.queries.CreateDLQMessage(ctx, params)
	if err != nil {
		return errors.InternalError("failed to create DLQ message", err)
	}

	message.ID = result.ID
	return nil
}

// GetDLQMessage implements Storage interface
func (s *SQLCAdapter) GetDLQMessage(id string) (*storage.DLQMessage, error) {
	ctx := context.Background()
	msg, err := s.queries.GetDLQMessage(ctx, id)
	if err != nil {
		return nil, s.HandleNotFound(err)
	}

	return s.dlqMessageFromDB(msg)
}

// GetDLQMessageByMessageID implements Storage interface
func (s *SQLCAdapter) GetDLQMessageByMessageID(messageID string) (*storage.DLQMessage, error) {
	ctx := context.Background()
	msg, err := s.queries.GetDLQMessageByMessageID(ctx, messageID)
	if err != nil {
		return nil, s.HandleNotFound(err)
	}

	return s.dlqMessageFromDB(msg)
}

// ListPendingDLQMessages implements Storage interface
func (s *SQLCAdapter) ListPendingDLQMessages(limit int) ([]*storage.DLQMessage, error) {
	ctx := context.Background()
	msgs, err := s.queries.ListPendingDLQMessages(ctx, int64(limit))
	if err != nil {
		return nil, err
	}

	var result []*storage.DLQMessage
	for _, msg := range msgs {
		dlqMsg, err := s.dlqMessageFromDB(msg)
		if err != nil {
			return nil, err
		}
		result = append(result, dlqMsg)
	}

	return result, nil
}

// ListDLQMessages implements Storage interface
func (s *SQLCAdapter) ListDLQMessages(limit, offset int) ([]*storage.DLQMessage, error) {
	ctx := context.Background()
	msgs, err := s.queries.ListDLQMessages(ctx, sqlite.ListDLQMessagesParams{
		Limit:  int64(limit),
		Offset: int64(offset),
	})
	if err != nil {
		return nil, err
	}

	var result []*storage.DLQMessage
	for _, msg := range msgs {
		dlqMsg, err := s.dlqMessageFromDB(msg)
		if err != nil {
			return nil, err
		}
		result = append(result, dlqMsg)
	}

	return result, nil
}

// ListDLQMessagesByRoute implements Storage interface
func (s *SQLCAdapter) ListDLQMessagesByRoute(routeID string, limit, offset int) ([]*storage.DLQMessage, error) {
	ctx := context.Background()
	msgs, err := s.queries.ListDLQMessagesByRoute(ctx, sqlite.ListDLQMessagesByRouteParams{
		RouteID: routeID,
		Limit:   int64(limit),
		Offset:  int64(offset),
	})
	if err != nil {
		return nil, err
	}

	var result []*storage.DLQMessage
	for _, msg := range msgs {
		dlqMsg, err := s.dlqMessageFromDB(msg)
		if err != nil {
			return nil, err
		}
		result = append(result, dlqMsg)
	}

	return result, nil
}

// ListDLQMessagesByStatus implements Storage interface
func (s *SQLCAdapter) ListDLQMessagesByStatus(status string, limit, offset int) ([]*storage.DLQMessage, error) {
	ctx := context.Background()
	msgs, err := s.queries.ListDLQMessagesByStatus(ctx, sqlite.ListDLQMessagesByStatusParams{
		Status: &status,
		Limit:  int64(limit),
		Offset: int64(offset),
	})
	if err != nil {
		return nil, err
	}

	var result []*storage.DLQMessage
	for _, msg := range msgs {
		dlqMsg, err := s.dlqMessageFromDB(msg)
		if err != nil {
			return nil, err
		}
		result = append(result, dlqMsg)
	}

	return result, nil
}

// UpdateDLQMessage implements Storage interface
func (s *SQLCAdapter) UpdateDLQMessage(message *storage.DLQMessage) error {
	ctx := context.Background()

	metadata, err := json.Marshal(message.Metadata)
	if err != nil {
		return errors.InternalError("failed to marshal metadata", err)
	}

	params := sqlite.UpdateDLQMessageParams{
		FailureCount: s.ConvertIntToNullableInt64(message.FailureCount),
		LastFailure:  message.LastFailure,
		NextRetry:    message.NextRetry,
		ErrorMessage: message.ErrorMessage,
		Status:       s.ConvertStringToNullable(message.Status),
		Metadata:     s.ConvertStringToNullable(string(metadata)),
		ID:           message.ID,
	}

	_, err = s.queries.UpdateDLQMessage(ctx, params)
	return err
}

// UpdateDLQMessageStatus implements Storage interface
func (s *SQLCAdapter) UpdateDLQMessageStatus(id string, status string) error {
	ctx := context.Background()
	return s.queries.UpdateDLQMessageStatus(ctx, sqlite.UpdateDLQMessageStatusParams{
		Status: &status,
		ID:     id,
	})
}

// DeleteDLQMessage implements Storage interface
func (s *SQLCAdapter) DeleteDLQMessage(id string) error {
	ctx := context.Background()
	return s.queries.DeleteDLQMessage(ctx, id)
}

// DeleteOldDLQMessages implements Storage interface
func (s *SQLCAdapter) DeleteOldDLQMessages(before time.Time) error {
	ctx := context.Background()
	return s.queries.DeleteOldDLQMessages(ctx, before)
}

// GetDLQStats implements Storage interface
func (s *SQLCAdapter) GetDLQStats() (*storage.DLQStats, error) {
	ctx := context.Background()

	// Count messages by status
	allMessages, err := s.queries.ListDLQMessages(ctx, sqlite.ListDLQMessagesParams{
		Limit:  1000,
		Offset: 0,
	})
	if err != nil {
		return nil, err
	}

	stats := &storage.DLQStats{}
	var oldestFailure *time.Time

	for _, msg := range allMessages {
		stats.TotalMessages++

		if msg.Status != nil {
			switch *msg.Status {
			case "pending":
				stats.PendingMessages++
			case "retrying":
				stats.RetryingMessages++
			case "abandoned":
				stats.AbandonedMessages++
			}
		}

		if oldestFailure == nil || msg.FirstFailure.Before(*oldestFailure) {
			oldestFailure = &msg.FirstFailure
		}
	}

	stats.OldestFailure = oldestFailure
	return stats, nil
}

// GetDLQStatsByRoute implements Storage interface
func (s *SQLCAdapter) GetDLQStatsByRoute() ([]*storage.DLQRouteStats, error) {
	ctx := context.Background()

	// Since we don't have a direct query for stats by route, we'll calculate it manually
	allMessages, err := s.queries.ListDLQMessages(ctx, sqlite.ListDLQMessagesParams{
		Limit:  10000, // Get all messages
		Offset: 0,
	})
	if err != nil {
		return nil, err
	}

	// Group by route ID
	routeStats := make(map[string]*storage.DLQRouteStats)
	for _, msg := range allMessages {
		routeID := msg.RouteID
		if _, exists := routeStats[routeID]; !exists {
			routeStats[routeID] = &storage.DLQRouteStats{
				RouteID: routeID,
			}
		}

		routeStats[routeID].MessageCount++

		if msg.Status != nil {
			switch *msg.Status {
			case "pending":
				routeStats[routeID].PendingCount++
			case "abandoned":
				routeStats[routeID].AbandonedCount++
			}
		}
	}

	// Convert map to slice
	var result []*storage.DLQRouteStats
	for _, stat := range routeStats {
		result = append(result, stat)
	}

	return result, nil
}

// GetDLQStatsByError implements Storage interface
func (s *SQLCAdapter) GetDLQStatsByError() ([]*storage.DLQErrorStats, error) {
	ctx := context.Background()

	// Since we don't have a direct query for stats by error, we'll calculate it manually
	allMessages, err := s.queries.ListDLQMessages(ctx, sqlite.ListDLQMessagesParams{
		Limit:  10000, // Get all messages
		Offset: 0,
	})
	if err != nil {
		return nil, err
	}

	// Group by error message
	errorStats := make(map[string]*storage.DLQErrorStats)
	for _, msg := range allMessages {
		if msg.ErrorMessage == "" {
			continue
		}

		errorMsg := msg.ErrorMessage
		if _, exists := errorStats[errorMsg]; !exists {
			errorStats[errorMsg] = &storage.DLQErrorStats{
				ErrorMessage: errorMsg,
				MessageCount: 0,
				FirstSeen:    msg.FirstFailure,
				LastSeen:     msg.LastFailure,
			}
		}

		errorStats[errorMsg].MessageCount++

		// Update first seen if earlier
		if msg.FirstFailure.Before(errorStats[errorMsg].FirstSeen) {
			errorStats[errorMsg].FirstSeen = msg.FirstFailure
		}

		// Update last seen if later
		if msg.LastFailure.After(errorStats[errorMsg].LastSeen) {
			errorStats[errorMsg].LastSeen = msg.LastFailure
		}
	}

	// Convert map to slice
	var result []*storage.DLQErrorStats
	for _, stat := range errorStats {
		result = append(result, stat)
	}

	// Sort by message count (descending)
	for i := 0; i < len(result)-1; i++ {
		for j := i + 1; j < len(result); j++ {
			if result[i].MessageCount < result[j].MessageCount {
				result[i], result[j] = result[j], result[i]
			}
		}
	}

	return result, nil
}

// ConvertNullableTimePtrToTimePtr converts a *time.Time to nullable *time.Time for SQLC
func (s *SQLCAdapter) ConvertNullableTimePtrToTimePtr(val *time.Time) *time.Time {
	return val
}

// dlqMessageFromDB converts database DLQ message to storage model
func (s *SQLCAdapter) dlqMessageFromDB(msg sqlite.DlqMessage) (*storage.DLQMessage, error) {
	var headers map[string]string
	if msg.Headers != nil && *msg.Headers != "" {
		if err := json.Unmarshal([]byte(*msg.Headers), &headers); err != nil {
			return nil, errors.InternalError("failed to unmarshal headers", err)
		}
	} else {
		headers = make(map[string]string)
	}

	var metadata map[string]interface{}
	if msg.Metadata != nil && *msg.Metadata != "" {
		if err := json.Unmarshal([]byte(*msg.Metadata), &metadata); err != nil {
			return nil, errors.InternalError("failed to unmarshal metadata", err)
		}
	} else {
		metadata = make(map[string]interface{})
	}

	result := &storage.DLQMessage{
		ID:           msg.ID,
		MessageID:    msg.MessageID,
		RouteID:      msg.RouteID,
		BrokerName:   msg.BrokerName,
		Queue:        msg.Queue,
		Exchange:     s.ConvertNullableString(msg.Exchange),
		RoutingKey:   msg.RoutingKey,
		Headers:      headers,
		Body:         msg.Body,
		ErrorMessage: msg.ErrorMessage,
		FailureCount: s.ConvertNullableInt64(msg.FailureCount),
		FirstFailure: msg.FirstFailure,
		LastFailure:  msg.LastFailure,
		Status:       s.ConvertNullableString(msg.Status),
		Metadata:     metadata,
		CreatedAt:    s.ConvertNullableTime(msg.CreatedAt),
		UpdatedAt:    s.ConvertNullableTime(msg.UpdatedAt),
	}

	// Convert nullable IDs
	if msg.TriggerID != nil {
		result.TriggerID = msg.TriggerID
	}
	if msg.PipelineID != nil {
		result.PipelineID = msg.PipelineID
	}
	if msg.NextRetry != nil {
		result.NextRetry = msg.NextRetry
	}

	return result, nil
}

// ConvertNullableInt64Ptr converts nullable int64 to *int
func (s *SQLCAdapter) ConvertNullableInt64Ptr(val *int64) *int {
	if val != nil {
		i := int(*val)
		return &i
	}
	return nil
}

// encryptSensitiveConfig encrypts sensitive fields in a configuration map
// Returns the encrypted config and any error encountered
func (s *SQLCAdapter) encryptSensitiveConfig(config map[string]interface{}) (map[string]interface{}, error) {
	if s.encryptor == nil {
		return config, nil // No encryption configured
	}

	// Create a copy to avoid modifying the original
	encrypted := make(map[string]interface{})
	for k, v := range config {
		encrypted[k] = v
	}

	// Define sensitive field patterns
	sensitivePatterns := []string{
		"password", "secret", "token", "api_key", "apikey", "access_key",
		"secret_access_key", "client_secret", "private_key", "encryption_key",
		"jwt_secret", "signature_secret", "oauth2_token", "refresh_token",
		"access_token", "auth_token", "bearer_token",
	}

	// Encrypt sensitive fields
	for key, value := range encrypted {
		if s.isSensitiveField(key, sensitivePatterns) {
			if strVal, ok := value.(string); ok && strVal != "" {
				encryptedValue, err := s.encryptor.Encrypt(strVal)
				if err != nil {
					return nil, err
				}
				encrypted[key] = encryptedValue
			}
		}
	}

	return encrypted, nil
}

// decryptSensitiveConfig decrypts sensitive fields in a configuration map
// Returns the decrypted config and any error encountered
func (s *SQLCAdapter) decryptSensitiveConfig(config map[string]interface{}) (map[string]interface{}, error) {
	if s.encryptor == nil {
		return config, nil // No encryption configured
	}

	// Create a copy to avoid modifying the original
	decrypted := make(map[string]interface{})
	for k, v := range config {
		decrypted[k] = v
	}

	// Define sensitive field patterns
	sensitivePatterns := []string{
		"password", "secret", "token", "api_key", "apikey", "access_key",
		"secret_access_key", "client_secret", "private_key", "encryption_key",
		"jwt_secret", "signature_secret", "oauth2_token", "refresh_token",
		"access_token", "auth_token", "bearer_token",
	}

	// Decrypt sensitive fields
	for key, value := range decrypted {
		if s.isSensitiveField(key, sensitivePatterns) {
			if strVal, ok := value.(string); ok && strVal != "" {
				decryptedValue, err := s.encryptor.Decrypt(strVal)
				if err != nil {
					// If decryption fails, it might be plaintext data from before encryption was enabled
					// Log warning but don't fail - return original value for backward compatibility
					continue
				}
				decrypted[key] = decryptedValue
			}
		}
	}

	return decrypted, nil
}

// isSensitiveField checks if a field name matches sensitive field patterns
func (s *SQLCAdapter) isSensitiveField(fieldName string, patterns []string) bool {
	fieldLower := strings.ToLower(fieldName)
	for _, pattern := range patterns {
		if strings.Contains(fieldLower, pattern) {
			return true
		}
	}
	return false
}
