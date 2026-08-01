package logger

import (
	"time"

	"github.com/sahithakellacodes/distributed-rate-limiter/internal/context"
	"github.com/sahithakellacodes/distributed-rate-limiter/internal/middleware"
)

type LoggerMiddleware struct {
	logger LoggerStrategy
}

func NewLoggerMiddleware(logger LoggerStrategy) *LoggerMiddleware {
	return &LoggerMiddleware{
		logger: logger,
	}
}

func (m *LoggerMiddleware) Handle(
	request *context.RequestContext,
	response *context.ResponseContext,
	chain middleware.MiddlewareChain,
) {
	start := time.Now()

	m.logger.Info("request received", map[string]any{
		"method": request.Method,
		"path":   request.Path,
	})

	chain.Next(request, response)

	m.logger.Info("request completed", map[string]any{
		"method":      request.Method,
		"path":        request.Path,
		"status_code": response.StatusCode,
		"duration_ms": time.Since(start).Milliseconds(),
	})
}
