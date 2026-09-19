package workflow

// WorkflowJobQueueArguments serves as the single source of truth for AMQP queue arguments.
// This prevents PRECONDITION_FAILED (406) channel closures due to mismatched queue assertions.
var WorkflowJobQueueArguments = map[string]map[string]interface{}{
	"scandrix.workflow.jobs.webhook.queue": {
		"x-queue-type":              "quorum",
		"x-dead-letter-exchange":    "scandrix.workflow.exchange.dlx",
		"x-dead-letter-routing-key": "scandrix.workflow.job.failed",
	},
	"scandrix.workflow.jobs.code_review.queue": {
		"x-queue-type":              "quorum",
		"x-dead-letter-exchange":    "scandrix.workflow.exchange.dlx",
		"x-dead-letter-routing-key": "scandrix.workflow.job.failed",
	},
	"scandrix.workflow.jobs.cli_code_review.queue": {
		"x-queue-type":              "quorum",
		"x-dead-letter-exchange":    "scandrix.workflow.exchange.dlx",
		"x-dead-letter-routing-key": "scandrix.workflow.job.failed",
	},
	"scandrix.workflow.jobs.check_implementation.queue": {
		"x-queue-type":              "quorum",
		"x-dead-letter-exchange":    "scandrix.workflow.exchange.dlx",
		"x-dead-letter-routing-key": "scandrix.workflow.job.failed",
	},
	"scandrix.workflow.jobs.ast_graph_build.queue": {
		"x-queue-type":              "quorum",
		"x-single-active-consumer":  true,
		"x-consumer-timeout":        25 * 60 * 1000,
		"x-dead-letter-exchange":    "scandrix.workflow.exchange.dlx",
		"x-dead-letter-routing-key": "scandrix.workflow.job.failed",
	},
	"scandrix.workflow.jobs.ast_graph_incremental.queue": {
		"x-queue-type":              "quorum",
		"x-single-active-consumer":  true,
		"x-consumer-timeout":        15 * 60 * 1000,
		"x-dead-letter-exchange":    "scandrix.workflow.exchange.dlx",
		"x-dead-letter-routing-key": "scandrix.workflow.job.failed",
	},
}

// GetQueueArguments returns the canonical queue arguments for a named workflow queue.
func GetQueueArguments(queueName string) map[string]interface{} {
	if args, ok := WorkflowJobQueueArguments[queueName]; ok {
		return args
	}
	return map[string]interface{}{
		"x-queue-type": "quorum",
	}
}
