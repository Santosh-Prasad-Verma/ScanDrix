package sse

import (
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// StreamEvent represents a real-time SSE event payload.
type StreamEvent struct {
	Event     string `json:"event"`
	AuditID   string `json:"audit_id"`
	Stage     string `json:"stage,omitempty"`
	Percent   int    `json:"percent,omitempty"`
	Data      any    `json:"data,omitempty"`
	Timestamp string `json:"timestamp"`
}

// Broker manages real-time SSE subscribers and event fan-out.
type Broker struct {
	mu          sync.RWMutex
	subscribers map[string]map[chan StreamEvent]struct{}
}

// NewBroker creates a new SSE broker instance.
func NewBroker() *Broker {
	return &Broker{
		subscribers: make(map[string]map[chan StreamEvent]struct{}),
	}
}

// Subscribe opens an event channel for a specific audit scan.
func (b *Broker) Subscribe(auditID string) (chan StreamEvent, func()) {
	b.mu.Lock()
	defer b.mu.Unlock()

	ch := make(chan StreamEvent, 100)
	if _, ok := b.subscribers[auditID]; !ok {
		b.subscribers[auditID] = make(map[chan StreamEvent]struct{})
	}
	b.subscribers[auditID][ch] = struct{}{}

	unsubscribe := func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		if subs, ok := b.subscribers[auditID]; ok {
			delete(subs, ch)
			close(ch)
			if len(subs) == 0 {
				delete(b.subscribers, auditID)
			}
		}
	}

	return ch, unsubscribe
}

// Publish broadcasts an event to all subscribers listening to an audit ID.
func (b *Broker) Publish(auditID string, event StreamEvent) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if event.Timestamp == "" {
		event.Timestamp = time.Now().UTC().Format(time.RFC3339)
	}

	if subs, ok := b.subscribers[auditID]; ok {
		for ch := range subs {
			select {
			case ch <- event:
			default:
				// Dropping event if slow subscriber channel is full
			}
		}
	}
}

// HandleStream handles the HTTP SSE streaming connection.
func (b *Broker) HandleStream(c *gin.Context, auditID string) {
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("Transfer-Encoding", "chunked")

	eventChan, unsubscribe := b.Subscribe(auditID)
	defer unsubscribe()

	c.Stream(func(w io.Writer) bool {
		select {
		case <-c.Request.Context().Done():
			return false
		case evt, ok := <-eventChan:
			if !ok {
				return false
			}
			dataJSON, err := json.Marshal(evt)
			if err != nil {
				return true
			}
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", evt.Event, string(dataJSON))
			c.Writer.Flush()
			return evt.Event != "audit_completed" && evt.Event != "audit_failed"
		}
	})
}
