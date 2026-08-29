package streaming

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
)

// SSEHandler serves live review events over Server-Sent Events (SSE).
type SSEHandler struct {
	broker *StreamBroker
}

// NewSSEHandler initializes the SSE HTTP handler.
func NewSSEHandler(broker *StreamBroker) *SSEHandler {
	return &SSEHandler{
		broker: broker,
	}
}

// ServeHTTP handles incoming SSE client connections.
func (h *SSEHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	reviewIDStr := r.URL.Query().Get("review_id")
	reviewID, err := uuid.Parse(reviewIDStr)
	if err != nil {
		http.Error(w, "Invalid or missing review_id parameter", http.StatusBadRequest)
		return
	}

	// Configure standard SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	clientID := fmt.Sprintf("client-%s-%d", uuid.New().String()[:8], time.Now().UnixNano())
	sub := h.broker.Subscribe(reviewID, clientID)
	defer h.broker.Unsubscribe(sub)

	// Send initial connected handshake event
	initEvent := StreamEvent{
		ID:        uuid.New(),
		ReviewID:  reviewID,
		Type:      EventHeartbeat,
		Payload:   map[string]string{"status": "connected", "client_id": clientID},
		Timestamp: time.Now().UTC(),
	}
	writeSSEFrame(w, initEvent)
	flusher.Flush()

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-sub.EventChan:
			if !ok {
				return
			}
			writeSSEFrame(w, event)
			flusher.Flush()
		}
	}
}

func writeSSEFrame(w http.ResponseWriter, event StreamEvent) {
	dataBytes, _ := json.Marshal(event.Payload)
	fmt.Fprintf(w, "id: %s\nevent: %s\ndata: %s\n\n", event.ID.String(), string(event.Type), string(dataBytes))
}
