package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"rekenraam/backend/internal/db"
)

const (
	homeLayoutVersion      = 1
	homeLayoutNameLimit    = 80
	homeLayoutAccountLimit = 100
	HomeHeadlineAccount    = "account"
	HomeHeadlineAccountSet = "account_set"
	HomeHeadlineAuto       = "auto"
)

var ErrHomeLayoutConflict = errors.New("home layout revision is stale")

type HomeLayout struct {
	Version           int                          `json:"version"`
	Headline          string                       `json:"headline"`
	AccountID         int64                        `json:"account_id,omitempty"`
	AccountSet        *HomeLayoutAccountSet        `json:"account_set,omitempty"`
	ShowReview        bool                         `json:"show_review"`
	ShortcutAccountID int64                        `json:"shortcut_account_id,omitempty"`
	Shortcuts         []HomeLayoutShortcut         `json:"shortcuts"`
	Widgets           []string                     `json:"widgets"`
	WidgetOptions     map[string]HomeWidgetOptions `json:"widget_options"`
	ExploreOpen       bool                         `json:"explore_open"`
}

type HomeLayoutAccountSet struct {
	Name       string  `json:"name"`
	AccountIDs []int64 `json:"account_ids"`
}

type HomeLayoutShortcut struct {
	ID        string `json:"id"`
	AccountID int64  `json:"account_id,omitempty"`
	Label     string `json:"label,omitempty"`
}

type HomeWidgetOptions struct {
	Size       string  `json:"size,omitempty"`
	Period     string  `json:"period,omitempty"`
	AccountIDs []int64 `json:"account_ids,omitempty"`
}

type HomeLayoutAccount struct {
	ID           int64  `json:"id"`
	Name         string `json:"name"`
	Code         string `json:"code"`
	StarterKey   string `json:"starter_key,omitempty"`
	CurrencyCode string `json:"currency_code,omitempty"`
}

type HomeLayoutState struct {
	Revision int64               `json:"revision"`
	Layout   HomeLayout          `json:"layout"`
	Defaults HomeLayout          `json:"defaults"`
	Accounts []HomeLayoutAccount `json:"accounts"`
}

var homeLayoutShortcuts = []string{"add", "transfer", "import", "transactions", "spending", "reconcile", "account"}

var homeLayoutWidgetIDs = []string{
	"spending", "budgets", "accounts", "recurring", "net_worth_trend",
	"cashflow", "holdings", "returns", "allocation", "income", "forecast", "imports",
	"prices", "reconciliation", "backups",
}

var homeWideWidgets = []string{"spending", "net_worth_trend", "cashflow", "holdings", "allocation"}

func DefaultHomeLayout() HomeLayout {
	return HomeLayout{
		Version: homeLayoutVersion, Headline: HomeHeadlineAuto, ShowReview: true, ExploreOpen: true,
		Shortcuts:     []HomeLayoutShortcut{{ID: "add"}, {ID: "transfer"}, {ID: "import"}, {ID: "transactions"}, {ID: "spending"}},
		Widgets:       []string{"spending", "budgets", "accounts", "recurring", "net_worth_trend"},
		WidgetOptions: map[string]HomeWidgetOptions{"net_worth_trend": {Size: "wide"}},
	}
}

func homeLayoutAccounts(accounts []db.LedgerAccountRecord) []HomeLayoutAccount {
	result := make([]HomeLayoutAccount, 0, len(accounts))
	for _, account := range accounts {
		if account.Status == "active" && countsNetWorthAccount(account) && !account.SystemRole.Valid && !homeHoldingAccountKinds[account.AccountKind] {
			result = append(result, HomeLayoutAccount{ID: account.ID, Name: account.Name.String, Code: account.Code.String,
				StarterKey: account.StarterKey.String, CurrencyCode: account.CurrencyCode.String})
		}
	}
	return result
}

func (s *HomeService) Layout(ctx context.Context, ownerID int64) (HomeLayoutState, error) {
	accounts, err := s.layoutAccounts(ctx)
	if err != nil {
		return HomeLayoutState{}, err
	}
	return s.transactions.readHomeLayout(ctx, ownerID, accounts)
}

func (s *HomeService) layoutAccounts(ctx context.Context) ([]HomeLayoutAccount, error) {
	ctx, err := s.transactions.settings.referenceContext(ctx, s.transactions.now())
	if err != nil {
		return nil, err
	}
	day, err := pinnedOwnerDay(ctx)
	if err != nil {
		return nil, err
	}
	accounts, err := s.transactions.repository.LedgerAccountsAsOf(ctx, BookID, day)
	if err != nil {
		return nil, fmt.Errorf("read home layout accounts: %w", err)
	}
	return homeLayoutAccounts(accounts), nil
}

func (s *HomeService) SaveLayout(ctx context.Context, ownerID, expected int64, layout *HomeLayout) (HomeLayoutState, error) {
	if expected < 0 {
		return HomeLayoutState{}, ValidationError{Message: "invalid home layout revision"}
	}
	accounts, err := s.layoutAccounts(ctx)
	if err != nil {
		return HomeLayoutState{}, err
	}
	var next, defaults HomeLayout
	if layout == nil {
		current, err := s.transactions.readHomeLayout(ctx, ownerID, accounts)
		if err != nil {
			return HomeLayoutState{}, err
		}
		defaults = current.Defaults
		next = defaults
	} else {
		if err := validateHomeLayout(*layout); err != nil {
			return HomeLayoutState{}, err
		}
		defaults = homeLayoutDefaults(*layout, accounts)
		next = effectiveHomeLayout(*layout, defaults, accounts)
	}
	document, err := json.Marshal(next)
	if err != nil {
		return HomeLayoutState{}, fmt.Errorf("encode home layout: %w", err)
	}
	revision, err := s.transactions.repository.SaveHomeLayout(ctx, BookID, ownerID, expected, string(document))
	if errors.Is(err, db.ErrNotFound) {
		return HomeLayoutState{}, ErrHomeLayoutConflict
	}
	if err != nil {
		return HomeLayoutState{}, err
	}
	return HomeLayoutState{Revision: revision, Layout: next, Defaults: defaults, Accounts: accounts}, nil
}

func (s *TransactionService) readHomeLayout(ctx context.Context, ownerID int64, accounts []HomeLayoutAccount) (HomeLayoutState, error) {
	stored, err := s.repository.ReadHomeLayout(ctx, BookID, ownerID)
	if err != nil {
		return HomeLayoutState{}, err
	}
	defaults := DefaultHomeLayout()
	var layout HomeLayout
	if stored.Revision == 0 || json.Unmarshal([]byte(stored.Document), &layout) != nil || layout.Version != homeLayoutVersion {
		layout = defaults
	} else {
		defaults = homeLayoutDefaults(layout, accounts)
		layout = effectiveHomeLayout(layout, defaults, accounts)
	}
	return HomeLayoutState{Revision: stored.Revision, Layout: layout, Defaults: defaults, Accounts: accounts}, nil
}

// Validate the complete current wire shape. References that disappeared between
// opening Customize and saving it are normalized by the same rules as reads.
func validateHomeLayout(layout HomeLayout) error {
	if layout.Version != homeLayoutVersion || layout.Shortcuts == nil || layout.Widgets == nil || layout.WidgetOptions == nil {
		return ValidationError{Message: "a complete version 1 home layout is required"}
	}
	if !validHomeHeadline(layout.Headline) || layout.AccountID < 0 || layout.ShortcutAccountID < 0 ||
		(layout.AccountSet != nil && !validHomeAccountSet(layout.AccountSet)) {
		return ValidationError{Message: "invalid home headline"}
	}
	seen := map[string]bool{}
	if len(layout.Shortcuts) > len(homeLayoutShortcuts) {
		return ValidationError{Message: "too many home shortcuts"}
	}
	for _, shortcut := range layout.Shortcuts {
		if shortcut.ID == "" || !validHomeName(shortcut.ID) || seen[shortcut.ID] ||
			(slices.Contains(homeLayoutShortcuts, shortcut.ID) && !validHomeShortcut(shortcut)) {
			return ValidationError{Message: "invalid or duplicate home shortcut"}
		}
		seen[shortcut.ID] = true
	}
	if !seen["add"] {
		return ValidationError{Message: "new transaction must stay in home shortcuts"}
	}
	seen = map[string]bool{}
	if len(layout.Widgets) > len(homeLayoutWidgetIDs) || len(layout.WidgetOptions) > len(homeLayoutWidgetIDs) {
		return ValidationError{Message: "too many home widgets"}
	}
	for _, widget := range layout.Widgets {
		if widget == "" || !validHomeName(widget) || seen[widget] {
			return ValidationError{Message: "invalid or duplicate home widget"}
		}
		seen[widget] = true
	}
	for widget, options := range layout.WidgetOptions {
		if widget == "" || !validHomeName(widget) || (slices.Contains(homeLayoutWidgetIDs, widget) && !validHomeWidgetOptions(widget, options)) {
			return ValidationError{Message: "invalid home widget options"}
		}
	}
	return nil
}

func validHomeHeadline(headline string) bool {
	switch headline {
	case HomeHeadlineAuto, HomeHeadlineNetWorth, HomeHeadlineCash, HomeHeadlineAccount, HomeHeadlineAccountSet:
		return true
	}
	return false
}

func validHomeName(name string) bool {
	return name == strings.TrimSpace(name) && utf8.RuneCountInString(name) <= homeLayoutNameLimit &&
		!strings.ContainsFunc(name, unicode.IsControl)
}

func validHomeAccountSet(set *HomeLayoutAccountSet) bool {
	return set.Name != "" && validHomeName(set.Name) && len(set.AccountIDs) > 0 && validHomeIDs(set.AccountIDs)
}

func validHomeIDs(ids []int64) bool {
	if len(ids) > homeLayoutAccountLimit {
		return false
	}
	seen := map[int64]bool{}
	for _, id := range ids {
		if id <= 0 || seen[id] {
			return false
		}
		seen[id] = true
	}
	return true
}

func validHomeShortcut(shortcut HomeLayoutShortcut) bool {
	return slices.Contains(homeLayoutShortcuts, shortcut.ID) && validHomeName(shortcut.Label) &&
		((shortcut.ID == "account" && shortcut.AccountID > 0) || (shortcut.ID != "account" && shortcut.AccountID == 0))
}

func validHomeWidgetOptions(widget string, options HomeWidgetOptions) bool {
	if !slices.Contains(homeLayoutWidgetIDs, widget) {
		return false
	}
	if options.Size != "" && options.Size != "standard" && (options.Size != "wide" || !slices.Contains(homeWideWidgets, widget)) {
		return false
	}
	if options.Period != "" && (widget != "cashflow" ||
		(options.Period != "this_month" && options.Period != "last_month")) {
		return false
	}
	return len(options.AccountIDs) == 0 || (widget == "spending" && validHomeIDs(options.AccountIDs))
}

// When pins are available they can supply this choice. Until then the owner
// establishes the default register by adding an account shortcut.
func homeLayoutDefaults(layout HomeLayout, accounts []HomeLayoutAccount) HomeLayout {
	defaults := DefaultHomeLayout()
	id := layout.ShortcutAccountID
	for _, shortcut := range layout.Shortcuts {
		if shortcut.ID == "account" {
			id = shortcut.AccountID
			break
		}
	}
	if id != 0 && slices.ContainsFunc(accounts, func(account HomeLayoutAccount) bool { return account.ID == id }) {
		defaults.ShortcutAccountID = id
		defaults.Shortcuts = append(defaults.Shortcuts, HomeLayoutShortcut{ID: "account", AccountID: id})
	}
	return defaults
}

// Omit retired widgets and routes, preserve intentional removals and use the
// defaults when an old selection is no longer available. Never value part of
// a named account set under the original name.
func effectiveHomeLayout(layout, defaults HomeLayout, accounts []HomeLayoutAccount) HomeLayout {
	active := make(map[int64]bool, len(accounts))
	for _, account := range accounts {
		active[account.ID] = true
	}
	available := func(ids []int64) bool {
		for _, id := range ids {
			if !active[id] {
				return false
			}
		}
		return true
	}

	if !active[layout.AccountID] {
		layout.AccountID = 0
	}
	if set := layout.AccountSet; set != nil && (!validHomeAccountSet(set) || !available(set.AccountIDs)) {
		layout.AccountSet = nil
	}
	if !validHomeHeadline(layout.Headline) || (layout.Headline == HomeHeadlineAccount && layout.AccountID == 0) ||
		(layout.Headline == HomeHeadlineAccountSet && layout.AccountSet == nil) {
		layout.Headline = defaults.Headline
	}

	shortcuts := make([]HomeLayoutShortcut, 0, len(layout.Shortcuts))
	seen := map[string]bool{}
	for _, shortcut := range layout.Shortcuts {
		if validHomeShortcut(shortcut) && !seen[shortcut.ID] && (shortcut.ID != "account" || active[shortcut.AccountID]) {
			shortcuts = append(shortcuts, shortcut)
			seen[shortcut.ID] = true
		}
	}
	if len(shortcuts) == 0 {
		shortcuts = defaults.Shortcuts
	} else if !seen["add"] {
		shortcuts = append([]HomeLayoutShortcut{{ID: "add"}}, shortcuts...)
	}
	layout.Shortcuts = shortcuts
	layout.ShortcutAccountID = defaults.ShortcutAccountID

	widgets := make([]string, 0, len(layout.Widgets))
	for _, widget := range layout.Widgets {
		if slices.Contains(homeLayoutWidgetIDs, widget) && !slices.Contains(widgets, widget) {
			widgets = append(widgets, widget)
		}
	}
	if layout.Widgets == nil || (len(widgets) == 0 && len(layout.Widgets) > 0) {
		widgets = defaults.Widgets
	}
	layout.Widgets = widgets

	options := make(map[string]HomeWidgetOptions)
	for widget, option := range defaults.WidgetOptions {
		options[widget] = option
	}
	for widget, option := range layout.WidgetOptions {
		if validHomeWidgetOptions(widget, option) {
			if !available(option.AccountIDs) {
				option.AccountIDs = nil
			}
			options[widget] = option
		}
	}
	layout.WidgetOptions = options
	return layout
}
