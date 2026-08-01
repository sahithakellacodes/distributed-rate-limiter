package auth

import (
	"net/http"
	"github.com/sahithakellacodes/distributed-rate-limiter/internal/context"
)

type APIKeyAuthStrategy struct {
	store AuthStore
}

func NewAPIKeyAuthStrategy(store AuthStore) *APIKeyAuthStrategy {
	return &APIKeyAuthStrategy{
		store: store,
	}
}

func (a *APIKeyAuthStrategy) Authenticate(
	request *context.RequestContext,
) AuthResult {
	apiKey := request.Headers[http.CanonicalHeaderKey("X-API-Key")]
	if apiKey == "" {
		return AuthResult{
			Authenticated: false,
			FailureReason: "missing API key",
		}
	}

	clientID, found := a.store.GetClientID(apiKey)
	if !found {
		return AuthResult{
			Authenticated: false,
			FailureReason: "invalid API key",
		}
	}

	return AuthResult{
		Authenticated: true,
		ClientID:      clientID,
	}
}
