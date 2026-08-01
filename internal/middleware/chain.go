package middleware

import "github.com/sahithakellacodes/distributed-rate-limiter/internal/context"

type MiddlewareChain interface {
	Next(
		request *context.RequestContext,
		response *context.ResponseContext,
	)
}