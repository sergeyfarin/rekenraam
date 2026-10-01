package app

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"rekenraam/backend/internal/db"
)

// CorrectTrading212Sale accepts provider quantity and positive net settlement
// revisions without changing the effective sale's date, position or election.
func (s *ImportService) CorrectTrading212Sale(ctx context.Context, input CorrectTrading212SaleInput) (ReplaceInvestmentSaleResult, error) {
	prepared, err := s.prepareTrading212SourceCorrection(ctx, input, "sell")
	if err != nil {
		return ReplaceInvestmentSaleResult{}, err
	}
	result, err := s.investmentService.replaceSaleWithPostWriteOrigin(ctx, sourceSaleReplacement(input, prepared),
		"import", "investment.sale.source_correct", func(tx *sql.Tx, correctionOperationID, auditEventID int64) error {
			params := prepared.Revision
			params.CorrectionOperationID = correctionOperationID
			params.CreatedAuditEventID = auditEventID
			params.CreatedAt = s.now().UTC().Format(time.RFC3339)
			return s.repository.CommitSourceRevisionInTx(ctx, tx, params)
		})
	if errors.Is(err, db.ErrImportSourceRevisionConflict) || errors.Is(err, db.ErrImportStagedRowAlreadyCommitted) ||
		errors.Is(err, ErrInvestmentSaleAlreadyCorrected) || errors.Is(err, ErrInvestmentSaleChanged) {
		return ReplaceInvestmentSaleResult{}, ErrImportSourceCorrectionConflict
	}
	return result, err
}

func (s *ImportService) Trading212SaleCorrectionReconciliationImpact(ctx context.Context, input CorrectTrading212SaleInput) (ReconciliationImpact, error) {
	prepared, err := s.prepareTrading212SourceCorrection(ctx, input, "sell")
	if err != nil {
		return ReconciliationImpact{}, err
	}
	return s.investmentService.ReplaceSaleReconciliationImpact(ctx, sourceSaleReplacement(input, prepared))
}

func sourceSaleReplacement(input CorrectTrading212SaleInput, prepared preparedSourceCorrection) ReplaceInvestmentSaleInput {
	return ReplaceInvestmentSaleInput{
		OwnerUserID: input.OwnerUserID, AuthSessionID: input.AuthSessionID,
		RequestID: input.RequestID, TransactionID: prepared.TransactionID,
		Reason: input.Reason, ReconciliationOverride: input.ReconciliationOverride,
		Replacement: prepared.Trade,
	}
}
