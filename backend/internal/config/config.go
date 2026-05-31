package config

import (
	"fmt"
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

func Load() (Config, error) {
	appEnv := env("APP_ENV", "production")
	if appEnv != "development" && appEnv != "production" {
		return Config{}, fmt.Errorf("APP_ENV must be development or production")
	}

	trustProxyHeaders, err := envBool("TRUST_PROXY_HEADERS", false)
	if err != nil {
		return Config{}, err
	}

	trustedProxyCIDRs, err := envCIDRs("TRUSTED_PROXY_CIDRS")
	if err != nil {
		return Config{}, err
	}
	if trustProxyHeaders && len(trustedProxyCIDRs) == 0 {
		return Config{}, fmt.Errorf("TRUSTED_PROXY_CIDRS is required when TRUST_PROXY_HEADERS is enabled")
	}

	return Config{
		AppEnv:            appEnv,
		HTTPAddr:          env("HTTP_ADDR", ":16888"),
		DatabaseURL:       env("DATABASE_URL", "file:var/dev.sqlite"),
		TrustProxyHeaders: trustProxyHeaders,
		TrustedProxyCIDRs: trustedProxyCIDRs,
	}, nil
}

func env(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}

func envBool(key string, fallback bool) (bool, error) {
	value := os.Getenv(key)
	if value == "" {
		return fallback, nil
	}

	switch value {
	case "1", "true", "TRUE", "yes", "YES", "on", "ON":
		return true, nil
	case "0", "false", "FALSE", "no", "NO", "off", "OFF":
		return false, nil
	default:
		return false, fmt.Errorf("%s must be a boolean", key)
	}
}

func envCIDRs(key string) ([]netip.Prefix, error) {
	value := os.Getenv(key)
	if value == "" {
		return nil, nil
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
			return nil, fmt.Errorf("%s contains invalid CIDR %q", key, part)
		}
		prefixes = append(prefixes, prefix)
	}

	return prefixes, nil
}
