package log

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"runtime"
	"sync"
	"time"
)

// LogLevel defines the severity level of a log entry.
type LogLevel string

const (
	LevelDebug LogLevel = "debug"
	LevelInfo  LogLevel = "info"
	LevelWarn  LogLevel = "warn"
	LevelError LogLevel = "error"
)

// LogContext represents arbitrary contextual key-value pairs.
type LogContext map[string]interface{}

// LogArguments mirrors the structured logger parameters used across the platform.
type LogArguments struct {
	Message     string                 `json:"message"`
	Context     string                 `json:"context,omitempty"`
	ServiceName string                 `json:"serviceName,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
	Error       error                  `json:"-"`
	ErrorString string                 `json:"error,omitempty"`
	TraceID     string                 `json:"traceId,omitempty"`
	SpanID      string                 `json:"spanId,omitempty"`
	CorrelationID string               `json:"correlationId,omitempty"`
	Timestamp   string                 `json:"timestamp"`
	Level       LogLevel               `json:"level"`
	Caller      string                 `json:"caller,omitempty"`
}

// Logger is the platform-wide contract for structured, contextual logging.
type Logger interface {
	Debug(args LogArguments)
	Info(args LogArguments)
	Warn(args LogArguments)
	Error(args LogArguments)
	WithContext(ctx context.Context) Logger
}

// StructuredLogger is a high-performance, thread-safe logger writing JSON entries.
type StructuredLogger struct {
	mu          sync.RWMutex
	writeMu     sync.Mutex
	component   string
	out         io.Writer
	minLevel    LogLevel
	ctx         context.Context
	extraFields map[string]interface{}
}

// CreateLogger creates a new StructuredLogger bound to a specific component or service name.
func CreateLogger(component string) *StructuredLogger {
	minLvl := LevelInfo
	if env := os.Getenv("LOG_LEVEL"); env != "" {
		switch env {
		case "debug", "DEBUG":
			minLvl = LevelDebug
		case "warn", "WARN":
			minLvl = LevelWarn
		case "error", "ERROR":
			minLvl = LevelError
		default:
			minLvl = LevelInfo
		}
	} else if os.Getenv("SCANDRIX_DEBUG") == "true" || os.Getenv("ENV") == "development" {
		minLvl = LevelDebug
	}

	return &StructuredLogger{
		component:   component,
		out:         os.Stdout,
		minLevel:    minLvl,
		ctx:         context.Background(),
		extraFields: make(map[string]interface{}),
	}
}

// SetOutput changes the output destination (useful for testing or log capture).
func (l *StructuredLogger) SetOutput(w io.Writer) {
	l.writeMu.Lock()
	defer l.writeMu.Unlock()
	l.mu.Lock()
	defer l.mu.Unlock()
	l.out = w
}

// SetMinLevel sets the minimum log level filter.
func (l *StructuredLogger) SetMinLevel(level LogLevel) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.minLevel = level
}

// WithContext attaches a request context for tracing extraction.
func (l *StructuredLogger) WithContext(ctx context.Context) Logger {
	l.mu.RLock()
	nl := &StructuredLogger{
		component:   l.component,
		out:         l.out,
		minLevel:    l.minLevel,
		ctx:         ctx,
		extraFields: make(map[string]interface{}),
	}
	for k, v := range l.extraFields {
		nl.extraFields[k] = v
	}
	l.mu.RUnlock()
	return nl
}

// WithField adds a persistent field to the logger.
func (l *StructuredLogger) WithField(key string, val interface{}) *StructuredLogger {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.extraFields[key] = val
	return l
}

func (l *StructuredLogger) shouldLog(lvl LogLevel) bool {
	l.mu.RLock()
	defer l.mu.RUnlock()
	levels := map[LogLevel]int{
		LevelDebug: 0,
		LevelInfo:  1,
		LevelWarn:  2,
		LevelError: 3,
	}
	return levels[lvl] >= levels[l.minLevel]
}

func (l *StructuredLogger) log(level LogLevel, args LogArguments) {
	if !l.shouldLog(level) {
		return
	}

	l.mu.RLock()
	out := l.out
	ctx := l.ctx
	comp := l.component
	extras := make(map[string]interface{})
	for k, v := range l.extraFields {
		extras[k] = v
	}
	l.mu.RUnlock()

	args.Level = level
	args.Timestamp = time.Now().UTC().Format(time.RFC3339Nano)
	if args.ServiceName == "" {
		args.ServiceName = comp
	}
	if args.Context == "" {
		args.Context = comp
	}
	if args.Error != nil && args.ErrorString == "" {
		args.ErrorString = args.Error.Error()
	}

	// Capture caller
	_, file, line, ok := runtime.Caller(2)
	if ok {
		args.Caller = fmt.Sprintf("%s:%d", file, line)
	}

	// Context trace extraction
	if ctx != nil {
		if tid, ok := ctx.Value("trace_id").(string); ok && args.TraceID == "" {
			args.TraceID = tid
		}
		if sid, ok := ctx.Value("span_id").(string); ok && args.SpanID == "" {
			args.SpanID = sid
		}
		if cid, ok := ctx.Value("correlation_id").(string); ok && args.CorrelationID == "" {
			args.CorrelationID = cid
		}
	}

	// Merge metadata
	if args.Metadata == nil {
		args.Metadata = make(map[string]interface{})
	}
	for k, v := range extras {
		if _, exists := args.Metadata[k]; !exists {
			args.Metadata[k] = v
		}
	}

	data, err := json.Marshal(args)
	if err != nil {
		l.writeMu.Lock()
		fmt.Fprintf(out, `{"level":"error","message":"failed to marshal log entry","error":"%v","timestamp":"%s"}`+"\n", err, args.Timestamp)
		l.writeMu.Unlock()
		return
	}

	l.writeMu.Lock()
	fmt.Fprintln(out, string(data))
	l.writeMu.Unlock()
}

// Debug logs a debug-level entry.
func (l *StructuredLogger) Debug(args LogArguments) {
	l.log(LevelDebug, args)
}

// Info logs an info-level entry.
func (l *StructuredLogger) Info(args LogArguments) {
	l.log(LevelInfo, args)
}

// Warn logs a warning-level entry.
func (l *StructuredLogger) Warn(args LogArguments) {
	l.log(LevelWarn, args)
}

// Error logs an error-level entry.
func (l *StructuredLogger) Error(args LogArguments) {
	l.log(LevelError, args)
}
