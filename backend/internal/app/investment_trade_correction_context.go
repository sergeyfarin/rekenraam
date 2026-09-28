package app

import (
	"context"
	"errors"
	"fmt"

	"rekenraam/backend/internal/db"
)

// TradeCorrectionContext returns recorded source facts for a correction form.
// Eligibility and all values are rechecked by the eventual write command.
func (s *InvestmentService) TradeCorrectionContext(ctx context.Context, ownerUserID, transactionID int64) (db.InvestmentTradeCorrectionContext, error) {
	if ownerUserID <= 0 || transactionID <= 0 {
		return db.InvestmentTradeCorrectionContext{}, ValidationError{Message: "owner and transaction id are required"}
	}
	result, err := s.repository.TradeCorrectionContext(ctx, BookID, transactionID)
	if errors.Is(err, db.ErrNotFound) {
		return db.InvestmentTradeCorrectionContext{}, ErrInvestmentOperationNotFound
	}
	if err != nil {
		return db.InvestmentTradeCorrectionContext{}, fmt.Errorf("read trade correction context: %w", err)
	}
	return result, nil
}
