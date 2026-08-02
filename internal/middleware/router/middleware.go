package router

import (
	"strings"

	"github.com/sahithakellacodes/distributed-rate-limiter/internal/context"
	"github.com/sahithakellacodes/distributed-rate-limiter/internal/middleware"
)

// RouterMiddleware checks whether the requested path is allowed
// and forwards allowed requests to the configured backend service.
type RouterMiddleware struct {
	httpClient      HttpClient
	backendBaseURL  string
	allowedPaths    []string
}

// NewRouterMiddleware creates a new instance of RouterMiddleware.
func NewRouterMiddleware(
	httpClient HttpClient,
	backendBaseURL string,
	allowedPaths []string,
) *RouterMiddleware {
	return &RouterMiddleware{
		httpClient:     httpClient,
		backendBaseURL: backendBaseURL,
		allowedPaths:   allowedPaths,
	}
}

// Handle processes the incoming request, checks if the path is allowed,
// forwards it to the backend if allowed, and populates the response context.
func (r *RouterMiddleware) Handle(
	request *context.RequestContext,
	response *context.ResponseContext,
	chain middleware.MiddlewareChain,
) {
	if !r.isPathAllowed(request.Path) {
		response.StatusCode = 404
		response.Body = []byte("route not found")
		return
	}

	backendResponse, err := r.httpClient.Forward(
		request,
		r.backendBaseURL,
	)

	if err != nil {
		response.StatusCode = 502
		response.Body = []byte("backend request failed")
		return
	}

	response.StatusCode = backendResponse.StatusCode
	response.Headers = backendResponse.Headers
	response.Body = backendResponse.Body

	// Router is terminal middleware.
	// It does not call chain.Next().
}

// isPathAllowed checks if the requested path is in the list of allowed paths.
func (r *RouterMiddleware) isPathAllowed(path string) bool {
	for _, allowedPath := range r.allowedPaths {
		if strings.HasPrefix(path, allowedPath) {
			return true
		}
	}

	return false
}