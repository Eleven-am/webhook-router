package app

import (
	"time"

	"webhook-router/internal/brokers/manager"
	"webhook-router/internal/common/logging"
	"webhook-router/internal/storage"
)

// StartDLQWorker starts a worker that periodically retries DLQ messages
func StartDLQWorker(store storage.Storage, brokerManager *manager.Manager) chan struct{} {
	stopWorker := make(chan struct{})

	go func() {
		ticker := time.NewTicker(1 * time.Minute)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				// Retry per-broker DLQ messages
				if brokerManager != nil {
					if err := brokerManager.RetryDLQMessages(); err != nil {
						logging.Warn("Per-broker DLQ retry error", logging.Field{"error", err})
					}
				}
			case <-stopWorker:
				return
			}
		}
	}()

	logging.Info("DLQ retry worker started")
	return stopWorker
}
