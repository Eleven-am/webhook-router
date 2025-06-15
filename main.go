package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/gorilla/mux"
	"github.com/joho/godotenv"
	"webhook-router/internal/config"
	"webhook-router/internal/database"
	"webhook-router/internal/handlers"
	"webhook-router/internal/rabbitmq"
)

func main() {
	// Set up CPU usage
	err := godotenv.Load()
	nCPU := runtime.NumCPU()
	runtime.GOMAXPROCS(nCPU)
	fmt.Printf("Number of CPUs: %d\n", nCPU)

	// Load configuration
	cfg := config.Load()

	// Initialize database
	db, err := database.Init(cfg.DatabasePath)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer db.Close()

	// Initialize RabbitMQ connection pool
	rmqPool, err := rabbitmq.NewConnectionPool(cfg.RabbitMQURL, 5)
	if err != nil {
		log.Fatalf("Failed to initialize RabbitMQ connection pool: %v", err)
	}
	defer rmqPool.Close()

	// Initialize handlers
	h := handlers.New(db, rmqPool, cfg)

	// Set up routes
	router := mux.NewRouter()

	// Webhook endpoints
	router.HandleFunc("/webhook/{endpoint}", h.HandleWebhook).Methods("POST", "PUT", "PATCH", "DELETE", "GET")
	router.HandleFunc("/webhook", h.HandleWebhook).Methods("POST", "PUT", "PATCH", "DELETE", "GET")

	// API endpoints for route management
	api := router.PathPrefix("/api").Subrouter()
	api.HandleFunc("/routes", h.GetRoutes).Methods("GET")
	api.HandleFunc("/routes", h.CreateRoute).Methods("POST")
	api.HandleFunc("/routes/{id}", h.GetRoute).Methods("GET")
	api.HandleFunc("/routes/{id}", h.UpdateRoute).Methods("PUT")
	api.HandleFunc("/routes/{id}", h.DeleteRoute).Methods("DELETE")
	api.HandleFunc("/routes/{id}/test", h.TestRoute).Methods("POST")

	// Statistics endpoint
	api.HandleFunc("/stats", h.GetStats).Methods("GET")
	api.HandleFunc("/stats/route/{id}", h.GetRouteStats).Methods("GET")

	// Health check
	router.HandleFunc("/health", h.HealthCheck).Methods("GET")

	// Serve static files and frontend
	router.PathPrefix("/static/").Handler(http.StripPrefix("/static/", http.FileServer(http.Dir("./web/static/"))))
	router.HandleFunc("/", h.ServeFrontend).Methods("GET")
	router.HandleFunc("/admin", h.ServeFrontend).Methods("GET")

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

	fmt.Println("Server exited")
}
