package interceptors

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestLoggingMiddleware(t *testing.T) {
	mgr := NewInterceptorManager("test-api")
	handler := mgr.LoggingMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		corrID := r.Context().Value("correlation_id")
		if corrID == nil || corrID == "" {
			t.Fatalf("expected correlation_id in context")
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))

	req := httptest.NewRequest("GET", "/test/path", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Header().Get("X-Correlation-ID") == "" {
		t.Fatalf("expected X-Correlation-ID header in response")
	}
}

func TestTimeoutMiddleware(t *testing.T) {
	mgr := NewInterceptorManager("test-api", 50*time.Millisecond)
	handler := mgr.TimeoutMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(100 * time.Millisecond):
			w.WriteHeader(http.StatusOK)
		case <-r.Context().Done():
			// Context canceled as expected
			return
		}
	}))

	req := httptest.NewRequest("GET", "/slow", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
}

func TestTransformResponse(t *testing.T) {
	mgr := NewInterceptorManager("test-api")
	w := httptest.NewRecorder()

	type SampleUser struct {
		Name  string `json:"name"`
		Email string `json:"email"`
	}

	user := SampleUser{Name: "Alice", Email: "alice@example.com"}
	mgr.TransformResponse(w, http.StatusOK, user)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var resp StandardDataResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected statusCode 200, got %d", resp.StatusCode)
	}
	if resp.Type != "interceptors.SampleUser" {
		t.Fatalf("expected type interceptors.SampleUser, got %s", resp.Type)
	}
}

func TestTransformResponseNotFound(t *testing.T) {
	mgr := NewInterceptorManager("test-api")
	w := httptest.NewRecorder()

	mgr.TransformResponse(w, http.StatusOK, nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for nil payload, got %d", w.Code)
	}
}
