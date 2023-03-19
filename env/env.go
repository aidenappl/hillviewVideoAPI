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
	CloudflareEmail = getEnvOrPanic("CLOUDFLARE_EMAIL")
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
