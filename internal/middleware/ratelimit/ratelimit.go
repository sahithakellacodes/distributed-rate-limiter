package ratelimit

import (
	"context"
	"time"
)

const tokenEpsilon float64 = 1e-9 // Accounts for floating-point precision errors when comparing token counts.

type RateLimitConfig struct {
	MaxRequestsPerWindow int           // Maximum number of requests allowed in the specified window
	WindowSize  time.Duration // Duration of the time window for rate limiting
}

type RateLimitResult struct {
	Allowed           bool  // Whether the request is allowed or not
	Remaining         int   // Remaining number of requests allowed in the current window
	RetryAfterSeconds int   // Number of seconds to wait before making the next request if the limit is exceeded
}

// RateLimitStrategy defines the interface for rate limiting strategies.
// 
// ctx carries a deadline so redis-backed implementations don't hang
// local implementations ignore it
// error is non nil only when the backend itself fails (e.g. redis unreachable); local implementations always return nil
type RateLimitStrategy interface {
	// Check checks if a request is allowed based on the rate limit configuration.
	// identifier is a unique identifier for the client. In this case it is same as clientID in authentication middleware.
	Check(ctx context.Context, identifier string, config RateLimitConfig) (RateLimitResult, error)
}
