package app

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParsePasswordHashRejectsOutOfRangeArgonParameters(t *testing.T) {
	t.Parallel()

	validHash := "$argon2id$v=19$m=19456,t=2,p=1$yFKjVHLDHsBTRk6lkk88Zg$dJ6u65HlcVuyRD4M7ArLq5QvFzwFgceJMsq/DIucXd0"

	cases := []struct {
		name        string
		replacement string
		message     string
	}{
		{
			name:        "memory overflows uint32",
			replacement: "m=4294967296,t=2,p=1",
			message:     "argon2 memory parameter out of range",
		},
		{
			name:        "iterations overflows uint32",
			replacement: "m=19456,t=4294967296,p=1",
			message:     "argon2 iterations parameter out of range",
		},
		{
			name:        "parallelism overflows uint8",
			replacement: "m=19456,t=2,p=256",
			message:     "argon2 parallelism parameter out of range",
		},
		{
			name:        "parallelism is zero",
			replacement: "m=19456,t=2,p=0",
			message:     "argon2 parallelism parameter out of range",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			encodedHash := strings.Replace(validHash, "m=19456,t=2,p=1", tc.replacement, 1)
			_, err := parsePasswordHash(encodedHash)

			require.Error(t, err)
			assert.Equal(t, tc.message, err.Error())
		})
	}
}

func TestParsePasswordHashAcceptsConfiguredArgonParameters(t *testing.T) {
	t.Parallel()

	parsed, err := parsePasswordHash("$argon2id$v=19$m=19456,t=2,p=1$yFKjVHLDHsBTRk6lkk88Zg$dJ6u65HlcVuyRD4M7ArLq5QvFzwFgceJMsq/DIucXd0")

	require.NoError(t, err)
	assert.Equal(t, 19, parsed.Version)
	assert.Equal(t, argon2MemoryKiB, parsed.MemoryKiB)
	assert.Equal(t, argon2Iterations, parsed.Iterations)
	assert.Equal(t, argon2Parallelism, parsed.Parallelism)
	assert.Len(t, parsed.Salt, argon2SaltLength)
	assert.Len(t, parsed.Hash, argon2KeyLength)
}
