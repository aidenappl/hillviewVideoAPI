package env

import (
	"fmt"
	"os"
)

var (
	Port            = getEnv("PORT", "8000")
	DatabaseDSN     = getEnvOrPanic("DATABASE_DSN")
	JWTSigningKey   = getEnvOrPanic("JWT_SIGNING_KEY")
	AWSRegion       = getEnvOrPanic("AWS_REGION")
	AWSKey          = getEnvOrPanic("AWS_ACCESS_KEY_ID")
	AWSSecret       = getEnvOrPanic("AWS_SECRET_ACCESS_KEY")
	CloudflareUID   = getEnvOrPanic("CLOUDFLARE_UID")
	CloudflareKey   = getEnvOrPanic("CLOUDFLARE_KEY")
	CloudflareToken = getEnvOrPanic("CLOUDFLARE_TOKEN")
	CloudflareEmail = getEnvOrPanic("CLOUDFLARE_EMAIL")
	SendgridAPIKey  = getEnvOrPanic("SENDGRID_API_KEY")
	HealthCheckURL  = getEnvOrPanic("HEALTH_CHECK_URL")

	// Mux (replacing Cloudflare Stream). Optional while both providers run:
	// unset, Mux calls fail with mux.ErrNotConfigured instead of the service
	// failing to boot.
	MuxTokenID       = getEnv("MUX_TOKEN_ID", "")
	MuxTokenSecret   = getEnv("MUX_TOKEN_SECRET", "")
	MuxWebhookSecret = getEnv("MUX_WEBHOOK_SECRET", "")

	// PublicAPIURL is this API's public base url. Mux fetches edited captions
	// from PublicAPIURL + /captions/{uuid}/en.vtt.
	PublicAPIURL = getEnv("PUBLIC_API_URL", "https://api.hillview.tv/video/v1.1")

	// Monitor (zone hillview). All optional: unset, the service runs without
	// telemetry rather than failing to boot.
	MonitorIngestURL = getEnv("MONITOR_INGEST_URL", "")
	MonitorAPIKey    = getEnv("MONITOR_API_KEY", "")
	MonitorEnv       = getEnv("MONITOR_ENV", "production")
	MonitorSpoolDir  = getEnv("MONITOR_SPOOL_DIR", "")
	MonitorStdout    = getEnv("MONITOR_STDOUT", "false") == "true"
)

func getEnv(key string, fallback string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}

	return fallback
}

func getEnvOrPanic(key string) string {
	value, ok := os.LookupEnv(key)
	if !ok {
		panic(fmt.Sprintf("Missing required environment variable '%v'\n", key))
	}
	return value
}
