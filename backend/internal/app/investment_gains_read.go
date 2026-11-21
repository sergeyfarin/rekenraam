package app

import (
	"context"
	"fmt"
	"time"

	"rekenraam/backend/internal/db"
)

// GainsReadInput describes one live gains response, including optional
// comparisons and reporting figures. Separate requests may see later history.
type GainsReadInput struct {
	Period            GainsReportParams
	ValuationAsOf     string
	AlternativeMethod string
	JurisdictionNote  string
	FX                *GainsFXInput
}

type GainsRead struct {
	GainsRunProjection
	Alternative *AlternativeGains
	Reporting   *GainsFXProjection
}

type GainsStatementRead struct {
	Statement  GainsStatement
	Currencies []Currency
	Reporting  *GainsFXStatement
}

// SetGainsReadStageForTest installs a deterministic interleaving at composition
// boundaries. Set it before serving requests; production leaves it nil.
func (s *InvestmentService) SetGainsReadStageForTest(hook func(string)) {
	s.gainsReadStageForTest = hook
}

func (s *InvestmentService) gainsReadStage(stage string) {
	if s.gainsReadStageForTest != nil {
		s.gainsReadStageForTest(stage)
	}
}

// ReadGains composes every live response component from one disposable SQLite
// image. Replay writes roll back there, and no report-run or audit is saved.
func (s *InvestmentService) ReadGains(ctx context.Context, input GainsReadInput) (GainsRead, error) {
	var result GainsRead
	err := s.withGainsReader(ctx, func(ctx context.Context, reader *InvestmentService, currencies *CurrencyService) error {
		var err error
		result.Currencies, err = currencies.ListCurrencies(ctx)
		if err != nil {
			return err
		}
		result.Realized, err = reader.ListRealizedGains(ctx, input.Period)
		if err != nil {
			return err
		}
		result.CapitalReturns, err = reader.ListCapitalReturns(ctx, input.Period)
		if err != nil {
			return err
		}
		result.Profile, result.Unrealized, err = reader.UnrealizedGainsProjection(ctx, input.ValuationAsOf)
		if err != nil {
			return err
		}
		reader.gainsReadStage("operational")
		if input.AlternativeMethod != "" {
			alternative, err := reader.AlternativeGainsForProfile(ctx, input.AlternativeMethod, input.JurisdictionNote, result.Profile, input.Period)
			if err != nil {
				return err
			}
			result.Alternative = &alternative
		}
		reader.gainsReadStage("comparison")
		if input.FX != nil {
			reporting, err := reader.ConvertGains(ctx, result.Profile, input.Period, result.Realized, result.Unrealized, *input.FX)
			if err != nil {
				return err
			}
			result.Reporting = &reporting
		}
		return nil
	})
	if err != nil {
		return GainsRead{}, err
	}
	return result, nil
}

// ReadGainsStatement captures both native boundaries, period movements and
// optional reporting conversion together, under today's effective history.
func (s *InvestmentService) ReadGainsStatement(ctx context.Context, from, to string, fx *GainsFXInput) (GainsStatementRead, error) {
	var result GainsStatementRead
	err := s.withGainsReader(ctx, func(ctx context.Context, reader *InvestmentService, currencies *CurrencyService) error {
		var err error
		result.Statement, err = reader.GainsStatement(ctx, from, to)
		if err != nil {
			return err
		}
		result.Currencies, err = currencies.ListCurrencies(ctx)
		if err != nil {
			return err
		}
		reader.gainsReadStage("statement")
		if fx != nil {
			reporting, err := reader.ConvertGainsStatement(ctx, result.Statement, *fx)
			if err != nil {
				return err
			}
			result.Reporting = &reporting
		}
		return nil
	})
	if err != nil {
		return GainsStatementRead{}, err
	}
	return result, nil
}

func (s *InvestmentService) withGainsReader(ctx context.Context, read func(context.Context, *InvestmentService, *CurrencyService) error) error {
	snapshot, cleanup, err := s.repository.LiveGainsSnapshot(ctx)
	if err != nil {
		return fmt.Errorf("capture live gains input: %w", err)
	}
	defer cleanup()
	captured := s.now().UTC()
	ctx, err = NewSettingsService(db.NewSettingsRepository(snapshot)).referenceContext(ctx, captured)
	if err != nil {
		return fmt.Errorf("resolve live gains reference day: %w", err)
	}
	// Every dependency, including reporting-currency metadata and rates, must
	// belong to the image. Do not shallow-copy services with live repositories.
	transactions := &TransactionService{pricingRepository: db.NewPricingRepository(snapshot)}
	reader := NewInvestmentService(db.NewInvestmentRepository(snapshot), nil, transactions, nil)
	reader.now = func() time.Time { return captured }
	reader.consistentGainsRead = true
	reader.gainsReadStageForTest = s.gainsReadStageForTest
	return read(ctx, reader, NewCurrencyService(db.NewCommodityRepository(snapshot), nil))
}
