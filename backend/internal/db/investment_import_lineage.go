package db

import (
	"context"
	"fmt"
)

// investmentOperationHasImportedLineageQuery includes the source operation
// and every predecessor. A manual replacement of an imported fill remains
// source-linked, so terminal reversal must not treat it as a manual trade.
func investmentOperationHasImportedLineageQuery(ctx context.Context, reader saleOperationReader, bookID, operationID int64) (bool, error) {
	var imported int
	err := reader.QueryRowContext(ctx, `WITH RECURSIVE ancestors(id, parent_id) AS (
		SELECT id, correction_of_operation_id FROM investment_operations
		WHERE book_id = ? AND id = ?
		UNION ALL
		SELECT parent.id, parent.correction_of_operation_id
		FROM investment_operations parent JOIN ancestors ancestor ON parent.id = ancestor.parent_id
		WHERE parent.book_id = ?
	)
	SELECT EXISTS(SELECT 1 FROM ancestors ancestor
		JOIN investment_operations operation ON operation.id = ancestor.id
		JOIN audit_events audit ON audit.id = operation.created_audit_event_id
		WHERE audit.origin_type = 'import' OR EXISTS(
			SELECT 1 FROM import_commit_identity_effects effect WHERE effect.operation_id = ancestor.id))`,
		bookID, operationID, bookID).Scan(&imported)
	if err != nil {
		return false, fmt.Errorf("read imported investment correction lineage: %w", err)
	}
	return imported != 0, nil
}
