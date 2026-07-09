package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadRejectsInvalidProxyBoolean(t *testing.T) {
	t.Setenv("TRUST_PROXY_HEADERS", "maybe")

	_, err := Load()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "TRUST_PROXY_HEADERS must be a boolean")
}

func TestLoadRejectsTrustedProxyWithoutCIDRs(t *testing.T) {
	t.Setenv("TRUST_PROXY_HEADERS", "1")

	_, err := Load()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "TRUSTED_PROXY_CIDRS is required")
}

func TestLoadRejectsInvalidTrustedProxyCIDR(t *testing.T) {
	t.Setenv("TRUST_PROXY_HEADERS", "1")
	t.Setenv("TRUSTED_PROXY_CIDRS", "not-a-cidr")

	_, err := Load()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "TRUSTED_PROXY_CIDRS contains invalid CIDR")
}

func TestLoadRejectsZeroPrefixTrustedProxyCIDR(t *testing.T) {
	t.Setenv("TRUST_PROXY_HEADERS", "1")
	t.Setenv("TRUSTED_PROXY_CIDRS", "0.0.0.0/0")

	_, err := Load()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "matches all addresses")
}

func TestLoadRejectsInvalidAppEnv(t *testing.T) {
	t.Setenv("APP_ENV", "staging")

	_, err := Load()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "APP_ENV must be development or production")
}

func TestLoadReadsOpenExchangeRatesAppID(t *testing.T) {
	t.Setenv("OPEN_EXCHANGE_RATES_APP_ID", " secret ")

	cfg, err := Load()

	require.NoError(t, err)
	assert.Equal(t, "secret", cfg.OpenExchangeRatesAppID)
}

func TestLoadDefaultsSessionLifetimeToThirtyDays(t *testing.T) {
	cfg, err := Load()

	require.NoError(t, err)
	assert.Equal(t, 720*time.Hour, cfg.SessionLifetime)
}

func TestLoadReadsSessionLifetimeHours(t *testing.T) {
	t.Setenv("SESSION_LIFETIME_HOURS", "12")

	cfg, err := Load()

	require.NoError(t, err)
	assert.Equal(t, 12*time.Hour, cfg.SessionLifetime)
}

func TestLoadRejectsInvalidSessionLifetimeHours(t *testing.T) {
	for _, value := range []string{"0", "-1", "soon"} {
		t.Run(value, func(t *testing.T) {
			t.Setenv("SESSION_LIFETIME_HOURS", value)

			_, err := Load()

			require.Error(t, err)
			assert.Contains(t, err.Error(), "SESSION_LIFETIME_HOURS must be a positive integer number of hours")
		})
	}
}
