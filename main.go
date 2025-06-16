// @title Webhook Router API
// @version 1.0
// @description Enterprise-grade webhook routing and data transformation platform
// @termsOfService http://swagger.io/terms/

// @contact.name API Support
// @contact.url http://www.swagger.io/support
// @contact.email support@swagger.io

// @license.name MIT
// @license.url https://opensource.org/licenses/MIT

// @host localhost:8080
// @BasePath /api

// @securityDefinitions.apikey SessionAuth
// @in cookie
// @name session

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
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/gorilla/mux"
	"github.com/joho/godotenv"
	httpSwagger "github.com/swaggo/http-swagger"
	"webhook-router/internal/auth"
	"webhook-router/internal/brokers"
	"webhook-router/internal/brokers/rabbitmq"
	"webhook-router/internal/brokers/kafka"
	"webhook-router/internal/brokers/redis"
	"webhook-router/internal/brokers/aws"
	"webhook-router/internal/config"
	"webhook-router/internal/handlers"
	"webhook-router/internal/pipeline"
	"webhook-router/internal/pipeline/stages"
	"webhook-router/internal/routing"
	"webhook-router/internal/storage/sqlite"
)

// Embed the web directory
//
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

	// Initialize storage (SQLite for now, PostgreSQL in Phase 1)
	dbPath := "./webhook_router.db"
	if cfg.DatabasePath != "./webhook_router.db" {
		dbPath = cfg.DatabasePath
	}

	storageConfig := &sqlite.Config{
		DatabasePath: dbPath,
	}

	store, err := sqlite.NewAdapter(storageConfig)
	if err != nil {
		log.Fatalf("Failed to initialize storage: %v", err)
	}
	defer store.Close()
	fmt.Printf("Database: %s\n", dbPath)

	// Initialize auth with storage interface
	authHandler := auth.New(store)

	// Initialize broker registry
	brokerRegistry := brokers.NewRegistry()
	brokerRegistry.Register("rabbitmq", &rabbitmq.Factory{})
	brokerRegistry.Register("kafka", &kafka.Factory{})
	brokerRegistry.Register("redis", &redis.Factory{})
	brokerRegistry.Register("aws", &aws.Factory{})

	// Initialize broker (support multiple broker types)
	var broker brokers.Broker

	// Try to get broker configuration from database settings
	brokerType, _ := store.GetSetting("broker_type")
	if brokerType == "" {
		brokerType = "rabbitmq" // Default to RabbitMQ for backward compatibility
	}

	switch brokerType {
	case "rabbitmq":
		rmqURL := cfg.RabbitMQURL
		if rmqURL == "" || rmqURL == "amqp://guest:guest@localhost:5672/" {
			if dbURL, err := store.GetSetting("rabbitmq_url"); err == nil && dbURL != "" {
				rmqURL = dbURL
				log.Println("Using RabbitMQ URL from database settings")
			}
		} else {
			log.Println("Using RabbitMQ URL from environment variable")
		}

		if rmqURL != "" && rmqURL != "amqp://guest:guest@localhost:5672/" {
			rmqConfig := &rabbitmq.Config{
				URL:      rmqURL,
				PoolSize: 5,
			}
			
			broker, err = brokerRegistry.Create("rabbitmq", rmqConfig)
			if err != nil {
				log.Printf("Warning: Failed to initialize RabbitMQ broker: %v", err)
			} else {
				defer broker.Close()
				fmt.Printf("RabbitMQ: Connected\n")
			}
		}

	case "kafka":
		if kafkaBrokers, err := store.GetSetting("kafka_brokers"); err == nil && kafkaBrokers != "" {
			kafkaConfig := kafka.DefaultConfig()
			// Parse comma-separated broker list
			kafkaConfig.Brokers = strings.Split(kafkaBrokers, ",")
			
			if groupID, err := store.GetSetting("kafka_group_id"); err == nil && groupID != "" {
				kafkaConfig.GroupID = groupID
			}
			if clientID, err := store.GetSetting("kafka_client_id"); err == nil && clientID != "" {
				kafkaConfig.ClientID = clientID
			}

			broker, err = brokerRegistry.Create("kafka", kafkaConfig)
			if err != nil {
				log.Printf("Warning: Failed to initialize Kafka broker: %v", err)
			} else {
				defer broker.Close()
				fmt.Printf("Kafka: Connected\n")
			}
		}

	case "redis":
		if redisAddr, err := store.GetSetting("redis_address"); err == nil && redisAddr != "" {
			redisConfig := redis.DefaultConfig()
			redisConfig.Address = redisAddr
			
			if redisPassword, err := store.GetSetting("redis_password"); err == nil {
				redisConfig.Password = redisPassword
			}
			if redisDB, err := store.GetSetting("redis_db"); err == nil && redisDB != "" {
				if db, parseErr := strconv.Atoi(redisDB); parseErr == nil {
					redisConfig.DB = db
				}
			}

			broker, err = brokerRegistry.Create("redis", redisConfig)
			if err != nil {
				log.Printf("Warning: Failed to initialize Redis broker: %v", err)
			} else {
				defer broker.Close()
				fmt.Printf("Redis: Connected\n")
			}
		}

	case "aws":
		if awsRegion, err := store.GetSetting("aws_region"); err == nil && awsRegion != "" {
			awsConfig := aws.DefaultConfig()
			awsConfig.Region = awsRegion
			
			if accessKey, err := store.GetSetting("aws_access_key_id"); err == nil && accessKey != "" {
				awsConfig.AccessKeyID = accessKey
			}
			if secretKey, err := store.GetSetting("aws_secret_access_key"); err == nil && secretKey != "" {
				awsConfig.SecretAccessKey = secretKey
			}
			if queueURL, err := store.GetSetting("aws_queue_url"); err == nil && queueURL != "" {
				awsConfig.QueueURL = queueURL
			}
			if topicArn, err := store.GetSetting("aws_topic_arn"); err == nil && topicArn != "" {
				awsConfig.TopicArn = topicArn
			}

			broker, err = brokerRegistry.Create("aws", awsConfig)
			if err != nil {
				log.Printf("Warning: Failed to initialize AWS broker: %v", err)
			} else {
				defer broker.Close()
				fmt.Printf("AWS: Connected\n")
			}
		}

	default:
		log.Printf("Unknown broker type: %s", brokerType)
	}

	if broker == nil {
		log.Println("No broker configured. Brokers can be configured through the admin interface.")
	}

	// Initialize routing engine
	ruleEngine := routing.NewBasicRuleEngine()
	destManager := routing.NewBasicDestinationManager()
	routingEngine := routing.NewBasicRouter(ruleEngine, destManager)
	
	// Start routing engine
	if err := routingEngine.Start(context.Background()); err != nil {
		log.Printf("Warning: Failed to start routing engine: %v", err)
	}
	fmt.Printf("Routing Engine: Started\n")

	// Initialize pipeline engine
	pipelineEngine := pipeline.NewBasicPipelineEngine()
	
	// Register stage factories
	pipelineEngine.RegisterStageFactory("json_transform", &stages.JSONTransformStageFactory{})
	pipelineEngine.RegisterStageFactory("xml_transform", &stages.XMLTransformStageFactory{})
	pipelineEngine.RegisterStageFactory("template", &stages.TemplateStageFactory{})
	pipelineEngine.RegisterStageFactory("validate", &stages.ValidationStageFactory{})
	pipelineEngine.RegisterStageFactory("filter", &stages.FilterStageFactory{})
	pipelineEngine.RegisterStageFactory("enrich", &stages.EnrichmentStageFactory{})
	pipelineEngine.RegisterStageFactory("aggregate", &stages.AggregationStageFactory{})
	
	// Start pipeline engine
	if err := pipelineEngine.Start(context.Background()); err != nil {
		log.Printf("Warning: Failed to start pipeline engine: %v", err)
	}
	fmt.Printf("Pipeline Engine: Started\n")

	// Initialize handlers with new interfaces
	h := handlers.New(store, broker, cfg, webFS, authHandler, routingEngine, pipelineEngine)

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

	// Swagger UI (no auth required)
	router.PathPrefix("/swagger/").Handler(httpSwagger.WrapHandler)

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

	// Broker management endpoints (protected)
	api.HandleFunc("/brokers", h.GetBrokers).Methods("GET")
	api.HandleFunc("/brokers", h.CreateBroker).Methods("POST")
	api.HandleFunc("/brokers/{id}", h.GetBroker).Methods("GET")
	api.HandleFunc("/brokers/{id}", h.UpdateBroker).Methods("PUT")
	api.HandleFunc("/brokers/{id}", h.DeleteBroker).Methods("DELETE")
	api.HandleFunc("/brokers/{id}/test", h.TestBroker).Methods("POST")
	api.HandleFunc("/brokers/types", h.GetAvailableBrokerTypes).Methods("GET")

	// Trigger management endpoints (protected)
	api.HandleFunc("/triggers", h.GetTriggers).Methods("GET")
	api.HandleFunc("/triggers", h.CreateTrigger).Methods("POST")
	api.HandleFunc("/triggers/{id}", h.GetTrigger).Methods("GET")
	api.HandleFunc("/triggers/{id}", h.UpdateTrigger).Methods("PUT")
	api.HandleFunc("/triggers/{id}", h.DeleteTrigger).Methods("DELETE")
	api.HandleFunc("/triggers/{id}/start", h.StartTrigger).Methods("POST")
	api.HandleFunc("/triggers/{id}/stop", h.StopTrigger).Methods("POST")
	api.HandleFunc("/triggers/{id}/test", h.TestTrigger).Methods("POST")
	api.HandleFunc("/triggers/types", h.GetAvailableTriggerTypes).Methods("GET")

	// Routing management endpoints (protected)
	api.HandleFunc("/routing/rules", h.GetRoutingRules).Methods("GET")
	api.HandleFunc("/routing/rules", h.CreateRoutingRule).Methods("POST")
	api.HandleFunc("/routing/rules/{id}", h.GetRoutingRule).Methods("GET")
	api.HandleFunc("/routing/rules/{id}", h.UpdateRoutingRule).Methods("PUT")
	api.HandleFunc("/routing/rules/{id}", h.DeleteRoutingRule).Methods("DELETE")
	api.HandleFunc("/routing/rules/test", h.TestRoutingRule).Methods("POST")
	api.HandleFunc("/routing/metrics", h.GetRouterMetrics).Methods("GET")

	// Pipeline management endpoints (protected)
	api.HandleFunc("/pipelines", h.GetPipelines).Methods("GET")
	api.HandleFunc("/pipelines", h.CreatePipeline).Methods("POST")
	api.HandleFunc("/pipelines/{id}", h.GetPipeline).Methods("GET")
	api.HandleFunc("/pipelines/{id}", h.DeletePipeline).Methods("DELETE")
	api.HandleFunc("/pipelines/{id}/execute", h.ExecutePipeline).Methods("POST")
	api.HandleFunc("/pipelines/metrics", h.GetPipelineMetrics).Methods("GET")
	api.HandleFunc("/pipelines/stages/types", h.GetAvailableStageTypes).Methods("GET")

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

	// Stop routing engine
	if err := routingEngine.Stop(); err != nil {
		log.Printf("Error stopping routing engine: %v", err)
	} else {
		fmt.Println("Routing Engine: Stopped")
	}

	// Stop pipeline engine
	if err := pipelineEngine.Stop(); err != nil {
		log.Printf("Error stopping pipeline engine: %v", err)
	} else {
		fmt.Println("Pipeline Engine: Stopped")
	}

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	fmt.Println("Server exited")
}
