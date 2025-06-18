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

// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization

// @securityDefinitions.apikey TokenAuth
// @in cookie
// @name token

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
	_ "webhook-router/docs" // Import generated docs
	"webhook-router/internal/auth"
	"webhook-router/internal/brokers"
	"webhook-router/internal/brokers/aws"
	"webhook-router/internal/brokers/gcp"
	"webhook-router/internal/brokers/kafka"
	"webhook-router/internal/brokers/rabbitmq"
	redisbroker "webhook-router/internal/brokers/redis"
	"webhook-router/internal/config"
	"webhook-router/internal/crypto"
	"webhook-router/internal/handlers"
	"webhook-router/internal/locks"
	"webhook-router/internal/oauth2"
	"webhook-router/internal/pipeline"
	"webhook-router/internal/pipeline/stages"
	"webhook-router/internal/ratelimit"
	"webhook-router/internal/redis"
	"webhook-router/internal/routing"
	"webhook-router/internal/storage"
	"webhook-router/internal/storage/sqlc"
	"webhook-router/internal/triggers"
	brokertrigger "webhook-router/internal/triggers/broker"
	"webhook-router/internal/triggers/caldav"
	"webhook-router/internal/triggers/carddav"
	httptrigger "webhook-router/internal/triggers/http"
	"webhook-router/internal/triggers/imap"
	"webhook-router/internal/triggers/polling"
	"webhook-router/internal/triggers/schedule"
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
	
	// Validate configuration
	if err := cfg.Validate(); err != nil {
		log.Fatalf("Configuration validation failed: %v", err)
	}

	// Initialize storage registry
	storageRegistry := storage.NewRegistry()
	storageRegistry.Register("sqlc", &sqlc.Factory{})

	// Initialize SQLC storage based on configuration
	genericConfig := storage.GenericConfig{
		"type": cfg.DatabaseType,
	}
	store, err := storageRegistry.Create("sqlc", genericConfig)
	if err != nil {
		log.Fatalf("Failed to initialize storage: %v", err)
	}
	
	switch cfg.DatabaseType {
	case "postgres", "postgresql":
		fmt.Printf("Database: PostgreSQL (%s:%s/%s)\n", cfg.PostgresHost, cfg.PostgresPort, cfg.PostgresDB)
	default:
		dbPath := cfg.DatabasePath
		if dbPath == "" {
			dbPath = "./webhook_router.db"
		}
		fmt.Printf("Database: SQLite (%s)\n", dbPath)
	}
	defer store.Close()

	// Initialize Redis client for rate limiting and distributed locks (optional)
	var redisClient *redis.Client
	var rateLimiter *ratelimit.Limiter
	var lockManager *locks.Manager

	if cfg.RedisAddress != "" {
		redisDB, _ := strconv.Atoi(cfg.RedisDB)
		redisPoolSize, _ := strconv.Atoi(cfg.RedisPoolSize)
		redisConfig := &redis.Config{
			Address:  cfg.RedisAddress,
			Password: cfg.RedisPassword,
			DB:       redisDB,
			PoolSize: redisPoolSize,
		}

		redisClient, err = redis.NewClient(redisConfig)
		if err != nil {
			log.Printf("Warning: Failed to initialize Redis client: %v", err)
		} else {
			fmt.Printf("Redis: Connected (%s)\n", cfg.RedisAddress)
			defer redisClient.Close()

			// Initialize rate limiter
			defaultLimit, _ := strconv.Atoi(cfg.RateLimitDefault)
			window, _ := time.ParseDuration(cfg.RateLimitWindow)
			rateLimitConfig := &ratelimit.Config{
				DefaultLimit:  defaultLimit,
				DefaultWindow: window,
				Enabled:       cfg.RateLimitEnabled,
			}
			rateLimiter = ratelimit.NewLimiter(redisClient, rateLimitConfig)

			// Initialize distributed lock manager
			lockManager = locks.NewManager(redisClient)
			defer lockManager.Close()

			fmt.Printf("Rate Limiting: Enabled (%d req/%s)\n", defaultLimit, window)
			fmt.Printf("Distributed Locks: Enabled\n")
		}
	} else {
		fmt.Printf("Redis: Not configured (rate limiting and distributed locks disabled)\n")
	}

	// Initialize auth with storage interface
	authHandler := auth.New(store, cfg, redisClient)
	
	// Initialize OAuth2 manager
	var oauthManager *oauth2.Manager
	if redisClient != nil {
		// Use Redis for distributed token storage
		tokenStorage := oauth2.NewRedisTokenStorage(redisClient)
		oauthManager = oauth2.NewManager(tokenStorage)
		fmt.Printf("OAuth2 Manager: Initialized with Redis storage\n")
	} else {
		// Use database for token storage
		tokenStorage := oauth2.NewDBTokenStorage(store)
		oauthManager = oauth2.NewManager(tokenStorage)
		fmt.Printf("OAuth2 Manager: Initialized with database storage\n")
	}
	defer oauthManager.Close()

	// Initialize broker registry
	brokerRegistry := brokers.NewRegistry()
	brokerRegistry.Register("rabbitmq", rabbitmq.GetFactory())
	brokerRegistry.Register("kafka", kafka.GetFactory())
	brokerRegistry.Register("redis", redisbroker.GetFactory())
	brokerRegistry.Register("aws", aws.GetFactory())
	brokerRegistry.Register("gcp", gcp.GetFactory())

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
			redisConfig := redisbroker.DefaultConfig()
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

	case "gcp":
		if gcpProjectID, err := store.GetSetting("gcp_project_id"); err == nil && gcpProjectID != "" {
			gcpConfig := gcp.DefaultConfig()
			gcpConfig.ProjectID = gcpProjectID

			if topicID, err := store.GetSetting("gcp_topic_id"); err == nil && topicID != "" {
				gcpConfig.TopicID = topicID
			}
			if subscriptionID, err := store.GetSetting("gcp_subscription_id"); err == nil && subscriptionID != "" {
				gcpConfig.SubscriptionID = subscriptionID
			}
			if credentialsJSON, err := store.GetSetting("gcp_credentials_json"); err == nil && credentialsJSON != "" {
				gcpConfig.CredentialsJSON = credentialsJSON
			}
			if credentialsPath, err := store.GetSetting("gcp_credentials_path"); err == nil && credentialsPath != "" {
				gcpConfig.CredentialsPath = credentialsPath
			}
			if createSub, err := store.GetSetting("gcp_create_subscription"); err == nil && createSub == "true" {
				gcpConfig.CreateSubscription = true
			}
			if orderingEnabled, err := store.GetSetting("gcp_enable_ordering"); err == nil && orderingEnabled == "true" {
				gcpConfig.EnableMessageOrdering = true
			}
			if orderingKey, err := store.GetSetting("gcp_ordering_key"); err == nil && orderingKey != "" {
				gcpConfig.OrderingKey = orderingKey
			}

			broker, err = brokerRegistry.Create("gcp", gcpConfig)
			if err != nil {
				log.Printf("Warning: Failed to initialize GCP Pub/Sub broker: %v", err)
			} else {
				defer broker.Close()
				fmt.Printf("GCP Pub/Sub: Connected\n")
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
	pipelineEngine.RegisterStageFactory("enrich", stages.NewEnrichmentStageFactory(redisClient))
	pipelineEngine.RegisterStageFactory("aggregate", &stages.AggregationStageFactory{})

	// Start pipeline engine
	if err := pipelineEngine.Start(context.Background()); err != nil {
		log.Printf("Warning: Failed to start pipeline engine: %v", err)
	}
	fmt.Printf("Pipeline Engine: Started\n")

	// Initialize trigger manager
	var triggerManager *triggers.Manager
	var distributedTriggerManager *triggers.DistributedManager

	triggerManagerConfig := &triggers.ManagerConfig{
		HealthCheckInterval: 5 * time.Minute,
		SyncInterval:        30 * time.Second,
		MaxRetries:          3,
	}
	triggerManager = triggers.NewManager(store, brokerRegistry, broker, triggerManagerConfig)
	
	// Set OAuth2 manager
	triggerManager.SetOAuthManager(oauthManager)

	// Register trigger factories
	triggerManager.GetRegistry().Register("schedule", schedule.GetFactory())
	triggerManager.GetRegistry().Register("http", httptrigger.GetFactory())
	triggerManager.GetRegistry().Register("polling", polling.GetFactory())
	triggerManager.GetRegistry().Register("broker", brokertrigger.GetFactory(brokerRegistry))
	triggerManager.GetRegistry().Register("imap", imap.GetFactory())
	triggerManager.GetRegistry().Register("caldav", caldav.GetFactory())
	triggerManager.GetRegistry().Register("carddav", carddav.GetFactory())

	// Use distributed trigger manager if Redis is available
	if lockManager != nil {
		distributedConfig := &triggers.DistributedConfig{
			NodeID:              fmt.Sprintf("webhook-router-%d", time.Now().UnixNano()),
			LeaderElectionTTL:   30 * time.Second,
			TaskLockTTL:         5 * time.Minute,
			LeaderCheckInterval: 10 * time.Second,
		}
		distributedTriggerManager = triggers.NewDistributedManager(triggerManager, redisClient, lockManager, distributedConfig)

		// Start distributed trigger management
		go func() {
			if err := distributedTriggerManager.Start(); err != nil {
				log.Printf("Warning: Failed to start distributed trigger manager: %v", err)
			}
		}()
		fmt.Printf("Distributed Trigger Manager: Started\n")
	} else {
		// Start regular trigger manager
		go func() {
			if err := triggerManager.Start(); err != nil {
				log.Printf("Warning: Failed to start trigger manager: %v", err)
			}
		}()
		fmt.Printf("Trigger Manager: Started\n")
	}

	// Initialize encryptor for sensitive data
	var encryptor *crypto.ConfigEncryptor
	if cfg.EncryptionKey != "" {
		enc, err := crypto.NewConfigEncryptor(cfg.EncryptionKey)
		if err != nil {
			log.Printf("Warning: Failed to create encryptor: %v", err)
		} else {
			encryptor = enc
		}
	}
	
	// Initialize handlers with new interfaces
	h := handlers.New(store, broker, cfg, webFS, authHandler, routingEngine, pipelineEngine, triggerManager, encryptor)
	
	// TODO: Register broker factories with the broker manager when available
	// brokerManager := h.GetBrokerManager()
	// brokerManager.RegisterFactory("rabbitmq", rabbitmq.NewFactory())
	// brokerManager.RegisterFactory("kafka", kafka.NewFactory())
	// brokerManager.RegisterFactory("redis", redisbroker.NewFactory())
	// brokerManager.RegisterFactory("aws", awsbroker.NewFactory())
	// 
	// // Load all broker configurations
	// if err := brokerManager.ReloadBrokers(); err != nil {
	//	log.Printf("Warning: Failed to load broker configurations: %v", err)
	// }

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

	// Webhook endpoints (no auth required, but rate limited)
	if rateLimiter != nil {
		rateLimitedWebhook := rateLimiter.HTTPMiddleware(ratelimit.IPBasedKey)(http.HandlerFunc(h.HandleWebhook))
		router.Handle("/webhook/{endpoint}", rateLimitedWebhook).Methods("POST", "PUT", "PATCH", "DELETE", "GET")
		router.Handle("/webhook", rateLimitedWebhook).Methods("POST", "PUT", "PATCH", "DELETE", "GET")
	} else {
		router.HandleFunc("/webhook/{endpoint}", h.HandleWebhook).Methods("POST", "PUT", "PATCH", "DELETE", "GET")
		router.HandleFunc("/webhook", h.HandleWebhook).Methods("POST", "PUT", "PATCH", "DELETE", "GET")
	}

	// Protected routes - require authentication and rate limiting
	protected := router.NewRoute().Subrouter()
	protected.Use(authHandler.RequireAuth)
	if rateLimiter != nil {
		protected.Use(rateLimiter.HTTPMiddleware(ratelimit.UserBasedKey))
	}

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
	
	// Dead Letter Queue endpoints (protected)
	api.HandleFunc("/dlq/stats", h.GetDLQStats).Methods("GET")
	api.HandleFunc("/dlq/messages", h.ListDLQMessages).Methods("GET")
	api.HandleFunc("/dlq/messages/{id}", h.DeleteDLQMessage).Methods("DELETE")
	api.HandleFunc("/dlq/retry", h.RetryDLQMessages).Methods("POST")
	api.HandleFunc("/dlq/policy", h.ConfigureDLQRetryPolicy).Methods("PUT")

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

	// JWT tokens are stateless and don't need cleanup
	// No session cleanup routine needed

	// Start DLQ retry worker if broker is configured
	var dlqWorkerStop chan struct{}
	if broker != nil {
		dlqWorkerStop = make(chan struct{})
		go func() {
			ticker := time.NewTicker(1 * time.Minute)
			defer ticker.Stop()
			
			for {
				select {
				case <-ticker.C:
					// Retry global DLQ messages
					if h.GetDLQ() != nil {
						if err := h.GetDLQ().RetryFailedMessages(); err != nil {
							log.Printf("Global DLQ retry error: %v", err)
						}
					}
					
					// TODO: Retry per-broker DLQ messages when broker manager available
					// if err := brokerManager.RetryDLQMessages(); err != nil {
					//	log.Printf("Per-broker DLQ retry error: %v", err)
					// }
				case <-dlqWorkerStop:
					return
				}
			}
		}()
		fmt.Println("DLQ Retry Worker: Started")
	}

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

	// Stop DLQ worker
	if dlqWorkerStop != nil {
		close(dlqWorkerStop)
		fmt.Println("DLQ Retry Worker: Stopped")
	}

	// Stop trigger managers
	if distributedTriggerManager != nil {
		if err := distributedTriggerManager.Stop(); err != nil {
			log.Printf("Error stopping distributed trigger manager: %v", err)
		} else {
			fmt.Println("Distributed Trigger Manager: Stopped")
		}
	} else if triggerManager != nil {
		if err := triggerManager.Stop(); err != nil {
			log.Printf("Error stopping trigger manager: %v", err)
		} else {
			fmt.Println("Trigger Manager: Stopped")
		}
	}

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	fmt.Println("Server exited")
}
