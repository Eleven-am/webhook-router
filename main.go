package main

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/gorilla/mux"
	"github.com/joho/godotenv"
	"webhook-router/internal/auth"
	"webhook-router/internal/config"
	"webhook-router/internal/database"
	"webhook-router/internal/handlers"
	"webhook-router/internal/rabbitmq"
)

// Embed the web directory
//go:embed web/*
var webFS embed.FS

func main() {
	// Set up CPU usage
	err := godotenv.Load()
	nCPU := runtime.NumCPU()
	runtime.GOMAXPROCS(nCPU)
	fmt.Printf("Number of CPUs: %d\n", nCPU)

	// Load configuration
	cfg := config.Load()

	// Initialize in-memory database (self-contained)
	db, err := database.Init(":memory:")
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer db.Close()

	// Initialize auth
	authHandler := auth.New(db)

	// Initialize RabbitMQ connection pool (optional)
	var rmqPool *rabbitmq.ConnectionPool
	rmqURL, err := db.GetSetting("rabbitmq_url")
	if err == nil && rmqURL != "" {
		rmqPool, err = rabbitmq.NewConnectionPool(rmqURL, 5)
		if err != nil {
			log.Printf("Warning: Failed to initialize RabbitMQ connection pool: %v", err)
			log.Println("RabbitMQ can be configured later through the admin interface")
		} else {
			defer rmqPool.Close()
		}
	} else {
		log.Println("RabbitMQ not configured. Configure it through the admin interface.")
	}

	// Initialize handlers with embedded filesystem
	h := handlers.New(db, rmqPool, cfg, webFS, authHandler)

	// Set up routes
	router := mux.NewRouter()

	// Auth routes (no auth required)
	router.HandleFunc("/login", h.ServeLogin).Methods("GET")
	router.HandleFunc("/login", h.HandleLogin).Methods("POST")
	router.HandleFunc("/logout", h.HandleLogout).Methods("POST", "GET")
	router.HandleFunc("/change-password", h.ServeChangePassword).Methods("GET")
	router.HandleFunc("/change-password", h.HandleChangePassword).Methods("POST")

	// Health check (no auth required)
	router.HandleFunc("/health", h.HealthCheck).Methods("GET")

	// Webhook endpoints (no auth required)
	router.HandleFunc("/webhook/{endpoint}", h.HandleWebhook).Methods("POST", "PUT", "PATCH", "DELETE", "GET")
	router.HandleFunc("/webhook", h.HandleWebhook).Methods("POST", "PUT", "PATCH", "DELETE", "GET")

	// Protected routes - require authentication
	protected := router.NewRoute().Subrouter()
	protected.Use(authHandler.RequireAuth)

	// API endpoints for route management (protected)
	api := protected.PathPrefix("/api").Subrouter()
	api.HandleFunc("/routes", h.GetRoutes).Methods("GET")
	api.HandleFunc("/routes", h.CreateRoute).Methods("POST")
	api.HandleFunc("/routes/{id}", h.GetRoute).Methods("GET")
	api.HandleFunc("/routes/{id}", h.UpdateRoute).Methods("PUT")
	api.HandleFunc("/routes/{id}", h.DeleteRoute).Methods("DELETE")
	api.HandleFunc("/routes/{id}/test", h.TestRoute).Methods("POST")

	// Settings endpoints (protected)
	api.HandleFunc("/settings", h.GetSettings).Methods("GET")
	api.HandleFunc("/settings", h.UpdateSettings).Methods("POST")

	// Statistics endpoints (protected)
	api.HandleFunc("/stats", h.GetStats).Methods("GET")
	api.HandleFunc("/stats/route/{id}", h.GetRouteStats).Methods("GET")

	// Serve static files from embedded filesystem (protected)
	staticFS, err := fs.Sub(webFS, "web/static")
	if err != nil {
		log.Fatalf("Failed to create static filesystem: %v", err)
	}
	protected.PathPrefix("/static/").Handler(http.StripPrefix("/static/", http.FileServer(http.FS(staticFS))))

	// Frontend routes (protected)
	protected.HandleFunc("/", h.ServeFrontend).Methods("GET")
	protected.HandleFunc("/admin", h.ServeFrontend).Methods("GET")
	protected.HandleFunc("/settings", h.ServeSettings).Methods("GET")

	// Start session cleanup routine
	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			authHandler.CleanupExpiredSessions()
		}
	}()

	// Set up HTTP server
	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      router,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Start server in a goroutine
	go func() {
		fmt.Printf("Server starting on port %s...\n", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server failed to start: %v", err)
		}
	}()

	// Wait for interrupt signal to gracefully shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	fmt.Println("Shutting down server...")

	// Graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	fmt.Println("✅ Server exited")
}