package carddav

import (
	"context"
	"strings"
	"time"

	commonhttp "webhook-router/internal/common/http"
	"webhook-router/internal/oauth2"
)

// oauth2ManagerAdapter adapts oauth2.Manager to commonhttp.OAuth2ManagerInterface
type oauth2ManagerAdapter struct {
	manager *oauth2.Manager
}

func (a *oauth2ManagerAdapter) GetToken(ctx context.Context, serviceID string) (*commonhttp.OAuth2Token, error) {
	token, err := a.manager.GetToken(ctx, serviceID)
	if err != nil {
		return nil, err
	}

	return &commonhttp.OAuth2Token{
		AccessToken: token.AccessToken,
		TokenType:   token.TokenType,
		Expiry:      token.Expiry,
	}, nil
}

// NewOAuth2Client creates a CardDAV client with OAuth2 authentication
func NewOAuth2Client(serverURL, oauth2ServiceID string, oauth2Manager *oauth2.Manager) *SimpleClient {
	// Create HTTP client wrapper with OAuth2 support
	clientWrapper := commonhttp.NewHTTPClientWrapper(
		commonhttp.WithTimeout(30 * time.Second),
	)

	// Configure OAuth2 with adapter
	if oauth2Manager != nil && oauth2ServiceID != "" {
		adapter := &oauth2ManagerAdapter{manager: oauth2Manager}
		clientWrapper.SetOAuth2Manager(adapter, oauth2ServiceID)
	}

	// Create a simple client with the HTTP wrapper's client
	return &SimpleClient{
		httpClient: clientWrapper.GetHTTPClient(),
		url:        strings.TrimSuffix(serverURL, "/"),
		username:   "", // Not needed for OAuth2
		password:   "", // Not needed for OAuth2
	}
}
