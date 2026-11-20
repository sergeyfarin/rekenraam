package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"rekenraam/backend/internal/exact"
)

var (
	// ErrEventLinkTarget means the transaction is not an effective posted
	// cash dividend of the event's instrument in its window, or another
	// provider event already admitted it.
	ErrEventLinkTarget = errors.New("transaction cannot be linked to this provider event")
)

// DividendEventLinkWindowDays bounds how long after an ex-date an owner-recorded
// payment is offered as the same dividend. Payment usually follows within weeks;
// the window is a discovery aid only, the owner still chooses the link.
const DividendEventLinkWindowDays = 90

// EntitlementHoldings is the dated position of one instrument at the close of
// the day before an ex-date, per holding account (#166 slot semantics).
type EntitlementHoldings struct {
	Long  []DatedHolding
	Short []DatedHolding
}

// SingleLongAccount refuses uncertain entitlement across accounts or sides.
func (h EntitlementHoldings) SingleLongAccount() int64 {
	if len(h.Long) == 0 || len(h.Short) != 0 {
		return 0
	}
	accountID := h.Long[0].AccountID
	for _, position := range h.Long[1:] {
		if position.AccountID != accountID {
			return 0
		}
	}
	return accountID
}

// DividendEntitlementHoldings replays every position of commodityID up to
// asOf in one rolled-back read. It is evidence for a suggestion only; the
// dividend writer never consumes it.
func (r *PricingRepository) DividendEntitlementHoldings(ctx context.Context, bookID, commodityID int64, asOf string) (EntitlementHoldings, error) {
	tx, err := r.database.BeginTx(ctx, nil)
	if err != nil {
		return EntitlementHoldings{}, fmt.Errorf("begin entitlement read: %w", err)
	}
	defer rollbackTx(ctx, tx)
	return entitlementHoldingsTx(ctx, tx, bookID, commodityID, asOf)
}

func entitlementHoldingsTx(ctx context.Context, tx *sql.Tx, bookID, commodityID int64, asOf string) (EntitlementHoldings, error) {
	if !isDisposalCalendarDate(asOf) {
		return EntitlementHoldings{}, fmt.Errorf("%w: entitlement needs a calendar date", ErrInvalidDisposalParams)
	}
	rows, err := tx.QueryContext(ctx, `
		SELECT DISTINCT account_id, cost_commodity_id, position_side
		FROM investment_lots
		WHERE book_id = ? AND commodity_id = ? AND opened_on <= ?
		ORDER BY account_id, cost_commodity_id, position_side`, bookID, commodityID, asOf)
	if err != nil {
		return EntitlementHoldings{}, fmt.Errorf("read entitlement positions: %w", err)
	}
	type position struct {
		key  investmentReplayPositionKey
		side string
	}
	var positions []position
	for rows.Next() {
		p := position{key: investmentReplayPositionKey{commodityID: commodityID}}
		if err := rows.Scan(&p.key.accountID, &p.key.costCommodityID, &p.side); err != nil {
			rows.Close()
			return EntitlementHoldings{}, fmt.Errorf("scan entitlement position: %w", err)
		}
		positions = append(positions, p)
	}
	if err := errors.Join(rows.Err(), rows.Close()); err != nil {
		return EntitlementHoldings{}, fmt.Errorf("iterate entitlement positions: %w", err)
	}
	var result EntitlementHoldings
	for _, p := range positions {
		holding, ok, err := datedHoldingTx(ctx, tx, bookID, p.key, p.side, asOf)
		if err != nil {
			return EntitlementHoldings{}, err
		}
		if !ok {
			continue
		}
		if p.side == PositionSideShort {
			result.Short = append(result.Short, holding)
		} else {
			result.Long = append(result.Long, holding)
		}
	}
	return result, nil
}

type StageProviderEventParams struct {
	BookID          int64
	ActorUserID     int64
	SourceID        int64
	ProviderEventID string
	InstrumentID    int64
	EventFamily     string
	EventDate       string
	NormalizedJSON  string
	RawJSON         string
	ConfidenceBPS   int
	ProposedJSON    string
	RecordedAt      string
}

// StageProviderEvent writes one immutable provider event and its pending
// suggestion under one audit event. A repeated fetch of the same source
// identity changes nothing and reports staged=false: the first observation,
// and every owner decision recorded against it, stay authoritative.
func (r *PricingRepository) StageProviderEvent(ctx context.Context, params StageProviderEventParams) (bool, error) {
	tx, err := r.database.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("begin stage provider event: %w", err)
	}
	defer rollbackTx(ctx, tx)
	var existing int64
	err = tx.QueryRowContext(ctx, `SELECT id FROM investment_provider_events WHERE source_id = ? AND provider_event_id = ?`,
		params.SourceID, params.ProviderEventID).Scan(&existing)
	if err == nil {
		return false, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return false, fmt.Errorf("read provider event identity: %w", err)
	}
	auditEventID, err := insertAuditEvent(ctx, tx, AuditEventParams{BookID: params.BookID, ActorUserID: params.ActorUserID, OccurredAt: params.RecordedAt,
		OriginType: "internal", Operation: "investment.provider_event.stage", Reason: "staged provider event for review"})
	if err != nil {
		return false, err
	}
	result, err := tx.ExecContext(ctx, `
		INSERT INTO investment_provider_events (book_id, source_id, provider_event_id, instrument_id, event_family, event_date,
			status, normalized_json, raw_json, created_at, created_audit_event_id)
		VALUES (?, ?, ?, ?, ?, ?, 'new', ?, ?, ?, ?)`,
		params.BookID, params.SourceID, params.ProviderEventID, params.InstrumentID, params.EventFamily, params.EventDate,
		params.NormalizedJSON, params.RawJSON, params.RecordedAt, auditEventID)
	if err != nil {
		return false, fmt.Errorf("insert provider event: %w", err)
	}
	eventID, err := result.LastInsertId()
	if err != nil {
		return false, fmt.Errorf("read provider event id: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO investment_event_suggestions (book_id, provider_event_id, instrument_id, confidence_bps, status,
			proposed_transaction_json, created_at, updated_at, created_audit_event_id, updated_audit_event_id)
		VALUES (?, ?, ?, ?, 'suggested', ?, ?, ?, ?, ?)`,
		params.BookID, eventID, params.InstrumentID, params.ConfidenceBPS, params.ProposedJSON,
		params.RecordedAt, params.RecordedAt, auditEventID, auditEventID); err != nil {
		return false, fmt.Errorf("insert provider event suggestion: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("commit stage provider event: %w", err)
	}
	return true, nil
}

// StagedProviderEvent is an already-staged event's identity and first
// observation, which a later fetch compares against (#254).
type StagedProviderEvent struct {
	ID             int64
	NormalizedJSON string
}

// StagedProviderEventByIdentity reads the event a source identity already
// staged, or ErrNotFound.
func (r *PricingRepository) StagedProviderEventByIdentity(ctx context.Context, bookID, sourceID int64, providerEventID string) (StagedProviderEvent, error) {
	var event StagedProviderEvent
	err := r.database.QueryRowContext(ctx, `
		SELECT id, normalized_json FROM investment_provider_events
		WHERE book_id = ? AND source_id = ? AND provider_event_id = ?`, bookID, sourceID, providerEventID).Scan(&event.ID, &event.NormalizedJSON)
	if errors.Is(err, sql.ErrNoRows) {
		return StagedProviderEvent{}, ErrNotFound
	}
	if err != nil {
		return StagedProviderEvent{}, fmt.Errorf("read staged provider event: %w", err)
	}
	return event, nil
}

type RecordProviderEventRevisionParams struct {
	BookID          int64
	ActorUserID     int64
	ProviderEventID int64
	ContentKey      string
	NormalizedJSON  string
	RawJSON         string
	ObservedAt      string
}

// RecordProviderEventRevision appends a later, differing observation of a
// staged event as immutable evidence under its own audit event. Content
// already recorded for the event reports recorded=false and writes nothing.
// The caller decides that the content differs from the first observation;
// the event, its suggestion and any posted or linked transaction are never
// touched here.
func (r *PricingRepository) RecordProviderEventRevision(ctx context.Context, params RecordProviderEventRevisionParams) (bool, error) {
	tx, err := r.database.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("begin record provider event revision: %w", err)
	}
	defer rollbackTx(ctx, tx)
	var existing int64
	err = tx.QueryRowContext(ctx, `SELECT id FROM investment_provider_event_revisions WHERE provider_event_id = ? AND content_key = ?`,
		params.ProviderEventID, params.ContentKey).Scan(&existing)
	if err == nil {
		return false, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return false, fmt.Errorf("read provider event revision: %w", err)
	}
	auditEventID, err := insertAuditEvent(ctx, tx, AuditEventParams{BookID: params.BookID, ActorUserID: params.ActorUserID, OccurredAt: params.ObservedAt,
		OriginType: "internal", Operation: "investment.provider_event.revise", Reason: "provider revised a staged event"})
	if err != nil {
		return false, err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO investment_provider_event_revisions (book_id, provider_event_id, content_key, normalized_json, raw_json,
			observed_at, created_by_user_id, created_audit_event_id)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		params.BookID, params.ProviderEventID, params.ContentKey, params.NormalizedJSON, params.RawJSON,
		params.ObservedAt, params.ActorUserID, auditEventID); err != nil {
		return false, fmt.Errorf("insert provider event revision: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("commit provider event revision: %w", err)
	}
	return true, nil
}

// ProviderEventRevisionRecord is one appended provider revision.
type ProviderEventRevisionRecord struct {
	ProviderEventID int64
	NormalizedJSON  string
	ObservedAt      string
}

// ProviderEventRevisions reads every revision in the book in observation
// order, so the review screen composes them without a request per event.
func (r *InvestmentRepository) ProviderEventRevisions(ctx context.Context, bookID int64) ([]ProviderEventRevisionRecord, error) {
	rows, err := r.database.QueryContext(ctx, `
		SELECT provider_event_id, normalized_json, observed_at
		FROM investment_provider_event_revisions
		WHERE book_id = ?
		ORDER BY provider_event_id, id`, bookID)
	if err != nil {
		return nil, fmt.Errorf("read provider event revisions: %w", err)
	}
	var result []ProviderEventRevisionRecord
	for rows.Next() {
		var record ProviderEventRevisionRecord
		if err := rows.Scan(&record.ProviderEventID, &record.NormalizedJSON, &record.ObservedAt); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan provider event revision: %w", err)
		}
		result = append(result, record)
	}
	if err := errors.Join(rows.Err(), rows.Close()); err != nil {
		return nil, fmt.Errorf("iterate provider event revisions: %w", err)
	}
	return result, nil
}

// EventLinkCandidate is an owner-recorded payment a pending provider
// suggestion may be linked to instead of posting it again: an effective posted
// cash dividend for a dividend suggestion, or an effective payment in lieu
// (#258) for a short-only one. Gross is the dividend's gross or the amount the
// payment in lieu owed, always positive; HoldingAccount names the short
// holding a payment in lieu was owed for.
type EventLinkCandidate struct {
	SuggestionID       int64
	TransactionID      int64
	TransactionDate    string
	CashCode           string
	Gross              *exact.ScaledInt
	HoldingAccountID   *int64
	HoldingAccountName string
}

// Both cash families share the once-per-root claim check. Migration 25 also
// enforces it at storage admission, including inverse/replacement journals.
const cashPaymentClaimSQL = `SELECT 1 FROM investment_cash_payment_identity target
	JOIN investment_cash_payment_identity previous ON previous.book_id=target.book_id AND previous.root_operation_id=target.root_operation_id
	JOIN investment_event_suggestions other ON other.book_id=previous.book_id
		AND (other.generated_transaction_id=previous.transaction_id OR other.linked_transaction_id=previous.transaction_id)
	WHERE target.book_id=o.book_id AND target.transaction_id=t.id`

// eventLinkCandidatesSQL offers each suggestion only its own kind of payment.
// Dividend candidates match instrument/date window and any recorded ex date;
// payment-in-lieu candidates match instrument, one proposed holding and
// ex-date. Claim identity spans the correction ancestry independently of the
// current owner attribution.
func eventLinkCandidatesSQL(ctx context.Context) string {
	return `
	SELECT suggestion_id, transaction_id, transaction_date, cash_code, amount_value, amount_scale, holding_account_id, holding_account_name
	FROM (
		SELECT s.id AS suggestion_id, tv.transaction_id AS transaction_id, o.event_date AS transaction_date, cash.code AS cash_code,
			c.amount_value AS amount_value, c.amount_scale AS amount_scale, c.component_seq AS component_seq,
			NULL AS holding_account_id, '' AS holding_account_name
		FROM investment_event_suggestions s
		JOIN investment_provider_events e ON e.id = s.provider_event_id AND e.book_id = s.book_id
		JOIN investment_instruments ii ON ii.id = s.instrument_id
		JOIN investment_dividend_facts facts ON facts.commodity_id = ii.commodity_id AND facts.book_id = s.book_id AND facts.withholding_only = 0
		JOIN effective_investment_operations o ON o.id = facts.operation_id AND o.operation_kind = 'dividend'
		JOIN investment_operation_journal_links link ON link.operation_id = o.id AND link.book_id = o.book_id AND link.role = 'primary'
		JOIN transaction_versions linked ON linked.id = link.transaction_version_id
		JOIN transactions t ON t.id = linked.transaction_id AND t.deleted_at IS NULL
		JOIN current_transaction_versions tv ON tv.transaction_id = t.id AND tv.status = 'posted'
		JOIN investment_operation_components c ON c.operation_id = o.id AND c.component_kind = 'dividend_gross'
		JOIN commodities cash ON cash.id = c.commodity_id
		WHERE s.book_id = ? AND s.status = 'suggested' AND e.event_family = 'dividend'
			-- A short holder's payment in lieu (#255) is never a cash dividend.
			AND json_extract(s.proposed_transaction_json, '$.kind') IS NOT 'payment_in_lieu'
			-- A dividend recording an ex date is paid for that ex date only;
			-- one recording none must fall in the payment window.
			AND CASE WHEN EXISTS (SELECT 1 FROM investment_operation_dates ex WHERE ex.operation_id = o.id AND ex.date_role = 'ex')
				THEN EXISTS (SELECT 1 FROM investment_operation_dates ex
					WHERE ex.operation_id = o.id AND ex.date_role = 'ex' AND ex.event_date = e.event_date)
				ELSE o.event_date >= e.event_date AND o.event_date <= date(e.event_date, '+' || ? || ' days') END
			AND NOT EXISTS (` + cashPaymentClaimSQL + `)
		UNION ALL
		SELECT s.id, tv.transaction_id, o.event_date, cash.code, c.amount_value, c.amount_scale, c.component_seq,
			facts.holding_account_id, COALESCE(holding.name, '')
		FROM investment_event_suggestions s
		JOIN investment_provider_events e ON e.id = s.provider_event_id AND e.book_id = s.book_id
		JOIN investment_instruments ii ON ii.id = s.instrument_id
		JOIN investment_payment_in_lieu_facts facts ON facts.commodity_id = ii.commodity_id AND facts.book_id = s.book_id
			AND facts.ex_date = e.event_date
		LEFT JOIN ` + CurrentReferenceVersions(ctx, "account") + ` holding ON holding.account_id = facts.holding_account_id
		JOIN effective_investment_operations o ON o.id = facts.operation_id AND o.operation_kind = 'payment_in_lieu'
		JOIN investment_operation_journal_links link ON link.operation_id = o.id AND link.book_id = o.book_id AND link.role = 'primary'
		JOIN transaction_versions linked ON linked.id = link.transaction_version_id
		JOIN transactions t ON t.id = linked.transaction_id AND t.deleted_at IS NULL
		JOIN current_transaction_versions tv ON tv.transaction_id = t.id AND tv.status = 'posted'
		JOIN investment_operation_components c ON c.operation_id = o.id AND c.component_kind = 'payment_in_lieu'
		JOIN commodities cash ON cash.id = c.commodity_id
		WHERE s.book_id = ? AND s.status = 'suggested' AND e.event_family = 'dividend'
			AND json_extract(s.proposed_transaction_json, '$.kind') = 'payment_in_lieu'
			AND json_extract(s.proposed_transaction_json, '$.commodity_id') = facts.commodity_id
			AND facts.holding_account_id IN (SELECT value FROM json_each(s.proposed_transaction_json, '$.holding_account_ids'))
			AND NOT EXISTS (` + cashPaymentClaimSQL + `)
	)`
}

func eventLinkCandidatesArgs(bookID int64) []any {
	return []any{bookID, DividendEventLinkWindowDays, bookID}
}

// EventLinkCandidates composes every pending suggestion's candidates in one
// read so the review screen needs one request.
func (r *InvestmentRepository) EventLinkCandidates(ctx context.Context, bookID int64) ([]EventLinkCandidate, error) {
	rows, err := r.database.QueryContext(ctx, eventLinkCandidatesSQL(ctx)+` ORDER BY suggestion_id, transaction_date, transaction_id, component_seq`, eventLinkCandidatesArgs(bookID)...)
	if err != nil {
		return nil, fmt.Errorf("read event link candidates: %w", err)
	}
	var result []EventLinkCandidate
	for rows.Next() {
		var candidate EventLinkCandidate
		var value exact.Coefficient
		var scale int
		var holding sql.NullInt64
		if err := rows.Scan(&candidate.SuggestionID, &candidate.TransactionID, &candidate.TransactionDate, &candidate.CashCode, &value, &scale,
			&holding, &candidate.HoldingAccountName); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan event link candidate: %w", err)
		}
		last := len(result) - 1
		if last < 0 || result[last].SuggestionID != candidate.SuggestionID || result[last].TransactionID != candidate.TransactionID {
			candidate.Gross = exact.NewScaledInt()
			if holding.Valid {
				candidate.HoldingAccountID = &holding.Int64
			}
			result = append(result, candidate)
			last++
		}
		result[last].Gross.AddCoefficient(value, scale)
	}
	if err := errors.Join(rows.Err(), rows.Close()); err != nil {
		return nil, fmt.Errorf("iterate event link candidates: %w", err)
	}
	for index := range result {
		if result[index].HoldingAccountID != nil {
			// The payment-in-lieu component carries the charge negative.
			result[index].Gross = result[index].Gross.Negated()
		}
	}
	return result, nil
}

type LinkSuggestionParams struct {
	BookID        int64
	SuggestionID  int64
	TransactionID int64
	ActorUserID   int64
	AuthSessionID int64
	RequestID     string
	RecordedAt    string
}

// LinkSuggestion records that an owner-recorded dividend or payment in lieu
// is this provider event. The candidate check, the suggestion transition and the event status
// share one transaction, so a concurrent accept, ignore or second link cannot
// admit the event or the payment twice.
func (r *InvestmentRepository) LinkSuggestion(ctx context.Context, params LinkSuggestionParams) (EventSuggestionRecord, error) {
	tx, err := r.database.BeginTx(ctx, nil)
	if err != nil {
		return EventSuggestionRecord{}, fmt.Errorf("begin link event suggestion: %w", err)
	}
	defer rollbackTx(ctx, tx)
	current, err := eventSuggestionByIDTx(ctx, tx, params.BookID, params.SuggestionID)
	if err != nil {
		return EventSuggestionRecord{}, err
	}
	if current.Status != "suggested" {
		return EventSuggestionRecord{}, ErrEventSuggestionNotPending
	}
	var found int64
	reason := "linked provider event to recorded payment"
	if current.EventFamily == "split" {
		reason = "linked provider event to recorded split"
		err = tx.QueryRowContext(ctx, splitEventLinkCandidatesSQL(ctx)+` AND s.id = ? AND t.id = ? LIMIT 1`,
			params.BookID, params.SuggestionID, params.TransactionID).Scan(&found, new(int64), new(int64), new(string))
	} else {
		err = tx.QueryRowContext(ctx, eventLinkCandidatesSQL(ctx)+` WHERE suggestion_id = ? AND transaction_id = ? LIMIT 1`,
			append(eventLinkCandidatesArgs(params.BookID), params.SuggestionID, params.TransactionID)...).Scan(&found, new(int64), new(string), new(string),
			new(exact.Coefficient), new(int), new(sql.NullInt64), new(string))
	}
	if errors.Is(err, sql.ErrNoRows) {
		if current.EventFamily == "split" {
			return EventSuggestionRecord{}, ErrSplitEventLinkTarget
		}
		return EventSuggestionRecord{}, ErrEventLinkTarget
	}
	if err != nil {
		return EventSuggestionRecord{}, fmt.Errorf("check event link target: %w", err)
	}
	auditEventID, err := insertAuditEvent(ctx, tx, AuditEventParams{BookID: params.BookID, ActorUserID: params.ActorUserID, AuthSessionID: params.AuthSessionID,
		OccurredAt: params.RecordedAt, RequestID: params.RequestID, OriginType: "browser_api", Operation: "investment.event_suggestion.link",
		Reason: reason})
	if err != nil {
		return EventSuggestionRecord{}, err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE investment_event_suggestions
		SET status = 'linked', linked_transaction_id = ?, updated_at = ?, updated_by_user_id = ?, updated_audit_event_id = ?
		WHERE book_id = ? AND id = ? AND status = 'suggested'`,
		params.TransactionID, params.RecordedAt, params.ActorUserID, auditEventID, params.BookID, params.SuggestionID); err != nil {
		return EventSuggestionRecord{}, fmt.Errorf("link event suggestion: %w", err)
	}
	if err := setProviderEventStatusTx(ctx, tx, params.BookID, current.ProviderEventID, "matched"); err != nil {
		return EventSuggestionRecord{}, err
	}
	record, err := eventSuggestionByIDTx(ctx, tx, params.BookID, params.SuggestionID)
	if err != nil {
		return EventSuggestionRecord{}, err
	}
	if err := tx.Commit(); err != nil {
		return EventSuggestionRecord{}, fmt.Errorf("commit link event suggestion: %w", err)
	}
	return record, nil
}

// setProviderEventStatusTx moves a new event to the owner's decision. The raw
// and normalized evidence never change.
func setProviderEventStatusTx(ctx context.Context, tx *sql.Tx, bookID, eventID int64, status string) error {
	result, err := tx.ExecContext(ctx, `UPDATE investment_provider_events SET status = ? WHERE book_id = ? AND id = ? AND status = 'new'`, status, bookID, eventID)
	if err != nil {
		return fmt.Errorf("update provider event status: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read provider event status rows: %w", err)
	}
	if rows != 1 {
		return ErrEventSuggestionNotPending
	}
	return nil
}
