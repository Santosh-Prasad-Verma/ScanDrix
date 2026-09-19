// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package trace

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type HookLogLevel string

const (
	LogLevelInfo  HookLogLevel = "INFO"
	LogLevelWarn  HookLogLevel = "WARN"
	LogLevelError HookLogLevel = "ERROR"
	LogLevelDebug HookLogLevel = "DEBUG"
)

// HookLogEntry is the structured record appended to ~/.scandrix/sessions/<repoKey>/logs/hooks.jsonl.
type HookLogEntry struct {
	Time      string         `json:"time"`
	Level     HookLogLevel   `json:"level"`
	Msg       string         `json:"msg"`
	Component string         `json:"component"`
	Fields    map[string]any `json:"fields,omitempty"`
}

// HookLogger writes scrubbed diagnostic records to the out-of-tree hook log.
type HookLogger struct {
	mu      sync.Mutex
	logPath string
}

// DefaultHookLogger is the process-wide hook logger instance.
var DefaultHookLogger = &HookLogger{}

// Init configures the logger for the target repository root.
func (l *HookLogger) Init(repoRoot string) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.logPath = HookLogPath(repoRoot)
	dir := filepath.Dir(l.logPath)
	return os.MkdirAll(dir, 0700)
}

func (l *HookLogger) Info(msg, component string, fields map[string]any) {
	l.Log(LogLevelInfo, msg, component, fields)
}

func (l *HookLogger) Warn(msg, component string, fields map[string]any) {
	l.Log(LogLevelWarn, msg, component, fields)
}

func (l *HookLogger) Error(msg, component string, fields map[string]any) {
	l.Log(LogLevelError, msg, component, fields)
}

func (l *HookLogger) Debug(msg, component string, fields map[string]any) {
	l.Log(LogLevelDebug, msg, component, fields)
}

// Log writes a scrubbed diagnostic log entry. Logging must NEVER fail or block the hook execution flow.
func (l *HookLogger) Log(level HookLogLevel, msg, component string, fields map[string]any) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.logPath == "" {
		return
	}

	var scrubbedFields map[string]any
	if fields != nil {
		if redacted, ok := RedactDeep(fields).(map[string]any); ok {
			scrubbedFields = redacted
		}
	}

	entry := map[string]any{
		"time":      time.Now().UTC().Format(time.RFC3339Nano),
		"level":     string(level),
		"msg":       Redact(msg),
		"component": component,
	}
	for k, v := range scrubbedFields {
		entry[k] = v
	}

	data, err := json.Marshal(entry)
	if err != nil {
		return
	}

	f, err := os.OpenFile(l.logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return
	}
	defer f.Close()

	_, _ = f.Write(append(data, '\n'))
}
