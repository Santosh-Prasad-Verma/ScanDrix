// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Rule System
// File: rule_like_entity.go
// ═══════════════════════════════════════════════════════════════

package entities

import "time"

// RuleFeedbackType indicates whether user sentiment was positive (like) or negative (dislike).
type RuleFeedbackType string

const (
	RuleFeedbackTypePositive RuleFeedbackType = "positive"
	RuleFeedbackTypeNegative RuleFeedbackType = "negative"
)

// IRuleLike defines the data structure for rule feedback persistence.
type IRuleLike struct {
	ID        string           `json:"id,omitempty"`
	RuleID    string           `json:"ruleId"`
	UserID    string           `json:"userId,omitempty"`
	Feedback  RuleFeedbackType `json:"feedback"`
	CreatedAt *time.Time       `json:"createdAt,omitempty"`
	UpdatedAt *time.Time       `json:"updatedAt,omitempty"`
}

// RuleLikeEntity represents feedback registered against a rule.
type RuleLikeEntity struct {
	id        string
	ruleID    string
	userID    string
	feedback  RuleFeedbackType
	createdAt *time.Time
	updatedAt *time.Time
}

// NewRuleLikeEntity constructs a new entity from raw data.
func NewRuleLikeEntity(props IRuleLike) *RuleLikeEntity {
	return &RuleLikeEntity{
		id:        props.ID,
		ruleID:    props.RuleID,
		userID:    props.UserID,
		feedback:  props.Feedback,
		createdAt: props.CreatedAt,
		updatedAt: props.UpdatedAt,
	}
}

// ID returns the feedback identifier.
func (e *RuleLikeEntity) ID() string {
	return e.id
}

// RuleID returns the target rule uuid.
func (e *RuleLikeEntity) RuleID() string {
	return e.ruleID
}

// UserID returns the voter identifier.
func (e *RuleLikeEntity) UserID() string {
	return e.userID
}

// Feedback returns positive or negative sentiment.
func (e *RuleLikeEntity) Feedback() RuleFeedbackType {
	return e.feedback
}

// CreatedAt returns creation timestamp.
func (e *RuleLikeEntity) CreatedAt() *time.Time {
	return e.createdAt
}

// UpdatedAt returns update timestamp.
func (e *RuleLikeEntity) UpdatedAt() *time.Time {
	return e.updatedAt
}

// ToObject converts entity to plain struct.
func (e *RuleLikeEntity) ToObject() IRuleLike {
	return IRuleLike{
		ID:        e.id,
		RuleID:    e.ruleID,
		UserID:    e.userID,
		Feedback:  e.feedback,
		CreatedAt: e.createdAt,
		UpdatedAt: e.updatedAt,
	}
}
