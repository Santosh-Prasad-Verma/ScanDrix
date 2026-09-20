package application

import (
	"encoding/json"
	"fmt"
	"sync"
)

// SSEMessage represents a formatted server-sent event packet.
type SSEMessage struct {
	Event string      `json:"event"`
	Data  interface{} `json:"data"`
}

// NotificationSseService manages real-time SSE stream listeners for connected web UI users.
type NotificationSseService struct {
	mu          sync.RWMutex
	connections map[string]map[chan []byte]struct{}
}

// NewNotificationSseService initializes an in-memory SSE broker for notifications.
func NewNotificationSseService() *NotificationSseService {
	return &NotificationSseService{
		connections: make(map[string]map[chan []byte]struct{}),
	}
}

// AddConnection registers a client SSE stream channel for a specific user ID.
func (s *NotificationSseService) AddConnection(userID string, ch chan []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()

	userConns, exists := s.connections[userID]
	if !exists {
		userConns = make(map[chan []byte]struct{})
		s.connections[userID] = userConns
	}
	userConns[ch] = struct{}{}
}

// RemoveConnection unregisters a client SSE stream channel.
func (s *NotificationSseService) RemoveConnection(userID string, ch chan []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()

	userConns, exists := s.connections[userID]
	if !exists {
		return
	}
	delete(userConns, ch)
	if len(userConns) == 0 {
		delete(s.connections, userID)
	}
}

// PushEvent formats and delivers an SSE message to all active tabs/devices of a user.
func (s *NotificationSseService) PushEvent(userID string, eventType string, data interface{}) {
	s.mu.RLock()
	userConns, exists := s.connections[userID]
	if !exists || len(userConns) == 0 {
		s.mu.RUnlock()
		return
	}

	payloadJSON, err := json.Marshal(data)
	if err != nil {
		s.mu.RUnlock()
		return
	}

	raw := []byte(fmt.Sprintf("event: %s\ndata: %s\n\n", eventType, string(payloadJSON)))

	for ch := range userConns {
		select {
		case ch <- raw:
		default:
			// Non-blocking drop if client buffer is congested
		}
	}
	s.mu.RUnlock()
}

// Broadcast sends an SSE message to all connected users across the platform.
func (s *NotificationSseService) Broadcast(eventType string, data interface{}) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	payloadJSON, err := json.Marshal(data)
	if err != nil {
		return
	}
	raw := []byte(fmt.Sprintf("event: %s\ndata: %s\n\n", eventType, string(payloadJSON)))

	for _, userConns := range s.connections {
		for ch := range userConns {
			select {
			case ch <- raw:
			default:
			}
		}
	}
}
