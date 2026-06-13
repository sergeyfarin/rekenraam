package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"rekenraam/backend/internal/db"
)

const (
	categoryNameMaxBytes = 200
	categoryCodeMaxBytes = 100
	categoryIconMaxBytes = 64
	categoryListLimit    = 2000
)

var (
	ErrCategoryNotFound         = errors.New("category not found")
	ErrCategoryExists           = errors.New("category already exists")
	ErrCategoryParentInvalid    = errors.New("category parent is invalid")
	ErrCategoryHasChildren      = errors.New("category has active child categories")
	ErrCategoryBuiltInProtected = errors.New("built-in category cannot be deleted")
	ErrCategoryUsed             = errors.New("category is used by financial records")
	categoryIconPattern         = regexp.MustCompile(`^[a-z][a-z0-9_-]*$`)
)

var categoryTypes = map[string]bool{
	"income":  true,
	"expense": true,
}

type Category struct {
	ID               int64
	BookID           int64
	Status           string
	Code             string
	Name             string
	CategoryType     string
	ParentCategoryID *int64
	AllowsPostings   bool
	Icon             string
	IsBuiltin        bool
	IsStarter        bool
	BuiltinKey       string
	MetadataJSON     string
	OpenedOn         string
	ClosedOn         string
	EffectiveFrom    string
	ChangeReason     string
	CreatedAt        string
	UpdatedAt        string
}

type ListCategoriesInput struct {
	Status          string
	CategoryType    string
	IncludeArchived bool
	Query           string
}

type CreateCategoryInput struct {
	OwnerUserID      int64
	AuthSessionID    int64
	RequestID        string
	OriginType       string
	Operation        string
	Code             string
	Name             string
	CategoryType     string
	ParentCategoryID *int64
	AllowsPostings   *bool
	Icon             string
	OpenedOn         string
	EffectiveFrom    string
	ChangeReason     string
}

type UpdateCategoryInput struct {
	OwnerUserID      int64
	AuthSessionID    int64
	RequestID        string
	OriginType       string
	Operation        string
	CategoryID       int64
	Code             *string
	Name             *string
	CategoryType     *string
	ParentCategoryID *int64
	ClearParent      bool
	AllowsPostings   *bool
	Icon             *string
	OpenedOn         string
	EffectiveFrom    string
	ChangeReason     string
}

type CategoryLifecycleInput struct {
	OwnerUserID   int64
	AuthSessionID int64
	RequestID     string
	OriginType    string
	CategoryID    int64
	EffectiveFrom string
	ChangeReason  string
}

type EnsureCategoriesInput struct {
	OwnerUserID   int64
	AuthSessionID int64
	RequestID     string
	OriginType    string
	Operation     string
}

type EnsureCategoriesResult struct {
	Categories  []Category
	SetupStatus SetupStatus
}

type CategoryService struct {
	repository   *db.CategoryRepository
	setupService *SetupService
	now          func() time.Time
}

func NewCategoryService(repository *db.CategoryRepository, setupService *SetupService) *CategoryService {
	return &CategoryService{
		repository:   repository,
		setupService: setupService,
		now:          time.Now,
	}
}

func (s *CategoryService) SetNowForTest(now func() time.Time) {
	s.now = now
}

func (s *CategoryService) ListCategories(ctx context.Context, input ListCategoriesInput) ([]Category, error) {
	status := strings.TrimSpace(input.Status)
	if status != "" && status != "active" && status != "archived" {
		return nil, ValidationError{Message: "category status is invalid"}
	}
	categoryType := strings.TrimSpace(input.CategoryType)
	if categoryType != "" && !categoryTypes[categoryType] {
		return nil, ValidationError{Message: "category type is invalid"}
	}

	records, err := s.repository.ListCategories(ctx, db.ListCategoriesParams{
		BookID:          BookID,
		Status:          status,
		CategoryType:    categoryType,
		IncludeArchived: input.IncludeArchived,
		Query:           strings.TrimSpace(input.Query),
		Limit:           categoryListLimit,
	})
	if err != nil {
		return nil, fmt.Errorf("list categories: %w", err)
	}

	return toCategories(records), nil
}

func (s *CategoryService) Category(ctx context.Context, categoryID int64) (Category, error) {
	if categoryID <= 0 {
		return Category{}, ValidationError{Message: "category id is required"}
	}

	record, err := s.repository.CategoryByID(ctx, BookID, categoryID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return Category{}, ErrCategoryNotFound
		}
		return Category{}, fmt.Errorf("read category: %w", err)
	}

	return toCategory(record), nil
}

func (s *CategoryService) CreateCategory(ctx context.Context, input CreateCategoryInput) (Category, error) {
	if input.OwnerUserID <= 0 {
		return Category{}, ValidationError{Message: "owner user is required"}
	}

	now := s.now().UTC()
	spec, err := s.newCategorySpec(ctx, categorySpecInput{
		Code:             input.Code,
		Name:             input.Name,
		CategoryType:     input.CategoryType,
		ParentCategoryID: input.ParentCategoryID,
		AllowsPostings:   input.AllowsPostings,
		Icon:             input.Icon,
		OpenedOn:         input.OpenedOn,
		EffectiveFrom:    input.EffectiveFrom,
		Now:              now,
	})
	if err != nil {
		return Category{}, err
	}
	effectiveInput := input.EffectiveFrom
	if strings.TrimSpace(effectiveInput) == "" {
		effectiveInput = spec.OpenedOn
	}
	effectiveFrom, err := cleanEffectiveFrom(effectiveInput, now)
	if err != nil {
		return Category{}, err
	}
	changeReason, err := cleanChangeReason(input.ChangeReason, "created category")
	if err != nil {
		return Category{}, err
	}

	record, err := s.repository.CreateCategory(ctx, db.CreateCategoryParams{
		BookID:          BookID,
		CreatedByUserID: input.OwnerUserID,
		AuthSessionID:   input.AuthSessionID,
		RequestID:       input.RequestID,
		OriginType:      input.OriginType,
		Operation:       input.Operation,
		Spec:            spec,
		ChangeReason:    changeReason,
		CreatedAt:       now.Format(time.RFC3339),
		EffectiveFrom:   effectiveFrom,
	})
	if err != nil {
		return Category{}, categoryRepositoryError("persist category", err)
	}

	return toCategory(record), nil
}

func (s *CategoryService) UpdateCategory(ctx context.Context, input UpdateCategoryInput) (Category, error) {
	if input.OwnerUserID <= 0 {
		return Category{}, ValidationError{Message: "owner user is required"}
	}
	if input.CategoryID <= 0 {
		return Category{}, ValidationError{Message: "category id is required"}
	}

	current, err := s.Category(ctx, input.CategoryID)
	if err != nil {
		return Category{}, err
	}
	if current.IsBuiltin {
		return Category{}, ErrCategoryBuiltInProtected
	}

	code := current.Code
	if input.Code != nil {
		code = *input.Code
	}
	name := current.Name
	if input.Name != nil {
		name = *input.Name
	}
	categoryType := current.CategoryType
	if input.CategoryType != nil {
		categoryType = *input.CategoryType
	}
	parentID := current.ParentCategoryID
	if input.ClearParent {
		parentID = nil
	} else if input.ParentCategoryID != nil {
		parentID = input.ParentCategoryID
	}
	allowsPostings := current.AllowsPostings
	if input.AllowsPostings != nil {
		allowsPostings = *input.AllowsPostings
	}
	icon := current.Icon
	if input.Icon != nil {
		icon = *input.Icon
	}

	now := s.now().UTC()
	spec, err := s.newCategorySpec(ctx, categorySpecInput{
		CategoryID:       input.CategoryID,
		Code:             code,
		Name:             name,
		CategoryType:     categoryType,
		ParentCategoryID: parentID,
		AllowsPostings:   &allowsPostings,
		Icon:             icon,
		OpenedOn:         input.OpenedOn,
		EffectiveFrom:    input.EffectiveFrom,
		Now:              now,
		Current:          &current,
	})
	if err != nil {
		return Category{}, err
	}
	effectiveFrom, err := cleanEffectiveFrom(input.EffectiveFrom, now)
	if err != nil {
		return Category{}, err
	}
	changeReason, err := cleanChangeReason(input.ChangeReason, "updated category")
	if err != nil {
		return Category{}, err
	}

	record, err := s.repository.UpdateCategory(ctx, db.UpdateCategoryParams{
		BookID:          BookID,
		CategoryID:      input.CategoryID,
		ChangedByUserID: input.OwnerUserID,
		AuthSessionID:   input.AuthSessionID,
		RequestID:       input.RequestID,
		OriginType:      input.OriginType,
		Operation:       input.Operation,
		Spec:            spec,
		ChangeReason:    changeReason,
		RecordedAt:      now.Format(time.RFC3339),
		EffectiveFrom:   effectiveFrom,
	})
	if err != nil {
		return Category{}, categoryRepositoryError("persist category update", err)
	}

	return toCategory(record), nil
}

func (s *CategoryService) DisableCategory(ctx context.Context, input CategoryLifecycleInput) (Category, error) {
	return s.changeCategoryStatus(ctx, input, "archived", "disabled category", "category.disable")
}

func (s *CategoryService) RestoreCategory(ctx context.Context, input CategoryLifecycleInput) (Category, error) {
	return s.changeCategoryStatus(ctx, input, "active", "restored category", "category.restore")
}

func (s *CategoryService) DeleteCategory(ctx context.Context, input CategoryLifecycleInput) error {
	if input.OwnerUserID <= 0 {
		return ValidationError{Message: "owner user is required"}
	}
	if input.CategoryID <= 0 {
		return ValidationError{Message: "category id is required"}
	}

	now := s.now().UTC()
	err := s.repository.DeleteUnusedCategory(ctx, db.DeleteCategoryParams{
		BookID:        BookID,
		CategoryID:    input.CategoryID,
		ActorUserID:   input.OwnerUserID,
		AuthSessionID: input.AuthSessionID,
		RequestID:     input.RequestID,
		OriginType:    input.OriginType,
		Operation:     "category.delete",
		OccurredAt:    now.Format(time.RFC3339),
	})
	if err != nil {
		return categoryRepositoryError("delete category", err)
	}
	return nil
}

func (s *CategoryService) EnsureCategories(ctx context.Context, input EnsureCategoriesInput) (EnsureCategoriesResult, error) {
	if input.OwnerUserID <= 0 {
		return EnsureCategoriesResult{}, ValidationError{Message: "owner user is required"}
	}

	now := s.now().UTC()
	result, err := s.repository.EnsureCategories(ctx, db.EnsureCategoriesParams{
		BookID:          BookID,
		ChangedByUserID: input.OwnerUserID,
		AuthSessionID:   input.AuthSessionID,
		RequestID:       input.RequestID,
		OriginType:      input.OriginType,
		Operation:       input.Operation,
		Specs:           builtinCategorySpecs(),
		CreatedAt:       now.Format(time.RFC3339),
		EffectiveFrom:   systemAccountDate,
	})
	if err != nil {
		switch {
		case errors.Is(err, db.ErrCategoriesSetupComplete):
			return EnsureCategoriesResult{}, ErrSetupAlreadyComplete
		case errors.Is(err, db.ErrNotFound):
			return EnsureCategoriesResult{}, ErrCategoryNotFound
		default:
			return EnsureCategoriesResult{}, fmt.Errorf("persist categories setup: %w", err)
		}
	}

	setupStatus, err := s.setupService.Status(ctx)
	if err != nil {
		return EnsureCategoriesResult{}, fmt.Errorf("reload setup status: %w", err)
	}

	return EnsureCategoriesResult{
		Categories:  toCategories(result.Categories),
		SetupStatus: setupStatus,
	}, nil
}

func (s *CategoryService) changeCategoryStatus(ctx context.Context, input CategoryLifecycleInput, status string, reason string, operation string) (Category, error) {
	if input.OwnerUserID <= 0 {
		return Category{}, ValidationError{Message: "owner user is required"}
	}
	if input.CategoryID <= 0 {
		return Category{}, ValidationError{Message: "category id is required"}
	}

	current, err := s.Category(ctx, input.CategoryID)
	if err != nil {
		return Category{}, err
	}
	if current.IsBuiltin && status == "archived" {
		return Category{}, ErrCategoryBuiltInProtected
	}

	now := s.now().UTC()
	effectiveFrom, err := cleanEffectiveFrom(input.EffectiveFrom, now)
	if err != nil {
		return Category{}, err
	}
	closedOn := sql.NullString{}
	if status == "archived" {
		closedOn = sql.NullString{String: effectiveFrom, Valid: true}
	}
	changeReason, err := cleanChangeReason(input.ChangeReason, reason)
	if err != nil {
		return Category{}, err
	}

	record, err := s.repository.ChangeCategoryStatus(ctx, db.CategoryLifecycleParams{
		BookID:          BookID,
		CategoryID:      input.CategoryID,
		ChangedByUserID: input.OwnerUserID,
		AuthSessionID:   input.AuthSessionID,
		RequestID:       input.RequestID,
		OriginType:      input.OriginType,
		Operation:       operation,
		ClosedOn:        closedOn,
		Status:          status,
		Reason:          changeReason,
		RecordedAt:      now.Format(time.RFC3339),
		EffectiveFrom:   effectiveFrom,
	})
	if err != nil {
		return Category{}, categoryRepositoryError("persist category status", err)
	}

	_ = current
	return toCategory(record), nil
}

type categorySpecInput struct {
	CategoryID       int64
	Code             string
	Name             string
	CategoryType     string
	ParentCategoryID *int64
	AllowsPostings   *bool
	Icon             string
	OpenedOn         string
	EffectiveFrom    string
	Now              time.Time
	Current          *Category
}

func (s *CategoryService) newCategorySpec(ctx context.Context, input categorySpecInput) (db.AccountSpec, error) {
	code := strings.TrimSpace(input.Code)
	if len(code) > categoryCodeMaxBytes {
		return db.AccountSpec{}, ValidationError{Message: fmt.Sprintf("category code must be at most %d bytes", categoryCodeMaxBytes)}
	}
	if hasControlCharacter(code) {
		return db.AccountSpec{}, ValidationError{Message: "category code must not contain control characters"}
	}

	name := strings.TrimSpace(input.Name)
	isBuiltin := input.Current != nil && input.Current.IsBuiltin
	isStarter := input.Current != nil && input.Current.IsStarter
	if name == "" && !isBuiltin && !isStarter {
		return db.AccountSpec{}, ValidationError{Message: "category name is required"}
	}
	if len(name) > categoryNameMaxBytes {
		return db.AccountSpec{}, ValidationError{Message: fmt.Sprintf("category name must be at most %d bytes", categoryNameMaxBytes)}
	}
	if hasControlCharacter(name) {
		return db.AccountSpec{}, ValidationError{Message: "category name must not contain control characters"}
	}

	categoryType := strings.TrimSpace(input.CategoryType)
	if !categoryTypes[categoryType] {
		return db.AccountSpec{}, ValidationError{Message: "category type is invalid"}
	}
	if input.Current != nil && input.Current.CategoryType != categoryType {
		return db.AccountSpec{}, ValidationError{Message: "category type cannot be changed"}
	}

	icon := strings.TrimSpace(input.Icon)
	if len(icon) > categoryIconMaxBytes {
		return db.AccountSpec{}, ValidationError{Message: fmt.Sprintf("category icon must be at most %d bytes", categoryIconMaxBytes)}
	}
	if icon != "" && !categoryIconPattern.MatchString(icon) {
		return db.AccountSpec{}, ValidationError{Message: "category icon may contain lowercase letters, numbers, underscores, and hyphens"}
	}

	allowsPostings := true
	if input.AllowsPostings != nil {
		allowsPostings = *input.AllowsPostings
	}

	parentID := nullableInt64(input.ParentCategoryID)
	if parentID.Valid {
		if input.CategoryID > 0 && parentID.Int64 == input.CategoryID {
			return db.AccountSpec{}, ErrCategoryParentInvalid
		}
		parent, err := s.Category(ctx, parentID.Int64)
		if err != nil {
			return db.AccountSpec{}, ErrCategoryParentInvalid
		}
		if parent.Status == "archived" || parent.CategoryType != categoryType {
			return db.AccountSpec{}, ErrCategoryParentInvalid
		}
		cycle, err := s.repository.WouldCreateCycle(ctx, BookID, input.CategoryID, parentID.Int64)
		if err != nil {
			return db.AccountSpec{}, fmt.Errorf("check category parent cycle: %w", err)
		}
		if cycle {
			return db.AccountSpec{}, ErrCategoryParentInvalid
		}
	}

	openedOn, err := cleanAccountOpenedOn(input.OpenedOn, input.EffectiveFrom, input.Now)
	if err != nil {
		return db.AccountSpec{}, err
	}
	if input.Current != nil && strings.TrimSpace(input.OpenedOn) == "" {
		openedOn = input.Current.OpenedOn
	}
	effectiveInput := input.EffectiveFrom
	if strings.TrimSpace(effectiveInput) == "" {
		effectiveInput = openedOn
	}
	effectiveFrom, err := cleanEffectiveFrom(effectiveInput, input.Now)
	if err != nil {
		return db.AccountSpec{}, err
	}
	if effectiveFrom < openedOn {
		return db.AccountSpec{}, ValidationError{Message: "category effective date must not be before opened date"}
	}

	metadata, err := categoryMetadataJSON(categoryMetadata{
		Type:         categoryType,
		IsBuiltin:    false,
		IsStarter:    isStarter,
		BuiltinKey:   builtinKeyForStarter(input.Current),
		NameOverride: "",
		Icon:         icon,
	})
	if err != nil {
		return db.AccountSpec{}, err
	}
	if isBuiltin {
		metadata, err = categoryMetadataJSON(categoryMetadata{
			Type:         categoryType,
			IsBuiltin:    true,
			IsStarter:    false,
			BuiltinKey:   input.Current.BuiltinKey,
			NameOverride: name,
			Icon:         icon,
		})
		if err != nil {
			return db.AccountSpec{}, err
		}
		code = input.Current.Code
	}

	return db.AccountSpec{
		Code:               code,
		Name:               name,
		AccountClass:       categoryType,
		AccountKind:        categoryType,
		ParentAccountID:    parentID,
		AllowsPostings:     allowsPostings,
		CommentMarkdown:    "",
		MetadataJSON:       metadata,
		OpenedOn:           openedOn,
		ClosedOn:           nullableSQLString(""),
		DefaultCommodityID: sql.NullInt64{},
	}, nil
}

type categoryMetadata struct {
	Type         string
	IsBuiltin    bool
	IsStarter    bool
	BuiltinKey   string
	NameOverride string
	Icon         string
}

func categoryMetadataJSON(metadata categoryMetadata) (string, error) {
	envelope := struct {
		Category struct {
			Type         string `json:"type"`
			IsBuiltin    bool   `json:"is_builtin"`
			IsStarter    bool   `json:"is_starter,omitempty"`
			BuiltinKey   string `json:"builtin_key,omitempty"`
			NameOverride string `json:"name_override,omitempty"`
			Icon         string `json:"icon,omitempty"`
		} `json:"category"`
	}{}
	envelope.Category.Type = metadata.Type
	envelope.Category.IsBuiltin = metadata.IsBuiltin
	envelope.Category.IsStarter = metadata.IsStarter
	envelope.Category.BuiltinKey = metadata.BuiltinKey
	envelope.Category.NameOverride = metadata.NameOverride
	envelope.Category.Icon = metadata.Icon

	data, err := json.Marshal(envelope)
	if err != nil {
		return "", fmt.Errorf("encode category metadata: %w", err)
	}
	return string(data), nil
}

func parseCategoryMetadata(jsonText string) categoryMetadata {
	var envelope struct {
		Category struct {
			Type         string `json:"type"`
			IsBuiltin    bool   `json:"is_builtin"`
			IsStarter    bool   `json:"is_starter"`
			BuiltinKey   string `json:"builtin_key"`
			NameOverride string `json:"name_override"`
			Icon         string `json:"icon"`
		} `json:"category"`
	}
	_ = json.Unmarshal([]byte(jsonText), &envelope)
	return categoryMetadata{
		Type:         envelope.Category.Type,
		IsBuiltin:    envelope.Category.IsBuiltin,
		IsStarter:    envelope.Category.IsStarter,
		BuiltinKey:   envelope.Category.BuiltinKey,
		NameOverride: envelope.Category.NameOverride,
		Icon:         envelope.Category.Icon,
	}
}

func builtinKeyForStarter(category *Category) string {
	if category == nil || !category.IsStarter {
		return ""
	}
	return category.BuiltinKey
}

func builtinCategorySpecs() []db.CategorySeedSpec {
	specs := []db.CategorySeedSpec{}
	add := func(code string, categoryType string, builtinKey string, parentCode string, allowsPostings bool, protected bool) {
		metadata, _ := categoryMetadataJSON(categoryMetadata{
			Type:       categoryType,
			IsBuiltin:  protected,
			IsStarter:  !protected,
			BuiltinKey: builtinKey,
		})
		specs = append(specs, db.CategorySeedSpec{
			Code:           code,
			CategoryType:   categoryType,
			BuiltinKey:     builtinKey,
			ParentCode:     parentCode,
			AllowsPostings: allowsPostings,
			MetadataJSON:   metadata,
		})
	}
	addExpenseGroup := func(code string, key string) { add(code, "expense", key, "", false, false) }
	addExpense := func(code string, key string, parent string) { add(code, "expense", key, parent, true, false) }
	addBuiltinExpense := func(code string, key string) { add(code, "expense", key, "", true, true) }
	addIncome := func(code string, key string) { add(code, "income", key, "", true, false) }
	addBuiltinIncome := func(code string, key string) { add(code, "income", key, "", true, true) }

	addExpenseGroup("expense_housing", "expense.housing")
	addExpense("expense_housing_rent_mortgage", "expense.housing.rent_mortgage", "expense_housing")
	addExpense("expense_housing_utilities", "expense.housing.utilities", "expense_housing")
	addExpense("expense_housing_home_maintenance", "expense.housing.home_maintenance", "expense_housing")
	addExpense("expense_housing_home_supplies", "expense.housing.home_supplies", "expense_housing")
	addExpenseGroup("expense_food", "expense.food")
	addExpense("expense_food_groceries", "expense.food.groceries", "expense_food")
	addExpense("expense_food_dining_out", "expense.food.dining_out", "expense_food")
	addExpense("expense_food_coffee_snacks", "expense.food.coffee_snacks", "expense_food")
	addExpenseGroup("expense_transport", "expense.transport")
	addExpense("expense_transport_fuel", "expense.transport.fuel", "expense_transport")
	addExpense("expense_transport_public_transport", "expense.transport.public_transport", "expense_transport")
	addExpense("expense_transport_taxi_rideshare", "expense.transport.taxi_rideshare", "expense_transport")
	addExpense("expense_transport_parking", "expense.transport.parking", "expense_transport")
	addExpense("expense_transport_vehicle_maintenance", "expense.transport.vehicle_maintenance", "expense_transport")
	addExpenseGroup("expense_health", "expense.health")
	addExpense("expense_health_medical", "expense.health.medical", "expense_health")
	addExpense("expense_health_pharmacy", "expense.health.pharmacy", "expense_health")
	addExpense("expense_health_dental_vision", "expense.health.dental_vision", "expense_health")
	addExpense("expense_health_fitness", "expense.health.fitness", "expense_health")
	addExpenseGroup("expense_insurance", "expense.insurance")
	addExpense("expense_insurance_health", "expense.insurance.health", "expense_insurance")
	addExpense("expense_insurance_home_renters", "expense.insurance.home_renters", "expense_insurance")
	addExpense("expense_insurance_vehicle", "expense.insurance.vehicle", "expense_insurance")
	addExpense("expense_insurance_life", "expense.insurance.life", "expense_insurance")
	addExpenseGroup("expense_family", "expense.family")
	addExpense("expense_family_childcare", "expense.family.childcare", "expense_family")
	addExpense("expense_family_education", "expense.family.education", "expense_family")
	addExpense("expense_family_pets", "expense.family.pets", "expense_family")
	addExpense("expense_family_gifts", "expense.family.gifts", "expense_family")
	addExpenseGroup("expense_personal", "expense.personal")
	addExpense("expense_personal_clothing", "expense.personal.clothing", "expense_personal")
	addExpense("expense_personal_personal_care", "expense.personal.personal_care", "expense_personal")
	addExpense("expense_personal_subscriptions", "expense.personal.subscriptions", "expense_personal")
	addExpense("expense_personal_hobbies", "expense.personal.hobbies", "expense_personal")
	addExpenseGroup("expense_travel", "expense.travel")
	addExpense("expense_travel_flights", "expense.travel.flights", "expense_travel")
	addExpense("expense_travel_hotels", "expense.travel.hotels", "expense_travel")
	addExpense("expense_travel_vacation_spending", "expense.travel.vacation_spending", "expense_travel")
	addExpenseGroup("expense_financial", "expense.financial")
	addExpense("expense_financial_bank_fees", "expense.financial.bank_fees", "expense_financial")
	addExpense("expense_financial_interest", "expense.financial.interest", "expense_financial")
	addExpense("expense_financial_taxes", "expense.financial.taxes", "expense_financial")
	addExpenseGroup("expense_giving", "expense.giving")
	addExpense("expense_giving_charity", "expense.giving.charity", "expense_giving")
	addExpense("expense_giving_religious", "expense.giving.religious", "expense_giving")
	addBuiltinExpense("expense_other", "expense.other")

	addIncome("income_salary_wages", "income.salary_wages")
	addIncome("income_bonus", "income.bonus")
	addIncome("income_interest", "income.interest")
	addIncome("income_dividends", "income.dividends")
	addIncome("income_business_freelance", "income.business_freelance")
	addIncome("income_gifts_received", "income.gifts_received")
	addIncome("income_refunds_reimbursements", "income.refunds_reimbursements")
	addBuiltinIncome("income_other", "income.other")

	return specs
}

func categoryRepositoryError(action string, err error) error {
	switch {
	case errors.Is(err, db.ErrNotFound):
		return ErrCategoryNotFound
	case errors.Is(err, db.ErrCategoryExists):
		return ErrCategoryExists
	case errors.Is(err, db.ErrCategoryHasChildren):
		return ErrCategoryHasChildren
	case errors.Is(err, db.ErrCategoryBuiltIn):
		return ErrCategoryBuiltInProtected
	case errors.Is(err, db.ErrCategoryUsed):
		return ErrCategoryUsed
	default:
		return fmt.Errorf("%s: %w", action, err)
	}
}

func toCategory(record db.AccountRecord) Category {
	metadata := parseCategoryMetadata(record.MetadataJSON)
	return Category{
		ID:               record.ID,
		BookID:           record.BookID,
		Status:           record.Status,
		Code:             nullableString(record.Code),
		Name:             nullableString(record.Name),
		CategoryType:     record.AccountClass,
		ParentCategoryID: int64Ptr(record.ParentAccountID),
		AllowsPostings:   record.AllowsPostings,
		Icon:             metadata.Icon,
		IsBuiltin:        metadata.IsBuiltin,
		IsStarter:        metadata.IsStarter,
		BuiltinKey:       metadata.BuiltinKey,
		MetadataJSON:     record.MetadataJSON,
		OpenedOn:         record.OpenedOn,
		ClosedOn:         nullableString(record.ClosedOn),
		EffectiveFrom:    record.EffectiveFrom,
		ChangeReason:     record.ChangeReason,
		CreatedAt:        record.CreatedAt,
		UpdatedAt:        record.RecordedAt,
	}
}

func toCategories(records []db.AccountRecord) []Category {
	categories := make([]Category, 0, len(records))
	for _, record := range records {
		categories = append(categories, toCategory(record))
	}
	return categories
}
