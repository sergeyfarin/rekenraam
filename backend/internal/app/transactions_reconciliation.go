package app

import (
	"context"
	"errors"
	"fmt"
	"math"

	"rekenraam/backend/internal/db"
)

// ReconciliationImpactForCreate returns the reconciliation impact of creating a
// new transaction without actually persisting anything.
func (s *TransactionService) ReconciliationImpactForCreate(ctx context.Context, input CreateReconciliationImpactInput) (ReconciliationImpact, error) {
	return s.reconciliationImpactForCreate(ctx, input, cleanTransactionOptions{})
}

// investmentReconciliationImpactForCreate is the preview counterpart of
// prepareInvestmentTransactionForWrite. The preview validates the same spec the
// write will, so it needs the same T-96 exemption — otherwise every investment
// impact preview is refused for a posting the commit would have accepted, and
// preview and commit stop agreeing.
func (s *TransactionService) investmentReconciliationImpactForCreate(ctx context.Context, input CreateReconciliationImpactInput) (ReconciliationImpact, error) {
	return s.reconciliationImpactForCreate(ctx, input, cleanTransactionOptions{AllowSubledgerManagedPostings: true})
}

func (s *TransactionService) reconciliationImpactForCreate(ctx context.Context, input CreateReconciliationImpactInput, options cleanTransactionOptions) (ReconciliationImpact, error) {
	if input.OwnerUserID <= 0 {
		return ReconciliationImpact{}, ValidationError{Message: "owner user is required"}
	}
	options.DefaultStatus = "posted"
	spec, err := s.cleanTransactionSpec(ctx, input.Spec, options)
	if err != nil {
		return ReconciliationImpact{}, err
	}
	refs, err := s.resolveCheckpointRefs(ctx, reconciliationCandidatesFromSpec(spec))
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
	// This preview endpoint has no draft-promotion input, so the transaction's
	// current status is always the final status — pin it the same way
	// UpdateTransaction does, so the balance-validation check here matches
	// what the real update will enforce.
	spec, err := s.cleanTransactionSpec(ctx, input.Spec, cleanTransactionOptions{
		ForcedStatus:     current.Status,
		ExistingLineKeys: lineKeySet(current),
		ExistingPostings: existingPostingStateSet(current),
	})
	if err != nil {
		return ReconciliationImpact{}, err
	}
	refs, err := s.resolveCheckpointRefs(ctx, reconciliationCandidates(current, spec))
	if err != nil {
		return ReconciliationImpact{}, err
	}
	if err := s.enrichCheckpointRefs(ctx, refs); err != nil {
		return ReconciliationImpact{}, err
	}
	return ReconciliationImpact{AffectedCheckpoints: refs}, nil
}

// ReconciliationImpactForPost validates the saved draft as posted and uses its
// existing posting positions, exactly as promotion does. This is advisory;
// posting always rechecks the guard and does not accept a preview as authority.
func (s *TransactionService) ReconciliationImpactForPost(ctx context.Context, ownerID, transactionID int64) (ReconciliationImpact, error) {
	if ownerID <= 0 || transactionID <= 0 {
		return ReconciliationImpact{}, ValidationError{Message: "owner and transaction ids are required"}
	}
	current, err := s.repository.TransactionByID(ctx, BookID, transactionID)
	if err != nil {
		return ReconciliationImpact{}, mapTransactionDBError(err)
	}
	if current.DeletedAt.Valid {
		return ReconciliationImpact{}, ErrTransactionDeleted
	}
	if current.Status == "voided" {
		return ReconciliationImpact{}, ErrTransactionVoided
	}
	if current.Status == "posted" {
		return ReconciliationImpact{}, nil
	}
	if err := s.rejectInvestmentLinkedMutation(ctx, transactionID); err != nil {
		return ReconciliationImpact{}, err
	}
	_, err = s.cleanTransactionSpec(ctx, transactionInputFromTransaction(toTransaction(current)), cleanTransactionOptions{
		ForcedStatus: "posted", ExistingLineKeys: lineKeySet(current), ExistingPostings: existingPostingStateSet(current),
	})
	if err != nil {
		return ReconciliationImpact{}, err
	}
	refs, err := s.resolveCheckpointRefs(ctx, periodScopedCandidatesFromRecord(current))
	if err != nil {
		return ReconciliationImpact{}, err
	}
	if err := s.enrichCheckpointRefs(ctx, refs); err != nil {
		return ReconciliationImpact{}, err
	}
	return ReconciliationImpact{AffectedCheckpoints: refs}, nil
}

// periodScopedCandidatesFromTransaction returns checkpoint candidates for all
// postings in the transaction that fall within the period of an active
// reconciliation checkpoint (the period-scoped rule from docs/conventions.md).
// This is used for void, unvoid, soft-delete, and restore guards.
func periodScopedCandidatesFromTransaction(transaction Transaction) []db.PeriodScopedCheckpointRef {
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
	return candidates
}

// periodScopedCandidatesFromRecord is the same as periodScopedCandidatesFromTransaction but
// takes a db.TransactionRecord (used in the update path before enrichment).
func periodScopedCandidatesFromRecord(record db.TransactionRecord) []db.PeriodScopedCheckpointRef {
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
	return candidates
}

// reconciliationCandidates returns candidates for the period-scoped update guard.
// Only postings that change a reconciliation-affecting field (account, commodity,
// quantity, or entry_date) are checked — pure memo/metadata edits are exempt.
// For each changed posting, both the current position (with its known sequence)
// and the proposed next position (with MaxInt64 sequence, reflecting that a new
// allocation will be above any existing statement_account_sequence) are added.
func reconciliationCandidates(current db.TransactionRecord, spec db.TransactionSpec) []db.PeriodScopedCheckpointRef {
	if current.Status == "draft" && spec.Status == "draft" {
		return nil
	}
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

	return candidates
}

func reconciliationAffectingChange(currentDate string, current db.PostingRecord, nextDate string, next db.PostingSpec) bool {
	return currentDate != nextDate ||
		current.AccountID != next.AccountID ||
		current.CommodityID != next.CommodityID ||
		current.QuantityValue.Cmp(next.QuantityValue) != 0 ||
		current.QuantityScale != next.QuantityScale
}

// reconciliationCandidatesFromSpec names every position the spec would occupy,
// would land inside an already-reconciled period. Used for the create-time guard.
// New postings always get sequence MAX+1, so math.MaxInt64 correctly represents
// "not inside" for the same-date case (see reconciliationCandidates).
func reconciliationCandidatesFromSpec(spec db.TransactionSpec) []db.PeriodScopedCheckpointRef {
	// Producer drafts are outside the ledger even when their dates fall in a
	// reconciled period. Promotion has its own guard over the stored positions.
	if spec.Status == "draft" {
		return nil
	}
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
	return candidates
}

// resolveCheckpointRefs answers, right now, which active checkpoints the given
// candidates fall inside. Only previews and the service's early rejection use
// it: the answer is advisory, because it is read outside the write transaction
// and anything can change before the write commits. The write's own guard lives
// in enforceCheckpointBoundaryTx, which resolves the same candidates against the
// transaction it is committing in (T-94).
func (s *TransactionService) resolveCheckpointRefs(ctx context.Context, candidates []db.PeriodScopedCheckpointRef) ([]db.CheckpointInvalidationRef, error) {
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
	case errors.Is(err, db.ErrInvestmentBasisRange):
		return ValidationError{Message: err.Error()}
	case errors.Is(err, db.ErrNotFound):
		return ErrTransactionNotFound
	case errors.Is(err, db.ErrTransactionHasPostedVersions):
		return ErrTransactionPosted
	case errors.Is(err, db.ErrTransactionVoided):
		return ErrTransactionVoided
	case errors.Is(err, db.ErrTransactionVersionStale):
		return ErrTransactionVersionStale
	case errors.Is(err, db.ErrPostingAccountVersionStale):
		return ErrPostingAccountVersionStale
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
