package utils

import (
	"fmt"
	"time"
)

// ParseDuration parses a duration string with support for additional time units.
//
// Extends the standard Go time.ParseDuration with support for days ("d") and
// weeks ("w") units. Falls back to standard parsing for all other formats.
//
// Supported formats:
//   - Standard Go durations: "1h30m", "500ms", "2.5h"
//   - Days: "1d", "7d", "365d" (converted to hours)
//   - Weeks: "1w", "4w", "52w" (converted to hours)
//   - Negative durations: "-1d", "-2w" (supported)
//
// Parameters:
//   - s: Duration string to parse
//
// Returns:
//   - time.Duration: Parsed duration
//   - error: Parsing error if format is invalid
//
// Examples:
//
//	ParseDuration("1d")    // 24 hours
//	ParseDuration("2w")    // 336 hours (14 days)
//	ParseDuration("1h30m") // 1.5 hours (standard Go format)
func ParseDuration(s string) (time.Duration, error) {
	// First try standard Go duration parsing
	if d, err := time.ParseDuration(s); err == nil {
		return d, nil
	}

	// Support for days
	var days int
	if n, err := fmt.Sscanf(s, "%dd", &days); err == nil && n == 1 {
		return time.Duration(days) * 24 * time.Hour, nil
	}

	// Support for weeks
	var weeks int
	if n, err := fmt.Sscanf(s, "%dw", &weeks); err == nil && n == 1 {
		return time.Duration(weeks) * 7 * 24 * time.Hour, nil
	}

	return 0, fmt.Errorf("invalid duration: %s", s)
}

// FormatDuration formats a duration in a human-readable way.
//
// Automatically selects the most appropriate unit based on the duration magnitude:
//   - < 1 minute: seconds with no decimal places
//   - < 1 hour: minutes with no decimal places
//   - < 24 hours: hours with 1 decimal place
//   - >= 24 hours: days with 1 decimal place
//
// Parameters:
//   - d: Duration to format
//
// Returns a human-readable string representation.
//
// Examples:
//
//	FormatDuration(30 * time.Second)   // "30s"
//	FormatDuration(90 * time.Minute)   // "90m"
//	FormatDuration(2.5 * time.Hour)    // "2.5h"
//	FormatDuration(36 * time.Hour)     // "1.5d"
func FormatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%.0fs", d.Seconds())
	}
	if d < time.Hour {
		return fmt.Sprintf("%.0fm", d.Minutes())
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%.1fh", d.Hours())
	}
	return fmt.Sprintf("%.1fd", d.Hours()/24)
}

// NextExecutionTime calculates the next execution time based on interval.
//
// Simple addition of an interval to a base time. Useful for scheduling
// the next run of periodic tasks.
//
// Parameters:
//   - lastExecution: The time of the last execution
//   - interval: Duration to add for the next execution
//
// Returns the calculated next execution time.
func NextExecutionTime(lastExecution time.Time, interval time.Duration) time.Time {
	return lastExecution.Add(interval)
}

// TimeUntilNext calculates the duration until the next execution.
//
// Computes how much time remains until a scheduled execution time.
// Returns negative duration if the execution time has already passed.
//
// Parameters:
//   - nextExecution: The scheduled execution time
//
// Returns the duration until execution (negative if in the past).
func TimeUntilNext(nextExecution time.Time) time.Duration {
	return time.Until(nextExecution)
}

// IsTimeInWindow checks if a time is within a specified time window.
//
// Performs an inclusive check - returns true if the time is exactly
// at the window boundaries or anywhere between them.
//
// Parameters:
//   - t: Time to check
//   - windowStart: Start of the time window (inclusive)
//   - windowEnd: End of the time window (inclusive)
//
// Returns true if t is within [windowStart, windowEnd], false otherwise.
//
// Note: If windowEnd is before windowStart, the result is always false.
func IsTimeInWindow(t, windowStart, windowEnd time.Time) bool {
	return !t.Before(windowStart) && !t.After(windowEnd)
}

// TruncateToMinute truncates a time to the nearest minute boundary.
//
// Removes seconds and nanoseconds, rounding down to the minute.
// Preserves the timezone of the input time.
//
// Example: 2023-01-01 12:34:56.789 → 2023-01-01 12:34:00.000
func TruncateToMinute(t time.Time) time.Time {
	return t.Truncate(time.Minute)
}

// TruncateToHour truncates a time to the nearest hour boundary.
//
// Removes minutes, seconds, and nanoseconds, rounding down to the hour.
// Preserves the timezone of the input time.
//
// Example: 2023-01-01 12:34:56.789 → 2023-01-01 12:00:00.000
func TruncateToHour(t time.Time) time.Time {
	return t.Truncate(time.Hour)
}

// TruncateToDay truncates a time to the start of the day (midnight).
//
// Sets the time to 00:00:00.000 while preserving the date and timezone.
// Unlike time.Truncate, this works correctly across timezone boundaries.
//
// Parameters:
//   - t: Time to truncate
//
// Returns the start of the day (midnight) in the same timezone.
//
// Example: 2023-01-01 12:34:56.789 EST → 2023-01-01 00:00:00.000 EST
func TruncateToDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}
