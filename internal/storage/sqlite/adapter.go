package sqlite

import (
	"database/sql"
	"encoding/json"
	"fmt"

	_ "github.com/mattn/go-sqlite3"
	"golang.org/x/crypto/bcrypt"
	"webhook-router/internal/storage"
)

type Adapter struct {
	db     *sql.DB
	config *Config
}

func NewAdapter(config *Config) (*Adapter, error) {
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid SQLite config: %w", err)
	}

	db, err := sql.Open("sqlite3", config.DatabasePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	adapter := &Adapter{
		db:     db,
		config: config,
	}

	if err := adapter.migrate(); err != nil {
		return nil, fmt.Errorf("failed to migrate database: %w", err)
	}

	// Create default user if none exists
	if err := adapter.createDefaultUser(); err != nil {
		return nil, fmt.Errorf("failed to create default user: %w", err)
	}

	return adapter, nil
}

func (a *Adapter) Connect(config storage.StorageConfig) error {
	sqliteConfig, ok := config.(*Config)
	if !ok {
		return fmt.Errorf("invalid config type for SQLite storage")
	}

	newAdapter, err := NewAdapter(sqliteConfig)
	if err != nil {
		return err
	}

	// Close existing connection
	if a.db != nil {
		a.db.Close()
	}

	a.db = newAdapter.db
	a.config = newAdapter.config

	return nil
}

func (a *Adapter) Close() error {
	if a.db != nil {
		return a.db.Close()
	}
	return nil
}

func (a *Adapter) Health() error {
	return a.db.Ping()
}

// Migration with new tables
func (a *Adapter) migrate() error {
	queries := []string{
		// Existing tables
		`CREATE TABLE IF NOT EXISTS routes (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL UNIQUE,
			endpoint TEXT NOT NULL,
			method TEXT NOT NULL DEFAULT 'POST',
			queue TEXT NOT NULL,
			exchange TEXT DEFAULT '',
			routing_key TEXT NOT NULL,
			filters TEXT DEFAULT '{}',
			headers TEXT DEFAULT '{}',
			active BOOLEAN DEFAULT 1,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			-- New Phase 1 columns
			pipeline_id INTEGER DEFAULT NULL,
			trigger_id INTEGER DEFAULT NULL,
			destination_broker_id INTEGER DEFAULT NULL,
			priority INTEGER DEFAULT 100,
			condition_expression TEXT DEFAULT ''
		)`,
		`CREATE TABLE IF NOT EXISTS webhook_logs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			route_id INTEGER,
			method TEXT NOT NULL,
			endpoint TEXT NOT NULL,
			headers TEXT,
			body TEXT,
			status_code INTEGER DEFAULT 200,
			error TEXT,
			processed_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			-- New Phase 1 columns
			trigger_id INTEGER DEFAULT NULL,
			pipeline_id INTEGER DEFAULT NULL,
			transformation_time_ms INTEGER DEFAULT 0,
			broker_publish_time_ms INTEGER DEFAULT 0,
			FOREIGN KEY (route_id) REFERENCES routes (id),
			FOREIGN KEY (trigger_id) REFERENCES triggers (id),
			FOREIGN KEY (pipeline_id) REFERENCES pipelines (id)
		)`,
		`CREATE TABLE IF NOT EXISTS users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			username TEXT NOT NULL UNIQUE,
			password_hash TEXT NOT NULL,
			is_default BOOLEAN DEFAULT 0,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS settings (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,

		// New Phase 1 tables
		`CREATE TABLE IF NOT EXISTS triggers (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL UNIQUE,
			type TEXT NOT NULL,
			config TEXT NOT NULL,
			status TEXT DEFAULT 'stopped',
			active BOOLEAN DEFAULT 1,
			error_message TEXT,
			last_execution DATETIME,
			next_execution DATETIME,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS pipelines (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL UNIQUE,
			description TEXT,
			stages TEXT NOT NULL,
			active BOOLEAN DEFAULT 1,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS broker_configs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL UNIQUE,
			type TEXT NOT NULL,
			config TEXT NOT NULL,
			active BOOLEAN DEFAULT 1,
			health_status TEXT DEFAULT 'unknown',
			last_health_check DATETIME,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,

		// Indexes
		`CREATE INDEX IF NOT EXISTS idx_routes_endpoint ON routes(endpoint)`,
		`CREATE INDEX IF NOT EXISTS idx_routes_active ON routes(active)`,
		`CREATE INDEX IF NOT EXISTS idx_routes_priority ON routes(priority)`,
		`CREATE INDEX IF NOT EXISTS idx_webhook_logs_route_id ON webhook_logs(route_id)`,
		`CREATE INDEX IF NOT EXISTS idx_webhook_logs_processed_at ON webhook_logs(processed_at)`,
		`CREATE INDEX IF NOT EXISTS idx_triggers_type ON triggers(type)`,
		`CREATE INDEX IF NOT EXISTS idx_triggers_status ON triggers(status)`,
		`CREATE INDEX IF NOT EXISTS idx_triggers_active ON triggers(active)`,
		`CREATE INDEX IF NOT EXISTS idx_pipelines_active ON pipelines(active)`,
		`CREATE INDEX IF NOT EXISTS idx_broker_configs_type ON broker_configs(type)`,
		`CREATE INDEX IF NOT EXISTS idx_broker_configs_active ON broker_configs(active)`,
	}

	for _, query := range queries {
		if _, err := a.db.Exec(query); err != nil {
			return fmt.Errorf("failed to execute migration query: %w", err)
		}
	}

	return nil
}

func (a *Adapter) createDefaultUser() error {
	var count int
	err := a.db.QueryRow("SELECT COUNT(*) FROM users").Scan(&count)
	if err != nil {
		return err
	}

	if count == 0 {
		// Create default user with username "admin" and password "admin"
		hashedPassword, err := bcrypt.GenerateFromPassword([]byte("admin"), bcrypt.DefaultCost)
		if err != nil {
			return fmt.Errorf("failed to hash default password: %w", err)
		}

		_, err = a.db.Exec(`INSERT INTO users (username, password_hash, is_default) VALUES (?, ?, ?)`,
			"admin", string(hashedPassword), true)
		if err != nil {
			return fmt.Errorf("failed to create default user: %w", err)
		}
	}

	return nil
}

// Route methods (existing functionality)
func (a *Adapter) CreateRoute(route *storage.Route) error {
	query := `INSERT INTO routes (name, endpoint, method, queue, exchange, routing_key, filters, headers, active, pipeline_id, trigger_id, destination_broker_id, priority, condition_expression)
			  VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	result, err := a.db.Exec(query, route.Name, route.Endpoint, route.Method, route.Queue,
		route.Exchange, route.RoutingKey, route.Filters, route.Headers, route.Active,
		route.PipelineID, route.TriggerID, route.DestinationBrokerID, route.Priority, route.ConditionExpression)
	if err != nil {
		return fmt.Errorf("failed to create route: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("failed to get last insert id: %w", err)
	}

	route.ID = int(id)
	return nil
}

func (a *Adapter) GetRoute(id int) (*storage.Route, error) {
	query := `SELECT id, name, endpoint, method, queue, exchange, routing_key, filters, headers, active, created_at, updated_at,
			  pipeline_id, trigger_id, destination_broker_id, priority, condition_expression
			  FROM routes WHERE id = ?`

	route := &storage.Route{}
	err := a.db.QueryRow(query, id).Scan(&route.ID, &route.Name, &route.Endpoint, &route.Method,
		&route.Queue, &route.Exchange, &route.RoutingKey, &route.Filters, &route.Headers,
		&route.Active, &route.CreatedAt, &route.UpdatedAt,
		&route.PipelineID, &route.TriggerID, &route.DestinationBrokerID, &route.Priority, &route.ConditionExpression)

	if err != nil {
		return nil, fmt.Errorf("failed to get route: %w", err)
	}

	return route, nil
}

func (a *Adapter) GetRoutes() ([]*storage.Route, error) {
	query := `SELECT id, name, endpoint, method, queue, exchange, routing_key, filters, headers, active, created_at, updated_at,
			  pipeline_id, trigger_id, destination_broker_id, priority, condition_expression
			  FROM routes ORDER BY priority ASC, created_at DESC`

	rows, err := a.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to get routes: %w", err)
	}
	defer rows.Close()

	var routes []*storage.Route
	for rows.Next() {
		route := &storage.Route{}
		err := rows.Scan(&route.ID, &route.Name, &route.Endpoint, &route.Method,
			&route.Queue, &route.Exchange, &route.RoutingKey, &route.Filters, &route.Headers,
			&route.Active, &route.CreatedAt, &route.UpdatedAt,
			&route.PipelineID, &route.TriggerID, &route.DestinationBrokerID, &route.Priority, &route.ConditionExpression)
		if err != nil {
			return nil, fmt.Errorf("failed to scan route: %w", err)
		}
		routes = append(routes, route)
	}

	return routes, nil
}

func (a *Adapter) UpdateRoute(route *storage.Route) error {
	query := `UPDATE routes SET name = ?, endpoint = ?, method = ?, queue = ?, exchange = ?, 
			  routing_key = ?, filters = ?, headers = ?, active = ?, pipeline_id = ?, trigger_id = ?,
			  destination_broker_id = ?, priority = ?, condition_expression = ?, updated_at = CURRENT_TIMESTAMP
			  WHERE id = ?`

	_, err := a.db.Exec(query, route.Name, route.Endpoint, route.Method, route.Queue,
		route.Exchange, route.RoutingKey, route.Filters, route.Headers, route.Active,
		route.PipelineID, route.TriggerID, route.DestinationBrokerID, route.Priority, route.ConditionExpression, route.ID)

	if err != nil {
		return fmt.Errorf("failed to update route: %w", err)
	}

	return nil
}

func (a *Adapter) DeleteRoute(id int) error {
	query := `DELETE FROM routes WHERE id = ?`
	_, err := a.db.Exec(query, id)
	if err != nil {
		return fmt.Errorf("failed to delete route: %w", err)
	}
	return nil
}

func (a *Adapter) FindMatchingRoutes(endpoint, method string) ([]*storage.Route, error) {
	query := `SELECT id, name, endpoint, method, queue, exchange, routing_key, filters, headers, active, created_at, updated_at,
			  pipeline_id, trigger_id, destination_broker_id, priority, condition_expression
			  FROM routes WHERE active = 1 AND (endpoint = ? OR endpoint = '*') AND (method = ? OR method = '*')
			  ORDER BY priority ASC, endpoint DESC, method DESC`

	rows, err := a.db.Query(query, endpoint, method)
	if err != nil {
		return nil, fmt.Errorf("failed to find matching routes: %w", err)
	}
	defer rows.Close()

	var routes []*storage.Route
	for rows.Next() {
		route := &storage.Route{}
		err := rows.Scan(&route.ID, &route.Name, &route.Endpoint, &route.Method,
			&route.Queue, &route.Exchange, &route.RoutingKey, &route.Filters, &route.Headers,
			&route.Active, &route.CreatedAt, &route.UpdatedAt,
			&route.PipelineID, &route.TriggerID, &route.DestinationBrokerID, &route.Priority, &route.ConditionExpression)
		if err != nil {
			return nil, fmt.Errorf("failed to scan route: %w", err)
		}
		routes = append(routes, route)
	}

	return routes, nil
}

// User methods (existing functionality)
func (a *Adapter) ValidateUser(username, password string) (*storage.User, error) {
	user := &storage.User{}
	err := a.db.QueryRow(`SELECT id, username, password_hash, is_default, created_at, updated_at 
						FROM users WHERE username = ?`, username).Scan(
		&user.ID, &user.Username, &user.PasswordHash, &user.IsDefault, &user.CreatedAt, &user.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("user not found: %w", err)
	}

	err = bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password))
	if err != nil {
		return nil, fmt.Errorf("invalid password")
	}

	return user, nil
}

func (a *Adapter) UpdateUserCredentials(userID int, username, password string) error {
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}

	_, err = a.db.Exec(`UPDATE users SET username = ?, password_hash = ?, is_default = 0, updated_at = CURRENT_TIMESTAMP 
					 WHERE id = ?`, username, string(hashedPassword), userID)
	if err != nil {
		return fmt.Errorf("failed to update user credentials: %w", err)
	}

	return nil
}

func (a *Adapter) IsDefaultUser(userID int) (bool, error) {
	var isDefault bool
	err := a.db.QueryRow("SELECT is_default FROM users WHERE id = ?", userID).Scan(&isDefault)
	return isDefault, err
}

// Settings methods (existing functionality)
func (a *Adapter) GetSetting(key string) (string, error) {
	var value string
	err := a.db.QueryRow("SELECT value FROM settings WHERE key = ?", key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", nil // Setting doesn't exist
	}
	return value, err
}

func (a *Adapter) SetSetting(key, value string) error {
	_, err := a.db.Exec(`INSERT OR REPLACE INTO settings (key, value, updated_at) 
					  VALUES (?, ?, CURRENT_TIMESTAMP)`, key, value)
	return err
}

func (a *Adapter) GetAllSettings() (map[string]string, error) {
	rows, err := a.db.Query("SELECT key, value FROM settings")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	settings := make(map[string]string)
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return nil, err
		}
		settings[key] = value
	}

	return settings, nil
}

// Webhook log methods (existing functionality)
func (a *Adapter) LogWebhook(log *storage.WebhookLog) error {
	query := `INSERT INTO webhook_logs (route_id, method, endpoint, headers, body, status_code, error, trigger_id, pipeline_id, transformation_time_ms, broker_publish_time_ms)
			  VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	_, err := a.db.Exec(query, log.RouteID, log.Method, log.Endpoint, log.Headers, log.Body, log.StatusCode, log.Error,
		log.TriggerID, log.PipelineID, log.TransformationTimeMS, log.BrokerPublishTimeMS)
	if err != nil {
		return fmt.Errorf("failed to log webhook: %w", err)
	}

	return nil
}

func (a *Adapter) GetStats() (*storage.Stats, error) {
	stats := &storage.Stats{}

	// Total requests
	err := a.db.QueryRow("SELECT COUNT(*) FROM webhook_logs").Scan(&stats.TotalRequests)
	if err != nil {
		return nil, fmt.Errorf("failed to get total requests: %w", err)
	}

	// Success requests
	err = a.db.QueryRow("SELECT COUNT(*) FROM webhook_logs WHERE status_code = 200").Scan(&stats.SuccessRequests)
	if err != nil {
		return nil, fmt.Errorf("failed to get success requests: %w", err)
	}

	// Failed requests
	err = a.db.QueryRow("SELECT COUNT(*) FROM webhook_logs WHERE status_code != 200").Scan(&stats.FailedRequests)
	if err != nil {
		return nil, fmt.Errorf("failed to get failed requests: %w", err)
	}

	// Active routes
	err = a.db.QueryRow("SELECT COUNT(*) FROM routes WHERE active = 1").Scan(&stats.ActiveRoutes)
	if err != nil {
		return nil, fmt.Errorf("failed to get active routes: %w", err)
	}

	return stats, nil
}

func (a *Adapter) GetRouteStats(routeID int) (map[string]interface{}, error) {
	stats := make(map[string]interface{})

	// Total requests for this route
	var totalRequests int
	err := a.db.QueryRow("SELECT COUNT(*) FROM webhook_logs WHERE route_id = ?", routeID).Scan(&totalRequests)
	if err != nil {
		return nil, fmt.Errorf("failed to get route total requests: %w", err)
	}
	stats["total_requests"] = totalRequests

	// Success requests for this route
	var successRequests int
	err = a.db.QueryRow("SELECT COUNT(*) FROM webhook_logs WHERE route_id = ? AND status_code = 200", routeID).Scan(&successRequests)
	if err != nil {
		return nil, fmt.Errorf("failed to get route success requests: %w", err)
	}
	stats["success_requests"] = successRequests

	// Failed requests for this route
	var failedRequests int
	err = a.db.QueryRow("SELECT COUNT(*) FROM webhook_logs WHERE route_id = ? AND status_code != 200", routeID).Scan(&failedRequests)
	if err != nil {
		return nil, fmt.Errorf("failed to get route failed requests: %w", err)
	}
	stats["failed_requests"] = failedRequests

	// Recent requests (last 24 hours)
	var recentRequests int
	err = a.db.QueryRow("SELECT COUNT(*) FROM webhook_logs WHERE route_id = ? AND processed_at > datetime('now', '-24 hours')", routeID).Scan(&recentRequests)
	if err != nil {
		return nil, fmt.Errorf("failed to get route recent requests: %w", err)
	}
	stats["recent_requests"] = recentRequests

	return stats, nil
}

// New Phase 1 methods - Triggers
func (a *Adapter) CreateTrigger(trigger *storage.Trigger) error {
	configJSON, err := json.Marshal(trigger.Config)
	if err != nil {
		return fmt.Errorf("failed to marshal trigger config: %w", err)
	}

	query := `INSERT INTO triggers (name, type, config, status, active, error_message, last_execution, next_execution)
			  VALUES (?, ?, ?, ?, ?, ?, ?, ?)`

	result, err := a.db.Exec(query, trigger.Name, trigger.Type, configJSON, trigger.Status, trigger.Active,
		trigger.ErrorMessage, trigger.LastExecution, trigger.NextExecution)
	if err != nil {
		return fmt.Errorf("failed to create trigger: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("failed to get last insert id: %w", err)
	}

	trigger.ID = int(id)
	return nil
}

func (a *Adapter) GetTrigger(id int) (*storage.Trigger, error) {
	query := `SELECT id, name, type, config, status, active, error_message, last_execution, next_execution, created_at, updated_at
			  FROM triggers WHERE id = ?`

	trigger := &storage.Trigger{}
	var configJSON string
	err := a.db.QueryRow(query, id).Scan(&trigger.ID, &trigger.Name, &trigger.Type, &configJSON,
		&trigger.Status, &trigger.Active, &trigger.ErrorMessage, &trigger.LastExecution, &trigger.NextExecution,
		&trigger.CreatedAt, &trigger.UpdatedAt)

	if err != nil {
		return nil, fmt.Errorf("failed to get trigger: %w", err)
	}

	if err := json.Unmarshal([]byte(configJSON), &trigger.Config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal trigger config: %w", err)
	}

	return trigger, nil
}

func (a *Adapter) GetTriggers(filters storage.TriggerFilters) ([]*storage.Trigger, error) {
	query := `SELECT id, name, type, config, status, active, error_message, last_execution, next_execution, created_at, updated_at
			  FROM triggers WHERE 1=1`
	args := []interface{}{}

	if filters.Type != "" {
		query += " AND type = ?"
		args = append(args, filters.Type)
	}
	if filters.Status != "" {
		query += " AND status = ?"
		args = append(args, filters.Status)
	}
	if filters.Active != nil {
		query += " AND active = ?"
		args = append(args, *filters.Active)
	}

	query += " ORDER BY created_at DESC"

	rows, err := a.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to get triggers: %w", err)
	}
	defer rows.Close()

	var triggers []*storage.Trigger
	for rows.Next() {
		trigger := &storage.Trigger{}
		var configJSON string
		err := rows.Scan(&trigger.ID, &trigger.Name, &trigger.Type, &configJSON,
			&trigger.Status, &trigger.Active, &trigger.ErrorMessage, &trigger.LastExecution, &trigger.NextExecution,
			&trigger.CreatedAt, &trigger.UpdatedAt)
		if err != nil {
			return nil, fmt.Errorf("failed to scan trigger: %w", err)
		}

		if err := json.Unmarshal([]byte(configJSON), &trigger.Config); err != nil {
			return nil, fmt.Errorf("failed to unmarshal trigger config: %w", err)
		}

		triggers = append(triggers, trigger)
	}

	return triggers, nil
}

func (a *Adapter) UpdateTrigger(trigger *storage.Trigger) error {
	configJSON, err := json.Marshal(trigger.Config)
	if err != nil {
		return fmt.Errorf("failed to marshal trigger config: %w", err)
	}

	query := `UPDATE triggers SET name = ?, type = ?, config = ?, status = ?, active = ?, error_message = ?,
			  last_execution = ?, next_execution = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`

	_, err = a.db.Exec(query, trigger.Name, trigger.Type, configJSON, trigger.Status, trigger.Active,
		trigger.ErrorMessage, trigger.LastExecution, trigger.NextExecution, trigger.ID)

	if err != nil {
		return fmt.Errorf("failed to update trigger: %w", err)
	}

	return nil
}

func (a *Adapter) DeleteTrigger(id int) error {
	query := `DELETE FROM triggers WHERE id = ?`
	_, err := a.db.Exec(query, id)
	if err != nil {
		return fmt.Errorf("failed to delete trigger: %w", err)
	}
	return nil
}

// Pipeline methods
func (a *Adapter) CreatePipeline(pipeline *storage.Pipeline) error {
	stagesJSON, err := json.Marshal(pipeline.Stages)
	if err != nil {
		return fmt.Errorf("failed to marshal pipeline stages: %w", err)
	}

	query := `INSERT INTO pipelines (name, description, stages, active) VALUES (?, ?, ?, ?)`

	result, err := a.db.Exec(query, pipeline.Name, pipeline.Description, stagesJSON, pipeline.Active)
	if err != nil {
		return fmt.Errorf("failed to create pipeline: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("failed to get last insert id: %w", err)
	}

	pipeline.ID = int(id)
	return nil
}

func (a *Adapter) GetPipeline(id int) (*storage.Pipeline, error) {
	query := `SELECT id, name, description, stages, active, created_at, updated_at FROM pipelines WHERE id = ?`

	pipeline := &storage.Pipeline{}
	var stagesJSON string
	err := a.db.QueryRow(query, id).Scan(&pipeline.ID, &pipeline.Name, &pipeline.Description,
		&stagesJSON, &pipeline.Active, &pipeline.CreatedAt, &pipeline.UpdatedAt)

	if err != nil {
		return nil, fmt.Errorf("failed to get pipeline: %w", err)
	}

	if err := json.Unmarshal([]byte(stagesJSON), &pipeline.Stages); err != nil {
		return nil, fmt.Errorf("failed to unmarshal pipeline stages: %w", err)
	}

	return pipeline, nil
}

func (a *Adapter) GetPipelines() ([]*storage.Pipeline, error) {
	query := `SELECT id, name, description, stages, active, created_at, updated_at FROM pipelines ORDER BY created_at DESC`

	rows, err := a.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to get pipelines: %w", err)
	}
	defer rows.Close()

	var pipelines []*storage.Pipeline
	for rows.Next() {
		pipeline := &storage.Pipeline{}
		var stagesJSON string
		err := rows.Scan(&pipeline.ID, &pipeline.Name, &pipeline.Description,
			&stagesJSON, &pipeline.Active, &pipeline.CreatedAt, &pipeline.UpdatedAt)
		if err != nil {
			return nil, fmt.Errorf("failed to scan pipeline: %w", err)
		}

		if err := json.Unmarshal([]byte(stagesJSON), &pipeline.Stages); err != nil {
			return nil, fmt.Errorf("failed to unmarshal pipeline stages: %w", err)
		}

		pipelines = append(pipelines, pipeline)
	}

	return pipelines, nil
}

func (a *Adapter) UpdatePipeline(pipeline *storage.Pipeline) error {
	stagesJSON, err := json.Marshal(pipeline.Stages)
	if err != nil {
		return fmt.Errorf("failed to marshal pipeline stages: %w", err)
	}

	query := `UPDATE pipelines SET name = ?, description = ?, stages = ?, active = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`

	_, err = a.db.Exec(query, pipeline.Name, pipeline.Description, stagesJSON, pipeline.Active, pipeline.ID)
	if err != nil {
		return fmt.Errorf("failed to update pipeline: %w", err)
	}

	return nil
}

func (a *Adapter) DeletePipeline(id int) error {
	query := `DELETE FROM pipelines WHERE id = ?`
	_, err := a.db.Exec(query, id)
	if err != nil {
		return fmt.Errorf("failed to delete pipeline: %w", err)
	}
	return nil
}

// Broker methods
func (a *Adapter) CreateBroker(broker *storage.BrokerConfig) error {
	configJSON, err := json.Marshal(broker.Config)
	if err != nil {
		return fmt.Errorf("failed to marshal broker config: %w", err)
	}

	query := `INSERT INTO broker_configs (name, type, config, active, health_status, last_health_check) VALUES (?, ?, ?, ?, ?, ?)`

	result, err := a.db.Exec(query, broker.Name, broker.Type, configJSON, broker.Active, broker.HealthStatus, broker.LastHealthCheck)
	if err != nil {
		return fmt.Errorf("failed to create broker: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("failed to get last insert id: %w", err)
	}

	broker.ID = int(id)
	return nil
}

func (a *Adapter) GetBroker(id int) (*storage.BrokerConfig, error) {
	query := `SELECT id, name, type, config, active, health_status, last_health_check, created_at, updated_at FROM broker_configs WHERE id = ?`

	broker := &storage.BrokerConfig{}
	var configJSON string
	err := a.db.QueryRow(query, id).Scan(&broker.ID, &broker.Name, &broker.Type, &configJSON,
		&broker.Active, &broker.HealthStatus, &broker.LastHealthCheck, &broker.CreatedAt, &broker.UpdatedAt)

	if err != nil {
		return nil, fmt.Errorf("failed to get broker: %w", err)
	}

	if err := json.Unmarshal([]byte(configJSON), &broker.Config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal broker config: %w", err)
	}

	return broker, nil
}

func (a *Adapter) GetBrokers() ([]*storage.BrokerConfig, error) {
	query := `SELECT id, name, type, config, active, health_status, last_health_check, created_at, updated_at FROM broker_configs ORDER BY created_at DESC`

	rows, err := a.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to get brokers: %w", err)
	}
	defer rows.Close()

	var brokers []*storage.BrokerConfig
	for rows.Next() {
		broker := &storage.BrokerConfig{}
		var configJSON string
		err := rows.Scan(&broker.ID, &broker.Name, &broker.Type, &configJSON,
			&broker.Active, &broker.HealthStatus, &broker.LastHealthCheck, &broker.CreatedAt, &broker.UpdatedAt)
		if err != nil {
			return nil, fmt.Errorf("failed to scan broker: %w", err)
		}

		if err := json.Unmarshal([]byte(configJSON), &broker.Config); err != nil {
			return nil, fmt.Errorf("failed to unmarshal broker config: %w", err)
		}

		brokers = append(brokers, broker)
	}

	return brokers, nil
}

func (a *Adapter) UpdateBroker(broker *storage.BrokerConfig) error {
	configJSON, err := json.Marshal(broker.Config)
	if err != nil {
		return fmt.Errorf("failed to marshal broker config: %w", err)
	}

	query := `UPDATE broker_configs SET name = ?, type = ?, config = ?, active = ?, health_status = ?, 
			  last_health_check = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`

	_, err = a.db.Exec(query, broker.Name, broker.Type, configJSON, broker.Active, broker.HealthStatus,
		broker.LastHealthCheck, broker.ID)
	if err != nil {
		return fmt.Errorf("failed to update broker: %w", err)
	}

	return nil
}

func (a *Adapter) DeleteBroker(id int) error {
	query := `DELETE FROM broker_configs WHERE id = ?`
	_, err := a.db.Exec(query, id)
	if err != nil {
		return fmt.Errorf("failed to delete broker: %w", err)
	}
	return nil
}

// Generic operations
func (a *Adapter) Query(query string, args ...interface{}) ([]map[string]interface{}, error) {
	rows, err := a.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to execute query: %w", err)
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("failed to get columns: %w", err)
	}

	var results []map[string]interface{}
	for rows.Next() {
		values := make([]interface{}, len(columns))
		valuePtrs := make([]interface{}, len(columns))
		for i := range values {
			valuePtrs[i] = &values[i]
		}

		if err := rows.Scan(valuePtrs...); err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}

		row := make(map[string]interface{})
		for i, col := range columns {
			row[col] = values[i]
		}
		results = append(results, row)
	}

	return results, nil
}

func (a *Adapter) Transaction(fn func(tx storage.Transaction) error) error {
	tx, err := a.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}

	sqliteTx := &sqliteTransaction{tx: tx}

	if err := fn(sqliteTx); err != nil {
		sqliteTx.Rollback()
		return err
	}

	return sqliteTx.Commit()
}

type sqliteTransaction struct {
	tx *sql.Tx
}

func (t *sqliteTransaction) Commit() error {
	return t.tx.Commit()
}

func (t *sqliteTransaction) Rollback() error {
	return t.tx.Rollback()
}
