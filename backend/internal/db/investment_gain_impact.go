package db

import (
	"cmp"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"
	"strings"

	"rekenraam/backend/internal/exact"
)

// Replay gain disclosure (T-114). A backdated or corrective command can
// revise, replace or remove a committed disposal's operational basis and
// gain without changing any reconciled balance, so the reconciliation guard
// cannot catch it. A command that opts in snapshots every effective disposal
// in the book when its write transaction begins, compares the effective set
// once its domain effects have run, and refuses to commit a non-empty change
// set unless the caller acknowledged exactly that set.

var (
	// ErrGainImpactAcknowledgementRequired means the command changes committed
	// disposals and the caller supplied no acknowledgement.
	ErrGainImpactAcknowledgementRequired = errors.New("committed disposal gains change; acknowledgement is required")
	// ErrGainImpactAcknowledgementStale means the acknowledgement names a
	// different change set from the one this commit would produce.
	ErrGainImpactAcknowledgementStale = errors.New("gain impact acknowledgement does not match the current change set")
)

// Gain-impact change kinds. A disposal the command itself creates under a new
// correction root is the command's own subject, not a revision of history.
const (
	GainImpactRevised  = "revised"  // same decision, new effective allocation result
	GainImpactReplaced = "replaced" // a correction superseded the decision
	GainImpactRemoved  = "removed"  // a reversal left no effective decision
)

// GainImpactPolicy opts a command into gain disclosure. A nil policy on the
// command's first journal means the path has not been migrated yet (T-126).
type GainImpactPolicy struct {
	// Acknowledgement is the token a preview returned for the change set the
	// user accepted. Empty acknowledges nothing.
	Acknowledgement string
}

// InvestmentGainIdentity names a disposal by its correction root and decision
// sequence, which survive replacement. Preview IDs never form an identity.
type InvestmentGainIdentity struct {
	RootOperationID int64
	DecisionSeq     int
}

// InvestmentGainState is one effective disposal's operational result.
// Basis and gain are nil exactly when basis knowledge is unknown.
type InvestmentGainState struct {
	AccountID       int64
	CommodityID     int64
	CostCommodityID int64
	DisposalDate    string
	CostBasisMethod string
	Quantity        *exact.ScaledInt
	BasisKnowledge  string
	DisposedBasis   *exact.ScaledInt
	Proceeds        *exact.ScaledInt
	Gain            *exact.ScaledInt
}

// InvestmentGainChange discloses one committed disposal whose operational
// result changes. The durable IDs name the committed decision; After carries
// values only, because a replacement decision exists only inside the write.
type InvestmentGainChange struct {
	Identity      InvestmentGainIdentity
	Kind          string
	OperationID   int64
	DecisionID    int64
	TransactionID int64
	Before        InvestmentGainState
	After         *InvestmentGainState
}

// InvestmentGainImpact is the complete change set and the token that binds an
// acknowledgement to it. Both are empty when no committed disposal changes.
type InvestmentGainImpact struct {
	Changes         []InvestmentGainChange
	Acknowledgement string
}

type investmentGainSnapshotEntry struct {
	operationID   int64
	decisionID    int64
	transactionID int64
	state         InvestmentGainState
}

// investmentGainSnapshotTx reads every effective disposal in the book with its
// current effective basis: the latest replay revision when one exists, else
// the immutable decision snapshot. Reversed and superseded operations are not
// effective, which is how removals and replacements become visible.
func investmentGainSnapshotTx(ctx context.Context, tx *sql.Tx, bookID int64) (map[InvestmentGainIdentity]investmentGainSnapshotEntry, error) {
	roots, err := investmentReplayOrderOperationIDsQuery(ctx, tx, bookID)
	if err != nil {
		return nil, err
	}
	rows, err := tx.QueryContext(ctx, `
		SELECT d.id, d.operation_id, d.decision_seq, d.transaction_id, d.account_id,
			d.commodity_id, d.cost_commodity_id, d.event_date, d.cost_basis_method,
			d.quantity_value, d.quantity_scale, d.disposed_basis_value, d.disposed_basis_scale,
			d.basis_knowledge, d.proceeds_value, d.proceeds_scale, r.id,
			r.disposed_basis_value, r.disposed_basis_scale, r.basis_knowledge, d.position_side
		FROM investment_disposal_decisions d
		JOIN effective_investment_operations o ON o.id = d.operation_id
		LEFT JOIN latest_investment_disposal_revisions r ON r.decision_id = d.id
		WHERE d.book_id = ?`, bookID)
	if err != nil {
		return nil, fmt.Errorf("read effective disposals for gain impact: %w", err)
	}
	defer rows.Close()
	snapshot := make(map[InvestmentGainIdentity]investmentGainSnapshotEntry)
	for rows.Next() {
		var entry investmentGainSnapshotEntry
		var seq, quantityScale, proceedsScale int
		var quantity, proceeds exact.Coefficient
		var basis, revisedBasis sql.NullString
		var basisScale, revisionID, revisedScale sql.NullInt64
		var knowledge string
		var revisedKnowledge sql.NullString
		var side string
		if err := rows.Scan(&entry.decisionID, &entry.operationID, &seq, &entry.transactionID,
			&entry.state.AccountID, &entry.state.CommodityID, &entry.state.CostCommodityID,
			&entry.state.DisposalDate, &entry.state.CostBasisMethod, &quantity, &quantityScale,
			&basis, &basisScale, &knowledge, &proceeds, &proceedsScale, &revisionID,
			&revisedBasis, &revisedScale, &revisedKnowledge, &side); err != nil {
			return nil, fmt.Errorf("scan effective disposal for gain impact: %w", err)
		}
		// The latest revision's knowledge and amounts replace the snapshot's
		// as one tuple; a NULL revised amount is never filled from the original.
		if revisionID.Valid {
			basis, basisScale, knowledge = revisedBasis, revisedScale, revisedKnowledge.String
		}
		root, ok := roots[entry.operationID]
		if !ok {
			return nil, fmt.Errorf("disposal operation %d has no correction root", entry.operationID)
		}
		entry.state.Quantity = exact.ScaledIntFromCoefficient(quantity, quantityScale)
		entry.state.Proceeds = exact.ScaledIntFromCoefficient(proceeds, proceedsScale)
		// Unknown basis has no disposed basis or gain, so an unknown-to-known
		// transition is a disclosed change, never a comparison against zero.
		switch {
		case knowledge == InvestmentBasisUnknown && !basis.Valid && !basisScale.Valid:
			entry.state.BasisKnowledge = InvestmentBasisUnknown
		case knowledge == InvestmentBasisKnown && basis.Valid && basisScale.Valid:
			entry.state.BasisKnowledge = InvestmentBasisKnown
			entry.state.DisposedBasis = exact.ScaledIntFromCoefficient(exact.Coefficient(basis.String), int(basisScale.Int64))
			entry.state.Gain = investmentGainValue(entry.state.Proceeds, entry.state.DisposedBasis)
			if side == PositionSideShort {
				// A cover's disposed amount is opening proceeds received.
				entry.state.Gain = exact.ScaledIntFromBig(entry.state.Proceeds.BigInt(), entry.state.Proceeds.Scale())
				entry.state.Gain.AddScaled(entry.state.DisposedBasis)
			}
		default:
			return nil, fmt.Errorf("disposal decision %d has an invalid effective basis knowledge/amount pair", entry.decisionID)
		}
		identity := InvestmentGainIdentity{RootOperationID: root, DecisionSeq: seq}
		if _, duplicate := snapshot[identity]; duplicate {
			return nil, fmt.Errorf("correction root %d has two effective decisions with sequence %d", root, seq)
		}
		snapshot[identity] = entry
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate effective disposals for gain impact: %w", err)
	}
	return snapshot, nil
}

func investmentGainValue(proceeds, basis *exact.ScaledInt) *exact.ScaledInt {
	gain := exact.ScaledIntFromBig(proceeds.BigInt(), proceeds.Scale())
	gain.SubScaled(basis)
	return gain
}

// compareInvestmentGainSnapshots returns the committed disposals whose
// operational result differs, ordered by identity. Values compare exactly and
// scale-insensitively. Re-selected allocations with identical totals are not a
// gain change: the revision is still recorded in the audit chain.
func compareInvestmentGainSnapshots(before, after map[InvestmentGainIdentity]investmentGainSnapshotEntry) InvestmentGainImpact {
	identities := make([]InvestmentGainIdentity, 0, len(before))
	for identity := range before {
		identities = append(identities, identity)
	}
	slices.SortFunc(identities, func(a, b InvestmentGainIdentity) int {
		if order := cmp.Compare(a.RootOperationID, b.RootOperationID); order != 0 {
			return order
		}
		return cmp.Compare(a.DecisionSeq, b.DecisionSeq)
	})
	var impact InvestmentGainImpact
	for _, identity := range identities {
		old := before[identity]
		change := InvestmentGainChange{Identity: identity, OperationID: old.operationID,
			DecisionID: old.decisionID, TransactionID: old.transactionID, Before: old.state}
		current, exists := after[identity]
		switch {
		case !exists:
			change.Kind = GainImpactRemoved
		case equalInvestmentGainStates(old.state, current.state):
			continue
		case current.decisionID == old.decisionID:
			change.Kind = GainImpactRevised
		default:
			change.Kind = GainImpactReplaced
		}
		if exists {
			state := current.state
			change.After = &state
		}
		impact.Changes = append(impact.Changes, change)
	}
	if len(impact.Changes) > 0 {
		impact.Acknowledgement = investmentGainAcknowledgement(impact.Changes)
	}
	return impact
}

func equalInvestmentGainStates(a, b InvestmentGainState) bool {
	return a.AccountID == b.AccountID && a.CommodityID == b.CommodityID &&
		a.CostCommodityID == b.CostCommodityID && a.DisposalDate == b.DisposalDate &&
		a.CostBasisMethod == b.CostBasisMethod && a.BasisKnowledge == b.BasisKnowledge &&
		equalOptionalScaled(a.Quantity, b.Quantity) && equalOptionalScaled(a.DisposedBasis, b.DisposedBasis) &&
		equalOptionalScaled(a.Proceeds, b.Proceeds)
}

func equalOptionalScaled(a, b *exact.ScaledInt) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.Cmp(b) == 0
}

// investmentGainAcknowledgement digests the canonical, scale-normalized change
// set. Any difference in identity, kind, committed IDs or values yields a new
// token, so an acknowledgement cannot be replayed against another change set.
func investmentGainAcknowledgement(changes []InvestmentGainChange) string {
	var canonical strings.Builder
	canonical.WriteString("gain-impact/v1\n")
	for _, change := range changes {
		fmt.Fprintf(&canonical, "%d|%d|%s|%d|%d|%d|%s|%s\n", change.Identity.RootOperationID,
			change.Identity.DecisionSeq, change.Kind, change.OperationID, change.DecisionID,
			change.TransactionID, canonicalInvestmentGainState(&change.Before), canonicalInvestmentGainState(change.After))
	}
	digest := sha256.Sum256([]byte(canonical.String()))
	return hex.EncodeToString(digest[:])
}

func canonicalInvestmentGainState(state *InvestmentGainState) string {
	if state == nil {
		return "none"
	}
	return fmt.Sprintf("%d,%d,%d,%s,%s,%s,%s,%s,%s", state.AccountID, state.CommodityID,
		state.CostCommodityID, state.DisposalDate, state.CostBasisMethod, state.BasisKnowledge,
		canonicalScaled(state.Quantity), canonicalScaled(state.DisposedBasis), canonicalScaled(state.Proceeds))
}

func canonicalScaled(value *exact.ScaledInt) string {
	if value == nil {
		return "unknown"
	}
	return value.Normalized().String()
}

// requireInvestmentGainAcknowledgement accepts an empty change set without an
// acknowledgement, and otherwise only the exact token for this change set.
func requireInvestmentGainAcknowledgement(policy GainImpactPolicy, impact InvestmentGainImpact) error {
	if len(impact.Changes) == 0 {
		return nil
	}
	if strings.TrimSpace(policy.Acknowledgement) == "" {
		return ErrGainImpactAcknowledgementRequired
	}
	if policy.Acknowledgement != impact.Acknowledgement {
		return ErrGainImpactAcknowledgementStale
	}
	return nil
}
