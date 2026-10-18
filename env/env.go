package env

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/aidenappl/go-keyring"
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

	// Mux (replacing Cloudflare Stream), resolved from the environment or
	// Keyring. Optional while both providers run: unset, Mux calls fail with
	// mux.ErrNotConfigured instead of the service failing to boot.
	MuxTokenID       = getSecret("MUX_TOKEN_ID")
	MuxTokenSecret   = getSecret("MUX_TOKEN_SECRET")
	MuxWebhookSecret = getSecret("MUX_WEBHOOK_SECRET")

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

// keyringSecrets holds the secrets granted to this service's Keyring token,
// loaded once at startup. Empty when Keyring is not configured
// (KEYRING_URL / KEYRING_ACCESS_KEY_ID / KEYRING_SECRET_ACCESS_KEY unset),
// disabled (KEYRING_SOURCE=env), or unreachable.
var keyringSecrets = loadKeyring()

func loadKeyring() map[string]string {
	if keyring.EnvOnly() {
		return nil
	}
	client, err := keyring.New()
	if err != nil {
		log.Printf("keyring: not configured, using environment only: %v", err)
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	secrets, err := client.Load(ctx)
	if err != nil {
		log.Printf("keyring: load failed, using environment only: %v", err)
		return nil
	}
	log.Printf("keyring: loaded %d secrets", len(secrets))
	return secrets
}

// getSecret resolves an optional secret: the environment first (so local runs
// and the stack can override), then Keyring, else "".
func getSecret(key string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return keyringSecrets[key]
}
