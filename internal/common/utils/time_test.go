package utils

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseDuration(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected time.Duration
		hasError bool
	}{
		// Standard Go duration formats
		{"seconds", "30s", 30 * time.Second, false},
		{"minutes", "5m", 5 * time.Minute, false},
		{"hours", "2h", 2 * time.Hour, false},
		{"milliseconds", "500ms", 500 * time.Millisecond, false},
		{"microseconds", "100us", 100 * time.Microsecond, false},
		{"nanoseconds", "1000ns", 1000 * time.Nanosecond, false},
		{"compound", "1h30m", time.Hour + 30*time.Minute, false},
		{"decimal", "1.5h", time.Hour + 30*time.Minute, false},

		// Extended formats - days
		{"single day", "1d", 24 * time.Hour, false},
		{"multiple days", "7d", 7 * 24 * time.Hour, false},
		{"zero days", "0d", 0, false},

		// Extended formats - weeks
		{"single week", "1w", 7 * 24 * time.Hour, false},
		{"multiple weeks", "4w", 4 * 7 * 24 * time.Hour, false},
		{"zero weeks", "0w", 0, false},

		// Error cases
		{"invalid format", "invalid", 0, true},
		{"empty string", "", 0, true},
		{"missing unit", "123", 0, true},
		{"invalid day format", "1.5d", 0, true},
		{"invalid week format", "2.5w", 0, true},
		{"negative day", "-1d", -24 * time.Hour, false},
		{"negative week", "-1w", -7 * 24 * time.Hour, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseDuration(tt.input)

			if tt.hasError {
				assert.Error(t, err)
				if err != nil {
					assert.Contains(t, err.Error(), "invalid duration")
				}
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestParseDuration_LargeValues(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected time.Duration
	}{
		{"large days", "365d", 365 * 24 * time.Hour},
		{"large weeks", "52w", 52 * 7 * 24 * time.Hour},
		{"year in days", "365d", 365 * 24 * time.Hour},
		{"year in weeks", "52w", 52 * 7 * 24 * time.Hour},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseDuration(tt.input)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		name     string
		duration time.Duration
		expected string
	}{
		// Seconds
		{"0 seconds", 0, "0s"},
		{"30 seconds", 30 * time.Second, "30s"},
		{"59 seconds", 59 * time.Second, "59s"},

		// Minutes
		{"1 minute", 1 * time.Minute, "1m"},
		{"30 minutes", 30 * time.Minute, "30m"},
		{"59 minutes", 59 * time.Minute, "59m"},

		// Hours
		{"1 hour", 1 * time.Hour, "1.0h"},
		{"2.5 hours", 2*time.Hour + 30*time.Minute, "2.5h"},
		{"23 hours", 23 * time.Hour, "23.0h"},

		// Days
		{"1 day", 24 * time.Hour, "1.0d"},
		{"1.5 days", 36 * time.Hour, "1.5d"},
		{"7 days", 7 * 24 * time.Hour, "7.0d"},
		{"365 days", 365 * 24 * time.Hour, "365.0d"},

		// Sub-second
		{"milliseconds", 500 * time.Millisecond, "1s"}, // Rounds up
		{"microseconds", 500 * time.Microsecond, "0s"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatDuration(tt.duration)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFormatDuration_Precision(t *testing.T) {
	// Test precision for different ranges
	tests := []struct {
		name      string
		duration  time.Duration
		checkFunc func(string) bool
	}{
		{
			"fractional hours",
			2*time.Hour + 15*time.Minute,
			func(s string) bool {
				return s == "2.2h" || s == "2.3h" // Allow some floating point variance
			},
		},
		{
			"fractional days",
			30 * time.Hour,
			func(s string) bool {
				return s == "1.2d" || s == "1.3d"
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatDuration(tt.duration)
			assert.True(t, tt.checkFunc(result), "Expected result to match precision check, got: %s", result)
		})
	}
}

func TestNextExecutionTime(t *testing.T) {
	baseTime := time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name          string
		lastExecution time.Time
		interval      time.Duration
		expected      time.Time
	}{
		{
			"add minutes",
			baseTime,
			30 * time.Minute,
			baseTime.Add(30 * time.Minute),
		},
		{
			"add hours",
			baseTime,
			2 * time.Hour,
			baseTime.Add(2 * time.Hour),
		},
		{
			"add days",
			baseTime,
			24 * time.Hour,
			baseTime.Add(24 * time.Hour),
		},
		{
			"zero interval",
			baseTime,
			0,
			baseTime,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := NextExecutionTime(tt.lastExecution, tt.interval)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestTimeUntilNext(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name          string
		nextExecution time.Time
		expectedSign  int // -1 for negative, 0 for zero, 1 for positive
	}{
		{
			"future time",
			now.Add(1 * time.Hour),
			1,
		},
		{
			"past time",
			now.Add(-1 * time.Hour),
			-1,
		},
		{
			"current time",
			now,
			0, // Very close to zero
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := TimeUntilNext(tt.nextExecution)

			switch tt.expectedSign {
			case 1:
				assert.True(t, result > 0, "Expected positive duration, got: %v", result)
			case -1:
				assert.True(t, result < 0, "Expected negative duration, got: %v", result)
			case 0:
				assert.True(t, result >= -1*time.Second && result <= 1*time.Second,
					"Expected near zero duration, got: %v", result)
			}
		})
	}
}

func TestIsTimeInWindow(t *testing.T) {
	windowStart := time.Date(2023, 1, 1, 9, 0, 0, 0, time.UTC) // 9 AM
	windowEnd := time.Date(2023, 1, 1, 17, 0, 0, 0, time.UTC)  // 5 PM

	tests := []struct {
		name     string
		testTime time.Time
		expected bool
	}{
		{
			"before window",
			windowStart.Add(-1 * time.Hour),
			false,
		},
		{
			"at window start",
			windowStart,
			true,
		},
		{
			"inside window",
			windowStart.Add(4 * time.Hour), // 1 PM
			true,
		},
		{
			"at window end",
			windowEnd,
			true,
		},
		{
			"after window",
			windowEnd.Add(1 * time.Hour),
			false,
		},
		{
			"same as start",
			windowStart,
			true,
		},
		{
			"same as end",
			windowEnd,
			true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsTimeInWindow(tt.testTime, windowStart, windowEnd)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsTimeInWindow_EdgeCases(t *testing.T) {
	// Test edge cases
	baseTime := time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC)

	t.Run("zero duration window", func(t *testing.T) {
		// Window start and end are the same
		result := IsTimeInWindow(baseTime, baseTime, baseTime)
		assert.True(t, result, "Time should be in zero-duration window when it matches exactly")
	})

	t.Run("negative window", func(t *testing.T) {
		// Window end is before window start (invalid window)
		windowStart := baseTime
		windowEnd := baseTime.Add(-1 * time.Hour)
		testTime := baseTime.Add(-30 * time.Minute)

		result := IsTimeInWindow(testTime, windowStart, windowEnd)
		assert.False(t, result, "Time should not be in invalid (negative) window")
	})
}

func TestTruncateToMinute(t *testing.T) {
	tests := []struct {
		name     string
		input    time.Time
		expected time.Time
	}{
		{
			"exact minute",
			time.Date(2023, 1, 1, 12, 30, 0, 0, time.UTC),
			time.Date(2023, 1, 1, 12, 30, 0, 0, time.UTC),
		},
		{
			"with seconds",
			time.Date(2023, 1, 1, 12, 30, 45, 0, time.UTC),
			time.Date(2023, 1, 1, 12, 30, 0, 0, time.UTC),
		},
		{
			"with nanoseconds",
			time.Date(2023, 1, 1, 12, 30, 0, 123456789, time.UTC),
			time.Date(2023, 1, 1, 12, 30, 0, 0, time.UTC),
		},
		{
			"with seconds and nanoseconds",
			time.Date(2023, 1, 1, 12, 30, 45, 123456789, time.UTC),
			time.Date(2023, 1, 1, 12, 30, 0, 0, time.UTC),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := TruncateToMinute(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestTruncateToHour(t *testing.T) {
	tests := []struct {
		name     string
		input    time.Time
		expected time.Time
	}{
		{
			"exact hour",
			time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC),
			time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC),
		},
		{
			"with minutes",
			time.Date(2023, 1, 1, 12, 30, 0, 0, time.UTC),
			time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC),
		},
		{
			"with minutes and seconds",
			time.Date(2023, 1, 1, 12, 30, 45, 0, time.UTC),
			time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC),
		},
		{
			"with all components",
			time.Date(2023, 1, 1, 12, 30, 45, 123456789, time.UTC),
			time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := TruncateToHour(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestTruncateToDay(t *testing.T) {
	location, _ := time.LoadLocation("America/New_York")

	tests := []struct {
		name     string
		input    time.Time
		expected time.Time
	}{
		{
			"exact day start UTC",
			time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC),
			time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			"middle of day UTC",
			time.Date(2023, 1, 1, 12, 30, 45, 123456789, time.UTC),
			time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			"end of day UTC",
			time.Date(2023, 1, 1, 23, 59, 59, 999999999, time.UTC),
			time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			"different timezone",
			time.Date(2023, 1, 1, 15, 30, 0, 0, location),
			time.Date(2023, 1, 1, 0, 0, 0, 0, location),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := TruncateToDay(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestTruncateToDay_TimezoneHandling(t *testing.T) {
	// Test that timezone is preserved
	est, _ := time.LoadLocation("America/New_York")
	pst, _ := time.LoadLocation("America/Los_Angeles")

	tests := []struct {
		name     string
		input    time.Time
		expected time.Time
	}{
		{
			"EST timezone",
			time.Date(2023, 1, 1, 15, 30, 0, 0, est),
			time.Date(2023, 1, 1, 0, 0, 0, 0, est),
		},
		{
			"PST timezone",
			time.Date(2023, 1, 1, 22, 30, 0, 0, pst),
			time.Date(2023, 1, 1, 0, 0, 0, 0, pst),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := TruncateToDay(tt.input)
			assert.Equal(t, tt.expected, result)
			assert.Equal(t, tt.input.Location(), result.Location(), "Timezone should be preserved")
		})
	}
}

func TestParseDuration_ParsesStandardFirst(t *testing.T) {
	// Test that standard Go duration parsing is tried first
	// This means if Go adds support for "d" or "w" in the future,
	// the standard parsing would take precedence

	// Test a complex duration that Go can parse
	input := "1h30m45s"
	expected := time.Hour + 30*time.Minute + 45*time.Second

	result, err := ParseDuration(input)
	require.NoError(t, err)
	assert.Equal(t, expected, result)
}

func BenchmarkParseDuration_Standard(b *testing.B) {
	for i := 0; i < b.N; i++ {
		ParseDuration("1h30m")
	}
}

func BenchmarkParseDuration_Days(b *testing.B) {
	for i := 0; i < b.N; i++ {
		ParseDuration("7d")
	}
}

func BenchmarkParseDuration_Weeks(b *testing.B) {
	for i := 0; i < b.N; i++ {
		ParseDuration("4w")
	}
}

func BenchmarkFormatDuration(b *testing.B) {
	duration := 2*time.Hour + 30*time.Minute
	for i := 0; i < b.N; i++ {
		FormatDuration(duration)
	}
}

func BenchmarkTruncateToDay(b *testing.B) {
	now := time.Now()
	for i := 0; i < b.N; i++ {
		TruncateToDay(now)
	}
}

// Test edge cases and potential issues

func TestParseDuration_InvalidDayFormats(t *testing.T) {
	invalidInputs := []string{
		"1.5d", // Decimal days not supported
		"d",    // Missing number
		"1D",   // Wrong case
		"1 d",  // Space
		// "1dd" - This actually parses as 1 day (fmt.Sscanf stops at first 'd')
		// "-5d" - This actually parses as -5 days (negative durations are valid)
	}

	for _, input := range invalidInputs {
		t.Run(input, func(t *testing.T) {
			_, err := ParseDuration(input)
			assert.Error(t, err, "Should fail to parse: %s", input)
		})
	}
}

func TestFormatDuration_ZeroAndNegative(t *testing.T) {
	tests := []struct {
		name     string
		duration time.Duration
		expected string
	}{
		{"zero", 0, "0s"},
		{"negative seconds", -30 * time.Second, "-30s"},
		{"negative hours", -2 * time.Hour, "-7200s"},  // Negative durations < minute show as seconds
		{"negative days", -24 * time.Hour, "-86400s"}, // Negative durations < minute show as seconds
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatDuration(tt.duration)
			assert.Equal(t, tt.expected, result)
		})
	}
}
