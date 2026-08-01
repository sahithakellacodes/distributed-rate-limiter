package context

// Please note that headers in RequestContext are intentionally single-value only.
// This is because the middleware chain is designed to work with single-value headers
// and it simplifies the handling of headers in the context of a distributed rate limiter.
type RequestContext struct {
	Path    string
	Method  string
	Headers map[string]string
	Body    []byte

	ClientID      string
	ResolvedRoute string
}
