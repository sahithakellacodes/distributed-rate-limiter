package router

import (
	"github.com/sahithakellacodes/distributed-rate-limiter/internal/context"
)

// HttpClient defines how the gateway forwards a request to the configured backend service.
type HttpClient interface {
	Forward(
		request *context.RequestContext,
		backendBaseURL string,
	) (*HttpResponse, error)
}

// HttpResponse represents the response received from the backend.
type HttpResponse struct {
	StatusCode int
	Headers    map[string]string
	Body       []byte
}