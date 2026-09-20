package queue

// QueueDefinition models an AMQP queue with quorum and dead-lettering parameters.
type QueueDefinition struct {
	Name       string
	Durable    bool
	Quorum     bool
	DLX        string
	RoutingKey string
	Arguments  map[string]interface{}
}

// TopologyMetadata holds the centralized exchange and queue definitions.
type TopologyMetadata struct {
	Exchanges []ExchangeDefinition
	Queues    []QueueDefinition
}

// GetDetailedTopologyMetadata returns the canonical topology configuration for the orchestrator.
func GetDetailedTopologyMetadata() *TopologyMetadata {
	return &TopologyMetadata{
		Exchanges: RabbitMQTopologyConfig,
		Queues: []QueueDefinition{
			{
				Name:       "scandrix.workflow.jobs",
				Durable:    true,
				Quorum:     true,
				DLX:        "scandrix.workflow.exchange.dlx",
				RoutingKey: "workflow.job.*",
				Arguments: map[string]interface{}{
					"x-queue-type": "quorum",
				},
			},
			{
				Name:       "scandrix.workflow.jobs.dlq",
				Durable:    true,
				Quorum:     true,
				RoutingKey: "workflow.job.dead",
				Arguments: map[string]interface{}{
					"x-queue-type": "quorum",
				},
			},
			{
				Name:       "scandrix.reviews.feedback",
				Durable:    true,
				Quorum:     true,
				DLX:        "scandrix.orchestrator.exchange.dlx",
				RoutingKey: "reviews.feedback.*",
				Arguments: map[string]interface{}{
					"x-queue-type": "quorum",
				},
			},
		},
	}
}
