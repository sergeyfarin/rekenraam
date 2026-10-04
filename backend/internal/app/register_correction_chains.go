package app

import (
	"context"
	"fmt"

	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/exact"
)

// RegisterCorrectionChain groups a register row with the other transactions of
// its correction chain (T-120 #135). The rows themselves stay separate posted
// records, so the running balance counts each posting once; the chain carries
// the explanation and the chain's net effect on this row's account and
// commodity, counted once. Every member is named even when it falls on another
// register page.
type RegisterCorrectionChain struct {
	RootTransactionID int64
	// Role is this row's transaction's role: original, reversal or replacement.
	Role string
	// EffectiveTransactionID is the chain's current effective transaction, or
	// nil when the chain ends in a pure reversal.
	EffectiveTransactionID *int64
	// NetEffect sums every posted, undeleted member's postings in this account
	// and commodity.
	NetEffect RegisterCorrectionAmount
	Members   []RegisterCorrectionMember
}

type RegisterCorrectionMember struct {
	TransactionID             int64
	CorrectionOfTransactionID *int64
	Role                      string
	TransactionDate           string
	Status                    string
	Deleted                   bool
	Reason                    string
	// Amount is the member's net in this account and commodity; nil when it
	// posts nothing here (a replacement moved to another account).
	Amount *RegisterCorrectionAmount
}

type RegisterCorrectionAmount struct {
	QuantityValue exact.Coefficient
	QuantityScale int
}

// attachRegisterCorrectionChains annotates every row whose transaction belongs
// to a chain of more than one transaction, reading all chains on the page in
// one repository call.
func (s *TransactionService) attachRegisterCorrectionChains(ctx context.Context, accountID int64, entries []AccountRegisterEntry) error {
	ids := make([]int64, 0, len(entries))
	seen := map[int64]bool{}
	for _, entry := range entries {
		if !seen[entry.TransactionID] {
			seen[entry.TransactionID] = true
			ids = append(ids, entry.TransactionID)
		}
	}
	records, err := s.repository.RegisterCorrectionChains(ctx, BookID, accountID, ids)
	if err != nil {
		return fmt.Errorf("read register correction chains: %w", err)
	}
	chains := map[int64][]db.RegisterCorrectionMemberRecord{}
	rootOf := map[int64]int64{}
	for _, record := range records {
		chains[record.RootTransactionID] = append(chains[record.RootTransactionID], record)
		rootOf[record.TransactionID] = record.RootTransactionID
	}
	for index := range entries {
		root, ok := rootOf[entries[index].TransactionID]
		if !ok || len(chains[root]) < 2 {
			continue
		}
		chain, err := registerCorrectionChain(chains[root], entries[index].TransactionID, entries[index].Posting.CommodityID)
		if err != nil {
			return err
		}
		entries[index].CorrectionChain = &chain
	}
	return nil
}

func registerCorrectionChain(records []db.RegisterCorrectionMemberRecord, transactionID, commodityID int64) (RegisterCorrectionChain, error) {
	chain := RegisterCorrectionChain{RootTransactionID: records[0].RootTransactionID}
	corrected := map[int64]bool{}
	for _, record := range records {
		if record.CorrectionOfTransactionID.Valid {
			corrected[record.CorrectionOfTransactionID.Int64] = true
		}
	}
	net := exact.NewScaledInt()
	for _, record := range records {
		member := RegisterCorrectionMember{
			TransactionID: record.TransactionID, Role: record.Role,
			TransactionDate: record.TransactionDate, Status: record.Status,
			Deleted: record.Deleted, Reason: record.Reason.String,
		}
		if record.CorrectionOfTransactionID.Valid {
			id := record.CorrectionOfTransactionID.Int64
			member.CorrectionOfTransactionID = &id
		}
		amount, touches := exact.NewScaledInt(), false
		for _, posting := range record.Postings {
			if posting.CommodityID == commodityID {
				amount.AddCoefficient(posting.QuantityValue, posting.QuantityScale)
				touches = true
			}
		}
		if touches {
			value, err := amount.Coefficient()
			if err != nil {
				return RegisterCorrectionChain{}, fmt.Errorf("correction member amount: %w", err)
			}
			member.Amount = &RegisterCorrectionAmount{QuantityValue: value, QuantityScale: amount.Scale()}
			if record.Status == "posted" && !record.Deleted {
				net.AddScaled(amount)
			}
		}
		if record.TransactionID == transactionID {
			chain.Role = record.Role
		}
		if record.Role != "reversal" && !corrected[record.TransactionID] {
			id := record.TransactionID
			chain.EffectiveTransactionID = &id
		}
		chain.Members = append(chain.Members, member)
	}
	value, err := net.Coefficient()
	if err != nil {
		return RegisterCorrectionChain{}, fmt.Errorf("correction chain net effect: %w", err)
	}
	chain.NetEffect = RegisterCorrectionAmount{QuantityValue: value, QuantityScale: net.Scale()}
	return chain, nil
}
