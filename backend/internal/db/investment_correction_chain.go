package db

import (
	"context"
	"database/sql"
	"fmt"
)

// InvestmentCorrectionNodeRecord is one immutable operation in a linear
// correction chain. The operation row, not a mutable transaction status,
// determines which historical intent is currently effective.
type InvestmentCorrectionNodeRecord struct {
	OperationID             int64
	TransactionID           sql.NullInt64
	OperationKind           string
	EventDate               string
	CorrectionOfOperationID sql.NullInt64
	CorrectionMode          sql.NullString
	CorrectionReason        sql.NullString
	CreatedAt               string
	AuditEventID            int64
	Imported                bool
	TransactionStatus       sql.NullString
	TransactionDeleted      bool
}

func (r *InvestmentRepository) CorrectionChainByTransactionID(ctx context.Context, bookID, transactionID int64) ([]InvestmentCorrectionNodeRecord, error) {
	rows, err := r.database.QueryContext(ctx, `WITH RECURSIVE ancestors(id, parent_id) AS (
		SELECT DISTINCT operation.id, operation.correction_of_operation_id
		FROM investment_operation_journal_links link
		JOIN investment_operations operation ON operation.id = link.operation_id
		JOIN transaction_versions version ON version.id = link.transaction_version_id
		WHERE operation.book_id = ? AND link.book_id = operation.book_id AND version.transaction_id = ?
		UNION ALL
		SELECT parent.id, parent.correction_of_operation_id
		FROM investment_operations parent JOIN ancestors a ON parent.id = a.parent_id
		WHERE parent.book_id = ?
	), root AS (SELECT id FROM ancestors WHERE parent_id IS NULL),
	chain(id, chain_seq) AS (
		SELECT id, 1 FROM root
		UNION ALL
		SELECT successor.id, chain.chain_seq + 1
		FROM investment_operations successor JOIN chain ON successor.correction_of_operation_id = chain.id
		WHERE successor.book_id = ?
	)
	SELECT operation.id, linked_version.transaction_id, operation.operation_kind,
		operation.event_date, operation.correction_of_operation_id,
		operation.correction_mode, operation.correction_reason,
		operation.created_at, operation.created_audit_event_id,
		(audit.origin_type = 'import' OR EXISTS (
			SELECT 1 FROM import_commit_identity_effects effect
			WHERE effect.operation_id = operation.id)),
		current.status, (transaction_record.deleted_at IS NOT NULL)
	FROM chain JOIN investment_operations operation ON operation.id = chain.id
	JOIN audit_events audit ON audit.id = operation.created_audit_event_id
	LEFT JOIN investment_operation_journal_links primary_link ON primary_link.operation_id = operation.id
		AND primary_link.book_id = operation.book_id AND primary_link.role = 'primary'
		AND primary_link.link_seq = (SELECT MIN(link_seq) FROM investment_operation_journal_links
			WHERE operation_id = operation.id AND role = 'primary')
	LEFT JOIN transaction_versions linked_version ON linked_version.id = primary_link.transaction_version_id
	LEFT JOIN transactions transaction_record ON transaction_record.id = linked_version.transaction_id
	LEFT JOIN current_transaction_versions current ON current.transaction_id = linked_version.transaction_id
		ORDER BY chain.chain_seq`, bookID, transactionID, bookID, bookID)
	if err != nil {
		return nil, fmt.Errorf("read investment correction chain: %w", err)
	}
	defer rows.Close()
	var nodes []InvestmentCorrectionNodeRecord
	for rows.Next() {
		var node InvestmentCorrectionNodeRecord
		var imported, deleted int
		if err := rows.Scan(&node.OperationID, &node.TransactionID, &node.OperationKind,
			&node.EventDate, &node.CorrectionOfOperationID, &node.CorrectionMode,
			&node.CorrectionReason, &node.CreatedAt, &node.AuditEventID,
			&imported, &node.TransactionStatus, &deleted); err != nil {
			return nil, fmt.Errorf("scan investment correction chain: %w", err)
		}
		node.Imported = imported != 0
		node.TransactionDeleted = deleted != 0
		nodes = append(nodes, node)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate investment correction chain: %w", err)
	}
	if len(nodes) == 0 {
		return nil, ErrNotFound
	}
	return nodes, nil
}
