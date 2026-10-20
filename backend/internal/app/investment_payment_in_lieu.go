package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/exact"
)

// A payment in lieu of a dividend is what a short holder owes the lender
// when the borrowed instrument goes ex-dividend (#255, ADR 0013 §3 and its
// payment-in-lieu refinement). It is a named cash charge, never income and
// never folded into the short lot: cash −gross against an owner-chosen
// expense account (+gross), plus an optional recovery credited back (cash
// +r against a recovery account −r). The app does not capitalize it into
// the short's opening proceeds, because whether and when that applies is a
// jurisdiction's rule. The short holding, instrument and ex-date are
// immutable facts; the holding must be short at the close before the
// ex-date, re-checked inside the write transaction.

var (
	ErrInvestmentPaymentInLieuNotFound         = errors.New("payment in lieu operation not found")
	ErrInvestmentPaymentInLieuAlreadyCorrected = errors.New("payment in lieu already corrected")
	ErrInvestmentPaymentInLieuChanged          = errors.New("payment in lieu changed")
	// ErrInvestmentPaymentInLieuNotShort is the repository's refusal under
	// this package's name.
	ErrInvestmentPaymentInLieuNotShort = db.ErrPaymentInLieuNotShort
	// ErrInvestmentPaymentInLieuDependency refuses any investment command
	// that would leave an effective payment in lieu without its short (#257);
	// InvestmentPaymentInLieuDependencyError names the payment.
	ErrInvestmentPaymentInLieuDependency = db.ErrPaymentInLieuDependency
)

type InvestmentPaymentInLieuDependencyError = db.PaymentInLieuDependencyError

var (
	paymentInLieuExpenseRole = accountRoleForInvestment{
		label:          "payment in lieu expense account",
		allowedClasses: []string{"expense"},
	}
	// A recovery is money credited back against the charge, such as tax the
	// lender would have suffered: it may net in the same expense account,
	// or be booked as income or a receivable.
	paymentInLieuRecoveryRole = accountRoleForInvestment{
		label:          "payment in lieu recovery account",
		allowedClasses: []string{"asset", "liability", "expense", "income"},
	}
)

type PaymentInLieuInput struct {
	OwnerUserID       int64
	AuthSessionID     int64
	RequestID         string
	HoldingAccountID  int64
	CommodityID       int64
	ExDate            string
	TransactionDate   string
	CashAccountID     int64
	CashCommodityID   int64
	ExpenseAccountID  int64
	AmountValue       int64
	AmountScale       int
	RecoveryValue     *int64
	RecoveryScale     *int
	RecoveryAccountID *int64
	Memo              string
	PayeeID           *int64
	ExternalRefHint   string
	ChangeReason      string
	// ReconciliationOverride allows a backdated payment to invalidate the
	// cash, expense or recovery checkpoints it lands behind.
	ReconciliationOverride CheckpointOverride
	OriginType             string
	Operation              string
}

func (s *InvestmentService) PaymentInLieu(ctx context.Context, input PaymentInLieuInput) (Transaction, error) {
	return s.paymentInLieu(ctx, input, nil)
}

// paymentInLieuWithPostWrite keeps an accepted provider suggestion's status
// in the same transaction as the payment.
func (s *InvestmentService) paymentInLieuWithPostWrite(ctx context.Context, input PaymentInLieuInput, postWrite func(*sql.Tx, int64) error) (Transaction, error) {
	return s.paymentInLieu(ctx, input, postWrite)
}

func (s *InvestmentService) paymentInLieu(ctx context.Context, input PaymentInLieuInput, postWrite func(*sql.Tx, int64) error) (Transaction, error) {
	params, err := s.preparePaymentInLieuWrite(ctx, input)
	if err != nil {
		return Transaction{}, err
	}
	record, err := s.repository.CreatePaymentInLieu(ctx, params, postWrite)
	if err != nil {
		return Transaction{}, mapPaymentInLieuError(err)
	}
	return s.transactionService.enrichOne(ctx, toTransaction(record))
}

// PaymentInLieuReconciliationImpact runs the complete writer, entitlement
// check included, and rolls it back.
func (s *InvestmentService) PaymentInLieuReconciliationImpact(ctx context.Context, input PaymentInLieuInput) (ReconciliationImpact, error) {
	input.ReconciliationOverride = unboundOverride
	params, err := s.preparePaymentInLieuWrite(ctx, input)
	if err != nil {
		return ReconciliationImpact{}, err
	}
	simulated, err := s.repository.PreviewPaymentInLieu(ctx, params)
	if err != nil {
		return ReconciliationImpact{}, mapPaymentInLieuError(err)
	}
	return s.simulatedReconciliationImpact(ctx, simulated)
}

func validatePaymentInLieuInput(input PaymentInLieuInput) (exDate, date string, err error) {
	switch {
	case input.OwnerUserID <= 0:
		return "", "", ValidationError{Message: "owner user is required"}
	case input.HoldingAccountID <= 0:
		return "", "", ValidationError{Message: "short holding account is required"}
	case input.CommodityID <= 0:
		return "", "", ValidationError{Message: "instrument is required"}
	case input.CashAccountID <= 0:
		return "", "", ValidationError{Message: "cash account is required"}
	case input.CashCommodityID <= 0:
		return "", "", ValidationError{Message: "cash commodity is required"}
	case input.ExpenseAccountID <= 0:
		return "", "", ValidationError{Message: "payment in lieu expense account is required"}
	case input.AmountValue <= 0 || input.AmountScale < 0:
		return "", "", ValidationError{Message: "payment in lieu amount owed is required"}
	}
	if date, err = cleanRequiredDate(input.TransactionDate, "payment date"); err != nil {
		return "", "", err
	}
	if exDate, err = cleanRequiredDate(input.ExDate, "ex-date"); err != nil {
		return "", "", err
	}
	if exDate > date {
		return "", "", ValidationError{Message: "payment date must be on or after the ex-date"}
	}
	if input.RecoveryValue == nil {
		if input.RecoveryScale != nil || input.RecoveryAccountID != nil {
			return "", "", ValidationError{Message: "recovery account and scale need a recovery amount"}
		}
		return exDate, date, nil
	}
	scale := input.AmountScale
	if input.RecoveryScale != nil {
		scale = *input.RecoveryScale
	}
	if *input.RecoveryValue <= 0 || scale < 0 {
		return "", "", ValidationError{Message: "recovery amount must be positive"}
	}
	if input.RecoveryAccountID == nil || *input.RecoveryAccountID <= 0 {
		return "", "", ValidationError{Message: "payment in lieu recovery account is required"}
	}
	if exact.ScaledIntFromInt64(*input.RecoveryValue, scale).Cmp(exact.ScaledIntFromInt64(input.AmountValue, input.AmountScale)) >= 0 {
		return "", "", ValidationError{Message: "recovery must be less than the amount owed"}
	}
	return exDate, date, nil
}

func (s *InvestmentService) paymentInLieuPlan(ctx context.Context, input PaymentInLieuInput) (investmentTransactionPlan, string, error) {
	exDate, date, err := validatePaymentInLieuInput(input)
	if err != nil {
		return investmentTransactionPlan{}, "", err
	}
	dependencies := newAccountRuleDependencies()
	if _, err := s.accountInRole(ctx, input.HoldingAccountID, exDate, holdingRole, dependencies); err != nil {
		return investmentTransactionPlan{}, "", err
	}
	if _, err := s.accountInRole(ctx, input.CashAccountID, date, settlementRole, dependencies); err != nil {
		return investmentTransactionPlan{}, "", err
	}
	if _, err := s.accountInRole(ctx, input.ExpenseAccountID, date, paymentInLieuExpenseRole, dependencies); err != nil {
		return investmentTransactionPlan{}, "", err
	}
	if input.RecoveryValue != nil {
		if _, err := s.accountInRole(ctx, *input.RecoveryAccountID, date, paymentInLieuRecoveryRole, dependencies); err != nil {
			return investmentTransactionPlan{}, "", err
		}
	}
	if err := s.requireCommodityKind(ctx, input.CommodityID, exDate, "traded commodity", false); err != nil {
		return investmentTransactionPlan{}, "", err
	}
	if err := s.requireCommodityKind(ctx, input.CashCommodityID, date, "settlement commodity", true); err != nil {
		return investmentTransactionPlan{}, "", err
	}
	memo, err := cleanOptionalText(input.Memo, "memo", investmentTextMaxBytes)
	if err != nil {
		return investmentTransactionPlan{}, "", err
	}
	// The cash leg comes first: paymentInLieuCorrectionTerms reads it back.
	postings := []PostingInput{
		{AccountID: input.CashAccountID, QuantityValue: exact.New(-input.AmountValue), QuantityScale: input.AmountScale, CommodityID: input.CashCommodityID, Memo: memo},
		{AccountID: input.ExpenseAccountID, QuantityValue: exact.New(input.AmountValue), QuantityScale: input.AmountScale, CommodityID: input.CashCommodityID, Memo: memo},
	}
	if input.RecoveryValue != nil {
		scale := input.AmountScale
		if input.RecoveryScale != nil {
			scale = *input.RecoveryScale
		}
		postings = append(postings,
			PostingInput{AccountID: input.CashAccountID, QuantityValue: exact.New(*input.RecoveryValue), QuantityScale: scale, CommodityID: input.CashCommodityID, Memo: memo},
			PostingInput{AccountID: *input.RecoveryAccountID, QuantityValue: exact.New(-*input.RecoveryValue), QuantityScale: scale, CommodityID: input.CashCommodityID, Memo: memo},
		)
	}
	return investmentTransactionPlan{
		AccountRuleDependencies: dependencies,
		Date:                    date,
		Create: CreateTransactionInput{
			OwnerUserID:            input.OwnerUserID,
			AuthSessionID:          input.AuthSessionID,
			RequestID:              input.RequestID,
			OriginType:             defaultString(input.OriginType, "browser_api"),
			Operation:              defaultString(input.Operation, "investment.payment_in_lieu"),
			ChangeReason:           input.ChangeReason,
			ReconciliationOverride: input.ReconciliationOverride.Allowed, ReconciliationOverrideCheckpointIDs: input.ReconciliationOverride.Reviewed,
			Spec: TransactionInput{
				Status:                  "posted",
				TransactionKind:         "investment",
				InvestmentOperationKind: "payment_in_lieu",
				TransactionDate:         date,
				PayeeID:                 input.PayeeID,
				Description:             memo,
				ExternalRefHint:         input.ExternalRefHint,
				JournalEntries: []JournalEntryInput{{
					EntryDate: date,
					EntryKind: "investment",
					Memo:      memo,
					Postings:  postings,
				}},
			},
		},
	}, exDate, nil
}

// preparePaymentInLieuWrite freezes the journal, its owed and recovered
// components and the entitlement facts. Entry, preview and replacement
// share it.
func (s *InvestmentService) preparePaymentInLieuWrite(ctx context.Context, input PaymentInLieuInput) (db.CreateTransactionParams, error) {
	plan, exDate, err := s.paymentInLieuPlan(ctx, input)
	if err != nil {
		return db.CreateTransactionParams{}, err
	}
	params, err := s.transactionService.prepareCreateTransactionForWriteCarrying(ctx, plan.Create, plan.AccountRuleDependencies)
	if err != nil {
		return db.CreateTransactionParams{}, err
	}
	params.PaymentInLieu = &db.PaymentInLieuFact{HoldingAccountID: input.HoldingAccountID, CommodityID: input.CommodityID, ExDate: exDate}
	// Components carry the owner's cash flow: the charge negative, the
	// recovery positive.
	params.InvestmentComponents = []db.InvestmentComponentSpec{{
		Kind: "payment_in_lieu", CommodityID: input.CashCommodityID,
		AmountValue: strconv.FormatInt(-input.AmountValue, 10), AmountScale: input.AmountScale, AmountDate: plan.Date,
	}}
	if input.RecoveryValue != nil {
		scale := input.AmountScale
		if input.RecoveryScale != nil {
			scale = *input.RecoveryScale
		}
		params.InvestmentComponents = append(params.InvestmentComponents, db.InvestmentComponentSpec{
			Kind: "payment_in_lieu_recovery", CommodityID: input.CashCommodityID,
			AmountValue: strconv.FormatInt(*input.RecoveryValue, 10), AmountScale: scale, AmountDate: plan.Date,
		})
	}
	return params, nil
}

type ReverseInvestmentPaymentInLieuInput struct {
	OwnerUserID            int64
	AuthSessionID          int64
	RequestID              string
	TransactionID          int64
	Reason                 string
	ReconciliationOverride CheckpointOverride
}

type ReplaceInvestmentPaymentInLieuInput struct {
	OwnerUserID            int64
	AuthSessionID          int64
	RequestID              string
	TransactionID          int64
	Reason                 string
	ReconciliationOverride CheckpointOverride
	Replacement            PaymentInLieuInput
}

type ReplaceInvestmentPaymentInLieuResult struct {
	Inverse                Transaction
	Replacement            Transaction
	CorrectedTransactionID int64
}

func (s *InvestmentService) ReversePaymentInLieu(ctx context.Context, input ReverseInvestmentPaymentInLieuInput) (Transaction, error) {
	operation, params, err := s.preparePaymentInLieuReversalWrite(ctx, input)
	if err != nil {
		return Transaction{}, err
	}
	record, err := s.repository.ReversePaymentInLieu(ctx, params, operation)
	if err != nil {
		return Transaction{}, mapPaymentInLieuError(err)
	}
	return s.transactionService.enrichOne(ctx, toTransaction(record))
}

func (s *InvestmentService) ReversePaymentInLieuReconciliationImpact(ctx context.Context, input ReverseInvestmentPaymentInLieuInput) (ReconciliationImpact, error) {
	input.ReconciliationOverride = unboundOverride
	operation, params, err := s.preparePaymentInLieuReversalWrite(ctx, input)
	if err != nil {
		return ReconciliationImpact{}, err
	}
	simulated, err := s.repository.PreviewPaymentInLieuReversal(ctx, params, operation)
	if err != nil {
		return ReconciliationImpact{}, mapPaymentInLieuError(err)
	}
	return s.simulatedReconciliationImpact(ctx, simulated)
}

func (s *InvestmentService) preparePaymentInLieuReversalWrite(ctx context.Context, input ReverseInvestmentPaymentInLieuInput) (db.PaymentInLieuOperationRecord, db.CreateTransactionParams, error) {
	operation, _, planned, err := s.paymentInLieuCorrectionPlan(ctx, input.OwnerUserID, input.AuthSessionID, input.RequestID,
		input.TransactionID, input.Reason, input.ReconciliationOverride, "investment.payment_in_lieu.reverse")
	if err != nil {
		return db.PaymentInLieuOperationRecord{}, db.CreateTransactionParams{}, err
	}
	planned.Spec.InvestmentOperationKind = "reversal"
	params, err := s.transactionService.prepareCreateTransactionForWriteCarrying(ctx, planned, nil)
	if err != nil {
		return db.PaymentInLieuOperationRecord{}, db.CreateTransactionParams{}, err
	}
	fact := operation.Fact
	params.PaymentInLieu = &fact
	params.InvestmentCorrectionOfOperationID = operation.OperationID
	params.InvestmentCorrectionMode = "reverse"
	params.InvestmentCorrectionReason = planned.ChangeReason
	return operation, params, nil
}

func (s *InvestmentService) ReplacePaymentInLieu(ctx context.Context, input ReplaceInvestmentPaymentInLieuInput) (ReplaceInvestmentPaymentInLieuResult, error) {
	operation, inverse, replacement, err := s.preparePaymentInLieuReplacementWrite(ctx, input)
	if err != nil {
		return ReplaceInvestmentPaymentInLieuResult{}, err
	}
	record, err := s.repository.ReplacePaymentInLieu(ctx, operation, inverse, replacement)
	if err != nil {
		return ReplaceInvestmentPaymentInLieuResult{}, mapPaymentInLieuError(err)
	}
	enriched, err := s.transactionService.enrichRecords(ctx, record.Inverse, record.Replacement)
	if err != nil {
		return ReplaceInvestmentPaymentInLieuResult{}, err
	}
	return ReplaceInvestmentPaymentInLieuResult{Inverse: enriched[0], Replacement: enriched[1],
		CorrectedTransactionID: operation.TransactionID}, nil
}

func (s *InvestmentService) ReplacePaymentInLieuReconciliationImpact(ctx context.Context, input ReplaceInvestmentPaymentInLieuInput) (ReconciliationImpact, error) {
	input.ReconciliationOverride = unboundOverride
	operation, inverse, replacement, err := s.preparePaymentInLieuReplacementWrite(ctx, input)
	if err != nil {
		return ReconciliationImpact{}, err
	}
	simulated, err := s.repository.SimulatePaymentInLieuReplacement(ctx, operation, inverse, replacement)
	if err != nil {
		return ReconciliationImpact{}, mapPaymentInLieuError(err)
	}
	return s.simulatedReconciliationImpact(ctx, simulated)
}

// The replacement keeps the entitlement (holding, instrument, ex-date), the
// payment date, cash account and currency; the amount, expense account and
// recovery are what a correction changes.
func (s *InvestmentService) preparePaymentInLieuReplacementWrite(ctx context.Context, input ReplaceInvestmentPaymentInLieuInput) (db.PaymentInLieuOperationRecord, db.CreateTransactionParams, db.CreateTransactionParams, error) {
	const operationCode = "investment.payment_in_lieu.replace"
	operation, original, inversePlan, err := s.paymentInLieuCorrectionPlan(ctx, input.OwnerUserID, input.AuthSessionID, input.RequestID,
		input.TransactionID, input.Reason, input.ReconciliationOverride, operationCode)
	if err != nil {
		return db.PaymentInLieuOperationRecord{}, db.CreateTransactionParams{}, db.CreateTransactionParams{}, err
	}
	replacement := input.Replacement
	cash := original.JournalEntries[0].Postings[0]
	if replacement.HoldingAccountID == 0 {
		replacement.HoldingAccountID = operation.Fact.HoldingAccountID
	}
	if replacement.CommodityID == 0 {
		replacement.CommodityID = operation.Fact.CommodityID
	}
	if replacement.ExDate == "" {
		replacement.ExDate = operation.Fact.ExDate
	}
	if replacement.HoldingAccountID != operation.Fact.HoldingAccountID || replacement.CommodityID != operation.Fact.CommodityID ||
		replacement.ExDate != operation.Fact.ExDate || replacement.TransactionDate != operation.EventDate ||
		replacement.CashAccountID != cash.AccountID || replacement.CashCommodityID != cash.CommodityID {
		return db.PaymentInLieuOperationRecord{}, db.CreateTransactionParams{}, db.CreateTransactionParams{},
			ValidationError{Message: "replacement must keep the short holding, instrument, ex-date, payment date, cash account and currency"}
	}
	replacement.OwnerUserID = input.OwnerUserID
	replacement.AuthSessionID = input.AuthSessionID
	replacement.RequestID = input.RequestID
	replacement.OriginType = "browser_api"
	replacement.Operation = operationCode
	replacement.ChangeReason = inversePlan.ChangeReason
	replacement.ReconciliationOverride = input.ReconciliationOverride
	replacement.ExternalRefHint = ""
	inverseParams, err := s.transactionService.prepareCreateTransactionForWriteCarrying(ctx, inversePlan, nil)
	if err != nil {
		return db.PaymentInLieuOperationRecord{}, db.CreateTransactionParams{}, db.CreateTransactionParams{}, err
	}
	replacementParams, err := s.preparePaymentInLieuWrite(ctx, replacement)
	if err != nil {
		return db.PaymentInLieuOperationRecord{}, db.CreateTransactionParams{}, db.CreateTransactionParams{}, err
	}
	replacementParams.CorrectionOfTransactionID = inverseParams.CorrectionOfTransactionID
	replacementParams.InvestmentCorrectionOfOperationID = operation.OperationID
	replacementParams.InvestmentCorrectionMode = "replace"
	replacementParams.InvestmentCorrectionReason = inversePlan.ChangeReason
	replacementParams.CreatedAt = inverseParams.CreatedAt
	return operation, inverseParams, replacementParams, nil
}

// paymentInLieuCorrectionPlan pins the effective payment and plans its exact
// inverse. The writer rechecks the same facts inside its transaction.
func (s *InvestmentService) paymentInLieuCorrectionPlan(ctx context.Context, ownerUserID, authSessionID int64, requestID string,
	transactionID int64, reason string, override CheckpointOverride, operationCode string,
) (db.PaymentInLieuOperationRecord, Transaction, CreateTransactionInput, error) {
	if ownerUserID <= 0 || transactionID <= 0 {
		return db.PaymentInLieuOperationRecord{}, Transaction{}, CreateTransactionInput{}, ValidationError{Message: "owner and payment in lieu id are required"}
	}
	if strings.TrimSpace(reason) == "" {
		return db.PaymentInLieuOperationRecord{}, Transaction{}, CreateTransactionInput{}, ValidationError{Message: "payment in lieu correction reason is required"}
	}
	cleanReason, err := cleanChangeReason(reason, "")
	if err != nil {
		return db.PaymentInLieuOperationRecord{}, Transaction{}, CreateTransactionInput{}, err
	}
	operation, err := s.repository.PaymentInLieuOperationByTransactionID(ctx, BookID, transactionID)
	if err != nil {
		return db.PaymentInLieuOperationRecord{}, Transaction{}, CreateTransactionInput{}, mapPaymentInLieuError(err)
	}
	if operation.AlreadyCorrected {
		return db.PaymentInLieuOperationRecord{}, Transaction{}, CreateTransactionInput{}, ErrInvestmentPaymentInLieuAlreadyCorrected
	}
	original, err := s.transactionService.Transaction(ctx, operation.TransactionID)
	if err != nil {
		return db.PaymentInLieuOperationRecord{}, Transaction{}, CreateTransactionInput{}, err
	}
	if original.VersionID != operation.CurrentVersionID || original.Status != "posted" ||
		original.TransactionKind != "investment" || original.DeletedAt != "" ||
		original.TransactionDate != operation.EventDate ||
		len(original.JournalEntries) == 0 || len(original.JournalEntries[0].Postings) == 0 {
		return db.PaymentInLieuOperationRecord{}, Transaction{}, CreateTransactionInput{}, ErrInvestmentPaymentInLieuChanged
	}
	return operation, original, CreateTransactionInput{
		OwnerUserID: ownerUserID, AuthSessionID: authSessionID, RequestID: requestID,
		OriginType: "browser_api", Operation: operationCode,
		CorrectionOfTransactionID: &operation.TransactionID,
		Spec:                      invertedInvestmentTransactionSpec(original), ChangeReason: cleanReason,
		ReconciliationOverride: override.Allowed, ReconciliationOverrideCheckpointIDs: override.Reviewed,
	}, nil
}

// InvestmentCorrectionPaymentInLieuTerms are an effective payment in lieu's
// posted facts, which pre-fill its replacement.
type InvestmentCorrectionPaymentInLieuTerms struct {
	HoldingAccountID  int64
	CommodityID       int64
	ExDate            string
	EventDate         string
	CashAccountID     int64
	CashCommodityID   int64
	ExpenseAccountID  int64
	AmountValue       exact.Coefficient
	AmountScale       int
	RecoveryAccountID *int64
	RecoveryValue     *exact.Coefficient
	RecoveryScale     *int
	Memo              string
	PayeeID           *int64
}

// paymentInLieuCorrectionTerms reads a posted payment in the shape
// paymentInLieuPlan writes: cash −a, expense +a, then optionally cash +r and
// recovery −r. Any other shape is not offered for correction.
func paymentInLieuCorrectionTerms(transaction Transaction, fact db.PaymentInLieuFact) (*InvestmentCorrectionPaymentInLieuTerms, bool) {
	if len(transaction.JournalEntries) != 1 {
		return nil, false
	}
	postings := transaction.JournalEntries[0].Postings
	if len(postings) != 2 && len(postings) != 4 {
		return nil, false
	}
	cash, expense := postings[0], postings[1]
	if cash.CommodityID != expense.CommodityID || cash.QuantityScale != expense.QuantityScale ||
		cash.QuantityValue.Sign() >= 0 || expense.QuantityValue != cash.QuantityValue.Negated() {
		return nil, false
	}
	terms := &InvestmentCorrectionPaymentInLieuTerms{
		HoldingAccountID: fact.HoldingAccountID, CommodityID: fact.CommodityID, ExDate: fact.ExDate,
		EventDate: transaction.TransactionDate, CashAccountID: cash.AccountID, CashCommodityID: cash.CommodityID,
		ExpenseAccountID: expense.AccountID, AmountValue: expense.QuantityValue, AmountScale: expense.QuantityScale,
		Memo: transaction.Description, PayeeID: transaction.PayeeID,
	}
	if len(postings) == 4 {
		received, recovery := postings[2], postings[3]
		if received.AccountID != cash.AccountID || received.CommodityID != cash.CommodityID ||
			recovery.CommodityID != cash.CommodityID || received.QuantityScale != recovery.QuantityScale ||
			received.QuantityValue.Sign() <= 0 || recovery.QuantityValue != received.QuantityValue.Negated() {
			return nil, false
		}
		account, value, scale := recovery.AccountID, received.QuantityValue, received.QuantityScale
		terms.RecoveryAccountID, terms.RecoveryValue, terms.RecoveryScale = &account, &value, &scale
	}
	return terms, true
}

func mapPaymentInLieuError(err error) error {
	switch {
	case errors.Is(err, db.ErrPaymentInLieuNotShort):
		return ErrInvestmentPaymentInLieuNotShort
	case errors.Is(err, db.ErrNotFound):
		return ErrInvestmentPaymentInLieuNotFound
	case errors.Is(err, db.ErrInvestmentOperationAlreadyCorrected):
		return ErrInvestmentPaymentInLieuAlreadyCorrected
	case errors.Is(err, db.ErrInvestmentSaleChanged):
		return ErrInvestmentPaymentInLieuChanged
	case errors.Is(err, db.ErrInvalidDisposalParams):
		return ValidationError{Message: err.Error()}
	default:
		return fmt.Errorf("payment in lieu: %w", mapTransactionDBError(err))
	}
}

// proposedTransactionKindPaymentInLieu is the provider proposal for a
// dividend whose ex-date found the book short only (#255). It is always
// incomplete: the owner supplies the payment date, cash account, amount
// owed, currency and expense account, and it commits through the native
// payment-in-lieu command.
const proposedTransactionKindPaymentInLieu = "payment_in_lieu"

type proposedPaymentInLieu struct {
	Kind              string  `json:"kind"`
	CommodityID       int64   `json:"commodity_id"`
	ExDate            string  `json:"ex_date"`
	HoldingAccountIDs []int64 `json:"holding_account_ids"`
	ExternalRefHint   string  `json:"external_ref_hint"`
}

func parseProposedPaymentInLieu(raw string) (proposedPaymentInLieu, bool) {
	var proposal proposedPaymentInLieu
	if err := json.Unmarshal([]byte(raw), &proposal); err != nil || proposal.Kind != proposedTransactionKindPaymentInLieu {
		return proposedPaymentInLieu{}, false
	}
	return proposal, true
}

// acceptPaymentInLieuSuggestion posts the owner's completion through the
// payment-in-lieu command, with the suggestion and event status in the same
// transaction. Every refusal, the entitlement included, is returned and
// leaves the suggestion pending.
func (s *InvestmentService) acceptPaymentInLieuSuggestion(ctx context.Context, record db.EventSuggestionRecord, proposal proposedPaymentInLieu,
	completion *SuggestionCompletion, now string, ownerUserID, authSessionID int64, requestID string,
) (InvestmentEventSuggestion, error) {
	if completion == nil {
		return InvestmentEventSuggestion{}, ValidationError{Message: "this payment in lieu is incomplete: supply the payment date, cash account, amount owed and expense account"}
	}
	if completion.IncomeAccountID != nil || completion.WithholdingValue != nil || completion.WithholdingAccountID != nil {
		return InvestmentEventSuggestion{}, ValidationError{Message: "a payment in lieu has no income or withholding; name an expense account and any recovery"}
	}
	if proposal.CommodityID <= 0 || proposal.ExDate != record.EventDate || len(proposal.HoldingAccountIDs) == 0 {
		return s.failSuggestion(ctx, record.ID, "payment in lieu proposal is malformed", now, ownerUserID, authSessionID, requestID)
	}
	holdingAccountID := proposal.HoldingAccountIDs[0]
	if completion.HoldingAccountID != nil {
		holdingAccountID = *completion.HoldingAccountID
	} else if len(proposal.HoldingAccountIDs) > 1 {
		return InvestmentEventSuggestion{}, ValidationError{Message: "several holdings were short; name the one that owes this payment"}
	}
	if !slices.Contains(proposal.HoldingAccountIDs, holdingAccountID) {
		return InvestmentEventSuggestion{}, ValidationError{Message: "the holding must be one the evidence shows short at the close before the ex-date"}
	}
	expenseAccountID := int64(0)
	if completion.ExpenseAccountID != nil {
		expenseAccountID = *completion.ExpenseAccountID
	}
	_, err := s.paymentInLieuWithPostWrite(ctx, PaymentInLieuInput{
		OwnerUserID: ownerUserID, AuthSessionID: authSessionID, RequestID: requestID,
		HoldingAccountID: holdingAccountID, CommodityID: proposal.CommodityID, ExDate: proposal.ExDate,
		TransactionDate: completion.TransactionDate, CashAccountID: completion.CashAccountID,
		CashCommodityID: completion.CashCommodityID, ExpenseAccountID: expenseAccountID,
		AmountValue: completion.AmountValue, AmountScale: completion.AmountScale,
		RecoveryValue: completion.RecoveryValue, RecoveryScale: completion.RecoveryScale, RecoveryAccountID: completion.RecoveryAccountID,
		Memo: completion.Memo, ExternalRefHint: proposal.ExternalRefHint,
		ReconciliationOverride: completion.ReconciliationOverride,
		ChangeReason:           "accepted provider event suggestion",
		Operation:              "investment.event_suggestion.accept",
	}, func(tx *sql.Tx, transactionID int64) error {
		return s.repository.MarkSuggestionAcceptedInTx(ctx, tx, BookID, record.ID, transactionID, now, ownerUserID, authSessionID, requestID)
	})
	if errors.Is(err, db.ErrEventSuggestionNotPending) {
		return InvestmentEventSuggestion{}, ErrInvestmentSuggestionNotPending
	}
	if err != nil {
		return InvestmentEventSuggestion{}, err
	}
	accepted, err := s.repository.EventSuggestionByID(ctx, BookID, record.ID)
	if err != nil {
		return InvestmentEventSuggestion{}, fmt.Errorf("reload accepted investment event suggestion: %w", err)
	}
	return toInvestmentEventSuggestion(accepted), nil
}
