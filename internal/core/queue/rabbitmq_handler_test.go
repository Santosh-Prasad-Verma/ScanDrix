package queue_test

import (
	"context"
	"errors"
	"testing"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/scandrix/backend/internal/core/queue"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockChannelPublisher struct {
	published []struct {
		Exchange string
		Key      string
		Msg      amqp.Publishing
	}
}

func (m *mockChannelPublisher) PublishWithContext(ctx context.Context, exchange, key string, mandatory, immediate bool, msg amqp.Publishing) error {
	m.published = append(m.published, struct {
		Exchange string
		Key      string
		Msg      amqp.Publishing
	}{Exchange: exchange, Key: key, Msg: msg})
	return nil
}

func TestRabbitMQErrorHandlerFirstFailureRetries(t *testing.T) {
	mockPub := &mockChannelPublisher{}
	handler := queue.NewRabbitMQErrorHandler(mockPub, 2, 1000)

	delivery := amqp.Delivery{
		Exchange:   "workflow.exchange",
		RoutingKey: "workflow.jobs.code_review.CODE_REVIEW",
		Body:       []byte(`{"jobId":"123"}`),
		Headers:    amqp.Table{},
	}

	err := handler.Handle(context.Background(), delivery, errors.New("network transient error"), "")
	require.NoError(t, err)

	require.Len(t, mockPub.published, 1)
	assert.Equal(t, "workflow.exchange.delayed", mockPub.published[0].Exchange)
	assert.Equal(t, "workflow.jobs.code_review.CODE_REVIEW", mockPub.published[0].Key)
	assert.Equal(t, int32(1), mockPub.published[0].Msg.Headers["x-retry-count"])
	assert.Equal(t, int64(1000), mockPub.published[0].Msg.Headers["x-delay"])
}

func TestRabbitMQErrorHandlerRateLimitResetBuffer(t *testing.T) {
	mockPub := &mockChannelPublisher{}
	handler := queue.NewRabbitMQErrorHandler(mockPub, 2, 1000)

	resetAt := time.Now().Add(10 * time.Minute)
	rateLimitErr := &queue.RateLimitError{
		Message: "GitHub Primary Rate Limit Exceeded",
		ResetAt: resetAt,
	}

	delivery := amqp.Delivery{
		Exchange:   "workflow.exchange",
		RoutingKey: "workflow.jobs.code_review.CODE_REVIEW",
		Body:       []byte(`{"jobId":"123"}`),
		Headers:    amqp.Table{},
	}

	err := handler.Handle(context.Background(), delivery, rateLimitErr, "")
	require.NoError(t, err)

	require.Len(t, mockPub.published, 1)
	assert.Equal(t, "workflow.exchange.delayed", mockPub.published[0].Exchange)
	delayMs, ok := mockPub.published[0].Msg.Headers["x-delay"].(int64)
	assert.True(t, ok)
	// Delay should be ~ 10 min + 5 min safety buffer = ~15 minutes (900,000 ms)
	assert.True(t, delayMs >= 890000 && delayMs <= 910000)
}

func TestRabbitMQErrorHandlerMaxRetriesExceededDLQ(t *testing.T) {
	mockPub := &mockChannelPublisher{}
	handler := queue.NewRabbitMQErrorHandler(mockPub, 2, 1000)

	delivery := amqp.Delivery{
		Exchange:   "workflow.exchange",
		RoutingKey: "workflow.jobs.code_review.CODE_REVIEW",
		Body:       []byte(`{"jobId":"123"}`),
		Headers: amqp.Table{
			"x-retry-count": int32(2), // already retried 2 times
		},
	}

	err := handler.Handle(context.Background(), delivery, errors.New("permanent failure"), "custom.dlq.key")
	require.NoError(t, err)

	require.Len(t, mockPub.published, 1)
	assert.Equal(t, "workflow.exchange.dlx", mockPub.published[0].Exchange)
	assert.Equal(t, "custom.dlq.key", mockPub.published[0].Key)
	assert.Equal(t, "permanent failure", mockPub.published[0].Msg.Headers["x-original-error"])
}

func TestRabbitMQTopologyDefinitions(t *testing.T) {
	exchanges := queue.RabbitMQTopologyConfig
	assert.True(t, len(exchanges) >= 7)

	names := make(map[string]bool)
	for _, e := range exchanges {
		names[e.Name] = true
	}

	assert.True(t, names["workflow.exchange"])
	assert.True(t, names["workflow.exchange.delayed"])
	assert.True(t, names["workflow.exchange.dlx"])
	assert.True(t, names["workflow.events"])
	assert.True(t, names["notification.exchange"])
}
