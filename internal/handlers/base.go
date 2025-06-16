package handlers

import (
	"embed"
	"net/url"
	"time"

	"webhook-router/internal/auth"
	"webhook-router/internal/brokers"
	"webhook-router/internal/config"
	"webhook-router/internal/pipeline"
	"webhook-router/internal/routing"
	"webhook-router/internal/storage"
)

type Handlers struct {
	storage        storage.Storage
	broker         brokers.Broker
	config         *config.Config
	webFS          embed.FS
	auth           *auth.Auth
	router         routing.Router
	pipelineEngine pipeline.PipelineEngine
}

type WebhookPayload struct {
	Method    string              `json:"method"`
	URL       *url.URL            `json:"url"`
	Headers   map[string][]string `json:"headers"`
	Body      string              `json:"body"`
	Timestamp time.Time           `json:"timestamp"`
	RouteID   int                 `json:"route_id,omitempty"`
	RouteName string              `json:"route_name,omitempty"`
}

func New(storage storage.Storage, broker brokers.Broker, cfg *config.Config, webFS embed.FS, authHandler *auth.Auth, router routing.Router, pipelineEngine pipeline.PipelineEngine) *Handlers {
	return &Handlers{
		storage:        storage,
		broker:         broker,
		config:         cfg,
		webFS:          webFS,
		auth:           authHandler,
		router:         router,
		pipelineEngine: pipelineEngine,
	}
}
