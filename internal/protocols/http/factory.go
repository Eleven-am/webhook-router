package http

import (
	"fmt"
	"webhook-router/internal/protocols"
)

type Factory struct{}

func (f *Factory) Create(config protocols.ProtocolConfig) (protocols.Protocol, error) {
	httpConfig, ok := config.(*Config)
	if !ok {
		return nil, fmt.Errorf("invalid config type for HTTP protocol")
	}

	return NewClient(httpConfig)
}

func (f *Factory) GetType() string {
	return "http"
}

func init() {
	protocols.Register("http", &Factory{})
}