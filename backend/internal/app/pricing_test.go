package app

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCleanPriceObservationSpecDefaultsAndCompactsJSON(t *testing.T) {
	spec, err := cleanPriceObservationSpec(PriceObservationInput{
		BaseCommodityID:    10,
		QuoteCommodityID:   20,
		PriceValue:         12345,
		PriceScale:         4,
		ValuationDate:      " 2026-06-12 ",
		ObservedAt:         "2026-06-12T11:59:00Z",
		SourcePublishedAt:  "2026-06-12T12:00:00Z",
		DerivationJSON:     `{" formula" : "manual" }`,
		SeriesMetadataJSON: `{" series" : "manual" }`,
		MetadataJSON:       `{" source" : "test" }`,
	})
	require.NoError(t, err)

	assert.Equal(t, int64(10), spec.BaseCommodityID)
	assert.Equal(t, int64(20), spec.QuoteCommodityID)
	assert.Equal(t, "manual", spec.QuoteType)
	assert.Equal(t, "raw", spec.AdjustmentBasis)
	assert.Equal(t, int64(1), spec.BaseQuantityValue)
	assert.Equal(t, 0, spec.BaseQuantityScale)
	assert.Equal(t, "2026-06-12", spec.ValuationDate)
	assert.Equal(t, `{" formula":"manual"}`, spec.DerivationJSON)
	assert.Equal(t, `{" series":"manual"}`, spec.SeriesMetadataJSON)
	assert.Equal(t, `{" source":"test"}`, spec.MetadataJSON)
}

func TestCleanPriceObservationSpecRejectsInvalidInputs(t *testing.T) {
	valid := PriceObservationInput{
		BaseCommodityID:   10,
		QuoteCommodityID:  20,
		QuoteType:         "official_fixing",
		AdjustmentBasis:   "not_applicable",
		PriceValue:        12345,
		PriceScale:        4,
		BaseQuantityValue: 1,
		BaseQuantityScale: 0,
		ValuationDate:     "2026-06-12",
		MetadataJSON:      `{}`,
	}
	tests := []struct {
		name   string
		mutate func(*PriceObservationInput)
	}{
		{name: "missing base commodity", mutate: func(input *PriceObservationInput) { input.BaseCommodityID = 0 }},
		{name: "missing quote commodity", mutate: func(input *PriceObservationInput) { input.QuoteCommodityID = 0 }},
		{name: "zero price", mutate: func(input *PriceObservationInput) { input.PriceValue = 0 }},
		{name: "negative price", mutate: func(input *PriceObservationInput) { input.PriceValue = -1 }},
		{name: "price scale too high", mutate: func(input *PriceObservationInput) { input.PriceScale = 19 }},
		{name: "invalid valuation date", mutate: func(input *PriceObservationInput) { input.ValuationDate = "2026/06/12" }},
		{name: "invalid quote type", mutate: func(input *PriceObservationInput) { input.QuoteType = "last_trade" }},
		{name: "invalid adjustment basis", mutate: func(input *PriceObservationInput) { input.AdjustmentBasis = "magic" }},
		{name: "negative base quantity", mutate: func(input *PriceObservationInput) { input.BaseQuantityValue = -1 }},
		{name: "base quantity scale too high", mutate: func(input *PriceObservationInput) { input.BaseQuantityScale = 19 }},
		{name: "invalid observed at", mutate: func(input *PriceObservationInput) { input.ObservedAt = "2026-06-12 12:00:00" }},
		{name: "metadata is array", mutate: func(input *PriceObservationInput) { input.MetadataJSON = `[]` }},
		{name: "metadata invalid json", mutate: func(input *PriceObservationInput) { input.MetadataJSON = `{` }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := valid
			test.mutate(&input)
			_, err := cleanPriceObservationSpec(input)
			require.Error(t, err)
		})
	}
}
