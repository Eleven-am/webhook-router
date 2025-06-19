// @title Event Router API
// @version 1.0
// @description Enterprise-grade event processing and routing platform with 7 event sources
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
	"webhook-router/internal/common/logging"
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

// Embed the frontend build
//
//go:embed frontend/dist/*
var frontendFS embed.FS

func main() {
	// Set up CPU usage
	err := godotenv.Load()
	nCPU := runtime.NumCPU()
	runtime.GOMAXPROCS(nCPU)
	fmt.Printf("Number of CPUs: %d\n", nCPU)

	// Load configuration
	cfg := config.Load()

	// Initialize logging
	logging.InitGlobalLogger()
	defer logging.MustSync() // Flush logs on exit

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		logging.Error("Configuration validation failed", err)
	os.Exit(1)
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
		logging.Error("Failed to initialize storage", err)
	os.Exit(1)
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
	var lockManager locks.LockManagerInterface

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
			logging.Warn("Failed to initialize Redis client", logging.Field{"error", err})
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
			var err error
			lockManager, err = locks.NewDistributedLockManager(redisClient)
			if err != nil {
				log.Fatalf("Failed to create distributed lock manager: %v", err)
			}
			defer lockManager.Close()

			fmt.Printf("Rate Limiting: Enabled (%d req/%s)\n", defaultLimit, window)
			fmt.Printf("Distributed Locks: Enabled\n")
		}
	} else {
		fmt.Printf("Redis: Not configured (rate limiting and distributed locks disabled)\n")
	}

	// Initialize auth with storage interface
	authHandler := auth.New(store, cfg, redisClient)

	// Initialize encryption if key is available
	var encryptor *crypto.ConfigEncryptor
	if encryptionKey := cfg.EncryptionKey; encryptionKey != "" {
		var err error
		encryptor, err = crypto.NewConfigEncryptor(encryptionKey)
		if err != nil {
			logging.Warn("Failed to initialize encryption", logging.Field{"error", err})
		} else {
			logging.Info("Configuration encryption enabled")
		}
	}

	// Initialize OAuth2 manager
	var oauthManager *oauth2.Manager
	if redisClient != nil {
		// Use Redis for distributed token storage
		tokenStorage := oauth2.NewRedisTokenStorage(redisClient)
		oauthManager = oauth2.NewManager(tokenStorage)
		logging.Info("OAuth2 manager initialized", logging.Field{"storage", "Redis"})
	} else {
		// Use database for token storage with optional encryption
		var tokenStorage oauth2.TokenStorage
		if encryptor != nil {
			tokenStorage = oauth2.NewSecureDBTokenStorage(store, encryptor)
			logging.Info("OAuth2 manager initialized", logging.Field{"storage", "Database (encrypted)"})
		} else {
			tokenStorage = oauth2.NewDBTokenStorage(store)
			logging.Info("OAuth2 manager initialized", logging.Field{"storage", "Database (plaintext)"})
		}
		oauthManager = oauth2.NewManager(tokenStorage)
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
				logging.Info("Using RabbitMQ URL from database settings")
			}
		} else {
			logging.Info("Using RabbitMQ URL from environment variable")
		}

		if rmqURL != "" && rmqURL != "amqp://guest:guest@localhost:5672/" {
			rmqConfig := &rabbitmq.Config{
				URL:      rmqURL,
				PoolSize: 5,
			}

			broker, err = brokerRegistry.Create("rabbitmq", rmqConfig)
			if err != nil {
				logging.Warn("Failed to initialize RabbitMQ broker", logging.Field{"error", err})
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
				logging.Warn("Failed to initialize Kafka broker", logging.Field{"error", err})
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
				logging.Warn("Failed to initialize Redis broker", logging.Field{"error", err})
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
				logging.Warn("Failed to initialize AWS broker", logging.Field{"error", err})
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
				logging.Warn("Failed to initialize GCP Pub/Sub broker", logging.Field{"error", err})
			} else {
				defer broker.Close()
				fmt.Printf("GCP Pub/Sub: Connected\n")
			}
		}

	default:
		logging.Warn("Unknown broker type", logging.Field{"type", brokerType})
	}

	if broker == nil {
		logging.Info("No broker configured", logging.Field{"note", "Brokers can be configured through the admin interface"})
	}

	// Initialize routing engine
	ruleEngine := routing.NewBasicRuleEngine()
	destManager := routing.NewBasicDestinationManager()
	routingEngine := routing.NewBasicRouter(ruleEngine, destManager)

	// Start routing engine
	if err := routingEngine.Start(context.Background()); err != nil {
		logging.Warn("Failed to start routing engine", logging.Field{"error", err})
	}
	fmt.Printf("Routing Engine: Started\n")

	// Initialize pipeline engine
	pipelineEngine := pipeline.NewBasicPipelineEngine()

	// Register stage factories
	pipelineEngine.RegisterStageFactory("json_transform", &stages.JSONTransformStageFactory{})
	pipelineEngine.RegisterStageFactory("xml_transform", &stages.XMLTransformStageFactory{})
	pipelineEngine.RegisterStageFactory("template", &stages.TemplateStageFactory{})
	pipelineEngine.RegisterStageFactory("js_transform", &stages.JSTransformStageFactory{})
	pipelineEngine.RegisterStageFactory("validate", &stages.ValidationStageFactory{})
	pipelineEngine.RegisterStageFactory("filter", &stages.FilterStageFactory{})
	pipelineEngine.RegisterStageFactory("enrich", stages.NewEnrichmentStageFactory(redisClient))
	pipelineEngine.RegisterStageFactory("aggregate", &stages.AggregationStageFactory{})
	pipelineEngine.RegisterStageFactory("branch", stages.NewBranchStageFactory(pipelineEngine))

	// Start pipeline engine
	if err := pipelineEngine.Start(context.Background()); err != nil {
		logging.Warn("Failed to start pipeline engine", logging.Field{"error", err})
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
	
	// Set Pipeline engine for response transformations
	pipelineAdapter := pipeline.NewEngineAdapter(pipelineEngine)
	triggerManager.SetPipelineEngine(pipelineAdapter)

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
				logging.Warn("Failed to start distributed trigger manager", logging.Field{"error", err})
			}
		}()
		logging.Info("Distributed trigger manager started")
	} else {
		// Start regular trigger manager
		go func() {
			if err := triggerManager.Start(); err != nil {
				logging.Warn("Failed to start trigger manager", logging.Field{"error", err})
			}
		}()
		logging.Info("Trigger manager started")
	}

	// Encryptor is already initialized above if EncryptionKey is set

	// Initialize handlers with new interfaces
	h := handlers.New(store, broker, cfg, frontendFS, authHandler, routingEngine, pipelineEngine, triggerManager, encryptor)

	// Register broker factories with the broker manager
	brokerManager := h.GetBrokerManager()
	brokerManager.RegisterFactory("rabbitmq", rabbitmq.GetFactory())
	brokerManager.RegisterFactory("kafka", kafka.GetFactory())
	brokerManager.RegisterFactory("redis", redisbroker.GetFactory())
	brokerManager.RegisterFactory("aws", aws.GetFactory())
	brokerManager.RegisterFactory("gcp", gcp.GetFactory())

	// Brokers are now loaded on-demand when first accessed
	logging.Info("Broker manager initialized", logging.Field{"mode", "lazy loading"})
	
	// Set broker manager on trigger manager for DLQ support
	triggerManager.SetBrokerManager(brokerManager)

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

	// Serve SPA for all non-API routes (protected)
	// This must be the last route to catch all paths
	protected.PathPrefix("/").Handler(h.ServeSPA())

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
					// Retry per-broker DLQ messages
					if err := brokerManager.RetryDLQMessages(); err != nil {
						logging.Warn("Per-broker DLQ retry error", logging.Field{"error", err})
					}
				case <-dlqWorkerStop:
					return
				}
			}
		}()
		logging.Info("DLQ retry worker started")
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
		logging.Info("Server starting", logging.Field{"port", cfg.Port})
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logging.Error("Server failed to start", err)
		os.Exit(1)
		}
	}()

	// Wait for interrupt signal to gracefully shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	logging.Info("Shutting down server...")

	// Graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Stop routing engine
	if err := routingEngine.Stop(); err != nil {
		logging.Warn("Error stopping routing engine", logging.Field{"error", err})
	} else {
		logging.Info("Routing engine stopped")
	}

	// Stop pipeline engine
	if err := pipelineEngine.Stop(); err != nil {
		logging.Warn("Error stopping pipeline engine", logging.Field{"error", err})
	} else {
		logging.Info("Pipeline engine stopped")
	}

	// Stop DLQ worker
	if dlqWorkerStop != nil {
		close(dlqWorkerStop)
		logging.Info("DLQ retry worker stopped")
	}

	// Stop trigger managers
	if distributedTriggerManager != nil {
		if err := distributedTriggerManager.Stop(); err != nil {
			logging.Warn("Error stopping distributed trigger manager", logging.Field{"error", err})
		} else {
			logging.Info("Distributed trigger manager stopped")
		}
	} else if triggerManager != nil {
		if err := triggerManager.Stop(); err != nil {
			logging.Warn("Error stopping trigger manager", logging.Field{"error", err})
		} else {
			logging.Info("Trigger manager stopped")
		}
	}

	if err := srv.Shutdown(ctx); err != nil {
		logging.Error("Server forced to shutdown", err)
		os.Exit(1)
	}

	logging.Info("Server exited")
}
