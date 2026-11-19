// Package config loads the proxy's runtime settings from the environment.
package config

import (
	"os"
	"strings"
)

// Config is the proxy's complete runtime configuration.
type Config struct {
	// APIKey authenticates every request to the upstream Mistral API. It may be
	// empty at startup and set later from the web UI.
	APIKey string
	// BaseURL is the upstream API origin, without a trailing slash.
	BaseURL string
	// ProxyKey, when set, must match the caller's bearer token.
	ProxyKey string
	// ListenAddr is the address the HTTP server binds to.
	ListenAddr string
	// WebDir holds the built dashboard served at the root path.
	WebDir string
	// AnthropicModel replaces Claude model names sent to the Anthropic route.
	AnthropicModel string
	// UserAgent is sent upstream when the caller does not supply one.
	UserAgent string
	// Debug logs request bodies and upstream error payloads.
	Debug bool
}

// Load reads configuration from the environment and validates it.
//
//	MISTRAL_API_KEY  upstream credential (can also be set from the web UI)
//	MISTRAL_BASE_URL upstream origin (default https://api.mistral.ai)
//	PROXY_API_KEY    optional inbound bearer token
//	LISTEN_ADDR      bind address (default :8080)
//	WEB_DIR          built dashboard directory (default web/dist)
func Load() (Config, error) {
	cfg := Config{
		APIKey:         os.Getenv("MISTRAL_API_KEY"),
		BaseURL:        strings.TrimRight(envOr("MISTRAL_BASE_URL", "https://api.mistral.ai"), "/"),
		ProxyKey:       os.Getenv("PROXY_API_KEY"),
		ListenAddr:     envOr("LISTEN_ADDR", ":8080"),
		WebDir:         envOr("WEB_DIR", "web/dist"),
		AnthropicModel: envOr("ANTHROPIC_MODEL", "mistral-large-4-0"),
		UserAgent:      envOr("UPSTREAM_USER_AGENT", "Mistral-Vibe/3.1.3"),
		Debug:          os.Getenv("DEBUG") == "1",
	}
	return cfg, nil
}

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
