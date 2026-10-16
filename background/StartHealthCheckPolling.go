package background

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/hillview.tv/videoAPI/env"
	"github.com/hillview.tv/videoAPI/telemetry"
)

func StartHealthCheckPolling(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			// Create HTTP client with timeout
			client := &http.Client{
				Timeout: 10 * time.Second,
			}

			// Make GET request to health check endpoint
			resp, err := client.Get(env.HealthCheckURL)
			if err != nil {
				log.Printf("❌ Health check failed: %v", err)
				telemetry.Warn(ctx, "healthcheck.ping.failed", err, nil)
				continue
			}

			resp.Body.Close()

			if resp.StatusCode == http.StatusOK {
				// Health check successful
			} else {
				log.Printf("⚠️ Health check returned status: %d", resp.StatusCode)
				telemetry.Warn(ctx, "healthcheck.ping.rejected", nil, map[string]any{"status_code": resp.StatusCode})
			}

		case <-ctx.Done():
			log.Println("🚦 Stopping health check polling...")
			return
		}
	}
}
