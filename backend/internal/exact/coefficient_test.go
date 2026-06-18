package exact

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCoefficientJSONIsLosslessAndAcceptsLegacyNumbers(t *testing.T) {
	var coefficient Coefficient
	require.NoError(t, json.Unmarshal([]byte(`"123456789012345678901234"`), &coefficient))
	assert.Equal(t, "123456789012345678901234", coefficient.String())

	encoded, err := json.Marshal(coefficient)
	require.NoError(t, err)
	assert.JSONEq(t, `"123456789012345678901234"`, string(encoded))

	require.NoError(t, json.Unmarshal([]byte(`-1200`), &coefficient))
	assert.Equal(t, "-1200", coefficient.String())
}

func TestCoefficientRejectsMoreThanThirtyEightDigits(t *testing.T) {
	_, err := Parse("123456789012345678901234567890123456789")
	require.Error(t, err)
}
