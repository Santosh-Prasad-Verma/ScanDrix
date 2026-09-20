// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package utils

import (
	"fmt"
	"io"
	"os"
	"sync"
)

// LOGGING STATE & WRITERS

var (
	loggerMu  sync.RWMutex
	quiet     bool
	verbose   bool
	outWriter io.Writer = os.Stdout
	errWriter io.Writer = os.Stderr
)

// SetOutputMode sets global quiet and verbose flags for CLI output.
func SetOutputMode(q, v bool) {
	loggerMu.Lock()
	defer loggerMu.Unlock()
	quiet = q
	verbose = v
}

// IsQuiet returns whether quiet mode is enabled.
func IsQuiet() bool {
	loggerMu.RLock()
	defer loggerMu.RUnlock()
	return quiet
}

// IsVerbose returns whether verbose debug mode is enabled.
func IsVerbose() bool {
	loggerMu.RLock()
	defer loggerMu.RUnlock()
	return verbose
}

// SetWriters allows overriding stdout and stderr for testing.
func SetWriters(out, err io.Writer) {
	loggerMu.Lock()
	defer loggerMu.Unlock()
	if out != nil {
		outWriter = out
	}
	if err != nil {
		errWriter = err
	}
}

// LOGGING METHODS (Info, Success, Warn, Error, Debug)

// Info writes informative messages unless quiet mode is active.
func Info(format string, args ...any) {
	loggerMu.RLock()
	defer loggerMu.RUnlock()
	if !quiet {
		fmt.Fprintf(outWriter, format+"\n", args...)
	}
}

// Success writes success message with green styling unless quiet mode is active.
func Success(format string, args ...any) {
	loggerMu.RLock()
	defer loggerMu.RUnlock()
	if !quiet {
		msg := fmt.Sprintf(format, args...)
		fmt.Fprintf(outWriter, "\033[32m%s\033[0m\n", msg)
	}
}

// Warn writes a warning message unless quiet mode is active.
func Warn(format string, args ...any) {
	loggerMu.RLock()
	defer loggerMu.RUnlock()
	if !quiet {
		msg := fmt.Sprintf(format, args...)
		fmt.Fprintf(errWriter, "\033[33m%s\033[0m\n", msg)
	}
}

// Error always writes error message to stderr.
func Error(format string, args ...any) {
	loggerMu.RLock()
	defer loggerMu.RUnlock()
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintf(errWriter, "\033[31m%s\033[0m\n", msg)
}

// Debug writes debug details if verbose mode is active.
func Debug(format string, args ...any) {
	loggerMu.RLock()
	defer loggerMu.RUnlock()
	if verbose {
		msg := fmt.Sprintf(format, args...)
		fmt.Fprintf(errWriter, "\033[90m[DEBUG] %s\033[0m\n", msg)
	}
}
