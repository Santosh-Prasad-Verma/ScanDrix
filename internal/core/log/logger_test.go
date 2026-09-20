package log

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestStructuredLogger(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := CreateLogger("TestComponent")
	logger.SetOutput(buf)
	logger.SetMinLevel(LevelDebug)

	ctx := context.WithValue(context.Background(), "trace_id", "trace-12345")
	ctx = context.WithValue(ctx, "span_id", "span-67890")
	ctx = context.WithValue(ctx, "correlation_id", "corr-abcde")

	ctxLogger := logger.WithContext(ctx)

	ctxLogger.Info(LogArguments{
		Message: "Hello structured logger",
		Metadata: map[string]interface{}{
			"user": "developer",
		},
	})

	output := buf.String()
	if !strings.Contains(output, "Hello structured logger") {
		t.Fatalf("expected message in output, got %s", output)
	}
	if !strings.Contains(output, "trace-12345") {
		t.Fatalf("expected trace_id in output, got %s", output)
	}
	if !strings.Contains(output, "span-67890") {
		t.Fatalf("expected span_id in output, got %s", output)
	}
	if !strings.Contains(output, "corr-abcde") {
		t.Fatalf("expected correlation_id in output, got %s", output)
	}
}

func TestStructuredLoggerConcurrency(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := CreateLogger("ConcurrentLogger")
	logger.SetOutput(buf)

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			logger.Info(LogArguments{
				Message: "concurrent log message",
				Metadata: map[string]interface{}{
					"goroutine": id,
				},
			})
			logger.Warn(LogArguments{
				Message: "warning event",
			})
			logger.Error(LogArguments{
				Message: "error event",
				Error:   errors.New("test error"),
			})
		}(i)
	}
	wg.Wait()
}

func TestLoggerWrapperService(t *testing.T) {
	wrapper := NewLoggerWrapperService("WrapperService")
	wrapper.Log("info message", "TestContext", map[string]interface{}{"key": "val"})
	wrapper.Debug("debug message", "TestContext")
	wrapper.Warn("warn message", "TestContext")
	wrapper.Error("error message", errors.New("boom"), "TestContext")

	type testContextKey string
	ctx := context.WithValue(context.Background(), testContextKey("trace_id"), "t-1")
	ctxWrapper := wrapper.WithContext(ctx)
	ctxWrapper.Log("contextual info", "TestContext")
}

func TestObservabilityServiceSanitization(t *testing.T) {
	obs := NewObservabilityService()

	raw := map[string]interface{}{
		"normal":   "safe-value",
		"password": "super-secret-password",
		"token":    "abc123456",
		"nested": map[string]interface{}{
			"api_key": "live-key-12345",
			"auth":    "Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.e30.t-IDN-dOvd9Wn-JWnBgjYxFeC",
		},
	}

	sanitized := obs.SanitizeMetadata(raw)

	if sanitized["password"] != "[REDACTED]" {
		t.Fatalf("expected password to be redacted, got %v", sanitized["password"])
	}
	if sanitized["token"] != "[REDACTED]" {
		t.Fatalf("expected token to be redacted, got %v", sanitized["token"])
	}
	nested := sanitized["nested"].(map[string]interface{})
	if nested["api_key"] != "[REDACTED]" {
		t.Fatalf("expected nested api_key to be redacted, got %v", nested["api_key"])
	}
	if !strings.Contains(nested["auth"].(string), "REDACTED") {
		t.Fatalf("expected nested auth bearer/jwt to be redacted, got %v", nested["auth"])
	}
}

func TestLangfuseManager(t *testing.T) {
	cfg := LangfuseConfig{
		PublicKey:      "pk-test",
		SecretKey:      "sk-test",
		Host:           "https://test.langfuse.com",
		TracingEnabled: true,
		FlushInterval:  100 * time.Millisecond,
		BatchSize:      5,
	}
	lm := NewLangfuseManager(cfg)

	span := &LangfuseSpan{
		ID:        "span-1",
		TraceID:   "trace-1",
		Name:      "llm-call",
		StartTime: time.Now(),
	}

	lm.RecordSpan(span)
	err := lm.Flush(context.Background())
	if err != nil {
		t.Fatalf("expected nil error on flush, got %v", err)
	}

	_ = lm.Shutdown(context.Background())
}
