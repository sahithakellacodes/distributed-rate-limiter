package logger

type LoggerStrategy interface {
	Info(message string, fields map[string]any)
	Error(message string, fields map[string]any)
}
