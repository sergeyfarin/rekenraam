package app

import (
	"context"
	"errors"

	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/exact"
)

var ErrInvestmentOperationNotFound = errors.New("investment operation not found")

type InvestmentCorrectionNode struct {
	OperationID             int64
	TransactionID           *int64
	OperationKind           string
	EventDate               string
	CorrectionOfOperationID *int64
	CorrectionMode          string
	CorrectionReason        string
	CreatedAt               string
	AuditEventID            int64
	Imported                bool
	Effective               bool
}

type InvestmentCorrectionChain struct {
	RootOperationID        int64
	EffectiveTransactionID *int64
	CanReverseManualSale   bool
	CanReverseManualBuy    bool
	CanReverseSale         bool
	CanReverseBuy          bool
	// CanCorrectSplit allows native split reversal and replacement (T-129);
	// EffectiveSplit then carries the split's terms for a replacement form.
	CanCorrectSplit bool
	EffectiveSplit  *InvestmentCorrectionSplitTerms
	// CanCorrectDividend / CanCorrectReinvestedDividend allow native reversal
	// and replacement (T-115); the Effective* terms pre-fill the replacement.
	CanCorrectDividend bool
	// CanReverseCapitalReturn allows reversing a return of capital (T-148).
	CanReverseCapitalReturn      bool
	CanReplaceCapitalReturn      bool
	EffectiveCapitalReturn       *CapitalReturnInput
	EffectiveDividend            *InvestmentCorrectionDividendTerms
	CanCorrectReinvestedDividend bool
	EffectiveReinvestment        *InvestmentCorrectionReinvestmentTerms
	// CanCorrectWriteOff allows native write-off reversal and replacement
	// (T-118); the trade correction context pre-fills the replacement.
	CanCorrectWriteOff   bool
	CanCorrectCashInLieu bool
	// CanReverseTransfer allows native reversal of an internal or
	// external-in transfer (T-119).
	CanReverseTransfer bool
	// CanReplaceTransfer allows native replacement of an internal or
	// external-in transfer (T-119); EffectiveTransfer pre-fills it.
	CanReplaceTransfer bool
	EffectiveTransfer  *InvestmentCorrectionTransferTerms
	Operations         []InvestmentCorrectionNode
}

// InvestmentCorrectionDividendTerms are an effective cash dividend's posted
// facts. The date, cash account and currency are fixed for a replacement.
type InvestmentCorrectionDividendTerms struct {
	EventDate            string
	CashAccountID        int64
	CashCommodityID      int64
	IncomeAccountID      int64
	AmountValue          exact.Coefficient
	AmountScale          int
	WithholdingAccountID *int64
	WithholdingValue     *exact.Coefficient
	WithholdingScale     *int
	Memo                 string
	PayeeID              *int64
}

// InvestmentCorrectionReinvestmentTerms are an effective reinvested
// dividend's posted facts. Date, holding, instrument and currency are fixed.
type InvestmentCorrectionReinvestmentTerms struct {
	EventDate        string
	HoldingAccountID int64
	CommodityID      int64
	CashCommodityID  int64
	IncomeAccountID  int64
	QuantityValue    exact.Coefficient
	QuantityScale    int
	AmountValue      exact.Coefficient
	AmountScale      int
	Memo             string
	PayeeID          *int64
}

// InvestmentCorrectionTransferTerms are an effective transfer's committed
// terms, which pre-fill its replacement form (T-119). SourceAccountID is zero
// and the carried basis and original date are set for an external transfer in.
type InvestmentCorrectionTransferTerms struct {
	TransferKind         string
	EffectiveOn          string
	SourceAccountID      int64
	DestinationAccountID int64
	CommodityID          int64
	CostCommodityID      int64
	BasisAllocation      string
	DestinationLineage   string
	Allocations          []InvestmentLotAllocationInput
	QuantityValue        exact.Coefficient
	QuantityScale        int
	CarriedBasisValue    exact.Coefficient
	CarriedBasisScale    int
	OriginalAcquiredOn   string
	SourceEvidenceJSON   string
	Memo                 string
}

// InvestmentCorrectionSplitTerms are an effective split's current terms.
type InvestmentCorrectionSplitTerms struct {
	HoldingAccountID int64
	CommodityID      int64
	EffectiveOn      string
	RatioNumerator   int64
	RatioDenominator int64
}

// CorrectionChain explains immutable investment history by transaction ID.
// The final reversal remains visible but contributes no effective position
// intent; a final replacement is the new effective operation.
func (s *InvestmentService) CorrectionChain(ctx context.Context, ownerUserID, transactionID int64) (InvestmentCorrectionChain, error) {
	if ownerUserID <= 0 || transactionID <= 0 {
		return InvestmentCorrectionChain{}, ValidationError{Message: "owner and transaction ids are required"}
	}
	records, err := s.repository.CorrectionChainByTransactionID(ctx, BookID, transactionID)
	if errors.Is(err, db.ErrNotFound) {
		return InvestmentCorrectionChain{}, ErrInvestmentOperationNotFound
	}
	if err != nil {
		return InvestmentCorrectionChain{}, err
	}
	chain := InvestmentCorrectionChain{
		RootOperationID: records[0].OperationID,
		Operations:      make([]InvestmentCorrectionNode, 0, len(records)),
	}
	importedLineage := false
	for _, record := range records {
		importedLineage = importedLineage || record.Imported
	}
	committedSource := false
	if importedLineage {
		committedSource, err = s.repository.HasCommittedImportSource(ctx, BookID, records[len(records)-1].OperationID)
		if err != nil {
			return InvestmentCorrectionChain{}, err
		}
	}
	for index, record := range records {
		isLast := index == len(records)-1
		effective := isLast && record.CorrectionMode.String != "reverse"
		node := InvestmentCorrectionNode{
			OperationID: record.OperationID, OperationKind: record.OperationKind,
			EventDate: record.EventDate, CorrectionMode: record.CorrectionMode.String,
			CorrectionReason: record.CorrectionReason.String, CreatedAt: record.CreatedAt,
			AuditEventID: record.AuditEventID, Imported: record.Imported, Effective: effective,
		}
		if record.TransactionID.Valid {
			id := record.TransactionID.Int64
			node.TransactionID = &id
			if effective {
				chain.EffectiveTransactionID = &id
			}
		}
		if record.CorrectionOfOperationID.Valid {
			id := record.CorrectionOfOperationID.Int64
			node.CorrectionOfOperationID = &id
		}
		chain.Operations = append(chain.Operations, node)
		if effective && record.OperationKind == "sell" && (!importedLineage || committedSource) &&
			record.TransactionStatus.String == "posted" && !record.TransactionDeleted {
			chain.CanReverseSale = true
			chain.CanReverseManualSale = !importedLineage
		}
		if effective && record.OperationKind == "split" && (!importedLineage || committedSource) &&
			record.TransactionStatus.String == "posted" && !record.TransactionDeleted {
			chain.CanCorrectSplit = true
			split, err := s.repository.SplitOperationByTransactionID(ctx, BookID, record.TransactionID.Int64)
			if err != nil {
				return InvestmentCorrectionChain{}, mapSplitCorrectionError(err)
			}
			chain.EffectiveSplit = &InvestmentCorrectionSplitTerms{
				HoldingAccountID: split.AccountID, CommodityID: split.CommodityID, EffectiveOn: split.EventDate,
				RatioNumerator: split.RatioNumerator, RatioDenominator: split.RatioDenominator,
			}
		}
		correctable := effective && (!importedLineage || committedSource) &&
			record.TransactionStatus.String == "posted" && !record.TransactionDeleted
		if correctable && record.OperationKind == "dividend" {
			transaction, err := s.transactionService.Transaction(ctx, record.TransactionID.Int64)
			if err != nil {
				return InvestmentCorrectionChain{}, err
			}
			chain.EffectiveDividend, chain.CanCorrectDividend = dividendCorrectionTerms(transaction)
		}
		if correctable && record.OperationKind == "reinvested_dividend" && !importedLineage {
			transaction, err := s.transactionService.Transaction(ctx, record.TransactionID.Int64)
			if err != nil {
				return InvestmentCorrectionChain{}, err
			}
			chain.EffectiveReinvestment, chain.CanCorrectReinvestedDividend = reinvestmentCorrectionTerms(transaction)
		}
		if correctable && record.OperationKind == "return_of_capital" {
			chain.CanReverseCapitalReturn = true
			terms, err := s.repository.CapitalReturnTerms(ctx, BookID, record.OperationID)
			if err != nil {
				return InvestmentCorrectionChain{}, err
			}
			transaction, err := s.transactionService.Transaction(ctx, record.TransactionID.Int64)
			if err != nil {
				return InvestmentCorrectionChain{}, err
			}
			chain.CanReplaceCapitalReturn = true
			chain.EffectiveCapitalReturn = &CapitalReturnInput{
				HoldingAccountID: terms.AccountID, CommodityID: terms.CommodityID, CurrencyID: terms.CostCommodityID,
				CashAccountID: terms.CashAccountID, EffectiveOn: terms.EffectiveOn, PaymentOn: terms.PaymentOn,
				AmountValue: terms.AmountValue, AmountScale: terms.AmountScale, SourceEvidenceJSON: terms.SourceEvidenceJSON,
				Memo: transaction.Description, EntitledLotIDs: terms.EntitledLotIDs, LotEntitlements: terms.LotEntitlements,
			}
		}
		if correctable && record.OperationKind == "cash_in_lieu" {
			chain.CanCorrectCashInLieu = true
		}
		if correctable && record.OperationKind == "write_off" {
			chain.CanCorrectWriteOff = true
		}
		if correctable && (record.OperationKind == "internal_transfer" || record.OperationKind == "external_transfer_in" ||
			record.OperationKind == "external_transfer_out") {
			chain.CanReverseTransfer = true
			chain.CanReplaceTransfer = true
			if chain.CanReplaceTransfer {
				terms, err := s.transferCorrectionTerms(ctx, record.OperationID, record.TransactionID.Int64)
				if err != nil {
					return InvestmentCorrectionChain{}, err
				}
				chain.EffectiveTransfer = &terms
			}
		}
		if effective && record.OperationKind == "buy" && (!importedLineage || committedSource) &&
			record.TransactionStatus.String == "posted" && !record.TransactionDeleted {
			chain.CanReverseBuy = true
			chain.CanReverseManualBuy = !importedLineage
		}
	}
	return chain, nil
}

// dividendCorrectionTerms reads a posted cash dividend in the shape
// dividendPlan writes: cash +gross, income -gross, then optionally
// withholding +w and cash -w. Any other shape is not offered for correction.
func dividendCorrectionTerms(transaction Transaction) (*InvestmentCorrectionDividendTerms, bool) {
	if len(transaction.JournalEntries) != 1 {
		return nil, false
	}
	postings := transaction.JournalEntries[0].Postings
	if len(postings) != 2 && len(postings) != 4 {
		return nil, false
	}
	cash, income := postings[0], postings[1]
	if cash.CommodityID != income.CommodityID || cash.QuantityScale != income.QuantityScale ||
		cash.QuantityValue.Sign() <= 0 || income.QuantityValue != cash.QuantityValue.Negated() {
		return nil, false
	}
	terms := &InvestmentCorrectionDividendTerms{
		EventDate: transaction.TransactionDate, CashAccountID: cash.AccountID, CashCommodityID: cash.CommodityID,
		IncomeAccountID: income.AccountID, AmountValue: cash.QuantityValue, AmountScale: cash.QuantityScale,
		Memo: transaction.Description, PayeeID: transaction.PayeeID,
	}
	if len(postings) == 4 {
		withholding, paid := postings[2], postings[3]
		if paid.AccountID != cash.AccountID || withholding.CommodityID != cash.CommodityID ||
			paid.CommodityID != cash.CommodityID || withholding.QuantityScale != paid.QuantityScale ||
			withholding.QuantityValue.Sign() <= 0 || paid.QuantityValue != withholding.QuantityValue.Negated() {
			return nil, false
		}
		account, value, scale := withholding.AccountID, withholding.QuantityValue, withholding.QuantityScale
		terms.WithholdingAccountID, terms.WithholdingValue, terms.WithholdingScale = &account, &value, &scale
	}
	return terms, true
}

// reinvestmentCorrectionTerms reads a posted reinvestment in the shape
// reinvestedDividendPlan writes: holding +q, trading -q, trading +amount,
// income -amount.
func reinvestmentCorrectionTerms(transaction Transaction) (*InvestmentCorrectionReinvestmentTerms, bool) {
	if len(transaction.JournalEntries) != 1 || len(transaction.JournalEntries[0].Postings) != 4 {
		return nil, false
	}
	postings := transaction.JournalEntries[0].Postings
	holding, units, cost, income := postings[0], postings[1], postings[2], postings[3]
	if holding.QuantityValue.Sign() <= 0 || units.CommodityID != holding.CommodityID ||
		units.QuantityValue != holding.QuantityValue.Negated() || units.AccountID != cost.AccountID ||
		cost.CommodityID != income.CommodityID || cost.QuantityScale != income.QuantityScale ||
		cost.QuantityValue.Sign() <= 0 || income.QuantityValue != cost.QuantityValue.Negated() {
		return nil, false
	}
	return &InvestmentCorrectionReinvestmentTerms{
		EventDate: transaction.TransactionDate, HoldingAccountID: holding.AccountID, CommodityID: holding.CommodityID,
		CashCommodityID: cost.CommodityID, IncomeAccountID: income.AccountID,
		QuantityValue: holding.QuantityValue, QuantityScale: holding.QuantityScale,
		AmountValue: cost.QuantityValue, AmountScale: cost.QuantityScale,
		Memo: transaction.Description, PayeeID: transaction.PayeeID,
	}, true
}

func (s *InvestmentService) transferCorrectionTerms(ctx context.Context, operationID, transactionID int64) (InvestmentCorrectionTransferTerms, error) {
	terms, err := s.repository.TransferTerms(ctx, BookID, operationID)
	if err != nil {
		return InvestmentCorrectionTransferTerms{}, mapTransferCorrectionError(err)
	}
	transaction, err := s.transactionService.Transaction(ctx, transactionID)
	if err != nil {
		return InvestmentCorrectionTransferTerms{}, err
	}
	out := InvestmentCorrectionTransferTerms{
		TransferKind: terms.TransferKind, EffectiveOn: terms.EffectiveOn,
		CarriedBasisValue: terms.CarriedBasisValue, CarriedBasisScale: terms.CarriedBasisScale,
		OriginalAcquiredOn: terms.OriginalAcquiredOn, SourceAccountID: terms.SourceAccountID,
		DestinationAccountID: terms.DestinationAccountID, CommodityID: terms.CommodityID,
		CostCommodityID: terms.CostCommodityID, BasisAllocation: terms.BasisAllocation,
		DestinationLineage: terms.DestinationLineage, QuantityValue: terms.QuantityValue,
		QuantityScale: terms.QuantityScale, SourceEvidenceJSON: terms.SourceEvidenceJSON,
		Memo: transaction.Description,
	}
	for _, allocation := range terms.Allocations {
		out.Allocations = append(out.Allocations, InvestmentLotAllocationInput{LotID: allocation.LotID,
			QuantityValue: allocation.QuantityValue, QuantityScale: allocation.QuantityScale})
	}
	return out, nil
}
