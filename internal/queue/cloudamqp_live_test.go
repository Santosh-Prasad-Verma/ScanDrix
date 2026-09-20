package queue_test

import (
	"os"
	"testing"

	"github.com/joho/godotenv"
	"github.com/scandrix/backend/internal/queue"
)

func TestCloudAMQPLiveConnection(t *testing.T) {
	if os.Getenv("TEST_LIVE_AMQP") != "true" {
		t.Skip("skipping live CloudAMQP test: set TEST_LIVE_AMQP=true to run against external broker")
	}
	_ = godotenv.Load("../../.env", "../../../.env")
	uri := os.Getenv("API_RABBITMQ_URI")
	if uri == "" {
		uri = os.Getenv("RABBITMQ_URL")
	}
	if uri == "" {
		t.Skip("skipping live CloudAMQP test: no RabbitMQ URI configured")
	}

	broker, err := queue.NewBroker(uri)
	if err != nil {
		t.Fatalf("failed to connect to CloudAMQP: %v", err)
	}
	defer broker.Close()

	t.Log("SUCCESS: Connected to managed CloudAMQP cluster and declared all queues!")
}
