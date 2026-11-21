package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/marketdata"
)

const splitEventWorkKind = "investment.split_events"
const splitEventLookbackDays = 365

type splitEventPayload struct {
	Reason       string `json:"reason"`
	InstrumentID int64  `json:"instrument_id"`
}

type providerSplitEvidence struct {
	Schema      int    `json:"schema"`
	Provider    string `json:"provider"`
	ProviderID  string `json:"provider_event_id"`
	Symbol      string `json:"symbol"`
	MIC         string `json:"mic"`
	ShareFIGI   string `json:"share_class_figi,omitempty"`
	Date        string `json:"effective_on"`
	Kind        string `json:"adjustment_type"`
	Numerator   int64  `json:"ratio_numerator,omitempty"`
	Denominator int64  `json:"ratio_denominator,omitempty"`
	Conflict    bool   `json:"conflict,omitempty"`
}

type proposedProviderSplit struct {
	Kind             string `json:"kind"`
	HoldingAccountID int64  `json:"holding_account_id,omitempty"`
	HoldingName      string `json:"holding_account_name,omitempty"`
	ReviewReason     string `json:"review_reason,omitempty"`
}

func (s *PricingService) QueueSplitEventFetch(ctx context.Context, ownerID int64) (int, error) {
	if ownerID <= 0 {
		return 0, ValidationError{Message: "owner user is required"}
	}
	ctx, err := s.settings.referenceContext(ctx, s.now())
	if err != nil {
		return 0, err
	}
	targets, err := s.repository.MassiveSplitTargets(ctx, BookID, 0)
	if err != nil {
		return 0, err
	}
	now := s.now().UTC().Format(time.RFC3339)
	for _, target := range targets {
		payload, _ := json.Marshal(splitEventPayload{Reason: "manual", InstrumentID: target.InstrumentID})
		if _, err := s.repository.EnqueueBackgroundWork(ctx, BookID, splitEventWorkKind, string(payload), now); err != nil {
			return 0, err
		}
	}
	select {
	case s.wake <- struct{}{}:
	default:
	}
	return len(targets), nil
}

func (s *PricingService) runDueSplitEvents(ctx context.Context, logger *slog.Logger, workerID string) {
	s.runDueProviderEvents(ctx, logger, workerID, splitEventWorkKind, s.fetchSplitEvents)
}

func (s *PricingService) fetchSplitEvents(ctx context.Context, item db.BackgroundWorkItemRecord) error {
	var payload splitEventPayload
	if item.PayloadVersion != 1 || json.Unmarshal([]byte(item.PayloadJSON), &payload) != nil || payload.InstrumentID <= 0 {
		return &marketdata.QuoteError{Status: 400, Terminal: true}
	}
	now := s.now()
	ctx, err := s.settings.referenceContext(ctx, now)
	if err != nil {
		return err
	}
	targets, err := s.repository.MassiveSplitTargets(ctx, BookID, payload.InstrumentID)
	if err != nil || len(targets) == 0 {
		return err
	}
	entry, _ := s.providerRegistry.Provider(marketdata.MassiveCode)
	provider, ok := entry.(*marketdata.MassiveProvider)
	if !ok {
		return marketdata.ErrUnsupportedPair
	}
	exchangeZone, err := time.LoadLocation("America/New_York")
	if err != nil {
		return err
	}
	marketDate := now.In(exchangeZone).Format(time.DateOnly)
	through := min(marketDate, db.ReferenceDate(ctx))
	end, err := time.Parse(time.DateOnly, through)
	if err != nil {
		return err
	}
	target := targets[0]
	start := end.AddDate(0, 0, -splitEventLookbackDays).Format(time.DateOnly)
	observations, err := provider.FetchSplits(ctx, target.Symbol, target.MIC, start, through)
	if err != nil {
		return err
	}
	current, err := s.repository.MassiveSplitTargets(ctx, BookID, target.InstrumentID)
	if err != nil || len(current) == 0 || current[0] != target {
		return err
	}
	ownerID, err := s.repository.CurrentBookOwnerID(ctx, BookID)
	if err != nil {
		return err
	}
	identityFailed := false
	for _, observation := range observations {
		identityFailed = identityFailed || (observation.Numerator > 0 && observation.ShareFIGI == "")
		if err := s.stageSplitObservation(ctx, target, ownerID, observation); err != nil {
			return err
		}
	}
	if identityFailed {
		return fmt.Errorf("%w: Massive could not verify a historical listing", marketdata.ErrUnsupportedPair)
	}
	return nil
}

func (s *PricingService) stageSplitObservation(ctx context.Context, target db.MassiveSplitTarget, ownerID int64, row marketdata.MassiveSplit) error {
	content := providerSplitEvidence{Schema: 1, Provider: marketdata.MassiveCode, ProviderID: row.ID, Symbol: target.Symbol, MIC: target.MIC, ShareFIGI: row.ShareFIGI,
		Date: row.Date, Kind: row.Kind, Numerator: row.Numerator, Denominator: row.Denominator, Conflict: row.Conflict}
	normalized, _ := json.Marshal(content)
	now := s.now().UTC().Format(time.RFC3339)
	staged, err := s.repository.StagedProviderEventByIdentity(ctx, BookID, target.SourceID, row.ID)
	switch {
	case err == nil:
		originalKey, err := marketdata.MassiveSplitContentKey(staged.RawJSON)
		if err != nil {
			return err
		}
		contentKey, err := marketdata.MassiveSplitContentKey(row.RawJSON)
		if err != nil {
			return err
		}
		if originalKey == contentKey {
			return nil
		}
		_, err = s.repository.RecordProviderEventRevision(ctx, db.RecordProviderEventRevisionParams{
			BookID: BookID, ActorUserID: ownerID, ProviderEventID: staged.ID, ContentKey: contentKey,
			NormalizedJSON: string(normalized), RawJSON: row.RawJSON, ObservedAt: now,
		})
		return err
	case !errors.Is(err, db.ErrNotFound):
		return err
	}
	if (row.Kind != "forward_split" && row.Kind != "reverse_split") || (row.Numerator > 0 && row.ShareFIGI == "") {
		return nil
	}

	date, err := time.Parse(time.DateOnly, row.Date)
	if err != nil {
		return err
	}
	holdings, err := s.repository.DividendEntitlementHoldings(ctx, BookID, target.CommodityID, date.AddDate(0, 0, -1).Format(time.DateOnly))
	if err != nil || len(holdings.Long)+len(holdings.Short) == 0 {
		return err
	}
	active, err := s.repository.ProviderSplitHasSameDayActivity(ctx, BookID, target.CommodityID, row.Date)
	if err != nil {
		return err
	}
	proposal := proposedProviderSplit{Kind: "split", HoldingAccountID: holdings.SingleLongAccount()}
	if proposal.HoldingAccountID > 0 {
		proposal.HoldingName, err = s.repository.ProviderSplitHoldingName(ctx, BookID, proposal.HoldingAccountID)
		if err != nil {
			return err
		}
	}
	switch {
	case row.Numerator == 0 || row.ShareFIGI == "" || row.Conflict:
		proposal.ReviewReason = "evidence"
	case proposal.HoldingAccountID == 0:
		proposal.ReviewReason = "holdings"
	case active:
		proposal.ReviewReason = "same_day_activity"
	}
	proposed, _ := json.Marshal(proposal)
	confidence := 10000
	if proposal.ReviewReason != "" {
		confidence = 0
	}
	_, err = s.repository.StageProviderEvent(ctx, db.StageProviderEventParams{
		BookID: BookID, ActorUserID: ownerID, SourceID: target.SourceID, ProviderEventID: row.ID,
		InstrumentID: target.InstrumentID, EventFamily: "split", EventDate: row.Date,
		NormalizedJSON: string(normalized), RawJSON: row.RawJSON, ConfidenceBPS: confidence,
		ProposedJSON: string(proposed), RecordedAt: now,
	})
	return err
}

func (s *InvestmentService) acceptSplitSuggestion(ctx context.Context, record db.EventSuggestionRecord, ownerID, authSessionID int64, requestID string) (InvestmentEventSuggestion, error) {
	var proposal proposedProviderSplit
	var evidence providerSplitEvidence
	if json.Unmarshal([]byte(record.ProposedTransactionJSON), &proposal) != nil ||
		json.Unmarshal([]byte(record.EventNormalizedJSON), &evidence) != nil ||
		record.SourceCode != marketdata.MassiveCode || proposal.Kind != "split" || proposal.HoldingAccountID <= 0 ||
		proposal.ReviewReason != "" || evidence.Schema != 1 || evidence.ShareFIGI == "" ||
		evidence.Date != record.EventDate || evidence.Conflict || !record.InstrumentID.Valid {
		return InvestmentEventSuggestion{}, ValidationError{Message: "provider split requires manual review"}
	}
	ctx, err := s.settings.referenceContext(ctx, s.now())
	if err != nil {
		return InvestmentEventSuggestion{}, err
	}
	if evidence.Date > db.ReferenceDate(ctx) {
		return InvestmentEventSuggestion{}, ValidationError{Message: "provider split date has not arrived"}
	}
	instrument, err := s.repository.InstrumentByID(ctx, BookID, record.InstrumentID.Int64)
	if err != nil {
		return InvestmentEventSuggestion{}, err
	}
	journal, params, _, err := s.prepareSplitWrite(ctx, InvestmentSplitInput{
		OwnerUserID: ownerID, AuthSessionID: authSessionID, RequestID: requestID,
		Operation: "investment.event_suggestion.split", EffectiveOn: evidence.Date,
		HoldingAccountID: proposal.HoldingAccountID, CommodityID: instrument.CommodityID,
		RatioNumerator: evidence.Numerator, RatioDenominator: evidence.Denominator,
		SourceEvidenceJSON: record.EventNormalizedJSON, ChangeReason: "accepted provider split suggestion",
	})
	if err != nil {
		return InvestmentEventSuggestion{}, err
	}
	journal.GainImpact = gainImpactPolicy("")
	if err := s.repository.CreateProviderSplit(ctx, journal, params, record.ID, evidence.Symbol, evidence.MIC); err != nil {
		switch {
		case errors.Is(err, db.ErrEventSuggestionNotPending):
			return InvestmentEventSuggestion{}, ErrInvestmentSuggestionNotPending
		case errors.Is(err, db.ErrSplitSuggestionNeedsReview):
			return InvestmentEventSuggestion{}, ValidationError{Message: "provider split requires manual review"}
		default:
			return InvestmentEventSuggestion{}, mapInvestmentSplitError(err)
		}
	}
	accepted, err := s.repository.EventSuggestionByID(ctx, BookID, record.ID)
	if err != nil {
		return InvestmentEventSuggestion{}, fmt.Errorf("reload accepted split suggestion: %w", err)
	}
	return toInvestmentEventSuggestion(accepted), nil
}
