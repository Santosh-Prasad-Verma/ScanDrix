package review

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/cache"
)

// StreamEvent models an SSE push message sent to connected clients.
type StreamEvent struct {
	Event string `json:"event"`
	Stage string `json:"stage"`
	Data  any    `json:"data"`
	Time  string `json:"timestamp"`
}

type streamRedisMessage struct {
	ReviewID  uuid.UUID   `json:"review_id"`
	EventType string      `json:"event_type"`
	Stage     string      `json:"stage"`
	Data      any         `json:"data"`
	Time      string      `json:"timestamp"`
}

// StreamHub manages real-time event distribution for active review jobs,
// bridging worker processes and API server SSE listeners via Redis Pub/Sub.
type StreamHub struct {
	mu           sync.RWMutex
	subscribers  map[uuid.UUID][]chan StreamEvent
	redisClient  *cache.Client
	redisCancels map[uuid.UUID]context.CancelFunc
}

// NewStreamHub initializes the SSE streaming hub.
func NewStreamHub() *StreamHub {
	return &StreamHub{
		subscribers:  make(map[uuid.UUID][]chan StreamEvent),
		redisCancels: make(map[uuid.UUID]context.CancelFunc),
	}
}

// SetRedisClient attaches a Redis distributed pub/sub client to bridge worker and API nodes.
func (h *StreamHub) SetRedisClient(rc *cache.Client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.redisClient = rc
}

// Subscribe attaches an HTTP client channel to a specific review ID.
func (h *StreamHub) Subscribe(reviewID uuid.UUID) (chan StreamEvent, func()) {
	h.mu.Lock()
	defer h.mu.Unlock()

	ch := make(chan StreamEvent, 64)
	h.subscribers[reviewID] = append(h.subscribers[reviewID], ch)

	// If Redis is configured and this is the first local subscriber for this reviewID,
	// spawn a background subscriber to listen to Redis channel reviews:{id}:events
	if h.redisClient != nil && len(h.subscribers[reviewID]) == 1 {
		ctx, cancel := context.WithCancel(context.Background())
		h.redisCancels[reviewID] = cancel
		go h.listenRedisChannel(ctx, reviewID)
	}

	var once sync.Once
	unsubscribe := func() {
		once.Do(func() {
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

			if len(h.subscribers[reviewID]) == 0 {
				delete(h.subscribers, reviewID)
				if cancel, exists := h.redisCancels[reviewID]; exists {
					cancel()
					delete(h.redisCancels, reviewID)
				}
			}
		})
	}

	return ch, unsubscribe
}

// Broadcast sends an event to all clients watching a review.
// It delivers to local in-process subscribers and publishes to Redis Pub/Sub for cross-pod subscribers.
func (h *StreamHub) Broadcast(reviewID uuid.UUID, eventType, stage string, payload any) {
	evt := StreamEvent{
		Event: eventType,
		Stage: stage,
		Data:  payload,
		Time:  time.Now().UTC().Format(time.RFC3339),
	}

	// 1. Deliver to local in-memory subscribers (if any)
	h.broadcastLocal(reviewID, evt)

	// 2. Distribute across cluster via Redis Pub/Sub if client is attached
	h.mu.RLock()
	rc := h.redisClient
	h.mu.RUnlock()

	if rc != nil {
		channelName := fmt.Sprintf("reviews:%s:events", reviewID.String())
		rmsg := streamRedisMessage{
			ReviewID:  reviewID,
			EventType: eventType,
			Stage:     stage,
			Data:      payload,
			Time:      evt.Time,
		}
		if data, err := json.Marshal(rmsg); err == nil {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			_ = rc.Publish(ctx, channelName, string(data))
			cancel()
		}
	}
}

// broadcastLocal delivers to all registered local channel subscribers without holding mutex during send.
func (h *StreamHub) broadcastLocal(reviewID uuid.UUID, evt StreamEvent) {
	h.mu.RLock()
	subs := h.subscribers[reviewID]
	if len(subs) == 0 {
		h.mu.RUnlock()
		return
	}
	channels := make([]chan StreamEvent, len(subs))
	copy(channels, subs)
	h.mu.RUnlock()

	for _, ch := range channels {
		select {
		case ch <- evt:
		default:
			// Non-blocking drop if client is too slow to avoid blocking other subscribers
		}
	}
}

// listenRedisChannel subscribes to Redis channel events for reviewID and dispatches to local listeners.
func (h *StreamHub) listenRedisChannel(ctx context.Context, reviewID uuid.UUID) {
	channelName := fmt.Sprintf("reviews:%s:events", reviewID.String())
	pubsub := h.redisClient.Subscribe(ctx, channelName)
	if pubsub == nil {
		return
	}
	defer pubsub.Close()

	ch := pubsub.Channel()
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-ch:
			if !ok {
				return
			}
			var rmsg streamRedisMessage
			if err := json.Unmarshal([]byte(msg.Payload), &rmsg); err != nil {
				continue
			}
			evt := StreamEvent{
				Event: rmsg.EventType,
				Stage: rmsg.Stage,
				Data:  rmsg.Data,
				Time:  rmsg.Time,
			}
			h.broadcastLocal(reviewID, evt)
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
	if _, err := fmt.Fprintf(w, "event: init\ndata: %s\n\n", string(initData)); err != nil {
		return
	}
	flusher.Flush()

	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	notify := r.Context().Done()
	for {
		select {
		case <-notify:
			return
		case <-ticker.C:
			if _, err := fmt.Fprint(w, ": keepalive\n\n"); err != nil {
				return
			}
			flusher.Flush()
		case evt, ok := <-ch:
			if !ok {
				return
			}
			bytes, err := json.Marshal(evt)
			if err != nil {
				continue
			}
			if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", evt.Event, string(bytes)); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}
