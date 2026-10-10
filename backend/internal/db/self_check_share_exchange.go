package db

import (
	"context"
	"database/sql"
	"fmt"
	"math/big"

	"rekenraam/backend/internal/exact"
)

// SelfCheckShareExchange is one share exchange (#177) folded exactly: whether
// every link's destination holds its source quantity times the recorded
// ratio, and the link totals beside what the primary journal posted to the
// holding and commodity_trading in each instrument.
type SelfCheckShareExchange struct {
	OperationID  int64
	Links        int
	RatioBroken  bool
	Source       *exact.ScaledInt // sum of link (old) quantities
	Destination  *exact.ScaledInt // sum of destination lot quantities
	OldHolding   *exact.ScaledInt
	OldTrading   *exact.ScaledInt
	NewHolding   *exact.ScaledInt
	NewTrading   *exact.ScaledInt
	StrayPosting bool // a primary posting outside the four legs
}

// Agrees reports whether the exchange converted every link at its ratio and
// posted exactly the four legs its links moved.
func (e SelfCheckShareExchange) Agrees() bool {
	return e.Links > 0 && !e.RatioBroken && !e.StrayPosting &&
		e.OldHolding.Cmp(e.Source.Negated()) == 0 && e.OldTrading.Cmp(e.Source) == 0 &&
		e.NewHolding.Cmp(e.Destination) == 0 && e.NewTrading.Cmp(e.Destination.Negated()) == 0
}

// SelfCheckShareExchanges reads every share exchange's links and primary
// journal postings. Coefficients are compared and summed in Go.
func (r *SelfCheckRepository) SelfCheckShareExchanges(ctx context.Context, transaction *sql.Tx, bookID int64) ([]SelfCheckShareExchange, error) {
	exchanges := make(map[int64]*SelfCheckShareExchange)
	var order []int64
	get := func(operationID int64) *SelfCheckShareExchange {
		exchange, exists := exchanges[operationID]
		if !exists {
			exchange = &SelfCheckShareExchange{OperationID: operationID, Source: exact.NewScaledInt(),
				Destination: exact.NewScaledInt(), OldHolding: exact.NewScaledInt(), OldTrading: exact.NewScaledInt(),
				NewHolding: exact.NewScaledInt(), NewTrading: exact.NewScaledInt()}
			exchanges[operationID] = exchange
			order = append(order, operationID)
		}
		return exchange
	}
	links, err := transaction.QueryContext(ctx, `
		SELECT o.id, f.ratio_numerator, f.ratio_denominator, x.quantity_value, x.quantity_scale,
			d.quantity_value, d.quantity_scale
		FROM investment_operations o
		LEFT JOIN investment_transfer_facts f ON f.operation_id = o.id AND f.transfer_kind = 'exchange'
		LEFT JOIN investment_transfer_lot_links x ON x.operation_id = f.operation_id
		LEFT JOIN investment_lots d ON d.id = x.destination_lot_id
		WHERE o.book_id = ? AND o.operation_kind = 'share_exchange'
		ORDER BY o.id, x.link_seq`, bookID)
	if err != nil {
		return nil, fmt.Errorf("read self-check share exchange links: %w", err)
	}
	defer links.Close()
	for links.Next() {
		var operationID int64
		var numerator, denominator, sourceScale, destinationScale sql.NullInt64
		var source, destination sql.NullString
		if err := links.Scan(&operationID, &numerator, &denominator, &source, &sourceScale,
			&destination, &destinationScale); err != nil {
			return nil, fmt.Errorf("scan self-check share exchange link: %w", err)
		}
		exchange := get(operationID)
		if !source.Valid {
			continue
		}
		exchange.Links++
		if !destination.Valid || !numerator.Valid || !denominator.Valid || denominator.Int64 <= 0 {
			exchange.RatioBroken = true
			continue
		}
		sourceQuantity := exact.ScaledIntFromCoefficient(exact.Coefficient(source.String), int(sourceScale.Int64))
		destinationQuantity := exact.ScaledIntFromCoefficient(exact.Coefficient(destination.String), int(destinationScale.Int64))
		exchange.Source.AddScaled(sourceQuantity)
		exchange.Destination.AddScaled(destinationQuantity)
		// destination × denominator = source × numerator, at one scale.
		scale := max(sourceQuantity.Scale(), destinationQuantity.Scale())
		sourceQuantity.Align(scale)
		destinationQuantity.Align(scale)
		left := new(big.Int).Mul(destinationQuantity.BigInt(), big.NewInt(denominator.Int64))
		right := new(big.Int).Mul(sourceQuantity.BigInt(), big.NewInt(numerator.Int64))
		if left.Cmp(right) != 0 {
			exchange.RatioBroken = true
		}
	}
	if err := links.Err(); err != nil {
		return nil, fmt.Errorf("iterate self-check share exchange links: %w", err)
	}
	postings, err := transaction.QueryContext(ctx, `
		SELECT f.operation_id, pv.commodity_id = f.commodity_id, pv.commodity_id = f.destination_commodity_id,
			pv.account_id = f.source_account_id, pv.account_id = f.destination_account_id,
			a.system_role IS 'commodity_trading', pv.quantity_value, pv.quantity_scale
		FROM investment_transfer_facts f
		JOIN investment_operation_journal_links link ON link.operation_id = f.operation_id AND link.role = 'primary'
		JOIN posting_versions pv ON pv.transaction_version_id = link.transaction_version_id
		JOIN accounts a ON a.id = pv.account_id
		WHERE f.book_id = ? AND f.transfer_kind = 'exchange'
		ORDER BY f.operation_id`, bookID)
	if err != nil {
		return nil, fmt.Errorf("read self-check share exchange postings: %w", err)
	}
	defer postings.Close()
	for postings.Next() {
		var operationID int64
		var oldCommodity, newCommodity, sourceHolding, destinationHolding, trading bool
		var value exact.Coefficient
		var scale int
		if err := postings.Scan(&operationID, &oldCommodity, &newCommodity, &sourceHolding, &destinationHolding,
			&trading, &value, &scale); err != nil {
			return nil, fmt.Errorf("scan self-check share exchange posting: %w", err)
		}
		exchange := get(operationID)
		amount := exact.ScaledIntFromCoefficient(value, scale)
		switch {
		case oldCommodity && sourceHolding:
			exchange.OldHolding.AddScaled(amount)
		case oldCommodity && trading:
			exchange.OldTrading.AddScaled(amount)
		case newCommodity && destinationHolding:
			exchange.NewHolding.AddScaled(amount)
		case newCommodity && trading:
			exchange.NewTrading.AddScaled(amount)
		default:
			exchange.StrayPosting = true
		}
	}
	if err := postings.Err(); err != nil {
		return nil, fmt.Errorf("iterate self-check share exchange postings: %w", err)
	}
	result := make([]SelfCheckShareExchange, 0, len(order))
	for _, operationID := range order {
		result = append(result, *exchanges[operationID])
	}
	return result, nil
}
