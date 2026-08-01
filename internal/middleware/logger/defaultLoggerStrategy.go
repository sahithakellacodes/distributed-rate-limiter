package logger

import "log"

type DefaultLoggerStrategy struct{}

func NewDefaultLoggerStrategy() *DefaultLoggerStrategy {
	return &DefaultLoggerStrategy{}
}

func (l *DefaultLoggerStrategy) Info(
	message string,
	fields map[string]any,
) {
	log.Printf("\033[34mINFO\033[0m: %s %v", message, fields)
}

func (l *DefaultLoggerStrategy) Error(
	message string,
	fields map[string]any,
) {
	log.Printf("\033[31mERROR\033[0m: %s %v", message, fields)
}
