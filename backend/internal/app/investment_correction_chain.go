package app

import (
	"context"
	"errors"

	"rekenraam/backend/internal/db"
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
	Operations             []InvestmentCorrectionNode
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
		if effective && record.OperationKind == "buy" && (!importedLineage || committedSource) &&
			record.TransactionStatus.String == "posted" && !record.TransactionDeleted {
			chain.CanReverseBuy = true
			chain.CanReverseManualBuy = !importedLineage
		}
	}
	return chain, nil
}
