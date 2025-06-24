package stages

// testRegistry is used for testing
var testRegistry *Registry

// SetRegistry sets a custom registry for testing
func SetRegistry(r *Registry) {
	testRegistry = r
}

// GetRegistry returns the test registry or creates a new one
func GetRegistry() *Registry {
	if testRegistry != nil {
		return testRegistry
	}
	return NewRegistry()
}

// ResetRegistry resets the test registry
func ResetRegistry() {
	testRegistry = nil
}
