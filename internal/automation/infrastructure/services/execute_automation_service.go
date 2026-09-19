package services

import (
	"context"
	"fmt"
	"sync"

	"github.com/scandrix/backend/internal/automation/domain"
)

// AutomationRegistry maintains thread-safe registration of automation strategies/factories.
type AutomationRegistry struct {
	mu         sync.RWMutex
	strategies map[domain.AutomationType]domain.AutomationFactory
}

// NewAutomationRegistry creates an initialized registry pre-populated with optional strategies.
func NewAutomationRegistry(strategies ...domain.AutomationFactory) *AutomationRegistry {
	r := &AutomationRegistry{
		strategies: make(map[domain.AutomationType]domain.AutomationFactory),
	}
	for _, s := range strategies {
		if s != nil {
			r.Register(s)
		}
	}
	return r
}

// Register adds or updates an automation factory strategy in the registry.
func (r *AutomationRegistry) Register(strategy domain.AutomationFactory) {
	if strategy == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.strategies[strategy.GetAutomationType()] = strategy
}

// GetStrategy returns the registered automation strategy for a given automation type.
func (r *AutomationRegistry) GetStrategy(name domain.AutomationType) (domain.AutomationFactory, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	strategy, exists := r.strategies[name]
	if !exists {
		return nil, fmt.Errorf("unsupported automation name: %s", name)
	}
	return strategy, nil
}

// DefaultExecuteAutomationService dispatches operations across registered automation strategies.
type DefaultExecuteAutomationService struct {
	registry *AutomationRegistry
}

// NewDefaultExecuteAutomationService creates an execute automation service backed by a registry.
func NewDefaultExecuteAutomationService(registry *AutomationRegistry) *DefaultExecuteAutomationService {
	return &DefaultExecuteAutomationService{
		registry: registry,
	}
}

// ExecuteStrategy executes the requested automation strategy with payload.
func (s *DefaultExecuteAutomationService) ExecuteStrategy(
	ctx context.Context,
	automationType domain.AutomationType,
	data any,
) (any, error) {
	strategy, err := s.registry.GetStrategy(automationType)
	if err != nil {
		return nil, err
	}
	return strategy.Run(ctx, data)
}

// SetupStrategy executes setup lifecycle hooks on the requested automation strategy.
func (s *DefaultExecuteAutomationService) SetupStrategy(
	ctx context.Context,
	automationType domain.AutomationType,
	data any,
) error {
	strategy, err := s.registry.GetStrategy(automationType)
	if err != nil {
		return err
	}
	return strategy.Setup(ctx, data)
}

// StopStrategy terminates an active automation strategy with payload.
func (s *DefaultExecuteAutomationService) StopStrategy(
	ctx context.Context,
	automationType domain.AutomationType,
	data any,
) error {
	strategy, err := s.registry.GetStrategy(automationType)
	if err != nil {
		return err
	}
	return strategy.Stop(ctx, data)
}

// GetAutomationMethods retrieves the underlying automation strategy factory for inspection.
func (s *DefaultExecuteAutomationService) GetAutomationMethods(
	automationType domain.AutomationType,
) (domain.AutomationFactory, error) {
	return s.registry.GetStrategy(automationType)
}
