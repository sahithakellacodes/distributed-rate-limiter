package context

import "time"

type ResponseContext struct {
	StatusCode int
	Headers    map[string]string
	Body       []byte
	EndTime    time.Time
}
