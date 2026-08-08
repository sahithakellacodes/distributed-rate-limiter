package router

import (
	"bytes"
	"io"
	"net/http"

	"strings"

	"github.com/sahithakellacodes/distributed-rate-limiter/internal/context"
)

type DefaultHttpClient struct {
	client *http.Client
}

func NewDefaultHttpClient() *DefaultHttpClient {
	return &DefaultHttpClient{
		client: &http.Client{},
	}
}

func (c *DefaultHttpClient) Forward(
	request *context.RequestContext,
	backendBaseURL string,
) (*HttpResponse, error) {

	targetURL := strings.TrimRight(backendBaseURL, "/") + request.Path

	backendRequest, err := http.NewRequest(
		request.Method,
		targetURL,
		bytes.NewReader(request.Body),
	)
	if err != nil {
		return nil, err
	}

	// Forward request headers.
	for name, value := range request.Headers {
		backendRequest.Header.Set(name, value)
	}

	backendResponse, err := c.client.Do(backendRequest)
	if err != nil {
		return nil, err
	}
	defer backendResponse.Body.Close()

	body, err := io.ReadAll(backendResponse.Body)
	if err != nil {
		return nil, err
	}

	headers := make(map[string]string)

	for name, values := range backendResponse.Header {
		if len(values) > 0 {
			headers[name] = values[0]
		}
	}

	return &HttpResponse{
		StatusCode: backendResponse.StatusCode,
		Headers:    headers,
		Body:       body,
	}, nil
}
