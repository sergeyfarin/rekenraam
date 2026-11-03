package db

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
)

type PricingCommodityRecord struct {
	ID     int64
	Code   string
	Name   string
	Kind   string
	Status string
}

type PricingPairRecord struct {
	BaseCommodityID  int64
	QuoteCommodityID int64
}

type PricingWorkspaceRecord struct {
	Reasons     map[[2]int64]string
	Commodities []PricingCommodityRecord
	Pairs       []PricingPairRecord
	Candidates  []PriceObservationRecord
}

// PriceManagementSnapshot returns full reference and candidate sets, never a
// silently capped observation list. One read transaction keeps pair discovery
// and candidates together; no provider or durable write participates.
//
// visitPosting folds posted amounts, currency and otherwise, without
// materializing the ledger: an unpriced instrument balance is an exposure as
// much as a foreign-currency one. It runs inside this snapshot and must not
// perform database I/O.
func (r *PricingRepository) PriceManagementSnapshot(ctx context.Context, bookID int64, asOf string, fxBaseID int64, visitPosting func(LedgerAccountRecord, LedgerPostingRecord), boundaries ...ObservationBoundary) (PricingWorkspaceRecord, error) {
	result := PricingWorkspaceRecord{Commodities: []PricingCommodityRecord{}, Pairs: []PricingPairRecord{}, Candidates: []PriceObservationRecord{}}
	boundary, err := r.observationBoundary(ctx, bookID, boundaries)
	if err != nil {
		return result, err
	}
	boundary.AsOf = asOf

	tx, err := r.database.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return result, fmt.Errorf("begin price workspace: %w", err)
	}
	defer rollbackTx(ctx, tx)
	if visitPosting != nil {
		if err := streamPricingPostings(ctx, tx, bookID, asOf, visitPosting); err != nil {
			return result, err
		}
	}
	rows, err := tx.QueryContext(ctx, `
		SELECT c.id, c.code, cv.name, c.kind, cv.status
		FROM commodities c
		JOIN `+CurrentReferenceVersions(ctx, "commodity")+` cv ON cv.commodity_id = c.id
		WHERE c.book_id = ?
		ORDER BY c.code, c.id
	`, bookID)
	if err != nil {
		return result, fmt.Errorf("read price commodities: %w", err)
	}
	for rows.Next() {
		var c PricingCommodityRecord
		if err := rows.Scan(&c.ID, &c.Code, &c.Name, &c.Kind, &c.Status); err != nil {
			rows.Close()
			return result, err
		}
		result.Commodities = append(result.Commodities, c)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return result, err
	}
	rows, err = tx.QueryContext(ctx, `
		SELECT base_commodity_id, quote_commodity_id
		FROM price_series WHERE book_id = ?
		UNION
		SELECT ii.commodity_id, iiv.quote_commodity_id
		FROM investment_instruments ii
		JOIN `+CurrentReferenceVersions(ctx, "investment_instrument")+` iiv ON iiv.instrument_id = ii.id
		WHERE ii.book_id = ? AND iiv.status = 'active' AND iiv.quote_commodity_id IS NOT NULL
		UNION
		SELECT lot.commodity_id, lot.cost_commodity_id
		FROM current_investment_lots lot
		WHERE lot.book_id = ? AND lot.remaining_quantity_value <> '0'
		UNION
		SELECT av.default_commodity_id, ?
		FROM accounts a
		JOIN `+CurrentReferenceVersions(ctx, "account")+` av ON av.account_id = a.id
		JOIN commodities c ON c.id = av.default_commodity_id
		WHERE a.book_id = ? AND av.status = 'active' AND av.allows_postings = 1
			AND c.kind = 'currency' AND ? > 0 AND av.default_commodity_id <> ?
		ORDER BY 1, 2
	`, bookID, bookID, bookID, fxBaseID, bookID, fxBaseID, fxBaseID)
	if err != nil {
		return result, fmt.Errorf("read price pairs: %w", err)
	}
	for rows.Next() {
		var p PricingPairRecord
		if err := rows.Scan(&p.BaseCommodityID, &p.QuoteCommodityID); err != nil {
			rows.Close()
			return result, err
		}
		result.Pairs = append(result.Pairs, p)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return result, err
	}
	rows, err = tx.QueryContext(ctx, observationSelectionCTE+priceObservationSelect(`
  WHERE id IN (SELECT id FROM selection_latest)
  ORDER BY base_commodity_id,quote_commodity_id
 `), bookID, boundary.AsOf, boundary.RecordedAt)
	if err != nil {
		return result, fmt.Errorf("read price candidates: %w", err)
	}
	result.Candidates, err = scanPriceObservations(rows)
	rows.Close()
	if err != nil {
		return result, err
	}
	rows, err = tx.QueryContext(ctx, observationSelectionCTE+`SELECT base_commodity_id,quote_commodity_id,reason FROM selection_diagnostics`, bookID, boundary.AsOf, boundary.RecordedAt)
	if err != nil {
		return result, err
	}
	result.Reasons = map[[2]int64]string{}
	for rows.Next() {
		var pair [2]int64
		var reason string
		if err := rows.Scan(&pair[0], &pair[1], &reason); err != nil {
			rows.Close()
			return result, err
		}
		result.Reasons[pair] = reason
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return result, err
	}

	return result, nil
}

// Account classification is selected at the requested valuation date, as in
// LedgerAccountsAsOf. Status is deliberately left to the consumer: closure
// does not erase a balance. Current versions, entry dates and deletion follow
// LedgerPostingsThrough; coefficients never enter SQLite SUM or a float.
func streamPricingPostings(ctx context.Context, tx *sql.Tx, bookID int64, asOf string, visit func(LedgerAccountRecord, LedgerPostingRecord)) error {
	rows, err := tx.QueryContext(ctx, `
		SELECT a.id, av.account_class, a.system_role,
			pv.commodity_id, pv.quantity_value, pv.quantity_scale
		FROM current_transaction_versions tv
		JOIN transactions t ON t.id = tv.transaction_id
		JOIN journal_entries je ON je.transaction_version_id = tv.id
		JOIN posting_versions pv ON pv.journal_entry_id = je.id
		JOIN commodities c ON c.id = pv.commodity_id AND c.book_id = tv.book_id
		JOIN accounts a ON a.id = pv.account_id AND a.book_id = tv.book_id
		JOIN account_versions av ON av.id = (
			SELECT candidate.id FROM account_versions candidate
			WHERE candidate.account_id = a.id AND candidate.effective_from <= ?
			ORDER BY candidate.effective_from DESC, candidate.version_seq DESC LIMIT 1)
		WHERE tv.book_id = ? AND tv.status = 'posted' AND t.deleted_at IS NULL
			AND je.entry_date <= ?
	`, asOf, bookID, asOf)
	if err != nil {
		return fmt.Errorf("read pricing postings: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var account LedgerAccountRecord
		var posting LedgerPostingRecord
		if err := rows.Scan(&account.ID, &account.AccountClass, &account.SystemRole,
			&posting.CommodityID, &posting.QuantityValue, &posting.QuantityScale); err != nil {
			return fmt.Errorf("scan pricing posting: %w", err)
		}
		posting.AccountID = account.ID
		visit(account, posting)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate pricing postings: %w", err)
	}
	return nil
}

// PriceHistoryPage uses observation ID as recording order. Inserts after page
// one never move old rows across a cursor; callers bind cursors to filters.
func (r *PricingRepository) PriceHistoryPage(ctx context.Context, bookID, baseID, quoteID, beforeID int64, includeVoided bool, limit int) ([]PriceObservationRecord, error) {
	where := "WHERE book_id = ? AND base_commodity_id = ? AND quote_commodity_id = ?"
	args := []any{bookID, baseID, quoteID}
	if beforeID > 0 {
		where += " AND id < ?"
		args = append(args, beforeID)
	}
	if !includeVoided {
		where += " AND voided_at IS NULL"
	}
	args = append(args, limit)
	rows, err := r.database.QueryContext(ctx, priceObservationSelect(where+" ORDER BY id DESC LIMIT ?"), args...)
	if err != nil {
		return nil, fmt.Errorf("read price history: %w", err)
	}
	defer rows.Close()
	return scanPriceObservations(rows)
}

var ErrPriceImpactStale = errors.New("price dependencies changed; review again")

type PriceImpactRecord struct {
	Prices []PriceObservationRecord
	Token  string
}

func priceImpactTx(ctx context.Context, tx *sql.Tx, bookID, rootID int64) (PriceImpactRecord, error) {
	root, err := priceObservationByIDTx(ctx, tx, bookID, rootID)
	if err != nil {
		return PriceImpactRecord{}, err
	}
	if root.VoidedAt.Valid {
		return PriceImpactRecord{}, ErrPriceObservationAlreadyVoided
	}
	voidedIDs := []int64{rootID}
	frontier := []int64{rootID}
	seen := map[int64]bool{rootID: true}
	// depth is the derivation level of the observations `next` collects: the
	// target is level 0 and observations derived directly from it are level 1.
	// The walk runs until the frontier empties, so a cycle terminates on `seen`
	// rather than on the depth bound; only genuinely deeper-than-bound graphs
	// abort. A chain of exactly maxVoidCascadeDepth levels is voided in full.
	for depth := 1; len(frontier) > 0; depth++ {
		var next []int64
		for _, observationID := range frontier {
			derived, err := activeDerivedObservationIDsTx(ctx, tx, bookID, observationID)
			if err != nil {
				return PriceImpactRecord{}, err
			}
			for _, derivedID := range derived {
				if seen[derivedID] {
					continue
				}
				seen[derivedID] = true
				next = append(next, derivedID)
			}
		}
		if len(next) > 0 && depth > maxVoidCascadeDepth {
			return PriceImpactRecord{}, fmt.Errorf("%w: observation %d in book %d has derived observations beyond %d levels",
				ErrPriceVoidCascadeTooDeep, rootID, bookID, maxVoidCascadeDepth)
		}
		voidedIDs = append(voidedIDs, next...)
		frontier = next
	}

	result := PriceImpactRecord{Prices: make([]PriceObservationRecord, 0, len(voidedIDs))}
	for _, id := range voidedIDs {
		price, err := priceObservationByIDTx(ctx, tx, bookID, id)
		if err != nil {
			return result, err
		}
		result.Prices = append(result.Prices, price)
	}
	payload, err := json.Marshal(result.Prices)
	if err != nil {
		return result, err
	}
	digest := sha256.Sum256(payload)
	result.Token = hex.EncodeToString(digest[:])
	return result, nil
}

func (r *PricingRepository) PriceImpact(ctx context.Context, bookID, rootID int64) (PriceImpactRecord, error) {
	tx, err := r.database.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return PriceImpactRecord{}, err
	}
	defer rollbackTx(ctx, tx)
	return priceImpactTx(ctx, tx, bookID, rootID)
}

type CorrectPriceParams struct {
	Create         CreatePriceObservationParams
	ObservationID  int64
	ExpectedImpact string
}

type CorrectPriceResult struct {
	Replacement PriceObservationRecord
	Retired     []PriceObservationRecord
}

func (r *PricingRepository) CorrectPrice(ctx context.Context, params CorrectPriceParams) (CorrectPriceResult, error) {
	result := CorrectPriceResult{}
	tx, err := r.database.BeginTx(ctx, nil)
	if err != nil {
		return result, err
	}
	defer rollbackTx(ctx, tx)
	impact, err := priceImpactTx(ctx, tx, params.Create.BookID, params.ObservationID)
	if err != nil {
		return result, err
	}
	if params.ExpectedImpact == "" || impact.Token != params.ExpectedImpact {
		return result, ErrPriceImpactStale
	}
	root := impact.Prices[0]
	params.Create.Spec.BaseCommodityID = root.BaseCommodityID
	params.Create.Spec.QuoteCommodityID = root.QuoteCommodityID
	params.Create.Spec.ValuationDate = root.ValuationDate
	params.Create.Spec.SeriesID = sql.NullInt64{}
	params.Create.Spec.SupersedesObservationID = sql.NullInt64{Int64: root.ID, Valid: true}
	auditID, err := insertAuditEvent(ctx, tx, AuditEventParams{
		BookID: params.Create.BookID, ActorUserID: params.Create.ActorUserID, AuthSessionID: params.Create.AuthSessionID,
		OccurredAt: params.Create.CreatedAt, RequestID: params.Create.RequestID, OriginType: params.Create.OriginType,
		Operation: "pricing.price.correct", Reason: params.Create.ChangeReason,
	})
	if err != nil {
		return result, err
	}
	result.Retired, err = voidPriceObservationWithAuditTx(ctx, tx, VoidPriceObservationParams{
		BookID: params.Create.BookID, ObservationID: root.ID, ActorUserID: params.Create.ActorUserID,
		VoidedAt: params.Create.CreatedAt, VoidReason: params.Create.ChangeReason,
	}, auditID)
	if err != nil {
		return result, err
	}
	result.Replacement, err = insertPriceObservationTx(ctx, tx, params.Create, auditID)
	if err != nil {
		return result, err
	}
	if err := tx.Commit(); err != nil {
		return result, fmt.Errorf("commit price correction: %w", err)
	}
	return result, nil
}

// PricePairBaseKind reports whether both commodities belong to the book and,
// when they do, the base commodity's kind (currency, security, crypto).
func (r *PricingRepository) PricePairBaseKind(ctx context.Context, bookID, baseID, quoteID int64) (string, bool, error) {
	var count int
	var kind sql.NullString
	err := r.database.QueryRowContext(ctx, `SELECT count(*), MAX(CASE WHEN id = ? THEN kind END) FROM commodities WHERE book_id = ? AND id IN (?, ?)`, baseID, bookID, baseID, quoteID).Scan(&count, &kind)
	if err != nil {
		return "", false, fmt.Errorf("read price commodity references: %w", err)
	}
	return kind.String, count == 2, nil
}
