package main

import (
	"fmt"
	"io"
	"net/http"

	requestcontext "github.com/sahithakellacodes/distributed-rate-limiter/internal/context"
	"github.com/sahithakellacodes/distributed-rate-limiter/internal/middleware"
	"github.com/sahithakellacodes/distributed-rate-limiter/internal/middleware/auth"
)

func main() {
	authStore := auth.NewInMemoryAuthStore()
	authStrategy := auth.NewAPIKeyAuthStrategy(authStore)
	authMiddleware := auth.NewAuthMiddleware(authStrategy)
	chain := middleware.NewDefaultMiddlewareChain([]middleware.Middleware{authMiddleware})

	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "failed to read request body", http.StatusBadRequest)
			return
		}

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
		response := &requestcontext.ResponseContext{Headers: make(map[string]string)}
		chain.Next(request, response)

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

	err := http.ListenAndServe(":8080", nil)
	if err != nil {
		panic(err)
	}
}
