package handlers

import (
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"webhook-router/internal/common/logging"
	"webhook-router/internal/signature"
	"webhook-router/internal/storage"
)

var startTime = time.Now()

// Webhook handlers

// HandleWebhook handles incoming webhook requests
// @Summary Process incoming webhook
// @Description Processes incoming webhook requests and routes them to configured destinations
// @Tags webhooks
// @Accept json,xml,plain
// @Produce json
// @Param endpoint path string false "Webhook endpoint name"
// @Param payload body object true "Webhook payload"
// @Success 200 {string} string "OK"
// @Router /webhook/{endpoint} [post]
// @Router /webhook/{endpoint} [put]
// @Router /webhook/{endpoint} [patch]
// @Router /webhook/{endpoint} [delete]
// @Router /webhook/{endpoint} [get]
// @Router /webhook [post]
// @Router /webhook [put]
// @Router /webhook [patch]
// @Router /webhook [delete]
// @Router /webhook [get]
func (h *Handlers) HandleWebhook(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	endpoint := vars["endpoint"]

	// Log webhook details for debugging
	h.logger.Debug("Received webhook request",
		logging.Field{"method", r.Method},
		logging.Field{"endpoint", endpoint},
	)

	// Read the body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		h.handleError(w, err, "Error reading request body", "Failed to read request body", http.StatusBadRequest)
		return
	}

	// Create webhook payload
	payload := h.createWebhookPayload(r, body)

	// Find matching routes
	matchedRoutes, err := h.findMatchingRoutes(endpoint, r.Method)
	if err != nil {
		h.logger.Error("Error finding matching routes", err)
		// Continue processing even if route lookup fails
	}

	// Process each matching route
	if len(matchedRoutes) > 0 {
		for _, route := range matchedRoutes {
			// Verify signature if configured
			if route.SignatureConfig != "" && h.encryptor != nil {
				signatureConfig, err := BuildSignatureAuthConfig(route, h.encryptor)
				if err != nil {
					h.logger.Error("Failed to build signature config", err,
						logging.Field{"route", route.Name},
					)
					http.Error(w, "Invalid signature configuration", http.StatusInternalServerError)
					return
				}
				
				if signatureConfig != nil && signatureConfig.Enabled {
					verifier := signature.NewVerifier(signatureConfig, h.logger)
					if err := verifier.Verify(r, body); err != nil {
						h.logger.Warn("Signature verification failed", 
							logging.Field{"route", route.Name},
							logging.Field{"error", err.Error()},
						)
						
						// Check failure action
						if signatureConfig.FailureAction == "reject" {
							http.Error(w, "Invalid signature", http.StatusUnauthorized)
							return
						}
						// Otherwise, continue processing (log or allow)
					}
				}
			}
			
			payload.RouteID = route.ID
			payload.RouteName = route.Name

			go h.processWebhookRoute(payload, *route)
		}
	} else {
		// No matching routes, send to default queue if broker is available
		go h.publishToDefaultQueue(payload)
	}

	// Always return 200 OK for webhooks
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

func (h *Handlers) processWebhookRoute(payload WebhookPayload, route storage.Route) {
	// Apply filters if they exist
	matches, err := h.processFilters(payload, route.Filters)
	if err != nil {
		h.logger.Error("Error processing filters", err,
			logging.Field{"route", route.Name},
		)
		return
	}
	if !matches {
		h.logger.Debug("Payload does not match filters",
			logging.Field{"route", route.Name},
		)
		return
	}

	// Parse headers
	headers, err := h.parseHeaders(route.Headers)
	if err != nil {
		h.logger.Error("Error parsing headers", err,
			logging.Field{"route", route.Name},
		)
		return
	}

	// Convert payload to JSON
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		h.logger.Error("Error marshaling payload", err)
		return
	}

	// Create message
	message := h.createBrokerMessage("webhook", route.Queue, route.RoutingKey, payloadJSON, headers)

	// Publish message with DLQ support
	if err := h.publishMessageWithDLQ(route.ID, message, route.DestinationBrokerID); err != nil {
		h.logger.Error("Error publishing webhook message", err,
			logging.Field{"route", route.Name},
			logging.Field{"queue", route.Queue},
		)
		return
	}

	h.logger.Info("Successfully published webhook",
		logging.Field{"queue", route.Queue},
		logging.Field{"route", route.Name},
	)
}

func (h *Handlers) publishToDefaultQueue(payload WebhookPayload) {
	if h.broker == nil {
		h.logger.Warn("No broker configured and no matching routes")
		return
	}

	// Convert payload to JSON
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		h.logger.Error("Error marshaling payload for default queue", err)
		return
	}

	defaultQueue := "webhooks"
	if h.config != nil && h.config.DefaultQueue != "" {
		defaultQueue = h.config.DefaultQueue
	}

	// Create message for default queue
	message := h.createBrokerMessage("webhook-default", defaultQueue, defaultQueue, payloadJSON, nil)

	// Publish with DLQ support (use route ID 0 for default queue)
	if err := h.publishMessageWithDLQ(0, message, nil); err != nil {
		h.logger.Error("Error publishing to default queue", err,
			logging.Field{"queue", defaultQueue},
		)
		return
	}

	h.logger.Info("Successfully published webhook to default queue",
		logging.Field{"queue", defaultQueue},
	)
}


// HealthCheck returns the health status of the application
// @Summary Health check
// @Description Returns the health status of the application and its dependencies
// @Tags system
// @Produce json
// @Success 200 {object} map[string]interface{} "Health status"
// @Router /health [get]
func (h *Handlers) HealthCheck(w http.ResponseWriter, r *http.Request) {
	overallHealthy := true
	status := map[string]interface{}{
		"status":    "healthy",
		"timestamp": time.Now(),
		"version":   "1.0.0",
		"uptime":    time.Since(startTime).String(),
	}

	// Check storage health
	storageHealthy := h.checkComponentHealth(status, "storage", h.storage.Health)
	if storageHealthy {
		h.addStorageMetrics(status)
	}
	overallHealthy = overallHealthy && storageHealthy

	// Check broker health if available
	if h.broker != nil {
		brokerHealthy := h.checkComponentHealth(status, "broker", h.broker.Health)
		overallHealthy = overallHealthy && brokerHealthy
	} else {
		status["broker_status"] = "not_configured"
	}

	// Add broker metrics
	h.addBrokerMetrics(status)
	
	// Add system metrics
	h.addSystemMetrics(status)
	
	// Set overall status and send response
	if !overallHealthy {
		status["status"] = "degraded"
		w.WriteHeader(http.StatusServiceUnavailable)
	}

	h.sendJSONResponse(w, status)
}
