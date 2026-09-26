package db

import (
	"context"
	"database/sql"
	"fmt"
)

// recordInvestmentFoundationTx attaches the posted version and immutable
// source facts to the operation created by createTransactionWithAuditTx. It
// also records the trade price with the command's audit event, before commit.
func recordInvestmentFoundationTx(ctx context.Context, tx *sql.Tx, params CreateTransactionParams, transaction TransactionRecord, auditEventID int64) error {
	var operationID int64
	if err := tx.QueryRowContext(ctx, `
		SELECT id FROM investment_operations WHERE book_id = ? AND transaction_id = ?
	`, params.BookID, transaction.ID).Scan(&operationID); err != nil {
		return fmt.Errorf("read investment operation for journal link: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO investment_operation_journal_links
			(book_id, operation_id, transaction_version_id, link_seq, role)
		VALUES (?, ?, ?, 1, 'primary')
	`, params.BookID, operationID, transaction.VersionID); err != nil {
		return fmt.Errorf("link investment operation to posted version: %w", err)
	}
	dateRole := "trade"
	if params.Spec.InvestmentOperationKind == "dividend" || params.Spec.InvestmentOperationKind == "reinvested_dividend" {
		dateRole = "payment"
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO investment_operation_dates (operation_id, date_role, event_date)
		VALUES (?, ?, ?)
	`, operationID, dateRole, params.Spec.TransactionDate); err != nil {
		return fmt.Errorf("record investment operation date: %w", err)
	}
	if params.Spec.InvestmentOperationKind == "buy" || params.Spec.InvestmentOperationKind == "sell" {
		// Current input has only one date; retain the explicit settlement slot
		// until slice 3 admits a separately sourced broker settlement date.
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO investment_operation_dates (operation_id, date_role, event_date)
			VALUES (?, 'settlement', ?)
		`, operationID, params.Spec.TransactionDate); err != nil {
			return fmt.Errorf("record trade settlement date: %w", err)
		}
	}
	for i, component := range params.InvestmentComponents {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO investment_operation_components
				(book_id, operation_id, component_seq, component_kind, commodity_id,
				 amount_value, amount_scale, amount_date, gross_unknown, created_audit_event_id)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, params.BookID, operationID, i+1, component.Kind, component.CommodityID,
			component.AmountValue, component.AmountScale, component.AmountDate,
			boolInt(component.GrossUnknown), auditEventID); err != nil {
			return fmt.Errorf("record investment source component: %w", err)
		}
	}
	if price := params.TradeImpliedPrice; price != nil {
		if err := recordTradeImpliedPriceTx(ctx, tx, params, transaction.VersionID, auditEventID, *price); err != nil {
			return err
		}
	}
	return nil
}

func recordTradeImpliedPriceTx(ctx context.Context, tx *sql.Tx, params CreateTransactionParams, versionID int64, auditEventID int64, price TradeImpliedPriceSpec) error {
	var sourceID int64
	if err := tx.QueryRowContext(ctx, `SELECT id FROM market_data_sources WHERE code = 'manual'`).Scan(&sourceID); err != nil {
		return fmt.Errorf("read trade price source: %w", err)
	}
	series, err := ensurePriceSeriesTx(ctx, tx, params.BookID, PriceSeriesSpec{
		BaseCommodityID: price.BaseCommodityID, QuoteCommodityID: price.QuoteCommodityID,
		SourceID:  sql.NullInt64{Int64: sourceID, Valid: true},
		QuoteType: "trade_implied", AdjustmentBasis: "not_applicable", MetadataJSON: "{}",
	}, params.ActorUserID, params.CreatedAt, auditEventID)
	if err != nil {
		return fmt.Errorf("ensure trade price series: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO price_observations (
			book_id, series_id, base_commodity_id, quote_commodity_id, quote_type,
			adjustment_basis, price_value, price_scale, base_quantity_value,
			base_quantity_scale, valuation_date, source_id, is_manual, is_derived,
			is_approximate, source_transaction_version_id, derivation_json,
			metadata_json, recorded_at, created_by_user_id, created_audit_event_id
		) VALUES (?, ?, ?, ?, 'trade_implied', 'not_applicable', ?, ?, 1, 0,
			?, ?, 0, 1, ?, ?, '{"kind":"trade_transaction_version"}', '{}', ?, ?, ?)
	`, params.BookID, series.ID, price.BaseCommodityID, price.QuoteCommodityID,
		price.PriceValue, price.PriceScale, price.ValuationDate, sourceID,
		boolInt(price.Approximate), versionID, params.CreatedAt, params.ActorUserID, auditEventID); err != nil {
		return fmt.Errorf("record trade-implied price: %w", err)
	}
	return nil
}

func investmentOperationIDTx(ctx context.Context, tx *sql.Tx, bookID, transactionID int64) (int64, error) {
	var id int64
	if err := tx.QueryRowContext(ctx, `
		SELECT id FROM investment_operations WHERE book_id = ? AND transaction_id = ?
	`, bookID, transactionID).Scan(&id); err != nil {
		return 0, fmt.Errorf("read investment operation: %w", err)
	}
	return id, nil
}

func linkLotEffectTx(ctx context.Context, tx *sql.Tx, operationID, eventID int64) error {
	var nextSeq int64
	if err := tx.QueryRowContext(ctx, `
		SELECT COALESCE(MAX(effect_seq), 0) + 1 FROM investment_operation_lot_effects
		WHERE operation_id = ?
	`, operationID).Scan(&nextSeq); err != nil {
		return fmt.Errorf("read next investment effect sequence: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO investment_operation_lot_effects (operation_id, lot_event_id, effect_seq)
		VALUES (?, ?, ?)
	`, operationID, eventID, nextSeq); err != nil {
		return fmt.Errorf("link investment lot effect: %w", err)
	}
	return nil
}
