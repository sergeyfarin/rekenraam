package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/onlinesource/trading212"
)

// trading212FetchWorkKind is the durable work queue kind processed by
// ImportService.StartBackgroundWorker. Payload shape per
// docs/trading212-import-plan.md "Data model".
const trading212FetchWorkKind = "import.fetch.trading212"

// maxTrading212FetchAttempts bounds retries before a fetch is given up on as
// terminal. Unlike FX coverage (which retries forever), an import fetch that
// keeps failing must eventually surface to the user as a failed batch rather
// than leave it "fetching" indefinitely.
const maxTrading212FetchAttempts = 8

// ErrImportFetchInProgress is returned when a connection already has an
// unfinished fetch (an existing previewing batch with fetch_status=fetching).
// The durable queue does not coalesce different connection/cursor payloads,
// so this guard lives at the service layer (see "Durable fetch worker" in
// the import plan).
var ErrImportFetchInProgress = errors.New("an import fetch is already in progress for this connection")

type trading212FetchWorkPayload struct {
	ConnectionID int64  `json:"connection_id"`
	BatchID      int64  `json:"batch_id"`
	Reason       string `json:"reason"` // "manual" | "continuation"
	Cursor       string `json:"cursor"` // incremental fetch cursor (empty = full)
}

// trading212BatchMeta is the JSON written to import_batches.source_meta_json
// for online batches. It deliberately carries connection_display_name (a
// snapshot, not a live join) so a batch's provenance stays readable even
// after the connection that produced it is deleted (closes T-12).
type trading212BatchMeta struct {
	FetchStatus           string         `json:"fetch_status"` // "fetching" | "ready" | "failed"
	ConnectionDisplayName string         `json:"connection_display_name,omitempty"`
	AccountHints          []string       `json:"account_hints,omitempty"`
	CurrencyHints         []string       `json:"currency_hints,omitempty"`
	DateFrom              string         `json:"date_from,omitempty"`
	DateTo                string         `json:"date_to,omitempty"`
	Warnings              []ParseWarning `json:"warnings,omitempty"`
	Error                 string         `json:"error,omitempty"`
}

// --- Service entry points (HTTP-facing) ---

// StartOnlineImport enqueues a full Trading 212 fetch (empty cursor) and
// creates the previewing batch it will stage rows into. Mirrors StartImport
// but the fetch runs on the durable queue instead of inline.
func (s *ImportService) StartOnlineImport(ctx context.Context, input StartOnlineImportInput) (StartOnlineImportResult, error) {
	if input.OwnerUserID <= 0 {
		return StartOnlineImportResult{}, ValidationError{Message: "owner user is required"}
	}
	if s.connectionService == nil || s.backgroundWork == nil {
		return StartOnlineImportResult{}, fmt.Errorf("online import is not configured")
	}

	conn, err := s.connectionService.GetImportConnection(ctx, input.ConnectionID)
	if err != nil {
		return StartOnlineImportResult{}, err
	}

	batch, err := s.startTrading212Fetch(ctx, trading212FetchStartParams{
		Connection:    conn,
		Cursor:        "",
		Reason:        "manual",
		OwnerUserID:   input.OwnerUserID,
		AuthSessionID: input.AuthSessionID,
		RequestID:     input.RequestID,
	})
	if err != nil {
		return StartOnlineImportResult{}, err
	}
	return StartOnlineImportResult{Batch: batch}, nil
}

// RefreshImportConnection enqueues an incremental fetch using the
// connection's saved cursor, opening a fresh batch for the new rows.
func (s *ImportService) RefreshImportConnection(ctx context.Context, input RefreshImportConnectionInput) (StartOnlineImportResult, error) {
	if input.OwnerUserID <= 0 {
		return StartOnlineImportResult{}, ValidationError{Message: "owner user is required"}
	}
	if s.connectionService == nil || s.backgroundWork == nil {
		return StartOnlineImportResult{}, fmt.Errorf("online import is not configured")
	}

	conn, err := s.connectionService.GetImportConnection(ctx, input.ConnectionID)
	if err != nil {
		return StartOnlineImportResult{}, err
	}

	batch, err := s.startTrading212Fetch(ctx, trading212FetchStartParams{
		Connection:    conn,
		Cursor:        conn.FetchCursor,
		Reason:        "manual",
		OwnerUserID:   input.OwnerUserID,
		AuthSessionID: input.AuthSessionID,
		RequestID:     input.RequestID,
	})
	if err != nil {
		return StartOnlineImportResult{}, err
	}
	return StartOnlineImportResult{Batch: batch}, nil
}

type trading212FetchStartParams struct {
	Connection    ImportConnection
	Cursor        string
	Reason        string
	OwnerUserID   int64
	AuthSessionID int64
	RequestID     string
}

// startTrading212Fetch creates the previewing batch and enqueues the durable
// fetch work item. Shared by StartOnlineImport (full fetch) and
// RefreshImportConnection (incremental fetch).
func (s *ImportService) startTrading212Fetch(ctx context.Context, params trading212FetchStartParams) (ImportBatch, error) {
	if params.Connection.SourceKind != "trading212" {
		return ImportBatch{}, ValidationError{Message: "unsupported online source"}
	}

	inFlight, err := s.repository.HasInFlightFetch(ctx, BookID, params.Connection.ID)
	if err != nil {
		return ImportBatch{}, fmt.Errorf("check in-flight fetch: %w", err)
	}
	if inFlight {
		return ImportBatch{}, ErrImportFetchInProgress
	}

	nowStr := s.now().UTC().Format(time.RFC3339)
	metaJSON, _ := json.Marshal(trading212BatchMeta{
		FetchStatus:           "fetching",
		ConnectionDisplayName: params.Connection.DisplayName,
	})

	batch, err := s.repository.CreateImportBatch(ctx, db.CreateImportBatchParams{
		BookID:         BookID,
		SourceKind:     params.Connection.SourceKind,
		ConnectionID:   sql.NullInt64{Int64: params.Connection.ID, Valid: true},
		SourceMetaJSON: string(metaJSON),
		CreatedAt:      nowStr,
		ActorUserID:    params.OwnerUserID,
		AuthSessionID:  params.AuthSessionID,
		RequestID:      params.RequestID,
	})
	if err != nil {
		return ImportBatch{}, fmt.Errorf("create online import batch: %w", err)
	}

	payload := trading212FetchWorkPayload{
		ConnectionID: params.Connection.ID,
		BatchID:      batch.ID,
		Reason:       params.Reason,
		Cursor:       params.Cursor,
	}
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return ImportBatch{}, fmt.Errorf("marshal fetch work payload: %w", err)
	}
	if _, err := s.backgroundWork.EnqueueBackgroundWork(ctx, BookID, trading212FetchWorkKind, string(payloadJSON), nowStr); err != nil {
		return ImportBatch{}, fmt.Errorf("enqueue import fetch work: %w", err)
	}

	return toImportBatch(batch), nil
}

// --- Durable worker ---

// StartBackgroundWorker starts the Trading 212 fetch worker loop, mirroring
// PricingService.StartBackgroundWorker (pricing_worker.go). No-ops if the
// service was constructed without online-import dependencies.
func (s *ImportService) StartBackgroundWorker(ctx context.Context, logger *slog.Logger) {
	if s.backgroundWork == nil || s.connectionService == nil {
		return
	}
	if logger == nil {
		logger = slog.Default()
	}
	workerID := uuid.NewString()
	go func() {
		s.runDueTrading212Fetches(ctx, logger, workerID)
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.runDueTrading212Fetches(ctx, logger, workerID)
			}
		}
	}()
}

func (s *ImportService) runDueTrading212Fetches(ctx context.Context, logger *slog.Logger, workerID string) {
	for processed := 0; processed < 4; processed++ {
		now := s.now().UTC()
		item, err := s.backgroundWork.ClaimBackgroundWork(ctx, trading212FetchWorkKind, workerID, now.Format(time.RFC3339), now.Add(15*time.Minute).Format(time.RFC3339))
		if errors.Is(err, db.ErrNotFound) || ctx.Err() != nil {
			return
		}
		if err != nil {
			logger.WarnContext(ctx, "claim import fetch work", slog.Any("err", err))
			return
		}
		s.processTrading212FetchWork(ctx, logger, workerID, item)
	}
}

func (s *ImportService) processTrading212FetchWork(ctx context.Context, logger *slog.Logger, workerID string, item db.BackgroundWorkItemRecord) {
	var payload trading212FetchWorkPayload
	if item.PayloadVersion != 1 || json.Unmarshal([]byte(item.PayloadJSON), &payload) != nil || payload.ConnectionID <= 0 || payload.BatchID <= 0 {
		if err := s.backgroundWork.FailBackgroundWork(ctx, item.ID, workerID, s.now().UTC().Format(time.RFC3339), "invalid import fetch work payload"); err != nil {
			logger.WarnContext(ctx, "fail invalid import fetch work", slog.Int64("work_id", item.ID), slog.Any("err", err))
		}
		return
	}

	err := s.runTrading212Fetch(ctx, payload)
	if err == nil {
		now := s.now().UTC().Format(time.RFC3339)
		if completeErr := s.backgroundWork.CompleteBackgroundWork(ctx, item.ID, workerID, now); completeErr != nil {
			logger.WarnContext(ctx, "complete import fetch work", slog.Int64("work_id", item.ID), slog.Any("err", completeErr))
		}
		return
	}
	if ctx.Err() != nil {
		return
	}

	terminal := errors.Is(err, trading212.ErrUnauthorized) || item.Attempts >= maxTrading212FetchAttempts
	if !terminal {
		now := s.now().UTC()
		delay := retryDelay(item.Attempts, item.ID)
		if retryErr := s.backgroundWork.RetryBackgroundWork(ctx, item.ID, workerID, now.Add(delay).Format(time.RFC3339), now.Format(time.RFC3339), err.Error()); retryErr != nil {
			logger.WarnContext(ctx, "schedule import fetch retry", slog.Int64("work_id", item.ID), slog.Any("err", retryErr))
			return
		}
		logger.WarnContext(ctx, "import fetch will retry", slog.Int64("work_id", item.ID), slog.Duration("retry_in", delay), slog.Any("err", err))
		return
	}

	now := s.now().UTC().Format(time.RFC3339)
	if failErr := s.backgroundWork.FailBackgroundWork(ctx, item.ID, workerID, now, err.Error()); failErr != nil {
		logger.WarnContext(ctx, "fail import fetch work", slog.Int64("work_id", item.ID), slog.Any("err", failErr))
		return
	}
	s.markTrading212FetchTerminalFailure(ctx, logger, payload, err, now)
}

// runTrading212Fetch performs one fetch attempt: load connection + secret,
// call the fetcher, parse via the registered Trading212Adapter (reusing the
// same RawInput.Bytes contract every adapter uses), stage rows through the
// shared stageParseResult pipeline, and persist the new cursor. Returns nil
// on success (including the no-op case where the batch was closed by the
// user mid-fetch).
func (s *ImportService) runTrading212Fetch(ctx context.Context, payload trading212FetchWorkPayload) error {
	batch, err := s.repository.ImportBatchByID(ctx, BookID, payload.BatchID)
	if err != nil {
		return fmt.Errorf("load import batch: %w", err)
	}
	if batch.Status != "previewing" {
		return nil // discarded or committed already; nothing to do.
	}

	conn, err := s.connectionService.GetImportConnection(ctx, payload.ConnectionID)
	if err != nil {
		return fmt.Errorf("load import connection: %w", err)
	}
	apiKey, err := s.connectionService.OpenSecret(ctx, payload.ConnectionID)
	if err != nil {
		return fmt.Errorf("open import connection secret: %w", err)
	}

	baseURL := trading212BaseURLFromConfig(conn.ConfigJSON)
	fetcher := trading212.NewFetcher(s.httpClient, baseURL)
	result, err := fetcher.Fetch(ctx, apiKey, payload.Cursor)
	if err != nil {
		return err
	}

	rawBytes, err := json.Marshal(trading212FetchPayload{
		ConnectionID: payload.ConnectionID,
		Movements:    toAdapterMovements(result.Movements),
	})
	if err != nil {
		return fmt.Errorf("marshal trading212 fetch payload: %w", err)
	}

	adapter, ok := s.registry.byKindName("trading212")
	if !ok {
		return fmt.Errorf("trading212 adapter not registered")
	}
	parseResult, err := adapter.Parse(ctx, RawInput{Bytes: rawBytes}, nil)
	if err != nil {
		return fmt.Errorf("parse trading212 fetch: %w", err)
	}

	// Re-check: fetching can take a while; the user may have discarded or
	// committed the batch while it was in flight.
	batch, err = s.repository.ImportBatchByID(ctx, BookID, payload.BatchID)
	if err != nil {
		return fmt.Errorf("reload import batch: %w", err)
	}
	if batch.Status != "previewing" {
		return nil
	}

	if _, err := s.stageParseResult(ctx, payload.BatchID, parseResult); err != nil {
		return fmt.Errorf("stage trading212 rows: %w", err)
	}

	now := s.now().UTC().Format(time.RFC3339)
	metaJSON, _ := json.Marshal(trading212BatchMeta{
		FetchStatus:           "ready",
		ConnectionDisplayName: conn.DisplayName,
		AccountHints:          parseResult.Meta.AccountHints,
		CurrencyHints:         parseResult.Meta.CurrencyHints,
		DateFrom:              parseResult.Meta.DateFrom,
		DateTo:                parseResult.Meta.DateTo,
		Warnings:              parseResult.Warnings,
	})
	if err := s.repository.UpdateImportBatchSourceMeta(ctx, payload.BatchID, string(metaJSON)); err != nil {
		return fmt.Errorf("update batch source meta: %w", err)
	}

	cursor := result.Cursor
	if cursor == "" {
		cursor = payload.Cursor // no movements seen this run; keep the existing cursor.
	}
	if err := s.connectionService.UpdateFetchCursor(ctx, payload.ConnectionID, cursor, "ready", now); err != nil {
		return fmt.Errorf("update connection cursor: %w", err)
	}

	return nil
}

func (s *ImportService) markTrading212FetchTerminalFailure(ctx context.Context, logger *slog.Logger, payload trading212FetchWorkPayload, fetchErr error, now string) {
	metaJSON, _ := json.Marshal(trading212BatchMeta{FetchStatus: "failed", Error: fetchErr.Error()})
	batch, err := s.repository.ImportBatchByID(ctx, BookID, payload.BatchID)
	if err == nil && batch.Status == "previewing" {
		if err := s.repository.UpdateImportBatchStatus(ctx, db.UpdateImportBatchStatusParams{
			BatchID:    payload.BatchID,
			Status:     "failed",
			EventKind:  "failed",
			DetailJSON: string(metaJSON),
			OccurredAt: now,
		}); err != nil {
			logger.WarnContext(ctx, "mark import batch failed", slog.Int64("batch_id", payload.BatchID), slog.Any("err", err))
		}
	} else if err != nil {
		logger.WarnContext(ctx, "load import batch for terminal failure", slog.Int64("batch_id", payload.BatchID), slog.Any("err", err))
	}

	if err := s.connectionService.MarkFetchFailed(ctx, payload.ConnectionID, now); err != nil {
		logger.WarnContext(ctx, "mark connection fetch failed", slog.Int64("connection_id", payload.ConnectionID), slog.Any("err", err))
	}
}

func toAdapterMovements(movements []trading212.Movement) []trading212Movement {
	out := make([]trading212Movement, len(movements))
	for i, m := range movements {
		out[i] = trading212Movement{
			ID:        m.ID,
			Type:      m.Type,
			Timestamp: m.Timestamp,
			Amount:    m.Amount,
			Currency:  m.Currency,
			Notes:     m.Notes,
		}
	}
	return out
}
