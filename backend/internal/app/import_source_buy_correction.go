package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"rekenraam/backend/internal/db"
)

var ErrImportSourceCorrectionConflict = errors.New("import source correction is no longer eligible")

type CorrectTrading212BuyInput struct {
	OwnerUserID            int64
	AuthSessionID          int64
	RequestID              string
	BatchID                int64
	RowID                  int64
	Reason                 string
	ReconciliationOverride bool
}

// CorrectTrading212Buy accepts a revised provider fill through the audited
// buy replacement writer. Only quantity and net settlement changes to the
// same dated instrument, holding, cash currency, and connection are supported.
func (s *ImportService) CorrectTrading212Buy(ctx context.Context, input CorrectTrading212BuyInput) (ReplaceInvestmentBuyResult, error) {
	prepared, err := s.prepareTrading212BuyCorrection(ctx, input)
	if err != nil {
		return ReplaceInvestmentBuyResult{}, err
	}
	result, err := s.investmentService.replaceBuyWithPostWriteOrigin(ctx, prepared.Replacement,
		"import", "investment.buy.source_correct", func(tx *sql.Tx, correctionOperationID, auditEventID int64) error {
			params := prepared.Revision
			params.CorrectionOperationID = correctionOperationID
			params.CreatedAuditEventID = auditEventID
			params.CreatedAt = s.now().UTC().Format(time.RFC3339)
			return s.repository.CommitSourceRevisionInTx(ctx, tx, params)
		})
	if errors.Is(err, db.ErrImportSourceRevisionConflict) || errors.Is(err, db.ErrImportStagedRowAlreadyCommitted) ||
		errors.Is(err, ErrInvestmentBuyAlreadyCorrected) || errors.Is(err, ErrInvestmentBuyChanged) {
		return ReplaceInvestmentBuyResult{}, ErrImportSourceCorrectionConflict
	}
	return result, err
}

// Trading212BuyCorrectionReconciliationImpact reads the same staged provider
// values and eligibility as the command, without posting or accepting a revision.
// The command still rechecks source eligibility in its write transaction.
func (s *ImportService) Trading212BuyCorrectionReconciliationImpact(ctx context.Context, input CorrectTrading212BuyInput) (ReconciliationImpact, error) {
	prepared, err := s.prepareTrading212BuyCorrection(ctx, input)
	if err != nil {
		return ReconciliationImpact{}, err
	}
	return s.investmentService.ReplaceBuyReconciliationImpact(ctx, prepared.Replacement)
}

type preparedSourceBuyCorrection struct {
	Replacement ReplaceInvestmentBuyInput
	Revision    db.CommitImportSourceRevisionParams
}

func (s *ImportService) prepareTrading212BuyCorrection(ctx context.Context, input CorrectTrading212BuyInput) (preparedSourceBuyCorrection, error) {
	if input.OwnerUserID <= 0 || input.BatchID <= 0 || input.RowID <= 0 {
		return preparedSourceBuyCorrection{}, ValidationError{Message: "owner, batch and row ids are required"}
	}
	if strings.TrimSpace(input.Reason) == "" {
		return preparedSourceBuyCorrection{}, ValidationError{Message: "source correction reason is required"}
	}
	if s.investmentService == nil {
		return preparedSourceBuyCorrection{}, ErrImportSourceCorrectionConflict
	}
	batch, err := s.repository.ImportBatchByID(ctx, BookID, input.BatchID)
	if errors.Is(err, db.ErrImportBatchNotFound) {
		return preparedSourceBuyCorrection{}, ErrImportBatchNotFound
	}
	if err != nil {
		return preparedSourceBuyCorrection{}, fmt.Errorf("read source correction batch: %w", err)
	}
	if batch.SourceKind != "trading212" || !batch.ConnectionID.Valid ||
		(batch.Status != "previewing" && batch.Status != "partially_committed" &&
			batch.Status != "committed" && batch.Status != "failed") {
		return preparedSourceBuyCorrection{}, ErrImportSourceCorrectionConflict
	}
	row, err := s.repository.ImportStagedRowByID(ctx, input.RowID)
	if errors.Is(err, db.ErrNotFound) {
		return preparedSourceBuyCorrection{}, ErrImportSourceCorrectionConflict
	}
	if err != nil {
		return preparedSourceBuyCorrection{}, fmt.Errorf("read revised fill: %w", err)
	}
	if row.BatchID != input.BatchID || !row.SourceChanged || !row.SourceBuyOperation || !row.SourceTransactionID.Valid ||
		(row.CommitStatus != "pending" && row.CommitStatus != "skipped") {
		return preparedSourceBuyCorrection{}, ErrImportSourceCorrectionConflict
	}
	var raw map[string]string
	var normalized struct {
		Date          string `json:"date"`
		CommodityHint string `json:"commodity_hint"`
		ExternalRef   string `json:"external_ref"`
	}
	if json.Unmarshal([]byte(row.RawJSON), &raw) != nil ||
		json.Unmarshal([]byte(row.NormalizedJSON), &normalized) != nil ||
		raw[rawKeyKind] != trading212RawKindOrderFill || strings.ToUpper(strings.TrimSpace(raw[rawKeySide])) != "BUY" {
		return preparedSourceBuyCorrection{}, ErrImportSourceCorrectionConflict
	}
	acceptedRawJSON, _, found, err := s.repository.FindCommittedTrading212FillSnapshot(ctx, BookID, row.DedupeFingerprint)
	if err != nil {
		return preparedSourceBuyCorrection{}, err
	}
	var acceptedRaw map[string]string
	if !found || json.Unmarshal([]byte(acceptedRawJSON), &acceptedRaw) != nil ||
		raw[rawKeyISIN] != acceptedRaw[rawKeyISIN] || raw[rawKeyTicker] != acceptedRaw[rawKeyTicker] {
		return preparedSourceBuyCorrection{}, ValidationError{Message: "source instrument change requires a different correction command"}
	}
	source, err := s.investmentService.TradeCorrectionContext(ctx, input.OwnerUserID, row.SourceTransactionID.Int64)
	if errors.Is(err, ErrInvestmentOperationNotFound) {
		return preparedSourceBuyCorrection{}, ErrImportSourceCorrectionConflict
	}
	if err != nil {
		return preparedSourceBuyCorrection{}, err
	}
	if source.OperationKind != "buy" || source.SourceKind != "trading212" || source.SourceIdentityID <= 0 {
		return preparedSourceBuyCorrection{}, ErrImportSourceCorrectionConflict
	}
	chain, err := s.investmentService.CorrectionChain(ctx, input.OwnerUserID, source.TransactionID)
	if err != nil {
		return preparedSourceBuyCorrection{}, err
	}
	if chain.EffectiveTransactionID == nil {
		return preparedSourceBuyCorrection{}, ErrImportSourceCorrectionConflict
	}
	effective, err := s.investmentService.TradeCorrectionContext(ctx, input.OwnerUserID, *chain.EffectiveTransactionID)
	if errors.Is(err, ErrInvestmentOperationNotFound) {
		return preparedSourceBuyCorrection{}, ErrImportSourceCorrectionConflict
	}
	if err != nil {
		return preparedSourceBuyCorrection{}, err
	}
	if effective.OperationKind != "buy" || effective.AlreadyCorrected || len(effective.Charges) != 0 || effective.GrossValue != nil {
		return preparedSourceBuyCorrection{}, ErrImportSourceCorrectionConflict
	}
	currency, err := s.accountRepository.CurrentCurrencyByCode(ctx, BookID, normalized.CommodityHint)
	if err != nil && !errors.Is(err, db.ErrNotFound) {
		return preparedSourceBuyCorrection{}, err
	}
	if currency.ID != effective.CostCommodityID || normalized.Date != effective.EventDate ||
		effective.HoldingAccountID != source.HoldingAccountID || effective.CommodityID != source.CommodityID ||
		effective.CashAccountID != source.CashAccountID {
		return preparedSourceBuyCorrection{}, ValidationError{Message: "source date, account, instrument or currency change requires a different correction command"}
	}
	quantity, quantityScale, err := parseDecimalAmount(raw[rawKeyQuantity])
	if err != nil || quantity.Sign() <= 0 {
		return preparedSourceBuyCorrection{}, ValidationError{Message: "source quantity is invalid"}
	}
	net, _, err := parseDecimalAmount(raw["net_value"])
	if err != nil || net.Sign() >= 0 {
		return preparedSourceBuyCorrection{}, ValidationError{Message: "source buy net settlement must be negative"}
	}
	cashValue, cashScale, err := parsePositiveDecimalAmount(raw["net_value"])
	if err != nil {
		return preparedSourceBuyCorrection{}, ValidationError{Message: "source net settlement is invalid"}
	}
	revision := db.CommitImportSourceRevisionParams{
		BookID: BookID, IdentityID: source.SourceIdentityID, StagedRowID: row.ID,
		SourceOperationID: source.OperationID,
	}
	if err := s.repository.CheckSourceRevision(ctx, revision); err != nil {
		if errors.Is(err, db.ErrImportSourceRevisionConflict) {
			return preparedSourceBuyCorrection{}, ErrImportSourceCorrectionConflict
		}
		return preparedSourceBuyCorrection{}, err
	}
	return preparedSourceBuyCorrection{Replacement: ReplaceInvestmentBuyInput{
		OwnerUserID: input.OwnerUserID, AuthSessionID: input.AuthSessionID,
		RequestID: input.RequestID, TransactionID: effective.TransactionID,
		Reason: input.Reason, ReconciliationOverride: input.ReconciliationOverride,
		Replacement: InvestmentTradeInput{
			TransactionDate: normalized.Date, CommodityID: effective.CommodityID,
			HoldingAccountID: effective.HoldingAccountID, CashAccountID: effective.CashAccountID,
			CashCommodityID: effective.CostCommodityID, QuantityValue: quantity, QuantityScale: quantityScale,
			CashAmountValue: cashValue, CashAmountScale: cashScale,
			Memo: strings.TrimSpace("BUY " + raw[rawKeyTicker]), ExternalRefHint: normalized.ExternalRef,
			MetadataJSON: trading212ImportMetadata(batch.ConnectionID.Int64, "order_fill", normalized.ExternalRef),
		},
	}, Revision: revision}, nil
}
