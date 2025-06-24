// Package sqlc provides type-safe database adapters using SQLC code generation.
package sqlc

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/lucsky/cuid"
	"golang.org/x/crypto/bcrypt"
	"webhook-router/internal/common/errors"
	"webhook-router/internal/common/utils"
	"webhook-router/internal/crypto"
	"webhook-router/internal/storage"
	postgres "webhook-router/internal/storage/generated/postgres"
)

// PostgreSQLCAdapter implements the Storage interface using SQLC-generated code for PostgreSQL.
// It provides type-safe database operations with automatic NULL handling for PostgreSQL-specific types.
// This adapter handles all storage operations including routes, users, settings, triggers,
// pipelines, brokers, and webhook logs.
type PostgreSQLCAdapter struct {
	BaseAdapter
	conn      *pgx.Conn
	queries   *postgres.Queries
	encryptor *crypto.ConfigEncryptor // Optional encryptor for sensitive data
}

// NewPostgreSQLCAdapter creates a new PostgreSQL storage adapter using SQLC-generated queries.
// The conn parameter should be an open PostgreSQL database connection.
func NewPostgreSQLCAdapter(conn *pgx.Conn) *PostgreSQLCAdapter {
	return &PostgreSQLCAdapter{
		conn:      conn,
		queries:   postgres.New(conn),
		encryptor: nil,
	}
}

// NewSecurePostgreSQLCAdapter creates a new PostgreSQL storage adapter with encryption.
func NewSecurePostgreSQLCAdapter(conn *pgx.Conn, encryptor *crypto.ConfigEncryptor) *PostgreSQLCAdapter {
	return &PostgreSQLCAdapter{
		conn:      conn,
		queries:   postgres.New(conn),
		encryptor: encryptor,
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

func (s *PostgreSQLCAdapter) CreateUser(username, password string) (*storage.User, error) {
	ctx := context.Background()

	// Hash the password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, errors.InternalError("failed to hash password", err)
	}

	// Check if this is the first user - they become the server owner
	userCount, err := s.queries.CountUsers(ctx)
	if err != nil {
		return nil, err
	}

	// First user is the server owner (not a default user)
	// Subsequent users are regular users (also not default users)
	// Note: Server ownership is implicit based on user creation order
	isDefault := false
	isFirstUser := userCount == 0

	result, err := s.queries.CreateUser(ctx, postgres.CreateUserParams{
		ID:           cuid.New(),
		Username:     username,
		PasswordHash: string(hashedPassword),
		IsDefault:    &isDefault,
	})
	if err != nil {
		return nil, err
	}

	// Log if this is the first user (server owner)
	if isFirstUser {
		// This user becomes the implicit server owner
		// Future authorization logic can check if userID matches the first created user
	}

	// Convert to storage.User
	return &storage.User{
		ID:           result.ID,
		Username:     result.Username,
		PasswordHash: result.PasswordHash,
		IsDefault:    s.ConvertNullableBool(result.IsDefault),
		CreatedAt:    s.timestampToTime(result.CreatedAt),
		UpdatedAt:    s.timestampToTime(result.UpdatedAt),
	}, nil
}

func (s *PostgreSQLCAdapter) GetUser(userID string) (*storage.User, error) {
	ctx := context.Background()
	user, err := s.queries.GetUser(ctx, userID)
	if err != nil {
		return nil, s.HandlePgxNotFound(err)
	}

	return &storage.User{
		ID:           user.ID,
		Username:     user.Username,
		PasswordHash: user.PasswordHash,
		IsDefault:    s.ConvertNullableBool(user.IsDefault),
		CreatedAt:    s.timestampToTime(user.CreatedAt),
		UpdatedAt:    s.timestampToTime(user.UpdatedAt),
	}, nil
}

func (s *PostgreSQLCAdapter) GetUserByUsername(username string) (*storage.User, error) {
	ctx := context.Background()
	user, err := s.queries.GetUserByUsername(ctx, username)
	if err != nil {
		return nil, s.HandlePgxNotFound(err)
	}

	return &storage.User{
		ID:           user.ID,
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

// func (s *PostgreSQLCAdapter) CreateRoute(route *storage.Route) error {
// 	ctx := context.Background()
//
// 	// Convert signature fields
// 	var signatureConfig *string
// 	var signatureSecret *string
// 	if route.SignatureConfig != "" {
// 		signatureConfig = &route.SignatureConfig
// 	}
// 	if route.SignatureSecret != "" {
// 		signatureSecret = &route.SignatureSecret
// 	}
//
// 	params := postgres.CreateRouteParams{
// 		ID:                  cuid.New(),
// 		Name:                route.Name,
// 		Method:              route.Method,
// 		Queue:               route.Queue,
// 		Exchange:            &route.Exchange,
// 		RoutingKey:          route.RoutingKey,
// 		Filters:             []byte(route.Filters),
// 		Headers:             []byte(route.Headers),
// 		Active:              &route.Active,
// 		Priority:            s.intToPgInt32(&route.Priority),
// 		ConditionExpression: &route.ConditionExpression,
// 		PipelineID:          route.PipelineID,
// 		TriggerID:           route.TriggerID,
// 		DestinationBrokerID: route.DestinationBrokerID,
// 		SignatureConfig:     signatureConfig,
// 		SignatureSecret:     signatureSecret,
// 		UserID:              route.UserID,
// 	}
//
// 	result, err := s.queries.CreateRoute(ctx, params)
// 	if err != nil {
// 		return err
// 	}
//
// 	route.ID = result.ID
// 	route.CreatedAt = s.timestampToTime(result.CreatedAt)
// 	route.UpdatedAt = s.timestampToTime(result.UpdatedAt)
// 	return nil
// }

// User-scoped route methods (new interface implementation)
// func (s *PostgreSQLCAdapter) GetRoute(id string, userID string) (*storage.Route, error) {
// 	ctx := context.Background()
// 	route, err := s.queries.GetRouteByUser(ctx, postgres.GetRouteByUserParams{
// 		ID:     id,
// 		UserID: userID,
// 	})
// 	if err != nil {
// 		return nil, s.HandlePgxNotFound(err)
// 	}
//
// 	return s.routeFromDB(route)
// }

// GetRouteByEndpoint finds a route by its endpoint path (interface method - checks any method).
// Returns nil if no matching route is found.

// func (s *PostgreSQLCAdapter) ListTriggers() ([]*storage.Route, error) {
// 	ctx := context.Background()
// 	routes, err := s.queries.ListTriggers(ctx)
// 	if err != nil {
// 		return nil, err
// 	}
//
// 	result := make([]*storage.Route, len(routes))
// 	for i, route := range routes {
// 		result[i], err = s.routeFromDB(route)
// 		if err != nil {
// 			return nil, err
// 		}
// 	}
//
// 	return result, nil
// }

// GetRoutesPaginated implements Storage interface with pagination
// func (s *PostgreSQLCAdapter) GetRoutesPaginated(limit, offset int) ([]*storage.Route, int, error) {
// 	ctx := context.Background()
//
// 	// Get total count
// 	countResult, err := s.queries.CountRoutes(ctx)
// 	if err != nil {
// 		return nil, 0, err
// 	}
// 	totalCount := int(countResult)
//
// 	// Get paginated routes
// 	routes, err := s.queries.ListTriggersPaginated(ctx, postgres.ListTriggersPaginatedParams{
// 		Limit:  int32(limit),
// 		Offset: int32(offset),
// 	})
// 	if err != nil {
// 		return nil, 0, err
// 	}
//
// 	result := make([]*storage.Route, len(routes))
// 	for i, route := range routes {
// 		result[i], err = s.routeFromDB(route)
// 		if err != nil {
// 			return nil, 0, err
// 		}
// 	}
//
// 	return result, totalCount, nil
// }

// User-scoped update
// func (s *PostgreSQLCAdapter) UpdateRoute(route *storage.Route, userID string) error {
// 	ctx := context.Background()
//
// 	// Convert signature fields
// 	var signatureConfig *string
// 	var signatureSecret *string
// 	if route.SignatureConfig != "" {
// 		signatureConfig = &route.SignatureConfig
// 	}
// 	if route.SignatureSecret != "" {
// 		signatureSecret = &route.SignatureSecret
// 	}
//
// 	params := postgres.UpdateRouteByUserParams{
// 		ID:                  route.ID,
// 		UserID:              userID,
// 		Name:                route.Name,
// 		Method:              route.Method,
// 		Queue:               route.Queue,
// 		Exchange:            &route.Exchange,
// 		RoutingKey:          route.RoutingKey,
// 		Filters:             []byte(route.Filters),
// 		Headers:             []byte(route.Headers),
// 		Active:              &route.Active,
// 		Priority:            s.intToPgInt32(&route.Priority),
// 		ConditionExpression: &route.ConditionExpression,
// 		PipelineID:          route.PipelineID,
// 		TriggerID:           route.TriggerID,
// 		DestinationBrokerID: route.DestinationBrokerID,
// 		SignatureConfig:     signatureConfig,
// 		SignatureSecret:     signatureSecret,
// 	}
//
// 	_, err := s.queries.UpdateRouteByUser(ctx, params)
// 	return err
// }

// User-scoped delete
// func (s *PostgreSQLCAdapter) DeleteRoute(id string, userID string) error {
// 	ctx := context.Background()
// 	return s.queries.DeleteRouteByUser(ctx, postgres.DeleteRouteByUserParams{
// 		ID:     id,
// 		UserID: userID,
// 	})
// }

// GetRoutesByUser returns all routes for a specific user
// func (s *PostgreSQLCAdapter) GetRoutesByUser(userID string) ([]*storage.Route, error) {
// 	ctx := context.Background()
// 	routes, err := s.queries.GetRoutesByUser(ctx, userID)
// 	if err != nil {
// 		return nil, err
// 	}
//
// 	result := make([]*storage.Route, len(routes))
// 	for i, route := range routes {
// 		result[i], err = s.routeFromDB(route)
// 		if err != nil {
// 			return nil, err
// 		}
// 	}
//
// 	return result, nil
// }

// Trigger operations

func (s *PostgreSQLCAdapter) CreateTrigger(trigger *storage.Trigger) error {
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

	params := postgres.CreateTriggerParams{
		ID:          cuid.New(),
		Name:        trigger.Name,
		Type:        trigger.Type,
		Config:      []byte(configJSON),
		Status:      &trigger.Status,
		Active:      &trigger.Active,
		DlqEnabled:  &trigger.DLQEnabled,
		DlqRetryMax: func() *int32 { v := int32(trigger.DLQRetryMax); return &v }(),
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
	trigger.CreatedAt = s.timestampToTime(result.CreatedAt)
	trigger.UpdatedAt = s.timestampToTime(result.UpdatedAt)
	return nil
}

func (s *PostgreSQLCAdapter) GetTrigger(id string) (*storage.Trigger, error) {
	ctx := context.Background()
	trigger, err := s.queries.GetTrigger(ctx, id)
	if err != nil {
		return nil, s.HandlePgxNotFound(err)
	}

	return s.triggerFromDB(trigger)
}

func (s *PostgreSQLCAdapter) GetHTTPTriggerByUserPathMethod(userID, path, method string) (*storage.Trigger, error) {
	ctx := context.Background()
	trigger, err := s.queries.GetHTTPTriggerByUserPathMethod(ctx, postgres.GetHTTPTriggerByUserPathMethodParams{
		UserID:   userID,
		Config:   []byte(path),   // First json extract parameter
		Config_2: []byte(method), // Second json extract parameter
	})
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

	// Encrypt sensitive fields in config before storing
	encryptedConfig, err := s.encryptSensitiveConfig(trigger.Config)
	if err != nil {
		return err
	}

	configJSON, err := s.MarshalJSON(encryptedConfig)
	if err != nil {
		return err
	}

	params := postgres.UpdateTriggerParams{
		ID:          trigger.ID,
		Name:        trigger.Name,
		Type:        trigger.Type,
		Config:      []byte(configJSON),
		Status:      &trigger.Status,
		Active:      &trigger.Active,
		DlqEnabled:  &trigger.DLQEnabled,
		DlqRetryMax: func() *int32 { v := int32(trigger.DLQRetryMax); return &v }(),
	}

	// Handle optional DLQ broker ID
	if trigger.DLQBrokerID != nil {
		params.DlqBrokerID = trigger.DLQBrokerID
	}

	_, err = s.queries.UpdateTrigger(ctx, params)
	return err
}

func (s *PostgreSQLCAdapter) DeleteTrigger(id string) error {
	ctx := context.Background()
	return s.queries.DeleteTrigger(ctx, id)
}

// Pipeline operations

func (s *PostgreSQLCAdapter) CreatePipeline(pipeline *storage.Pipeline) error {
	ctx := context.Background()

	stagesJSON, err := s.MarshalJSON(pipeline.Stages)
	if err != nil {
		return err
	}

	params := postgres.CreatePipelineParams{
		ID:          cuid.New(),
		Name:        pipeline.Name,
		Description: s.ConvertStringToNullable(pipeline.Description),
		Stages:      []byte(stagesJSON),
		Active:      &pipeline.Active,
	}

	result, err := s.queries.CreatePipeline(ctx, params)
	if err != nil {
		return err
	}

	pipeline.ID = result.ID
	pipeline.CreatedAt = s.timestampToTime(result.CreatedAt)
	pipeline.UpdatedAt = s.timestampToTime(result.UpdatedAt)
	return nil
}

func (s *PostgreSQLCAdapter) GetPipeline(id string) (*storage.Pipeline, error) {
	ctx := context.Background()
	pipeline, err := s.queries.GetPipeline(ctx, id)
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
		ID:          pipeline.ID,
		Name:        pipeline.Name,
		Description: s.ConvertStringToNullable(pipeline.Description),
		Stages:      []byte(stagesJSON),
		Active:      &pipeline.Active,
	}

	_, err = s.queries.UpdatePipeline(ctx, params)
	return err
}

func (s *PostgreSQLCAdapter) DeletePipeline(id string) error {
	ctx := context.Background()
	return s.queries.DeletePipeline(ctx, id)
}

// BrokerConfig operations

func (s *PostgreSQLCAdapter) CreateBroker(config *storage.BrokerConfig) error {
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

	params := postgres.CreateBrokerConfigParams{
		ID:           cuid.New(),
		Name:         config.Name,
		Type:         config.Type,
		Config:       []byte(configJSON),
		Active:       &config.Active,
		HealthStatus: &config.HealthStatus,
		DlqEnabled:   config.DlqEnabled,
		DlqBrokerID:  config.DlqBrokerID,
	}

	result, err := s.queries.CreateBrokerConfig(ctx, params)
	if err != nil {
		return err
	}

	config.ID = result.ID
	config.CreatedAt = s.timestampToTime(result.CreatedAt)
	config.UpdatedAt = s.timestampToTime(result.UpdatedAt)
	return nil
}

func (s *PostgreSQLCAdapter) GetBroker(id string) (*storage.BrokerConfig, error) {
	ctx := context.Background()
	config, err := s.queries.GetBrokerConfig(ctx, id)
	if err != nil {
		return nil, s.HandlePgxNotFound(err)
	}

	return s.brokerConfigFromDB(config)
}

func (s *PostgreSQLCAdapter) GetBrokers() ([]*storage.BrokerConfig, error) {
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

// GetBrokersPaginated implements Storage interface with pagination
func (s *PostgreSQLCAdapter) GetBrokersPaginated(limit, offset int) ([]*storage.BrokerConfig, int, error) {
	ctx := context.Background()

	// Get total count
	countResult, err := s.queries.CountBrokerConfigs(ctx)
	if err != nil {
		return nil, 0, err
	}
	totalCount := int(countResult)

	// Get paginated broker configs
	configs, err := s.queries.ListBrokerConfigsPaginated(ctx, postgres.ListBrokerConfigsPaginatedParams{
		Limit:  int32(limit),
		Offset: int32(offset),
	})
	if err != nil {
		return nil, 0, err
	}

	result := make([]*storage.BrokerConfig, len(configs))
	for i, config := range configs {
		result[i], err = s.brokerConfigFromDB(config)
		if err != nil {
			return nil, 0, err
		}
	}

	return result, totalCount, nil
}

func (s *PostgreSQLCAdapter) UpdateBroker(config *storage.BrokerConfig) error {
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

	params := postgres.UpdateBrokerConfigParams{
		ID:     config.ID,
		Name:   config.Name,
		Type:   config.Type,
		Config: []byte(configJSON),
		Active: &config.Active,
	}

	_, err = s.queries.UpdateBrokerConfig(ctx, params)
	return err
}

func (s *PostgreSQLCAdapter) DeleteBroker(id string) error {
	ctx := context.Background()
	return s.queries.DeleteBrokerConfig(ctx, id)
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

// func (s *PostgreSQLCAdapter) routeFromDB(route postgres.Route) (*storage.Route, error) {
// 	// Convert signature fields
// 	signatureConfig := ""
// 	if route.SignatureConfig != nil && *route.SignatureConfig != "" {
// 		signatureConfig = *route.SignatureConfig
// 	}
//
// 	return &storage.Route{
// 		ID:                  route.ID,
// 		Name:                route.Name,
// 		Method:              route.Method,
// 		Queue:               route.Queue,
// 		Exchange:            s.ConvertNullableString(route.Exchange),
// 		RoutingKey:          route.RoutingKey,
// 		Filters:             string(route.Filters),
// 		Headers:             string(route.Headers),
// 		Active:              s.ConvertNullableBool(route.Active),
// 		Priority:            s.convertInt32PtrToInt(route.Priority),
// 		ConditionExpression: s.ConvertNullableString(route.ConditionExpression),
// 		SignatureConfig:     signatureConfig,
// 		SignatureSecret:     s.ConvertNullableString(route.SignatureSecret),
// 		CreatedAt:           s.timestampToTime(route.CreatedAt),
// 		UpdatedAt:           s.timestampToTime(route.UpdatedAt),
// 		PipelineID:          route.PipelineID,
// 		TriggerID:           route.TriggerID,
// 		DestinationBrokerID: route.DestinationBrokerID,
// 	}, nil
// }

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

	// Decrypt sensitive fields in config after loading
	decryptedConfig, err := s.decryptSensitiveConfig(config)
	if err != nil {
		return nil, err
	}

	t := &storage.Trigger{
		ID:           trigger.ID,
		Name:         trigger.Name,
		Type:         trigger.Type,
		Config:       decryptedConfig,
		Status:       s.ConvertNullableString(trigger.Status),
		Active:       s.ConvertNullableBool(trigger.Active),
		ErrorMessage: s.ConvertNullableString(trigger.ErrorMessage),
		CreatedAt:    s.timestampToTime(trigger.CreatedAt),
		UpdatedAt:    s.timestampToTime(trigger.UpdatedAt),
		DLQEnabled:   s.ConvertNullableBool(trigger.DlqEnabled),
		DLQRetryMax: func() int {
			if trigger.DlqRetryMax != nil {
				return int(*trigger.DlqRetryMax)
			}
			return 3
		}(),
	}

	if trigger.LastExecution.Valid {
		t.LastExecution = &trigger.LastExecution.Time
	}
	if trigger.NextExecution.Valid {
		t.NextExecution = &trigger.NextExecution.Time
	}

	// Handle DLQ broker ID
	if trigger.DlqBrokerID != nil {
		t.DLQBrokerID = trigger.DlqBrokerID
	}

	return t, nil
}

func (s *PostgreSQLCAdapter) pipelineFromDB(pipeline postgres.Pipeline) (*storage.Pipeline, error) {
	var stages []map[string]interface{}
	if err := s.UnmarshalJSON(string(pipeline.Stages), &stages); err != nil {
		return nil, err
	}

	return &storage.Pipeline{
		ID:          pipeline.ID,
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

	// Decrypt sensitive fields in config after loading
	decryptedConfig, err := s.decryptSensitiveConfig(configData)
	if err != nil {
		return nil, err
	}

	bc := &storage.BrokerConfig{
		ID:           config.ID,
		Name:         config.Name,
		Type:         config.Type,
		Config:       decryptedConfig,
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

// GetStatistics is not part of the Storage interface and is not used anywhere.
// The actual statistics methods are GetStats() and GetRouteStats() which are already implemented.

// Storage interface implementations

func (s *PostgreSQLCAdapter) Connect(config storage.StorageConfig) error {
	// Connection is already established through conn parameter in constructor
	return nil
}

func (s *PostgreSQLCAdapter) Health() error {
	return s.Ping()
}

// func (s *PostgreSQLCAdapter) GetRoutes() ([]*storage.Route, error) {
// 	return s.ListTriggers()
// }

func (s *PostgreSQLCAdapter) ValidateUser(username, password string) (*storage.User, error) {
	user, err := s.GetUserByUsername(username)
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

func (s *PostgreSQLCAdapter) GetUserCount() (int, error) {
	ctx := context.Background()
	count, err := s.queries.CountUsers(ctx)
	if err != nil {
		return 0, err
	}
	return int(count), nil
}

// IsServerOwner determines if the given user is the server owner
// The server owner is the first user created (earliest created_at timestamp)
func (s *PostgreSQLCAdapter) IsServerOwner(userID string) (bool, error) {
	// Get the earliest created user ID
	ctx := context.Background()
	var firstUserID string
	err := s.conn.QueryRow(ctx, "SELECT id FROM users ORDER BY created_at ASC LIMIT 1").Scan(&firstUserID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return false, nil // No users exist
		}
		return false, err
	}

	return firstUserID == userID, nil
}

func (s *PostgreSQLCAdapter) UpdateUserCredentials(userID string, username, password string) error {
	ctx := context.Background()

	// Hash the new password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return errors.InternalError("failed to hash password", err)
	}

	return s.queries.UpdateUserCredentials(ctx, postgres.UpdateUserCredentialsParams{
		ID:           userID,
		Username:     username,
		PasswordHash: string(hashedPassword),
	})
}

func (s *PostgreSQLCAdapter) IsDefaultUser(userID string) (bool, error) {
	ctx := context.Background()
	user, err := s.queries.GetUser(ctx, userID)
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
	routes, err := s.queries.ListTriggers(ctx)
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
		return nil, errors.InternalError("failed to get webhook log stats", err)
	}

	return &storage.Stats{
		TotalRequests:   int(stats.TotalCount),
		SuccessRequests: int(stats.SuccessCount),
		FailedRequests:  int(stats.ErrorCount),
		ActiveTriggers:  activeRoutes,
	}, nil
}

// GetRouteStats returns detailed statistics for a specific route
// func (s *PostgreSQLCAdapter) GetRouteStats(routeID string) (map[string]interface{}, error) {
// 	ctx := context.Background()
//
// 	stats, err := s.queries.GetRouteStatistics(ctx, &routeID)
// 	if err != nil {
// 		return nil, errors.InternalError("failed to get route statistics", err)
// 	}
//
// 	// Convert to sql.Null types for BuildStatsResult
// 	avgTransformTime := sql.NullFloat64{
// 		Float64: stats.AvgTransformationTime,
// 		Valid:   stats.AvgTransformationTime != 0, // PostgreSQL uses 0 for no data
// 	}
//
// 	avgTotalTime := sql.NullFloat64{
// 		Float64: stats.AvgTotalTime,
// 		Valid:   stats.AvgTotalTime != 0, // PostgreSQL uses 0 for no data
// 	}
//
// 	var lastProcessed sql.NullTime
// 	// LastProcessed should be a pgtype.Timestamp from PostgreSQL
// 	if stats.LastProcessed != nil {
// 		switch v := stats.LastProcessed.(type) {
// 		case pgtype.Timestamp:
// 			if v.Valid {
// 				lastProcessed = sql.NullTime{Time: v.Time, Valid: true}
// 			}
// 		case time.Time:
// 			lastProcessed = sql.NullTime{Time: v, Valid: true}
// 		}
// 	}
//
// 	return s.BuildStatsResult(
// 		routeID,
// 		stats.Name,
// 		stats.TotalRequests,
// 		stats.SuccessfulRequests,
// 		stats.FailedRequests,
// 		avgTransformTime,
// 		avgTotalTime,
// 		lastProcessed,
// 	), nil
// }

// GetDashboardStats implements Storage interface for PostgreSQL
// GetDashboardStats is implemented in postgres_adapter_dashboard.go

// GetDLQStatsForUser returns DLQ statistics for a specific user
func (s *PostgreSQLCAdapter) GetDLQStatsForUser(userID string) (*storage.DLQStats, error) {
	ctx := context.Background()

	// Get pending messages count for user
	var pendingCount int64
	pendingQuery := `
		SELECT COUNT(*) 
		FROM dlq_messages dm
		INNER JOIN routes r ON dm.route_id = r.id
		WHERE dm.status = 'pending' AND r.user_id = $1
	`
	err := s.conn.QueryRow(ctx, pendingQuery, userID).Scan(&pendingCount)
	if err != nil {
		return nil, err
	}

	// Get abandoned messages count for user
	var abandonedCount int64
	abandonedQuery := `
		SELECT COUNT(*) 
		FROM dlq_messages dm
		INNER JOIN routes r ON dm.route_id = r.id
		WHERE dm.status = 'abandoned' AND r.user_id = $1
	`
	err = s.conn.QueryRow(ctx, abandonedQuery, userID).Scan(&abandonedCount)
	if err != nil {
		return nil, err
	}

	// Get processed messages count for user (last 24 hours)
	var processedCount int64
	processedQuery := `
		SELECT COUNT(*) 
		FROM dlq_messages dm
		INNER JOIN routes r ON dm.route_id = r.id
		WHERE dm.status = 'processed' 
		AND dm.updated_at >= $1
		AND r.user_id = $2
	`
	since := time.Now().Add(-24 * time.Hour)
	err = s.conn.QueryRow(ctx, processedQuery, since, userID).Scan(&processedCount)
	if err != nil {
		return nil, err
	}

	// We could get average retry count here if needed
	// but it's not part of the current DLQStats struct

	return &storage.DLQStats{
		TotalMessages:     pendingCount + abandonedCount,
		PendingMessages:   pendingCount,
		RetryingMessages:  0, // We don't track retrying state separately
		AbandonedMessages: abandonedCount,
		OldestFailure:     nil, // Could be added if needed
	}, nil
}

// GetTriggers returns triggers matching the specified filters
func (s *PostgreSQLCAdapter) GetTriggers(filters storage.TriggerFilters) ([]*storage.Trigger, error) {
	triggers, err := s.ListTriggers()
	if err != nil {
		return nil, err
	}

	return s.ApplyTriggerFilters(triggers, filters), nil
}

// GetTriggersPaginated implements Storage interface with pagination
func (s *PostgreSQLCAdapter) GetTriggersPaginated(filters storage.TriggerFilters, limit, offset int) ([]*storage.Trigger, int, error) {
	ctx := context.Background()

	// Get total count
	countResult, err := s.queries.CountTriggers(ctx)
	if err != nil {
		return nil, 0, err
	}
	totalCount := int(countResult)

	// Get paginated triggers
	triggers, err := s.queries.ListTriggersPaginated(ctx, postgres.ListTriggersPaginatedParams{
		Limit:  int32(limit),
		Offset: int32(offset),
	})
	if err != nil {
		return nil, 0, err
	}

	// Convert and apply filters
	result := make([]*storage.Trigger, 0, len(triggers))
	for _, trigger := range triggers {
		t, err := s.triggerFromDB(trigger)
		if err != nil {
			return nil, 0, err
		}

		// Apply filters
		if filters.Type != "" && t.Type != filters.Type {
			continue
		}
		if filters.Status != "" && t.Status != filters.Status {
			continue
		}
		if filters.Active != nil && t.Active != *filters.Active {
			continue
		}

		result = append(result, t)
	}

	return result, totalCount, nil
}

func (s *PostgreSQLCAdapter) GetPipelines() ([]*storage.Pipeline, error) {
	return s.ListPipelines()
}

// GetPipelinesPaginated implements Storage interface with pagination
func (s *PostgreSQLCAdapter) GetPipelinesPaginated(limit, offset int) ([]*storage.Pipeline, int, error) {
	ctx := context.Background()

	// Get total count
	countResult, err := s.queries.CountPipelines(ctx)
	if err != nil {
		return nil, 0, err
	}
	totalCount := int(countResult)

	// Get paginated pipelines
	pipelines, err := s.queries.ListPipelinesPaginated(ctx, postgres.ListPipelinesPaginatedParams{
		Limit:  int32(limit),
		Offset: int32(offset),
	})
	if err != nil {
		return nil, 0, err
	}

	result := make([]*storage.Pipeline, len(pipelines))
	for i, pipeline := range pipelines {
		result[i], err = s.pipelineFromDB(pipeline)
		if err != nil {
			return nil, 0, err
		}
	}

	return result, totalCount, nil
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

	params := postgres.CreateDLQMessageParams{
		ID:             cuid.New(),
		MessageID:      message.MessageID,
		TriggerID:      message.TriggerID,
		PipelineID:     message.PipelineID,
		SourceBrokerID: sourceBrokerID,
		DlqBrokerID:    dlqBrokerID,
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
		return errors.InternalError("failed to create DLQ message", err)
	}

	message.ID = result.ID
	return nil
}

// GetDLQMessage implements Storage interface
func (s *PostgreSQLCAdapter) GetDLQMessage(id string) (*storage.DLQMessage, error) {
	ctx := context.Background()
	msg, err := s.queries.GetDLQMessage(ctx, id)
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

// ListDLQMessagesWithCount implements Storage interface with count
func (s *PostgreSQLCAdapter) ListDLQMessagesWithCount(limit, offset int) ([]*storage.DLQMessage, int, error) {
	ctx := context.Background()

	// Get total count
	countResult, err := s.queries.CountDLQMessages(ctx)
	if err != nil {
		return nil, 0, err
	}
	totalCount := int(countResult)

	// Get paginated messages
	messages, err := s.queries.ListDLQMessages(ctx, postgres.ListDLQMessagesParams{
		Limit:  int32(limit),
		Offset: int32(offset),
	})
	if err != nil {
		return nil, 0, err
	}

	result := make([]*storage.DLQMessage, len(messages))
	for i, msg := range messages {
		result[i], err = s.dlqMessageFromDB(msg)
		if err != nil {
			return nil, 0, err
		}
	}

	return result, totalCount, nil
}

// ListDLQMessagesByRoute implements Storage interface
// func (s *PostgreSQLCAdapter) ListDLQMessagesByRoute(routeID string, limit, offset int) ([]*storage.DLQMessage, error) {
// 	ctx := context.Background()
// 	messages, err := s.queries.ListDLQMessagesByRoute(ctx, postgres.ListDLQMessagesByRouteParams{
// 		TriggerID: routeID,
// 		Limit:   int32(limit),
// 		Offset:  int32(offset),
// 	})
// 	if err != nil {
// 		return nil, err
// 	}
//
// 	result := make([]*storage.DLQMessage, len(messages))
// 	for i, msg := range messages {
// 		result[i], err = s.dlqMessageFromDB(msg)
// 		if err != nil {
// 			return nil, err
// 		}
// 	}
//
// 	return result, nil
// }

// ListDLQMessagesByRouteWithCount implements Storage interface with count
// func (s *PostgreSQLCAdapter) ListDLQMessagesByRouteWithCount(routeID string, limit, offset int) ([]*storage.DLQMessage, int, error) {
// 	ctx := context.Background()
//
// 	// Get total count for this route
// 	countResult, err := s.queries.CountDLQMessagesByRoute(ctx, routeID)
// 	if err != nil {
// 		return nil, 0, err
// 	}
// 	totalCount := int(countResult)
//
// 	// Get paginated messages
// 	messages, err := s.queries.ListDLQMessagesByRoute(ctx, postgres.ListDLQMessagesByRouteParams{
// 		TriggerID: routeID,
// 		Limit:   int32(limit),
// 		Offset:  int32(offset),
// 	})
// 	if err != nil {
// 		return nil, 0, err
// 	}
//
// 	result := make([]*storage.DLQMessage, len(messages))
// 	for i, msg := range messages {
// 		result[i], err = s.dlqMessageFromDB(msg)
// 		if err != nil {
// 			return nil, 0, err
// 		}
// 	}
//
// 	return result, totalCount, nil
// }

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
		return errors.InternalError("failed to marshal metadata", err)
	}

	params := postgres.UpdateDLQMessageParams{
		ID:           message.ID,
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
func (s *PostgreSQLCAdapter) UpdateDLQMessageStatus(id string, status string) error {
	ctx := context.Background()
	return s.queries.UpdateDLQMessageStatus(ctx, postgres.UpdateDLQMessageStatusParams{
		ID:     id,
		Status: &status,
	})
}

// DeleteDLQMessage implements Storage interface
func (s *PostgreSQLCAdapter) DeleteDLQMessage(id string) error {
	ctx := context.Background()
	return s.queries.DeleteDLQMessage(ctx, id)
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
// func (s *PostgreSQLCAdapter) GetDLQStatsByRoute() ([]*storage.DLQRouteStats, error) {
// 	ctx := context.Background()
//
// 	// Calculate stats manually since we don't have a dedicated query
// 	allMessages, err := s.queries.ListDLQMessages(ctx, postgres.ListDLQMessagesParams{
// 		Limit:  10000,
// 		Offset: 0,
// 	})
// 	if err != nil {
// 		return nil, err
// 	}
//
// 	// Group by route ID
// 	routeStats := make(map[string]*storage.DLQRouteStats)
// 	for _, msg := range allMessages {
// 		routeID := msg.RouteID
// 		if _, exists := routeStats[routeID]; !exists {
// 			routeStats[routeID] = &storage.DLQRouteStats{
// 				TriggerID: routeID,
// 			}
// 		}
//
// 		routeStats[routeID].MessageCount++
//
// 		if msg.Status != nil {
// 			switch *msg.Status {
// 			case "pending":
// 				routeStats[routeID].PendingCount++
// 			case "abandoned":
// 				routeStats[routeID].AbandonedCount++
// 			}
// 		}
// 	}
//
// 	// Convert map to slice
// 	var result []*storage.DLQRouteStats
// 	for _, stat := range routeStats {
// 		result = append(result, stat)
// 	}
//
// 	return result, nil
// }

// GetDLQStatsByError implements Storage interface
func (s *PostgreSQLCAdapter) GetDLQStatsByError() ([]*storage.DLQErrorStats, error) {
	ctx := context.Background()

	// Calculate stats manually since we don't have a dedicated query
	allMessages, err := s.queries.ListDLQMessages(ctx, postgres.ListDLQMessagesParams{
		Limit:  10000,
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

		// Convert timestamps
		firstFailure := msg.FirstFailure.Time
		lastFailure := msg.LastFailure.Time

		if _, exists := errorStats[errorMsg]; !exists {
			errorStats[errorMsg] = &storage.DLQErrorStats{
				ErrorMessage: errorMsg,
				MessageCount: 0,
				FirstSeen:    firstFailure,
				LastSeen:     lastFailure,
			}
		}

		errorStats[errorMsg].MessageCount++

		// Update first seen if earlier
		if firstFailure.Before(errorStats[errorMsg].FirstSeen) {
			errorStats[errorMsg].FirstSeen = firstFailure
		}

		// Update last seen if later
		if lastFailure.After(errorStats[errorMsg].LastSeen) {
			errorStats[errorMsg].LastSeen = lastFailure
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

// GetTriggerStats implements Storage interface - replaces GetRouteStats
func (s *PostgreSQLCAdapter) GetTriggerStats(triggerID string) (map[string]interface{}, error) {
	ctx := context.Background()

	stats, err := s.queries.GetTriggerStatistics(ctx, &triggerID)
	if err != nil {
		return nil, errors.InternalError("failed to get trigger statistics", err)
	}

	// Convert to sql.Null types for BuildStatsResult
	var avgTransformTime sql.NullFloat64
	if stats.AvgTransformationTime != 0 {
		avgTransformTime = sql.NullFloat64{Float64: stats.AvgTransformationTime, Valid: true}
	}

	var avgTotalTime sql.NullFloat64
	if stats.AvgTotalTime != 0 {
		avgTotalTime = sql.NullFloat64{Float64: stats.AvgTotalTime, Valid: true}
	}

	var lastProcessed sql.NullTime
	if stats.LastProcessed != nil {
		if t, ok := stats.LastProcessed.(time.Time); ok {
			lastProcessed = sql.NullTime{Time: t, Valid: true}
		}
	}

	return s.BuildStatsResult(
		triggerID,
		stats.Name,
		stats.TotalRequests,
		stats.SuccessfulRequests,
		stats.FailedRequests,
		avgTransformTime,
		avgTotalTime,
		lastProcessed,
	), nil
}

// ListDLQMessagesByTrigger implements Storage interface - replaces ListDLQMessagesByRoute
func (s *PostgreSQLCAdapter) ListDLQMessagesByTrigger(triggerID string, limit, offset int) ([]*storage.DLQMessage, error) {
	ctx := context.Background()
	msgs, err := s.queries.ListDLQMessagesByTrigger(ctx, postgres.ListDLQMessagesByTriggerParams{
		TriggerID: &triggerID,
		Limit:     int32(limit),
		Offset:    int32(offset),
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

// ListDLQMessagesByTriggerWithCount implements Storage interface with count - replaces ListDLQMessagesByRouteWithCount
func (s *PostgreSQLCAdapter) ListDLQMessagesByTriggerWithCount(triggerID string, limit, offset int) ([]*storage.DLQMessage, int, error) {
	ctx := context.Background()

	// Get total count for this trigger
	countResult, err := s.queries.CountDLQMessagesByTrigger(ctx, &triggerID)
	if err != nil {
		return nil, 0, err
	}
	totalCount := int(countResult)

	// Get paginated messages
	msgs, err := s.queries.ListDLQMessagesByTrigger(ctx, postgres.ListDLQMessagesByTriggerParams{
		TriggerID: &triggerID,
		Limit:     int32(limit),
		Offset:    int32(offset),
	})
	if err != nil {
		return nil, 0, err
	}

	var result []*storage.DLQMessage
	for _, msg := range msgs {
		dlqMsg, err := s.dlqMessageFromDB(msg)
		if err != nil {
			return nil, 0, err
		}
		result = append(result, dlqMsg)
	}

	return result, totalCount, nil
}

// GetDLQStatsByTrigger implements Storage interface - replaces GetDLQStatsByRoute
func (s *PostgreSQLCAdapter) GetDLQStatsByTrigger() ([]*storage.DLQTriggerStats, error) {
	ctx := context.Background()

	stats, err := s.queries.GetDLQStatsByTrigger(ctx)
	if err != nil {
		return nil, err
	}

	var result []*storage.DLQTriggerStats
	for _, stat := range stats {
		if stat.TriggerID == nil {
			continue
		}
		result = append(result, &storage.DLQTriggerStats{
			TriggerID:      *stat.TriggerID,
			MessageCount:   stat.MessageCount,
			PendingCount:   stat.PendingCount,
			AbandonedCount: stat.AbandonedCount,
		})
	}

	return result, nil
}

// Helper method to convert database DLQ message to domain model
func (s *PostgreSQLCAdapter) dlqMessageFromDB(msg postgres.DlqMessage) (*storage.DLQMessage, error) {
	var headers map[string]string
	if err := json.Unmarshal(msg.Headers, &headers); err != nil {
		return nil, errors.InternalError("failed to unmarshal headers", err)
	}

	var metadata map[string]interface{}
	if err := json.Unmarshal(msg.Metadata, &metadata); err != nil {
		return nil, errors.InternalError("failed to unmarshal metadata", err)
	}

	result := &storage.DLQMessage{
		ID:           msg.ID,
		MessageID:    msg.MessageID,
		TriggerID:    msg.TriggerID,
		PipelineID:   msg.PipelineID,
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

// encryptSensitiveConfig encrypts sensitive fields in a configuration map
// Returns the encrypted config and any error encountered
func (s *PostgreSQLCAdapter) encryptSensitiveConfig(config map[string]interface{}) (map[string]interface{}, error) {
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
func (s *PostgreSQLCAdapter) decryptSensitiveConfig(config map[string]interface{}) (map[string]interface{}, error) {
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
func (s *PostgreSQLCAdapter) isSensitiveField(fieldName string, patterns []string) bool {
	fieldLower := strings.ToLower(fieldName)
	for _, pattern := range patterns {
		if strings.Contains(fieldLower, pattern) {
			return true
		}
	}
	return false
}

// Execution log methods for comprehensive trigger tracking

// CreateExecutionLog creates a new execution log entry
func (s *PostgreSQLCAdapter) CreateExecutionLog(log *storage.ExecutionLog) error {
	ctx := context.Background()

	query := `
		INSERT INTO execution_logs (
			id, trigger_id, trigger_type, trigger_config,
			input_method, input_endpoint, input_headers, input_body,
			pipeline_id, pipeline_stages, transformation_data, transformation_time_ms,
			broker_id, broker_type, broker_queue, broker_exchange, broker_routing_key,
			broker_publish_time_ms, broker_response, status, status_code,
			error_message, output_data, total_latency_ms, started_at, completed_at, user_id
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24, $25, $26, $27, $28)
	`

	_, err := s.conn.Exec(ctx, query,
		log.InputMethod, log.InputEndpoint, log.InputHeaders, log.InputBody,
		log.PipelineID, log.PipelineStages, log.TransformationData, log.TransformationTimeMS,
		log.BrokerID, log.BrokerType, log.BrokerQueue, log.BrokerExchange, log.BrokerRoutingKey,
		log.BrokerPublishTimeMS, log.BrokerResponse, log.Status, log.StatusCode,
		log.ErrorMessage, log.OutputData, log.TotalLatencyMS, log.StartedAt, log.CompletedAt, log.UserID,
	)
	return err
}

// UpdateExecutionLog updates an existing execution log
func (s *PostgreSQLCAdapter) UpdateExecutionLog(log *storage.ExecutionLog) error {
	ctx := context.Background()

	query := `
		UPDATE execution_logs SET
			trigger_config = $1, input_headers = $2, input_body = $3,
			pipeline_stages = $4, transformation_data = $5, transformation_time_ms = $6,
			broker_type = $7, broker_queue = $8, broker_exchange = $9, broker_routing_key = $10,
			broker_publish_time_ms = $11, broker_response = $12, status = $13, status_code = $14,
			error_message = $15, output_data = $16, total_latency_ms = $17, completed_at = $18
		WHERE id = $19
	`

	_, err := s.conn.Exec(ctx, query,
		log.TriggerConfig, log.InputHeaders, log.InputBody,
		log.PipelineStages, log.TransformationData, log.TransformationTimeMS,
		log.BrokerType, log.BrokerQueue, log.BrokerExchange, log.BrokerRoutingKey,
		log.BrokerPublishTimeMS, log.BrokerResponse, log.Status, log.StatusCode,
		log.ErrorMessage, log.OutputData, log.TotalLatencyMS, log.CompletedAt,
		log.ID,
	)
	return err
}

// GetExecutionLog retrieves a single execution log by ID
func (s *PostgreSQLCAdapter) GetExecutionLog(id string) (*storage.ExecutionLog, error) {
	ctx := context.Background()

	query := `
		SELECT id, trigger_id, trigger_type, trigger_config,
			input_method, input_endpoint, input_headers, input_body,
			pipeline_id, pipeline_stages, transformation_data, transformation_time_ms,
			broker_id, broker_type, broker_queue, broker_exchange, broker_routing_key,
			broker_publish_time_ms, broker_response, status, status_code,
			error_message, output_data, total_latency_ms, started_at, completed_at, user_id
		FROM execution_logs WHERE id = $1
	`

	var log storage.ExecutionLog
	var triggerID, routeID, pipelineID, brokerID *string
	var completedAt *time.Time

	err := s.conn.QueryRow(ctx, query, id).Scan(
		&log.ID, &triggerID, &log.TriggerType, &log.TriggerConfig, &routeID,
		&log.InputMethod, &log.InputEndpoint, &log.InputHeaders, &log.InputBody,
		&pipelineID, &log.PipelineStages, &log.TransformationData, &log.TransformationTimeMS,
		&brokerID, &log.BrokerType, &log.BrokerQueue, &log.BrokerExchange, &log.BrokerRoutingKey,
		&log.BrokerPublishTimeMS, &log.BrokerResponse, &log.Status, &log.StatusCode,
		&log.ErrorMessage, &log.OutputData, &log.TotalLatencyMS, &log.StartedAt, &completedAt, &log.UserID,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, errors.NotFoundError("execution log")
		}
		return nil, err
	}

	// Handle nullable fields
	log.TriggerID = triggerID
	log.PipelineID = pipelineID
	log.BrokerID = brokerID
	log.CompletedAt = completedAt

	return &log, nil
}

// ListExecutionLogsWithCount retrieves paginated execution logs with total count for a user
func (s *PostgreSQLCAdapter) ListExecutionLogsWithCount(userID string, limit, offset int) ([]*storage.ExecutionLog, int, error) {
	ctx := context.Background()

	// Get logs
	query := `
		SELECT id, trigger_id, trigger_type, trigger_config,
			input_method, input_endpoint, input_headers, input_body,
			pipeline_id, pipeline_stages, transformation_data, transformation_time_ms,
			broker_id, broker_type, broker_queue, broker_exchange, broker_routing_key,
			broker_publish_time_ms, broker_response, status, status_code,
			error_message, output_data, total_latency_ms, started_at, completed_at, user_id
		FROM execution_logs 
		WHERE user_id = $1
		ORDER BY started_at DESC
		LIMIT $2 OFFSET $3
	`

	rows, err := s.conn.Query(ctx, query, userID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var logs []*storage.ExecutionLog
	for rows.Next() {
		var log storage.ExecutionLog
		var triggerID, routeID, pipelineID, brokerID *string
		var completedAt *time.Time

		err := rows.Scan(
			&log.ID, &triggerID, &log.TriggerType, &log.TriggerConfig, &routeID,
			&log.InputMethod, &log.InputEndpoint, &log.InputHeaders, &log.InputBody,
			&pipelineID, &log.PipelineStages, &log.TransformationData, &log.TransformationTimeMS,
			&brokerID, &log.BrokerType, &log.BrokerQueue, &log.BrokerExchange, &log.BrokerRoutingKey,
			&log.BrokerPublishTimeMS, &log.BrokerResponse, &log.Status, &log.StatusCode,
			&log.ErrorMessage, &log.OutputData, &log.TotalLatencyMS, &log.StartedAt, &completedAt, &log.UserID,
		)
		if err != nil {
			return nil, 0, err
		}

		// Handle nullable fields
		log.TriggerID = triggerID
		log.PipelineID = pipelineID
		log.BrokerID = brokerID
		log.CompletedAt = completedAt

		logs = append(logs, &log)
	}

	// Get total count
	countQuery := `SELECT COUNT(*) FROM execution_logs WHERE user_id = $1`
	var total int
	err = s.conn.QueryRow(ctx, countQuery, userID).Scan(&total)
	if err != nil {
		return logs, 0, err
	}

	return logs, total, nil
}

// ListExecutionLogs retrieves paginated execution logs for a user
func (s *PostgreSQLCAdapter) ListExecutionLogs(userID string, limit, offset int) ([]*storage.ExecutionLog, error) {
	logs, _, err := s.ListExecutionLogsWithCount(userID, limit, offset)
	return logs, err
}

// GetExecutionLogsByTriggerID retrieves execution logs for a specific trigger
func (s *PostgreSQLCAdapter) GetExecutionLogsByTriggerID(triggerID string, userID string, limit, offset int) ([]*storage.ExecutionLog, error) {
	ctx := context.Background()

	query := `
		SELECT id, trigger_id, trigger_type, trigger_config,
			input_method, input_endpoint, input_headers, input_body,
			pipeline_id, pipeline_stages, transformation_data, transformation_time_ms,
			broker_id, broker_type, broker_queue, broker_exchange, broker_routing_key,
			broker_publish_time_ms, broker_response, status, status_code,
			error_message, output_data, total_latency_ms, started_at, completed_at, user_id
		FROM execution_logs 
		WHERE trigger_id = $1 AND user_id = $2
		ORDER BY started_at DESC
		LIMIT $3 OFFSET $4
	`

	rows, err := s.conn.Query(ctx, query, triggerID, userID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []*storage.ExecutionLog
	for rows.Next() {
		var log storage.ExecutionLog
		var triggerIDVal, routeID, pipelineID, brokerID *string
		var completedAt *time.Time

		err := rows.Scan(
			&log.ID, &triggerIDVal, &log.TriggerType, &log.TriggerConfig, &routeID,
			&log.InputMethod, &log.InputEndpoint, &log.InputHeaders, &log.InputBody,
			&pipelineID, &log.PipelineStages, &log.TransformationData, &log.TransformationTimeMS,
			&brokerID, &log.BrokerType, &log.BrokerQueue, &log.BrokerExchange, &log.BrokerRoutingKey,
			&log.BrokerPublishTimeMS, &log.BrokerResponse, &log.Status, &log.StatusCode,
			&log.ErrorMessage, &log.OutputData, &log.TotalLatencyMS, &log.StartedAt, &completedAt, &log.UserID,
		)
		if err != nil {
			return nil, err
		}

		// Handle nullable fields
		log.TriggerID = triggerIDVal
		log.PipelineID = pipelineID
		log.BrokerID = brokerID
		log.CompletedAt = completedAt

		logs = append(logs, &log)
	}

	return logs, nil
}

// GetExecutionLogsByTriggerType retrieves execution logs for a specific trigger type
func (s *PostgreSQLCAdapter) GetExecutionLogsByTriggerType(triggerType string, userID string, limit, offset int) ([]*storage.ExecutionLog, error) {
	ctx := context.Background()

	query := `
		SELECT id, trigger_id, trigger_type, trigger_config,
			input_method, input_endpoint, input_headers, input_body,
			pipeline_id, pipeline_stages, transformation_data, transformation_time_ms,
			broker_id, broker_type, broker_queue, broker_exchange, broker_routing_key,
			broker_publish_time_ms, broker_response, status, status_code,
			error_message, output_data, total_latency_ms, started_at, completed_at, user_id
		FROM execution_logs 
		WHERE trigger_type = $1 AND user_id = $2
		ORDER BY started_at DESC
		LIMIT $3 OFFSET $4
	`

	rows, err := s.conn.Query(ctx, query, triggerType, userID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []*storage.ExecutionLog
	for rows.Next() {
		var log storage.ExecutionLog
		var triggerID, routeID, pipelineID, brokerID *string
		var completedAt *time.Time

		err := rows.Scan(
			&log.ID, &triggerID, &log.TriggerType, &log.TriggerConfig, &routeID,
			&log.InputMethod, &log.InputEndpoint, &log.InputHeaders, &log.InputBody,
			&pipelineID, &log.PipelineStages, &log.TransformationData, &log.TransformationTimeMS,
			&brokerID, &log.BrokerType, &log.BrokerQueue, &log.BrokerExchange, &log.BrokerRoutingKey,
			&log.BrokerPublishTimeMS, &log.BrokerResponse, &log.Status, &log.StatusCode,
			&log.ErrorMessage, &log.OutputData, &log.TotalLatencyMS, &log.StartedAt, &completedAt, &log.UserID,
		)
		if err != nil {
			return nil, err
		}

		// Handle nullable fields
		log.TriggerID = triggerID
		log.PipelineID = pipelineID
		log.BrokerID = brokerID
		log.CompletedAt = completedAt

		logs = append(logs, &log)
	}

	return logs, nil
}

// GetExecutionLogsByStatus retrieves execution logs by status
func (s *PostgreSQLCAdapter) GetExecutionLogsByStatus(status string, userID string, limit, offset int) ([]*storage.ExecutionLog, error) {
	ctx := context.Background()

	query := `
		SELECT id, trigger_id, trigger_type, trigger_config,
			input_method, input_endpoint, input_headers, input_body,
			pipeline_id, pipeline_stages, transformation_data, transformation_time_ms,
			broker_id, broker_type, broker_queue, broker_exchange, broker_routing_key,
			broker_publish_time_ms, broker_response, status, status_code,
			error_message, output_data, total_latency_ms, started_at, completed_at, user_id
		FROM execution_logs 
		WHERE status = $1 AND user_id = $2
		ORDER BY started_at DESC
		LIMIT $3 OFFSET $4
	`

	rows, err := s.conn.Query(ctx, query, status, userID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []*storage.ExecutionLog
	for rows.Next() {
		var log storage.ExecutionLog
		var triggerID, routeID, pipelineID, brokerID *string
		var completedAt *time.Time

		err := rows.Scan(
			&log.ID, &triggerID, &log.TriggerType, &log.TriggerConfig, &routeID,
			&log.InputMethod, &log.InputEndpoint, &log.InputHeaders, &log.InputBody,
			&pipelineID, &log.PipelineStages, &log.TransformationData, &log.TransformationTimeMS,
			&brokerID, &log.BrokerType, &log.BrokerQueue, &log.BrokerExchange, &log.BrokerRoutingKey,
			&log.BrokerPublishTimeMS, &log.BrokerResponse, &log.Status, &log.StatusCode,
			&log.ErrorMessage, &log.OutputData, &log.TotalLatencyMS, &log.StartedAt, &completedAt, &log.UserID,
		)
		if err != nil {
			return nil, err
		}

		// Handle nullable fields
		log.TriggerID = triggerID
		log.PipelineID = pipelineID
		log.BrokerID = brokerID
		log.CompletedAt = completedAt

		logs = append(logs, &log)
	}

	return logs, nil
}

// OAuth2 Service methods

// CreateOAuth2Service implements Storage interface
func (s *PostgreSQLCAdapter) CreateOAuth2Service(service *storage.OAuth2Service) error {
	ctx := context.Background()

	// Generate ID if not provided
	if service.ID == "" {
		service.ID = cuid.New()
	}

	// Encrypt client secret if encryptor is available
	encryptedSecret := service.ClientSecret
	if s.encryptor != nil && service.ClientSecret != "" {
		encrypted, err := s.encryptor.Encrypt(service.ClientSecret)
		if err != nil {
			return fmt.Errorf("failed to encrypt client secret: %w", err)
		}
		encryptedSecret = encrypted
	}

	// Serialize scopes to JSON
	scopesJSON, err := json.Marshal(service.Scopes)
	if err != nil {
		return fmt.Errorf("failed to marshal scopes: %w", err)
	}

	scopesStr := string(scopesJSON)
	_, err = s.queries.CreateOAuth2Service(ctx, postgres.CreateOAuth2ServiceParams{
		ID:           service.ID,
		Name:         service.Name,
		ClientID:     service.ClientID,
		ClientSecret: encryptedSecret,
		TokenUrl:     service.TokenURL,
		AuthUrl:      utils.StringOrNil(service.AuthURL),
		RedirectUrl:  utils.StringOrNil(service.RedirectURL),
		Scopes:       utils.StringOrNil(scopesStr),
		GrantType:    "client_credentials", // Default grant type
		UserID:       service.UserID,
	})

	return err
}

// GetOAuth2Service implements Storage interface
func (s *PostgreSQLCAdapter) GetOAuth2Service(id string) (*storage.OAuth2Service, error) {
	ctx := context.Background()

	dbService, err := s.queries.GetOAuth2Service(ctx, id)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("OAuth2 service not found")
		}
		return nil, err
	}

	return s.oauth2ServiceFromDB(dbService)
}

// GetOAuth2ServiceByName implements Storage interface
func (s *PostgreSQLCAdapter) GetOAuth2ServiceByName(name string, userID string) (*storage.OAuth2Service, error) {
	ctx := context.Background()

	dbService, err := s.queries.GetOAuth2ServiceByName(ctx, postgres.GetOAuth2ServiceByNameParams{
		Name:   name,
		UserID: userID,
	})
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("OAuth2 service not found")
		}
		return nil, err
	}

	return s.oauth2ServiceFromDB(dbService)
}

// ListOAuth2Services implements Storage interface
func (s *PostgreSQLCAdapter) ListOAuth2Services(userID string) ([]*storage.OAuth2Service, error) {
	ctx := context.Background()

	dbServices, err := s.queries.ListOAuth2Services(ctx, userID)
	if err != nil {
		return nil, err
	}

	var services []*storage.OAuth2Service
	for _, dbService := range dbServices {
		service, err := s.oauth2ServiceFromDB(dbService)
		if err != nil {
			return nil, err
		}
		services = append(services, service)
	}

	return services, nil
}

// UpdateOAuth2Service implements Storage interface
func (s *PostgreSQLCAdapter) UpdateOAuth2Service(service *storage.OAuth2Service) error {
	ctx := context.Background()

	// Encrypt client secret if encryptor is available
	encryptedSecret := service.ClientSecret
	if s.encryptor != nil && service.ClientSecret != "" {
		encrypted, err := s.encryptor.Encrypt(service.ClientSecret)
		if err != nil {
			return fmt.Errorf("failed to encrypt client secret: %w", err)
		}
		encryptedSecret = encrypted
	}

	// Serialize scopes to JSON
	scopesJSON, err := json.Marshal(service.Scopes)
	if err != nil {
		return fmt.Errorf("failed to marshal scopes: %w", err)
	}

	scopesStr := string(scopesJSON)
	_, err = s.queries.UpdateOAuth2Service(ctx, postgres.UpdateOAuth2ServiceParams{
		ID:           service.ID,
		Name:         service.Name,
		ClientID:     service.ClientID,
		ClientSecret: encryptedSecret,
		TokenUrl:     service.TokenURL,
		AuthUrl:      utils.StringOrNil(service.AuthURL),
		RedirectUrl:  utils.StringOrNil(service.RedirectURL),
		Scopes:       utils.StringOrNil(scopesStr),
		GrantType:    "client_credentials", // Default grant type
	})

	return err
}

// DeleteOAuth2Service implements Storage interface
func (s *PostgreSQLCAdapter) DeleteOAuth2Service(id string) error {
	ctx := context.Background()
	return s.queries.DeleteOAuth2Service(ctx, id)
}

// oauth2ServiceFromDB converts database OAuth2 service to storage model
func (s *PostgreSQLCAdapter) oauth2ServiceFromDB(dbService postgres.Oauth2Service) (*storage.OAuth2Service, error) {
	service := &storage.OAuth2Service{
		ID:           dbService.ID,
		Name:         dbService.Name,
		ClientID:     dbService.ClientID,
		ClientSecret: dbService.ClientSecret,
		TokenURL:     dbService.TokenUrl,
		UserID:       dbService.UserID,
		CreatedAt:    dbService.CreatedAt.Time,
		UpdatedAt:    dbService.UpdatedAt.Time,
	}

	// Decrypt client secret if encryptor is available
	if s.encryptor != nil && service.ClientSecret != "" {
		decrypted, err := s.encryptor.Decrypt(service.ClientSecret)
		if err == nil {
			service.ClientSecret = decrypted
		}
		// If decryption fails, keep the original value (might be plaintext)
	}

	// Handle optional fields
	if dbService.AuthUrl != nil {
		service.AuthURL = *dbService.AuthUrl
	}
	if dbService.RedirectUrl != nil {
		service.RedirectURL = *dbService.RedirectUrl
	}
	if dbService.Scopes != nil && *dbService.Scopes != "" {
		if err := json.Unmarshal([]byte(*dbService.Scopes), &service.Scopes); err != nil {
			// Log error but don't fail - scopes are optional
			service.Scopes = []string{}
		}
	}

	return service, nil
}
