package context

type ResponseContext struct {
	StatusCode int
	Headers    map[string]string
	Body       []byte
}
