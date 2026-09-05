package middleware

import (
	stdcontext "context"

	"github.com/sahithakellacodes/distributed-rate-limiter/internal/context"
)

type Middleware interface {
	Handle(
		ctx stdcontext.Context,
		request *context.RequestContext,
		response *context.ResponseContext,
		chain MiddlewareChain,
	)
}
