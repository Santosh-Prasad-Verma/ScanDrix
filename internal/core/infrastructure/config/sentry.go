package config

import (
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/scandrix/backend/internal/core/log"
)

var (
	sentryMu          sync.RWMutex
	sentryInitialized bool
	sentryLogger      = log.CreateLogger("Sentry")
)

// SentryOptions configures exception and error capturing to Sentry.
type SentryOptions struct {
	ComponentType string
	Environment   string
	Release       string
	DSN           string
	SampleRate    float64
}

// SetupSentry initializes Sentry integration if SENTRY_DSN or API_BETTERSTACK_DSN is present.
func SetupSentry(componentType string) bool {
	sentryMu.Lock()
	defer sentryMu.Unlock()

	if sentryInitialized {
		return true
	}

	dsn := os.Getenv("SENTRY_DSN")
	if dsn == "" {
		dsn = os.Getenv("API_BETTERSTACK_DSN")
	}
	if dsn == "" {
		return false
	}

	env := os.Getenv("SCANDRIX_ENV")
	if env == "" {
		env = os.Getenv("API_NODE_ENV")
	}
	if env == "" {
		env = os.Getenv("NODE_ENV")
	}
	if env == "" {
		env = "development"
	}

	release := os.Getenv("SENTRY_RELEASE")
	if release == "" {
		release = fmt.Sprintf("scandrix@1.0.0-%s", env)
	}

	err := sentry.Init(sentry.ClientOptions{
		Dsn:              dsn,
		Environment:      env,
		Release:          release,
		ServerName:       fmt.Sprintf("scandrix-%s", componentType),
		AttachStacktrace: true,
	})
	if err != nil {
		sentryLogger.Warn(log.LogArguments{
			Message: fmt.Sprintf("Failed to initialize Sentry SDK: %v", err),
			Context: "Sentry",
		})
		return false
	}

	sentryLogger.Info(log.LogArguments{
		Message: "Sentry error tracking initialized",
		Context: "Sentry",
		Metadata: map[string]interface{}{
			"component":   componentType,
			"environment": env,
			"release":     release,
		},
	})

	sentryInitialized = true
	return true
}

// ReportExceptionToSentry reports a critical error or panic with tags and metadata to Sentry.
func ReportExceptionToSentry(err error, contextName string, tags map[string]string, extra map[string]interface{}) {
	sentryMu.RLock()
	isInit := sentryInitialized
	sentryMu.RUnlock()

	if !isInit || err == nil {
		return
	}

	meta := make(map[string]interface{})
	for k, v := range extra {
		meta[k] = v
	}
	for k, v := range tags {
		meta["tag_"+k] = v
	}
	meta["context"] = contextName
	meta["timestamp"] = time.Now().UTC().Format(time.RFC3339Nano)

	sentry.WithScope(func(scope *sentry.Scope) {
		if contextName != "" {
			scope.SetTag("context", contextName)
		}
		if len(tags) > 0 {
			scope.SetTags(tags)
		}
		if len(extra) > 0 {
			scope.SetContext("extra", extra)
		}
		sentry.CaptureException(err)
	})

	sentryLogger.Error(log.LogArguments{
		Message:  fmt.Sprintf("[Sentry Capture] %s: %v", contextName, err),
		Context:  "SentryReporter",
		Error:    err,
		Metadata: meta,
	})

	_ = sentry.Flush(2 * time.Second)
}

// FlushSentry flushes queued events before shutdown.
func FlushSentry(timeout time.Duration) {
	sentryMu.RLock()
	isInit := sentryInitialized
	sentryMu.RUnlock()

	if isInit {
		sentry.Flush(timeout)
	}
}
