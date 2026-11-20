package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

type MassiveSplitTarget struct {
	InstrumentID, CommodityID, SourceID int64
	Symbol, MIC                         string
}

func (r *PricingRepository) MassiveSplitTargets(ctx context.Context, bookID, instrumentID int64) ([]MassiveSplitTarget, error) {
	return massiveSplitTargets(ctx, r.database, bookID, instrumentID)
}

func massiveSplitTargets(ctx context.Context, reader queryer, bookID, instrumentID int64) ([]MassiveSplitTarget, error) {
	rows, err := reader.QueryContext(ctx, `
		SELECT ii.id, ii.commodity_id, source.id, json_extract(iiv.identifiers_json, '$.massive_symbol'), COALESCE(iiv.mic, '')
		FROM investment_instruments ii
		JOIN `+CurrentReferenceVersions(ctx, "investment_instrument")+` iiv ON iiv.instrument_id = ii.id
		JOIN market_data_sources source ON source.code = 'massive' AND source.status = 'active'
		WHERE ii.book_id = ? AND (? = 0 OR ii.id = ?) AND iiv.status = 'active' AND iiv.instrument_type <> 'crypto'
			AND json_type(iiv.identifiers_json, '$.massive_symbol') = 'text'
			AND json_extract(iiv.identifiers_json, '$.massive_symbol') <> ''
		ORDER BY ii.id`, bookID, instrumentID, instrumentID)
	if err != nil {
		return nil, fmt.Errorf("read Massive split assignments: %w", err)
	}
	defer rows.Close()
	var targets []MassiveSplitTarget
	for rows.Next() {
		var target MassiveSplitTarget
		if err := rows.Scan(&target.InstrumentID, &target.CommodityID, &target.SourceID, &target.Symbol, &target.MIC); err != nil {
			return nil, fmt.Errorf("read Massive split assignment: %w", err)
		}
		targets = append(targets, target)
	}
	return targets, rows.Err()
}

var (
	ErrSplitSuggestionNeedsReview = errors.New("provider split requires manual review")
	ErrSplitEventLinkTarget       = errors.New("recorded split cannot be linked to this provider event")
)

func (r *PricingRepository) ProviderSplitHoldingName(ctx context.Context, bookID, accountID int64) (string, error) {
	var name string
	err := r.database.QueryRowContext(ctx, `SELECT COALESCE(
		(SELECT a.name FROM `+CurrentReferenceVersions(ctx, "account")+` a
		 JOIN accounts head ON head.id = a.account_id WHERE head.book_id = ? AND a.account_id = ?), '')`,
		bookID, accountID).Scan(&name)
	if err != nil {
		return "", fmt.Errorf("read provider split holding name: %w", err)
	}
	return name, nil
}

// ProviderSplitHasSameDayActivity guards ambiguous slot ordering and prevents
// posting the same event after an owner has already recorded a split.
func (r *PricingRepository) ProviderSplitHasSameDayActivity(ctx context.Context, bookID, commodityID int64, date string) (bool, error) {
	return providerSplitHasSameDayActivity(ctx, r.database, bookID, commodityID, date)
}

func providerSplitHasSameDayActivity(ctx context.Context, reader queryRowContexter, bookID, commodityID int64, date string) (bool, error) {
	var found bool
	err := reader.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM effective_investment_operations o
			JOIN investment_split_facts f ON f.operation_id = o.id
			WHERE f.book_id = ? AND f.commodity_id = ? AND f.effective_on = ?
			UNION ALL
			SELECT 1 FROM current_transaction_versions v
			JOIN transactions t ON t.id = v.transaction_id AND t.deleted_at IS NULL
			JOIN posting_versions p ON p.transaction_version_id = v.id
			WHERE v.book_id = ? AND v.transaction_date = ? AND v.status = 'posted'
				AND p.commodity_id = ?
		)`, bookID, commodityID, date, bookID, date, commodityID).Scan(&found)
	if err != nil {
		return false, fmt.Errorf("check split-date position activity: %w", err)
	}
	return found, nil
}

// CreateProviderSplit uses the ordinary split writer with an admission guard
// and a suggestion update inside its transaction.
func (r *InvestmentRepository) CreateProviderSplit(ctx context.Context, journal CreateTransactionParams, params CreateSplitParams, suggestionID int64, symbol, mic string) error {
	date, err := time.Parse(time.DateOnly, params.EffectiveOn)
	if err != nil {
		return err
	}
	dayBefore := date.AddDate(0, 0, -1).Format(time.DateOnly)
	guard := func(tx *sql.Tx) error {
		record, err := eventSuggestionByIDTx(ctx, tx, params.BookID, suggestionID)
		if err != nil {
			return err
		}
		if record.Status != "suggested" {
			return ErrEventSuggestionNotPending
		}
		targets, err := massiveSplitTargets(ctx, tx, params.BookID, record.InstrumentID.Int64)
		if err != nil {
			return err
		}
		if len(targets) != 1 || targets[0].CommodityID != params.CommodityID || targets[0].Symbol != symbol || targets[0].MIC != mic {
			return ErrSplitSuggestionNeedsReview
		}
		var conflicting bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS (
			SELECT 1 FROM investment_provider_event_revisions WHERE provider_event_id = ?
			UNION ALL
			SELECT 1 FROM investment_provider_events other
			WHERE other.book_id = ? AND other.instrument_id = ? AND other.event_family = 'split'
				AND other.event_date = ? AND other.id <> ?
		)`, record.ProviderEventID, params.BookID, record.InstrumentID, params.EffectiveOn, record.ProviderEventID).Scan(&conflicting); err != nil {
			return fmt.Errorf("check for conflicting split evidence: %w", err)
		}
		if conflicting {
			return ErrSplitSuggestionNeedsReview
		}
		active, err := providerSplitHasSameDayActivity(ctx, tx, params.BookID, params.CommodityID, params.EffectiveOn)
		if err != nil {
			return err
		}
		holdings, err := entitlementHoldingsTx(ctx, tx, params.BookID, params.CommodityID, dayBefore)
		if err != nil {
			return err
		}
		if active || holdings.SingleLongAccount() != params.AccountID {
			return ErrSplitSuggestionNeedsReview
		}
		return nil
	}
	post := func(tx *sql.Tx, transactionID int64) error {
		if transactionID <= 0 {
			return ErrSplitSuggestionNeedsReview
		}
		return r.MarkSuggestionAcceptedInTx(ctx, tx, params.BookID, suggestionID, transactionID,
			journal.CreatedAt, journal.ActorUserID, journal.AuthSessionID, journal.RequestID)
	}
	_, _, err = executeInvestmentWriteWithGuardTx(ctx, r.database, journal, guard, splitEffectsForWrite(ctx, journal, params), post)
	return err
}

type SplitEventLinkCandidate struct {
	SuggestionID       int64
	TransactionID      int64
	HoldingAccountID   int64
	HoldingAccountName string
}

func splitEventLinkCandidatesSQL(ctx context.Context) string {
	return `
		SELECT s.id, t.id, f.account_id, COALESCE(holding.name, '')
		FROM investment_event_suggestions s
		JOIN investment_provider_events e ON e.id = s.provider_event_id AND e.book_id = s.book_id
		JOIN market_data_sources source ON source.id = e.source_id AND source.code = 'massive'
		JOIN investment_instruments ii ON ii.id = s.instrument_id AND ii.book_id = s.book_id
		JOIN investment_split_facts f ON f.book_id = s.book_id AND f.commodity_id = ii.commodity_id
			AND f.effective_on = e.event_date
		JOIN effective_investment_operations o ON o.id = f.operation_id
		JOIN investment_operation_journal_links link ON link.operation_id = f.operation_id AND link.role = 'primary'
		JOIN transaction_versions linked ON linked.id = link.transaction_version_id
		JOIN transactions t ON t.id = linked.transaction_id AND t.deleted_at IS NULL
		JOIN current_transaction_versions current ON current.transaction_id = t.id AND current.status = 'posted'
		LEFT JOIN ` + CurrentReferenceVersions(ctx, "account") + ` holding ON holding.account_id = f.account_id
		WHERE s.book_id = ? AND s.status = 'suggested' AND e.event_family = 'split'
			AND json_extract(s.proposed_transaction_json, '$.holding_account_id') = f.account_id
			AND json_extract(e.normalized_json, '$.share_class_figi') <> ''
			AND json_extract(e.normalized_json, '$.conflict') IS NOT 1
			AND json_extract(e.normalized_json, '$.ratio_numerator') = f.ratio_numerator
			AND json_extract(e.normalized_json, '$.ratio_denominator') = f.ratio_denominator
			AND NOT EXISTS (SELECT 1 FROM investment_provider_event_revisions WHERE provider_event_id = e.id)
			AND NOT EXISTS (
				SELECT 1 FROM investment_provider_events other
				WHERE other.book_id = s.book_id AND other.instrument_id = s.instrument_id
					AND other.event_family = 'split' AND other.event_date = e.event_date AND other.id <> e.id
			)
	`
}

func (r *InvestmentRepository) SplitEventLinkCandidates(ctx context.Context, bookID int64) ([]SplitEventLinkCandidate, error) {
	rows, err := r.database.QueryContext(ctx, splitEventLinkCandidatesSQL(ctx)+` ORDER BY s.id, t.id`, bookID)
	if err != nil {
		return nil, fmt.Errorf("read split event link candidates: %w", err)
	}
	defer rows.Close()
	var candidates []SplitEventLinkCandidate
	for rows.Next() {
		var c SplitEventLinkCandidate
		if err := rows.Scan(&c.SuggestionID, &c.TransactionID, &c.HoldingAccountID, &c.HoldingAccountName); err != nil {
			return nil, fmt.Errorf("scan split event link candidate: %w", err)
		}
		candidates = append(candidates, c)
	}
	return candidates, rows.Err()
}
