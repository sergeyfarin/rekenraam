package app

import (
	"database/sql"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCleanInvestmentInstrumentSpecDefaultsAndNormalizesFields(t *testing.T) {
	quoteCommodityID := int64(10)
	tradingCommodityID := int64(20)
	spec, err := cleanInvestmentInstrumentSpec(InvestmentInstrumentInput{
		CommodityID:        ptrInt64(30),
		CommodityCode:      " VWRL ",
		DisplayName:        " Vanguard FTSE All-World UCITS ETF ",
		Symbol:             " vwrl ",
		ExchangeCode:       " ams ",
		MIC:                " xams ",
		Issuer:             " Vanguard ",
		CountryCode:        " nl ",
		QuoteCommodityID:   &quoteCommodityID,
		TradingCommodityID: &tradingCommodityID,
		QuantityScale:      6,
		PriceScale:         4,
		IdentifiersJSON:    `{" isin" : "IE00B3RBWM25" }`,
		MetadataJSON:       ``,
	})
	require.NoError(t, err)

	require.True(t, spec.CommodityID.Valid)
	assert.Equal(t, int64(30), spec.CommodityID.Int64)
	assert.Equal(t, "VWRL", spec.CommodityCode)
	assert.Equal(t, "stock", spec.InstrumentType)
	assert.Equal(t, "Vanguard FTSE All-World UCITS ETF", spec.DisplayName)
	assert.Equal(t, "VWRL", spec.Symbol)
	assert.Equal(t, "AMS", spec.ExchangeCode)
	assert.Equal(t, "XAMS", spec.MIC)
	assert.Equal(t, "NL", spec.CountryCode)
	assert.Equal(t, sql.NullInt64{Int64: quoteCommodityID, Valid: true}, spec.QuoteCommodityID)
	assert.Equal(t, sql.NullInt64{Int64: tradingCommodityID, Valid: true}, spec.TradingCommodityID)
	assert.Equal(t, `{" isin":"IE00B3RBWM25"}`, spec.IdentifiersJSON)
	assert.Equal(t, `{}`, spec.MetadataJSON)
}

func TestCleanInvestmentInstrumentSpecRejectsInvalidInputs(t *testing.T) {
	valid := InvestmentInstrumentInput{
		CommodityCode:   "VWRL",
		InstrumentType:  "etf",
		DisplayName:     "Vanguard FTSE All-World UCITS ETF",
		Symbol:          "VWRL",
		CountryCode:     "NL",
		QuantityScale:   6,
		PriceScale:      4,
		IdentifiersJSON: `{}`,
		MetadataJSON:    `{}`,
	}
	tests := []struct {
		name   string
		mutate func(*InvestmentInstrumentInput)
	}{
		{name: "invalid instrument type", mutate: func(input *InvestmentInstrumentInput) { input.InstrumentType = "crypto_derivative" }},
		{name: "missing display name", mutate: func(input *InvestmentInstrumentInput) { input.DisplayName = " " }},
		{name: "display name too long", mutate: func(input *InvestmentInstrumentInput) { input.DisplayName = strings.Repeat("a", 201) }},
		{name: "country code too short", mutate: func(input *InvestmentInstrumentInput) { input.CountryCode = "N" }},
		{name: "country code has digit", mutate: func(input *InvestmentInstrumentInput) { input.CountryCode = "N1" }},
		{name: "quantity scale too high", mutate: func(input *InvestmentInstrumentInput) { input.QuantityScale = 13 }},
		{name: "price scale negative", mutate: func(input *InvestmentInstrumentInput) { input.PriceScale = -1 }},
		{name: "identifiers must be object", mutate: func(input *InvestmentInstrumentInput) { input.IdentifiersJSON = `[]` }},
		{name: "metadata invalid json", mutate: func(input *InvestmentInstrumentInput) { input.MetadataJSON = `{` }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := valid
			test.mutate(&input)
			_, err := cleanInvestmentInstrumentSpec(input)
			require.Error(t, err)
		})
	}
}

func ptrInt64(value int64) *int64 {
	return &value
}
