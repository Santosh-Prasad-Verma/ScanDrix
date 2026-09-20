package httpclient

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestWith429Retry_Success(t *testing.T) {
	opts := WithRetryOptions{
		MaxAttempts: 3,
		BaseDelay:   5 * time.Millisecond,
		MaxDelay:    20 * time.Millisecond,
	}

	callCount := 0
	val, err := With429Retry(context.Background(), opts, func(ctx context.Context, attempt int) (string, error) {
		callCount++
		if attempt < 2 {
			return "", &HTTPStatusError{
				Code:    http.StatusTooManyRequests,
				Message: "rate limited",
			}
		}
		return "success", nil
	})

	assert.NoError(t, err)
	assert.Equal(t, "success", val)
	assert.Equal(t, 3, callCount)
}

func TestWith429Retry_NonRetryable(t *testing.T) {
	opts := WithRetryOptions{
		MaxAttempts: 3,
		BaseDelay:   5 * time.Millisecond,
		MaxDelay:    20 * time.Millisecond,
	}

	callCount := 0
	nonRetryableErr := errors.New("invalid credentials")
	_, err := With429Retry(context.Background(), opts, func(ctx context.Context, attempt int) (string, error) {
		callCount++
		return "", nonRetryableErr
	})

	assert.Error(t, err)
	assert.Equal(t, 1, callCount, "should fail fast on non-retryable error")
}

func TestParseRetryAfter_Seconds(t *testing.T) {
	headers := http.Header{}
	headers.Set("Retry-After", "5")
	err := &HTTPStatusError{
		Code:    429,
		Headers: headers,
	}

	dur := parseRetryAfter(err)
	assert.Equal(t, 5*time.Second, dur)
}
