package try_test

import (
	"context"
	"testing"
	"time"

	"github.com/scandrix/backend/internal/rules"
	"github.com/scandrix/backend/internal/try"
)

func TestParseGitHubURL(t *testing.T) {
	cases := []struct {
		url       string
		wantOwner string
		wantRepo  string
		wantPR    int
		wantErr   bool
	}{
		{"https://github.com/facebook/react/pull/24589", "facebook", "react", 24589, false},
		{"http://github.com/golang/go/pull/42/", "golang", "go", 42, false},
		{"https://github.com/invalid-format", "", "", 0, true},
		{"not-a-url", "", "", 0, true},
	}

	for _, c := range cases {
		owner, repo, pr, err := try.ParseGitHubURL(c.url)
		if (err != nil) != c.wantErr {
			t.Errorf("url %q: got err %v, wantErr %v", c.url, err, c.wantErr)
			continue
		}
		if !c.wantErr {
			if owner != c.wantOwner || repo != c.wantRepo || pr != c.wantPR {
				t.Errorf("url %q: got (%s, %s, %d), want (%s, %s, %d)", c.url, owner, repo, pr, c.wantOwner, c.wantRepo, c.wantPR)
			}
		}
	}
}

func TestRateLimiter(t *testing.T) {
	limiter := try.NewRateLimiter(2, 100*time.Millisecond)

	// 1st request -> Allowed
	r1 := limiter.CheckAndRecord("device-1")
	if !r1.Allowed || r1.Remaining != 1 {
		t.Fatalf("expected 1st request allowed with 1 remaining, got %+v", r1)
	}

	// 2nd request -> Allowed
	r2 := limiter.CheckAndRecord("device-1")
	if !r2.Allowed || r2.Remaining != 0 {
		t.Fatalf("expected 2nd request allowed with 0 remaining, got %+v", r2)
	}

	// 3rd request -> Blocked
	r3 := limiter.CheckAndRecord("device-1")
	if r3.Allowed {
		t.Fatalf("expected 3rd request blocked, got %+v", r3)
	}

	// Different device -> Allowed
	rOther := limiter.CheckAndRecord("device-2")
	if !rOther.Allowed {
		t.Fatalf("expected device-2 allowed, got %+v", rOther)
	}

	// Wait for window to expire
	time.Sleep(120 * time.Millisecond)
	rAfter := limiter.CheckAndRecord("device-1")
	if !rAfter.Allowed {
		t.Fatalf("expected device-1 allowed after window expiration, got %+v", rAfter)
	}
}

func TestFeaturedRegistry(t *testing.T) {
	reg := try.NewFeaturedRegistry()
	summaries := reg.ListSummaries()
	if len(summaries) < 2 {
		t.Fatalf("expected at least 2 default featured summaries, got %d", len(summaries))
	}

	detail, ok := reg.GetDetail("react-fizz-ssr-abort")
	if !ok {
		t.Fatalf("expected 'react-fizz-ssr-abort' detail to exist")
	}
	if len(detail.Result.Issues) == 0 {
		t.Errorf("expected featured detail issues to be populated")
	}
}

func TestPublicServiceEnqueueAndPoll(t *testing.T) {
	evaluator, _ := rules.NewEvaluator(rules.DefaultCatalog())
	limiter := try.NewRateLimiter(5, time.Hour)
	featured := try.NewFeaturedRegistry()
	svc := try.NewPublicReviewService(limiter, featured, evaluator)

	_, err := svc.EnqueueReview(context.Background(), try.EnqueueRequest{
		PRURL:       "invalid-url",
		Fingerprint: "fp-test",
	})
	if err == nil {
		t.Fatalf("expected error on invalid URL")
	}
}
