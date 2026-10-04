package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"sort"

	"rekenraam/backend/internal/exact"
)

// ErrCommandSupersedesVersions marks a command routed through combined
// checkpoint netting that rewrote an existing transaction version. Netting
// only covers appended journals; rewriting reconciled postings keeps the
// per-posting guard, so this is a programming error, never a user refusal.
var ErrCommandSupersedesVersions = errors.New("combined checkpoint netting covers appended journals only")

// CheckpointDelta is one posting a command adds to the ledger, at its register
// position (entry date and account day sequence).
type CheckpointDelta struct {
	AccountID          int64
	CommodityID        int64
	EntryDate          string
	AccountDaySequence int64
	QuantityValue      exact.Coefficient
	QuantityScale      int
}

func (d CheckpointDelta) atOrBefore(date string, sequence int64) bool {
	return d.EntryDate < date || (d.EntryDate == date && d.AccountDaySequence <= sequence)
}

// CheckpointDeltasFromSpec lists the postings a new posted spec would add. A
// new posting is allocated after every existing posting on its date, so it
// sits after any checkpoint boundary on that date, as math.MaxInt64 does.
func CheckpointDeltasFromSpec(spec TransactionSpec) []CheckpointDelta {
	if spec.Status == "draft" {
		return nil
	}
	var deltas []CheckpointDelta
	for _, entry := range spec.JournalEntries {
		for _, posting := range entry.Postings {
			deltas = append(deltas, CheckpointDelta{
				AccountID: posting.AccountID, CommodityID: posting.CommodityID,
				EntryDate: entry.EntryDate, AccountDaySequence: math.MaxInt64,
				QuantityValue: posting.QuantityValue, QuantityScale: posting.QuantityScale,
			})
		}
	}
	return deltas
}

// NetCheckpointInvalidationRefs is the pool-side reading of the combined
// calculation, for previews of a single new journal.
func (r *TransactionRepository) NetCheckpointInvalidationRefs(ctx context.Context, bookID int64, deltas []CheckpointDelta) ([]CheckpointInvalidationRef, error) {
	return netCheckpointInvalidationRefs(ctx, r.database, bookID, deltas)
}

// netCheckpointInvalidationRefs is a command's combined reconciliation impact
// (T-120 #135). For each account and commodity it sums the command's deltas
// up to each active checkpoint's own (statement_date,
// statement_account_sequence) boundary. The first boundary whose sum is not
// zero changed its reconciled balance; it and every later active checkpoint
// are returned, the cascade every other guard applies. Deltas that cancel
// before a boundary leave it — and any earlier one — active; equal amounts on
// different sides of a boundary do not cancel at it. Each ref carries the
// earliest delta date for its account and commodity.
func netCheckpointInvalidationRefs(ctx context.Context, queryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}, bookID int64, deltas []CheckpointDelta) ([]CheckpointInvalidationRef, error) {
	type position struct{ accountID, commodityID int64 }
	grouped := map[position][]CheckpointDelta{}
	var order []position
	for _, delta := range deltas {
		key := position{delta.AccountID, delta.CommodityID}
		if _, exists := grouped[key]; !exists {
			order = append(order, key)
		}
		grouped[key] = append(grouped[key], delta)
	}
	sort.Slice(order, func(i, j int) bool {
		if order[i].accountID != order[j].accountID {
			return order[i].accountID < order[j].accountID
		}
		return order[i].commodityID < order[j].commodityID
	})

	var refs []CheckpointInvalidationRef
	for _, key := range order {
		group := grouped[key]
		earliest := group[0]
		for _, delta := range group[1:] {
			if delta.atOrBefore(earliest.EntryDate, earliest.AccountDaySequence) {
				earliest = delta
			}
		}
		checkpoints, err := activeCheckpointRefsAtOrAfter(ctx, queryer, bookID, PeriodScopedCheckpointRef{
			AccountID: key.accountID, CommodityID: key.commodityID,
			EntryDate: earliest.EntryDate, AccountDaySequence: earliest.AccountDaySequence,
		})
		if err != nil {
			return nil, err
		}
		for index, checkpoint := range checkpoints {
			sum := exact.NewScaledInt()
			for _, delta := range group {
				if delta.atOrBefore(checkpoint.StatementDate, checkpoint.StatementAccountSequence) {
					sum.AddCoefficient(delta.QuantityValue, delta.QuantityScale)
				}
			}
			if sum.Sign() != 0 {
				refs = append(refs, checkpoints[index:]...)
				break
			}
		}
	}
	return refs, nil
}

// commandCheckpointDeltasTx reads every posting the command wrote under its
// audit event, at the positions they were actually allocated.
func commandCheckpointDeltasTx(ctx context.Context, tx *sql.Tx, bookID, auditEventID int64) ([]CheckpointDelta, error) {
	var superseding int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM transaction_versions
		WHERE book_id = ? AND change_audit_event_id = ? AND supersedes_version_id IS NOT NULL`,
		bookID, auditEventID).Scan(&superseding); err != nil {
		return nil, fmt.Errorf("check command version supersession: %w", err)
	}
	if superseding > 0 {
		return nil, ErrCommandSupersedesVersions
	}
	rows, err := tx.QueryContext(ctx, `
		SELECT pv.account_id, pv.commodity_id, je.entry_date, pv.account_day_sequence,
			pv.quantity_value, pv.quantity_scale
		FROM transaction_versions tv
		JOIN posting_versions pv ON pv.transaction_version_id = tv.id
		JOIN journal_entries je ON je.id = pv.journal_entry_id
		WHERE tv.book_id = ? AND tv.change_audit_event_id = ? AND tv.status = 'posted'
		ORDER BY pv.id`, bookID, auditEventID)
	if err != nil {
		return nil, fmt.Errorf("read command postings: %w", err)
	}
	defer rows.Close()
	var deltas []CheckpointDelta
	for rows.Next() {
		var delta CheckpointDelta
		if err := rows.Scan(&delta.AccountID, &delta.CommodityID, &delta.EntryDate, &delta.AccountDaySequence,
			&delta.QuantityValue, &delta.QuantityScale); err != nil {
			return nil, fmt.Errorf("scan command posting: %w", err)
		}
		deltas = append(deltas, delta)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate command postings: %w", err)
	}
	return deltas, nil
}
