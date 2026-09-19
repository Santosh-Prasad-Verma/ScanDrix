package log

import (
	"context"
	"sync"
)

// LoggerWrapperService provides an injectable, thread-safe logger service wrapper
// compatible with multi-tier enterprise services.
type LoggerWrapperService struct {
	mu     sync.RWMutex
	logger *StructuredLogger
}

// NewLoggerWrapperService constructs a new LoggerWrapperService instance.
func NewLoggerWrapperService(serviceName string) *LoggerWrapperService {
	return &LoggerWrapperService{
		logger: CreateLogger(serviceName),
	}
}

// Log logs an informational message.
func (s *LoggerWrapperService) Log(message string, contextName string, metadata ...map[string]interface{}) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var meta map[string]interface{}
	if len(metadata) > 0 {
		meta = metadata[0]
	}
	s.logger.Info(LogArguments{
		Message:  message,
		Context:  contextName,
		Metadata: meta,
	})
}

// Debug logs a debug-level message.
func (s *LoggerWrapperService) Debug(message string, contextName string, metadata ...map[string]interface{}) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var meta map[string]interface{}
	if len(metadata) > 0 {
		meta = metadata[0]
	}
	s.logger.Debug(LogArguments{
		Message:  message,
		Context:  contextName,
		Metadata: meta,
	})
}

// Warn logs a warning message.
func (s *LoggerWrapperService) Warn(message string, contextName string, metadata ...map[string]interface{}) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var meta map[string]interface{}
	if len(metadata) > 0 {
		meta = metadata[0]
	}
	s.logger.Warn(LogArguments{
		Message:  message,
		Context:  contextName,
		Metadata: meta,
	})
}

// Error logs an error message with optional error and metadata.
func (s *LoggerWrapperService) Error(message string, err error, contextName string, metadata ...map[string]interface{}) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var meta map[string]interface{}
	if len(metadata) > 0 {
		meta = metadata[0]
	}
	s.logger.Error(LogArguments{
		Message:  message,
		Context:  contextName,
		Error:    err,
		Metadata: meta,
	})
}

// WithContext returns a context-bound logger wrapper.
func (s *LoggerWrapperService) WithContext(ctx context.Context) *LoggerWrapperService {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return &LoggerWrapperService{
		logger: s.logger.WithContext(ctx).(*StructuredLogger),
	}
}
