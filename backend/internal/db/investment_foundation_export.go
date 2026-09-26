package db

import (
	"context"
	"database/sql"
	"fmt"
)

// ExportInvestmentFoundation returns exact stored coefficients and provenance.
// These rows supplement the human-readable lot, price, and import views in bundle v2.
func (r *ExportRepository) ExportInvestmentFoundation(ctx context.Context, tx *sql.Tx, bookID int64, kind string) ([][]string, error) {
	queries := map[string]string{
		"journal-links": `SELECT l.operation_id, l.link_seq, l.transaction_version_id, l.role
			FROM investment_operation_journal_links l WHERE l.book_id = ? ORDER BY l.operation_id, l.link_seq`,
		"dates": `SELECT d.operation_id, d.date_role, d.event_date
			FROM investment_operation_dates d JOIN investment_operations o ON o.id = d.operation_id
			WHERE o.book_id = ? ORDER BY d.operation_id, d.date_role`,
		"components": `SELECT c.id, c.operation_id, c.component_seq, c.component_kind,
			c.commodity_id, c.amount_value, c.amount_scale, c.amount_date, c.gross_unknown,
			c.charge_treatment, c.charge_account_id, c.resolution_tier,
			c.fee_policy_version_id, c.source_evidence_json, c.created_audit_event_id
			FROM investment_operation_components c WHERE c.book_id = ? ORDER BY c.operation_id, c.component_seq`,
		"lot-facts": `SELECT f.lot_id, f.operation_id, f.account_id, f.commodity_id,
			f.position_side, f.opened_on, f.quantity_value, f.quantity_scale,
			f.consideration_value, f.consideration_scale, f.cost_commodity_id, f.created_audit_event_id
			FROM investment_lot_facts f WHERE f.book_id = ? ORDER BY f.lot_id`,
		"lot-events": `SELECT e.id, e.lot_id, e.event_kind, e.transaction_id, e.event_date,
			e.quantity_value, e.quantity_scale, e.cost_basis_value, e.cost_basis_scale,
			e.cost_basis_method, e.metadata_json, e.created_audit_event_id
			FROM investment_lot_events e WHERE e.book_id = ? ORDER BY e.id`,
		"lot-effects": `SELECT e.operation_id, e.effect_seq, e.lot_event_id
			FROM investment_operation_lot_effects e JOIN investment_operations o ON o.id = e.operation_id
			WHERE o.book_id = ? ORDER BY e.operation_id, e.effect_seq`,
		"fee-policies": `SELECT p.id, p.account_id, p.charge_kind, p.created_at, p.created_audit_event_id
			FROM investment_fee_policies p WHERE p.book_id = ? ORDER BY p.id`,
		"fee-policy-versions": `SELECT v.id, v.policy_id, v.version_seq, v.effective_from,
			v.treatment, v.charge_account_id, v.recorded_at, v.audit_event_id
			FROM investment_fee_policy_versions v JOIN investment_fee_policies p ON p.id = v.policy_id
			WHERE p.book_id = ? ORDER BY v.policy_id, v.version_seq`,
		"import-identities": `SELECT id, dedupe_fingerprint, source_kind, account_id, created_at
			FROM import_commit_identities WHERE book_id = ? ORDER BY id`,
		"import-effects": `SELECT e.identity_id, e.effect_seq, e.operation_id, e.transaction_id
			FROM import_commit_identity_effects e JOIN import_commit_identities i ON i.id = e.identity_id
			WHERE i.book_id = ? ORDER BY e.identity_id, e.effect_seq`,
	}
	query, ok := queries[kind]
	if !ok {
		return nil, fmt.Errorf("unknown investment foundation export %q", kind)
	}
	rows, err := tx.QueryContext(ctx, query, bookID)
	if err != nil {
		return nil, fmt.Errorf("read investment %s export: %w", kind, err)
	}
	defer rows.Close()
	columns, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("read investment %s columns: %w", kind, err)
	}
	var result [][]string
	for rows.Next() {
		values := make([]sql.NullString, len(columns))
		dest := make([]any, len(columns))
		for i := range values {
			dest[i] = &values[i]
		}
		if err := rows.Scan(dest...); err != nil {
			return nil, fmt.Errorf("scan investment %s export: %w", kind, err)
		}
		record := make([]string, len(columns))
		for i, value := range values {
			if value.Valid {
				record[i] = value.String
			}
		}
		result = append(result, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate investment %s export: %w", kind, err)
	}
	return result, nil
}
