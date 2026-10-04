package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"rekenraam/backend/internal/exact"
)

// RegisterCorrectionMemberRecord is one transaction of a correction chain, with
// its current posted postings in the register's account (T-120 #135).
type RegisterCorrectionMemberRecord struct {
	RootTransactionID         int64
	TransactionID             int64
	CorrectionOfTransactionID sql.NullInt64
	// Role is original, reversal or replacement, read from the operation
	// journal link: a replacement command links its inverse as 'reversal' and
	// a pure reversal is an operation of kind 'reversal'.
	Role            string
	TransactionDate string
	Status          string
	Deleted         bool
	Reason          sql.NullString
	Postings        []RegisterCorrectionPostingRecord
}

// RegisterCorrectionPostingRecord is one current posting of a chain member in
// the register's account.
type RegisterCorrectionPostingRecord struct {
	CommodityID   int64
	EntryDate     string
	QuantityValue exact.Coefficient
	QuantityScale int
}

// RegisterCorrectionChains returns every member of each correction chain that
// any of the given transactions belongs to, in creation order per chain. A
// transaction outside any chain returns only itself as its root. The page's
// whole chains are read in one round trip, so a member on another register
// page is still named (linked evidence survives pagination).
func (r *TransactionRepository) RegisterCorrectionChains(ctx context.Context, bookID, accountID int64, transactionIDs []int64) ([]RegisterCorrectionMemberRecord, error) {
	if len(transactionIDs) == 0 {
		return nil, nil
	}
	encoded, err := json.Marshal(transactionIDs)
	if err != nil {
		return nil, fmt.Errorf("encode register transaction IDs: %w", err)
	}
	rows, err := r.database.QueryContext(ctx, `
		WITH RECURSIVE
		up(id, parent) AS (
			SELECT t.id, t.correction_of_transaction_id
			FROM transactions t
			WHERE t.book_id = ? AND t.id IN (SELECT value FROM json_each(?))
			UNION
			SELECT t.id, t.correction_of_transaction_id
			FROM transactions t JOIN up ON t.id = up.parent
		),
		down(id, root) AS (
			SELECT id, id FROM up WHERE parent IS NULL
			UNION
			SELECT t.id, down.root
			FROM transactions t JOIN down ON t.correction_of_transaction_id = down.id
			WHERE t.book_id = ?
		)
		SELECT down.root, t.id, t.correction_of_transaction_id,
			CASE
				WHEN EXISTS (
					SELECT 1 FROM investment_operation_journal_links link
					JOIN investment_operations o ON o.id = link.operation_id
					JOIN transaction_versions v ON v.id = link.transaction_version_id
					WHERE v.transaction_id = t.id
						AND (link.role = 'reversal' OR o.operation_kind = 'reversal')
				) THEN 'reversal'
				WHEN t.correction_of_transaction_id IS NOT NULL THEN 'replacement'
				ELSE 'original'
			END,
			tv.transaction_date, tv.status, t.deleted_at IS NOT NULL,
			(
				SELECT o.correction_reason FROM investment_operation_journal_links link
				JOIN investment_operations o ON o.id = link.operation_id
				JOIN transaction_versions v ON v.id = link.transaction_version_id
				WHERE v.transaction_id = t.id AND o.correction_reason IS NOT NULL
				ORDER BY o.id LIMIT 1
			)
		FROM down
		JOIN transactions t ON t.id = down.id
		JOIN current_transaction_versions tv ON tv.transaction_id = t.id
		ORDER BY down.root, t.id
	`, bookID, string(encoded), bookID)
	if err != nil {
		return nil, fmt.Errorf("read register correction chains: %w", err)
	}
	defer rows.Close()
	var members []RegisterCorrectionMemberRecord
	index := map[int64]int{}
	for rows.Next() {
		var member RegisterCorrectionMemberRecord
		if err := rows.Scan(&member.RootTransactionID, &member.TransactionID, &member.CorrectionOfTransactionID,
			&member.Role, &member.TransactionDate, &member.Status, &member.Deleted, &member.Reason); err != nil {
			return nil, fmt.Errorf("scan register correction member: %w", err)
		}
		index[member.TransactionID] = len(members)
		members = append(members, member)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate register correction members: %w", err)
	}
	if len(members) == 0 {
		return nil, nil
	}

	memberIDs := make([]int64, 0, len(members))
	for _, member := range members {
		memberIDs = append(memberIDs, member.TransactionID)
	}
	encoded, err = json.Marshal(memberIDs)
	if err != nil {
		return nil, fmt.Errorf("encode correction member IDs: %w", err)
	}
	postings, err := r.database.QueryContext(ctx, `
		SELECT tv.transaction_id, pv.commodity_id, je.entry_date, pv.quantity_value, pv.quantity_scale
		FROM current_transaction_versions tv
		JOIN posting_versions pv ON pv.transaction_version_id = tv.id
		JOIN journal_entries je ON je.id = pv.journal_entry_id
		WHERE tv.book_id = ? AND pv.account_id = ?
			AND tv.transaction_id IN (SELECT value FROM json_each(?))
		ORDER BY tv.transaction_id, je.entry_date, pv.id
	`, bookID, accountID, string(encoded))
	if err != nil {
		return nil, fmt.Errorf("read correction member postings: %w", err)
	}
	defer postings.Close()
	for postings.Next() {
		var transactionID int64
		var posting RegisterCorrectionPostingRecord
		if err := postings.Scan(&transactionID, &posting.CommodityID, &posting.EntryDate,
			&posting.QuantityValue, &posting.QuantityScale); err != nil {
			return nil, fmt.Errorf("scan correction member posting: %w", err)
		}
		members[index[transactionID]].Postings = append(members[index[transactionID]].Postings, posting)
	}
	if err := postings.Err(); err != nil {
		return nil, fmt.Errorf("iterate correction member postings: %w", err)
	}
	return members, nil
}
