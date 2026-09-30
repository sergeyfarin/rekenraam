package db

import (
	"context"
	"database/sql"
	"fmt"

	"rekenraam/backend/internal/exact"
)

// Headers and portions are streamed twice: once for each decision, then once
// for each distinct pinned clearing posting. Even empty sets have a header.
type SelfCheckDisposalClearingAllocationRecord struct {
	IsPosting         bool
	EntityID          int64
	IsAllocation      bool
	AmountValue       exact.Coefficient
	AmountScale       int
	ValidRelationship bool
}

func (r *SelfCheckRepository) StreamDisposalClearingAllocations(ctx context.Context, tx *sql.Tx, bookID int64, visit func(SelfCheckDisposalClearingAllocationRecord) error) error {
	rows, err := tx.QueryContext(ctx, `WITH decisions AS (
		SELECT d.* FROM investment_disposal_decisions d
		JOIN investment_operations o ON o.id = d.operation_id AND o.book_id = d.book_id
		WHERE d.book_id = ? AND o.operation_kind IN ('sell', 'write_off')
	), postings AS (
		SELECT DISTINCT pv.id, pv.quantity_value, pv.quantity_scale FROM decisions d
		JOIN posting_versions pv ON pv.book_id = d.book_id
			AND pv.transaction_version_id = d.transaction_version_id AND pv.commodity_id = d.cost_commodity_id
		JOIN accounts a ON a.id = pv.account_id AND a.book_id = d.book_id AND a.system_role = 'commodity_trading'
	)
	SELECT 0 AS is_posting, d.id AS entity_id, 0 AS is_allocation,
		d.proceeds_value AS amount_value, d.proceeds_scale AS amount_scale, 1 AS valid_relationship
	FROM decisions d
	UNION ALL
	SELECT 0, d.id, 1, c.proceeds_value, c.proceeds_scale,
		EXISTS (SELECT 1 FROM posting_versions pv
			JOIN accounts a ON a.id = pv.account_id AND a.book_id = d.book_id AND a.system_role = 'commodity_trading'
			JOIN investment_operation_journal_links l ON l.operation_id = d.operation_id
				AND l.book_id = d.book_id AND l.transaction_version_id = d.transaction_version_id AND l.role <> 'reversal'
			WHERE pv.id = c.posting_version_id AND pv.book_id = d.book_id AND c.book_id = d.book_id
				AND pv.transaction_version_id = d.transaction_version_id AND pv.commodity_id = d.cost_commodity_id)
	FROM decisions d JOIN investment_disposal_clearing_allocations c ON c.decision_id = d.id
	UNION ALL
	SELECT 1, p.id, 0, p.quantity_value, p.quantity_scale, 1 FROM postings p
	UNION ALL
	SELECT 1, p.id, 1, c.proceeds_value, c.proceeds_scale, 1
	FROM postings p JOIN investment_disposal_clearing_allocations c ON c.posting_version_id = p.id
	ORDER BY is_posting, entity_id, is_allocation`, bookID)
	if err != nil {
		return fmt.Errorf("read disposal clearing allocations: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var record SelfCheckDisposalClearingAllocationRecord
		if err := rows.Scan(&record.IsPosting, &record.EntityID, &record.IsAllocation,
			&record.AmountValue, &record.AmountScale, &record.ValidRelationship); err != nil {
			return fmt.Errorf("scan disposal clearing allocation: %w", err)
		}
		if err := visit(record); err != nil {
			return err
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate disposal clearing allocations: %w", err)
	}
	return nil
}
