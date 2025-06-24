package utils

// StringOrNil returns a pointer to the string if it's not empty, otherwise returns nil.
// This is useful for converting empty strings to NULL values in database operations.
func StringOrNil(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// StringFromPtr safely dereferences a string pointer, returning an empty string if nil.
func StringFromPtr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
