package db

import (
	"context"
	"database/sql"
	"fmt"
)

// ExportInvestmentFoundation returns exact stored coefficients and provenance.
// These rows supplement the human-readable lot, price, and import bundle views.
func (r *ExportRepository) ExportInvestmentFoundation(ctx context.Context, tx *sql.Tx, bookID int64, kind string) ([][]string, error) {
	queries := map[string]string{
		"disposal-clearing-allocations": `SELECT decision_id, posting_version_id, proceeds_value, proceeds_scale
			FROM investment_disposal_clearing_allocations WHERE book_id = ? ORDER BY decision_id, posting_version_id`,
		"journal-links": `SELECT l.operation_id, l.link_seq, l.transaction_version_id, l.role
			FROM investment_operation_journal_links l WHERE l.book_id = ? ORDER BY l.operation_id, l.link_seq`,
		"dates": `SELECT d.operation_id, d.date_role, d.event_date
			FROM investment_operation_dates d JOIN investment_operations o ON o.id = d.operation_id
			WHERE o.book_id = ? ORDER BY d.operation_id, d.date_role`,
		"components": `SELECT c.id, c.operation_id, c.component_seq, c.component_kind,
			c.commodity_id, c.amount_value, c.amount_scale, c.amount_date, c.gross_unknown,
			c.charge_treatment, c.charge_account_id, c.resolution_tier,
			c.fee_policy_version_id, c.source_evidence_json, c.created_audit_event_id, c.charge_kind, c.cash_account_id,
			c.separately_paid, c.posting_version_id
			FROM investment_operation_components c WHERE c.book_id = ? ORDER BY c.operation_id, c.component_seq`,
		"lot-state": `SELECT lot_id, status, remaining_quantity_value, remaining_quantity_scale,
		remaining_cost_basis_value, remaining_cost_basis_scale, updated_at, updated_by_user_id, updated_audit_event_id, basis_knowledge
		FROM investment_lot_state WHERE book_id = ? ORDER BY lot_id`,
		// Opening facts live on the operation-linked lot row itself (T-124);
		// original knowledge remains separate from the current projection.
		"lot-facts": `SELECT l.id, l.operation_id, l.account_id, l.commodity_id,
			l.position_side, l.opened_on, l.quantity_value, l.quantity_scale,
			l.cost_basis_value, l.cost_basis_scale, l.cost_commodity_id, l.created_audit_event_id, l.opening_basis_knowledge
			FROM investment_lots l WHERE l.book_id = ? AND l.operation_id IS NOT NULL ORDER BY l.id`,
		"lot-events": `SELECT e.id, e.lot_id, e.event_kind, e.transaction_id, e.event_date,
			e.quantity_value, e.quantity_scale, e.cost_basis_value, e.cost_basis_scale,
			e.cost_basis_method, e.metadata_json, e.created_audit_event_id, e.basis_knowledge
			FROM investment_lot_events e WHERE e.book_id = ? ORDER BY e.id`,
		"lot-effects": `SELECT e.operation_id, e.effect_seq, e.lot_event_id
			FROM investment_operation_lot_effects e JOIN investment_operations o ON o.id = e.operation_id
			WHERE o.book_id = ? ORDER BY e.operation_id, e.effect_seq`,
		"transfer-facts": `SELECT f.operation_id, f.transfer_kind, f.effective_on, f.commodity_id,
			f.source_account_id, f.destination_account_id, f.source_evidence_json, f.created_audit_event_id,
			f.basis_allocation, f.cost_basis_method, f.method_resolution_tier,
			f.method_account_version_id, f.method_profile_version_id, f.destination_lineage
			FROM investment_transfer_facts f WHERE f.book_id = ? ORDER BY f.operation_id`,
		"transfer-lot-links": `SELECT l.operation_id, l.link_seq, l.source_lot_id, l.destination_lot_id,
			l.quantity_value, l.quantity_scale, l.basis_knowledge, l.carried_basis_value,
			l.carried_basis_scale, l.cost_commodity_id, l.original_date_knowledge,
			l.original_acquired_on, l.source_evidence_json
			FROM investment_transfer_lot_links l JOIN investment_transfer_facts f ON f.operation_id = l.operation_id
			WHERE f.book_id = ? ORDER BY l.operation_id, l.link_seq`,
		"transfer-link-revisions": `SELECT r.id, r.operation_id, r.link_seq, r.revision_seq,
			r.caused_by_operation_id, r.supersedes_revision_id, r.source_lot_id, r.carried_basis_value,
			r.carried_basis_scale, r.created_at, r.created_audit_event_id,
			r.original_date_knowledge, r.original_acquired_on, r.basis_knowledge
			FROM investment_transfer_link_revisions r WHERE r.book_id = ?
			ORDER BY r.operation_id, r.link_seq, r.revision_seq`,
		"transfer-link-revision-depletions": `SELECT d.revision_id, d.depletion_seq, d.source_lot_id,
			d.quantity_value, d.quantity_scale, d.cost_basis_value, d.cost_basis_scale, d.basis_knowledge
			FROM investment_transfer_link_revision_depletions d WHERE d.book_id = ?
			ORDER BY d.revision_id, d.depletion_seq`,
		"split-facts": `SELECT f.operation_id, f.account_id, f.commodity_id, f.effective_on,
			f.ratio_numerator, f.ratio_denominator, f.source_evidence_json, f.created_audit_event_id
			FROM investment_split_facts f WHERE f.book_id = ? ORDER BY f.operation_id`,
		"split-revisions": `SELECT r.id, r.operation_id, r.cost_commodity_id, r.revision_seq,
			r.caused_by_operation_id, r.supersedes_revision_id, r.created_at, r.created_audit_event_id,
			r.adjustment_transaction_version_id
			FROM investment_split_revisions r WHERE r.book_id = ?
			ORDER BY r.operation_id, r.cost_commodity_id, r.revision_seq`,
		"split-revision-effects": `SELECT e.revision_id, e.effect_seq, e.lot_id,
			e.quantity_delta_value, e.quantity_delta_scale
			FROM investment_split_revision_effects e WHERE e.book_id = ?
			ORDER BY e.revision_id, e.effect_seq`,
		"cash-in-lieu-facts": `SELECT f.operation_id, f.split_operation_id, f.payment_on, f.created_audit_event_id
			FROM investment_cash_in_lieu_facts f WHERE f.book_id = ? ORDER BY f.operation_id`,
		"capital-return-facts": `SELECT f.operation_id, f.account_id, f.commodity_id, f.cost_commodity_id,
			f.cash_account_id, f.effective_on, f.payment_on, f.amount_value, f.amount_scale, f.entitlement_rule,
			f.source_evidence_json, f.created_audit_event_id
			FROM investment_capital_return_facts f WHERE f.book_id = ? ORDER BY f.operation_id`,
		"capital-return-entitlements": `SELECT e.operation_id, e.entitlement_seq, e.lot_id, e.quantity_value, e.quantity_scale
			FROM investment_capital_return_entitlements e WHERE e.book_id = ? ORDER BY e.operation_id, e.entitlement_seq`,
		"capital-return-revisions": `SELECT r.id, r.operation_id, r.revision_seq, r.caused_by_operation_id,
			r.supersedes_revision_id, r.created_at, r.created_audit_event_id
			FROM investment_capital_return_revisions r WHERE r.book_id = ? ORDER BY r.operation_id, r.revision_seq`,
		"capital-return-revision-effects": `SELECT e.revision_id, e.effect_seq, e.lot_id,
			e.entitled_quantity_value, e.entitled_quantity_scale, e.allocated_value, e.allocated_scale,
			e.reduction_value, e.reduction_scale, e.excess_value, e.excess_scale
			FROM investment_capital_return_revision_effects e WHERE e.book_id = ? ORDER BY e.revision_id, e.effect_seq`,
		"capital-return-effects": `SELECT e.operation_id, e.effect_seq, e.lot_id, e.lot_event_id,
			e.entitled_quantity_value, e.entitled_quantity_scale, e.allocated_value, e.allocated_scale,
			e.reduction_value, e.reduction_scale, e.excess_value, e.excess_scale
			FROM investment_capital_return_effects e WHERE e.book_id = ? ORDER BY e.operation_id, e.effect_seq`,
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
		"disposal-revisions": `SELECT r.id, r.decision_id, r.revision_seq, r.caused_by_operation_id,
			r.supersedes_revision_id, r.disposed_basis_value, r.disposed_basis_scale,
			r.created_at, r.created_audit_event_id, r.basis_knowledge
			FROM investment_disposal_revisions r WHERE r.book_id = ?
			ORDER BY r.decision_id, r.revision_seq`,
		"disposal-revision-allocations": `SELECT a.revision_id, a.allocation_seq, a.lot_id,
			a.quantity_value, a.quantity_scale, a.cost_basis_value, a.cost_basis_scale,
			a.proceeds_value, a.proceeds_scale, a.basis_knowledge
			FROM investment_disposal_revision_allocations a WHERE a.book_id = ?
			ORDER BY a.revision_id, a.allocation_seq`,
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
