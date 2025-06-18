package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"webhook-router/internal/brokers"
	awsbroker "webhook-router/internal/brokers/aws"
	"webhook-router/internal/brokers/kafka"
	"webhook-router/internal/brokers/rabbitmq"
	redisbroker "webhook-router/internal/brokers/redis"
	"webhook-router/internal/storage"

	"github.com/gorilla/mux"
)

// Broker management handlers

// GetBrokers returns all configured brokers
// @Summary Get all message brokers
// @Description Returns a list of all configured message broker connections
// @Tags brokers
// @Produce json
// @Security SessionAuth
// @Success 200 {array} storage.BrokerConfig "List of broker configurations"
// @Failure 500 {string} string "Internal server error"
// @Router /brokers [get]
func (h *Handlers) GetBrokers(w http.ResponseWriter, r *http.Request) {
	brokers, err := h.storage.GetBrokers()
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get brokers: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(brokers)
}

// GetBroker returns a specific broker configuration
// @Summary Get message broker
// @Description Returns a specific message broker configuration by ID
// @Tags brokers
// @Produce json
// @Security SessionAuth
// @Param id path int true "Broker ID"
// @Success 200 {object} storage.BrokerConfig "Broker configuration"
// @Failure 400 {string} string "Invalid broker ID"
// @Failure 404 {string} string "Broker not found"
// @Router /brokers/{id} [get]
func (h *Handlers) GetBroker(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, err := strconv.Atoi(vars["id"])
	if err != nil {
		http.Error(w, "Invalid broker ID", http.StatusBadRequest)
		return
	}

	broker, err := h.storage.GetBroker(id)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get broker: %v", err), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(broker)
}

// CreateBroker creates a new broker configuration
// @Summary Create message broker
// @Description Creates a new message broker configuration
// @Tags brokers
// @Accept json
// @Produce json
// @Security SessionAuth
// @Param broker body storage.BrokerConfig true "Broker configuration"
// @Success 201 {object} storage.BrokerConfig "Created broker configuration"
// @Failure 400 {string} string "Invalid JSON"
// @Failure 500 {string} string "Internal server error"
// @Router /brokers [post]
func (h *Handlers) CreateBroker(w http.ResponseWriter, r *http.Request) {
	var broker storage.BrokerConfig
	if err := json.NewDecoder(r.Body).Decode(&broker); err != nil {
		http.Error(w, fmt.Sprintf("Invalid JSON: %v", err), http.StatusBadRequest)
		return
	}

	// Set current timestamp
	broker.CreatedAt = time.Now()
	broker.UpdatedAt = time.Now()

	if err := h.storage.CreateBroker(&broker); err != nil {
		http.Error(w, fmt.Sprintf("Failed to create broker: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(broker)
}

// UpdateBroker updates an existing broker configuration
// @Summary Update message broker
// @Description Updates an existing message broker configuration
// @Tags brokers
// @Accept json
// @Produce json
// @Security SessionAuth
// @Param id path int true "Broker ID"
// @Param broker body storage.BrokerConfig true "Broker configuration"
// @Success 200 {object} storage.BrokerConfig "Updated broker configuration"
// @Failure 400 {string} string "Invalid JSON or broker ID"
// @Failure 500 {string} string "Failed to update broker"
// @Router /brokers/{id} [put]
func (h *Handlers) UpdateBroker(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, err := strconv.Atoi(vars["id"])
	if err != nil {
		http.Error(w, "Invalid broker ID", http.StatusBadRequest)
		return
	}

	var broker storage.BrokerConfig
	if err := json.NewDecoder(r.Body).Decode(&broker); err != nil {
		http.Error(w, fmt.Sprintf("Invalid JSON: %v", err), http.StatusBadRequest)
		return
	}

	broker.ID = id
	broker.UpdatedAt = time.Now()

	if err := h.storage.UpdateBroker(&broker); err != nil {
		http.Error(w, fmt.Sprintf("Failed to update broker: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(broker)
}

// DeleteBroker removes a broker configuration
// @Summary Delete message broker
// @Description Removes a message broker configuration
// @Tags brokers
// @Security SessionAuth
// @Param id path int true "Broker ID"
// @Success 204 "No Content"
// @Failure 400 {string} string "Invalid broker ID"
// @Failure 500 {string} string "Failed to delete broker"
// @Router /brokers/{id} [delete]
func (h *Handlers) DeleteBroker(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, err := strconv.Atoi(vars["id"])
	if err != nil {
		http.Error(w, "Invalid broker ID", http.StatusBadRequest)
		return
	}

	if err := h.storage.DeleteBroker(id); err != nil {
		http.Error(w, fmt.Sprintf("Failed to delete broker: %v", err), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// TestBroker tests connectivity to a broker
// @Summary Test broker connection
// @Description Tests connectivity to a message broker
// @Tags brokers
// @Security SessionAuth
// @Param id path int true "Broker ID"
// @Success 200 {object} map[string]interface{} "Test result"
// @Failure 400 {string} string "Invalid broker ID or configuration"
// @Failure 404 {string} string "Broker not found"
// @Failure 503 {string} string "Broker health check failed"
// @Router /brokers/{id}/test [post]
func (h *Handlers) TestBroker(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, err := strconv.Atoi(vars["id"])
	if err != nil {
		http.Error(w, "Invalid broker ID", http.StatusBadRequest)
		return
	}

	brokerConfig, err := h.storage.GetBroker(id)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get broker: %v", err), http.StatusNotFound)
		return
	}

	// Create a registry and test the broker
	registry := brokers.NewRegistry()

	// Register our known broker factories
	registry.Register("rabbitmq", rabbitmq.GetFactory())
	registry.Register("kafka", kafka.GetFactory())
	registry.Register("redis", redisbroker.GetFactory())
	registry.Register("aws", awsbroker.GetFactory())

	// Convert the stored config map to the appropriate broker config type
	var brokerConfigInterface brokers.BrokerConfig
	switch brokerConfig.Type {
	case "rabbitmq":
		rmqConfig := &rabbitmq.Config{}
		configMap := brokerConfig.Config
		if configMap != nil {
			if url, exists := configMap["url"]; exists {
				rmqConfig.URL = fmt.Sprintf("%v", url)
			}
			if poolSize, exists := configMap["pool_size"]; exists {
				if ps, ok := poolSize.(float64); ok {
					rmqConfig.PoolSize = int(ps)
				}
			}
		}
		brokerConfigInterface = rmqConfig
	case "kafka":
		kafkaConfig := &kafka.Config{}
		configMap := brokerConfig.Config
		if configMap != nil {
			if brokers, exists := configMap["brokers"]; exists {
				// Handle both string and array formats
				switch v := brokers.(type) {
				case string:
					kafkaConfig.Brokers = []string{v}
				case []interface{}:
					for _, b := range v {
						kafkaConfig.Brokers = append(kafkaConfig.Brokers, fmt.Sprintf("%v", b))
					}
				}
			}
			if groupID, exists := configMap["group_id"]; exists {
				kafkaConfig.GroupID = fmt.Sprintf("%v", groupID)
			}
			if clientID, exists := configMap["client_id"]; exists {
				kafkaConfig.ClientID = fmt.Sprintf("%v", clientID)
			}
		}
		brokerConfigInterface = kafkaConfig
	case "redis":
		redisConfig := &redisbroker.Config{}
		configMap := brokerConfig.Config
		if configMap != nil {
			if address, exists := configMap["address"]; exists {
				redisConfig.Address = fmt.Sprintf("%v", address)
			}
			if password, exists := configMap["password"]; exists {
				redisConfig.Password = fmt.Sprintf("%v", password)
			}
			if db, exists := configMap["db"]; exists {
				if dbNum, ok := db.(float64); ok {
					redisConfig.DB = int(dbNum)
				}
			}
			if poolSize, exists := configMap["pool_size"]; exists {
				if ps, ok := poolSize.(float64); ok {
					redisConfig.PoolSize = int(ps)
				}
			}
		}
		brokerConfigInterface = redisConfig
	case "aws":
		awsConfig := &awsbroker.Config{}
		configMap := brokerConfig.Config
		if configMap != nil {
			if region, exists := configMap["region"]; exists {
				awsConfig.Region = fmt.Sprintf("%v", region)
			}
			if accessKeyID, exists := configMap["access_key_id"]; exists {
				awsConfig.AccessKeyID = fmt.Sprintf("%v", accessKeyID)
			}
			if secretAccessKey, exists := configMap["secret_access_key"]; exists {
				awsConfig.SecretAccessKey = fmt.Sprintf("%v", secretAccessKey)
			}
			if queueURL, exists := configMap["queue_url"]; exists {
				awsConfig.QueueURL = fmt.Sprintf("%v", queueURL)
			}
			if topicArn, exists := configMap["topic_arn"]; exists {
				awsConfig.TopicArn = fmt.Sprintf("%v", topicArn)
			}
		}
		brokerConfigInterface = awsConfig
	default:
		result := map[string]interface{}{
			"status": "error",
			"error":  fmt.Sprintf("Unsupported broker type for testing: %s", brokerConfig.Type),
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(result)
		return
	}

	testBroker, err := registry.Create(brokerConfig.Type, brokerConfigInterface)
	if err != nil {
		result := map[string]interface{}{
			"status": "error",
			"error":  fmt.Sprintf("Failed to create broker: %v", err),
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(result)
		return
	}
	defer testBroker.Close()

	// Test broker health
	if err := testBroker.Health(); err != nil {
		result := map[string]interface{}{
			"status": "error",
			"error":  fmt.Sprintf("Broker health check failed: %v", err),
		}

		// Update broker health status in database
		brokerConfig.HealthStatus = "error"
		brokerConfig.LastHealthCheck = &time.Time{}
		*brokerConfig.LastHealthCheck = time.Now()
		h.storage.UpdateBroker(brokerConfig)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(result)
		return
	}

	// Update broker health status in database
	brokerConfig.HealthStatus = "healthy"
	brokerConfig.LastHealthCheck = &time.Time{}
	*brokerConfig.LastHealthCheck = time.Now()
	h.storage.UpdateBroker(brokerConfig)

	result := map[string]interface{}{
		"status":  "healthy",
		"message": "Broker connection successful",
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// GetAvailableBrokerTypes returns the list of supported broker types
// @Summary Get broker types
// @Description Returns a list of supported message broker types
// @Tags brokers
// @Produce json
// @Security SessionAuth
// @Success 200 {object} map[string]interface{} "List of broker types"
// @Router /brokers/types [get]
func (h *Handlers) GetAvailableBrokerTypes(w http.ResponseWriter, r *http.Request) {
	// Return a static list of supported broker types
	types := []string{"rabbitmq", "kafka", "redis", "aws"}

	result := map[string]interface{}{
		"types": types,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}
