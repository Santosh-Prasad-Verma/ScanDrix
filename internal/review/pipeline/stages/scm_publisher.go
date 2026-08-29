package stages

import (
	"context"
	"time"

	"github.com/scandrix/backend/internal/review/pipeline"
)

// SCMPublisherStage manages throttled delivery of review comments back to the VCS host.
type SCMPublisherStage struct {
	throttleInterval time.Duration
}

func NewSCMPublisherStage(throttleInterval time.Duration) *SCMPublisherStage {
	if throttleInterval <= 0 {
		throttleInterval = 10 * time.Millisecond // fast in local tests
	}
	return &SCMPublisherStage{throttleInterval: throttleInterval}
}

func (s *SCMPublisherStage) Name() string {
	return "scm_publisher"
}

func (s *SCMPublisherStage) Execute(ctx context.Context, pCtx *pipeline.PipelineContext) error {
	// If DryRunMode is set, do not post comments
	if pCtx.ReviewParams.DryRunMode {
		pCtx.AddMetric("scm_publish_dry_run", 0, true, nil, len(pCtx.InlineComments))
		return nil
	}

	// Simulate batch posting inline comments with rate limit backoff
	for i := range pCtx.InlineComments {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			// Apply rate limit delay between API calls
			if s.throttleInterval > 0 && i > 0 {
				time.Sleep(s.throttleInterval)
			}
		}
	}

	pCtx.AddMetric("scm_comments_published", 0, true, nil, len(pCtx.InlineComments))
	return nil
}
