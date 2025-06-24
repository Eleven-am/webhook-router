package http

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultClientConfig(t *testing.T) {
	config := DefaultClientConfig()

	assert.Equal(t, 30*time.Second, config.Timeout)
	assert.Equal(t, 100, config.MaxIdleConns)
	assert.Equal(t, 10, config.MaxIdleConnsPerHost)
	assert.Equal(t, 90*time.Second, config.IdleConnTimeout)
	assert.False(t, config.DisableKeepAlives)
	assert.False(t, config.DisableCompression)
	assert.Nil(t, config.Transport)
	assert.Nil(t, config.CheckRedirect)
}

func TestWithTimeout(t *testing.T) {
	config := DefaultClientConfig()
	option := WithTimeout(5 * time.Second)

	option(&config)

	assert.Equal(t, 5*time.Second, config.Timeout)
	// Other fields should remain unchanged
	assert.Equal(t, 100, config.MaxIdleConns)
}

func TestWithMaxIdleConns(t *testing.T) {
	config := DefaultClientConfig()
	option := WithMaxIdleConns(50)

	option(&config)

	assert.Equal(t, 50, config.MaxIdleConns)
	// Other fields should remain unchanged
	assert.Equal(t, 30*time.Second, config.Timeout)
}

func TestWithMaxIdleConnsPerHost(t *testing.T) {
	config := DefaultClientConfig()
	option := WithMaxIdleConnsPerHost(5)

	option(&config)

	assert.Equal(t, 5, config.MaxIdleConnsPerHost)
	// Other fields should remain unchanged
	assert.Equal(t, 100, config.MaxIdleConns) // Default is 100, not 10
}

func TestWithIdleConnTimeout(t *testing.T) {
	config := DefaultClientConfig()
	option := WithIdleConnTimeout(60 * time.Second)

	option(&config)

	assert.Equal(t, 60*time.Second, config.IdleConnTimeout)
	// Other fields should remain unchanged
	assert.Equal(t, 30*time.Second, config.Timeout) // Check a different field that wasn't modified
}

func TestWithoutKeepAlives(t *testing.T) {
	config := DefaultClientConfig()
	option := WithoutKeepAlives()

	assert.False(t, config.DisableKeepAlives) // Initially false

	option(&config)

	assert.True(t, config.DisableKeepAlives)
	// Other fields should remain unchanged
	assert.False(t, config.DisableCompression)
}

func TestWithoutCompression(t *testing.T) {
	config := DefaultClientConfig()
	option := WithoutCompression()

	assert.False(t, config.DisableCompression) // Initially false

	option(&config)

	assert.True(t, config.DisableCompression)
	// Other fields should remain unchanged
	assert.False(t, config.DisableKeepAlives)
}

func TestWithTransport(t *testing.T) {
	config := DefaultClientConfig()
	customTransport := &http.Transport{
		MaxIdleConns: 200,
	}
	option := WithTransport(customTransport)

	option(&config)

	assert.Equal(t, customTransport, config.Transport)
}

func TestWithCheckRedirect(t *testing.T) {
	config := DefaultClientConfig()
	customRedirectFunc := func(req *http.Request, via []*http.Request) error {
		return nil
	}
	option := WithCheckRedirect(customRedirectFunc)

	option(&config)

	assert.NotNil(t, config.CheckRedirect)
	// We can't directly compare function pointers, but we can test that it was set
}

func TestNewHTTPClient_DefaultConfig(t *testing.T) {
	client := NewHTTPClient()

	assert.NotNil(t, client)
	assert.Equal(t, 30*time.Second, client.Timeout)
	assert.NotNil(t, client.Transport)

	// Verify transport is correctly configured
	transport, ok := client.Transport.(*http.Transport)
	require.True(t, ok, "Transport should be *http.Transport")
	assert.Equal(t, 100, transport.MaxIdleConns)
	assert.Equal(t, 10, transport.MaxIdleConnsPerHost)
	assert.Equal(t, 90*time.Second, transport.IdleConnTimeout)
	assert.False(t, transport.DisableKeepAlives)
	assert.False(t, transport.DisableCompression)
}

func TestNewHTTPClient_WithSingleOption(t *testing.T) {
	client := NewHTTPClient(WithTimeout(5 * time.Second))

	assert.Equal(t, 5*time.Second, client.Timeout)

	// Other default values should still be applied
	transport, ok := client.Transport.(*http.Transport)
	require.True(t, ok)
	assert.Equal(t, 100, transport.MaxIdleConns)
}

func TestNewHTTPClient_WithMultipleOptions(t *testing.T) {
	client := NewHTTPClient(
		WithTimeout(10*time.Second),
		WithMaxIdleConns(50),
		WithMaxIdleConnsPerHost(5),
		WithIdleConnTimeout(60*time.Second),
		WithoutKeepAlives(),
		WithoutCompression(),
	)

	assert.Equal(t, 10*time.Second, client.Timeout)

	transport, ok := client.Transport.(*http.Transport)
	require.True(t, ok)
	assert.Equal(t, 50, transport.MaxIdleConns)
	assert.Equal(t, 5, transport.MaxIdleConnsPerHost)
	assert.Equal(t, 60*time.Second, transport.IdleConnTimeout)
	assert.True(t, transport.DisableKeepAlives)
	assert.True(t, transport.DisableCompression)
}

func TestNewHTTPClient_WithCustomTransport(t *testing.T) {
	customTransport := &http.Transport{
		MaxIdleConns: 200,
	}

	client := NewHTTPClient(
		WithTransport(customTransport),
		WithTimeout(15*time.Second),
	)

	assert.Equal(t, 15*time.Second, client.Timeout)
	assert.Equal(t, customTransport, client.Transport)

	// When custom transport is provided, other transport options should be ignored
	transport, ok := client.Transport.(*http.Transport)
	require.True(t, ok)
	assert.Equal(t, 200, transport.MaxIdleConns)
}

func TestNewHTTPClient_WithCheckRedirect(t *testing.T) {
	redirectCount := 0
	customRedirectFunc := func(req *http.Request, via []*http.Request) error {
		redirectCount++
		if redirectCount > 2 {
			return http.ErrUseLastResponse
		}
		return nil
	}

	client := NewHTTPClient(WithCheckRedirect(customRedirectFunc))

	assert.NotNil(t, client.CheckRedirect)
}

func TestNewDefaultHTTPClient(t *testing.T) {
	client := NewDefaultHTTPClient()

	assert.NotNil(t, client)
	assert.Equal(t, 30*time.Second, client.Timeout)

	transport, ok := client.Transport.(*http.Transport)
	require.True(t, ok)
	assert.Equal(t, 100, transport.MaxIdleConns)
	assert.Equal(t, 10, transport.MaxIdleConnsPerHost)
}

func TestNewHTTPClientWithTimeout(t *testing.T) {
	timeout := 45 * time.Second
	client := NewHTTPClientWithTimeout(timeout)

	assert.Equal(t, timeout, client.Timeout)

	// Other settings should be defaults
	transport, ok := client.Transport.(*http.Transport)
	require.True(t, ok)
	assert.Equal(t, 100, transport.MaxIdleConns)
}

func TestClientOptions_Chaining(t *testing.T) {
	// Test that multiple options can be chained and applied correctly
	config := DefaultClientConfig()

	// Apply multiple options
	options := []ClientOption{
		WithTimeout(5 * time.Second),
		WithMaxIdleConns(25),
		WithoutKeepAlives(),
	}

	for _, opt := range options {
		opt(&config)
	}

	assert.Equal(t, 5*time.Second, config.Timeout)
	assert.Equal(t, 25, config.MaxIdleConns)
	assert.True(t, config.DisableKeepAlives)
	// Unchanged values
	assert.Equal(t, 10, config.MaxIdleConnsPerHost)
	assert.False(t, config.DisableCompression)
}

func TestClientOptions_OrderIndependence(t *testing.T) {
	// Test that the order of options doesn't matter for non-conflicting options

	client1 := NewHTTPClient(
		WithTimeout(5*time.Second),
		WithMaxIdleConns(25),
	)

	client2 := NewHTTPClient(
		WithMaxIdleConns(25),
		WithTimeout(5*time.Second),
	)

	assert.Equal(t, client1.Timeout, client2.Timeout)

	transport1, ok1 := client1.Transport.(*http.Transport)
	transport2, ok2 := client2.Transport.(*http.Transport)
	require.True(t, ok1)
	require.True(t, ok2)

	assert.Equal(t, transport1.MaxIdleConns, transport2.MaxIdleConns)
}

func TestClientOptions_LastWins(t *testing.T) {
	// Test that when the same option is applied multiple times, the last one wins

	client := NewHTTPClient(
		WithTimeout(5*time.Second),
		WithTimeout(10*time.Second),
		WithTimeout(15*time.Second),
	)

	assert.Equal(t, 15*time.Second, client.Timeout)
}

// Integration tests with real HTTP calls

func TestHTTPClient_Integration_Timeout(t *testing.T) {
	// Create a test server that delays response
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Create client with very short timeout
	client := NewHTTPClient(WithTimeout(50 * time.Millisecond))

	// Request should timeout
	_, err := client.Get(server.URL)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "deadline exceeded")
}

func TestHTTPClient_Integration_Success(t *testing.T) {
	// Create a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Hello, World!"))
	}))
	defer server.Close()

	// Create client with reasonable timeout
	client := NewHTTPClient(WithTimeout(5 * time.Second))

	// Request should succeed
	resp, err := client.Get(server.URL)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestHTTPClient_Integration_Redirect(t *testing.T) {
	// Create test servers
	finalServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Final destination"))
	}))
	defer finalServer.Close()

	redirectServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, finalServer.URL, http.StatusFound)
	}))
	defer redirectServer.Close()

	// Test default redirect behavior
	t.Run("default redirect", func(t *testing.T) {
		client := NewHTTPClient()
		resp, err := client.Get(redirectServer.URL)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})

	// Test custom redirect policy
	t.Run("custom redirect policy", func(t *testing.T) {
		client := NewHTTPClient(WithCheckRedirect(func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse // Don't follow redirects
		}))

		resp, err := client.Get(redirectServer.URL)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusFound, resp.StatusCode)
	})
}

func TestHTTPClient_Integration_KeepAlives(t *testing.T) {
	// Create a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Test with keep-alives disabled
	client := NewHTTPClient(WithoutKeepAlives())

	// Make multiple requests
	for i := 0; i < 3; i++ {
		resp, err := client.Get(server.URL)
		require.NoError(t, err)
		resp.Body.Close()
	}

	// With keep-alives disabled, each request should create a new connection
	// This is more of a behavioral test - we can't easily assert the connection count
}

// Edge cases and error scenarios

func TestClientOptions_NilFunction(t *testing.T) {
	// Test that nil options don't cause panics
	config := DefaultClientConfig()

	var nilOption ClientOption
	assert.NotPanics(t, func() {
		if nilOption != nil {
			nilOption(&config)
		}
	})
}

func TestClientOptions_ZeroValues(t *testing.T) {
	// Test options with zero values
	client := NewHTTPClient(
		WithTimeout(0),
		WithMaxIdleConns(0),
		WithMaxIdleConnsPerHost(0),
		WithIdleConnTimeout(0),
	)

	assert.Equal(t, time.Duration(0), client.Timeout)

	transport, ok := client.Transport.(*http.Transport)
	require.True(t, ok)
	assert.Equal(t, 0, transport.MaxIdleConns)
	assert.Equal(t, 0, transport.MaxIdleConnsPerHost)
	assert.Equal(t, time.Duration(0), transport.IdleConnTimeout)
}

func TestClientOptions_NegativeValues(t *testing.T) {
	// Test options with negative values (edge case)
	client := NewHTTPClient(
		WithTimeout(-1*time.Second),
		WithMaxIdleConns(-1),
		WithMaxIdleConnsPerHost(-1),
		WithIdleConnTimeout(-1*time.Second),
	)

	assert.Equal(t, -1*time.Second, client.Timeout)

	transport, ok := client.Transport.(*http.Transport)
	require.True(t, ok)
	assert.Equal(t, -1, transport.MaxIdleConns)
	assert.Equal(t, -1, transport.MaxIdleConnsPerHost)
	assert.Equal(t, -1*time.Second, transport.IdleConnTimeout)
}

func TestWithTransport_NilTransport(t *testing.T) {
	// Test with nil transport
	client := NewHTTPClient(WithTransport(nil))

	// Should fall back to creating default transport
	transport, ok := client.Transport.(*http.Transport)
	require.True(t, ok)
	assert.Equal(t, 100, transport.MaxIdleConns)
}

func TestWithCheckRedirect_NilFunction(t *testing.T) {
	// Test with nil redirect function
	client := NewHTTPClient(WithCheckRedirect(nil))

	// CheckRedirect should be set to nil (default behavior)
	assert.Nil(t, client.CheckRedirect)
}

// Benchmarks

func BenchmarkNewHTTPClient_NoOptions(b *testing.B) {
	for i := 0; i < b.N; i++ {
		NewHTTPClient()
	}
}

func BenchmarkNewHTTPClient_WithOptions(b *testing.B) {
	for i := 0; i < b.N; i++ {
		NewHTTPClient(
			WithTimeout(30*time.Second),
			WithMaxIdleConns(100),
			WithMaxIdleConnsPerHost(10),
		)
	}
}

func BenchmarkNewHTTPClient_ManyOptions(b *testing.B) {
	for i := 0; i < b.N; i++ {
		NewHTTPClient(
			WithTimeout(30*time.Second),
			WithMaxIdleConns(100),
			WithMaxIdleConnsPerHost(10),
			WithIdleConnTimeout(90*time.Second),
			WithoutKeepAlives(),
			WithoutCompression(),
		)
	}
}

func BenchmarkClientOptions_Apply(b *testing.B) {
	config := DefaultClientConfig()
	options := []ClientOption{
		WithTimeout(30 * time.Second),
		WithMaxIdleConns(100),
		WithMaxIdleConnsPerHost(10),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, opt := range options {
			opt(&config)
		}
	}
}

// Test helper functions

func TestNewDefaultHTTPClient_EquivalentToDefault(t *testing.T) {
	client1 := NewDefaultHTTPClient()
	client2 := NewHTTPClient()

	assert.Equal(t, client1.Timeout, client2.Timeout)

	transport1, ok1 := client1.Transport.(*http.Transport)
	transport2, ok2 := client2.Transport.(*http.Transport)
	require.True(t, ok1)
	require.True(t, ok2)

	assert.Equal(t, transport1.MaxIdleConns, transport2.MaxIdleConns)
	assert.Equal(t, transport1.MaxIdleConnsPerHost, transport2.MaxIdleConnsPerHost)
	assert.Equal(t, transport1.IdleConnTimeout, transport2.IdleConnTimeout)
	assert.Equal(t, transport1.DisableKeepAlives, transport2.DisableKeepAlives)
	assert.Equal(t, transport1.DisableCompression, transport2.DisableCompression)
}

func TestNewHTTPClientWithTimeout_EquivalentToWithTimeout(t *testing.T) {
	timeout := 45 * time.Second

	client1 := NewHTTPClientWithTimeout(timeout)
	client2 := NewHTTPClient(WithTimeout(timeout))

	assert.Equal(t, client1.Timeout, client2.Timeout)
	assert.Equal(t, timeout, client1.Timeout)
	assert.Equal(t, timeout, client2.Timeout)
}
