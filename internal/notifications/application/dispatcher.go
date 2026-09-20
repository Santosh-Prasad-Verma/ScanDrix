package application

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/notifications/domain/catalog"
	"github.com/scandrix/backend/internal/notifications/domain/contracts"
	"github.com/scandrix/backend/internal/notifications/domain/entities"
	"github.com/scandrix/backend/internal/notifications/domain/enums"
	"github.com/scandrix/backend/internal/notifications/domain/recipient"
)

// NotificationMessage encapsulates an inbound message ready for fanout and dispatch.
type NotificationMessage struct {
	Event          catalog.Event          `json:"event"`
	Payload        map[string]interface{} `json:"payload"`
	OrganizationID string                 `json:"organizationId"`
	Recipients     []recipient.Recipient  `json:"recipients"`
	CorrelationID  string                 `json:"correlationId"`
}

// ResolvedRecipient contains resolved identity coordinates for a target recipient.
type ResolvedRecipient struct {
	UserID   string
	Email    string
	Role     string
	Channels []enums.Channel
	Directed bool
}

// InAppTemplateResult represents resolved title and body for notification drawer display.
type InAppTemplateResult struct {
	Title  string
	Body   string
	CtaURL string
}

// InAppTemplateResolver provides dynamic in-app title and body resolution.
type InAppTemplateResolver interface {
	ResolveInAppTemplate(event catalog.Event, payload map[string]interface{}) InAppTemplateResult
}

// NotificationDispatcherService coordinates recipient resolution, channel matrix routing, and adapter dispatch.
type NotificationDispatcherService struct {
	adapters         map[enums.Channel]contracts.ChannelAdapter
	deliveryRepo     contracts.NotificationDeliveryRepository
	routingRuleRepo  contracts.RoutingRuleRepository
	userLookup       UserLookupService
	sseService       *NotificationSseService
	templateResolver InAppTemplateResolver
}

// NewNotificationDispatcherService constructs a fully wired notification dispatcher.
func NewNotificationDispatcherService(
	adapters []contracts.ChannelAdapter,
	deliveryRepo contracts.NotificationDeliveryRepository,
	routingRuleRepo contracts.RoutingRuleRepository,
	userLookup UserLookupService,
	sseService *NotificationSseService,
	templateResolver InAppTemplateResolver,
) *NotificationDispatcherService {
	adapterMap := make(map[enums.Channel]contracts.ChannelAdapter)
	for _, a := range adapters {
		if a != nil && a.Channel().IsActive() {
			adapterMap[a.Channel()] = a
		}
	}

	return &NotificationDispatcherService{
		adapters:         adapterMap,
		deliveryRepo:     deliveryRepo,
		routingRuleRepo:  routingRuleRepo,
		userLookup:       userLookup,
		sseService:       sseService,
		templateResolver: templateResolver,
	}
}

// Dispatch processes a notification envelope across all resolved recipients and channels.
func (d *NotificationDispatcherService) Dispatch(ctx context.Context, msg NotificationMessage) error {
	defaults, exists := catalog.EventDefaultsMap[msg.Event]
	if !exists {
		return fmt.Errorf("unknown notification event: %s", msg.Event)
	}

	orgUUID, err := uuid.Parse(msg.OrganizationID)
	if err != nil {
		return fmt.Errorf("invalid organization id: %w", err)
	}

	// 1. Preload organization routing rules once for O(1) in-memory lookup during fanout
	ruleByKey := make(map[string]*entities.RoutingRule)
	if d.routingRuleRepo != nil {
		rules, err := d.routingRuleRepo.FindByOrganization(ctx, orgUUID)
		if err == nil {
			for _, r := range rules {
				key := fmt.Sprintf("%s:%s", r.Event, r.Role)
				ruleByKey[key] = r
			}
		}
	}

	// 2. Resolve directed recipients and config-driven audience members
	directed, err := d.resolveRecipients(ctx, msg.Recipients, msg.OrganizationID)
	if err != nil {
		return err
	}
	for i := range directed {
		directed[i].Directed = true
	}

	var audience []ResolvedRecipient
	if len(defaults.DefaultRoles) > 0 && d.userLookup != nil {
		members, err := d.userLookup.FindAllOrgMembers(ctx, msg.OrganizationID)
		if err == nil {
			for _, m := range members {
				audience = append(audience, ResolvedRecipient{
					UserID: m.UserID,
					Email:  m.Email,
					Role:   m.Role,
				})
			}
		}
	}

	recipients := d.dedupeRecipients(append(directed, audience...))

	// 3. Dispatch to each resolved recipient
	for _, rec := range recipients {
		d.dispatchToRecipient(ctx, rec, msg.Event, defaults, msg.Payload, orgUUID, msg.CorrelationID, ruleByKey)
	}

	return nil
}

func (d *NotificationDispatcherService) dispatchToRecipient(
	ctx context.Context,
	rec ResolvedRecipient,
	event catalog.Event,
	defaults catalog.EventDefaults,
	payload map[string]interface{},
	orgID uuid.UUID,
	correlationID string,
	ruleByKey map[string]*entities.RoutingRule,
) {
	// Resolve active channels
	var enabledChannels []enums.Channel
	if rec.Directed {
		for _, ch := range defaults.DefaultChannels {
			if ch.IsActive() {
				enabledChannels = append(enabledChannels, ch)
			}
		}
	} else {
		enabledChannels = d.resolveEnabledChannels(ruleByKey, string(event), rec.Role, defaults)
	}

	// Apply recipient-specific channel narrowing if declared
	if len(rec.Channels) > 0 {
		allowed := make(map[enums.Channel]bool)
		for _, ch := range rec.Channels {
			allowed[ch] = true
		}
		var filtered []enums.Channel
		for _, ch := range enabledChannels {
			if allowed[ch] {
				filtered = append(filtered, ch)
			}
		}
		enabledChannels = filtered
	}

	title, body, ctaURL := d.resolveTemplate(event, payload, defaults)

	for _, ch := range enabledChannels {
		adapter, hasAdapter := d.adapters[ch]
		if !hasAdapter {
			continue
		}

		// In-app requires a registered user UUID
		if ch == enums.ChannelInApp && rec.UserID == "" {
			continue
		}

		var userUUID *uuid.UUID
		if rec.UserID != "" {
			if parsed, err := uuid.Parse(rec.UserID); err == nil {
				userUUID = &parsed
			}
		}

		del := entities.NewDelivery(
			orgID,
			string(event),
			defaults.Criticality,
			ch,
			title,
			body,
			defaults.Category,
			correlationID,
		)
		del.RecipientUserID = userUUID
		if rec.Email != "" {
			emailCopy := rec.Email
			del.RecipientEmail = &emailCopy
		}
		if rec.Role != "" {
			roleCopy := rec.Role
			del.RecipientRole = &roleCopy
		}
		if ctaURL != "" {
			ctaCopy := ctaURL
			del.CtaURL = &ctaCopy
		}
		del.Metadata = payload

		if d.deliveryRepo != nil {
			if err := d.deliveryRepo.Create(ctx, del); err != nil {
				continue
			}
		}

		deliveryCtx := contracts.DeliveryContext{
			DeliveryID:     del.UUID.String(),
			UserID:         rec.UserID,
			UserEmail:      rec.Email,
			UserRole:       rec.Role,
			OrganizationID: orgID.String(),
			Event:          event,
			Criticality:    defaults.Criticality,
			Title:          title,
			Body:           body,
			CtaURL:         ctaURL,
			Category:       defaults.Category,
			Metadata:       payload,
			CorrelationID:  correlationID,
		}

		if err := adapter.Deliver(ctx, deliveryCtx); err != nil {
			d.handleDeliveryFailure(ctx, del, err, 1, defaults.Criticality)
			continue
		}

		now := time.Now().UTC()
		del.DeliveryStatus = enums.StatusDelivered
		del.DeliveredAt = &now
		if d.deliveryRepo != nil {
			_ = d.deliveryRepo.UpdateStatus(ctx, del.UUID, enums.StatusDelivered, nil)
		}

		if ch == enums.ChannelInApp && d.sseService != nil && rec.UserID != "" {
			d.sseService.PushEvent(rec.UserID, "notification", map[string]interface{}{
				"id":          del.UUID.String(),
				"title":       title,
				"category":    defaults.Category,
				"criticality": defaults.Criticality,
			})
		}
	}
}

// Redeliver re-attempts delivery for a claimed delivery row.
func (d *NotificationDispatcherService) Redeliver(ctx context.Context, del *entities.NotificationDelivery, attemptsSoFar int) error {
	adapter, hasAdapter := d.adapters[del.Channel]
	if !hasAdapter {
		errStr := fmt.Sprintf("No adapter registered for channel: %s", del.Channel)
		if d.deliveryRepo != nil {
			_ = d.deliveryRepo.UpdateStatus(ctx, del.UUID, enums.StatusFailed, &errStr)
		}
		return nil
	}

	userID := ""
	if del.RecipientUserID != nil {
		userID = del.RecipientUserID.String()
	}
	email := ""
	if del.RecipientEmail != nil {
		email = *del.RecipientEmail
	}
	role := "contributor"
	if del.RecipientRole != nil {
		role = *del.RecipientRole
	}
	cta := ""
	if del.CtaURL != nil {
		cta = *del.CtaURL
	}

	deliveryCtx := contracts.DeliveryContext{
		DeliveryID:     del.UUID.String(),
		UserID:         userID,
		UserEmail:      email,
		UserRole:       role,
		OrganizationID: del.OrganizationID.String(),
		Event:          catalog.Event(del.Event),
		Criticality:    del.Criticality,
		Title:          del.Title,
		Body:           del.Body,
		CtaURL:         cta,
		Category:       del.Category,
		Metadata:       del.Metadata,
		CorrelationID:  del.CorrelationID,
	}

	if err := adapter.Deliver(ctx, deliveryCtx); err != nil {
		d.handleDeliveryFailure(ctx, del, err, attemptsSoFar, del.Criticality)
		return err
	}

	now := time.Now().UTC()
	del.DeliveryStatus = enums.StatusDelivered
	del.DeliveredAt = &now
	if d.deliveryRepo != nil {
		_ = d.deliveryRepo.UpdateStatus(ctx, del.UUID, enums.StatusDelivered, nil)
	}

	if del.Channel == enums.ChannelInApp && d.sseService != nil && userID != "" {
		d.sseService.PushEvent(userID, "notification", map[string]interface{}{
			"id":          del.UUID.String(),
			"title":       del.Title,
			"category":    del.Category,
			"criticality": del.Criticality,
		})
	}

	return nil
}

func (d *NotificationDispatcherService) handleDeliveryFailure(
	ctx context.Context,
	del *entities.NotificationDelivery,
	err error,
	attemptsSoFar int,
	criticality enums.Criticality,
) {
	decision := DecideRetry(criticality, attemptsSoFar, time.Now().UTC())
	errMsg := err.Error()

	if decision.ShouldRetry && d.deliveryRepo != nil {
		_ = d.deliveryRepo.ScheduleRetry(ctx, del.UUID, decision.NextAttemptAt, errMsg)
		return
	}

	if d.deliveryRepo != nil {
		_ = d.deliveryRepo.UpdateStatus(ctx, del.UUID, enums.StatusFailed, &errMsg)
	}
}

func (d *NotificationDispatcherService) resolveEnabledChannels(
	ruleByKey map[string]*entities.RoutingRule,
	event string,
	role string,
	defaults catalog.EventDefaults,
) []enums.Channel {
	if defaults.Criticality == enums.CriticalitySystem {
		var active []enums.Channel
		for _, ch := range defaults.DefaultChannels {
			if ch.IsActive() {
				active = append(active, ch)
			}
		}
		return active
	}

	// 1. Specific role rule wins
	if specific, ok := ruleByKey[fmt.Sprintf("%s:%s", event, role)]; ok {
		return d.extractActiveChannels(specific.Channels)
	}

	// 2. Wildcard role rule baseline
	if wildcard, ok := ruleByKey[fmt.Sprintf("%s:%s", event, catalog.RoleWildcard)]; ok {
		return d.extractActiveChannels(wildcard.Channels)
	}

	// 3. Fall back to catalog defaults for default roles
	isDefaultRole := len(defaults.DefaultRoles) == 0
	for _, dr := range defaults.DefaultRoles {
		if dr == role {
			isDefaultRole = true
			break
		}
	}
	if isDefaultRole {
		var active []enums.Channel
		for _, ch := range defaults.DefaultChannels {
			if ch.IsActive() {
				active = append(active, ch)
			}
		}
		return active
	}

	return nil
}

func (d *NotificationDispatcherService) extractActiveChannels(channels map[string]bool) []enums.Channel {
	var active []enums.Channel
	for chStr, enabled := range channels {
		ch := enums.Channel(chStr)
		if enabled && ch.IsActive() {
			active = append(active, ch)
		}
	}
	return active
}

func (d *NotificationDispatcherService) resolveTemplate(
	event catalog.Event,
	payload map[string]interface{},
	defaults catalog.EventDefaults,
) (string, string, string) {
	if d.templateResolver != nil {
		res := d.templateResolver.ResolveInAppTemplate(event, payload)
		if res.Title != "" {
			return res.Title, res.Body, res.CtaURL
		}
	}

	cta := ""
	if raw, ok := payload["ctaUrl"].(string); ok {
		cta = raw
	}
	return defaults.Label, "", cta
}

func (d *NotificationDispatcherService) resolveRecipients(
	ctx context.Context,
	recipients []recipient.Recipient,
	orgID string,
) ([]ResolvedRecipient, error) {
	var resolved []ResolvedRecipient
	for _, r := range recipients {
		switch r.Kind {
		case recipient.KindUser:
			if d.userLookup != nil {
				user, err := d.userLookup.FindUserByID(ctx, r.UserID)
				if err == nil && user != nil {
					resolved = append(resolved, ResolvedRecipient{
						UserID:   user.UserID,
						Email:    user.Email,
						Role:     user.Role,
						Channels: r.Channels,
					})
					continue
				}
			}
			resolved = append(resolved, ResolvedRecipient{
				UserID:   r.UserID,
				Role:     "contributor",
				Channels: r.Channels,
			})

		case recipient.KindEmail:
			if d.userLookup != nil {
				user, err := d.userLookup.FindUserByEmail(ctx, r.Email, orgID)
				if err == nil && user != nil {
					resolved = append(resolved, ResolvedRecipient{
						UserID:   user.UserID,
						Email:    user.Email,
						Role:     user.Role,
						Channels: r.Channels,
					})
					continue
				}
			}
			resolved = append(resolved, ResolvedRecipient{
				Email:    r.Email,
				Role:     "contributor",
				Channels: r.Channels,
			})

		case recipient.KindRole:
			if d.userLookup != nil {
				users, err := d.userLookup.FindUsersByRole(ctx, orgID, r.Role)
				if err == nil {
					for _, u := range users {
						resolved = append(resolved, ResolvedRecipient{
							UserID:   u.UserID,
							Email:    u.Email,
							Role:     u.Role,
							Channels: r.Channels,
						})
					}
				}
			}

		case recipient.KindAllOrgMembers:
			if d.userLookup != nil {
				users, err := d.userLookup.FindAllOrgMembers(ctx, orgID)
				if err == nil {
					for _, u := range users {
						resolved = append(resolved, ResolvedRecipient{
							UserID:   u.UserID,
							Email:    u.Email,
							Role:     u.Role,
							Channels: r.Channels,
						})
					}
				}
			}
		}
	}
	return resolved, nil
}

func (d *NotificationDispatcherService) dedupeRecipients(recipients []ResolvedRecipient) []ResolvedRecipient {
	seen := make(map[string]bool)
	var deduped []ResolvedRecipient
	for _, r := range recipients {
		key := r.UserID
		if key == "" {
			key = "EMAIL:" + r.Email
		}
		if !seen[key] {
			seen[key] = true
			deduped = append(deduped, r)
		}
	}
	return deduped
}
