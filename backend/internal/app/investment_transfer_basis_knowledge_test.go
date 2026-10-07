package app

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// Test the immutable evidence constraint independently of the known-only
// command. Each transaction replaces a fixture link and rolls back, including
// the temporary removal of its deletion guard.
func TestTransferBasisKnowledgeRequiresCompleteAmountPair(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	seedExternalTransferEquity(t, f.database)
	_, err := f.investmentService.ExternalTransferIn(context.Background(), knownTransferInput(f))
	require.NoError(t, err)
	for _, knowledge := range []string{"known", "unknown"} {
		for mask := 0; mask < 8; mask++ {
			name := knowledge + "/"
			values := []any{nil, nil, nil}
			if mask&1 != 0 {
				values[0] = "1250"
				name += "value_"
			}
			if mask&2 != 0 {
				values[1] = 2
				name += "scale_"
			}
			if mask&4 != 0 {
				values[2] = f.eurCommodityID
				name += "currency"
			}
			t.Run(name, func(t *testing.T) {
				tx, err := f.database.BeginTx(context.Background(), nil)
				require.NoError(t, err)
				defer tx.Rollback()
				_, err = tx.Exec(`CREATE TEMP TABLE basis_link_fixture AS SELECT * FROM investment_transfer_lot_links`)
				require.NoError(t, err)
				_, err = tx.Exec(`DROP TRIGGER investment_transfer_lot_links_no_delete`)
				require.NoError(t, err)
				_, err = tx.Exec(`DELETE FROM investment_transfer_lot_links`)
				require.NoError(t, err)
				_, err = tx.Exec(`INSERT INTO investment_transfer_lot_links
					(operation_id, link_seq, source_lot_id, destination_lot_id, quantity_value, quantity_scale,
					 basis_knowledge, carried_basis_value, carried_basis_scale, cost_commodity_id,
					 original_date_knowledge, original_acquired_on, source_evidence_json)
					SELECT operation_id, link_seq, source_lot_id, destination_lot_id, quantity_value, quantity_scale,
					 ?, ?, ?, ?, original_date_knowledge, original_acquired_on, source_evidence_json
					FROM basis_link_fixture`, knowledge, values[0], values[1], values[2])
				valid := knowledge == "known" && mask == 7 || knowledge == "unknown" && mask&3 == 0
				if valid {
					require.NoError(t, err, "unknown may retain its named currency, but neither amount field")
				} else {
					require.ErrorContains(t, err, "CHECK constraint failed", "partial amounts must not masquerade as unknown")
				}
			})
		}
	}
}
