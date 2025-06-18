// Package sqlc provides type-safe database adapters using SQLC code generation.
package sqlc

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"webhook-router/internal/storage"
	postgres "webhook-router/internal/storage/generated/postgres"
)

// PostgreSQLCAdapter implements the Storage interface using SQLC-generated code for PostgreSQL.
// It provides type-safe database operations with automatic NULL handling for PostgreSQL-specific types.
// This adapter handles all storage operations including routes, users, settings, triggers,
// pipelines, brokers, and webhook logs.
type PostgreSQLCAdapter struct {
	BaseAdapter
	conn    *pgx.Conn
	queries *postgres.Queries
}

// NewPostgreSQLCAdapter creates a new PostgreSQL storage adapter using SQLC-generated queries.
// The conn parameter should be an open PostgreSQL database connection.
func NewPostgreSQLCAdapter(conn *pgx.Conn) *PostgreSQLCAdapter {
	return &PostgreSQLCAdapter{
		conn:    conn,
		queries: postgres.New(conn),
	}
}

// Helper methods for PostgreSQL-specific type conversions

// timestampToTime converts pgtype.Timestamp to time.Time
func (s *PostgreSQLCAdapter) timestampToTime(ts pgtype.Timestamp) time.Time {
	if ts.Valid {
		return ts.Time
	}
	return time.Now()
}

// pgTextToString converts pgtype.Text to string
func (s *PostgreSQLCAdapter) pgTextToString(val pgtype.Text) string {
	if val.Valid {
		return val.String
	}
	return ""
}

// intToPgInt32 converts int to nullable pgtype.Int4
func (s *PostgreSQLCAdapter) intToPgInt32(val *int) *int32 {
	if val != nil && *val > 0 {
		i32 := int32(*val)
		return &i32
	}
	return nil
}

// User operations

func (s *PostgreSQLCAdapter) CreateUser(username, passwordHash string) error {
	ctx := context.Background()
	isDefault := false
	_, err := s.queries.CreateUser(ctx, postgres.CreateUserParams{
		Username:     username,
		PasswordHash: passwordHash,
		IsDefault:    &isDefault,
	})
	return err
}

func (s *PostgreSQLCAdapter) GetUser(username string) (*storage.User, error) {
	ctx := context.Background()
	user, err := s.queries.GetUserByUsername(ctx, username)
	if err != nil {
		return nil, s.HandlePgxNotFound(err)
	}

	return &storage.User{
		ID:           int(user.ID),
		Username:     user.Username,
		PasswordHash: user.PasswordHash,
		IsDefault:    s.ConvertNullableBool(user.IsDefault),
		CreatedAt:    s.timestampToTime(user.CreatedAt),
	}, nil
}

// Settings operations

func (s *PostgreSQLCAdapter) SetSetting(key, value string) error {
	ctx := context.Background()
	return s.queries.SetSetting(ctx, postgres.SetSettingParams{
		Key:   key,
		Value: value,
	})
}

func (s *PostgreSQLCAdapter) GetSetting(key string) (string, error) {
	ctx := context.Background()
	setting, err := s.queries.GetSetting(ctx, key)
	if err != nil {
		if err == pgx.ErrNoRows {
			return "", nil
		}
		return "", err
	}
	return setting.Value, nil
}

func (s *PostgreSQLCAdapter) DeleteSetting(key string) error {
	ctx := context.Background()
	return s.queries.DeleteSetting(ctx, key)
}

// Route operations

func (s *PostgreSQLCAdapter) CreateRoute(route *storage.Route) error {
	ctx := context.Background()

	// Convert signature fields
	var signatureConfig []byte
	var signatureSecret *string
	if route.SignatureConfig != "" {
		signatureConfig = []byte(route.SignatureConfig)
	}
	if route.SignatureSecret != "" {
		signatureSecret = &route.SignatureSecret
	}

	params := postgres.CreateRouteParams{
		Name:                route.Name,
		Endpoint:            route.Endpoint,
		Method:              route.Method,
		Queue:               route.Queue,
		Exchange:            &route.Exchange,
		RoutingKey:          route.RoutingKey,
		Filters:             []byte(route.Filters),
		Headers:             []byte(route.Headers),
		Active:              &route.Active,
		Priority:            s.intToPgInt32(&route.Priority),
		ConditionExpression: &route.ConditionExpression,
		PipelineID:          s.intToPgInt32(route.PipelineID),
		TriggerID:           s.intToPgInt32(route.TriggerID),
		DestinationBrokerID: s.intToPgInt32(route.DestinationBrokerID),
		SignatureConfig:     signatureConfig,
		SignatureSecret:     signatureSecret,
	}

	result, err := s.queries.CreateRoute(ctx, params)
	if err != nil {
		return err
	}

	route.ID = int(result.ID)
	route.CreatedAt = s.timestampToTime(result.CreatedAt)
	route.UpdatedAt = s.timestampToTime(result.UpdatedAt)
	return nil
}

func (s *PostgreSQLCAdapter) GetRoute(id int) (*storage.Route, error) {
	ctx := context.Background()
	route, err := s.queries.GetRoute(ctx, int32(id))
	if err != nil {
		return nil, s.HandlePgxNotFound(err)
	}

	return s.routeFromDB(route)
}

func (s *PostgreSQLCAdapter) GetRouteByEndpoint(endpoint, method string) (*storage.Route, error) {
	ctx := context.Background()

	route, err := s.queries.GetRouteByEndpoint(ctx, postgres.GetRouteByEndpointParams{
		Endpoint: endpoint,
		Method:   method,
	})
	if err != nil {
		return nil, s.HandlePgxNotFound(err)
	}

	return s.routeFromDB(route)
}

func (s *PostgreSQLCAdapter) ListRoutes() ([]*storage.Route, error) {
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

func (s *PostgreSQLCAdapter) UpdateRoute(route *storage.Route) error {
	ctx := context.Background()

	// Convert signature fields
	var signatureConfig []byte
	var signatureSecret *string
	if route.SignatureConfig != "" {
		signatureConfig = []byte(route.SignatureConfig)
	}
	if route.SignatureSecret != "" {
		signatureSecret = &route.SignatureSecret
	}

	params := postgres.UpdateRouteParams{
		ID:                  int32(route.ID),
		Name:                route.Name,
		Endpoint:            route.Endpoint,
		Method:              route.Method,
		Queue:               route.Queue,
		Exchange:            &route.Exchange,
		RoutingKey:          route.RoutingKey,
		Filters:             []byte(route.Filters),
		Headers:             []byte(route.Headers),
		Active:              &route.Active,
		Priority:            s.intToPgInt32(&route.Priority),
		ConditionExpression: &route.ConditionExpression,
		PipelineID:          s.intToPgInt32(route.PipelineID),
		TriggerID:           s.intToPgInt32(route.TriggerID),
		DestinationBrokerID: s.intToPgInt32(route.DestinationBrokerID),
		SignatureConfig:     signatureConfig,
		SignatureSecret:     signatureSecret,
	}

	_, err := s.queries.UpdateRoute(ctx, params)
	return err
}

func (s *PostgreSQLCAdapter) DeleteRoute(id int) error {
	ctx := context.Background()
	return s.queries.DeleteRoute(ctx, int32(id))
}

// Trigger operations

func (s *PostgreSQLCAdapter) CreateTrigger(trigger *storage.Trigger) error {
	ctx := context.Background()

	configJSON, err := s.MarshalJSON(trigger.Config)
	if err != nil {
		return err
	}

	params := postgres.CreateTriggerParams{
		Name:   trigger.Name,
		Type:   trigger.Type,
		Config: []byte(configJSON),
		Status: &trigger.Status,
		Active: &trigger.Active,
	}

	result, err := s.queries.CreateTrigger(ctx, params)
	if err != nil {
		return err
	}

	trigger.ID = int(result.ID)
	trigger.CreatedAt = s.timestampToTime(result.CreatedAt)
	trigger.UpdatedAt = s.timestampToTime(result.UpdatedAt)
	return nil
}

func (s *PostgreSQLCAdapter) GetTrigger(id int) (*storage.Trigger, error) {
	ctx := context.Background()
	trigger, err := s.queries.GetTrigger(ctx, int32(id))
	if err != nil {
		return nil, s.HandlePgxNotFound(err)
	}

	return s.triggerFromDB(trigger)
}

func (s *PostgreSQLCAdapter) ListTriggers() ([]*storage.Trigger, error) {
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

func (s *PostgreSQLCAdapter) UpdateTrigger(trigger *storage.Trigger) error {
	ctx := context.Background()

	configJSON, err := s.MarshalJSON(trigger.Config)
	if err != nil {
		return err
	}

	params := postgres.UpdateTriggerParams{
		ID:     int32(trigger.ID),
		Name:   trigger.Name,
		Type:   trigger.Type,
		Config: []byte(configJSON),
		Status: &trigger.Status,
		Active: &trigger.Active,
	}

	_, err = s.queries.UpdateTrigger(ctx, params)
	return err
}

func (s *PostgreSQLCAdapter) DeleteTrigger(id int) error {
	ctx := context.Background()
	return s.queries.DeleteTrigger(ctx, int32(id))
}

// Pipeline operations

func (s *PostgreSQLCAdapter) CreatePipeline(pipeline *storage.Pipeline) error {
	ctx := context.Background()

	stagesJSON, err := s.MarshalJSON(pipeline.Stages)
	if err != nil {
		return err
	}

	params := postgres.CreatePipelineParams{
		Name:        pipeline.Name,
		Description: s.ConvertStringToNullable(pipeline.Description),
		Stages:      []byte(stagesJSON),
		Active:      &pipeline.Active,
	}

	result, err := s.queries.CreatePipeline(ctx, params)
	if err != nil {
		return err
	}

	pipeline.ID = int(result.ID)
	pipeline.CreatedAt = s.timestampToTime(result.CreatedAt)
	pipeline.UpdatedAt = s.timestampToTime(result.UpdatedAt)
	return nil
}

func (s *PostgreSQLCAdapter) GetPipeline(id int) (*storage.Pipeline, error) {
	ctx := context.Background()
	pipeline, err := s.queries.GetPipeline(ctx, int32(id))
	if err != nil {
		return nil, s.HandlePgxNotFound(err)
	}

	return s.pipelineFromDB(pipeline)
}

func (s *PostgreSQLCAdapter) ListPipelines() ([]*storage.Pipeline, error) {
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

func (s *PostgreSQLCAdapter) UpdatePipeline(pipeline *storage.Pipeline) error {
	ctx := context.Background()

	stagesJSON, err := s.MarshalJSON(pipeline.Stages)
	if err != nil {
		return err
	}

	params := postgres.UpdatePipelineParams{
		ID:          int32(pipeline.ID),
		Name:        pipeline.Name,
		Description: s.ConvertStringToNullable(pipeline.Description),
		Stages:      []byte(stagesJSON),
		Active:      &pipeline.Active,
	}

	_, err = s.queries.UpdatePipeline(ctx, params)
	return err
}

func (s *PostgreSQLCAdapter) DeletePipeline(id int) error {
	ctx := context.Background()
	return s.queries.DeletePipeline(ctx, int32(id))
}

// BrokerConfig operations

func (s *PostgreSQLCAdapter) CreateBrokerConfig(config *storage.BrokerConfig) error {
	ctx := context.Background()

	configJSON, err := s.MarshalJSON(config.Config)
	if err != nil {
		return err
	}

	params := postgres.CreateBrokerConfigParams{
		Name:         config.Name,
		Type:         config.Type,
		Config:       []byte(configJSON),
		Active:       &config.Active,
		HealthStatus: &config.HealthStatus,
	}

	result, err := s.queries.CreateBrokerConfig(ctx, params)
	if err != nil {
		return err
	}

	config.ID = int(result.ID)
	config.CreatedAt = s.timestampToTime(result.CreatedAt)
	config.UpdatedAt = s.timestampToTime(result.UpdatedAt)
	return nil
}

func (s *PostgreSQLCAdapter) GetBrokerConfig(id int) (*storage.BrokerConfig, error) {
	ctx := context.Background()
	config, err := s.queries.GetBrokerConfig(ctx, int32(id))
	if err != nil {
		return nil, s.HandlePgxNotFound(err)
	}

	return s.brokerConfigFromDB(config)
}

func (s *PostgreSQLCAdapter) ListBrokerConfigs() ([]*storage.BrokerConfig, error) {
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

func (s *PostgreSQLCAdapter) UpdateBrokerConfig(config *storage.BrokerConfig) error {
	ctx := context.Background()

	configJSON, err := s.MarshalJSON(config.Config)
	if err != nil {
		return err
	}

	params := postgres.UpdateBrokerConfigParams{
		ID:     int32(config.ID),
		Name:   config.Name,
		Type:   config.Type,
		Config: []byte(configJSON),
		Active: &config.Active,
	}

	_, err = s.queries.UpdateBrokerConfig(ctx, params)
	return err
}

func (s *PostgreSQLCAdapter) DeleteBrokerConfig(id int) error {
	ctx := context.Background()
	return s.queries.DeleteBrokerConfig(ctx, int32(id))
}

// WebhookLog operations

func (s *PostgreSQLCAdapter) LogWebhook(log *storage.WebhookLog) error {
	ctx := context.Background()

	params := postgres.CreateWebhookLogParams{
		RouteID:              s.intToPgInt32(&log.RouteID),
		Method:               log.Method,
		Endpoint:             log.Endpoint,
		Headers:              []byte(log.Headers),
		Body:                 s.ConvertStringToNullable(log.Body),
		StatusCode:           s.intToPgInt32(&log.StatusCode),
		Error:                s.ConvertStringToNullable(log.Error),
		TransformationTimeMs: s.intToPgInt32(&log.TransformationTimeMS),
		BrokerPublishTimeMs:  s.intToPgInt32(&log.BrokerPublishTimeMS),
		TriggerID:            s.intToPgInt32(log.TriggerID),
		PipelineID:           s.intToPgInt32(log.PipelineID),
	}

	_, err := s.queries.CreateWebhookLog(ctx, params)
	return err
}

func (s *PostgreSQLCAdapter) GetWebhookLogs(limit, offset int) ([]*storage.WebhookLog, error) {
	ctx := context.Background()
	logs, err := s.queries.ListWebhookLogs(ctx, postgres.ListWebhookLogsParams{
		Limit:  int32(limit),
		Offset: int32(offset),
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

// Transaction support

func (s *PostgreSQLCAdapter) BeginTx() (storage.Transaction, error) {
	ctx := context.Background()
	tx, err := s.conn.Begin(ctx)
	if err != nil {
		return nil, err
	}
	return &PostgreSQLCTransaction{
		tx:      tx,
		queries: s.queries.WithTx(tx),
		adapter: s,
	}, nil
}

// Helper methods for converting from database types

func (s *PostgreSQLCAdapter) routeFromDB(route postgres.Route) (*storage.Route, error) {
	// Convert signature fields
	signatureConfig := ""
	if len(route.SignatureConfig) > 0 {
		signatureConfig = string(route.SignatureConfig)
	}

	return &storage.Route{
		ID:                  int(route.ID),
		Name:                route.Name,
		Endpoint:            route.Endpoint,
		Method:              route.Method,
		Queue:               route.Queue,
		Exchange:            s.ConvertNullableString(route.Exchange),
		RoutingKey:          route.RoutingKey,
		Filters:             string(route.Filters),
		Headers:             string(route.Headers),
		Active:              s.ConvertNullableBool(route.Active),
		Priority:            s.convertInt32PtrToInt(route.Priority),
		ConditionExpression: s.ConvertNullableString(route.ConditionExpression),
		SignatureConfig:     signatureConfig,
		SignatureSecret:     s.ConvertNullableString(route.SignatureSecret),
		CreatedAt:           s.timestampToTime(route.CreatedAt),
		UpdatedAt:           s.timestampToTime(route.UpdatedAt),
		PipelineID:          s.convertInt32PtrToIntPtr(route.PipelineID),
		TriggerID:           s.convertInt32PtrToIntPtr(route.TriggerID),
		DestinationBrokerID: s.convertInt32PtrToIntPtr(route.DestinationBrokerID),
	}, nil
}

// Helper functions for type conversions
func (s *PostgreSQLCAdapter) convertInt32PtrToInt(val *int32) int {
	if val != nil {
		return int(*val)
	}
	return 0
}

func (s *PostgreSQLCAdapter) convertInt32PtrToIntPtr(val *int32) *int {
	if val != nil {
		result := int(*val)
		return &result
	}
	return nil
}

func (s *PostgreSQLCAdapter) convertInt32PtrToInt64(val *int32) int {
	if val != nil {
		return int(*val)
	}
	return 0
}

func (s *PostgreSQLCAdapter) triggerFromDB(trigger postgres.Trigger) (*storage.Trigger, error) {
	var config map[string]interface{}
	if err := s.UnmarshalJSON(string(trigger.Config), &config); err != nil {
		return nil, err
	}

	t := &storage.Trigger{
		ID:           int(trigger.ID),
		Name:         trigger.Name,
		Type:         trigger.Type,
		Config:       config,
		Status:       s.ConvertNullableString(trigger.Status),
		Active:       s.ConvertNullableBool(trigger.Active),
		ErrorMessage: s.ConvertNullableString(trigger.ErrorMessage),
		CreatedAt:    s.timestampToTime(trigger.CreatedAt),
		UpdatedAt:    s.timestampToTime(trigger.UpdatedAt),
	}

	if trigger.LastExecution.Valid {
		t.LastExecution = &trigger.LastExecution.Time
	}
	if trigger.NextExecution.Valid {
		t.NextExecution = &trigger.NextExecution.Time
	}

	return t, nil
}

func (s *PostgreSQLCAdapter) pipelineFromDB(pipeline postgres.Pipeline) (*storage.Pipeline, error) {
	var stages []map[string]interface{}
	if err := s.UnmarshalJSON(string(pipeline.Stages), &stages); err != nil {
		return nil, err
	}

	return &storage.Pipeline{
		ID:          int(pipeline.ID),
		Name:        pipeline.Name,
		Description: s.ConvertNullableString(pipeline.Description),
		Stages:      stages,
		Active:      s.ConvertNullableBool(pipeline.Active),
		CreatedAt:   s.timestampToTime(pipeline.CreatedAt),
		UpdatedAt:   s.timestampToTime(pipeline.UpdatedAt),
	}, nil
}

func (s *PostgreSQLCAdapter) brokerConfigFromDB(config postgres.BrokerConfig) (*storage.BrokerConfig, error) {
	var configData map[string]interface{}
	if err := s.UnmarshalJSON(string(config.Config), &configData); err != nil {
		return nil, err
	}

	bc := &storage.BrokerConfig{
		ID:           int(config.ID),
		Name:         config.Name,
		Type:         config.Type,
		Config:       configData,
		Active:       s.ConvertNullableBool(config.Active),
		HealthStatus: s.ConvertNullableString(config.HealthStatus),
		CreatedAt:    s.timestampToTime(config.CreatedAt),
		UpdatedAt:    s.timestampToTime(config.UpdatedAt),
	}

	if config.LastHealthCheck.Valid {
		bc.LastHealthCheck = &config.LastHealthCheck.Time
	}

	return bc, nil
}

func (s *PostgreSQLCAdapter) webhookLogFromDB(log postgres.WebhookLog) (*storage.WebhookLog, error) {
	wl := &storage.WebhookLog{
		ID:                   int(log.ID),
		RouteID:              s.convertInt32PtrToInt64(log.RouteID),
		Method:               log.Method,
		Endpoint:             log.Endpoint,
		Headers:              string(log.Headers),
		Body:                 s.ConvertNullableString(log.Body),
		StatusCode:           s.convertInt32PtrToInt64(log.StatusCode),
		Error:                s.ConvertNullableString(log.Error),
		ProcessedAt:          s.timestampToTime(log.ProcessedAt),
		TransformationTimeMS: s.convertInt32PtrToInt64(log.TransformationTimeMs),
		BrokerPublishTimeMS:  s.convertInt32PtrToInt64(log.BrokerPublishTimeMs),
	}

	if log.TriggerID != nil {
		tid := int(*log.TriggerID)
		wl.TriggerID = &tid
	}
	if log.PipelineID != nil {
		pid := int(*log.PipelineID)
		wl.PipelineID = &pid
	}

	return wl, nil
}

// PostgreSQLCTransaction implements storage.Transaction interface
type PostgreSQLCTransaction struct {
	tx      pgx.Tx
	queries *postgres.Queries
	adapter *PostgreSQLCAdapter
}

func (t *PostgreSQLCTransaction) Commit() error {
	ctx := context.Background()
	return t.tx.Commit(ctx)
}

func (t *PostgreSQLCTransaction) Rollback() error {
	ctx := context.Background()
	return t.tx.Rollback(ctx)
}

// Other required methods

func (s *PostgreSQLCAdapter) Close() error {
	ctx := context.Background()
	return s.conn.Close(ctx)
}

func (s *PostgreSQLCAdapter) Ping() error {
	ctx := context.Background()
	return s.conn.Ping(ctx)
}

func (s *PostgreSQLCAdapter) GetStatistics(since time.Time) (*storage.Statistics, error) {
	// TODO: Implement statistics gathering
	return &storage.Statistics{
		TotalRequests:      0,
		SuccessfulRequests: 0,
		FailedRequests:     0,
		Since:              since,
		TopEndpoints:       []storage.EndpointStats{},
		ErrorBreakdown:     map[string]int{},
	}, nil
}

// Storage interface implementations

func (s *PostgreSQLCAdapter) Connect(config storage.StorageConfig) error {
	// Connection is already established through conn parameter in constructor
	return nil
}

func (s *PostgreSQLCAdapter) Health() error {
	return s.Ping()
}

func (s *PostgreSQLCAdapter) GetRoutes() ([]*storage.Route, error) {
	return s.ListRoutes()
}

func (s *PostgreSQLCAdapter) FindMatchingRoutes(endpoint, method string) ([]*storage.Route, error) {
	// Use the optimized GetRouteByEndpoint method
	route, err := s.GetRouteByEndpoint(endpoint, method)
	if err != nil {
		return nil, err
	}
	if route == nil {
		return []*storage.Route{}, nil
	}
	return []*storage.Route{route}, nil
}

func (s *PostgreSQLCAdapter) ValidateUser(username, password string) (*storage.User, error) {
	user, err := s.GetUser(username)
	if err != nil || user == nil {
		return nil, err
	}
	// Note: Password validation should be done by the caller
	return user, nil
}

func (s *PostgreSQLCAdapter) UpdateUserCredentials(userID int, username, password string) error {
	ctx := context.Background()
	return s.queries.UpdateUserCredentials(ctx, postgres.UpdateUserCredentialsParams{
		ID:           int32(userID),
		Username:     username,
		PasswordHash: password,
	})
}

func (s *PostgreSQLCAdapter) IsDefaultUser(userID int) (bool, error) {
	ctx := context.Background()
	user, err := s.queries.GetUser(ctx, int32(userID))
	if err != nil {
		return false, err
	}

	return s.ConvertNullableBool(user.IsDefault), nil
}

func (s *PostgreSQLCAdapter) GetAllSettings() (map[string]string, error) {
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

// GetStats returns overall system statistics for the last 24 hours
func (s *PostgreSQLCAdapter) GetStats() (*storage.Stats, error) {
	ctx := context.Background()

	// Get active routes count
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

	stats, err := s.queries.GetWebhookLogStats(ctx, pgtype.Timestamp{
		Time:  since,
		Valid: true,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get webhook log stats: %w", err)
	}

	return &storage.Stats{
		TotalRequests:   int(stats.TotalCount),
		SuccessRequests: int(stats.SuccessCount),
		FailedRequests:  int(stats.ErrorCount),
		ActiveRoutes:    activeRoutes,
	}, nil
}

// GetRouteStats returns detailed statistics for a specific route
func (s *PostgreSQLCAdapter) GetRouteStats(routeID int) (map[string]interface{}, error) {
	ctx := context.Background()

	stats, err := s.queries.GetRouteStatistics(ctx, int32(routeID))
	if err != nil {
		return nil, fmt.Errorf("failed to get route statistics: %w", err)
	}

	// Convert to sql.Null types for BuildStatsResult
	avgTransformTime := sql.NullFloat64{
		Float64: stats.AvgTransformationTime,
		Valid:   stats.AvgTransformationTime != 0, // PostgreSQL uses 0 for no data
	}

	avgPublishTime := sql.NullFloat64{
		Float64: stats.AvgPublishTime,
		Valid:   stats.AvgPublishTime != 0, // PostgreSQL uses 0 for no data
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

// GetTriggers returns triggers matching the specified filters
func (s *PostgreSQLCAdapter) GetTriggers(filters storage.TriggerFilters) ([]*storage.Trigger, error) {
	triggers, err := s.ListTriggers()
	if err != nil {
		return nil, err
	}

	return s.ApplyTriggerFilters(triggers, filters), nil
}

func (s *PostgreSQLCAdapter) GetPipelines() ([]*storage.Pipeline, error) {
	return s.ListPipelines()
}

// Broker interface aliases
func (s *PostgreSQLCAdapter) CreateBroker(broker *storage.BrokerConfig) error {
	return s.CreateBrokerConfig(broker)
}

func (s *PostgreSQLCAdapter) GetBroker(id int) (*storage.BrokerConfig, error) {
	return s.GetBrokerConfig(id)
}

func (s *PostgreSQLCAdapter) GetBrokers() ([]*storage.BrokerConfig, error) {
	return s.ListBrokerConfigs()
}

func (s *PostgreSQLCAdapter) UpdateBroker(broker *storage.BrokerConfig) error {
	return s.UpdateBrokerConfig(broker)
}

func (s *PostgreSQLCAdapter) DeleteBroker(id int) error {
	return s.DeleteBrokerConfig(id)
}

// Query executes a raw SQL query and returns results as a slice of maps
func (s *PostgreSQLCAdapter) Query(query string, args ...interface{}) ([]map[string]interface{}, error) {
	ctx := context.Background()
	rows, err := s.conn.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []map[string]interface{}
	for rows.Next() {
		vals, err := rows.Values()
		if err != nil {
			return nil, err
		}

		fields := rows.FieldDescriptions()
		row := make(map[string]interface{})
		for i, field := range fields {
			row[string(field.Name)] = vals[i]
		}

		results = append(results, row)
	}

	return results, rows.Err()
}

// Transaction executes a function within a database transaction
func (s *PostgreSQLCAdapter) Transaction(fn func(storage.Transaction) error) error {
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

// DLQ Operations

// CreateDLQMessage implements Storage interface
func (s *PostgreSQLCAdapter) CreateDLQMessage(message *storage.DLQMessage) error {
	ctx := context.Background()

	headers, err := json.Marshal(message.Headers)
	if err != nil {
		return fmt.Errorf("failed to marshal headers: %w", err)
	}

	metadata, err := json.Marshal(message.Metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	// For PostgreSQL, we need to handle the DLQ broker IDs
	// Assuming the storage layer will provide the correct broker IDs
	params := postgres.CreateDLQMessageParams{
		MessageID:      message.MessageID,
		RouteID:        int32(message.RouteID),
		TriggerID:      s.intToPgInt32(message.TriggerID),
		PipelineID:     s.intToPgInt32(message.PipelineID),
		SourceBrokerID: int32(1), // Default to 1 if not provided
		DlqBrokerID:    int32(1), // Default to 1 if not provided
		BrokerName:     message.BrokerName,
		Queue:          message.Queue,
		Exchange:       s.ConvertStringToNullable(message.Exchange),
		RoutingKey:     message.RoutingKey,
		Headers:        headers,
		Body:           message.Body,
		ErrorMessage:   message.ErrorMessage,
		FailureCount:   s.intToPgInt32(&message.FailureCount),
		FirstFailure: pgtype.Timestamp{
			Time:  message.FirstFailure,
			Valid: true,
		},
		LastFailure: pgtype.Timestamp{
			Time:  message.LastFailure,
			Valid: true,
		},
		Status:   s.ConvertStringToNullable(message.Status),
		Metadata: metadata,
	}

	if message.NextRetry != nil {
		params.NextRetry = pgtype.Timestamp{
			Time:  *message.NextRetry,
			Valid: true,
		}
	}

	result, err := s.queries.CreateDLQMessage(ctx, params)
	if err != nil {
		return fmt.Errorf("failed to create DLQ message: %w", err)
	}

	message.ID = int(result.ID)
	return nil
}

// GetDLQMessage implements Storage interface
func (s *PostgreSQLCAdapter) GetDLQMessage(id int) (*storage.DLQMessage, error) {
	ctx := context.Background()
	msg, err := s.queries.GetDLQMessage(ctx, int32(id))
	if err != nil {
		return nil, s.HandlePgxNotFound(err)
	}

	return s.dlqMessageFromDB(msg)
}

// GetDLQMessageByMessageID implements Storage interface
func (s *PostgreSQLCAdapter) GetDLQMessageByMessageID(messageID string) (*storage.DLQMessage, error) {
	ctx := context.Background()
	msg, err := s.queries.GetDLQMessageByMessageID(ctx, messageID)
	if err != nil {
		return nil, s.HandlePgxNotFound(err)
	}

	return s.dlqMessageFromDB(msg)
}

// ListPendingDLQMessages implements Storage interface
func (s *PostgreSQLCAdapter) ListPendingDLQMessages(limit int) ([]*storage.DLQMessage, error) {
	ctx := context.Background()
	messages, err := s.queries.ListPendingDLQMessages(ctx, int32(limit))
	if err != nil {
		return nil, err
	}

	result := make([]*storage.DLQMessage, len(messages))
	for i, msg := range messages {
		result[i], err = s.dlqMessageFromDB(msg)
		if err != nil {
			return nil, err
		}
	}

	return result, nil
}

// ListDLQMessages implements Storage interface
func (s *PostgreSQLCAdapter) ListDLQMessages(limit, offset int) ([]*storage.DLQMessage, error) {
	ctx := context.Background()
	messages, err := s.queries.ListDLQMessages(ctx, postgres.ListDLQMessagesParams{
		Limit:  int32(limit),
		Offset: int32(offset),
	})
	if err != nil {
		return nil, err
	}

	result := make([]*storage.DLQMessage, len(messages))
	for i, msg := range messages {
		result[i], err = s.dlqMessageFromDB(msg)
		if err != nil {
			return nil, err
		}
	}

	return result, nil
}

// ListDLQMessagesByRoute implements Storage interface
func (s *PostgreSQLCAdapter) ListDLQMessagesByRoute(routeID int, limit, offset int) ([]*storage.DLQMessage, error) {
	ctx := context.Background()
	messages, err := s.queries.ListDLQMessagesByRoute(ctx, postgres.ListDLQMessagesByRouteParams{
		RouteID: int32(routeID),
		Limit:   int32(limit),
		Offset:  int32(offset),
	})
	if err != nil {
		return nil, err
	}

	result := make([]*storage.DLQMessage, len(messages))
	for i, msg := range messages {
		result[i], err = s.dlqMessageFromDB(msg)
		if err != nil {
			return nil, err
		}
	}

	return result, nil
}

// ListDLQMessagesByStatus implements Storage interface
func (s *PostgreSQLCAdapter) ListDLQMessagesByStatus(status string, limit, offset int) ([]*storage.DLQMessage, error) {
	ctx := context.Background()
	messages, err := s.queries.ListDLQMessagesByStatus(ctx, postgres.ListDLQMessagesByStatusParams{
		Status: &status,
		Limit:  int32(limit),
		Offset: int32(offset),
	})
	if err != nil {
		return nil, err
	}

	result := make([]*storage.DLQMessage, len(messages))
	for i, msg := range messages {
		result[i], err = s.dlqMessageFromDB(msg)
		if err != nil {
			return nil, err
		}
	}

	return result, nil
}

// UpdateDLQMessage implements Storage interface
func (s *PostgreSQLCAdapter) UpdateDLQMessage(message *storage.DLQMessage) error {
	ctx := context.Background()

	metadata, err := json.Marshal(message.Metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	params := postgres.UpdateDLQMessageParams{
		ID:           int32(message.ID),
		ErrorMessage: message.ErrorMessage,
		FailureCount: s.intToPgInt32(&message.FailureCount),
		LastFailure: pgtype.Timestamp{
			Time:  message.LastFailure,
			Valid: true,
		},
		Status:   s.ConvertStringToNullable(message.Status),
		Metadata: metadata,
	}

	if message.NextRetry != nil {
		params.NextRetry = pgtype.Timestamp{
			Time:  *message.NextRetry,
			Valid: true,
		}
	} else {
		params.NextRetry = pgtype.Timestamp{Valid: false}
	}

	_, err = s.queries.UpdateDLQMessage(ctx, params)
	return err
}

// UpdateDLQMessageStatus implements Storage interface
func (s *PostgreSQLCAdapter) UpdateDLQMessageStatus(id int, status string) error {
	ctx := context.Background()
	return s.queries.UpdateDLQMessageStatus(ctx, postgres.UpdateDLQMessageStatusParams{
		ID:     int32(id),
		Status: &status,
	})
}

// DeleteDLQMessage implements Storage interface
func (s *PostgreSQLCAdapter) DeleteDLQMessage(id int) error {
	ctx := context.Background()
	return s.queries.DeleteDLQMessage(ctx, int32(id))
}

// DeleteOldDLQMessages implements Storage interface
func (s *PostgreSQLCAdapter) DeleteOldDLQMessages(before time.Time) error {
	ctx := context.Background()
	return s.queries.DeleteOldDLQMessages(ctx, pgtype.Timestamp{
		Time:  before,
		Valid: true,
	})
}

// GetDLQStats implements Storage interface
func (s *PostgreSQLCAdapter) GetDLQStats() (*storage.DLQStats, error) {
	ctx := context.Background()

	// Calculate stats manually since we don't have a dedicated query
	allMessages, err := s.queries.ListDLQMessages(ctx, postgres.ListDLQMessagesParams{
		Limit:  10000,
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

		if oldestFailure == nil || msg.FirstFailure.Time.Before(*oldestFailure) {
			oldestFailure = &msg.FirstFailure.Time
		}
	}

	stats.OldestFailure = oldestFailure
	return stats, nil
}

// GetDLQStatsByRoute implements Storage interface
func (s *PostgreSQLCAdapter) GetDLQStatsByRoute() ([]*storage.DLQRouteStats, error) {
	ctx := context.Background()

	// Calculate stats manually since we don't have a dedicated query
	allMessages, err := s.queries.ListDLQMessages(ctx, postgres.ListDLQMessagesParams{
		Limit:  10000,
		Offset: 0,
	})
	if err != nil {
		return nil, err
	}

	// Group by route ID
	routeStats := make(map[int]*storage.DLQRouteStats)
	for _, msg := range allMessages {
		routeID := int(msg.RouteID)
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

// Helper method to convert database DLQ message to domain model
func (s *PostgreSQLCAdapter) dlqMessageFromDB(msg postgres.DlqMessage) (*storage.DLQMessage, error) {
	var headers map[string]string
	if err := json.Unmarshal(msg.Headers, &headers); err != nil {
		return nil, fmt.Errorf("failed to unmarshal headers: %w", err)
	}

	var metadata map[string]interface{}
	if err := json.Unmarshal(msg.Metadata, &metadata); err != nil {
		return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
	}

	result := &storage.DLQMessage{
		ID:           int(msg.ID),
		MessageID:    msg.MessageID,
		RouteID:      int(msg.RouteID),
		TriggerID:    s.convertInt32PtrToIntPtr(msg.TriggerID),
		PipelineID:   s.convertInt32PtrToIntPtr(msg.PipelineID),
		BrokerName:   msg.BrokerName,
		Queue:        msg.Queue,
		Exchange:     s.ConvertNullableString(msg.Exchange),
		RoutingKey:   msg.RoutingKey,
		Headers:      headers,
		Body:         msg.Body,
		ErrorMessage: msg.ErrorMessage,
		FailureCount: s.convertInt32PtrToInt(msg.FailureCount),
		FirstFailure: s.timestampToTime(msg.FirstFailure),
		LastFailure:  s.timestampToTime(msg.LastFailure),
		Status:       s.ConvertNullableString(msg.Status),
		Metadata:     metadata,
		CreatedAt:    s.timestampToTime(msg.CreatedAt),
		UpdatedAt:    s.timestampToTime(msg.UpdatedAt),
	}

	if msg.NextRetry.Valid {
		result.NextRetry = &msg.NextRetry.Time
	}

	return result, nil
}
