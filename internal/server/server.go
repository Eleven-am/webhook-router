package server

import (
	"context"
	"crypto/tls"
	"net/http"
	"time"
)

// Server represents an HTTP server
type Server struct {
	srv     *http.Server
	tlsCert string
	tlsKey  string
}

// New creates a new server instance
func New(handler http.Handler, port, tlsCert, tlsKey string) *Server {
	return &Server{
		srv: &http.Server{
			Addr:         ":" + port,
			Handler:      handler,
			ReadTimeout:  30 * time.Second,
			WriteTimeout: 30 * time.Second,
			IdleTimeout:  120 * time.Second,
		},
		tlsCert: tlsCert,
		tlsKey:  tlsKey,
	}
}

// Start starts the server
func (s *Server) Start() error {
	if s.tlsCert != "" && s.tlsKey != "" {
		// Configure TLS
		tlsConfig := &tls.Config{
			MinVersion: tls.VersionTLS12,
		}
		s.srv.TLSConfig = tlsConfig

		// Start HTTPS server in goroutine
		go func() {
			if err := s.srv.ListenAndServeTLS(s.tlsCert, s.tlsKey); err != nil && err != http.ErrServerClosed {
				panic(err)
			}
		}()
		return nil
	}

	// Start HTTP server in goroutine
	go func() {
		if err := s.srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			panic(err)
		}
	}()
	return nil
}

// Shutdown gracefully shuts down the server
func (s *Server) Shutdown(ctx context.Context) error {
	return s.srv.Shutdown(ctx)
}
