package main

import (
	"fmt"
	"io"
	"net/http"

	"github.com/sahithakellacodes/distributed-rate-limiter/internal/config"
	requestcontext "github.com/sahithakellacodes/distributed-rate-limiter/internal/context"
	"github.com/sahithakellacodes/distributed-rate-limiter/internal/middleware"
	"github.com/sahithakellacodes/distributed-rate-limiter/internal/middleware/auth"
	"github.com/sahithakellacodes/distributed-rate-limiter/internal/middleware/logger"
	"github.com/sahithakellacodes/distributed-rate-limiter/internal/middleware/router"
)

func main() {
	// Load configuration
	cfg, err := config.Load("internal/config/config.yaml")
	if err != nil {
		panic(err)
	}

	// Create auth store
	authStore := auth.NewInMemoryAuthStore()

	// Create middleware strategies
	authStrategy := auth.NewAPIKeyAuthStrategy(authStore)
	loggerStrategy := logger.NewDefaultLoggerStrategy()
	httpClient := router.NewDefaultHttpClient(loggerStrategy)

	// Create middlewares
	authMiddleware := auth.NewAuthMiddleware(authStrategy)
	loggerMiddleware := logger.NewLoggerMiddleware(loggerStrategy)
	routerMiddleware := router.NewRouterMiddleware(httpClient, cfg.BackendBaseURL, cfg.AllowedPaths)
	
	// Create middleware chain
	chain := middleware.NewDefaultMiddlewareChain([]middleware.Middleware{
		loggerMiddleware,
		authMiddleware,
		routerMiddleware,
	})

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "failed to read request body", http.StatusBadRequest)
			return
		}

		// Build request context
		// Keep only the first value for each header
		// RequestContext intentionally supports single-value headers only
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
		chain.Execute(request, response)

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
