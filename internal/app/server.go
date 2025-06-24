package app

import (
	"context"
	"embed"
	"net/http"

	"github.com/gorilla/mux"
	"webhook-router/internal/common/logging"
	"webhook-router/internal/handlers"
	"webhook-router/internal/server"
)

// RunServer starts the HTTP server with all handlers configured
func (app *App) RunServer(frontendFiles embed.FS) (*server.Server, http.Handler) {
	// Initialize handlers
	h := handlers.New(
		app.Storage,
		app.Broker,
		app.Config,
		frontendFiles,
		app.Auth,
		app.Router,
		app.PipelineEngine,
		app.TriggerManager,
		app.Encryptor,
		app.OAuthManager,
	)

	// Set broker manager for handlers
	RegisterBrokerFactories(h.GetBrokerManager())

	// Set up routes
	router := mux.NewRouter()
	rateLimiter := app.InitializeRateLimiter()
	SetupRoutes(router, h, app.Auth.RequireAuth, rateLimiter)

	// Start DLQ retry worker if we have storage
	stopDLQWorker := StartDLQWorker(app.Storage, h.GetBrokerManager())
	go func() {
		<-app.shutdownCh
		close(stopDLQWorker)
	}()

	// Create server
	srv := server.New(router, app.Config.Port, "", "")

	return srv, router
}

// Shutdown gracefully shuts down the application
func (app *App) Shutdown(ctx context.Context) error {
	close(app.shutdownCh)

	// Stop routing engine
	if app.Router != nil {
		if err := app.Router.Stop(); err != nil {
			app.Logger.Warn("Error stopping routing engine", logging.Field{"error", err})
		} else {
			app.Logger.Info("Routing engine stopped")
		}
	}

	// Stop pipeline engine
	if app.PipelineEngine != nil {
		if err := app.PipelineEngine.Stop(); err != nil {
			app.Logger.Warn("Error stopping pipeline engine", logging.Field{"error", err})
		} else {
			app.Logger.Info("Pipeline engine stopped")
		}
	}

	// Stop trigger manager is handled in Cleanup()
	return nil
}
