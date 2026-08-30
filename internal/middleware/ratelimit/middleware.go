package ratelimit

import (
	"fmt"
	"strconv"

	"github.com/sahithakellacodes/distributed-rate-limiter/internal/context"
	"github.com/sahithakellacodes/distributed-rate-limiter/internal/middleware"
)

type RateLimiterMiddleware struct {
	strategy RateLimitStrategy
	config   RateLimitConfig
}

func NewRateLimiterMiddleware(strategy RateLimitStrategy, config RateLimitConfig) *RateLimiterMiddleware {
	return &RateLimiterMiddleware{
		strategy: strategy,
		config:   config,
	}
}

func (m *RateLimiterMiddleware) Handle(
	request *context.RequestContext,
	response *context.ResponseContext,
	chain middleware.MiddlewareChain,
) {
	fmt.Println("Rate limiter activated")
	rateLimitResult := m.strategy.Check(request.ClientID, m.config)

	if !rateLimitResult.Allowed {
		response.StatusCode = 429
		response.Headers["X-RateLimit-Remaining"] = strconv.Itoa(rateLimitResult.Remaining)
		response.Headers["Retry-After"] = strconv.Itoa(rateLimitResult.RetryAfterSeconds)
		response.Body = []byte("Rate limit exceeded!")
		return
	}

	response.Headers["X-RateLimit-Remaining"] = strconv.Itoa(rateLimitResult.Remaining)

	chain.Next(request, response)
}
