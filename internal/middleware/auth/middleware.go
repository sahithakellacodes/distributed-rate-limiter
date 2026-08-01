package auth

import (
	"github.com/sahithakellacodes/distributed-rate-limiter/internal/context"
	"github.com/sahithakellacodes/distributed-rate-limiter/internal/middleware"
)


// AuthMiddleware integrates authentication into the middleware chain.
//
// It does NOT know how authentication works.
// It delegates that responsibility to AuthStrategy.
//
// This means AuthMiddleware doesn't know about Redis, API-key lookup,
// or any other authentication implementation detail.
type AuthMiddleware struct {
	strategy AuthStrategy
}

func NewAuthMiddleware(strategy AuthStrategy) *AuthMiddleware {
	return &AuthMiddleware{
		strategy: strategy,
	}
}

// Handle authenticates the request using the configured strategy.
//
// If authentication fails:
//   - write an unauthorized response
//   - stop the middleware chain
//
// If authentication succeeds:
//   - enrich RequestContext with ClientID
//   - continue to the next middleware
func (a *AuthMiddleware) Handle(
	request *context.RequestContext,
	response *context.ResponseContext,
	chain middleware.MiddlewareChain,
) {
	result := a.strategy.Authenticate(request)

	if !result.Authenticated {
		response.StatusCode = 401
		response.Body = []byte(result.FailureReason)
		return
	}

	request.ClientID = result.ClientID
	chain.Next(request, response)
}
