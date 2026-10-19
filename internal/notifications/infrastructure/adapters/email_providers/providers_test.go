package email_providers

import (
	"context"
	"errors"
	"testing"
)

type failingEmailProvider struct {
	err error
}

func (f *failingEmailProvider) Send(ctx context.Context, msg EmailMessage) error {
	return f.err
}

func TestDualRailEmailProvider_PrimarySuccess(t *testing.T) {
	primary := NewMemoryEmailProvider()
	fallback := NewMemoryEmailProvider()

	dual := NewDualRailEmailProvider(primary, fallback)

	msg := EmailMessage{
		From:    "alert@scandrix.dev",
		To:      "security@acme.io",
		Subject: "Critical Alert",
		HTML:    "<p>Alert</p>",
	}

	err := dual.Send(context.Background(), msg)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	if len(primary.Sent) != 1 {
		t.Errorf("expected 1 message in primary, got %d", len(primary.Sent))
	}
	if len(fallback.Sent) != 0 {
		t.Errorf("expected 0 messages in fallback, got %d", len(fallback.Sent))
	}
}

func TestDualRailEmailProvider_FallbackOnPrimaryFailure(t *testing.T) {
	primary := &failingEmailProvider{err: errors.New("resend 429 quota exhausted")}
	fallback := NewMemoryEmailProvider()

	dual := NewDualRailEmailProvider(primary, fallback)

	msg := EmailMessage{
		From:    "alert@scandrix.dev",
		To:      "security@acme.io",
		Subject: "Critical Alert",
		HTML:    "<p>Alert</p>",
	}

	err := dual.Send(context.Background(), msg)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	if len(fallback.Sent) != 1 {
		t.Errorf("expected fallback to receive message, got %d", len(fallback.Sent))
	}
}
