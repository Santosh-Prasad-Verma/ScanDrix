package audit

import (
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSIEMExporter_StreamingFormats(t *testing.T) {
	var receivedCount int64
	var lastBody string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		lastBody = string(body)
		atomic.AddInt64(&receivedCount, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	streamer := NewSIEMAuditStreamer()
	cfg := SIEMExporterConfig{
		EndpointURL:   server.URL,
		Format:        ExportFormatCEF,
		BatchSize:     2,
		FlushInterval: 50 * time.Millisecond,
		Timeout:       2 * time.Second,
	}

	exporter := NewSIEMExporter(streamer, cfg)
	defer exporter.Close()

	ev1 := streamer.RecordEvent(uuid.New(), "usr-1", "user@scandrix.dev", "127.0.0.1", ActionUpdate, "Rules", "rule-1", "old", "new")
	ev2 := streamer.RecordEvent(uuid.New(), "usr-2", "admin@scandrix.dev", "127.0.0.1", ActionDelete, "Rules", "rule-2", "active", "deleted")

	err := exporter.Send(ev1)
	require.NoError(t, err)
	err = exporter.Send(ev2)
	require.NoError(t, err)

	// Wait for batch flush
	time.Sleep(150 * time.Millisecond)

	assert.True(t, atomic.LoadInt64(&receivedCount) >= 1)
	assert.Contains(t, lastBody, "CEF:0|Scandrix|EnterprisePlatform")
	assert.Contains(t, lastBody, "user@scandrix.dev")
}
