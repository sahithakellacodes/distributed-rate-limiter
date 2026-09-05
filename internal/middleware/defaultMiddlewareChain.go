package middleware

import (
	stdcontext "context"

	"github.com/sahithakellacodes/distributed-rate-limiter/internal/context"
)

// DefaultMiddlewareChain executes middleware in the order it was supplied.
// A fresh cursor is created for every request so the chain is safe to reuse.
type DefaultMiddlewareChain struct {
	middlewares []Middleware
}

func NewDefaultMiddlewareChain(middlewares []Middleware) *DefaultMiddlewareChain {
	return &DefaultMiddlewareChain{middlewares: middlewares}
}

func (c *DefaultMiddlewareChain) Execute(
	ctx stdcontext.Context,
	request *context.RequestContext,
	response *context.ResponseContext,
) {
	(&requestMiddlewareChain{middlewares: c.middlewares}).Next(ctx, request, response)
}

type requestMiddlewareChain struct {
	middlewares []Middleware
	index       int
}

func (c *requestMiddlewareChain) Next(
	ctx stdcontext.Context,
	request *context.RequestContext,
	response *context.ResponseContext,
) {
	if c.index >= len(c.middlewares) {
		return
	}

	current := c.middlewares[c.index]
	c.index++
	current.Handle(ctx, request, response, c)
}
