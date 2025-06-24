package app

import (
	"crypto/md5"
	"database/sql"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"webhook-router/internal/common/logging"
)

// Compile regex once at package level for performance
var migrationVersionRegex = regexp.MustCompile(`^(\d+)_.*\.sql$`)

// Migration represents a single database migration
type Migration struct {
	Version  string
	Filename string
	Content  string
	Checksum string
}

// MigrationManager handles database schema migrations
type MigrationManager struct {
	db        *sql.DB
	logger    logging.Logger
	schemaDir string
	dbType    string // "sqlite" or "postgres"
}

// NewMigrationManager creates a new migration manager
func NewMigrationManager(db *sql.DB, logger logging.Logger, schemaDir string) *MigrationManager {
	// Detect database type
	dbType := detectDatabaseType(db)

	return &MigrationManager{
		db:        db,
		logger:    logger,
		schemaDir: schemaDir,
		dbType:    dbType,
	}
}

// detectDatabaseType detects the database type from the driver name
func detectDatabaseType(db *sql.DB) string {
	// Try to execute a PostgreSQL-specific query
	_, err := db.Exec("SELECT version()")
	if err == nil {
		// If this succeeds, it's likely PostgreSQL
		return "postgres"
	}

	// Try SQLite-specific query
	_, err = db.Exec("SELECT sqlite_version()")
	if err == nil {
		return "sqlite"
	}

	// Default to SQLite if we can't determine
	return "sqlite"
}

// RunMigrations runs all pending migrations
func (m *MigrationManager) RunMigrations() error {
	m.logger.Info("Starting database migrations",
		logging.Field{"db_type", m.dbType},
		logging.Field{"schema_dir", m.schemaDir},
	)

	// Ensure migrations table exists
	if err := m.ensureMigrationsTable(); err != nil {
		return fmt.Errorf("failed to create migrations table: %w", err)
	}

	// Dynamically discover all migration files
	migrations, err := m.loadMigrations()
	if err != nil {
		return fmt.Errorf("failed to dynamically load migration files: %w", err)
	}

	m.logger.Info("Discovered migration files",
		logging.Field{"total_files", len(migrations)},
		logging.Field{"db_type", m.dbType},
	)

	// Get applied migrations from database
	appliedMigrations, err := m.getAppliedMigrations()
	if err != nil {
		return fmt.Errorf("failed to get applied migrations: %w", err)
	}

	// Find pending migrations
	pendingMigrations := m.findPendingMigrations(migrations, appliedMigrations)

	if len(pendingMigrations) == 0 {
		m.logger.Info("No pending migrations found - database is up to date")
		return nil
	}

	m.logger.Info("Found pending migrations",
		logging.Field{"count", len(pendingMigrations)},
		logging.Field{"versions", m.extractVersionList(pendingMigrations)},
	)

	// Apply pending migrations in sequence
	for _, migration := range pendingMigrations {
		if err := m.applyMigration(migration); err != nil {
			return fmt.Errorf("failed to apply migration %s: %w", migration.Version, err)
		}
	}

	m.logger.Info("All migrations completed successfully",
		logging.Field{"applied_count", len(pendingMigrations)},
	)
	return nil
}

// ensureMigrationsTable creates the schema_migrations table if it doesn't exist
func (m *MigrationManager) ensureMigrationsTable() error {
	query := `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version TEXT PRIMARY KEY,
			filename TEXT NOT NULL,
			applied_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			checksum TEXT
		);
		CREATE INDEX IF NOT EXISTS idx_schema_migrations_applied_at ON schema_migrations(applied_at);
	`

	_, err := m.db.Exec(query)
	if err != nil {
		return fmt.Errorf("failed to create schema_migrations table: %w", err)
	}

	return nil
}

// loadMigrations dynamically loads all migration files from the schema directory
func (m *MigrationManager) loadMigrations() ([]Migration, error) {
	var migrations []Migration
	var discoveredFiles []string

	m.logger.Debug("Scanning for migration files",
		logging.Field{"directory", m.schemaDir},
		logging.Field{"db_type", m.dbType},
	)

	err := filepath.WalkDir(m.schemaDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			m.logger.Warn("Error accessing file during migration discovery",
				logging.Field{"path", path},
				logging.Field{"error", err.Error()},
			)
			return err
		}

		// Skip directories and non-SQL files
		if d.IsDir() || !strings.HasSuffix(path, ".sql") {
			return nil
		}

		filename := d.Name()
		discoveredFiles = append(discoveredFiles, filename)

		// Skip files that don't match the current database type
		if !m.isFileCompatible(filename) {
			m.logger.Debug("Skipping incompatible migration file",
				logging.Field{"filename", filename},
				logging.Field{"db_type", m.dbType},
			)
			return nil
		}

		// Extract version from filename (e.g., "001_initial.sql" -> "001")
		version := m.extractVersion(filename)
		if version == "" {
			m.logger.Warn("Skipping file with invalid version format",
				logging.Field{"filename", filename},
				logging.Field{"expected_format", "###_name.sql"},
			)
			return nil
		}

		// Read file content
		content, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("failed to read migration file %s: %w", path, err)
		}

		// Calculate checksum for integrity verification
		checksum := fmt.Sprintf("%x", md5.Sum(content))

		m.logger.Debug("Discovered valid migration file",
			logging.Field{"filename", filename},
			logging.Field{"version", version},
			logging.Field{"size_bytes", len(content)},
		)

		migrations = append(migrations, Migration{
			Version:  version,
			Filename: filename,
			Content:  string(content),
			Checksum: checksum,
		})

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("error during dynamic migration discovery: %w", err)
	}

	m.logger.Debug("Migration file discovery complete",
		logging.Field{"total_files_found", len(discoveredFiles)},
		logging.Field{"compatible_migrations", len(migrations)},
		logging.Field{"all_files", discoveredFiles},
	)

	// Sort migrations by version for sequential application
	sort.Slice(migrations, func(i, j int) bool {
		return m.compareVersions(migrations[i].Version, migrations[j].Version) < 0
	})

	return migrations, nil
}

// extractVersion extracts version from filename using pre-compiled regex
func (m *MigrationManager) extractVersion(filename string) string {
	// Match patterns like "001_initial.sql", "002_dlq.sql", etc.
	matches := migrationVersionRegex.FindStringSubmatch(filename)
	if len(matches) < 2 {
		return ""
	}
	return matches[1]
}

// isFileCompatible checks if a migration file is compatible with the current database type
func (m *MigrationManager) isFileCompatible(filename string) bool {
	switch m.dbType {
	case "postgres":
		// For PostgreSQL, load files ending with "_postgres.sql" or generic files without db suffix
		if strings.HasSuffix(filename, "_postgres.sql") {
			return true
		}
		// Skip SQLite-specific files
		if strings.Contains(filename, "_sqlite") || (!strings.Contains(filename, "_postgres") && !strings.Contains(filename, "_sqlite")) {
			return false
		}
		return true
	case "sqlite":
		// For SQLite, skip PostgreSQL-specific files and load generic or SQLite-specific files
		if strings.HasSuffix(filename, "_postgres.sql") {
			return false
		}
		return true
	default:
		// Default to allowing all files
		return true
	}
}

// compareVersions compares two version strings numerically
func (m *MigrationManager) compareVersions(v1, v2 string) int {
	n1, _ := strconv.Atoi(v1)
	n2, _ := strconv.Atoi(v2)

	if n1 < n2 {
		return -1
	} else if n1 > n2 {
		return 1
	}
	return 0
}

// getAppliedMigrations returns a map of applied migration versions
func (m *MigrationManager) getAppliedMigrations() (map[string]bool, error) {
	query := "SELECT version FROM schema_migrations"
	rows, err := m.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	appliedMigrations := make(map[string]bool)
	for rows.Next() {
		var version string
		if err := rows.Scan(&version); err != nil {
			return nil, err
		}
		appliedMigrations[version] = true
	}

	return appliedMigrations, nil
}

// findPendingMigrations finds migrations that haven't been applied yet
func (m *MigrationManager) findPendingMigrations(allMigrations []Migration, appliedMigrations map[string]bool) []Migration {
	var pendingMigrations []Migration

	for _, migration := range allMigrations {
		if !appliedMigrations[migration.Version] {
			pendingMigrations = append(pendingMigrations, migration)
		}
	}

	return pendingMigrations
}

// extractVersionList extracts version numbers from migrations for logging
func (m *MigrationManager) extractVersionList(migrations []Migration) []string {
	versions := make([]string, len(migrations))
	for i, migration := range migrations {
		versions[i] = migration.Version
	}
	return versions
}

// applyMigration applies a single migration within a transaction
func (m *MigrationManager) applyMigration(migration Migration) error {
	m.logger.Info("Applying migration",
		logging.Field{"version", migration.Version},
		logging.Field{"filename", migration.Filename},
	)

	// Start transaction
	tx, err := m.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to start transaction: %w", err)
	}
	defer tx.Rollback()

	// Execute migration SQL
	_, err = tx.Exec(migration.Content)
	if err != nil {
		return fmt.Errorf("failed to execute migration SQL: %w", err)
	}

	// Record migration as applied
	_, err = tx.Exec(
		"INSERT INTO schema_migrations (version, filename, applied_at, checksum) VALUES (?, ?, ?, ?)",
		migration.Version,
		migration.Filename,
		time.Now(),
		migration.Checksum,
	)
	if err != nil {
		return fmt.Errorf("failed to record migration: %w", err)
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit migration: %w", err)
	}

	m.logger.Info("Migration applied successfully",
		logging.Field{"version", migration.Version},
	)

	return nil
}

// GetMigrationStatus returns the current migration status
func (m *MigrationManager) GetMigrationStatus() (map[string]interface{}, error) {
	migrations, err := m.loadMigrations()
	if err != nil {
		return nil, err
	}

	appliedMigrations, err := m.getAppliedMigrations()
	if err != nil {
		return nil, err
	}

	pendingMigrations := m.findPendingMigrations(migrations, appliedMigrations)

	return map[string]interface{}{
		"total_migrations":   len(migrations),
		"applied_migrations": len(appliedMigrations),
		"pending_migrations": len(pendingMigrations),
		"status":             fmt.Sprintf("%d/%d migrations applied", len(appliedMigrations), len(migrations)),
	}, nil
}
