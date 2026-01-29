// Package deployment provides integration tests against a deployed chat service.
package deployment

import (
	"fmt"
	"net/http"
	"os"
	"time"
)

// Config holds the configuration for deployment tests.
type Config struct {
	// ServiceEndpoint is the base URL of the deployed service (e.g., "https://chat.example.com")
	ServiceEndpoint string

	// GatewayEndpoint is the base URL of the gateway service for real-time connections
	// If empty, defaults to ServiceEndpoint
	GatewayEndpoint string

	// ProfileEndpoint is the base URL of the profile service (e.g., "https://profile.antinvestor.com")
	ProfileEndpoint string

	// AuthToken is the JWT bearer token for authentication
	AuthToken string

	// Timeout for individual API calls
	RequestTimeout time.Duration

	// TestTimeout is the overall timeout for the test suite
	TestTimeout time.Duration

	// Verbose enables detailed logging
	Verbose bool

	// CleanupAfterTests removes test data after completion
	CleanupAfterTests bool

	// TestDataPrefix is prepended to test resource names for identification
	TestDataPrefix string
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		ProfileEndpoint:   "https://profile.antinvestor.com",
		RequestTimeout:    30 * time.Second,
		TestTimeout:       10 * time.Minute,
		Verbose:           false,
		CleanupAfterTests: true,
		TestDataPrefix:    "integration-test-",
	}
}

// ConfigFromEnv loads configuration from environment variables.
func ConfigFromEnv() (*Config, error) {
	cfg := DefaultConfig()

	endpoint := os.Getenv("CHAT_SERVICE_ENDPOINT")
	if endpoint == "" {
		return nil, fmt.Errorf("CHAT_SERVICE_ENDPOINT environment variable is required")
	}
	cfg.ServiceEndpoint = endpoint

	token := os.Getenv("CHAT_AUTH_TOKEN")
	if token == "" {
		return nil, fmt.Errorf("CHAT_AUTH_TOKEN environment variable is required")
	}
	cfg.AuthToken = token

	if timeout := os.Getenv("CHAT_REQUEST_TIMEOUT"); timeout != "" {
		if d, err := time.ParseDuration(timeout); err == nil {
			cfg.RequestTimeout = d
		}
	}

	if timeout := os.Getenv("CHAT_TEST_TIMEOUT"); timeout != "" {
		if d, err := time.ParseDuration(timeout); err == nil {
			cfg.TestTimeout = d
		}
	}

	if os.Getenv("CHAT_VERBOSE") == "true" {
		cfg.Verbose = true
	}

	if os.Getenv("CHAT_NO_CLEANUP") == "true" {
		cfg.CleanupAfterTests = false
	}

	if prefix := os.Getenv("CHAT_TEST_PREFIX"); prefix != "" {
		cfg.TestDataPrefix = prefix
	}

	if profileEndpoint := os.Getenv("PROFILE_SERVICE_ENDPOINT"); profileEndpoint != "" {
		cfg.ProfileEndpoint = profileEndpoint
	}

	if gatewayEndpoint := os.Getenv("CHAT_GATEWAY_ENDPOINT"); gatewayEndpoint != "" {
		cfg.GatewayEndpoint = gatewayEndpoint
	}

	return cfg, nil
}

// GetGatewayEndpoint returns the gateway endpoint, defaulting to service endpoint if not set.
func (c *Config) GetGatewayEndpoint() string {
	if c.GatewayEndpoint != "" {
		return c.GatewayEndpoint
	}
	return c.ServiceEndpoint
}

// Validate checks that the configuration is valid.
func (c *Config) Validate() error {
	if c.ServiceEndpoint == "" {
		return fmt.Errorf("service endpoint is required")
	}
	if c.AuthToken == "" {
		return fmt.Errorf("auth token is required")
	}
	return nil
}

// AuthTransport wraps an http.RoundTripper to add authentication headers.
type AuthTransport struct {
	Token string
	Base  http.RoundTripper
}

// RoundTrip implements http.RoundTripper.
func (t *AuthTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.Header.Set("Authorization", "Bearer "+t.Token)
	base := t.Base
	if base == nil {
		base = http.DefaultTransport
	}
	return base.RoundTrip(req)
}
