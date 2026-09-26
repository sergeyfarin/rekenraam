package api

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestInvestmentMoneyCoefficientWireBoundary(t *testing.T) {
	for _, tc := range []struct{ input, output string }{
		{`"9007199254740993"`, `"9007199254740993"`},
		{`"9223372036854775807"`, `"9223372036854775807"`},
		{`"-9223372036854775808"`, `"-9223372036854775808"`},
		{`9007199254740993`, `"9007199254740993"`},
	} {
		var coefficient moneyCoefficient
		require.NoError(t, json.Unmarshal([]byte(tc.input), &coefficient), tc.input)
		wire, err := json.Marshal(coefficient)
		require.NoError(t, err)
		require.Equal(t, tc.output, string(wire))
	}
	for _, input := range []string{`"9223372036854775808"`, `"-9223372036854775809"`, `"01"`, `"-0"`, `1.5`, `null`} {
		var coefficient moneyCoefficient
		require.Error(t, json.Unmarshal([]byte(input), &coefficient), input)
	}
}
