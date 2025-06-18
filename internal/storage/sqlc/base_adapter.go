// Package sqlc provides type-safe database adapters using SQLC code generation.
package sqlc

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"webhook-router/internal/storage"
)

// BaseAdapter provides common functionality for SQLC adapters to reduce code duplication
// between SQLite and PostgreSQL implementations. It includes:
// - Type conversion helpers for nullable database types
// - JSON marshaling/unmarshaling utilities
// - Common filtering and query building logic
// - Transaction management helpers
// - Error handling utilities
type BaseAdapter struct {
	// DLQ methods are now implemented in SQLCAdapter
}

// ConvertNullableInt64 converts a nullable int64 pointer to int.
// Returns 0 if the pointer is nil.
func (b *BaseAdapter) ConvertNullableInt64(val *int64) int {
	if val != nil {
		return int(*val)
	}
	return 0
}

// ConvertNullableString converts a nullable string to string
func (b *BaseAdapter) ConvertNullableString(val *string) string {
	if val != nil {
		return *val
	}
	return ""
}

// ConvertNullableBool converts a nullable bool to bool
func (b *BaseAdapter) ConvertNullableBool(val *bool) bool {
	if val != nil {
		return *val
	}
	return false
}

// ConvertNullableTime converts a nullable time to time.Time
func (b *BaseAdapter) ConvertNullableTime(val *time.Time) time.Time {
	if val != nil {
		return *val
	}
	return time.Now()
}

// ConvertIntToNullableInt64 converts int to nullable int64
func (b *BaseAdapter) ConvertIntToNullableInt64(val int) *int64 {
	if val > 0 {
		i64 := int64(val)
		return &i64
	}
	return nil
}

// ConvertIntPtrToNullableInt64 converts *int to nullable int64
func (b *BaseAdapter) ConvertIntPtrToNullableInt64(val *int) *int64 {
	if val != nil && *val > 0 {
		i64 := int64(*val)
		return &i64
	}
	return nil
}

// ConvertStringToNullable converts string to nullable string
func (b *BaseAdapter) ConvertStringToNullable(val string) *string {
	if val != "" {
		return &val
	}
	return nil
}

// UnmarshalJSON unmarshals a JSON string into the target interface.
// Commonly used for deserializing JSON config fields from the database.
func (b *BaseAdapter) UnmarshalJSON(data string, target interface{}) error {
	return json.Unmarshal([]byte(data), target)
}

// MarshalJSON marshals map to JSON string
func (b *BaseAdapter) MarshalJSON(data interface{}) (string, error) {
	bytes, err := json.Marshal(data)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

// ApplyTriggerFilters filters a slice of triggers based on the provided criteria.
// Filters by type, status, and active state when specified.
func (b *BaseAdapter) ApplyTriggerFilters(triggers []*storage.Trigger, filters storage.TriggerFilters) []*storage.Trigger {
	var filtered []*storage.Trigger
	for _, trigger := range triggers {
		// Apply filters
		if filters.Type != "" && trigger.Type != filters.Type {
			continue
		}
		if filters.Status != "" && trigger.Status != filters.Status {
			continue
		}
		if filters.Active != nil && trigger.Active != *filters.Active {
			continue
		}
		filtered = append(filtered, trigger)
	}
	return filtered
}

// BuildStatsResult constructs a standardized statistics result map from database query results.
// Handles nullable SQL types and provides default values for missing data.
// Returns a map with keys: route_id, route_name, total, success, failed,
// avg_transformation_time, avg_publish_time, and optionally last_processed.
func (b *BaseAdapter) BuildStatsResult(
	routeID int,
	routeName string,
	totalRequests int64,
	successRequests int64,
	failedRequests int64,
	avgTransformTime sql.NullFloat64,
	avgPublishTime sql.NullFloat64,
	lastProcessed sql.NullTime,
) map[string]interface{} {
	result := map[string]interface{}{
		"route_id":                routeID,
		"route_name":              routeName,
		"total":                   totalRequests,
		"success":                 successRequests,
		"failed":                  failedRequests,
		"avg_transformation_time": float64(0),
		"avg_publish_time":        float64(0),
	}

	// Handle nullable float values
	if avgTransformTime.Valid {
		result["avg_transformation_time"] = avgTransformTime.Float64
	}
	if avgPublishTime.Valid {
		result["avg_publish_time"] = avgPublishTime.Float64
	}
	if lastProcessed.Valid {
		result["last_processed"] = lastProcessed.Time
	}

	return result
}

// ExecuteQuery executes a generic SQL query and returns results as a slice of maps.
// Each map represents a row with column names as keys.
// This is used for implementing the generic Query interface method.
func (b *BaseAdapter) ExecuteQuery(db *sql.DB, query string, args ...interface{}) ([]map[string]interface{}, error) {
	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	// Get column names
	columns, err := rows.Columns()
	if err != nil {
		return nil, err
	}

	// Prepare result
	var results []map[string]interface{}

	// Create a slice of interface{}'s to represent each column
	values := make([]interface{}, len(columns))
	for i := range values {
		values[i] = new(interface{})
	}

	// Iterate through rows
	for rows.Next() {
		err = rows.Scan(values...)
		if err != nil {
			return nil, err
		}

		// Create map for this row
		row := make(map[string]interface{})
		for i, col := range columns {
			row[col] = *(values[i].(*interface{}))
		}

		results = append(results, row)
	}

	return results, rows.Err()
}

// TransactionWrapper provides a safe way to execute database operations within a transaction.
// If the function returns an error, the transaction is rolled back.
// Otherwise, the transaction is committed.
func (b *BaseAdapter) TransactionWrapper(db *sql.DB, fn func(tx *sql.Tx) error) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}

	if err := fn(tx); err != nil {
		tx.Rollback()
		return err
	}

	return tx.Commit()
}

// HandleNotFound converts sql.ErrNoRows to nil, allowing methods to return
// nil for both the result and error when a record is not found.
// This provides a consistent API across storage methods.
func (b *BaseAdapter) HandleNotFound(err error) error {
	if err == sql.ErrNoRows {
		return nil
	}
	return err
}

// HandlePgxNotFound handles PostgreSQL specific not found errors.
// It checks for both pgx.ErrNoRows and the error string for compatibility.
func (b *BaseAdapter) HandlePgxNotFound(err error) error {
	if err == nil {
		return nil
	}
	// Check for pgx.ErrNoRows or the error string
	if err.Error() == "no rows in result set" {
		return nil
	}
	return err
}

// GetStatsSince calculates stats for a given time period
func (b *BaseAdapter) GetStatsSince(ctx context.Context, since time.Time, getStats func(context.Context, time.Time) (interface{}, error)) (*storage.Stats, error) {
	_, err := getStats(ctx, since)
	if err != nil {
		return nil, fmt.Errorf("failed to get webhook log stats: %w", err)
	}

	// The actual conversion will be done by the specific adapter
	// This is just a helper to standardize the error handling
	return nil, err
}
