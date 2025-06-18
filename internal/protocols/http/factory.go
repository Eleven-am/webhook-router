package http

import (
	"fmt"
	"webhook-router/internal/protocols"
)

// Factory implements the ProtocolFactory interface for creating HTTP protocol clients.
type Factory struct{}

// Create instantiates a new HTTP client with the provided configuration.
// The config parameter must be of type *Config, otherwise an error is returned.
func (f *Factory) Create(config protocols.ProtocolConfig) (protocols.Protocol, error) {
	httpConfig, ok := config.(*Config)
	if !ok {
		return nil, fmt.Errorf("invalid config type for HTTP protocol")
	}

	return NewClient(httpConfig)
}

// GetType returns "http" as the protocol type identifier.
func (f *Factory) GetType() string {
	return "http"
}

// init automatically registers the HTTP factory with the default registry when the package is imported.
// This ensures HTTP protocol support is available without manual registration.
func init() {
	protocols.Register("http", &Factory{})
}