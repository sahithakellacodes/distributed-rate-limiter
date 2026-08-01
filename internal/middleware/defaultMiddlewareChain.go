package middleware

import "github.com/sahithakellacodes/distributed-rate-limiter/internal/context"

// DefaultMiddlewareChain executes middleware in the order it was supplied.
// A fresh cursor is created for every request so the chain is safe to reuse.
type DefaultMiddlewareChain struct {
	middlewares []Middleware
}

func NewDefaultMiddlewareChain(middlewares []Middleware) *DefaultMiddlewareChain {
	return &DefaultMiddlewareChain{middlewares: middlewares}
}

func (c *DefaultMiddlewareChain) Execute(
	request *context.RequestContext,
	response *context.ResponseContext,
) {
	(&requestMiddlewareChain{middlewares: c.middlewares}).Next(request, response)
}

type requestMiddlewareChain struct {
	middlewares []Middleware
	index       int
}

func (c *requestMiddlewareChain) Next(
	request *context.RequestContext,
	response *context.ResponseContext,
) {
	if c.index >= len(c.middlewares) {
		return
	}

	current := c.middlewares[c.index]
	c.index++
	current.Handle(request, response, c)
}
