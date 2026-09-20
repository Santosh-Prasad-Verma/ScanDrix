// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise MCP Server Protocol Errors
// File: errors.go
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package protocol

import (
	"errors"
	"fmt"
	"os"
)

// JsonRpcCode represents standard and extended JSON-RPC 2.0 error codes.
type JsonRpcCode int

const (
	ParseError     JsonRpcCode = -32700
	InvalidRequest JsonRpcCode = -32600
	MethodNotFound JsonRpcCode = -32601
	InvalidParams  JsonRpcCode = -32602
	InternalError  JsonRpcCode = -32603
	// Server error range: -32000 to -32099
	ServerError     JsonRpcCode = -32000
	Timeout         JsonRpcCode = -32001
	RateLimit       JsonRpcCode = -32002
	BackendError    JsonRpcCode = -32003
	AccessDenied    JsonRpcCode = -32004
	NotFound        JsonRpcCode = -32005
	ValidationError JsonRpcCode = -32006
)

// Standard typed domain errors.
type DomainError struct {
	Code    JsonRpcCode
	Name    string
	Message string
	Data    map[string]any
}

func (e *DomainError) Error() string {
	return e.Message
}

func NewNotFoundError(msg string, data map[string]any) *DomainError {
	return &DomainError{Code: NotFound, Name: "NotFoundError", Message: msg, Data: data}
}

func NewAccessDeniedError(msg string, data map[string]any) *DomainError {
	return &DomainError{Code: AccessDenied, Name: "AccessDeniedError", Message: msg, Data: data}
}

func NewBackendError(msg string, data map[string]any) *DomainError {
	return &DomainError{Code: BackendError, Name: "BackendError", Message: msg, Data: data}
}

func NewTimeoutError(msg string, data map[string]any) *DomainError {
	return &DomainError{Code: Timeout, Name: "TimeoutError", Message: msg, Data: data}
}

func NewRateLimitError(msg string, data map[string]any) *DomainError {
	return &DomainError{Code: RateLimit, Name: "RateLimitError", Message: msg, Data: data}
}

func NewValidationError(msg string, data map[string]any) *DomainError {
	return &DomainError{Code: ValidationError, Name: "ValidationError", Message: msg, Data: data}
}

// JSONRPCErrorResponse represents the standard JSON-RPC 2.0 error wire structure.
type JSONRPCErrorResponse struct {
	JSONRPC string       `json:"jsonrpc"`
	ID      any          `json:"id"`
	Error   JSONRPCError `json:"error"`
}

// JSONRPCError holds the error payload inside JSONRPCErrorResponse.
type JSONRPCError struct {
	Code    int            `json:"code"`
	Message string         `json:"message"`
	Data    map[string]any `json:"data,omitempty"`
}

// ToolErrorPayload represents the structured error payload for MCP tool execution failures.
type ToolErrorPayload struct {
	Code    int            `json:"code"`
	Name    string         `json:"name"`
	Message string         `json:"message"`
	Data    map[string]any `json:"data,omitempty"`
}

// SafeData filters error metadata through a strict whitelist to prevent leaking credentials,
// tokens, or secrets into client responses (Master Rules 1.1-1.7, 5.7).
func SafeData(data map[string]any) map[string]any {
	if data == nil {
		return nil
	}

	whitelistedKeys := []string{
		"service",
		"resource",
		"httpStatus",
		"method",
		"url",
		"retryAfter",
		"retryable",
		"requestId",
		"reason",
		"tool",
	}

	out := make(map[string]any)
	for _, k := range whitelistedKeys {
		if val, exists := data[k]; exists && val != nil {
			out[k] = val
		}
	}

	if len(out) == 0 {
		return nil
	}
	return out
}

// MapError inspects any error and maps it to a canonical JsonRpcCode, message, and sanitized data.
func MapError(err error) (JsonRpcCode, string, string, map[string]any) {
	if err == nil {
		return InternalError, "Internal error", "InternalError", nil
	}

	var dErr *DomainError
	if errors.As(err, &dErr) {
		return dErr.Code, dErr.Message, dErr.Name, SafeData(dErr.Data)
	}

	msg := err.Error()
	return InternalError, msg, "Error", nil
}

// ToJSONRPCError creates a production-hardened JSON-RPC 2.0 error response.
func ToJSONRPCError(err error, id any) JSONRPCErrorResponse {
	code, msg, name, data := MapError(err)

	outData := make(map[string]any)
	for k, v := range data {
		outData[k] = v
	}
	outData["name"] = name

	// Only attach stack or debug reason in non-production environments
	if os.Getenv("APP_ENV") != "production" && os.Getenv("NODE_ENV") != "production" {
		outData["debug"] = fmt.Sprintf("%+v", err)
	}

	return JSONRPCErrorResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error: JSONRPCError{
			Code:    int(code),
			Message: msg,
			Data:    outData,
		},
	}
}

// ToToolErrorPayload creates an error payload for ToolResult.isError = true.
func ToToolErrorPayload(err error) ToolErrorPayload {
	code, msg, name, data := MapError(err)
	return ToolErrorPayload{
		Code:    int(code),
		Name:    name,
		Message: msg,
		Data:    data,
	}
}
