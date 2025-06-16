package protocols

import (
	"context"
	"io"
	"time"
)

type Protocol interface {
	Name() string
	Connect(config ProtocolConfig) error
	Send(request *Request) (*Response, error)
	Listen(ctx context.Context, handler RequestHandler) error
	Close() error
}

type ProtocolConfig interface {
	Validate() error
	GetType() string
}

type Request struct {
	Method      string
	URL         string
	Headers     map[string]string
	Body        []byte
	Auth        AuthConfig
	Timeout     time.Duration
	Retries     int
	QueryParams map[string]string
}

type Response struct {
	StatusCode int
	Headers    map[string]string
	Body       []byte
	Duration   time.Duration
}

type RequestHandler func(request *IncomingRequest) (*Response, error)

type IncomingRequest struct {
	Method      string
	URL         string
	Path        string
	Headers     map[string]string
	Body        []byte
	QueryParams map[string]string
	RemoteAddr  string
	Timestamp   time.Time
}

type AuthConfig interface {
	Apply(request *Request) error
	GetType() string
}

type BasicAuth struct {
	Username string
	Password string
}

func (b *BasicAuth) Apply(request *Request) error {
	if request.Headers == nil {
		request.Headers = make(map[string]string)
	}
	// Basic auth will be implemented in the protocol handler
	request.Headers["Authorization"] = "Basic " + encodeBasicAuth(b.Username, b.Password)
	return nil
}

func (b *BasicAuth) GetType() string {
	return "basic"
}

type BearerToken struct {
	Token string
}

func (b *BearerToken) Apply(request *Request) error {
	if request.Headers == nil {
		request.Headers = make(map[string]string)
	}
	request.Headers["Authorization"] = "Bearer " + b.Token
	return nil
}

func (b *BearerToken) GetType() string {
	return "bearer"
}

type APIKey struct {
	Key    string
	Header string
}

func (a *APIKey) Apply(request *Request) error {
	if request.Headers == nil {
		request.Headers = make(map[string]string)
	}
	headerName := a.Header
	if headerName == "" {
		headerName = "X-API-Key"
	}
	request.Headers[headerName] = a.Key
	return nil
}

func (a *APIKey) GetType() string {
	return "apikey"
}

type ProtocolFactory interface {
	Create(config ProtocolConfig) (Protocol, error)
	GetType() string
}

// Utility function for basic auth encoding
func encodeBasicAuth(username, password string) string {
	// This should use base64 encoding
	return username + ":" + password // Simplified for now
}