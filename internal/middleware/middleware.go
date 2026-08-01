package middleware

import "github.com/sahithakellacodes/distributed-rate-limiter/internal/context"

type Middleware interface {
	Handle(
		request *context.RequestContext,
		response *context.ResponseContext,
		chain MiddlewareChain,
	)
}
