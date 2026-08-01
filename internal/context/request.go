package context

import "time"

type RequestContext struct {
	Path      string
	Method    string
	Headers   map[string]string
	Body      []byte
	StartTime time.Time

	Data map[string]any
}