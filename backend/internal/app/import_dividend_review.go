package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/exact"
)

var ErrImportDividendReviewRequired = errors.New("imported dividend requires owner review")
var ErrImportDividendReviewUnavailable = errors.New("imported dividend review is no longer available")

type ImportDividendReview struct {
	RowID                        int64               `json:"row_id"`
	BatchID                      int64               `json:"batch_id"`
	RowIndex                     int                 `json:"row_index"`
	State                        string              `json:"state"`
	SourceInstrument             string              `json:"source_instrument"`
	SourceDate                   string              `json:"source_date"`
	SourceCashAmount             string              `json:"source_cash_amount"`
	SourceCurrency               string              `json:"source_currency"`
	TransactionID                *int64              `json:"transaction_id,omitempty"`
	ExpectedTransactionVersionID *int64              `json:"expected_transaction_version_id,omitempty"`
	Resolution                   ImportRowResolution `json:"resolution"`
}

type ImportDividendReviews struct {
	Rows       []ImportDividendReview `json:"rows"`
	NextCursor *string                `json:"next_cursor"`
}
type ListImportDividendsInput struct {
	OwnerUserID, BatchID int64
	Cursor               string
	Limit                int
}
type ReviewImportDividendInput struct {
	OwnerUserID, AuthSessionID, BatchID, RowID, ExpectedTransactionVersionID int64
	RequestID, Reason                                                        string
	Resolution                                                               ImportRowResolution
	ReconciliationOverride                                                   CheckpointOverride
}

type importDividendSource struct {
	Date          string `json:"date"`
	Amount        string `json:"amount"`
	CommodityHint string `json:"commodity_hint"`
	Memo          string `json:"memo"`
	ExternalRef   string `json:"external_ref"`
}
type importDividendContext struct {
	row        db.ImportStagedRowRecord
	batch      db.ImportBatchRecord
	raw        map[string]string
	source     importDividendSource
	identity   db.ImportCommitIdentityRecord
	original   *db.TransactionRecord
	resolution ImportRowResolution
}

func isTrading212DividendRow(batch db.ImportBatchRecord, row db.ImportStagedRowRecord) bool {
	var raw map[string]string
	return batch.SourceKind == "trading212" && json.Unmarshal([]byte(row.RawJSON), &raw) == nil && raw[rawKeyKind] == trading212RawKindDividend
}

func (s *ImportService) importDividendContext(ctx context.Context, ownerID, batchID, rowID int64) (importDividendContext, error) {
	var c importDividendContext
	if ownerID <= 0 || batchID <= 0 || rowID <= 0 {
		return c, ValidationError{Message: "owner, batch and dividend row are required"}
	}
	if s.investmentService == nil {
		return c, ErrImportDividendReviewUnavailable
	}
	var err error
	c.batch, err = s.repository.ImportBatchByID(ctx, BookID, batchID)
	if errors.Is(err, db.ErrImportBatchNotFound) {
		return c, ErrImportBatchNotFound
	}
	if err != nil {
		return c, err
	}
	c.row, err = s.repository.ImportStagedRowByID(ctx, rowID)
	if errors.Is(err, db.ErrNotFound) {
		return c, ErrImportDividendReviewUnavailable
	}
	if err != nil {
		return c, err
	}
	if c.row.BatchID != batchID || !isTrading212DividendRow(c.batch, c.row) || c.batch.Status == "discarded" || c.batch.Status == "rolled_back" {
		return c, ErrImportDividendReviewUnavailable
	}
	if err = json.Unmarshal([]byte(c.row.RawJSON), &c.raw); err != nil {
		return c, err
	}
	if err = json.Unmarshal([]byte(c.row.NormalizedJSON), &c.source); err != nil {
		return c, err
	}
	c.resolution, err = parseResolutionJSON(c.row.ResolutionJSON)
	if err != nil {
		return c, err
	}
	identity, found, err := s.repository.FindCommitIdentity(ctx, BookID, c.row.DedupeFingerprint)
	if err != nil {
		return c, err
	}
	if found {
		if identity.SourceKind != "trading212" || identity.CommittedTransactionID <= 0 {
			return c, ErrImportDividendReviewUnavailable
		}
		record, err := s.transactionService.repository.TransactionByID(ctx, BookID, identity.CommittedTransactionID)
		if err != nil {
			return c, err
		}
		linked, err := s.transactionService.repository.TransactionHasInvestmentLinks(ctx, BookID, record.ID)
		if err != nil {
			return c, err
		}
		if linked || record.Status != "posted" || record.TransactionKind != "ordinary" || record.DeletedAt.Valid {
			return c, ErrImportDividendReviewUnavailable
		}
		c.identity, c.original = identity, &record
	} else if (c.row.CommitStatus != "pending" && c.row.CommitStatus != "failed") || c.row.DedupeStatus == "duplicate" || c.row.DedupeStatus == "excluded" || (c.batch.Status != "previewing" && c.batch.Status != "partially_committed" && !(c.batch.Status == "failed" && c.row.CommitStatus == "failed")) {
		return c, ErrImportDividendReviewUnavailable
	}
	return c, nil
}

func (s *ImportService) dividendReviewDefaults(ctx context.Context, c importDividendContext, res ImportRowResolution) (ImportRowResolution, error) {
	ctx, referenceErr := s.settings.referenceContext(ctx, s.now())
	if referenceErr != nil {
		return ImportRowResolution{}, referenceErr
	}

	if c.original != nil {
		// Historical settlement is pinned by the original import effect, not a
		// connection that may have since been edited or removed.
		res.AccountID = c.identity.AccountID
	}
	if res.AccountID == 0 && c.batch.ConnectionID.Valid && s.connectionService != nil {
		conn, err := s.connectionService.GetImportConnection(ctx, c.batch.ConnectionID.Int64)
		if err != nil && !errors.Is(err, ErrImportConnectionNotFound) {
			return res, err
		}
		if err == nil && conn.CashAccountID != nil {
			res.AccountID = *conn.CashAccountID
		}
	}
	if res.CommodityID == 0 {
		currency, err := s.accountRepository.CurrentCurrencyByCode(ctx, BookID, c.source.CommodityHint)
		if err != nil && !errors.Is(err, db.ErrNotFound) {
			return res, err
		}
		if err == nil {
			res.CommodityID = currency.ID
		}
	}
	if res.InstrumentCommodityID == 0 {
		_, id, found, err := s.investmentService.FindInstrumentForImport(ctx, c.raw[rawKeyISIN], c.raw[rawKeyTicker])
		if err != nil {
			return res, err
		}
		if found {
			res.InstrumentCommodityID = id
		}
	}
	if res.CategoryID == nil || res.WithholdingAccountID == nil {
		defaults, _, err := s.investmentService.applicableDividendDefault(ctx, res.InstrumentCommodityID, c.source.Date)
		if err != nil && !errors.Is(err, db.ErrNotFound) {
			return res, err
		}
		if err == nil {
			if res.CategoryID == nil && defaults.IncomeAccountID > 0 {
				id := defaults.IncomeAccountID
				res.CategoryID = &id
			}
			if res.WithholdingAccountID == nil && defaults.WithholdingAccountID.Valid {
				id := defaults.WithholdingAccountID.Int64
				res.WithholdingAccountID = &id
			}
		}
	}
	return res, nil
}

func (s *ImportService) ListImportDividends(ctx context.Context, input ListImportDividendsInput) (ImportDividendReviews, error) {
	result := ImportDividendReviews{Rows: []ImportDividendReview{}}
	if input.OwnerUserID <= 0 || input.BatchID < 0 {
		return result, ValidationError{Message: "invalid dividend review filters"}
	}
	var after int64
	if input.Cursor != "" {
		var err error
		after, err = strconv.ParseInt(input.Cursor, 10, 64)
		if err != nil || after <= 0 {
			return result, ValidationError{Message: "invalid dividend review cursor"}
		}
	}
	limit := input.Limit
	if limit == 0 {
		limit = 25
	}
	if limit < 1 || limit > 100 {
		return result, ValidationError{Message: "invalid dividend review limit"}
	}
	rows, err := s.repository.ListImportDividendReviewRows(ctx, BookID, input.BatchID, after, limit+1)
	if err != nil {
		return result, err
	}
	if len(rows) > limit {
		rows = rows[:limit]
		cursor := strconv.FormatInt(rows[len(rows)-1].ID, 10)
		result.NextCursor = &cursor
	}
	for _, row := range rows {
		c, err := s.importDividendContext(ctx, input.OwnerUserID, row.BatchID, row.ID)
		if errors.Is(err, ErrImportDividendReviewUnavailable) {
			continue
		}
		if err != nil {
			return result, err
		}
		res, err := s.dividendReviewDefaults(ctx, c, c.resolution)
		if err != nil {
			return result, err
		}
		review := ImportDividendReview{RowID: row.ID, BatchID: row.BatchID, RowIndex: row.RowIndex, State: "pending", SourceInstrument: strings.TrimSpace(c.raw[rawKeyTicker] + " " + c.raw[rawKeyISIN]), SourceDate: c.source.Date, SourceCashAmount: c.source.Amount, SourceCurrency: c.source.CommodityHint, Resolution: res}
		if c.original != nil {
			review.State = "repairable"
			id, version := c.original.ID, c.original.VersionID
			review.TransactionID = &id
			review.ExpectedTransactionVersionID = &version
		}
		result.Rows = append(result.Rows, review)
	}
	return result, nil
}

func (s *ImportService) reviewedDividendInput(ctx context.Context, c importDividendContext, resolution ImportRowResolution, ownerID, sessionID int64, requestID string) (DividendInput, ImportRowResolution, error) {
	ctx, referenceErr := s.settings.referenceContext(ctx, s.now())
	if referenceErr != nil {
		return DividendInput{}, ImportRowResolution{}, referenceErr
	}

	refuse := func() (DividendInput, ImportRowResolution, error) {
		return DividendInput{}, resolution, ErrImportDividendReviewRequired
	}
	res, err := s.dividendReviewDefaults(ctx, c, resolution)
	if err != nil {
		return DividendInput{}, res, err
	}
	if res.AccountID <= 0 || res.CommodityID <= 0 || res.CategoryID == nil || res.GrossAmount == nil || res.WithholdingAmount == nil || res.TransferAccountID != nil || (res.InvestmentKind != "" && res.InvestmentKind != "dividend") {
		return refuse()
	}
	gross, gs, ge := parseDecimalAmount(*res.GrossAmount)
	tax, ts, te := parseDecimalAmount(*res.WithholdingAmount)
	cash, cs, ce := parseDecimalAmount(c.source.Amount)
	if ge != nil || te != nil || ce != nil || gross.Sign() <= 0 || tax.Sign() < 0 || cash.Sign() <= 0 {
		return refuse()
	}
	net := exact.ScaledIntFromCoefficient(gross, gs)
	net.SubScaled(exact.ScaledIntFromCoefficient(tax, ts))
	if net.Cmp(exact.ScaledIntFromCoefficient(cash, cs)) != 0 {
		return refuse()
	}
	// The source currency is independent evidence, not replaceable by review.
	currency, err := s.accountRepository.CurrentCurrencyByCode(ctx, BookID, c.source.CommodityHint)
	if err != nil && !errors.Is(err, db.ErrNotFound) {
		return DividendInput{}, res, err
	}
	if err == nil && currency.ID != res.CommodityID {
		return refuse()
	}
	amount, err := exact.ScaledIntFromCoefficient(gross, gs).Int64()
	if err != nil {
		return DividendInput{}, res, err
	}
	input := DividendInput{OwnerUserID: ownerID, AuthSessionID: sessionID, RequestID: requestID, NeedsReview: true, TransactionDate: c.source.Date, CashAccountID: res.AccountID, CashCommodityID: res.CommodityID, IncomeAccountID: res.CategoryID, AmountValue: amount, AmountScale: gs, WithholdingAccountID: res.WithholdingAccountID, Memo: c.source.Memo, PayeeID: res.PayeeID, PayeeName: res.PayeeName, TagIDs: res.TagIDs, ExternalRefHint: c.source.ExternalRef, OriginType: "import", Operation: "investment.dividend.review", ChangeReason: "reviewed imported dividend"}
	if res.InstrumentCommodityID > 0 {
		input.CommodityID = NullableValue(res.InstrumentCommodityID)
	}
	if tax.Sign() > 0 {
		value, err := exact.ScaledIntFromCoefficient(tax, ts).Int64()
		if err != nil {
			return DividendInput{}, res, err
		}
		input.WithholdingValue, input.WithholdingScale = &value, &ts
	}
	if c.batch.ConnectionID.Valid {
		input.MetadataJSON = trading212ImportMetadata(c.batch.ConnectionID.Int64, "dividend", c.source.ExternalRef)
	}
	return input, res, nil
}

func dividendDeclarationEvidence(row db.ImportStagedRowRecord, res ImportRowResolution) string {
	evidence, _ := json.Marshal(map[string]any{"origin": "owner_review", "import_row_id": row.ID, "resolution": res, "previous_resolution": json.RawMessage(defaultString(row.ResolutionJSON, "{}"))})
	return string(evidence)
}

func (s *ImportService) commitReviewedTrading212DividendRow(ctx context.Context, row db.ImportStagedRowRecord, batch db.ImportBatchRecord, input CommitImportBatchInput, now string) (bool, int64, int64, error) {
	c, err := s.importDividendContext(ctx, input.OwnerUserID, batch.ID, row.ID)
	if err != nil {
		return true, 0, 0, err
	}
	if c.original != nil {
		return true, 0, 0, ErrImportDividendReviewUnavailable
	}
	dividend, res, err := s.reviewedDividendInput(ctx, c, c.resolution, input.OwnerUserID, input.AuthSessionID, input.RequestID)
	if err != nil {
		return true, 0, 0, err
	}
	dividend.ReconciliationOverride = input.ReconciliationOverride
	record, err := s.writeReviewedPendingDividend(ctx, c, dividend, res, now, false)
	return true, record.ID, dividend.CashAccountID, err
}

func (s *ImportService) writeReviewedPendingDividend(ctx context.Context, c importDividendContext, input DividendInput, res ImportRowResolution, now string, finishBatch bool) (db.TransactionRecord, error) {
	// Validate roles, exact economics and existing checkpoint impact before
	// creating any new instrument. The writer rechecks all financial guards.
	impact, err := s.investmentService.DividendReconciliationImpact(ctx, input)
	if err != nil {
		return db.TransactionRecord{}, err
	}
	if len(impact.AffectedCheckpoints) > 0 && !input.ReconciliationOverride.Allowed {
		return db.TransactionRecord{}, ErrReconciliationOverrideRequired
	}
	var instrumentID, commodityID int64
	var created bool
	if input.CommodityID.Value == nil && strings.TrimSpace(c.raw[rawKeyTicker]) != "" {
		instrumentID, commodityID, created, err = s.investmentService.ResolveOrCreateInstrumentForImport(ctx, input.OwnerUserID, input.AuthSessionID, input.RequestID, c.raw[rawKeyISIN], c.raw[rawKeyTicker])
		if err != nil {
			return db.TransactionRecord{}, err
		}
		input.CommodityID = NullableValue(commodityID)
	}
	params, err := s.investmentService.prepareDividendWrite(ctx, input)
	if err == nil {
		for i := range params.InvestmentComponents {
			params.InvestmentComponents[i].SourceEvidenceJSON = dividendDeclarationEvidence(c.row, res)
		}
		var encoded []byte
		encoded, err = json.Marshal(res)
		if err == nil {
			record, writeErr := s.transactionService.repository.CreateTransactionWithPostWrite(ctx, params, func(tx *sql.Tx, transactionID int64) error {
				if err := s.repository.CheckPendingDividendRowInTx(ctx, tx, c.row, c.row.ResolutionJSON); err != nil {
					return err
				}
				if err := s.repository.SaveDividendResolutionInTx(ctx, tx, c.row.ID, string(encoded)); err != nil {
					return err
				}
				if err := s.recordCommitIdentityAndMarkRowInTx(ctx, tx, c.row.ID, c.row.DedupeFingerprint, c.batch.SourceKind, input.CashAccountID, transactionID, now); err != nil {
					return err
				}
				if finishBatch {
					return s.repository.FinishDividendReviewInTx(ctx, tx, c.batch.ID, transactionID, now)
				}
				return nil
			})
			if writeErr == nil {
				return record, nil
			}
			err = writeErr
		}
	}
	if created {
		err = s.cleanupTrading212InvestmentSetup(ctx, c.batch.ConnectionID.Int64, commodityID, instrumentID, created, trading212HoldingResolution{}, input.OwnerUserID, err)
	}
	return db.TransactionRecord{}, mapTransactionDBError(err)
}

func (s *ImportService) prepareImportDividendRepair(ctx context.Context, c importDividendContext, input ReviewImportDividendInput) (db.RepairImportDividendParams, error) {
	var p db.RepairImportDividendParams
	if c.original == nil || input.ExpectedTransactionVersionID != c.original.VersionID {
		return p, ErrTransactionVersionStale
	}
	reason, err := cleanChangeReason(input.Reason, "")
	if err != nil {
		return p, err
	}
	if reason == "" {
		return p, ValidationError{Message: "repair reason is required"}
	}
	dividend, res, err := s.reviewedDividendInput(ctx, c, input.Resolution, input.OwnerUserID, input.AuthSessionID, input.RequestID)
	if err != nil {
		return p, err
	}
	if input.Resolution.AccountID > 0 && input.Resolution.AccountID != c.identity.AccountID {
		return p, ValidationError{Message: "repair must retain the source cash account"}
	}
	if c.original.TransactionDate != dividend.TransactionDate {
		return p, ErrImportDividendReviewUnavailable
	}
	// A repaired version must represent the same actual payment as the source.
	cash := exact.NewScaledInt()
	for _, entry := range c.original.JournalEntries {
		if entry.EntryDate != dividend.TransactionDate {
			return p, ErrImportDividendReviewUnavailable
		}
		for _, posting := range entry.Postings {
			if posting.CommodityID != dividend.CashCommodityID {
				return p, ErrImportDividendReviewUnavailable
			}
			if posting.AccountID == dividend.CashAccountID {
				cash.AddCoefficient(posting.QuantityValue, posting.QuantityScale)
			}
		}
	}
	sourceCash, sourceScale, err := parseDecimalAmount(c.source.Amount)
	if err != nil || cash.Cmp(exact.ScaledIntFromCoefficient(sourceCash, sourceScale)) != 0 {
		return p, ErrImportDividendReviewUnavailable
	}
	dividend.ChangeReason = reason
	dividend.ReconciliationOverride = input.ReconciliationOverride
	params, err := s.investmentService.prepareDividendWrite(ctx, dividend)
	if err != nil {
		return p, err
	}
	plan, err := s.investmentService.dividendPlan(ctx, dividend)
	if err != nil {
		return p, err
	}
	original := transactionInputFromTransaction(toTransaction(*c.original))
	spec := plan.Create.Spec
	spec.NeedsReview = c.original.NeedsReview
	spec.ExternalRefHint = original.ExternalRefHint
	spec.PayeeID, spec.PayeeName = original.PayeeID, original.PayeeName
	spec.Description, spec.NoteMarkdown, spec.MetadataJSON = original.Description, original.NoteMarkdown, original.MetadataJSON
	spec.TagIDs = original.TagIDs
	used := map[string]bool{}
	for ei := range spec.JournalEntries {
		if ei < len(original.JournalEntries) {
			spec.JournalEntries[ei].Memo = original.JournalEntries[ei].Memo
			spec.JournalEntries[ei].MetadataJSON = original.JournalEntries[ei].MetadataJSON
		}
		for pi := range spec.JournalEntries[ei].Postings {
			posting := &spec.JournalEntries[ei].Postings[pi]
			for _, entry := range original.JournalEntries {
				for _, old := range entry.Postings {
					if !used[old.LineKey] && old.AccountID == posting.AccountID && old.CommodityID == posting.CommodityID {
						posting.LineKey, posting.Memo, posting.MetadataJSON, posting.TagIDs = old.LineKey, old.Memo, old.MetadataJSON, old.TagIDs
						used[old.LineKey] = true
						break
					}
				}
				if posting.LineKey != "" {
					break
				}
			}
		}
	}
	deps := newAccountRuleDependencies()
	cleaned, err := s.transactionService.cleanTransactionSpec(ctx, spec, cleanTransactionOptions{ForcedStatus: "posted", ExistingLineKeys: lineKeySet(*c.original), ExistingPostings: existingPostingStateSet(*c.original), ExistingTagIDs: tagIDSet(c.original.TagIDs), AccountRuleDependencies: deps})
	if err != nil {
		return p, err
	}
	params.Spec = cleaned
	// Role dependencies from prepareDividendWrite and posting dependencies
	// from cleaning both describe the same accounts; retain the role snapshot.
	for i := range params.InvestmentComponents {
		params.InvestmentComponents[i].SourceEvidenceJSON = dividendDeclarationEvidence(c.row, res)
	}
	encoded, err := json.Marshal(res)
	if err != nil {
		return p, err
	}
	return db.RepairImportDividendParams{RowID: c.row.ID, BatchID: c.batch.ID, IdentityID: c.identity.ID, TransactionID: c.original.ID, ExpectedVersionID: input.ExpectedTransactionVersionID, ResolutionJSON: string(encoded), Journal: params, CheckpointCandidates: reconciliationCandidates(*c.original, cleaned)}, nil
}

func (s *ImportService) ReviewImportDividend(ctx context.Context, input ReviewImportDividendInput) (Transaction, error) {
	c, err := s.importDividendContext(ctx, input.OwnerUserID, input.BatchID, input.RowID)
	if err != nil {
		return Transaction{}, err
	}
	var record db.TransactionRecord
	if c.original != nil {
		p, prepareErr := s.prepareImportDividendRepair(ctx, c, input)
		if prepareErr != nil {
			return Transaction{}, mapImportDividendReviewError(prepareErr)
		}
		record, err = s.writeImportDividendRepair(ctx, c, input, p)
	} else {
		if input.ExpectedTransactionVersionID != 0 {
			return Transaction{}, ErrTransactionVersionStale
		}
		dividend, res, prepareErr := s.reviewedDividendInput(ctx, c, input.Resolution, input.OwnerUserID, input.AuthSessionID, input.RequestID)
		if prepareErr != nil {
			return Transaction{}, mapImportDividendReviewError(prepareErr)
		}
		dividend.ReconciliationOverride = input.ReconciliationOverride
		record, err = s.writeReviewedPendingDividend(ctx, c, dividend, res, s.now().UTC().Format(time.RFC3339), true)
	}
	if err != nil {
		return Transaction{}, mapImportDividendReviewError(err)
	}
	return s.transactionService.enrichOne(ctx, toTransaction(record))
}

// Preview validates the unassigned shape without creating setup. At commit,
// resolve the recorded source identity just as for a new imported dividend.
func (s *ImportService) writeImportDividendRepair(ctx context.Context, c importDividendContext, input ReviewImportDividendInput, p db.RepairImportDividendParams) (db.TransactionRecord, error) {
	var instrumentID, commodityID int64
	var created bool
	var err error
	if p.Journal.Dividend != nil && !p.Journal.Dividend.CommodityID.Valid && strings.TrimSpace(c.raw[rawKeyTicker]) != "" {
		instrumentID, commodityID, created, err = s.investmentService.ResolveOrCreateInstrumentForImport(ctx, input.OwnerUserID, input.AuthSessionID, input.RequestID, c.raw[rawKeyISIN], c.raw[rawKeyTicker])
		if err != nil {
			return db.TransactionRecord{}, err
		}
		input.Resolution.InstrumentCommodityID = commodityID
		p, err = s.prepareImportDividendRepair(ctx, c, input)
	}
	var record db.TransactionRecord
	if err == nil {
		record, err = s.repository.RepairImportDividend(ctx, p, false)
	}
	if err != nil && created {
		err = s.cleanupTrading212InvestmentSetup(ctx, c.batch.ConnectionID.Int64, commodityID, instrumentID, true, trading212HoldingResolution{}, input.OwnerUserID, err)
	}
	return record, err
}

func (s *ImportService) ImportDividendReviewImpact(ctx context.Context, input ReviewImportDividendInput) (ReconciliationImpact, error) {
	c, err := s.importDividendContext(ctx, input.OwnerUserID, input.BatchID, input.RowID)
	if err != nil {
		return ReconciliationImpact{}, err
	}
	if c.original == nil {
		if input.ExpectedTransactionVersionID != 0 {
			return ReconciliationImpact{}, ErrTransactionVersionStale
		}
		dividend, _, err := s.reviewedDividendInput(ctx, c, input.Resolution, input.OwnerUserID, input.AuthSessionID, input.RequestID)
		if err != nil {
			return ReconciliationImpact{}, mapImportDividendReviewError(err)
		}
		return s.investmentService.DividendReconciliationImpact(ctx, dividend)
	}
	input.ReconciliationOverride = unboundOverride
	p, err := s.prepareImportDividendRepair(ctx, c, input)
	if err != nil {
		return ReconciliationImpact{}, mapImportDividendReviewError(err)
	}
	record, err := s.repository.RepairImportDividend(ctx, p, true)
	if err != nil {
		return ReconciliationImpact{}, mapImportDividendReviewError(err)
	}
	return s.investmentService.simulatedReconciliationImpact(ctx, db.SimulatedInvestmentWrite{InvalidatedCheckpointRefs: record.InvalidatedCheckpointRefs})
}

func mapImportDividendReviewError(err error) error {
	if errors.Is(err, ErrImportDividendReviewRequired) {
		return ValidationError{Message: "confirm dividend gross, withholding and accounts; gross minus withholding must equal source cash"}
	}
	if errors.Is(err, db.ErrImportDividendReviewUnavailable) {
		return ErrImportDividendReviewUnavailable
	}
	return mapTransactionDBError(err)
}
