package app

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/big"

	"rekenraam/backend/internal/db"
)

// ReconciliationImpactForCreate returns the reconciliation impact of creating a
// new transaction without actually persisting anything.
func (s *TransactionService) ReconciliationImpactForCreate(ctx context.Context, input CreateReconciliationImpactInput) (ReconciliationImpact, error) {
	if input.OwnerUserID <= 0 {
		return ReconciliationImpact{}, ValidationError{Message: "owner user is required"}
	}
	spec, err := s.cleanTransactionSpec(ctx, input.Spec, cleanTransactionOptions{DefaultStatus: "posted"})
	if err != nil {
		return ReconciliationImpact{}, err
	}
	refs, err := s.reconciliationInvalidationRefsFromSpec(ctx, spec)
	if err != nil {
		return ReconciliationImpact{}, err
	}
	if err := s.enrichCheckpointRefs(ctx, refs); err != nil {
		return ReconciliationImpact{}, err
	}
	return ReconciliationImpact{AffectedCheckpoints: refs}, nil
}

// ReconciliationImpactForUpdate returns the reconciliation impact of updating an
// existing transaction without actually persisting anything.
func (s *TransactionService) ReconciliationImpactForUpdate(ctx context.Context, input UpdateReconciliationImpactInput) (ReconciliationImpact, error) {
	if input.OwnerUserID <= 0 {
		return ReconciliationImpact{}, ValidationError{Message: "owner user is required"}
	}
	if input.TransactionID <= 0 {
		return ReconciliationImpact{}, ValidationError{Message: "transaction id is required"}
	}
	current, err := s.repository.TransactionByID(ctx, BookID, input.TransactionID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return ReconciliationImpact{}, ErrTransactionNotFound
		}
		return ReconciliationImpact{}, fmt.Errorf("read transaction: %w", err)
	}
	spec, err := s.cleanTransactionSpec(ctx, input.Spec, cleanTransactionOptions{
		DefaultStatus:    current.Status,
		ExistingLineKeys: lineKeySet(current),
		ExistingPostings: existingPostingStateSet(current),
	})
	if err != nil {
		return ReconciliationImpact{}, err
	}
	refs, err := s.reconciliationInvalidationRefs(ctx, current, spec)
	if err != nil {
		return ReconciliationImpact{}, err
	}
	if err := s.enrichCheckpointRefs(ctx, refs); err != nil {
		return ReconciliationImpact{}, err
	}
	return ReconciliationImpact{AffectedCheckpoints: refs}, nil
}

// maxPow10Scale caps the exponent accepted by pow10. No legitimate financial
// quantity or alignment delta should exceed this; a larger value indicates a
// logic bug in the caller, not a data problem.
const maxPow10Scale = 60

// pow10Cache holds precomputed 10^i for i in [0, maxPow10Scale].
var pow10Cache = func() [maxPow10Scale + 1]*big.Int {
	var t [maxPow10Scale + 1]*big.Int
	t[0] = big.NewInt(1)
	for i := 1; i <= maxPow10Scale; i++ {
		t[i] = new(big.Int).Mul(t[i-1], big.NewInt(10))
	}
	return t
}()

func pow10(scale int) *big.Int {
	if scale < 0 || scale > maxPow10Scale {
		panic(fmt.Sprintf("pow10: scale %d out of range [0, %d]", scale, maxPow10Scale))
	}
	return new(big.Int).Set(pow10Cache[scale])
}

// periodScopedRefsFromTransaction returns CheckpointInvalidationRefs for all
// postings in the transaction that fall within the period of an active
// reconciliation checkpoint (the period-scoped rule from docs/conventions.md).
// This is used for void, unvoid, soft-delete, and restore guards.
func (s *TransactionService) periodScopedRefsFromTransaction(ctx context.Context, transaction Transaction) ([]db.CheckpointInvalidationRef, error) {
	candidates := make([]db.PeriodScopedCheckpointRef, 0)
	for _, entry := range transaction.JournalEntries {
		for _, posting := range entry.Postings {
			candidates = append(candidates, db.PeriodScopedCheckpointRef{
				AccountID:          posting.AccountID,
				CommodityID:        posting.CommodityID,
				EntryDate:          entry.EntryDate,
				AccountDaySequence: posting.AccountDaySequence,
			})
		}
	}
	return s.repository.PeriodScopedCheckpointInvalidationRefs(ctx, BookID, candidates)
}

// periodScopedRefsFromRecord is the same as periodScopedRefsFromTransaction but
// takes a db.TransactionRecord (used in the update path before enrichment).
func (s *TransactionService) periodScopedRefsFromRecord(ctx context.Context, record db.TransactionRecord) ([]db.CheckpointInvalidationRef, error) {
	candidates := make([]db.PeriodScopedCheckpointRef, 0)
	for _, entry := range record.JournalEntries {
		for _, posting := range entry.Postings {
			candidates = append(candidates, db.PeriodScopedCheckpointRef{
				AccountID:          posting.AccountID,
				CommodityID:        posting.CommodityID,
				EntryDate:          entry.EntryDate,
				AccountDaySequence: posting.AccountDaySequence,
			})
		}
	}
	return s.repository.PeriodScopedCheckpointInvalidationRefs(ctx, BookID, candidates)
}

// reconciliationInvalidationRefs returns refs for the period-scoped update guard.
// Only postings that change a reconciliation-affecting field (account, commodity,
// quantity, or entry_date) are checked — pure memo/metadata edits are exempt.
// For each changed posting, both the current position (with its known sequence)
// and the proposed next position (with MaxInt64 sequence, reflecting that a new
// allocation will be above any existing statement_account_sequence) are added.
func (s *TransactionService) reconciliationInvalidationRefs(ctx context.Context, current db.TransactionRecord, spec db.TransactionSpec) ([]db.CheckpointInvalidationRef, error) {
	// Index current postings by line_key for O(1) lookup.
	type currentPosting struct {
		entryDate string
		posting   db.PostingRecord
	}
	currentByKey := map[string]currentPosting{}
	for _, entry := range current.JournalEntries {
		for _, posting := range entry.Postings {
			currentByKey[posting.LineKey] = currentPosting{entryDate: entry.EntryDate, posting: posting}
		}
	}

	// Index spec postings by line_key.
	type specPosting struct {
		entryDate string
		posting   db.PostingSpec
	}
	specByKey := map[string]specPosting{}
	for _, entry := range spec.JournalEntries {
		for _, posting := range entry.Postings {
			specByKey[posting.LineKey] = specPosting{entryDate: entry.EntryDate, posting: posting}
		}
	}

	candidates := make([]db.PeriodScopedCheckpointRef, 0)

	for key, cur := range currentByKey {
		next, exists := specByKey[key]
		if !exists || reconciliationAffectingChange(cur.entryDate, cur.posting, next.entryDate, next.posting) {
			candidates = append(candidates, db.PeriodScopedCheckpointRef{
				AccountID:          cur.posting.AccountID,
				CommodityID:        cur.posting.CommodityID,
				EntryDate:          cur.entryDate,
				AccountDaySequence: cur.posting.AccountDaySequence,
			})
		}
	}

	for key, next := range specByKey {
		cur, exists := currentByKey[key]
		if !exists || reconciliationAffectingChange(cur.entryDate, cur.posting, next.entryDate, next.posting) {
			candidates = append(candidates, db.PeriodScopedCheckpointRef{
				AccountID:          next.posting.AccountID,
				CommodityID:        next.posting.CommodityID,
				EntryDate:          next.entryDate,
				AccountDaySequence: math.MaxInt64,
			})
		}
	}

	if len(candidates) == 0 {
		return nil, nil
	}
	return s.repository.PeriodScopedCheckpointInvalidationRefs(ctx, BookID, candidates)
}

func reconciliationAffectingChange(currentDate string, current db.PostingRecord, nextDate string, next db.PostingSpec) bool {
	return currentDate != nextDate ||
		current.AccountID != next.AccountID ||
		current.CommodityID != next.CommodityID ||
		current.QuantityValue.Cmp(next.QuantityValue) != 0 ||
		current.QuantityScale != next.QuantityScale
}

// reconciliationInvalidationRefsFromSpec checks whether any posting in the spec
// would land inside an already-reconciled period. Used for the create-time guard.
// New postings always get sequence MAX+1, so math.MaxInt64 correctly represents
// "not inside" for the same-date case (see reconciliationInvalidationRefs).
func (s *TransactionService) reconciliationInvalidationRefsFromSpec(ctx context.Context, spec db.TransactionSpec) ([]db.CheckpointInvalidationRef, error) {
	candidates := make([]db.PeriodScopedCheckpointRef, 0)
	for _, entry := range spec.JournalEntries {
		for _, posting := range entry.Postings {
			candidates = append(candidates, db.PeriodScopedCheckpointRef{
				AccountID:          posting.AccountID,
				CommodityID:        posting.CommodityID,
				EntryDate:          entry.EntryDate,
				AccountDaySequence: math.MaxInt64,
			})
		}
	}
	return s.repository.PeriodScopedCheckpointInvalidationRefs(ctx, BookID, candidates)
}

// enrichCheckpointRefs populates AccountLabel and CommodityCode on each ref
// using one bulk account lookup and one bulk commodity lookup.
func (s *TransactionService) enrichCheckpointRefs(ctx context.Context, refs []db.CheckpointInvalidationRef) error {
	if len(refs) == 0 {
		return nil
	}

	accountIDSet := make(map[int64]struct{}, len(refs))
	commodityIDSet := make(map[int64]struct{}, len(refs))
	for _, ref := range refs {
		accountIDSet[ref.AccountID] = struct{}{}
		commodityIDSet[ref.CommodityID] = struct{}{}
	}

	accountIDs := make([]int64, 0, len(accountIDSet))
	for id := range accountIDSet {
		accountIDs = append(accountIDs, id)
	}
	commodityIDs := make([]int64, 0, len(commodityIDSet))
	for id := range commodityIDSet {
		commodityIDs = append(commodityIDs, id)
	}

	accountMap, err := s.accountRepository.AccountsByIDs(ctx, BookID, accountIDs)
	if err != nil {
		return fmt.Errorf("enrich checkpoint refs: accounts lookup: %w", err)
	}
	commodityMap, err := s.commodityRepository.CommoditiesByIDs(ctx, BookID, commodityIDs)
	if err != nil {
		return fmt.Errorf("enrich checkpoint refs: commodities lookup: %w", err)
	}

	for i := range refs {
		if acct, ok := accountMap[refs[i].AccountID]; ok {
			switch {
			case acct.SystemRole.Valid:
				refs[i].AccountLabel = acct.SystemRole.String
			case acct.BuiltinKey.Valid:
				refs[i].AccountLabel = acct.BuiltinKey.String
			case acct.Name.Valid:
				refs[i].AccountLabel = acct.Name.String
			case acct.Code.Valid:
				refs[i].AccountLabel = acct.Code.String
			default:
				refs[i].AccountLabel = fmt.Sprintf("account:%d", refs[i].AccountID)
			}
		}
		if comm, ok := commodityMap[refs[i].CommodityID]; ok {
			refs[i].CommodityCode = comm.Code
		}
	}

	return nil
}

func mapTransactionDBError(err error) error {
	switch {
	case errors.Is(err, db.ErrNotFound):
		return ErrTransactionNotFound
	case errors.Is(err, db.ErrTransactionHasPostedVersions):
		return ErrTransactionPosted
	case errors.Is(err, db.ErrTransactionVoided):
		return ErrTransactionVoided
	case errors.Is(err, db.ErrTransactionDeleted):
		return ErrTransactionDeleted
	case errors.Is(err, db.ErrTransactionReconciled):
		return ErrTransactionProtected
	case errors.Is(err, db.ErrArchivedTag):
		return ErrTransactionTag
	case errors.Is(err, db.ErrReconciliationNotFound):
		return ErrReconciliationNotFound
	case errors.Is(err, db.ErrReconciliationClosed):
		return ErrReconciliationClosed
	case errors.Is(err, db.ErrReconciliationNotBalanced):
		return ErrReconciliationNotBalanced
	case errors.Is(err, db.ErrReconciliationPosting), errors.Is(err, db.ErrReconciliationCheckpoint):
		return ErrReconciliationPosting
	case errors.Is(err, db.ErrReconciliationOverrideRequired):
		return ErrReconciliationOverrideRequired
	default:
		return fmt.Errorf("transaction repository: %w", err)
	}
}
