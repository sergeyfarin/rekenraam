package config

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net/netip"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	AppEnv                 string
	HTTPAddr               string
	DatabaseURL            string
	SetupToken             string
	GeneratedSetupToken    bool
	SessionLifetime        time.Duration
	TrustProxyHeaders      bool
	TrustedProxyCIDRs      []netip.Prefix
	OpenExchangeRatesAppID string
	// SecretKey is a 32-byte AES-256 key (base64-encoded) used to seal online
	// provider credentials at rest. Required when any import connection exists;
	// the server starts without it but returns CONFIG_REQUIRED if unset when
	// a connection operation is attempted.
	SecretKey []byte
}

func Load() (Config, error) {
	appEnv := env("APP_ENV", "production")
	if appEnv != "development" && appEnv != "production" {
		return Config{}, fmt.Errorf("APP_ENV must be development or production")
	}

	sessionLifetime, err := envPositiveHours("SESSION_LIFETIME_HOURS", 720)
	if err != nil {
		return Config{}, err
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
	for _, cidr := range trustedProxyCIDRs {
		if cidr.Bits() == 0 {
			return Config{}, fmt.Errorf("TRUSTED_PROXY_CIDRS: %s matches all addresses; specify a narrower network range", cidr)
		}
	}

	secretKey, err := loadSecretKey()
	if err != nil {
		return Config{}, err
	}
	setupToken, generatedSetupToken, err := loadSetupToken(appEnv)
	if err != nil {
		return Config{}, err
	}

	return Config{
		AppEnv:                 appEnv,
		HTTPAddr:               env("HTTP_ADDR", ":16888"),
		DatabaseURL:            env("DATABASE_URL", "file:var/dev.sqlite"),
		SetupToken:             setupToken,
		GeneratedSetupToken:    generatedSetupToken,
		SessionLifetime:        sessionLifetime,
		TrustProxyHeaders:      trustProxyHeaders,
		TrustedProxyCIDRs:      trustedProxyCIDRs,
		OpenExchangeRatesAppID: strings.TrimSpace(os.Getenv("OPEN_EXCHANGE_RATES_APP_ID")),
		SecretKey:              secretKey,
	}, nil
}

func loadSetupToken(appEnv string) (string, bool, error) {
	token := strings.TrimSpace(os.Getenv("SETUP_TOKEN"))
	if token != "" {
		if len(token) < 32 {
			return "", false, fmt.Errorf("SETUP_TOKEN must be at least 32 characters")
		}
		return token, false, nil
	}
	if appEnv != "production" {
		return "", false, nil
	}

	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", false, fmt.Errorf("generate setup token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(bytes), true, nil
}

// loadSecretKey reads REKENRAAM_SECRET_KEY from the environment.
// The value must be base64-encoded standard encoding of exactly 32 bytes.
// An absent key is allowed (returns nil); an invalid value is a hard error.
func loadSecretKey() ([]byte, error) {
	raw := strings.TrimSpace(os.Getenv("REKENRAAM_SECRET_KEY"))
	if raw == "" {
		return nil, nil
	}
	key, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return nil, fmt.Errorf("REKENRAAM_SECRET_KEY: invalid base64: %w", err)
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("REKENRAAM_SECRET_KEY: must decode to exactly 32 bytes (got %d)", len(key))
	}
	return key, nil
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

func envPositiveHours(key string, fallback int) (time.Duration, error) {
	value := os.Getenv(key)
	if value == "" {
		return time.Duration(fallback) * time.Hour, nil
	}

	hours, err := strconv.Atoi(value)
	if err != nil || hours <= 0 {
		return 0, fmt.Errorf("%s must be a positive integer number of hours", key)
	}

	return time.Duration(hours) * time.Hour, nil
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
