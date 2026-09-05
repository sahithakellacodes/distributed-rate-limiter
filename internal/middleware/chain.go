package middleware

import (
	stdcontext "context"

	"github.com/sahithakellacodes/distributed-rate-limiter/internal/context"
)

type MiddlewareChain interface {
	Next(
		ctx stdcontext.Context,
		request *context.RequestContext,
		response *context.ResponseContext,
	)
}