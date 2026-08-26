package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	log.Println("⚡ CodeHound Ingestion & AST Analysis Worker starting...")

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	log.Println("✅ Worker ready to process NATS / Temporal ingestion queues.")
	<-sigChan
	log.Println("🛑 Worker stopping gracefully...")
}
