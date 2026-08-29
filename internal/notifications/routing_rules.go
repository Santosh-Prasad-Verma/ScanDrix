package notifications

import (
	"sync"

	"github.com/google/uuid"
)

var criticalityRanks = map[AlertCriticality]int{
	CriticalityLow:      1,
	CriticalityMedium:   2,
	CriticalityHigh:     3,
	CriticalityCritical: 4,
}

// RoutingRuleService evaluates which channels should receive a specific notification event.
type RoutingRuleService struct {
	mu    sync.RWMutex
	rules map[uuid.UUID][]NotificationRoutingRule // workspaceID -> rules
}

func NewRoutingRuleService() *RoutingRuleService {
	return &RoutingRuleService{
		rules: make(map[uuid.UUID][]NotificationRoutingRule),
	}
}

// AddRule registers a notification routing rule for a workspace.
func (s *RoutingRuleService) AddRule(rule NotificationRoutingRule) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rules[rule.WorkspaceID] = append(s.rules[rule.WorkspaceID], rule)
}

// ResolveChannels determines the delivery channels and targets for an incoming event.
func (s *RoutingRuleService) ResolveChannels(event NotificationEvent) []ChannelType {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rules, exists := s.rules[event.WorkspaceID]
	if !exists || len(rules) == 0 {
		// Default routing: in-app for all, email/slack for high/critical
		if criticalityRanks[event.Criticality] >= criticalityRanks[CriticalityHigh] {
			return []ChannelType{ChannelInApp, ChannelEmail, ChannelSlack}
		}
		return []ChannelType{ChannelInApp}
	}

	channelSet := make(map[ChannelType]bool)
	eventRank := criticalityRanks[event.Criticality]

	for _, rule := range rules {
		if !rule.Enabled {
			continue
		}
		minRank := criticalityRanks[rule.MinCriticality]
		if eventRank >= minRank {
			for _, ch := range rule.Channels {
				channelSet[ch] = true
			}
		}
	}

	var channels []ChannelType
	for ch := range channelSet {
		channels = append(channels, ch)
	}

	return channels
}
