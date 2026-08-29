package review

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
)

// StreamEvent models an SSE push message sent to connected clients.
type StreamEvent struct {
	Event string `json:"event"`
	Stage string `json:"stage"`
	Data  any    `json:"data"`
	Time  string `json:"timestamp"`
}

// StreamHub manages real-time event distribution for active review jobs.
type StreamHub struct {
	mu          sync.RWMutex
	subscribers map[uuid.UUID][]chan StreamEvent
}

// NewStreamHub initializes the SSE streaming hub.
func NewStreamHub() *StreamHub {
	return &StreamHub{
		subscribers: make(map[uuid.UUID][]chan StreamEvent),
	}
}

// Subscribe attaches an HTTP client channel to a specific review ID.
func (h *StreamHub) Subscribe(reviewID uuid.UUID) (chan StreamEvent, func()) {
	h.mu.Lock()
	defer h.mu.Unlock()

	ch := make(chan StreamEvent, 32)
	h.subscribers[reviewID] = append(h.subscribers[reviewID], ch)

	unsubscribe := func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		subs := h.subscribers[reviewID]
		for i, sub := range subs {
			if sub == ch {
				h.subscribers[reviewID] = append(subs[:i], subs[i+1:]...)
				close(ch)
				break
			}
		}
	}

	return ch, unsubscribe
}

// Broadcast sends an event to all clients watching a review.
func (h *StreamHub) Broadcast(reviewID uuid.UUID, eventType, stage string, payload any) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	subs := h.subscribers[reviewID]
	if len(subs) == 0 {
		return
	}

	evt := StreamEvent{
		Event: eventType,
		Stage: stage,
		Data:  payload,
		Time:  time.Now().UTC().Format(time.RFC3339),
	}

	for _, ch := range subs {
		select {
		case ch <- evt:
		default:
			// Non-blocking drop if client is too slow
		}
	}
}

// ServeHTTP handles Server-Sent Events requests for a review stream.
func (h *StreamHub) HandleSSE(w http.ResponseWriter, r *http.Request, reviewID uuid.UUID) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	ch, unsubscribe := h.Subscribe(reviewID)
	defer unsubscribe()

	// Send initial connection event
	initData, _ := json.Marshal(map[string]string{"status": "connected", "review_id": reviewID.String()})
	fmt.Fprintf(w, "event: init\ndata: %s\n\n", string(initData))
	flusher.Flush()

	notify := r.Context().Done()
	for {
		select {
		case <-notify:
			return
		case evt, ok := <-ch:
			if !ok {
				return
			}
			bytes, err := json.Marshal(evt)
			if err != nil {
				continue
			}
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", evt.Event, string(bytes))
			flusher.Flush()
		}
	}
}
