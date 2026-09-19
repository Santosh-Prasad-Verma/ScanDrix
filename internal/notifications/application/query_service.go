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
)

// PaginatedNotifications wraps paginated in-app notifications for the web drawer.
type PaginatedNotifications struct {
	Data  []*contracts.UserNotificationWithDelivery `json:"data"`
	Total int                                       `json:"total"`
	Page  int                                       `json:"page"`
	Limit int                                       `json:"limit"`
}

// NotificationQueryService provides query and read-state operations for the user notification center.
type NotificationQueryService struct {
	userNotifRepo contracts.UserNotificationRepository
	deliveryRepo  contracts.NotificationDeliveryRepository
}

// NewNotificationQueryService creates a new notification query service.
func NewNotificationQueryService(
	userNotifRepo contracts.UserNotificationRepository,
	deliveryRepo contracts.NotificationDeliveryRepository,
) *NotificationQueryService {
	return &NotificationQueryService{
		userNotifRepo: userNotifRepo,
		deliveryRepo:  deliveryRepo,
	}
}

// List returns a paginated slice of in-app notifications for the target user.
func (s *NotificationQueryService) List(
	ctx context.Context,
	userID uuid.UUID,
	page int,
	limit int,
	unreadOnly bool,
) (*PaginatedNotifications, error) {
	if s.userNotifRepo == nil {
		return &PaginatedNotifications{Data: []*contracts.UserNotificationWithDelivery{}, Total: 0, Page: page, Limit: limit}, nil
	}
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}
	offset := (page - 1) * limit

	data, total, err := s.userNotifRepo.FindByUser(ctx, userID, limit, offset, unreadOnly)
	if err != nil {
		return nil, err
	}

	return &PaginatedNotifications{
		Data:  data,
		Total: total,
		Page:  page,
		Limit: limit,
	}, nil
}

// UnreadCount returns the number of unread in-app notifications for a user.
func (s *NotificationQueryService) UnreadCount(ctx context.Context, userID uuid.UUID) (int, error) {
	if s.userNotifRepo == nil {
		return 0, nil
	}
	return s.userNotifRepo.CountUnread(ctx, userID)
}

// MarkAsRead marks a single notification as read by the user.
func (s *NotificationQueryService) MarkAsRead(ctx context.Context, notificationID, userID uuid.UUID) error {
	if s.userNotifRepo == nil {
		return nil
	}
	return s.userNotifRepo.MarkAsRead(ctx, notificationID, userID)
}

// MarkAllAsRead marks all pending unread notifications as read for the user.
func (s *NotificationQueryService) MarkAllAsRead(ctx context.Context, userID uuid.UUID) (int, error) {
	if s.userNotifRepo == nil {
		return 0, nil
	}
	return s.userNotifRepo.MarkAllAsRead(ctx, userID)
}

// SeedDevNotifications populates realistic sample notifications in local/dev environments.
func (s *NotificationQueryService) SeedDevNotifications(ctx context.Context, userID, orgID uuid.UUID) (int, error) {
	if s.deliveryRepo == nil || s.userNotifRepo == nil {
		return 0, nil
	}

	correlationID := fmt.Sprintf("dev-seed-%d", time.Now().UnixNano())

	type sampleItem struct {
		event       catalog.Event
		criticality enums.Criticality
		category    string
		title       string
		body        string
		ctaURL      *string
		read        bool
	}

	cta1 := "/library/drixy-rules"
	cta2 := "/organization/sso"
	cta3 := "/cockpit"

	samples := []sampleItem{
		{
			event:       catalog.EventDrixyRulesGenerated,
			criticality: enums.CriticalityInformational,
			category:    "drixy_rules",
			title:       "Drixy rules generated",
			body:        "Drixy finished generating rules from your most recent code reviews. Check them out and approve the ones you want active.",
			ctaURL:      &cta1,
			read:        false,
		},
		{
			event:       catalog.EventTeamMemberInvited,
			criticality: enums.CriticalityTransactional,
			category:    "team",
			title:       "New teammate joined",
			body:        "Alex Rivera accepted your invite and joined the organization.",
			read:        false,
		},
		{
			event:       catalog.EventSSODomainVerification,
			criticality: enums.CriticalityCritical,
			category:    "sso",
			title:       "SSO domain verified",
			body:        "Your SSO domain scandrix.dev has been verified. SSO is now active for new team sign-ins.",
			ctaURL:      &cta2,
			read:        false,
		},
		{
			event:       catalog.EventOrgReport,
			criticality: enums.CriticalityInformational,
			category:    "cockpit",
			title:       "Your ScanDrix report is ready",
			body:        "PR cycle time dropped 12% this week. See what changed.",
			ctaURL:      &cta3,
			read:        true,
		},
	}

	created := 0
	now := time.Now().UTC()
	for _, item := range samples {
		del := entities.NewDelivery(
			orgID,
			string(item.event),
			item.criticality,
			enums.ChannelInApp,
			item.title,
			item.body,
			item.category,
			correlationID,
		)
		del.RecipientUserID = &userID
		del.CtaURL = item.ctaURL
		del.DeliveryStatus = enums.StatusDelivered
		del.DeliveredAt = &now

		if err := s.deliveryRepo.Create(ctx, del); err != nil {
			continue
		}

		un := entities.NewUserNotification(userID, del.UUID)
		if item.read {
			un.ReadAt = &now
		}

		if err := s.userNotifRepo.Create(ctx, un); err == nil {
			created++
		}
	}

	return created, nil
}
