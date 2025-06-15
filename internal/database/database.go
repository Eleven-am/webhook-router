package database

import (
	"database/sql"
	"fmt"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"golang.org/x/crypto/bcrypt"
)

type DB struct {
	*sql.DB
}

type Route struct {
	ID         int       `json:"id"`
	Name       string    `json:"name"`
	Endpoint   string    `json:"endpoint"`
	Method     string    `json:"method"`
	Queue      string    `json:"queue"`
	Exchange   string    `json:"exchange"`
	RoutingKey string    `json:"routing_key"`
	Filters    string    `json:"filters"` // JSON string of filter conditions
	Headers    string    `json:"headers"` // JSON string of additional headers
	Active     bool      `json:"active"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type WebhookLog struct {
	ID          int       `json:"id"`
	RouteID     int       `json:"route_id"`
	Method      string    `json:"method"`
	Endpoint    string    `json:"endpoint"`
	Headers     string    `json:"headers"`
	Body        string    `json:"body"`
	StatusCode  int       `json:"status_code"`
	Error       string    `json:"error,omitempty"`
	ProcessedAt time.Time `json:"processed_at"`
}

type User struct {
	ID           int       `json:"id"`
	Username     string    `json:"username"`
	PasswordHash string    `json:"-"` // Don't expose in JSON
	IsDefault    bool      `json:"is_default"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type Setting struct {
	Key       string    `json:"key"`
	Value     string    `json:"value"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Stats struct {
	TotalRequests   int `json:"total_requests"`
	SuccessRequests int `json:"success_requests"`
	FailedRequests  int `json:"failed_requests"`
	ActiveRoutes    int `json:"active_routes"`
}

func Init(dbPath string) (*DB, error) {
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	dbWrapper := &DB{db}
	if err := dbWrapper.migrate(); err != nil {
		return nil, fmt.Errorf("failed to migrate database: %w", err)
	}

	// Create default user if none exists
	if err := dbWrapper.createDefaultUser(); err != nil {
		return nil, fmt.Errorf("failed to create default user: %w", err)
	}

	return dbWrapper, nil
}

func (db *DB) migrate() error {
	queries := []string{
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
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
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
			FOREIGN KEY (route_id) REFERENCES routes (id)
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
		`CREATE INDEX IF NOT EXISTS idx_routes_endpoint ON routes(endpoint)`,
		`CREATE INDEX IF NOT EXISTS idx_routes_active ON routes(active)`,
		`CREATE INDEX IF NOT EXISTS idx_webhook_logs_route_id ON webhook_logs(route_id)`,
		`CREATE INDEX IF NOT EXISTS idx_webhook_logs_processed_at ON webhook_logs(processed_at)`,
	}

	for _, query := range queries {
		if _, err := db.Exec(query); err != nil {
			return fmt.Errorf("failed to execute migration query: %w", err)
		}
	}

	return nil
}

func (db *DB) createDefaultUser() error {
	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM users").Scan(&count)
	if err != nil {
		return err
	}

	if count == 0 {
		// Create default user with username "admin" and password "admin"
		hashedPassword, err := bcrypt.GenerateFromPassword([]byte("admin"), bcrypt.DefaultCost)
		if err != nil {
			return fmt.Errorf("failed to hash default password: %w", err)
		}

		_, err = db.Exec(`INSERT INTO users (username, password_hash, is_default) VALUES (?, ?, ?)`,
			"admin", string(hashedPassword), true)
		if err != nil {
			return fmt.Errorf("failed to create default user: %w", err)
		}
	}

	return nil
}

// Auth methods
func (db *DB) ValidateUser(username, password string) (*User, error) {
	user := &User{}
	err := db.QueryRow(`SELECT id, username, password_hash, is_default, created_at, updated_at 
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

func (db *DB) UpdateUserCredentials(userID int, username, password string) error {
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}

	_, err = db.Exec(`UPDATE users SET username = ?, password_hash = ?, is_default = 0, updated_at = CURRENT_TIMESTAMP 
					 WHERE id = ?`, username, string(hashedPassword), userID)
	if err != nil {
		return fmt.Errorf("failed to update user credentials: %w", err)
	}

	return nil
}

func (db *DB) IsDefaultUser(userID int) (bool, error) {
	var isDefault bool
	err := db.QueryRow("SELECT is_default FROM users WHERE id = ?", userID).Scan(&isDefault)
	return isDefault, err
}

// Settings methods
func (db *DB) GetSetting(key string) (string, error) {
	var value string
	err := db.QueryRow("SELECT value FROM settings WHERE key = ?", key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", nil // Setting doesn't exist
	}
	return value, err
}

func (db *DB) SetSetting(key, value string) error {
	_, err := db.Exec(`INSERT OR REPLACE INTO settings (key, value, updated_at) 
					  VALUES (?, ?, CURRENT_TIMESTAMP)`, key, value)
	return err
}

func (db *DB) GetAllSettings() (map[string]string, error) {
	rows, err := db.Query("SELECT key, value FROM settings")
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

// Existing route methods
func (db *DB) CreateRoute(route *Route) error {
	query := `INSERT INTO routes (name, endpoint, method, queue, exchange, routing_key, filters, headers, active)
			  VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`

	result, err := db.Exec(query, route.Name, route.Endpoint, route.Method, route.Queue,
		route.Exchange, route.RoutingKey, route.Filters, route.Headers, route.Active)
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

func (db *DB) GetRoute(id int) (*Route, error) {
	query := `SELECT id, name, endpoint, method, queue, exchange, routing_key, filters, headers, active, created_at, updated_at
			  FROM routes WHERE id = ?`

	route := &Route{}
	err := db.QueryRow(query, id).Scan(&route.ID, &route.Name, &route.Endpoint, &route.Method,
		&route.Queue, &route.Exchange, &route.RoutingKey, &route.Filters, &route.Headers,
		&route.Active, &route.CreatedAt, &route.UpdatedAt)

	if err != nil {
		return nil, fmt.Errorf("failed to get route: %w", err)
	}

	return route, nil
}

func (db *DB) GetRoutes() ([]*Route, error) {
	query := `SELECT id, name, endpoint, method, queue, exchange, routing_key, filters, headers, active, created_at, updated_at
			  FROM routes ORDER BY created_at DESC`

	rows, err := db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to get routes: %w", err)
	}
	defer rows.Close()

	var routes []*Route
	for rows.Next() {
		route := &Route{}
		err := rows.Scan(&route.ID, &route.Name, &route.Endpoint, &route.Method,
			&route.Queue, &route.Exchange, &route.RoutingKey, &route.Filters, &route.Headers,
			&route.Active, &route.CreatedAt, &route.UpdatedAt)
		if err != nil {
			return nil, fmt.Errorf("failed to scan route: %w", err)
		}
		routes = append(routes, route)
	}

	return routes, nil
}

func (db *DB) UpdateRoute(route *Route) error {
	query := `UPDATE routes SET name = ?, endpoint = ?, method = ?, queue = ?, exchange = ?, 
			  routing_key = ?, filters = ?, headers = ?, active = ?, updated_at = CURRENT_TIMESTAMP
			  WHERE id = ?`

	_, err := db.Exec(query, route.Name, route.Endpoint, route.Method, route.Queue,
		route.Exchange, route.RoutingKey, route.Filters, route.Headers, route.Active, route.ID)

	if err != nil {
		return fmt.Errorf("failed to update route: %w", err)
	}

	return nil
}

func (db *DB) DeleteRoute(id int) error {
	query := `DELETE FROM routes WHERE id = ?`
	_, err := db.Exec(query, id)
	if err != nil {
		return fmt.Errorf("failed to delete route: %w", err)
	}
	return nil
}

func (db *DB) FindMatchingRoutes(endpoint, method string) ([]*Route, error) {
	query := `SELECT id, name, endpoint, method, queue, exchange, routing_key, filters, headers, active, created_at, updated_at
			  FROM routes WHERE active = 1 AND (endpoint = ? OR endpoint = '*') AND (method = ? OR method = '*')
			  ORDER BY endpoint DESC, method DESC`

	rows, err := db.Query(query, endpoint, method)
	if err != nil {
		return nil, fmt.Errorf("failed to find matching routes: %w", err)
	}
	defer rows.Close()

	var routes []*Route
	for rows.Next() {
		route := &Route{}
		err := rows.Scan(&route.ID, &route.Name, &route.Endpoint, &route.Method,
			&route.Queue, &route.Exchange, &route.RoutingKey, &route.Filters, &route.Headers,
			&route.Active, &route.CreatedAt, &route.UpdatedAt)
		if err != nil {
			return nil, fmt.Errorf("failed to scan route: %w", err)
		}
		routes = append(routes, route)
	}

	return routes, nil
}

func (db *DB) LogWebhook(log *WebhookLog) error {
	query := `INSERT INTO webhook_logs (route_id, method, endpoint, headers, body, status_code, error)
			  VALUES (?, ?, ?, ?, ?, ?, ?)`

	_, err := db.Exec(query, log.RouteID, log.Method, log.Endpoint, log.Headers, log.Body, log.StatusCode, log.Error)
	if err != nil {
		return fmt.Errorf("failed to log webhook: %w", err)
	}

	return nil
}

func (db *DB) GetStats() (*Stats, error) {
	stats := &Stats{}

	// Total requests
	err := db.QueryRow("SELECT COUNT(*) FROM webhook_logs").Scan(&stats.TotalRequests)
	if err != nil {
		return nil, fmt.Errorf("failed to get total requests: %w", err)
	}

	// Success requests
	err = db.QueryRow("SELECT COUNT(*) FROM webhook_logs WHERE status_code = 200").Scan(&stats.SuccessRequests)
	if err != nil {
		return nil, fmt.Errorf("failed to get success requests: %w", err)
	}

	// Failed requests
	err = db.QueryRow("SELECT COUNT(*) FROM webhook_logs WHERE status_code != 200").Scan(&stats.FailedRequests)
	if err != nil {
		return nil, fmt.Errorf("failed to get failed requests: %w", err)
	}

	// Active routes
	err = db.QueryRow("SELECT COUNT(*) FROM routes WHERE active = 1").Scan(&stats.ActiveRoutes)
	if err != nil {
		return nil, fmt.Errorf("failed to get active routes: %w", err)
	}

	return stats, nil
}

func (db *DB) GetRouteStats(routeID int) (map[string]interface{}, error) {
	stats := make(map[string]interface{})

	// Total requests for this route
	var totalRequests int
	err := db.QueryRow("SELECT COUNT(*) FROM webhook_logs WHERE route_id = ?", routeID).Scan(&totalRequests)
	if err != nil {
		return nil, fmt.Errorf("failed to get route total requests: %w", err)
	}
	stats["total_requests"] = totalRequests

	// Success requests for this route
	var successRequests int
	err = db.QueryRow("SELECT COUNT(*) FROM webhook_logs WHERE route_id = ? AND status_code = 200", routeID).Scan(&successRequests)
	if err != nil {
		return nil, fmt.Errorf("failed to get route success requests: %w", err)
	}
	stats["success_requests"] = successRequests

	// Failed requests for this route
	var failedRequests int
	err = db.QueryRow("SELECT COUNT(*) FROM webhook_logs WHERE route_id = ? AND status_code != 200", routeID).Scan(&failedRequests)
	if err != nil {
		return nil, fmt.Errorf("failed to get route failed requests: %w", err)
	}
	stats["failed_requests"] = failedRequests

	// Recent requests (last 24 hours)
	var recentRequests int
	err = db.QueryRow("SELECT COUNT(*) FROM webhook_logs WHERE route_id = ? AND processed_at > datetime('now', '-24 hours')", routeID).Scan(&recentRequests)
	if err != nil {
		return nil, fmt.Errorf("failed to get route recent requests: %w", err)
	}
	stats["recent_requests"] = recentRequests

	return stats, nil
}
