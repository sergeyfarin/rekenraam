package app

import (
	"testing"

	"github.com/stretchr/testify/require"

	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/exact"
)

func TestValidateBalancedSupportsCoefficientsBeyondInt64(t *testing.T) {
	value := exact.MustParse("12345678901234567890123456789012345678")
	entries := []db.JournalEntrySpec{{Postings: []db.PostingSpec{
		{CommodityID: 1, QuantityValue: value, QuantityScale: 24},
		{CommodityID: 1, QuantityValue: value.Negated(), QuantityScale: 24},
	}}}

	require.NoError(t, validateBalanced(entries))
}
