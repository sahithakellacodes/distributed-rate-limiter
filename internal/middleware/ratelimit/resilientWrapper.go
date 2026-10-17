package ratelimit

import (
	"context"

	"github.com/sahithakellacodes/distributed-rate-limiter/internal/health"
	"github.com/sahithakellacodes/distributed-rate-limiter/internal/middleware/logger"
)

type ResilientWrapper struct {
	primary  RateLimitStrategy // redis backed
	fallback RateLimitStrategy // local
	health   health.Checker
	logger   logger.LoggerStrategy
}

func NewResilientWrapper(
	primary RateLimitStrategy,
	fallback RateLimitStrategy,
	healthChecker health.Checker,
	logger logger.LoggerStrategy,
) *ResilientWrapper {
	return &ResilientWrapper{
		primary:  primary,
		fallback: fallback,
		health:   healthChecker,
		logger:   logger,
	}
}

func (w *ResilientWrapper) Check(
	ctx context.Context,
	identifier string,
	config RateLimitConfig,
) (RateLimitResult, error) {
	if w.health.IsHealthy() {
		res, err := w.primary.Check(ctx, identifier, config)
		if err == nil {
			return res, nil
		}
		w.health.ReportFailure() // accelerate failure detection
		w.logger.Error("redis rate limiter failed, degrading to local fallback", map[string]any{
			"identifier": identifier,
			"error":      err.Error(),
		})
		return w.fallback.Check(ctx, identifier, config)
	}
	w.logger.Info("redis unhealthy, using local fallback", map[string]any{
		"identifier": identifier,
	})
	return w.fallback.Check(ctx, identifier, config)
}
