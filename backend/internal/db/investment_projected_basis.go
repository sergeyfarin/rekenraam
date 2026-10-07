package db

import (
	"database/sql"
	"fmt"

	"rekenraam/backend/internal/exact"
)

const (
	InvestmentBasisKnown   = "known"
	InvestmentBasisUnknown = "unknown"
)

// Empty means known only for existing internal Go callers. Stored facts and
// wire responses always carry explicit knowledge.
func normalizedBasisKnowledge(knowledge string) string {
	if knowledge == "" {
		return InvestmentBasisKnown
	}
	return knowledge
}

func nullableBasisValue(value int64, knowledge string) any {
	if knowledge == InvestmentBasisUnknown {
		return nil
	}
	return value
}

func nullableBasisScale(scale int, knowledge string) any {
	if knowledge == InvestmentBasisUnknown {
		return nil
	}
	return scale
}

// Legacy arithmetic consumers may only use a known amount. The read models
// carry knowledge separately and expose NULL on the wire for unknown basis.
var ErrUnknownInvestmentBasis = fmt.Errorf("%w: remaining investment basis is unknown", ErrInvalidDisposalParams)

func projectedBasis(value, scale sql.NullInt64, knowledge string) (int64, int, error) {
	if knowledge == InvestmentBasisUnknown && !value.Valid && !scale.Valid {
		return 0, 0, nil // Unused numeric fields; callers must carry knowledge.
	}
	if knowledge != InvestmentBasisKnown || !value.Valid || !scale.Valid {
		return 0, 0, fmt.Errorf("invalid projected investment basis knowledge/amount pair")
	}
	return value.Int64, int(scale.Int64), nil
}

// knownInvestmentBasis prevents NULL from entering a known-basis algorithm,
// including average pooling, disposal precision, range admission and replay.
type knownInvestmentBasis int64

func (value *knownInvestmentBasis) Scan(source any) error {
	if source == nil {
		return ErrUnknownInvestmentBasis
	}
	var scanned sql.NullInt64
	if err := scanned.Scan(source); err != nil {
		return err
	}
	*value = knownInvestmentBasis(scanned.Int64)
	return nil
}

type disposalAllocationBasis struct {
	value     int64
	scale     int
	knowledge string
}

// disposalBasisTotal is a decision or revision total. Any unknown allocation
// makes the total unknown; known allocation amounts stay on their own rows.
type disposalBasisTotal struct {
	value     exact.Coefficient
	scale     int
	knowledge string
}

func disposalTotalBasis(allocations []disposalAllocationBasis) (disposalBasisTotal, error) {
	total := exact.NewScaledInt()
	unknown := false
	for _, allocation := range allocations {
		switch normalizedBasisKnowledge(allocation.knowledge) {
		case InvestmentBasisUnknown:
			unknown = true
		case InvestmentBasisKnown:
			total.AddInt64(allocation.value, allocation.scale)
		default:
			return disposalBasisTotal{}, fmt.Errorf("invalid disposal allocation basis knowledge %q", allocation.knowledge)
		}
	}
	if unknown {
		return disposalBasisTotal{knowledge: InvestmentBasisUnknown}, nil
	}
	value, err := total.Coefficient()
	if err != nil {
		return disposalBasisTotal{}, err
	}
	return disposalBasisTotal{value: value, scale: total.Scale(), knowledge: InvestmentBasisKnown}, nil
}

func (total disposalBasisTotal) nullableValue() any {
	if total.knowledge == InvestmentBasisUnknown {
		return nil
	}
	return total.value
}

func (total disposalBasisTotal) nullableScale() any {
	if total.knowledge == InvestmentBasisUnknown {
		return nil
	}
	return total.scale
}
