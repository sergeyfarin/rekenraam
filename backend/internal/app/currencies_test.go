package app

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCurrencySpecNormalizesCodeAndUsesCatalogFallback(t *testing.T) {
	service := NewCurrencyService(nil, nil)

	spec, err := service.currencySpec(" usd ", "")
	require.NoError(t, err)

	assert.Equal(t, "USD", spec.Code)
	assert.Equal(t, "US Dollar", spec.Name)
	assert.Equal(t, "$", spec.Symbol)
	assert.Equal(t, 2, spec.StandardScale)
}

func TestCurrencySpecRejectsInvalidInputs(t *testing.T) {
	service := NewCurrencyService(nil, nil)
	tests := []struct {
		name      string
		code      string
		valueName string
		wantErr   error
	}{
		{name: "empty code", code: " ", wantErr: ValidationError{}},
		{name: "too short", code: "US", wantErr: ValidationError{}},
		{name: "unknown catalog code", code: "ZZZ", wantErr: ErrCurrencyNotFound},
		{name: "name too long", code: "USD", valueName: strings.Repeat("a", currencyNameMaxBytes+1), wantErr: ValidationError{}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := service.currencySpec(test.code, test.valueName)
			require.Error(t, err)
			if errors.Is(test.wantErr, ErrCurrencyNotFound) {
				assert.ErrorIs(t, err, ErrCurrencyNotFound)
			} else {
				var validationErr ValidationError
				assert.ErrorAs(t, err, &validationErr)
			}
		})
	}
}
