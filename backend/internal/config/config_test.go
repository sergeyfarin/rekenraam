package config

import (
	"testing"

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
