package errors_test

import (
	"errors"
	"net/http"
	"testing"

	apperrors "github.com/codehound/codehound/shared/pkg/errors"
)

func TestAppErrorCodesAndStatus(t *testing.T) {
	tests := []struct {
		code         apperrors.Code
		expectedHTTP int
	}{
		{apperrors.CodeNotFound, http.StatusNotFound},
		{apperrors.CodeUnauthorized, http.StatusUnauthorized},
		{apperrors.CodeForbidden, http.StatusForbidden},
		{apperrors.CodeTenantMismatch, http.StatusForbidden},
		{apperrors.CodeConflict, http.StatusConflict},
		{apperrors.CodeInvalidInput, http.StatusBadRequest},
		{apperrors.CodeQuotaExceeded, http.StatusTooManyRequests},
		{apperrors.CodeServiceUnavailable, http.StatusServiceUnavailable},
		{apperrors.CodeInternalError, http.StatusInternalServerError},
	}

	for _, tt := range tests {
		err := apperrors.New(tt.code, "test message")
		if err.HTTPStatusCode() != tt.expectedHTTP {
			t.Errorf("for code %s, expected HTTP status %d, got %d", tt.code, tt.expectedHTTP, err.HTTPStatusCode())
		}
	}
}

func TestAppErrorWrapAndUnwrap(t *testing.T) {
	origErr := errors.New("underlying io timeout")
	wrapped := apperrors.Wrap(apperrors.CodeServiceUnavailable, "database down", origErr)

	if !errors.Is(wrapped, origErr) {
		t.Errorf("expected wrapped error to match original error with errors.Is")
	}

	if wrapped.Unwrap() != origErr {
		t.Errorf("expected unwrapped error to equal origErr")
	}

	expectedStr := "[SERVICE_UNAVAILABLE] database down: underlying io timeout"
	if wrapped.Error() != expectedStr {
		t.Errorf("expected error string %q, got %q", expectedStr, wrapped.Error())
	}
}
