package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/sahithakellacodes/distributed-rate-limiter/internal/config"
	requestcontext "github.com/sahithakellacodes/distributed-rate-limiter/internal/context"
	"github.com/sahithakellacodes/distributed-rate-limiter/internal/health"
	"github.com/sahithakellacodes/distributed-rate-limiter/internal/middleware"
	"github.com/sahithakellacodes/distributed-rate-limiter/internal/middleware/auth"
	"github.com/sahithakellacodes/distributed-rate-limiter/internal/middleware/logger"
	"github.com/sahithakellacodes/distributed-rate-limiter/internal/middleware/ratelimit"
	"github.com/sahithakellacodes/distributed-rate-limiter/internal/middleware/router"
	"github.com/sahithakellacodes/distributed-rate-limiter/internal/redis"
)

func main() {
	// Load configuration
	cfg, err := config.Load("internal/config/config.yaml")
	if err != nil {
		panic(err)
	}

	backendBaseURL := os.Getenv("BACKEND_BASE_URL")
	redisAddr := os.Getenv("REDIS_ADDR")

	if backendBaseURL == "" {
		panic("BACKEND_BASE_URL is not set")
	}

	if redisAddr == "" {
		panic("redisAddr is not set")
	}

	redisClient := redis.NewClient(redisAddr)

	ctx := context.Background()

	if err := redisClient.Ping(ctx); err != nil {
		panic(err)
	}

	fmt.Println("Connected to Redis")

	healthChecker := health.NewHealthChecker()

	// Create auth store
	authStore := auth.NewInMemoryAuthStore()

	// Create RateLimitConfig
	rateLimitConfig := ratelimit.RateLimitConfig{
		MaxRequestsPerWindow: 40,
		WindowSize:           60 * time.Second,
	}

	// Create middleware strategies
	authStrategy := auth.NewAPIKeyAuthStrategy(authStore)
	loggerStrategy := logger.NewDefaultLoggerStrategy()
	localTokenBucketStrategy := ratelimit.NewLocalTokenBucketStrategy()
	redisTokenBucketStrategy := ratelimit.NewRedisTokenBucketStrategy(redisClient)
	rateLimitStrategy := ratelimit.NewResilientWrapper(redisTokenBucketStrategy, localTokenBucketStrategy, healthChecker)
	httpClient := router.NewDefaultHttpClient()

	// Create middlewares
	authMiddleware := auth.NewAuthMiddleware(authStrategy)
	loggerMiddleware := logger.NewLoggerMiddleware(loggerStrategy)
	rateLimitMiddleware := ratelimit.NewRateLimiterMiddleware(rateLimitStrategy, rateLimitConfig)
	routerMiddleware := router.NewRouterMiddleware(httpClient, backendBaseURL, cfg.AllowedPaths)

	// Create middleware chain
	chain := middleware.NewDefaultMiddlewareChain([]middleware.Middleware{
		loggerMiddleware,
		authMiddleware,
		rateLimitMiddleware,
		routerMiddleware,
	})

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "failed to read request body", http.StatusBadRequest)
			return
		}

		// Build request context
		// RequestContext stores only the first value of each HTTP header.
		// This is a deliberate simplification for this project and does not
		// preserve the full HTTP header representation.
		headers := make(map[string]string, len(r.Header))
		for name, values := range r.Header {
			if len(values) > 0 {
				headers[http.CanonicalHeaderKey(name)] = values[0]
			}
		}

		request := &requestcontext.RequestContext{
			Path:    r.URL.Path,
			Method:  r.Method,
			Headers: headers,
			Body:    body,
		}

		// Build response context and execute the middleware chain
		response := &requestcontext.ResponseContext{Headers: make(map[string]string)}
		chain.Execute(r.Context(), request, response)

		// Copy response headers and status code to the actual HTTP response
		for name, value := range response.Headers {
			w.Header().Set(name, value)
		}
		if response.StatusCode == 0 {
			response.StatusCode = http.StatusOK
		}
		w.WriteHeader(response.StatusCode)
		_, _ = w.Write(response.Body)
	})

	fmt.Println("Gateway listening on :8080")

	err = http.ListenAndServe(":8080", nil)
	if err != nil {
		panic(err)
	}
}
