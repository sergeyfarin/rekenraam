package app

import (
	"context"
	"fmt"

	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/exact"
)

// DatedHolding is a long position as a command entered on a date would find
// it, including one a later sale has since closed (#166).
type DatedHolding struct {
	AccountID       int64
	CommodityID     int64
	CostCommodityID int64
	QuantityValue   exact.Coefficient
	QuantityScale   int
	// TransferBasisAllocation is how an outbound transfer from it allocates
	// basis at commit: the current method lock, else the resolved default.
	TransferBasisAllocation string
	Lots                    []db.DatedHoldingLot
}

// DatedHoldings composes every long holding and its open lots at the slot a
// new entry dated asOf takes. It authorizes nothing: the command's writer
// rechecks availability, replay, gains and checkpoints at commit.
func (s *InvestmentService) DatedHoldings(ctx context.Context, ownerUserID int64, asOf string) ([]DatedHolding, error) {
	if ownerUserID <= 0 {
		return nil, ValidationError{Message: "owner is required"}
	}
	date, err := cleanRequiredDate(asOf, "as-of date")
	if err != nil {
		return nil, err
	}
	records, err := s.repository.DatedLongHoldings(ctx, BookID, date)
	if err != nil {
		return nil, fmt.Errorf("read dated holdings: %w", err)
	}
	defaults := make(map[int64]string)
	holdings := make([]DatedHolding, 0, len(records))
	for _, record := range records {
		method, cached := defaults[record.AccountID]
		if !cached {
			if method, _, err = s.resolveCostBasisMethod(ctx, record.AccountID, ""); err != nil {
				return nil, err
			}
			defaults[record.AccountID] = method
		}
		holdings = append(holdings, DatedHolding{
			AccountID: record.AccountID, CommodityID: record.CommodityID, CostCommodityID: record.CostCommodityID,
			QuantityValue: record.QuantityValue, QuantityScale: record.QuantityScale,
			TransferBasisAllocation: db.InternalTransferAllocation(record.MethodFamily, method),
			Lots:                    record.Lots,
		})
	}
	return holdings, nil
}
