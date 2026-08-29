package notifications

import (
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

// NotificationRateLimiter prevents alert fatigue by throttling notifications for the same resource.
type NotificationRateLimiter struct {
	mu       sync.Mutex
	cooldown time.Duration
	history  map[string]time.Time // key -> lastSentTime
}

func NewNotificationRateLimiter(cooldown time.Duration) *NotificationRateLimiter {
	if cooldown <= 0 {
		cooldown = 15 * time.Minute
	}
	return &NotificationRateLimiter{
		cooldown: cooldown,
		history:  make(map[string]time.Time),
	}
}

// Allow checks whether an event for a workspace and resource is allowed to notify or should be suppressed.
func (l *NotificationRateLimiter) Allow(workspaceID uuid.UUID, resourceKey, eventType string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	key := fmt.Sprintf("%s:%s:%s", workspaceID, resourceKey, eventType)
	lastSent, exists := l.history[key]
	now := time.Now().UTC()

	if exists && now.Sub(lastSent) < l.cooldown {
		return false // Throttled
	}

	l.history[key] = now
	return true
}

// Reset clears throttling history for a specific key.
func (l *NotificationRateLimiter) Reset(workspaceID uuid.UUID, resourceKey, eventType string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	key := fmt.Sprintf("%s:%s:%s", workspaceID, resourceKey, eventType)
	delete(l.history, key)
}
