package app

import (
	"net/http"

	"github.com/gorilla/mux"
	httpSwagger "github.com/swaggo/http-swagger"
	"webhook-router/internal/common/ratelimit"
	"webhook-router/internal/handlers"
	"webhook-router/internal/middleware"
)

// SetupRoutes configures all HTTP routes for the application
func SetupRoutes(router *mux.Router, h *handlers.Handlers, authMiddleware func(http.Handler) http.Handler, rateLimiter ratelimit.Limiter) {
	// Add logging middleware to all routes
	router.Use(middleware.LoggingMiddleware)
	// API Auth routes (no auth required for login and create account)
	router.HandleFunc("/api/auth/create", h.HandleCreateAccount).Methods("POST")
	router.HandleFunc("/api/auth/login", h.HandleLogin).Methods("POST")
	router.HandleFunc("/api/auth/logout", h.HandleLogout).Methods("POST")
	router.HandleFunc("/api/auth/forgot-password", h.HandleForgotPassword).Methods("POST")
	router.HandleFunc("/api/auth/reset-password", h.HandleResetPassword).Methods("POST")
	router.Handle("/api/auth/me", authMiddleware(http.HandlerFunc(h.HandleGetCurrentUser))).Methods("GET")

	// Health check (no auth required)
	router.HandleFunc("/health", h.HealthCheck).Methods("GET")

	// Swagger UI (no auth required)
	router.PathPrefix("/swagger/").Handler(httpSwagger.WrapHandler)

	// Protected routes - require authentication and rate limiting
	protected := router.NewRoute().Subrouter()
	protected.Use(authMiddleware)
	if rateLimiter != nil {
		protected.Use(ratelimit.HTTPMiddleware(rateLimiter, ratelimit.UserKey))
	}

	// API endpoints for route management (protected)
	api := protected.PathPrefix("/api").Subrouter()

	// Auth endpoints (protected)
	api.HandleFunc("/auth/change-password", h.HandleChangePassword).Methods("POST")

	// Settings endpoints (protected)
	api.HandleFunc("/settings", h.GetSettings).Methods("GET")
	api.HandleFunc("/settings", h.UpdateSettings).Methods("POST")

	// Statistics endpoints (protected)
	api.HandleFunc("/stats", h.GetStats).Methods("GET")
	api.HandleFunc("/stats/dashboard", h.GetDashboardStats).Methods("GET")
	api.HandleFunc("/stats/trigger/{id}", h.GetTriggerStats).Methods("GET")

	// Logs endpoints (protected)
	api.HandleFunc("/logs", h.GetExecutionLogs).Methods("GET")
	api.HandleFunc("/logs/{id}", h.GetExecutionLog).Methods("GET")

	// Broker management endpoints (protected)
	api.HandleFunc("/brokers", h.GetBrokers).Methods("GET")
	api.HandleFunc("/brokers", h.CreateBroker).Methods("POST")
	api.HandleFunc("/brokers/{id}", h.GetBroker).Methods("GET")
	api.HandleFunc("/brokers/{id}", h.UpdateBroker).Methods("PUT")
	api.HandleFunc("/brokers/{id}", h.DeleteBroker).Methods("DELETE")
	api.HandleFunc("/brokers/{id}/test", h.TestBroker).Methods("POST")
	api.HandleFunc("/brokers/test", h.TestBrokerConfig).Methods("POST")
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
	api.HandleFunc("/pipelines/execute", h.ExecutePipeline).Methods("POST")

	// OAuth2 management endpoints (protected with rate limiting)
	// Use per-user rate limiting for OAuth2 endpoints to prevent brute force attacks
	if rateLimiter != nil {
		// Create a stricter rate limiter for OAuth2 endpoints
		// Allow only 10 requests per minute per user for OAuth2 operations
		oauth2Config := ratelimit.NewConfigBuilder().
			WithRPS(1).    // 1 request per second = 60 per minute, but burst controls actual rate
			WithBurst(10). // Allow burst of 10 requests
			WithKeyPrefix("oauth2:").
			ForUsers().
			Build()

		// Create a local rate limiter for OAuth2 endpoints
		oauth2Limiter, _ := ratelimit.NewLocalLimiter(oauth2Config)

		oauth2Middleware := ratelimit.HTTPMiddleware(oauth2Limiter, func(r *http.Request) string {
			// Rate limit by user ID from context
			if userID, ok := r.Context().Value("userID").(string); ok {
				return userID
			}
			// Fallback to IP-based rate limiting
			return ratelimit.IPKey(r)
		})

		// Apply rate limiting to OAuth2 endpoints
		api.Handle("/oauth2/services", oauth2Middleware(http.HandlerFunc(h.ListOAuth2Services))).Methods("GET")
		api.Handle("/oauth2/services", oauth2Middleware(http.HandlerFunc(h.CreateOAuth2Service))).Methods("POST")
		api.Handle("/oauth2/services/{id}", oauth2Middleware(http.HandlerFunc(h.GetOAuth2Service))).Methods("GET")
		api.Handle("/oauth2/services/{id}", oauth2Middleware(http.HandlerFunc(h.UpdateOAuth2Service))).Methods("PUT")
		api.Handle("/oauth2/services/{id}", oauth2Middleware(http.HandlerFunc(h.DeleteOAuth2Service))).Methods("DELETE")
		api.Handle("/oauth2/services/{id}/refresh", oauth2Middleware(http.HandlerFunc(h.RefreshOAuth2Token))).Methods("POST")
	} else {
		// No rate limiting configured
		api.HandleFunc("/oauth2/services", h.ListOAuth2Services).Methods("GET")
		api.HandleFunc("/oauth2/services", h.CreateOAuth2Service).Methods("POST")
		api.HandleFunc("/oauth2/services/{id}", h.GetOAuth2Service).Methods("GET")
		api.HandleFunc("/oauth2/services/{id}", h.UpdateOAuth2Service).Methods("PUT")
		api.HandleFunc("/oauth2/services/{id}", h.DeleteOAuth2Service).Methods("DELETE")
		api.HandleFunc("/oauth2/services/{id}/refresh", h.RefreshOAuth2Token).Methods("POST")
	}

	// Dead Letter Queue endpoints (protected)
	api.HandleFunc("/dlq/stats", h.GetDLQStats).Methods("GET")
	api.HandleFunc("/dlq/messages", h.ListDLQMessages).Methods("GET")
	api.HandleFunc("/dlq/messages/{id}", h.DeleteDLQMessage).Methods("DELETE")
	api.HandleFunc("/dlq/retry", h.RetryDLQMessages).Methods("POST")
	api.HandleFunc("/dlq/policy", h.ConfigureDLQRetryPolicy).Methods("PUT")

	// HTTP Trigger endpoints (no auth required, but rate limited)
	// Username-based routing for HTTP triggers - placed after API routes to avoid conflicts
	if rateLimiter != nil {
		rateLimitedHTTPTrigger := ratelimit.HTTPMiddleware(rateLimiter, ratelimit.IPKey)(http.HandlerFunc(h.HandleHTTPTrigger))
		router.Handle("/{username}/{path:.*}", rateLimitedHTTPTrigger).Methods("POST", "PUT", "PATCH", "DELETE", "GET")
	} else {
		router.HandleFunc("/{username}/{path:.*}", h.HandleHTTPTrigger).Methods("POST", "PUT", "PATCH", "DELETE", "GET")
	}

	// Serve SPA for all non-API routes (no auth required for initial load)
	// This must be the last route to catch all paths
	router.PathPrefix("/").Handler(h.ServeSPA())
}
