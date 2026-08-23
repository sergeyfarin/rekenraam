package app

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"rekenraam/backend/internal/db"
)

// Characterization test for sortRowsForInvestmentCommitOrder. The commit order
// decides which lots a disposal matches, so this pins the exact ordering
// contract described in that function's doc comment: investment rows sort by
// their fill/date key, non-investment rows keep their relative order, and the
// sort is stable throughout.
func TestSortRowsForInvestmentCommitOrder(t *testing.T) {
	row := func(id int64, raw, normalized string) db.ImportStagedRowRecord {
		return db.ImportStagedRowRecord{ID: id, RawJSON: raw, NormalizedJSON: normalized}
	}
	fill := func(id int64, filledAt string) db.ImportStagedRowRecord {
		return row(id, `{"kind":"trading212_order_fill","filled_at":"`+filledAt+`"}`, `{}`)
	}
	dividend := func(id int64, date string) db.ImportStagedRowRecord {
		return row(id, `{"kind":"trading212_dividend"}`, `{"date":"`+date+`"}`)
	}
	plain := func(id int64) db.ImportStagedRowRecord {
		return row(id, `{"kind":"trading212_transaction"}`, `{}`)
	}

	t.Run("order fills sort by filled_at", func(t *testing.T) {
		rows := []db.ImportStagedRowRecord{
			fill(1, "2026-03-01T10:00:00Z"),
			fill(2, "2026-01-01T10:00:00Z"),
			fill(3, "2026-02-01T10:00:00Z"),
		}
		sortRowsForInvestmentCommitOrder(rows)
		assert.Equal(t, []int64{2, 3, 1}, ids(rows))
	})

	t.Run("dividends fall back to the normalized date", func(t *testing.T) {
		rows := []db.ImportStagedRowRecord{
			dividend(1, "2026-05-01"),
			dividend(2, "2026-04-01"),
		}
		sortRowsForInvestmentCommitOrder(rows)
		assert.Equal(t, []int64{2, 1}, ids(rows))
	})

	t.Run("empty-key rows keep their original relative order", func(t *testing.T) {
		rows := []db.ImportStagedRowRecord{plain(1), plain(2), plain(3)}
		sortRowsForInvestmentCommitOrder(rows)
		assert.Equal(t, []int64{1, 2, 3}, ids(rows))
	})

	t.Run("empty keys sort ahead of investment rows, stably", func(t *testing.T) {
		rows := []db.ImportStagedRowRecord{
			fill(1, "2026-03-01T10:00:00Z"),
			plain(2),
			fill(3, "2026-01-01T10:00:00Z"),
			plain(4),
		}
		sortRowsForInvestmentCommitOrder(rows)
		assert.Equal(t, []int64{2, 4, 3, 1}, ids(rows))
	})

	t.Run("unparseable json sorts as an empty key", func(t *testing.T) {
		rows := []db.ImportStagedRowRecord{
			fill(1, "2026-03-01T10:00:00Z"),
			row(2, `{not json`, `{}`),
		}
		sortRowsForInvestmentCommitOrder(rows)
		assert.Equal(t, []int64{2, 1}, ids(rows))
	})

	t.Run("an order fill without filled_at falls back to the normalized date", func(t *testing.T) {
		rows := []db.ImportStagedRowRecord{
			fill(1, "2026-03-01T10:00:00Z"),
			row(2, `{"kind":"trading212_order_fill"}`, `{"date":"2026-01-01"}`),
		}
		sortRowsForInvestmentCommitOrder(rows)
		assert.Equal(t, []int64{2, 1}, ids(rows))
	})
}

func ids(rows []db.ImportStagedRowRecord) []int64 {
	out := make([]int64, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.ID)
	}
	return out
}

func BenchmarkSortRowsForInvestmentCommitOrder(b *testing.B) {
	const n = 2000
	base := make([]db.ImportStagedRowRecord, n)
	for i := range n {
		base[i] = db.ImportStagedRowRecord{
			ID:             int64(i + 1),
			RawJSON:        `{"kind":"trading212_order_fill","filled_at":"2026-01-01T10:00:00Z"}`,
			NormalizedJSON: `{"date":"2026-01-01"}`,
		}
	}
	rows := make([]db.ImportStagedRowRecord, n)
	for b.Loop() {
		copy(rows, base)
		sortRowsForInvestmentCommitOrder(rows)
	}
}
