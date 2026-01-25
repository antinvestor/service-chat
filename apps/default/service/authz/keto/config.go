package keto

import "time"

// Config holds the configuration for connecting to Ory Keto.
type Config struct {
	// ReadURL is the URL of the Keto read API.
	ReadURL string

	// WriteURL is the URL of the Keto write API.
	WriteURL string

	// Timeout is the request timeout.
	Timeout time.Duration

	// RetryAttempts is the number of retry attempts for transient failures.
	RetryAttempts int

	// RetryDelay is the delay between retry attempts.
	RetryDelay time.Duration

	// Enabled determines if Keto authorization is enabled.
	// When disabled, all checks will be allowed (permissive mode for migration).
	Enabled bool
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		ReadURL:       "http://localhost:4466",
		WriteURL:      "http://localhost:4467",
		Timeout:       5 * time.Second,
		RetryAttempts: 3,
		RetryDelay:    100 * time.Millisecond,
		Enabled:       true,
	}
}

// Validate checks if the configuration is valid.
func (c *Config) Validate() error {
	if c.ReadURL == "" {
		return ErrInvalidConfig{Field: "ReadURL", Reason: "cannot be empty"}
	}
	if c.WriteURL == "" {
		return ErrInvalidConfig{Field: "WriteURL", Reason: "cannot be empty"}
	}
	if c.Timeout <= 0 {
		c.Timeout = 5 * time.Second
	}
	if c.RetryAttempts < 0 {
		c.RetryAttempts = 0
	}
	if c.RetryDelay < 0 {
		c.RetryDelay = 100 * time.Millisecond
	}
	return nil
}

// ErrInvalidConfig represents a configuration validation error.
type ErrInvalidConfig struct {
	Field  string
	Reason string
}

func (e ErrInvalidConfig) Error() string {
	return "invalid keto config: " + e.Field + " " + e.Reason
}
