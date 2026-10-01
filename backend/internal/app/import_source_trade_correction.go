package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/exact"
)

type preparedSourceCorrection struct {
	TransactionID int64
	Trade         InvestmentTradeInput
	Revision      db.CommitImportSourceRevisionParams
}

func (s *ImportService) prepareTrading212SourceCorrection(ctx context.Context, input CorrectTrading212SourceInput, kind string) (preparedSourceCorrection, error) {
	if input.OwnerUserID <= 0 || input.BatchID <= 0 || input.RowID <= 0 {
		return preparedSourceCorrection{}, ValidationError{Message: "owner, batch and row ids are required"}
	}
	if strings.TrimSpace(input.Reason) == "" {
		return preparedSourceCorrection{}, ValidationError{Message: "source correction reason is required"}
	}
	if s.investmentService == nil {
		return preparedSourceCorrection{}, ErrImportSourceCorrectionConflict
	}
	batch, err := s.repository.ImportBatchByID(ctx, BookID, input.BatchID)
	if errors.Is(err, db.ErrImportBatchNotFound) {
		return preparedSourceCorrection{}, ErrImportBatchNotFound
	}
	if err != nil {
		return preparedSourceCorrection{}, fmt.Errorf("read source correction batch: %w", err)
	}
	if batch.SourceKind != "trading212" || !batch.ConnectionID.Valid ||
		(batch.Status != "previewing" && batch.Status != "partially_committed" &&
			batch.Status != "committed" && batch.Status != "failed") {
		return preparedSourceCorrection{}, ErrImportSourceCorrectionConflict
	}
	row, err := s.repository.ImportStagedRowByID(ctx, input.RowID)
	if errors.Is(err, db.ErrNotFound) {
		return preparedSourceCorrection{}, ErrImportSourceCorrectionConflict
	}
	if err != nil {
		return preparedSourceCorrection{}, fmt.Errorf("read revised fill: %w", err)
	}
	if row.BatchID != input.BatchID || !row.SourceChanged || !((kind == "buy" && row.SourceBuyOperation) || (kind == "sell" && row.SourceSaleOperation)) || !row.SourceTransactionID.Valid ||
		(row.CommitStatus != "pending" && row.CommitStatus != "skipped") {
		return preparedSourceCorrection{}, ErrImportSourceCorrectionConflict
	}
	var raw map[string]string
	var normalized struct {
		Date          string `json:"date"`
		CommodityHint string `json:"commodity_hint"`
		ExternalRef   string `json:"external_ref"`
	}
	if json.Unmarshal([]byte(row.RawJSON), &raw) != nil ||
		json.Unmarshal([]byte(row.NormalizedJSON), &normalized) != nil ||
		raw[rawKeyKind] != trading212RawKindOrderFill || strings.ToUpper(strings.TrimSpace(raw[rawKeySide])) != strings.ToUpper(kind) {
		return preparedSourceCorrection{}, ErrImportSourceCorrectionConflict
	}
	acceptedRawJSON, _, found, err := s.repository.FindCommittedTrading212FillSnapshot(ctx, BookID, row.DedupeFingerprint)
	if err != nil {
		return preparedSourceCorrection{}, err
	}
	var acceptedRaw map[string]string
	if !found || json.Unmarshal([]byte(acceptedRawJSON), &acceptedRaw) != nil ||
		raw[rawKeyISIN] != acceptedRaw[rawKeyISIN] || raw[rawKeyTicker] != acceptedRaw[rawKeyTicker] {
		return preparedSourceCorrection{}, ValidationError{Message: "source instrument change requires a different correction command"}
	}
	source, err := s.investmentService.TradeCorrectionContext(ctx, input.OwnerUserID, row.SourceTransactionID.Int64)
	if errors.Is(err, ErrInvestmentOperationNotFound) {
		return preparedSourceCorrection{}, ErrImportSourceCorrectionConflict
	}
	if err != nil {
		return preparedSourceCorrection{}, err
	}
	if source.OperationKind != kind || source.SourceKind != "trading212" || source.SourceIdentityID <= 0 {
		return preparedSourceCorrection{}, ErrImportSourceCorrectionConflict
	}
	chain, err := s.investmentService.CorrectionChain(ctx, input.OwnerUserID, source.TransactionID)
	if err != nil {
		return preparedSourceCorrection{}, err
	}
	if chain.EffectiveTransactionID == nil {
		return preparedSourceCorrection{}, ErrImportSourceCorrectionConflict
	}
	effective, err := s.investmentService.TradeCorrectionContext(ctx, input.OwnerUserID, *chain.EffectiveTransactionID)
	if errors.Is(err, ErrInvestmentOperationNotFound) {
		return preparedSourceCorrection{}, ErrImportSourceCorrectionConflict
	}
	if err != nil {
		return preparedSourceCorrection{}, err
	}
	if effective.OperationKind != kind || effective.AlreadyCorrected || len(effective.Charges) != 0 || effective.GrossValue != nil {
		return preparedSourceCorrection{}, ErrImportSourceCorrectionConflict
	}
	currency, err := s.accountRepository.CurrentCurrencyByCode(ctx, BookID, normalized.CommodityHint)
	if err != nil && !errors.Is(err, db.ErrNotFound) {
		return preparedSourceCorrection{}, err
	}
	if currency.ID != effective.CostCommodityID || normalized.Date != effective.EventDate ||
		effective.HoldingAccountID != source.HoldingAccountID || effective.CommodityID != source.CommodityID ||
		effective.CashAccountID != source.CashAccountID {
		return preparedSourceCorrection{}, ValidationError{Message: "source date, account, instrument or currency change requires a different correction command"}
	}
	quantity, quantityScale, err := parseDecimalAmount(raw[rawKeyQuantity])
	if err != nil || quantity.Sign() <= 0 {
		return preparedSourceCorrection{}, ValidationError{Message: "source quantity is invalid"}
	}
	net, _, err := parseDecimalAmount(raw["net_value"])
	if err != nil || (kind == "buy" && net.Sign() >= 0) || (kind == "sell" && net.Sign() <= 0) {
		return preparedSourceCorrection{}, ValidationError{Message: "source settlement sign is invalid for this trade"}
	}
	cashValue, cashScale, err := parsePositiveDecimalAmount(raw["net_value"])
	if err != nil {
		return preparedSourceCorrection{}, ValidationError{Message: "source net settlement is invalid"}
	}
	revision := db.CommitImportSourceRevisionParams{
		BookID: BookID, IdentityID: source.SourceIdentityID, StagedRowID: row.ID,
		SourceOperationID: source.OperationID,
	}
	if err := s.repository.CheckSourceRevision(ctx, revision); err != nil {
		if errors.Is(err, db.ErrImportSourceRevisionConflict) {
			return preparedSourceCorrection{}, ErrImportSourceCorrectionConflict
		}
		return preparedSourceCorrection{}, err
	}
	trade := InvestmentTradeInput{
		TransactionDate: normalized.Date, CommodityID: effective.CommodityID,
		HoldingAccountID: effective.HoldingAccountID, CashAccountID: effective.CashAccountID,
		CashCommodityID: effective.CostCommodityID, QuantityValue: quantity, QuantityScale: quantityScale,
		CashAmountValue: cashValue, CashAmountScale: cashScale,
		Memo: strings.TrimSpace(strings.ToUpper(raw[rawKeySide]) + " " + raw[rawKeyTicker]), ExternalRefHint: normalized.ExternalRef,
		MetadataJSON: trading212ImportMetadata(batch.ConnectionID.Int64, "order_fill", normalized.ExternalRef),
	}
	if kind == "sell" {
		trade.CostBasisMethod = effective.CostBasisMethod
		if effective.CostBasisMethod == "specific_lot" {
			// Provider data contains no lot election. Preserve the effective
			// election for settlement-only corrections; quantity changes need
			// an explicit manual election instead of a guessed redistribution.
			originalQuantity, err := exact.Parse(effective.QuantityValue)
			if err != nil {
				return preparedSourceCorrection{}, err
			}
			if exact.ScaledIntFromCoefficient(quantity, quantityScale).Cmp(exact.ScaledIntFromCoefficient(originalQuantity, effective.QuantityScale)) != 0 {
				return preparedSourceCorrection{}, ValidationError{Message: "specific-lot source quantity changes require an explicit lot election"}
			}
			for _, choice := range effective.EffectiveElectedLots {
				value, err := exact.Parse(choice.QuantityValue)
				if err != nil {
					return preparedSourceCorrection{}, err
				}
				trade.LotAllocations = append(trade.LotAllocations, InvestmentLotAllocationInput{LotID: choice.LotID, QuantityValue: value, QuantityScale: choice.QuantityScale})
			}
		}
	}
	return preparedSourceCorrection{TransactionID: effective.TransactionID, Trade: trade, Revision: revision}, nil
}
