package errors

import (
	"fmt"
	"net/http"
)

// Code defines standardized application error codes.
type Code string

const (
	CodeNotFound          Code = "NOT_FOUND"
	CodeUnauthorized      Code = "UNAUTHORIZED"
	CodeForbidden         Code = "FORBIDDEN"
	CodeConflict          Code = "CONFLICT"
	CodeInvalidInput      Code = "INVALID_INPUT"
	CodeTenantMismatch    Code = "TENANT_MISMATCH"
	CodeQuotaExceeded     Code = "QUOTA_EXCEEDED"
	CodeInternalError     Code = "INTERNAL_ERROR"
	CodeServiceUnavailable Code = "SERVICE_UNAVAILABLE"
)

// AppError represents a structured, domain-level error.
type AppError struct {
	Code    Code   `json:"code"`
	Message string `json:"message"`
	Err     error  `json:"-"`
}

func (e *AppError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("[%s] %s: %v", e.Code, e.Message, e.Err)
	}
	return fmt.Sprintf("[%s] %s", e.Code, e.Message)
}

func (e *AppError) Unwrap() error {
	return e.Err
}

// HTTPStatusCode maps the domain code to standard HTTP status codes.
func (e *AppError) HTTPStatusCode() int {
	switch e.Code {
	case CodeNotFound:
		return http.StatusNotFound
	case CodeUnauthorized:
		return http.StatusUnauthorized
	case CodeForbidden, CodeTenantMismatch:
		return http.StatusForbidden
	case CodeConflict:
		return http.StatusConflict
	case CodeInvalidInput:
		return http.StatusBadRequest
	case CodeQuotaExceeded:
		return http.StatusTooManyRequests
	case CodeServiceUnavailable:
		return http.StatusServiceUnavailable
	default:
		return http.StatusInternalServerError
	}
}

// Factory helpers
func New(code Code, message string) *AppError {
	return &AppError{Code: code, Message: message}
}

func Wrap(code Code, message string, err error) *AppError {
	return &AppError{Code: code, Message: message, Err: err}
}
