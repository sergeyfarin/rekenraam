package config

import (
	"net/netip"
	"os"
	"strings"
)

type Config struct {
	AppEnv            string
	HTTPAddr          string
	DatabaseURL       string
	TrustProxyHeaders bool
	TrustedProxyCIDRs []netip.Prefix
}

func Load() Config {
	return Config{
		AppEnv:            env("APP_ENV", "production"),
		HTTPAddr:          env("HTTP_ADDR", ":16888"),
		DatabaseURL:       env("DATABASE_URL", "file:var/dev.sqlite"),
		TrustProxyHeaders: envBool("TRUST_PROXY_HEADERS", false),
		TrustedProxyCIDRs: envCIDRs("TRUSTED_PROXY_CIDRS"),
	}
}

func env(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}

func envBool(key string, fallback bool) bool {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	switch value {
	case "1", "true", "TRUE", "yes", "YES", "on", "ON":
		return true
	case "0", "false", "FALSE", "no", "NO", "off", "OFF":
		return false
	default:
		return fallback
	}
}

func envCIDRs(key string) []netip.Prefix {
	value := os.Getenv(key)
	if value == "" {
		return nil
	}

	parts := strings.Split(value, ",")
	prefixes := make([]netip.Prefix, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		prefix, err := netip.ParsePrefix(part)
		if err != nil {
			continue
		}
		prefixes = append(prefixes, prefix)
	}

	return prefixes
}
