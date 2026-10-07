package db

import (
	"database/sql"
	"fmt"
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
