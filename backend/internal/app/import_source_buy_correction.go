package app

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"rekenraam/backend/internal/db"
)

var ErrImportSourceCorrectionConflict = errors.New("import source correction is no longer eligible")

type CorrectTrading212SourceInput struct {
	OwnerUserID            int64
	AuthSessionID          int64
	RequestID              string
	BatchID                int64
	RowID                  int64
	Reason                 string
	ReconciliationOverride bool
}

type CorrectTrading212BuyInput = CorrectTrading212SourceInput
type CorrectTrading212SaleInput = CorrectTrading212SourceInput

// CorrectTrading212Buy accepts a revised provider fill through the audited
// buy replacement writer. Only quantity and net settlement changes to the
// same dated instrument, holding, cash currency, and connection are supported.
func (s *ImportService) CorrectTrading212Buy(ctx context.Context, input CorrectTrading212BuyInput) (ReplaceInvestmentBuyResult, error) {
	prepared, err := s.prepareTrading212SourceCorrection(ctx, input, "buy")
	if err != nil {
		return ReplaceInvestmentBuyResult{}, err
	}
	result, err := s.investmentService.replaceBuyWithPostWriteOrigin(ctx, sourceBuyReplacement(input, prepared),
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
	prepared, err := s.prepareTrading212SourceCorrection(ctx, input, "buy")
	if err != nil {
		return ReconciliationImpact{}, err
	}
	return s.investmentService.ReplaceBuyReconciliationImpact(ctx, sourceBuyReplacement(input, prepared))
}

func sourceBuyReplacement(input CorrectTrading212BuyInput, prepared preparedSourceCorrection) ReplaceInvestmentBuyInput {
	return ReplaceInvestmentBuyInput{
		OwnerUserID: input.OwnerUserID, AuthSessionID: input.AuthSessionID,
		RequestID: input.RequestID, TransactionID: prepared.TransactionID,
		Reason: input.Reason, ReconciliationOverride: input.ReconciliationOverride,
		Replacement: prepared.Trade,
	}
}
