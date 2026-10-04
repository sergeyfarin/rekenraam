package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// InvestmentTradeCorrectionContext is an immutable source snapshot for
// preparing a full buy or sale replacement. A command always rechecks the
// source under its write transaction; this read is never an authorization.
type InvestmentTradeCorrectionContext struct {
	OperationID          int64
	TransactionID        int64
	OperationKind        string
	EventDate            string
	HoldingAccountID     int64
	CommodityID          int64
	CommodityCode        string
	CostCommodityID      int64
	QuantityValue        string
	QuantityScale        int
	CostBasisMethod      string
	CashAccountID        int64
	NetValue             string
	NetScale             int
	SettlementDate       string
	Memo                 string
	PayeeID              *int64
	GrossValue           *string
	GrossScale           *int
	Imported             bool
	SourceIdentityID     int64
	SourceKind           string
	AlreadyCorrected     bool
	Charges              []InvestmentTradeCorrectionCharge
	ElectedLots          []InvestmentTradeCorrectionLotChoice
	CanReplaceSale       bool
	AvailableLots        []InvestmentTradeCorrectionAvailableLot
	EffectiveElectedLots []InvestmentTradeCorrectionLotChoice
}

type InvestmentTradeCorrectionAvailableLot struct {
	LotID         int64
	OpenedOn      string
	QuantityValue string
	QuantityScale int
}

type InvestmentTradeCorrectionLotChoice struct {
	LotID         int64
	QuantityValue string
	QuantityScale int
}

type InvestmentTradeCorrectionCharge struct {
	Kind            string
	AmountValue     string
	AmountScale     int
	CommodityID     int64
	Treatment       string
	ChargeAccountID *int64
	CashAccountID   *int64
	PaidOn          string
}

func (r *InvestmentRepository) TradeCorrectionContext(ctx context.Context, bookID, transactionID int64) (InvestmentTradeCorrectionContext, error) {
	tx, err := r.database.BeginTx(ctx, nil)
	if err != nil {
		return InvestmentTradeCorrectionContext{}, fmt.Errorf("begin investment trade correction snapshot: %w", err)
	}
	defer rollbackTx(ctx, tx)
	var record InvestmentTradeCorrectionContext
	var grossValue sql.NullString
	var grossScale sql.NullInt64
	var payeeID sql.NullInt64
	var imported, corrected int
	err = tx.QueryRowContext(ctx, `
		WITH RECURSIVE source_lineage(id, parent_id, depth) AS (
			SELECT source.id, source.correction_of_operation_id, 0
			FROM investment_operations source
			JOIN investment_operation_journal_links source_link ON source_link.operation_id = source.id
				AND source_link.book_id = source.book_id AND source_link.role = 'primary'
			JOIN transaction_versions source_version ON source_version.id = source_link.transaction_version_id
			WHERE source.book_id = ? AND source_version.transaction_id = ?
			UNION ALL
			SELECT parent.id, parent.correction_of_operation_id, lineage.depth + 1
			FROM investment_operations parent JOIN source_lineage lineage ON parent.id = lineage.parent_id
			WHERE parent.book_id = ?
		)
		SELECT o.id, linked_version.transaction_id, o.operation_kind, o.event_date,
			COALESCE(f.account_id, d.account_id), COALESCE(f.commodity_id, d.commodity_id),
			c.code, COALESCE(f.cost_commodity_id, d.cost_commodity_id),
			COALESCE(f.quantity_value, d.quantity_value),
			COALESCE(f.quantity_scale, d.quantity_scale), COALESCE(d.cost_basis_method, ''),
			-- A write-off has no cash leg (T-118): zero net, no cash account.
			COALESCE(net.cash_account_id, 0), COALESCE(net.amount_value, '0'), COALESCE(net.amount_scale, 0),
			COALESCE(net.amount_date, o.event_date),
			version.description, version.payee_id,
			gross.amount_value, gross.amount_scale,
			(audit.origin_type = 'import' OR EXISTS(SELECT 1 FROM import_commit_identity_effects effect
				WHERE effect.operation_id = o.id)),
			COALESCE((SELECT effect.identity_id FROM import_commit_identity_effects effect
				JOIN source_lineage lineage ON lineage.id = effect.operation_id
				ORDER BY lineage.depth, effect.effect_seq LIMIT 1), 0),
			COALESCE((SELECT identity.source_kind FROM import_commit_identity_effects effect
				JOIN import_commit_identities identity ON identity.id = effect.identity_id
				JOIN source_lineage lineage ON lineage.id = effect.operation_id
				ORDER BY lineage.depth, effect.effect_seq LIMIT 1), ''),
			EXISTS(SELECT 1 FROM investment_operations successor WHERE successor.correction_of_operation_id = o.id)
		FROM investment_operations o
		JOIN audit_events audit ON audit.id = o.created_audit_event_id
		JOIN investment_operation_journal_links link ON link.operation_id = o.id AND link.book_id = o.book_id AND link.role = 'primary'
		JOIN transaction_versions linked_version ON linked_version.id = link.transaction_version_id
		JOIN current_transaction_versions version ON version.transaction_id = linked_version.transaction_id
		LEFT JOIN investment_operation_components net ON net.operation_id = o.id AND net.component_kind = 'net_settlement'
			AND net.component_seq = (SELECT MIN(component_seq) FROM investment_operation_components
				WHERE operation_id = o.id AND component_kind = 'net_settlement')
		LEFT JOIN investment_operation_components gross ON gross.operation_id = o.id AND gross.component_kind = 'gross_consideration'
		LEFT JOIN investment_lots f ON f.operation_id = o.id AND f.position_side = 'long'
		LEFT JOIN investment_disposal_decisions d ON d.operation_id = o.id AND d.position_side = 'long'
		JOIN commodities c ON c.id = COALESCE(f.commodity_id, d.commodity_id)
		WHERE o.book_id = ? AND linked_version.transaction_id = ? AND o.operation_kind IN ('buy', 'sell', 'write_off')
			AND (SELECT count(*) FROM investment_lots WHERE operation_id = o.id) <= 1
			AND (SELECT count(*) FROM investment_disposal_decisions WHERE operation_id = o.id) <= 1
	`, bookID, transactionID, bookID, bookID, transactionID).Scan(&record.OperationID, &record.TransactionID,
		&record.OperationKind, &record.EventDate, &record.HoldingAccountID,
		&record.CommodityID, &record.CommodityCode, &record.CostCommodityID,
		&record.QuantityValue, &record.QuantityScale, &record.CostBasisMethod,
		&record.CashAccountID, &record.NetValue, &record.NetScale,
		&record.SettlementDate, &record.Memo, &payeeID,
		&grossValue, &grossScale, &imported, &record.SourceIdentityID,
		&record.SourceKind, &corrected)
	if errors.Is(err, sql.ErrNoRows) {
		return InvestmentTradeCorrectionContext{}, ErrNotFound
	}
	if err != nil {
		return InvestmentTradeCorrectionContext{}, fmt.Errorf("read investment trade correction context: %w", err)
	}
	if grossValue.Valid && grossScale.Valid {
		value, scale := grossValue.String, int(grossScale.Int64)
		record.GrossValue, record.GrossScale = &value, &scale
	}
	if payeeID.Valid {
		record.PayeeID = &payeeID.Int64
	}
	record.Imported, record.AlreadyCorrected = imported != 0, corrected != 0
	rows, err := tx.QueryContext(ctx, `SELECT charge_kind, amount_value, amount_scale,
		commodity_id, charge_treatment, charge_account_id, cash_account_id, amount_date
		FROM investment_operation_components
		WHERE operation_id = ? AND component_kind = 'charge' ORDER BY component_seq`, record.OperationID)
	if err != nil {
		return InvestmentTradeCorrectionContext{}, fmt.Errorf("read investment trade correction charges: %w", err)
	}
	for rows.Next() {
		var charge InvestmentTradeCorrectionCharge
		var chargeAccountID, cashAccountID sql.NullInt64
		if err := rows.Scan(&charge.Kind, &charge.AmountValue, &charge.AmountScale,
			&charge.CommodityID, &charge.Treatment, &chargeAccountID, &cashAccountID,
			&charge.PaidOn); err != nil {
			rows.Close()
			return InvestmentTradeCorrectionContext{}, fmt.Errorf("scan investment trade correction charge: %w", err)
		}
		if chargeAccountID.Valid {
			charge.ChargeAccountID = &chargeAccountID.Int64
		}
		if cashAccountID.Valid {
			charge.CashAccountID = &cashAccountID.Int64
		}
		record.Charges = append(record.Charges, charge)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return InvestmentTradeCorrectionContext{}, fmt.Errorf("iterate investment trade correction charges: %w", err)
	}
	if err := rows.Close(); err != nil {
		return InvestmentTradeCorrectionContext{}, fmt.Errorf("close investment trade correction charges: %w", err)
	}
	if record.CostBasisMethod == "specific_lot" {
		lotRows, err := tx.QueryContext(ctx, `SELECT a.lot_id, a.quantity_value, a.quantity_scale
			FROM investment_disposal_allocations a
			JOIN investment_disposal_decisions d ON d.id = a.decision_id
			WHERE d.operation_id = ? ORDER BY a.allocation_seq`, record.OperationID)
		if err != nil {
			return InvestmentTradeCorrectionContext{}, fmt.Errorf("read correction elected lots: %w", err)
		}
		for lotRows.Next() {
			var choice InvestmentTradeCorrectionLotChoice
			if err := lotRows.Scan(&choice.LotID, &choice.QuantityValue, &choice.QuantityScale); err != nil {
				lotRows.Close()
				return InvestmentTradeCorrectionContext{}, fmt.Errorf("scan correction elected lot: %w", err)
			}
			record.ElectedLots = append(record.ElectedLots, choice)
		}
		if err := lotRows.Err(); err != nil {
			lotRows.Close()
			return InvestmentTradeCorrectionContext{}, fmt.Errorf("iterate correction elected lots: %w", err)
		}
		if err := lotRows.Close(); err != nil {
			return InvestmentTradeCorrectionContext{}, fmt.Errorf("close correction elected lots: %w", err)
		}
	}
	if (record.OperationKind == "sell" || record.OperationKind == "write_off") && (!record.Imported || record.SourceIdentityID > 0) && !record.AlreadyCorrected {
		intents, err := investmentReplayIntentsQuery(ctx, tx, bookID,
			record.HoldingAccountID, record.CommodityID, record.CostCommodityID, "long")
		if err != nil {
			return InvestmentTradeCorrectionContext{}, err
		}
		saleIndex := -1
		for index, intent := range intents {
			if intent.OperationID == record.OperationID && intent.Kind == "disposal" {
				saleIndex = index
				break
			}
		}
		if saleIndex >= 0 {
			// The write rechecks the source and dependent replay atomically.
			// Historical lots must not be inferred from today's projection.
			record.CanReplaceSale = true
			for _, choice := range intents[saleIndex].SpecificLots {
				record.EffectiveElectedLots = append(record.EffectiveElectedLots, InvestmentTradeCorrectionLotChoice{
					LotID: choice.LotID, QuantityValue: choice.QuantityValue.String(), QuantityScale: choice.QuantityScale,
				})
			}
			projection, err := simulateInvestmentReplayTx(ctx, tx, bookID,
				record.HoldingAccountID, record.CommodityID, record.CostCommodityID, intents[:saleIndex])
			if err != nil {
				return InvestmentTradeCorrectionContext{}, fmt.Errorf("read pre-sale available lots: %w", err)
			}
			openedOn := make(map[int64]string)
			for _, intent := range intents[:saleIndex] {
				if intent.Kind == "opening" {
					openedOn[intent.LotID] = intent.EventDate
				}
			}
			for _, lot := range projection.Lots {
				if lot.RemainingQuantityValue.Sign() > 0 && openedOn[lot.LotID] != "" {
					record.AvailableLots = append(record.AvailableLots, InvestmentTradeCorrectionAvailableLot{
						LotID: lot.LotID, OpenedOn: openedOn[lot.LotID],
						QuantityValue: lot.RemainingQuantityValue.String(), QuantityScale: lot.RemainingQuantityScale,
					})
				}
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return InvestmentTradeCorrectionContext{}, fmt.Errorf("close investment trade correction snapshot: %w", err)
	}
	return record, nil
}
