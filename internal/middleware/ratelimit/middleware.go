package ratelimit

import (
	"context"
	"strconv"

	rlcontext "github.com/sahithakellacodes/distributed-rate-limiter/internal/context"
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
	ctx context.Context,
	request *rlcontext.RequestContext,
	response *rlcontext.ResponseContext,
	chain middleware.MiddlewareChain,
) {
	rateLimitResult, err := m.strategy.Check(ctx, request.ClientID, m.config)
	if err != nil {
		// Fail closed: the rate limiter could not reach its backend. The resilient
		// wrapper never returns an error here (it degrades to local), so this path
		// is only hit by a pure-redis strategy when redis is unreachable, which is
		// exactly the fragility we want to surface rather than mask.
		response.StatusCode = 503
		response.Body = []byte("rate limiter unavailable")
		return
	}

	if !rateLimitResult.Allowed {
		response.StatusCode = 429
		response.Headers["X-RateLimit-Remaining"] = strconv.Itoa(rateLimitResult.Remaining)
		response.Headers["Retry-After"] = strconv.Itoa(rateLimitResult.RetryAfterSeconds)
		response.Body = []byte("Rate limit exceeded!")
		return
	}

	response.Headers["X-RateLimit-Remaining"] = strconv.Itoa(rateLimitResult.Remaining)

	chain.Next(ctx, request, response)
}
