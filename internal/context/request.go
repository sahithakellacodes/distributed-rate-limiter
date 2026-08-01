package context

type RequestContext struct {
	Path    string
	Method  string
	Headers map[string]string
	Body    []byte

	ClientID      string
	ResolvedRoute string
}
