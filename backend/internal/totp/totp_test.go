package totp

import (
	"encoding/base32"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// rfc6238Secret is the ASCII seed "12345678901234567890" from RFC 6238
// Appendix B, base32-encoded as this package stores secrets.
var rfc6238Secret = base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString([]byte("12345678901234567890"))

// The published SHA-1 vectors are 8-digit; a 6-digit implementation is the
// same value truncated to its low six digits, which is what every
// authenticator app shows.
func TestCodeMatchesRFC6238Vectors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		unixTime int64
		want     string
	}{
		{name: "59s", unixTime: 59, want: "287082"},
		{name: "1111111109s", unixTime: 1111111109, want: "081804"},
		{name: "1111111111s", unixTime: 1111111111, want: "050471"},
		{name: "1234567890s", unixTime: 1234567890, want: "005924"},
		{name: "2000000000s", unixTime: 2000000000, want: "279037"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			code, err := Code(rfc6238Secret, Step(time.Unix(test.unixTime, 0)))
			require.NoError(t, err)
			assert.Equal(t, test.want, code)
		})
	}
}

func TestValidateAcceptsTheCurrentStepAndOneStepOfDrift(t *testing.T) {
	t.Parallel()

	now := time.Unix(1111111111, 0).UTC()
	current, err := Code(rfc6238Secret, Step(now))
	require.NoError(t, err)
	previous, err := Code(rfc6238Secret, Step(now)-1)
	require.NoError(t, err)
	next, err := Code(rfc6238Secret, Step(now)+1)
	require.NoError(t, err)

	for name, code := range map[string]string{"current": current, "previous": previous, "next": next} {
		step, ok := Validate(rfc6238Secret, code, now, 1)
		assert.True(t, ok, "%s step code must be accepted with skew 1", name)
		assert.NotZero(t, step)
	}

	twoStepsAgo, err := Code(rfc6238Secret, Step(now)-2)
	require.NoError(t, err)
	_, ok := Validate(rfc6238Secret, twoStepsAgo, now, 1)
	assert.False(t, ok, "drift beyond the skew window must be rejected")
}

// The matched step is what the caller stores to stop a code being replayed
// for the rest of its 30-second life, so Validate must report the step the
// code actually came from — not the current one.
func TestValidateReportsTheStepTheCodeCameFrom(t *testing.T) {
	t.Parallel()

	now := time.Unix(1111111111, 0).UTC()
	previous, err := Code(rfc6238Secret, Step(now)-1)
	require.NoError(t, err)

	step, ok := Validate(rfc6238Secret, previous, now, 1)
	require.True(t, ok)
	assert.Equal(t, Step(now)-1, step)
}

func TestValidateRejectsMalformedInput(t *testing.T) {
	t.Parallel()

	now := time.Unix(1111111111, 0).UTC()
	for _, code := range []string{"", "12345", "1234567", "abcdef", "        "} {
		_, ok := Validate(rfc6238Secret, code, now, 1)
		assert.False(t, ok, "code %q must be rejected", code)
	}

	_, ok := Validate("not-base32!", "000000", now, 1)
	assert.False(t, ok, "an unreadable secret must never validate")
}

func TestValidateIgnoresSpacingUsersPasteIn(t *testing.T) {
	t.Parallel()

	now := time.Unix(1111111111, 0).UTC()
	code, err := Code(rfc6238Secret, Step(now))
	require.NoError(t, err)

	spaced := code[:3] + " " + code[3:]
	_, ok := Validate(rfc6238Secret, spaced, now, 1)
	assert.True(t, ok)
}

func TestGenerateSecretIsRandomAndDecodable(t *testing.T) {
	t.Parallel()

	first, err := GenerateSecret()
	require.NoError(t, err)
	second, err := GenerateSecret()
	require.NoError(t, err)
	assert.NotEqual(t, first, second)

	key, err := decodeSecret(first)
	require.NoError(t, err)
	assert.Len(t, key, secretBytes, "160-bit secret per RFC 4226")

	_, err = Code(first, 1)
	assert.NoError(t, err)
}

func TestURIStatesEveryParameterExplicitly(t *testing.T) {
	t.Parallel()

	uri := URI("Rekenraam", "owner", "ABCDEFGH")
	assert.True(t, strings.HasPrefix(uri, "otpauth://totp/Rekenraam:owner?"), "unexpected URI: %s", uri)
	for _, want := range []string{"secret=ABCDEFGH", "issuer=Rekenraam", "algorithm=SHA1", "digits=6", "period=30"} {
		assert.Contains(t, uri, want)
	}
}

func TestStepClampsPreEpochTimes(t *testing.T) {
	t.Parallel()

	assert.Equal(t, uint64(0), Step(time.Unix(-100, 0)))
}
