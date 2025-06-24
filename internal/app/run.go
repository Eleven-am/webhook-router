package app

import (
	"context"
	"embed"
	"flag"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"webhook-router/internal/common/logging"
	"webhook-router/internal/config"
)

// Run is the main entry point for the application
func Run(frontendFiles embed.FS) error {
	// Load environment variables
	_ = godotenv.Load()

	// Set up CPU usage
	runtime.GOMAXPROCS(runtime.NumCPU())

	// Parse command line flags
	var populateExecutionLogs bool
	flag.BoolVar(&populateExecutionLogs, "populate-execution-logs", false, "Populate execution logs with sample data")
	flag.Parse()

	// Initialize logging
	logging.InitGlobalLogger()
	defer logging.MustSync()

	logging.Info("Starting webhook router",
		logging.Field{"cpus", runtime.NumCPU()},
		logging.Field{"version", "1.0.0"},
	)

	// Load and validate configuration
	cfg := config.Load()
	if err := cfg.Validate(); err != nil {
		logging.Error("Configuration validation failed", err)
		return err
	}

	// Initialize application
	app, err := New(cfg)
	if err != nil {
		logging.Error("Failed to initialize application", err)
		return err
	}
	defer app.Cleanup()

	// Handle sample data population if requested
	if populateExecutionLogs {
		if err := PopulateExecutionLogs(app.Storage); err != nil {
			logging.Error("Failed to populate execution logs", err)
			return err
		}
		logging.Info("Successfully populated execution logs with sample data")
		return nil
	}

	// Start server
	srv, _ := app.RunServer(frontendFiles)
	if err := srv.Start(); err != nil {
		logging.Error("Server failed to start", err)
		return err
	}

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit

	logging.Info("Shutting down server...")

	// Graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Shutdown application components
	if err := app.Shutdown(ctx); err != nil {
		logging.Warn("Error during app shutdown", logging.Field{"error", err})
	}

	// Shutdown HTTP server
	if err := srv.Shutdown(ctx); err != nil {
		logging.Error("Server forced to shutdown", err)
		return err
	}

	logging.Info("Server exited")
	return nil
}
