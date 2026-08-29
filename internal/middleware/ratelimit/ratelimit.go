package ratelimit

import "time"

type RateLimitConfig struct {
	MaxRequestsPerMinute int
	WindowSizeInSeconds  time.Duration
}

type RateLimitResult struct {
	Allowed           bool
	Remaining         int
	ResetTimestamp    time.Time // Epoch timestamp when the rate limit will reset
	RetryAfterSeconds int
}

// RateLimitStrategy defines the interface for rate limiting strategies.
type RateLimitStrategy interface {
	// Check checks if a request is allowed based on the rate limit configuration.
	// identifier is a unique identifier for the client. In this case it is same as clientID in authentication middleware.
	Check(identifier string, config RateLimitConfig) RateLimitResult
}
