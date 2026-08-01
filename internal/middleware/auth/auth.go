package auth

import (
	"github.com/sahithakellacodes/distributed-rate-limiter/internal/context"
)

// AuthResult represents the result of attempting to authenticate a request.
//
// AuthStrategy produces this result and AuthMiddleware consumes it.
// It is NOT the HTTP response returned to the client.
type AuthResult struct {
	Authenticated bool
	ClientID       string
	APIKey         string
	FailureReason  string
}

// AuthStrategy defines how a request is authenticated.
//
// This keeps the authentication algorithm separate from the storage mechanism.
type AuthStrategy interface {
	Authenticate(request *context.RequestContext) AuthResult
}

// AuthStore abstracts where API-key authentication data is stored.
// The authentication strategy should not need to know that Redis is being used.
// RedisAuthStore will eventually implement this interface.
type AuthStore interface {
	GetClientID(apiKey string) (string, bool)
}
