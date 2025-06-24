package handlers

import (
	"encoding/json"
	"net/http"
	"net/url"
	"time"

	"github.com/gorilla/mux"
	"github.com/lucsky/cuid"
	"webhook-router/internal/common/logging"
	"webhook-router/internal/oauth2"
	"webhook-router/internal/storage"
)

// OAuth2ServiceRequest represents the request to create/update an OAuth2 service
type OAuth2ServiceRequest struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	ClientID     string   `json:"client_id"`
	ClientSecret string   `json:"client_secret"`
	TokenURL     string   `json:"token_url"`
	AuthURL      string   `json:"auth_url,omitempty"`
	RedirectURL  string   `json:"redirect_url,omitempty"`
	Scopes       []string `json:"scopes,omitempty"`
	GrantType    string   `json:"grant_type"`
	Username     string   `json:"username,omitempty"`
	Password     string   `json:"password,omitempty"`
	RefreshToken string   `json:"refresh_token,omitempty"`
}

// OAuth2ServiceResponse represents an OAuth2 service in API responses
type OAuth2ServiceResponse struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	ClientID    string     `json:"client_id"`
	TokenURL    string     `json:"token_url"`
	AuthURL     string     `json:"auth_url,omitempty"`
	RedirectURL string     `json:"redirect_url,omitempty"`
	Scopes      []string   `json:"scopes,omitempty"`
	GrantType   string     `json:"grant_type"`
	Username    string     `json:"username,omitempty"`
	HasToken    bool       `json:"has_token"`
	TokenExpiry *time.Time `json:"token_expiry,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// ListOAuth2Services returns all OAuth2 services for the authenticated user
func (h *Handlers) ListOAuth2Services(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID, ok := ctx.Value("userID").(string)
	if !ok {
		h.sendJSONError(w, nil, "Failed to get user ID from context", "Authentication required", http.StatusUnauthorized)
		return
	}

	// Get all OAuth2 services for the user
	services, err := h.storage.ListOAuth2Services(userID)
	if err != nil {
		h.sendJSONError(w, err, "Failed to get OAuth2 services", "Failed to retrieve OAuth2 services", http.StatusInternalServerError)
		return
	}

	// Convert to response format
	response := make([]OAuth2ServiceResponse, 0, len(services))
	for _, svc := range services {
		// Check if service has a valid token
		token, _ := h.oauth2Manager.GetToken(ctx, svc.ID)
		hasToken := token != nil && !token.IsExpired()

		resp := OAuth2ServiceResponse{
			ID:          svc.ID,
			Name:        svc.Name,
			ClientID:    svc.ClientID,
			TokenURL:    svc.TokenURL,
			AuthURL:     svc.AuthURL,
			RedirectURL: svc.RedirectURL,
			Scopes:      svc.Scopes,
			GrantType:   svc.GrantType,
			HasToken:    hasToken,
			CreatedAt:   svc.CreatedAt,
			UpdatedAt:   svc.UpdatedAt,
		}

		if hasToken && token != nil {
			resp.TokenExpiry = &token.Expiry
		}

		response = append(response, resp)
	}

	h.sendJSONResponse(w, response)
}

// CreateOAuth2Service creates a new OAuth2 service configuration
func (h *Handlers) CreateOAuth2Service(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID, ok := ctx.Value("userID").(string)
	if !ok {
		h.sendJSONError(w, nil, "Failed to get user ID from context", "Authentication required", http.StatusUnauthorized)
		return
	}

	var req OAuth2ServiceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.sendJSONError(w, nil, "Invalid request body", "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate required fields
	if req.Name == "" {
		h.sendJSONError(w, nil, "Name is required", "Name is required", http.StatusBadRequest)
		return
	}
	if req.ClientID == "" {
		h.sendJSONError(w, nil, "Client ID is required", "Client ID is required", http.StatusBadRequest)
		return
	}
	if req.ClientSecret == "" {
		h.sendJSONError(w, nil, "Client secret is required", "Client secret is required", http.StatusBadRequest)
		return
	}
	if req.TokenURL == "" {
		h.sendJSONError(w, nil, "Token URL is required", "Token URL is required", http.StatusBadRequest)
		return
	}
	if req.GrantType == "" {
		h.sendJSONError(w, nil, "Grant type is required", "Grant type is required", http.StatusBadRequest)
		return
	}

	// Validate grant type
	validGrantTypes := map[string]bool{
		"client_credentials": true,
		"password":           true,
		"authorization_code": true,
		"refresh_token":      true,
	}
	if !validGrantTypes[req.GrantType] {
		h.sendJSONError(w, nil, "Invalid grant type. Must be one of: client_credentials, password, authorization_code, refresh_token", "Invalid grant type. Must be one of: client_credentials, password, authorization_code, refresh_token", http.StatusBadRequest)
		return
	}

	// Validate grant type specific fields
	if req.GrantType == "password" {
		if req.Username == "" {
			h.sendJSONError(w, nil, "Username is required for password grant type", "Username is required for password grant type", http.StatusBadRequest)
			return
		}
		if req.Password == "" {
			h.sendJSONError(w, nil, "Password is required for password grant type", "Password is required for password grant type", http.StatusBadRequest)
			return
		}
	}

	if req.GrantType == "authorization_code" {
		if req.AuthURL == "" {
			h.sendJSONError(w, nil, "Auth URL is required for authorization_code grant type", "Auth URL is required for authorization_code grant type", http.StatusBadRequest)
			return
		}
		if req.RedirectURL == "" {
			h.sendJSONError(w, nil, "Redirect URL is required for authorization_code grant type", "Redirect URL is required for authorization_code grant type", http.StatusBadRequest)
			return
		}
	}

	// Validate URLs
	if _, err := url.Parse(req.TokenURL); err != nil {
		h.sendJSONError(w, nil, "Invalid token URL format", "Invalid token URL format", http.StatusBadRequest)
		return
	}
	if req.AuthURL != "" {
		if _, err := url.Parse(req.AuthURL); err != nil {
			h.sendJSONError(w, nil, "Invalid auth URL format", "Invalid auth URL format", http.StatusBadRequest)
			return
		}
	}
	if req.RedirectURL != "" {
		if _, err := url.Parse(req.RedirectURL); err != nil {
			h.sendJSONError(w, nil, "Invalid redirect URL format", "Invalid redirect URL format", http.StatusBadRequest)
			return
		}
	}

	// Generate ID if not provided
	if req.ID == "" {
		req.ID = cuid.New()
	}

	// Create OAuth2 config
	config := &oauth2.Config{
		ClientID:     req.ClientID,
		ClientSecret: req.ClientSecret,
		TokenURL:     req.TokenURL,
		AuthURL:      req.AuthURL,
		RedirectURL:  req.RedirectURL,
		Scopes:       req.Scopes,
		GrantType:    req.GrantType,
		Username:     req.Username,
		Password:     req.Password,
	}

	// Register with OAuth2 manager
	if err := h.oauth2Manager.RegisterService(req.ID, config); err != nil {
		h.logger.Error("Failed to register OAuth2 service", err, logging.Field{"serviceID", req.ID})
		h.sendJSONError(w, nil, "Failed to register OAuth2 service", "Failed to register OAuth2 service", http.StatusInternalServerError)
		return
	}

	// If refresh token is provided, set it in the OAuth2 manager
	if req.RefreshToken != "" {
		if err := h.oauth2Manager.SetRefreshToken(req.ID, req.RefreshToken); err != nil {
			h.logger.Error("Failed to set refresh token", err, logging.Field{"serviceID", req.ID})
			// Continue even if setting refresh token fails
		}
	}

	// Save to storage
	oauth2Service := &storage.OAuth2Service{
		ID:           req.ID,
		Name:         req.Name,
		ClientID:     req.ClientID,
		ClientSecret: req.ClientSecret, // Should be encrypted by storage layer
		TokenURL:     req.TokenURL,
		AuthURL:      req.AuthURL,
		RedirectURL:  req.RedirectURL,
		Scopes:       req.Scopes,
		GrantType:    req.GrantType,
		UserID:       userID,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	if err := h.storage.CreateOAuth2Service(oauth2Service); err != nil {
		h.logger.Error("Failed to save OAuth2 service", err, logging.Field{"serviceID", req.ID})
		h.sendJSONError(w, nil, "Failed to save OAuth2 service", "Failed to save OAuth2 service", http.StatusInternalServerError)
		return
	}

	// Audit log
	h.logger.Info("OAuth2 service created",
		logging.Field{"oauth2_service_id", oauth2Service.ID},
		logging.Field{"oauth2_service_name", oauth2Service.Name},
		logging.Field{"user_id", userID},
		logging.Field{"client_id", req.ClientID},
		logging.Field{"grant_type", req.GrantType},
		logging.Field{"action", "oauth2_service_create"})

	// Return created service
	response := OAuth2ServiceResponse{
		ID:          oauth2Service.ID,
		Name:        oauth2Service.Name,
		ClientID:    oauth2Service.ClientID,
		TokenURL:    oauth2Service.TokenURL,
		AuthURL:     oauth2Service.AuthURL,
		RedirectURL: oauth2Service.RedirectURL,
		Scopes:      oauth2Service.Scopes,
		GrantType:   req.GrantType,
		Username:    req.Username,
		HasToken:    req.RefreshToken != "",
		CreatedAt:   oauth2Service.CreatedAt,
		UpdatedAt:   oauth2Service.UpdatedAt,
	}

	h.sendJSONResponse(w, response)
}

// GetOAuth2Service returns a single OAuth2 service by ID
func (h *Handlers) GetOAuth2Service(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID, ok := ctx.Value("userID").(string)
	if !ok {
		h.sendJSONError(w, nil, "Failed to get user ID from context", "Authentication required", http.StatusUnauthorized)
		return
	}

	vars := mux.Vars(r)
	serviceID := vars["id"]

	// Get service from storage
	service, err := h.storage.GetOAuth2Service(serviceID)
	if err != nil {
		h.logger.Error("Failed to get OAuth2 service", err, logging.Field{"serviceID", serviceID})
		h.sendJSONError(w, nil, "OAuth2 service not found", "OAuth2 service not found", http.StatusNotFound)
		return
	}

	// Check ownership
	if service.UserID != userID {
		h.sendJSONError(w, nil, "OAuth2 service not found", "OAuth2 service not found", http.StatusNotFound)
		return
	}

	// Check if service has a valid token
	token, _ := h.oauth2Manager.GetToken(ctx, service.ID)
	hasToken := token != nil && !token.IsExpired()

	response := OAuth2ServiceResponse{
		ID:          service.ID,
		Name:        service.Name,
		ClientID:    service.ClientID,
		TokenURL:    service.TokenURL,
		AuthURL:     service.AuthURL,
		RedirectURL: service.RedirectURL,
		Scopes:      service.Scopes,
		GrantType:   service.GrantType,
		Username:    "", // Username not stored in database for security
		HasToken:    hasToken,
		CreatedAt:   service.CreatedAt,
		UpdatedAt:   service.UpdatedAt,
	}

	if hasToken && token != nil {
		response.TokenExpiry = &token.Expiry
	}

	h.sendJSONResponse(w, response)
}

// UpdateOAuth2Service updates an existing OAuth2 service
func (h *Handlers) UpdateOAuth2Service(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID, ok := ctx.Value("userID").(string)
	if !ok {
		h.sendJSONError(w, nil, "Failed to get user ID from context", "Authentication required", http.StatusUnauthorized)
		return
	}

	vars := mux.Vars(r)
	serviceID := vars["id"]

	// Get existing service
	service, err := h.storage.GetOAuth2Service(serviceID)
	if err != nil {
		h.logger.Error("Failed to get OAuth2 service", err, logging.Field{"serviceID", serviceID})
		h.sendJSONError(w, nil, "OAuth2 service not found", "OAuth2 service not found", http.StatusNotFound)
		return
	}

	// Check ownership
	if service.UserID != userID {
		h.sendJSONError(w, nil, "OAuth2 service not found", "OAuth2 service not found", http.StatusNotFound)
		return
	}

	var req OAuth2ServiceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.sendJSONError(w, nil, "Invalid request body", "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate grant type if provided
	if req.GrantType != "" {
		validGrantTypes := map[string]bool{
			"client_credentials": true,
			"password":           true,
			"authorization_code": true,
			"refresh_token":      true,
		}
		if !validGrantTypes[req.GrantType] {
			h.sendJSONError(w, nil, "Invalid grant type. Must be one of: client_credentials, password, authorization_code, refresh_token", "Invalid grant type. Must be one of: client_credentials, password, authorization_code, refresh_token", http.StatusBadRequest)
			return
		}

		// Validate grant type specific fields
		if req.GrantType == "password" {
			if req.Username == "" {
				h.sendJSONError(w, nil, "Username is required for password grant type", "Username is required for password grant type", http.StatusBadRequest)
				return
			}
			if req.Password == "" {
				h.sendJSONError(w, nil, "Password is required for password grant type", "Password is required for password grant type", http.StatusBadRequest)
				return
			}
		}

		if req.GrantType == "authorization_code" {
			authURL := req.AuthURL
			if authURL == "" {
				authURL = service.AuthURL
			}
			if authURL == "" {
				h.sendJSONError(w, nil, "Auth URL is required for authorization_code grant type", "Auth URL is required for authorization_code grant type", http.StatusBadRequest)
				return
			}

			redirectURL := req.RedirectURL
			if redirectURL == "" {
				redirectURL = service.RedirectURL
			}
			if redirectURL == "" {
				h.sendJSONError(w, nil, "Redirect URL is required for authorization_code grant type", "Redirect URL is required for authorization_code grant type", http.StatusBadRequest)
				return
			}
		}
	}

	// Validate URLs if provided
	if req.TokenURL != "" {
		if _, err := url.Parse(req.TokenURL); err != nil {
			h.sendJSONError(w, nil, "Invalid token URL format", "Invalid token URL format", http.StatusBadRequest)
			return
		}
	}
	if req.AuthURL != "" {
		if _, err := url.Parse(req.AuthURL); err != nil {
			h.sendJSONError(w, nil, "Invalid auth URL format", "Invalid auth URL format", http.StatusBadRequest)
			return
		}
	}
	if req.RedirectURL != "" {
		if _, err := url.Parse(req.RedirectURL); err != nil {
			h.sendJSONError(w, nil, "Invalid redirect URL format", "Invalid redirect URL format", http.StatusBadRequest)
			return
		}
	}

	// Update fields
	if req.Name != "" {
		service.Name = req.Name
	}
	if req.ClientID != "" {
		service.ClientID = req.ClientID
	}
	if req.ClientSecret != "" {
		service.ClientSecret = req.ClientSecret
	}
	if req.TokenURL != "" {
		service.TokenURL = req.TokenURL
	}
	if req.AuthURL != "" {
		service.AuthURL = req.AuthURL
	}
	if req.RedirectURL != "" {
		service.RedirectURL = req.RedirectURL
	}
	if req.Scopes != nil {
		service.Scopes = req.Scopes
	}
	// GrantType, Username, and Password are stored in OAuth2 manager config, not in storage

	service.UpdatedAt = time.Now()

	// Determine grant type to use (from request or keep existing from storage)
	grantType := req.GrantType
	if grantType == "" {
		// Use the existing grant type from storage
		grantType = service.GrantType
	}

	// Update grant type in storage if changed
	if grantType != "" && grantType != service.GrantType {
		service.GrantType = grantType
	}

	// Update OAuth2 config in manager
	config := &oauth2.Config{
		ClientID:     service.ClientID,
		ClientSecret: service.ClientSecret,
		TokenURL:     service.TokenURL,
		AuthURL:      service.AuthURL,
		RedirectURL:  service.RedirectURL,
		Scopes:       service.Scopes,
		GrantType:    grantType,
		Username:     req.Username,
		Password:     req.Password,
	}

	if err := h.oauth2Manager.RegisterService(serviceID, config); err != nil {
		h.logger.Error("Failed to update OAuth2 service in manager", err, logging.Field{"serviceID", serviceID})
		h.sendJSONError(w, nil, "Failed to update OAuth2 service", "Failed to update OAuth2 service", http.StatusInternalServerError)
		return
	}

	// If new refresh token is provided, update it in the OAuth2 manager
	if req.RefreshToken != "" {
		if err := h.oauth2Manager.SetRefreshToken(serviceID, req.RefreshToken); err != nil {
			h.logger.Error("Failed to update refresh token", err, logging.Field{"serviceID", serviceID})
			// Continue even if updating refresh token fails
		}
	}

	// Save to storage
	if err := h.storage.UpdateOAuth2Service(service); err != nil {
		h.logger.Error("Failed to update OAuth2 service", err, logging.Field{"serviceID", serviceID})
		h.sendJSONError(w, nil, "Failed to update OAuth2 service", "Failed to update OAuth2 service", http.StatusInternalServerError)
		return
	}

	// Audit log
	h.logger.Info("OAuth2 service updated",
		logging.Field{"oauth2_service_id", service.ID},
		logging.Field{"oauth2_service_name", service.Name},
		logging.Field{"user_id", userID},
		logging.Field{"action", "oauth2_service_update"})

	// Return updated service
	token, _ := h.oauth2Manager.GetToken(ctx, service.ID)
	hasToken := token != nil && !token.IsExpired()

	response := OAuth2ServiceResponse{
		ID:          service.ID,
		Name:        service.Name,
		ClientID:    service.ClientID,
		TokenURL:    service.TokenURL,
		AuthURL:     service.AuthURL,
		RedirectURL: service.RedirectURL,
		Scopes:      service.Scopes,
		GrantType:   req.GrantType, // Use from request since not stored
		Username:    req.Username,  // Use from request since not stored
		HasToken:    hasToken,
		CreatedAt:   service.CreatedAt,
		UpdatedAt:   service.UpdatedAt,
	}

	if hasToken && token != nil {
		response.TokenExpiry = &token.Expiry
	}

	h.sendJSONResponse(w, response)
}

// DeleteOAuth2Service deletes an OAuth2 service
func (h *Handlers) DeleteOAuth2Service(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value("userID").(string)
	if !ok {
		h.sendJSONError(w, nil, "Failed to get user ID from context", "Authentication required", http.StatusUnauthorized)
		return
	}

	vars := mux.Vars(r)
	serviceID := vars["id"]

	// Get service to check ownership
	service, err := h.storage.GetOAuth2Service(serviceID)
	if err != nil {
		h.logger.Error("Failed to get OAuth2 service", err, logging.Field{"serviceID", serviceID})
		h.sendJSONError(w, nil, "OAuth2 service not found", "OAuth2 service not found", http.StatusNotFound)
		return
	}

	// Check ownership
	if service.UserID != userID {
		h.sendJSONError(w, nil, "OAuth2 service not found", "OAuth2 service not found", http.StatusNotFound)
		return
	}

	// Unregister service from OAuth2 manager (removes config and tokens)
	if err := h.oauth2Manager.UnregisterService(serviceID); err != nil {
		h.logger.Error("Failed to unregister OAuth2 service", err, logging.Field{"serviceID", serviceID})
		// Continue with deletion even if unregistration fails
	}

	// Delete from storage
	if err := h.storage.DeleteOAuth2Service(serviceID); err != nil {
		h.logger.Error("Failed to delete OAuth2 service", err, logging.Field{"serviceID", serviceID})
		h.sendJSONError(w, nil, "Failed to delete OAuth2 service", "Failed to delete OAuth2 service", http.StatusInternalServerError)
		return
	}

	// Audit log
	h.logger.Info("OAuth2 service deleted",
		logging.Field{"oauth2_service_id", serviceID},
		logging.Field{"oauth2_service_name", service.Name},
		logging.Field{"user_id", userID},
		logging.Field{"action", "oauth2_service_delete"})

	w.WriteHeader(http.StatusNoContent)
}

// RefreshOAuth2Token manually refreshes the token for an OAuth2 service
func (h *Handlers) RefreshOAuth2Token(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID, ok := ctx.Value("userID").(string)
	if !ok {
		h.sendJSONError(w, nil, "Failed to get user ID from context", "Authentication required", http.StatusUnauthorized)
		return
	}

	vars := mux.Vars(r)
	serviceID := vars["id"]

	// Get service to check ownership
	service, err := h.storage.GetOAuth2Service(serviceID)
	if err != nil {
		h.logger.Error("Failed to get OAuth2 service", err, logging.Field{"serviceID", serviceID})
		h.sendJSONError(w, nil, "OAuth2 service not found", "OAuth2 service not found", http.StatusNotFound)
		return
	}

	// Check ownership
	if service.UserID != userID {
		h.sendJSONError(w, nil, "OAuth2 service not found", "OAuth2 service not found", http.StatusNotFound)
		return
	}

	// Force token refresh by revoking current token
	if err := h.oauth2Manager.RevokeToken(serviceID); err != nil {
		h.logger.Error("Failed to revoke OAuth2 token for refresh", err, logging.Field{"serviceID", serviceID})
	}

	// Get a new token (this will trigger a fresh token request)
	token, err := h.oauth2Manager.GetToken(ctx, serviceID)
	if err != nil {
		h.logger.Error("Failed to get new OAuth2 token", err, logging.Field{"serviceID", serviceID})
		h.sendJSONError(w, nil, "Failed to refresh token", "Failed to refresh token", http.StatusInternalServerError)
		return
	}

	// Audit log
	h.logger.Info("OAuth2 token refreshed",
		logging.Field{"oauth2_service_id", serviceID},
		logging.Field{"user_id", userID},
		logging.Field{"action", "oauth2_token_refresh"})

	response := map[string]interface{}{
		"success":      true,
		"token_expiry": token.Expiry,
	}

	h.sendJSONResponse(w, response)
}
