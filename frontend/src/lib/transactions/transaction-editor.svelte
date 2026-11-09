<script lang="ts">
  import { useOwnerToday } from '#lib/dates/owner-date-context.ts';
  import { transactionTitle } from '#lib/transactions/transaction-title.ts';
  import { systemLabelText } from '#lib/transactions/system-label.ts';
  import { onDestroy, tick, untrack, type Snippet } from 'svelte';
  import { tagsQueryOptions } from '#lib/api/tags.ts';
  import Plus from '@lucide/svelte/icons/plus';
  import Save from '@lucide/svelte/icons/save';
  import Trash from '@lucide/svelte/icons/trash';
  import X from '@lucide/svelte/icons/x';
  import TriangleExclamationPoint from '@lucide/svelte/icons/triangle-exclamation-point';
  import ModalDialog from '#lib/components/modal-dialog.svelte';
  import { createQuery, useQueryClient } from '@tanstack/svelte-query';
  import APIFormError from '#lib/components/api-form-error.svelte';
  import { isSubledgerManagedAccountKind } from '#lib/accounts/subledger-accounts.ts';
  import { isOrdinaryEntryAccount } from './entry-account';
  import { suggestionKey } from './suggestion-keys.ts';
  import { editablePostings, hasPostingDay, postedOnForRequest } from './posted-on';
  import { simpleFormFits, simpleFormKeeps, splitEntryCarry, splitEntryRequest, type SplitEntryCarry, type SplitPosting } from './editable-entry';
  import {
    convertedAmounts,
    sameCurrencyAmount,
    sameCurrencyStatements,
    transferEntry,
    transferFeeCategoryKey,
    transferFields,
    transferProblem,
    type TransferFields,
    type TransferProblem,
    type TransferSideName
  } from './transfer';
  import {
    accountLedgerValue,
    approvalKey,
    approvalStillValid,
    cashAccountProblem,
    directionForTab,
    directionOf,
    entryTabLabel,
    entryTabs,
    investmentMuted,
    isCreditCard,
    rebindSplitAccount,
    shapeForTab,
    splitDirection,
    splitLegCommodities,
    splitLineProblem,
    startingTab,
    type CashDirection,
    type CashShape,
    type EntryDefaults,
    type EntryTab,
    type EntryTabLabel
  } from './entry-mode';
  import EntryTabs from './entry-tabs.svelte';
  import { EntryContext, provideEntryContext } from './entry-context.svelte.ts';
  import { beforeNavigate, goto } from '$app/navigation';
  import { editorHold } from '#lib/components/editor-hold.svelte.ts';
  import type { InvestmentSaveKind } from '#lib/transactions/add-transaction.svelte.ts';
  import InvestmentEntry from '#lib/investments/investment-entry.svelte';
  import { getLocale } from '#lib/paraglide/runtime.js';
  import { formatQuantity, formatRatio, joinCommodityAmount } from '#lib/money/format.ts';
  import { m } from '#lib/paraglide/messages.js';
  import { accountsQueryOptions } from '#lib/api/accounts.ts';
  import { categoriesQueryOptions } from '#lib/api/categories.ts';
  import { createPayee, payeesQueryKey, payeesQueryOptions, type PayeeResponse } from '#lib/api/payees.ts';
  import {
    needsPayeeConfirmation,
    payeeSuggestions,
    resolveTypedPayee,
    type PayeeResolution
  } from './payee-matching';
  import { currenciesQueryOptions, type CurrencyResponse } from '#lib/api/currencies.ts';
  import {
    createTransaction,
    updateTransaction,
    correctTransaction,
    reconciliationImpactForCreate,
    reconciliationImpactForUpdate,
    getTransaction,
    isReconciliationOverrideRequired,
    isReconciliationImpactStale,
    isTransactionVersionStale,
    reviewedOverride,
    type TransactionRequest,
    type TransactionResponse,
    type ReconciliationImpactResponse
  } from '#lib/api/transactions.ts';
  import type { AccountResponse } from '#lib/api/accounts.ts';
  import type { CategoryResponse } from '#lib/api/categories.ts';
  import type { components } from '#lib/api/schema.js';

  type JournalEntryResponse = components['schemas']['JournalEntryResponse'];
  type PostingResponse = components['schemas']['PostingResponse'];
  type JournalEntryPostingRequest = NonNullable<
    NonNullable<TransactionRequest['journal_entries']>[number]['postings']
  >[number];
  import { accountDisplayName } from '#lib/accounts/account-labels.ts';
  import { systemRoleLabel } from './transaction-labels';
  import { categoryDisplayName } from '#lib/categories/category-labels.ts';
  import {
    commodityImbalance,
    formatEditableAmount,
    negateCoefficient,
    parseDecimalAmount
  } from '#lib/money/amount.ts';

  const ownerToday = useOwnerToday();

  // ── Tier types ────────────────────────────────────────────────────
  // `locked`: a journal no editor restates exactly; only what it describes
  // is edited, and the save keeps the stored journal (#481).
  type Tier = 'simple' | 'split' | 'transfer' | 'locked';

  // A single editable posting leg in the split editor.
  type SplitLeg = {
    accountID: string;       // '' means unset
    commodityID: string;     // '' means unset
    amountStr: string;       // user-entered, e.g. "25.00" or "-25.00"
    memo: string;
    lineKey?: string;
    /** The day this account posting posts on, '' when it follows the
     *  transaction date (#187). */
    postedOn: string;
    /** Carried, never shown (#481): the day a category was restated on and
     *  its tags. */
    followsDay?: string;
    tagIDs?: number[];
  };

  // ── Props ─────────────────────────────────────────────────────────
  type EditorMode = 'create' | 'edit' | 'correct' | 'template';

  let {
    mode,
    transaction: transactionProp,
    csrfToken,
    onSaved,
    onCancel,
    initialValues,
    createDefaults,
    templateFields,
    saveTemplate,
    onPendingChange,
    onInvestmentSaved,
    preservePostings = false
  }: {
    mode: EditorMode;
    transaction?: TransactionResponse;
    csrfToken?: string;
    onSaved?: (saved: TransactionResponse) => Promise<void> | void;
    initialValues?: TransactionRequest;
    /** What a new transaction starts from: the account or category of the
     *  screen it is entered from (#185), the tab it opens on (Transfer puts
     *  the account on From, #481) and the Investment tab's prefill (#506). */
    createDefaults?: EntryDefaults;
    templateFields?: Snippet;
    saveTemplate?: (payload: TransactionRequest) => Promise<void>;
    preservePostings?: boolean;
    onPendingChange?: (pending: boolean) => void;
    /** An investment form opened from the Investment tab saved (#506). */
    onInvestmentSaved?: (kind?: InvestmentSaveKind) => void;
    onCancel: () => void;
  } = $props();

  // A save refused because the transaction changed elsewhere reloads it here,
  // so the form shows the newer version instead of re-sending a stale one
  // (#296). A newer version handed down by the parent wins.
  let reloaded = $state.raw<TransactionResponse | undefined>(undefined);
  const transaction = $derived(
    reloaded && transactionProp && reloaded.id === transactionProp.id && reloaded.version_seq > transactionProp.version_seq
      ? reloaded
      : transactionProp
  );
  /** Shown once the form has been reloaded with a newer version. */
  let reloadedNotice = $state(false);

  let tagIDs = $state<number[]>([]);
  const tagsQuery = createQuery(() => tagsQueryOptions());

  // ── Queries ───────────────────────────────────────────────────────
  // Load accounts (asset/liability for the account leg; income/expense categories loaded separately)
  const accountsQuery = createQuery(() => accountsQueryOptions(false, mode === 'template' || preservePostings));
  const categoriesQuery = createQuery(() => categoriesQueryOptions());
  const currenciesQuery = createQuery(() => currenciesQueryOptions());

  // Payee autocomplete. The whole payee list is loaded once and ranked here
  // rather than re-queried per keystroke: a personal book's payees are a small
  // set, and local fuzzy ranking finds the typo that a server-side LIKE cannot
  // — which is the case this control exists for (T-50).
  let payeeSearch = $state('');
  let payeeDropdownOpen = $state(false);
  /** The name the editor opened with, so an untouched payee never blocks a save. */
  let initialPayeeName = $state('');
  let payeeConfirm = $state<{ open: boolean; resolution: PayeeResolution | null }>({
    open: false,
    resolution: null
  });
  let payeeCreatePending = $state(false);
  const payeeInputID = 'transaction-payee-name';
  const queryClient = useQueryClient();
  let categoryDropdownOpen = $state(false);
  let categorySearch = $state('');
  // Suggestion lists are comboboxes: the active option, -1 for none.
  const uid = $props.id();
  let payeeActive = $state(-1);
  let categoryActive = $state(-1);

  function onPayeeInput(e: Event) {
    const val = (e.target as HTMLInputElement).value;
    payeeSearch = val;
    payeeID = undefined; // clear resolved ID when user edits
    payeeDropdownOpen = val.length > 0;
    payeeActive = -1;
  }

  function choosePayee(payee: PayeeResponse) {
    payeeID = payee.id;
    payeeSearch = payee.name;
    payeeDropdownOpen = false;
    payeeActive = -1;
  }

  function payeeKeydown(event: KeyboardEvent) {
    if (event.isComposing) return;
    const result = suggestionKey(event.key, payeeDropdownOpen, payeeMatches.length, payeeActive);
    if (result.kind === 'none') return;
    event.preventDefault();
    if (result.kind === 'move') {
      payeeDropdownOpen = true;
      payeeActive = result.active;
      void tick().then(() => document.getElementById(`${uid}-payee-${payeeActive}`)?.scrollIntoView({ block: 'nearest' }));
    }
    else if (result.kind === 'choose') choosePayee(payeeMatches[result.index]);
    else { payeeDropdownOpen = false; payeeActive = -1; }
  }

  async function categoryKeydown(event: KeyboardEvent) {
    if (event.isComposing) return;
    const result = suggestionKey(event.key, categoryDropdownOpen, filteredCategories.length, categoryActive);
    if (result.kind === 'none') return;
    event.preventDefault();
    if (result.kind === 'move') {
      categoryDropdownOpen = true;
      categoryActive = result.active;
      await tick();
      document.getElementById(`${uid}-category-${categoryActive}`)?.scrollIntoView({ block: 'nearest' });
    }
    else if (result.kind === 'choose') {
      // The field becomes the chosen label and its clear button; keep focus there.
      selectCategory(filteredCategories[result.index]);
      await tick();
      document.getElementById(`${uid}-category-clear`)?.focus();
    } else { categoryDropdownOpen = false; categoryActive = -1; }
  }

  const payeesQuery = createQuery(() => payeesQueryOptions({}));
  const allPayees = $derived<PayeeResponse[]>(payeesQuery.data?.payees ?? []);
  const payeeMatches = $derived(payeeSuggestions(payeeSearch, allPayees));
  const payeeNameChanged = $derived(payeeSearch.trim() !== initialPayeeName.trim());

  // ── Field state ───────────────────────────────────────────────────
  let tier = $state<Tier>('simple');

  // New transaction's tabs (#506). The editor stays mounted on every tab, so
  // its fields are the entry's shared context and working copies: the date,
  // payee, memo, account and currency are one value each, Transfer keeps its
  // own fields, and the cash tabs share one amount and their shape.
  const tabbed = $derived(mode === 'create');
  let entryTab = $state<EntryTab>('out');
  /** The cash tabs' shape, kept while Transfer or Investment is shown. */
  let cashShape = $state<CashShape>('simple');
  /** Money out of or into the account; the amount is always positive. */
  let direction = $state<CashDirection>('out');
  // Why the simple form cannot be saved yet, translated; '' when it can.
  let entryMessage = $state('');

  // Tier 1 / shared fields
  let transactionDate = $state(ownerToday());
  let payeeID = $state<number | undefined>(undefined);
  let description = $state('');

  // Tier 1: simple two-posting form
  let categoryID = $state<number | undefined>(undefined);
  let categoryLabel = $state('');
  // The positive amount; `direction` says which way it moves (#506).
  let amountStr = $state('');
  // The entry's account: the simple form's account and a transfer's From.
  let accountID = $state<number | undefined>(undefined);
  // The day the account posting posts on, '' when it follows the date (#187).
  let accountPostedOn = $state('');
  let showAccountPostedOn = $state(false);
  // Split legs whose posted-on field the user opened (by leg index).
  let splitPostedOnOpen = $state<Record<number, boolean>>({});
  // Manually chosen commodity when the selected account has no
  // default_commodity_id; a transfer's From currency too.
  let simpleCommodityID = $state<number | undefined>(undefined);

  // Tier 2 fields (under Advanced)
  let transactionKind = $state<string>('ordinary');
  let noteMarkdown = $state('');
  let externalRefHint = $state('');

  // New transaction's shared context (#508): the date, description and
  // reference above are the entry's main date, memo and reference for every
  // tab and investment activity, and the activities' working copies live here
  // for as long as the entry is open.
  const entry = new EntryContext({
    date: () => transactionDate,
    setDate: (value) => { transactionDate = value; },
    memo: () => description,
    setMemo: (value) => { description = value; },
    reference: () => externalRefHint,
    setReference: (value) => { externalRefHint = value; }
  });
  provideEntryContext(entry);

  // What the split editor carries for the entry and never shows (#481).
  let splitCarry = $state<SplitEntryCarry>({});
  // The simple form's two lines, kept so an edit keeps their identity.
  let simpleLineKeys = $state<{ account?: string; category?: string }>({});

  // Tier 3: split legs
  let splitLegs = $state<SplitLeg[]>([
    { accountID: '', commodityID: '', amountStr: '', memo: '', postedOn: '' },
    { accountID: '', commodityID: '', amountStr: '', memo: '', postedOn: '' }
  ]);

  // Transfer between two of the owner's accounts (#481). In one currency the
  // amount transferred; in two, what left From and what reached To as their
  // statements show them. A fee on top, on Bank fees, posts with its payer.
  // From is the entry's account (`accountID`, `simpleCommodityID`).
  let transferToID = $state<number | undefined>(undefined);
  // Chosen currency for a To account that has no default commodity.
  let transferToCommodityID = $state<number | undefined>(undefined);
  let sentStr = $state('');
  let receivedStr = $state('');
  let feeStr = $state('');
  let feePaidBy = $state<TransferSideName>('sending');
  let conversionSide = $state<TransferSideName>('sending');
  // Memos the form does not show, kept so a save leaves them as they were.
  let transferMemos = $state<{ from?: string; to?: string; fee?: string; entry?: string }>({});
  let feeAccountID = $state<number | undefined>(undefined);
  let leavesOn = $state('');
  let arrivesOn = $state('');
  let showTransferDays = $state(false);
  let transferLineKeys = $state<{ from?: string; to?: string; fee?: string }>({});
  // The saved entries' kind, sent back so an edit keeps it.
  let transferEntryKind = $state<TransferFields['entryKind']>(undefined);
  // Why the transfer cannot be saved yet, translated; '' when it can.
  let transferMessage = $state('');

  // ── Form state ────────────────────────────────────────────────────
  let pending = $state(false);
  // The whole save chain is busy, the payee it creates first included: a
  // dialog or panel never dismisses (and so discards) the entry while a write
  // that leads to posting it is in flight.
  const busy = $derived(pending || payeeCreatePending);
  $effect(() => { onPendingChange?.(busy); });
  // A dismissed editor never continues to a save of its own.
  let dismissed = false;
  onDestroy(() => { dismissed = true; });
  let formError = $state<unknown>(undefined);

  // Reconciliation override modal
  // The override authorizes only the request it reviewed (`reviewedKey`):
  // a later change to the entry makes it stale (#506).
  let reconciliationModal = $state<{
    open: boolean;
    impacts: ReconciliationImpactResponse['affected_checkpoints'];
    pendingPayload: TransactionRequest;
    reviewedKey?: string;
  }>({ open: false, impacts: [], pendingPayload: {} as TransactionRequest });

  // ── Derived ───────────────────────────────────────────────────────
  const assetLiabilityAccounts = $derived(
    (accountsQuery.data?.accounts ?? []).filter(
      (a: AccountResponse) => isOrdinaryEntryAccount(a)
    )
  );

  const incomeExpenseCategories = $derived(
    (categoriesQuery.data?.categories ?? []).filter(
      (c: CategoryResponse) => c.status === 'active' && c.allows_postings
    )
  );

  const filteredCategories = $derived.by(() => {
    const q = categorySearch.trim().toLowerCase();
    if (!q) return incomeExpenseCategories.slice(0, 40);
    return incomeExpenseCategories
      .filter((c: CategoryResponse) => {
        const label = categoryDisplayName(c);
        return label.toLowerCase().includes(q);
      })
      .slice(0, 40);
  });

  // A pre-selected category gets its label once the categories load, without
  // re-running the form initialisation (which would clear typed input).
  $effect(() => {
    if (categoryID === undefined || categoryLabel !== '') return;
    const category = (categoriesQuery.data?.categories ?? []).find((c: CategoryResponse) => c.id === categoryID);
    if (category) categoryLabel = categoryDisplayName(category);
  });

  const selectedAccount = $derived(
    accountID !== undefined
      ? assetLiabilityAccounts.find((a: AccountResponse) => a.id === accountID)
      : undefined
  );

  // Infer commodity from the selected account's default_commodity_id.
  const inferredCommodityID = $derived(selectedAccount?.default_commodity_id);

  // Effective commodity for the simple form: inferred from account, or manually chosen.
  const effectiveCommodityID = $derived(inferredCommodityID ?? simpleCommodityID);

  // The entry's account as the book has it, a holding included. A starting
  // account the cash tabs and Transfer cannot post to stays selected and
  // explained until the owner chooses another; nothing is substituted (#506).
  const contextAccount = $derived(
    accountID === undefined ? undefined : (accountsQuery.data?.accounts ?? []).find((a: AccountResponse) => a.id === accountID)
  );
  const accountProblem = $derived(selectedAccount ? undefined : cashAccountProblem(contextAccount));
  const accountProblemText = $derived(
    !accountProblem || !contextAccount
      ? ''
      : accountProblem === 'holding'
        ? m.transactions_entry_account_holding({ account: accountDisplayName(contextAccount) })
        : m.transactions_entry_account_unavailable({ account: accountDisplayName(contextAccount) })
  );

  const tabLabelText: Record<EntryTabLabel, () => string> = {
    withdrawal: m.transactions_entry_tab_withdrawal,
    deposit: m.transactions_entry_tab_deposit,
    charge: m.transactions_entry_tab_charge,
    credit: m.transactions_entry_tab_credit,
    transfer: m.transactions_entry_tab_transfer,
    investment: m.transactions_entry_tab_investment
  };
  // Labels follow the account; positions never move.
  const entryTabList = $derived(
    entryTabs.map((tab) => ({
      id: tab,
      label: tabLabelText[entryTabLabel(tab, contextAccount)](),
      muted: tab === 'investment' && investmentMuted(contextAccount)
    }))
  );
  const directionLabel = (value: CashDirection) => tabLabelText[entryTabLabel(value, contextAccount)]();

  const currenciesByID = $derived(
    new Map<number, CurrencyResponse>(
      (currenciesQuery.data?.currencies ?? []).map((c: CurrencyResponse) => [c.id, c])
    )
  );

  const transferFromAccount = $derived(selectedAccount);
  const transferToAccount = $derived(assetLiabilityAccounts.find((a: AccountResponse) => a.id === transferToID));
  const transferFromCommodity = $derived(effectiveCommodityID);
  const transferToCommodity = $derived(transferToAccount?.default_commodity_id ?? transferToCommodityID);
  const transferIsExchange = $derived(
    transferFromCommodity !== undefined && transferToCommodity !== undefined && transferFromCommodity !== transferToCommodity
  );
  const bankFeesCategoryID = $derived(
    (categoriesQuery.data?.categories ?? []).find((c: CategoryResponse) => c.builtin_key === transferFeeCategoryKey && c.status === 'active')?.id
  );
  // Display only: what converted (the statement amounts with the fee taken
  // back out of its payer) and the rate those imply, never used to fill one in.
  const transferConversion = $derived.by(() => {
    if (!transferIsExchange) return null;
    const sent = parseDecimalAmount(sentStr);
    const received = parseDecimalAmount(receivedStr);
    if (!sent || !received) return null;
    const fee = parsedFee();
    const converted = convertedAmounts({ sent, received, fee: fee ? { amount: fee, paidBy: feePaidBy } : undefined });
    if (!converted) return null;
    const locale = getLocale();
    return {
      sent: joinCommodityAmount(commodityCode(transferFromCommodity), formatQuantity(converted.sent.value, converted.sent.scale, locale)),
      received: joinCommodityAmount(commodityCode(transferToCommodity), formatQuantity(converted.received.value, converted.received.scale, locale)),
      rate: formatRatio(converted.received, converted.sent, 6, locale)
    };
  });
  const commodityCode = (id: number | undefined) => (id === undefined ? '' : currenciesByID.get(id)?.code ?? '');

  // Each split line's currency: its own, its account's, or the one the
  // split's account lines share; '' when the owner must choose it (#511).
  const splitCommodities = $derived(splitLegCommodities(splitLegs, commodityForAccount, (id) => hasPostingDay(accountForLeg(id))));
  const resolvedSplitLegs = $derived(splitLegs.map((leg, i) => ({ ...leg, commodityID: splitCommodities[i] ?? '' })));

  // For Tier 3 imbalance check — summed per commodity, scale-aware, in #lib/money.
  const splitImbalance = $derived.by(() => commodityImbalance(resolvedSplitLegs));
  // The first filled line the save cannot take whole, with the owner's words for it.
  const splitProblem = $derived(splitLineProblem(resolvedSplitLegs));
  function splitProblemMessage(problem: NonNullable<typeof splitProblem>): string {
    const line = problem.line;
    const account = accountForLeg(splitLegs[line - 1]?.accountID ?? '');
    switch (problem.missing) {
      case 'account':
        return m.transactions_split_line_needs_account({ line });
      case 'amount':
        return m.transactions_split_line_needs_amount({ line });
      case 'number':
        return m.transactions_split_line_needs_number({ line });
      case 'currency':
        return m.transactions_split_line_needs_currency({ line, account: account ? postingAccountLabel(account) : '' });
    }
  }

  const title = $derived(
    mode === 'template' ? m.recurring_entry_title() : mode === 'correct'
      ? m.transactions_form_correction_title()
      : mode === 'edit'
        ? m.transactions_form_edit_title()
        : m.transactions_form_create_title()
  );

  const submitLabel = $derived(
    mode === 'template' ? (pending ? m.recurring_saving() : m.recurring_save_template()) : pending
      ? mode === 'edit'
        ? m.transactions_form_save_pending()
        : m.transactions_form_create_pending()
      : mode === 'edit'
        ? m.transactions_form_save()
        : m.transactions_form_create()
  );

  const isVoided = $derived(transaction?.status === 'voided');

  // A locked journal is shown as it is stored, system postings included.
  const lockedPostings = $derived(
    tier === 'locked'
      ? (transaction?.journal_entries ?? []).flatMap((entry) =>
          entry.postings.map((posting) => ({
            key: `${entry.entry_seq}-${posting.line_key}`,
            date: entry.entry_date,
            label: posting.account_system_role ? systemRoleLabel(posting.account_system_role) : (posting.account_name ?? String(posting.account_id)),
            amount: joinCommodityAmount(posting.commodity_code ?? '', formatQuantity(posting.quantity_value, posting.quantity_scale, getLocale()))
          }))
        )
      : []
  );

  // ── Initialise from existing transaction (edit mode) ──────────────
  $effect(() => {
    tagIDs = [...(transaction?.tag_ids ?? initialValues?.tag_ids ?? [])];
    splitPostedOnOpen = {};
    splitCarry = {};
    simpleLineKeys = {};
    if (mode === 'template' && initialValues) {
      transactionDate = initialValues.transaction_date ?? untrack(ownerToday);
      payeeID = initialValues.payee_id ?? undefined;
      payeeSearch = initialValues.payee_name ?? '';
      initialPayeeName = initialValues.payee_name ?? '';
      description = initialValues.description ?? '';
      transactionKind = initialValues.transaction_kind ?? 'ordinary';
      noteMarkdown = initialValues.note_markdown ?? '';
      const postings = initialValues.journal_entries?.flatMap((e: NonNullable<TransactionRequest['journal_entries']>[number]) => e.postings ?? []) ?? [];
      tier = postings.length ? 'split' : 'simple';
      direction = 'out';
      if (postings.length) splitLegs = postings.map((p: JournalEntryPostingRequest) => ({
        accountID: String(p.account_id), commodityID: String(p.commodity_id),
        amountStr: formatEditableAmount(p.quantity_value ?? '0', p.quantity_scale ?? 0),
        memo: p.memo ?? '', lineKey: p.line_key, postedOn: ''
      }));
      return;
    }
    if (!transaction) {
      // A shared session refresh must preserve this unsaved entry.
      transactionDate = untrack(ownerToday);
      payeeID = undefined;
      payeeSearch = '';
      initialPayeeName = '';
      description = '';
      // Read once: a later filter change must not clear a form in progress.
      categoryID = untrack(() => createDefaults?.categoryID);
      categoryLabel = '';
      amountStr = '';
      accountID = untrack(() => createDefaults?.accountID);
      entryMessage = '';
      accountPostedOn = '';
      showAccountPostedOn = false;
      simpleCommodityID = undefined;
      transactionKind = 'ordinary';
      noteMarkdown = '';
      externalRefHint = '';
      resetTransfer();
      // Locals, not read back from `tier` or `entryTab`: the effect must not
      // track them, or switching tabs would reset the form. The starting
      // account is the transfer's From as well (#481, #506).
      const start = startingTab({
        tab: untrack(() => createDefaults?.tab),
        account: untrack(() => accountsQuery.data?.accounts.find((a: AccountResponse) => a.id === createDefaults?.accountID))
      });
      entryTab = start;
      direction = directionForTab(start) ?? 'out';
      cashShape = 'simple';
      tier = start === 'transfer' ? 'transfer' : 'simple';
      splitLegs = [
        { accountID: '', commodityID: '', amountStr: '', memo: '', postedOn: '' },
        { accountID: '', commodityID: '', amountStr: '', memo: '', postedOn: '' }
      ];
      formError = undefined;
      workBaseline = untrack(entryWork);
      entry.copies.sharedBaseline = untrack(() => ({
        holdingAccountID: createDefaults?.investment?.holdingAccountID,
        commodityID: createDefaults?.investment?.commodityID
      }));
      return;
    }

    transactionDate = transaction.transaction_date;
    payeeID = transaction.payee_id;
    payeeSearch = transaction.payee_name ?? '';
    initialPayeeName = transaction.payee_name ?? '';
    description = transaction.description;
    transactionKind = transaction.transaction_kind ?? 'ordinary';
    noteMarkdown = transaction.note_markdown ?? '';
    externalRefHint = transaction.external_ref_hint ?? '';
    formError = undefined;
    entryMessage = '';
    direction = 'out';

    // Determine tier based on journal entry complexity. A journal posted on
    // several days loses its transfer_clearing bridge here; the server
    // rebuilds it from each leg's posted date (#187).
    const entries: JournalEntryResponse[] = transaction.journal_entries ?? [];
    const loaded = editablePostings<PostingResponse>(entries, transaction.transaction_date);
    const shown = preservePostings ? loaded : loaded.filter(({ posting }) => !posting.account_system_role);
    const assetLegs = shown.filter(({ posting }) => hasPostingDay(posting));
    const categoryLegs = shown.filter(
      ({ posting }) => posting.account_class === 'income' || posting.account_class === 'expense'
    );

    // A transfer between two accounts, in one currency or two, with or
    // without a fee, opens in Transfer mode (#481).
    const transfer = preservePostings ? null : transferFields(transaction);
    resetTransfer();
    if (transfer) {
      tier = 'transfer';
      accountID = transfer.from.accountID;
      transferToID = transfer.to.accountID;
      simpleCommodityID = transfer.from.commodityID;
      transferToCommodityID = transfer.to.commodityID;
      const twoCurrencies = transfer.from.commodityID !== transfer.to.commodityID;
      const amount = twoCurrencies ? transfer.sent : sameCurrencyAmount(transfer);
      sentStr = formatEditableAmount(amount.value, amount.scale);
      receivedStr = twoCurrencies ? formatEditableAmount(transfer.received.value, transfer.received.scale) : '';
      leavesOn = transfer.from.postedOn;
      arrivesOn = transfer.to.postedOn;
      showTransferDays = transfer.from.postedOn !== '' || transfer.to.postedOn !== '' || transfer.conversionSide === 'receiving';
      conversionSide = transfer.conversionSide ?? 'sending';
      transferLineKeys = { from: transfer.from.lineKey, to: transfer.to.lineKey, fee: transfer.fee?.lineKey };
      transferMemos = { from: transfer.from.memo, to: transfer.to.memo, fee: transfer.fee?.memo, entry: transfer.entryMemo };
      transferEntryKind = transfer.entryKind;
      if (transfer.fee) {
        feeStr = formatEditableAmount(transfer.fee.amount.value, transfer.fee.amount.scale);
        feePaidBy = transfer.fee.paidBy;
        feeAccountID = transfer.fee.accountID;
      }
      return;
    }

    // Everything else is edited as the server restates it; a journal it
    // cannot restate exactly is locked to what it describes (#481).
    const editable = preservePostings ? undefined : transaction.editable_entry;
    if (!preservePostings && !editable) {
      tier = 'locked';
      return;
    }
    if (editable) splitCarry = splitEntryCarry(editable);
    const accountIDs = new Set(loaded.filter(({ posting }) => hasPostingDay(posting)).map(({ posting }) => posting.account_id));

    // The simple form is one account and one category, with nothing it would
    // drop; anything else opens as splits.
    if (editable && simpleFormKeeps(editable, (id) => accountIDs.has(id)) && shown.length <= 2 && assetLegs.length <= 1 && categoryLegs.length <= 1 && assetLegs.length + categoryLegs.length === shown.length) {
      tier = 'simple';
      const assetLeg = assetLegs[0];
      const catLeg = categoryLegs[0]?.posting;
      simpleLineKeys = { account: assetLeg?.posting.line_key, category: catLeg?.line_key };
      if (assetLeg) {
        accountID = assetLeg.posting.account_id;
        simpleCommodityID = assetLeg.posting.commodity_id;
        // A positive amount and the way it moves the account (#506).
        const shown = directionOf(assetLeg.posting.quantity_value);
        direction = shown.direction;
        amountStr = formatEditableAmount(shown.magnitude, assetLeg.posting.quantity_scale);
        accountPostedOn = assetLeg.postedOn;
        showAccountPostedOn = assetLeg.postedOn !== '';
      }
      if (catLeg) {
        categoryID = catLeg.account_id;
        // Untracked: a categories refetch must not reset an edit in progress.
        // Until categories load the label stays empty for the label effect
        // above to fill.
        const allCats = untrack(() => categoriesQuery.data?.categories);
        const cat = allCats?.find((c: CategoryResponse) => c.id === catLeg.account_id);
        categoryLabel = cat ? categoryDisplayName(cat) : allCats ? String(catLeg.account_id) : '';
      }
    } else if (editable) {
      tier = 'split';
      splitLegs = editable.postings.map((p) => {
        const isAccount = accountIDs.has(p.account_id);
        return {
          accountID: String(p.account_id),
          commodityID: String(p.commodity_id),
          amountStr: formatEditableAmount(p.quantity_value, p.quantity_scale),
          memo: p.memo,
          lineKey: p.line_key,
          postedOn: isAccount ? (p.posted_on ?? '') : '',
          followsDay: isAccount ? undefined : p.posted_on,
          tagIDs: p.tag_ids
        };
      });
    } else {
      tier = 'split';
      splitLegs = shown.map(({ posting: p, postedOn }) => ({
        accountID: String(p.account_id),
        commodityID: String(p.commodity_id),
        amountStr: formatEditableAmount(p.quantity_value, p.quantity_scale),
        memo: p.memo ?? '',
        lineKey: p.line_key,
        postedOn
      }));
    }
  });

  // ── CSS helpers ───────────────────────────────────────────────────
  const inputClass =
    'mt-1.5 h-10 w-full rounded-[var(--radius-control)] border border-border bg-control px-3 text-sm text-foreground shadow-sm outline-none transition placeholder:text-muted hover:bg-control-hover focus:border-accent disabled:cursor-not-allowed disabled:opacity-70';
  const textAreaClass =
    'mt-1.5 min-h-20 w-full rounded-[var(--radius-control)] border border-border bg-control px-3 py-2 text-sm text-foreground shadow-sm outline-none transition placeholder:text-muted hover:bg-control-hover focus:border-accent';
  const labelClass = 'block text-xs font-semibold uppercase tracking-[0.12em] text-muted';

  // ── Submit ────────────────────────────────────────────────────────
  async function handleSubmit(event: SubmitEvent) {
    event.preventDefault();

    if (!csrfToken) {
      formError = new Error(m.transactions_form_missing_session());
      return;
    }

    if (isVoided) {
      return; // blocked by UI; should not happen
    }

    transferMessage = '';
    entryMessage = '';
    // An account that cannot take this posting stays selected, explained
    // beside its picker; the save waits for another choice (#506).
    if ((tier === 'simple' || tier === 'transfer') && accountProblem) {
      if (tier === 'transfer') transferMessage = accountProblemText;
      else entryMessage = accountProblemText;
      return;
    }
    if (tier === 'simple' && simpleAmount() === 'negative') {
      entryMessage = m.transactions_entry_amount_positive();
      return;
    }
    // A filled split line saves whole or the save stops on it (#511).
    if (tier === 'split' && splitProblem) {
      entryMessage = splitProblemMessage(splitProblem);
      return;
    }
    // A split's amounts are signed: under a cash tab they must move the
    // entry's account the way the tab says, or the tab would misname what
    // is saved (#506 review).
    if (tabbed && tier === 'split' && (entryTab === 'out' || entryTab === 'in')) {
      const moves = currentSplitDirection();
      if (moves && moves !== entryTab && contextAccount) {
        entryMessage = m.transactions_entry_split_direction({ account: accountDisplayName(contextAccount), tab: directionLabel(moves) });
        return;
      }
    }
    if (tier === 'transfer') {
      const problem = currentTransferProblem();
      if (problem) {
        transferMessage = problem === 'no_fee_category' ? m.transactions_transfer_problem_no_fee_category() : transferProblemMessage(problem);
        return;
      }
    }

    // A newly typed name that names no payee stops here rather than being saved
    // as free text. Free text is what collapsed every such transaction into one
    // "no payee recorded" row in the spending report (T-50). An untouched name
    // is left alone, so editing an unrelated field on older data is never
    // blocked by history the user did not create.
    const resolution = resolveTypedPayee(payeeSearch, payeeID, allPayees);
    if (needsPayeeConfirmation(resolution, payeeNameChanged)) {
      payeeConfirm = { open: true, resolution };
      return;
    }

    await savePayee();
  }

  /** The save itself, reached directly or once the payee question is settled. */
  async function savePayee() {
    pending = true;
    formError = undefined;
    reloadedNotice = false;

    try {
      const payload = buildPayload();
      await submitPayload(payload);
    } catch (error) {
      if (isReconciliationOverrideRequired(error)) {
        // Fetch impact details and open modal.
        try {
          const payload = buildPayload();
          const impact = await fetchImpact(payload);
          reconciliationModal = { open: true, impacts: impact.affected_checkpoints, pendingPayload: payload, reviewedKey: approvalKey(payload) };
        } catch (impactErr) {
          if (isTransactionVersionStale(impactErr)) await reloadNewerVersion();
          else formError = impactErr;
        }
      } else if (isTransactionVersionStale(error)) {
        await reloadNewerVersion();
      } else {
        formError = error;
      }
    } finally {
      pending = false;
    }
  }

  /** Links the transaction to an existing payee the user recognised. */
  async function usePayeeSuggestion(payee: PayeeResponse) {
    payeeID = payee.id;
    payeeSearch = payee.name;
    payeeConfirm = { open: false, resolution: null };
    await savePayee();
  }

  /** Creates the payee the user typed, then saves — the confirmed act. */
  async function createTypedPayee(name: string) {
    if (!csrfToken) {
      formError = new Error(m.transactions_form_missing_session());
      return;
    }
    payeeCreatePending = true;
    formError = undefined;
    try {
      const created = await createPayee({ name }, csrfToken);
      await queryClient.invalidateQueries({ queryKey: payeesQueryKey });
      payeeID = created.id;
      payeeSearch = created.name;
      payeeConfirm = { open: false, resolution: null };
    } catch (error) {
      formError = error;
      return;
    } finally {
      payeeCreatePending = false;
    }
    if (dismissed) return;
    await savePayee();
  }

  function editPayeeName() {
    payeeConfirm = { open: false, resolution: null };
    payeeDropdownOpen = false;
    document.getElementById(payeeInputID)?.focus();
  }

  async function confirmReconciliationOverride() {
    if (!csrfToken) return;
    // The override acknowledges the request it reviewed and nothing else: an
    // entry changed since (tab, account, direction, amount) needs a new save
    // and a new review (#506).
    if (!approvalStillValid(reconciliationModal.reviewedKey, buildPayload())) {
      reconciliationModal = { open: false, impacts: [], pendingPayload: {} as TransactionRequest };
      formError = new Error(m.transactions_entry_review_stale());
      return;
    }
    reconciliationModal = { ...reconciliationModal, open: false };
    pending = true;
    formError = undefined;
    try {
      // The override acknowledges exactly the checkpoints this modal showed;
      // the write refuses a set that changed meanwhile (#296).
      await submitPayload(reconciliationModal.pendingPayload, { affected_checkpoints: reconciliationModal.impacts });
    } catch (error) {
      if (isTransactionVersionStale(error)) {
        await reloadNewerVersion();
      } else if (isReconciliationImpactStale(error)) {
        // Show the set as it is now; the owner confirms again or cancels.
        formError = error;
        try {
          const impact = await fetchImpact(reconciliationModal.pendingPayload);
          reconciliationModal = { ...reconciliationModal, open: true, impacts: impact.affected_checkpoints };
        } catch (impactErr) {
          if (isTransactionVersionStale(impactErr)) await reloadNewerVersion();
          else formError = impactErr;
        }
      } else {
        formError = error;
      }
    } finally {
      pending = false;
    }
  }

  /** Loads the version that refused this save into the form (#296). */
  async function reloadNewerVersion() {
    if (!transaction) return;
    try {
      reloaded = await getTransaction(transaction.id);
      reloadedNotice = true;
    } catch (error) {
      formError = error;
    }
  }

  /** reviewed is the previewed impact an override acknowledges; absent, no override. */
  async function submitPayload(payload: TransactionRequest, reviewed?: ReconciliationImpactResponse) {
    if (mode === 'template') {
      if (!saveTemplate) throw new Error(m.recurring_unavailable());
      await saveTemplate(payload);
      return;
    }
    const finalPayload = reviewed ? { ...payload, ...reviewedOverride(reviewed) } : payload;

    let saved: TransactionResponse;

    if (mode === 'correct' && transaction) {
      saved = await correctTransaction(transaction.id, finalPayload, csrfToken!);
    } else if (mode === 'edit' && transaction) {
      saved = await updateTransaction(transaction.id, finalPayload, csrfToken!);
    } else {
      saved = await createTransaction(finalPayload, csrfToken!);
    }

    leaving = true;
    await onSaved?.(saved);
  }

  async function fetchImpact(payload: TransactionRequest): Promise<ReconciliationImpactResponse> {
    if (mode === 'edit' && transaction) {
      return reconciliationImpactForUpdate(transaction.id, payload);
    }
    return reconciliationImpactForCreate(payload);
  }

  // ── Payload construction ──────────────────────────────────────────
  function buildPayload(): TransactionRequest {
    const payload: TransactionRequest = {
      status: mode === 'edit' ? transaction?.status : 'posted',
      tag_ids: [...tagIDs],
      transaction_date: transactionDate || undefined,
      description: description.trim() || undefined,
      note_markdown: noteMarkdown.trim() || undefined,
      external_ref_hint: externalRefHint.trim() || undefined
    };

    if (transactionKind && transactionKind !== 'ordinary') {
      payload.transaction_kind = transactionKind as TransactionRequest['transaction_kind'];
    }

    // An edit names the version this form shows; the server refuses it if
    // another write replaced that version meanwhile (#296).
    if (mode === 'edit' && transaction) {
      payload.expected_version_id = transaction.version_id;
    }

    if (payeeID !== undefined) {
      payload.payee_id = payeeID;
    } else if (payeeSearch.trim()) {
      payload.payee_name = payeeSearch.trim();
    }

    if (tier === 'transfer') {
      // A new transfer is one by kind; an edit or correction keeps the kind
      // it was saved with.
      if (mode === 'create') payload.transaction_kind = 'transfer';
      payload.journal_entries = buildTransferJournalEntries();
    } else if (tier === 'locked') {
      payload.keep_journal = true;
    } else if (tier === 'simple') {
      payload.journal_entries = buildSimpleJournalEntries();
    } else {
      payload.journal_entries = buildSplitJournalEntries();
    }

    return payload;
  }

  function buildSimpleJournalEntries(): TransactionRequest['journal_entries'] {
    if (!accountID || !effectiveCommodityID || !categoryID || !amountStr.trim()) {
      return [];
    }

    const account = assetLiabilityAccounts.find((a: AccountResponse) => a.id === accountID);
    if (!account) return [];

    // The owner enters a positive amount; the tab (or, editing, the
    // direction) decides the sign: money out is a credit, money in a debit,
    // for assets and liabilities alike (#506).
    const parsed = simpleAmount();
    if (parsed === null || parsed === 'negative') return [];
    const { value: assetValue, scale } = parsed;
    const assetLedgerValue = accountLedgerValue(direction, assetValue);
    // Category posting is the balancing leg — opposite sign.
    const catLedgerValue = negateCoefficient(assetLedgerValue);

    return [
      {
        entry_date: transactionDate,
        postings: [
          {
            line_key: simpleLineKeys.account,
            account_id: accountID,
            posted_on: mode === 'template' ? undefined : postedOnForRequest(accountPostedOn, transactionDate, account),
            commodity_id: effectiveCommodityID,
            quantity_value: assetLedgerValue,
            quantity_scale: scale,
            memo: ''
          },
          {
            line_key: simpleLineKeys.category,
            account_id: categoryID,
            commodity_id: effectiveCommodityID,
            quantity_value: catLedgerValue,
            quantity_scale: scale,
            memo: ''
          }
        ]
      }
    ];
  }

  /**
   * The filled split legs as postings, or null when one cannot be saved
   * whole: a filled line is never silently dropped (#511) — `splitProblem`
   * says which line and why. Blank lines are skipped.
   */
  function splitPostings(): SplitPosting[] | null {
    if (splitProblem) return null;
    const legs = resolvedSplitLegs.filter((leg) => leg.accountID !== '' && leg.amountStr.trim() !== '');

    const postings: SplitPosting[] = [];
    for (const leg of legs) {
      const parsed = parseDecimalAmount(leg.amountStr);
      // A leg the user filled in but we cannot parse must not silently become a
      // zero posting — refuse to build the payload, as the simple tier does.
      if (parsed === null) return null;
      const account = accountForLeg(leg.accountID);
      const isAccount = hasPostingDay(account);
      postings.push({
        lineKey: leg.lineKey,
        accountID: Number(leg.accountID),
        commodityID: Number(leg.commodityID),
        value: parsed.value,
        scale: parsed.scale,
        memo: leg.memo.trim(),
        isAccount,
        postedOn: mode === 'template' || !isAccount ? '' : leg.postedOn.trim(),
        followsDay: mode === 'template' ? undefined : leg.followsDay,
        tagIDs: leg.tagIDs
      });
    }
    return postings;
  }

  function buildSplitJournalEntries(): TransactionRequest['journal_entries'] {
    const postings = splitPostings();
    if (!postings || postings.length === 0) return [];

    // Account postings in two currencies are an exchange: the server adds
    // the commodity_trading legs the editor never shows (#481). What the
    // editor carries goes back while it still fits.
    const hasSystemAccount = postings.some((posting) => !!accountForLeg(String(posting.accountID))?.system_role);
    return [splitEntryRequest(postings, splitCarry, transactionDate, hasSystemAccount)];
  }

  /** The simple form's positive amount, null when it does not parse. */
  function simpleAmount(): { value: string; scale: number } | null | 'negative' {
    const parsed = parseDecimalAmount(amountStr);
    if (parsed === null) return null;
    return parsed.value.startsWith('-') ? 'negative' : parsed;
  }

  /** Which way the split moves the entry's account, when it says (#506 review). */
  function currentSplitDirection(): CashDirection | undefined {
    return splitDirection(
      splitLegs.map((leg) => ({ accountID: leg.accountID, amount: leg.amountStr.trim() ? parseDecimalAmount(leg.amountStr) : null })),
      accountID
    );
  }

  /**
   * The split's mirror: every leg's sign flips, so a Withdrawal's split
   * becomes the same Deposit and still balances. A carried transfer side
   * stays with its account, which now sends instead of receives.
   */
  function mirrorSplit() {
    splitLegs = splitLegs.map((leg) => {
      const parsed = leg.amountStr.trim() ? parseDecimalAmount(leg.amountStr) : null;
      return parsed ? { ...leg, amountStr: formatEditableAmount(negateCoefficient(parsed.value), parsed.scale) } : leg;
    });
    const other = (side?: TransferSideName) => (side === 'sending' ? 'receiving' : side === 'receiving' ? 'sending' : undefined);
    splitCarry = { ...splitCarry, conversionSide: other(splitCarry.conversionSide), feePaidBy: other(splitCarry.feePaidBy) };
  }

  /** Clears Transfer's own fields; From is the entry's account and stays. */
  function resetTransfer() {
    transferToID = undefined;
    transferToCommodityID = undefined;
    sentStr = '';
    receivedStr = '';
    feeStr = '';
    feePaidBy = 'sending';
    conversionSide = 'sending';
    transferMemos = {};
    feeAccountID = undefined;
    leavesOn = '';
    arrivesOn = '';
    showTransferDays = false;
    transferLineKeys = {};
    transferEntryKind = undefined;
    transferMessage = '';
  }

  function parsedFee() {
    return feeStr.trim() ? parseDecimalAmount(feeStr) : null;
  }

  function currentTransferProblem(): TransferProblem | 'no_fee_category' | null {
    if (feeStr.trim() && parsedFee() === null) return 'invalid_fee';
    const fee = parsedFee();
    if (fee && fee.value !== '0' && feeAccountID === undefined && bankFeesCategoryID === undefined) return 'no_fee_category';
    if (sentStr.trim() && parseDecimalAmount(sentStr) === null) return 'missing_amount';
    return transferProblem({
      fromAccountID: accountID && transferFromCommodity !== undefined ? accountID : undefined,
      toAccountID: transferToID && transferToCommodity !== undefined ? transferToID : undefined,
      sent: parseDecimalAmount(sentStr),
      received: parseDecimalAmount(receivedStr),
      exchange: transferIsExchange,
      leavesOn: leavesOn || transactionDate,
      arrivesOn: arrivesOn || transactionDate,
      fee: parsedFee(),
      feePaidBy
    });
  }

  function transferProblemMessage(problem: TransferProblem): string {
    switch (problem) {
      case 'missing_account': return m.transactions_transfer_problem_missing_account();
      case 'same_account': return m.transactions_transfer_problem_same_account();
      case 'missing_amount': return m.transactions_transfer_problem_missing_amount();
      case 'missing_received': return m.transactions_transfer_problem_missing_received();
      case 'arrives_before_leaves': return m.transactions_transfer_problem_arrives_before_leaves();
      case 'invalid_fee': return m.transactions_transfer_problem_invalid_fee();
      case 'fee_exceeds_amount': return m.transactions_transfer_problem_fee_exceeds_amount();
    }
  }

  function buildTransferJournalEntries(): TransactionRequest['journal_entries'] {
    const amount = parseDecimalAmount(sentStr);
    if (!accountID || !transferToID || transferFromCommodity === undefined || transferToCommodity === undefined || !amount) return [];
    const parsed = parsedFee();
    const feeCategory = feeAccountID ?? bankFeesCategoryID;
    const fee = parsed && parsed.value !== '0' && feeCategory ? { amount: parsed, paidBy: feePaidBy } : undefined;
    let statements = sameCurrencyStatements(amount, fee);
    if (transferIsExchange) {
      const received = parseDecimalAmount(receivedStr);
      if (!received) return [];
      statements = { sent: amount, received };
    }
    const entry = transferEntry({
      from: { accountID, commodityID: transferFromCommodity, postedOn: mode === 'template' ? '' : leavesOn, lineKey: transferLineKeys.from, memo: transferMemos.from },
      to: { accountID: transferToID, commodityID: transferToCommodity, postedOn: mode === 'template' ? '' : arrivesOn, lineKey: transferLineKeys.to, memo: transferMemos.to },
      sent: statements.sent,
      received: statements.received,
      fee: fee && feeCategory ? { accountID: feeCategory, amount: fee.amount, paidBy: fee.paidBy, lineKey: transferLineKeys.fee, memo: transferMemos.fee } : undefined,
      conversionSide: transferIsExchange ? conversionSide : undefined,
      entryMemo: transferMemos.entry,
      entryKind: transferEntryKind
    }, transactionDate);
    return [{ entry_date: transactionDate, ...entry }];
  }

  function switchTransferToSplit() {
    const built = buildTransferJournalEntries() ?? [];
    const entry = built[0];
    const postings = entry?.postings ?? [];
    if (postings.length >= 2) {
      // The transfer's days, memos and choices travel into the split: a fee's
      // day is carried, as the split shows no day for a category (#481).
      splitLegs = postings.map((p: JournalEntryPostingRequest) => {
        const isAccount = hasPostingDay(accountForLeg(String(p.account_id)));
        return {
          accountID: String(p.account_id),
          commodityID: String(p.commodity_id),
          amountStr: formatEditableAmount(p.quantity_value ?? '0', p.quantity_scale ?? 0),
          memo: p.memo ?? '',
          lineKey: p.line_key,
          postedOn: isAccount ? (p.posted_on ?? '') : '',
          followsDay: isAccount ? undefined : p.posted_on
        };
      });
      splitCarry = {
        entryKind: entry.entry_kind ?? splitCarry.entryKind,
        memo: entry.memo,
        conversionSide: entry.conversion_side,
        feePaidBy: entry.fee_paid_by
      };
    }
    tier = 'split';
    // New transaction: split entry belongs to the cash tabs (#506).
    if (tabbed) {
      cashShape = 'split';
      direction = currentSplitDirection() ?? direction;
      entryTab = direction;
    }
  }

  /** Editing: the simple form becomes a transfer from its account (#481). */
  function switchToTransfer() {
    resetTransfer();
    tier = 'transfer';
  }

  // ── Discarding (#508) ─────────────────────────────────────────────
  // Closing or leaving asks first only when something entered would be
  // lost: a field differs from what the entry opened with, or an investment
  // activity's working copy holds the owner's work.
  function entryWork(): string {
    // Every field the owner enters, financial dates included (#521 review).
    // simpleCommodityID holds only a chosen currency; one taken from the
    // account is inferredCommodityID and stays out.
    return JSON.stringify([transactionDate, payeeSearch, description, categoryID, amountStr, accountID, simpleCommodityID,
      accountPostedOn, noteMarkdown, externalRefHint, tagIDs, sentStr, receivedStr, feeStr, feePaidBy,
      feeAccountID, conversionSide, transferToID, transferToCommodityID, leavesOn, arrivesOn,
      splitLegs.map((leg) => [leg.accountID, leg.commodityID, leg.amountStr, leg.memo, leg.postedOn])]);
  }
  let workBaseline: string | undefined;
  function hasUnsavedWork(): boolean {
    return tabbed && (entryWork() !== workBaseline || entry.copies.hasUnsavedWork());
  }
  let discardOpen = $state(false);
  let afterDiscard: (() => void) | null = null;
  // Set once the entry saved or the owner chose to discard it.
  let leaving = false;

  /**
   * Anything that would close or replace the entry: runs `then` now, or asks
   * first when work would be lost. A host routes every dismissal through it:
   * Cancel, Escape, a backdrop, or a row opened into the same panel.
   */
  export function requestLeave(then: () => void) {
    if (busy || entry.saving || discardOpen) return;
    if (!hasUnsavedWork()) {
      then();
      return;
    }
    afterDiscard = then;
    discardOpen = true;
  }

  /** Cancel, Escape or the close button. */
  export function requestCancel() {
    requestLeave(onCancel);
  }

  function confirmDiscard() {
    discardOpen = false;
    leaving = true;
    const next = afterDiscard;
    afterDiscard = null;
    next?.();
  }

  function keepEditing() {
    discardOpen = false;
    afterDiscard = null;
  }

  // Leaving the page the entry lives on loses it too. A same-page URL change
  // (a filter, a selection) keeps the entry, and closing the browser tab is
  // the browser's own decision.
  beforeNavigate((navigation) => {
    if (leaving || !navigation.to || navigation.type === 'leave') return;
    if (navigation.to.url.pathname === navigation.from?.url.pathname || !hasUnsavedWork()) return;
    navigation.cancel();
    const target = navigation.to.url;
    const delta = navigation.type === 'popstate' ? navigation.delta : undefined;
    afterDiscard = () => {
      onCancel();
      if (delta) history.go(delta);
      else void goto(target);
    };
    discardOpen = true;
  });

  /**
   * New transaction's tab switch (#506). Nothing is reset: each tab's fields
   * stay as entered, Withdrawal ↔ Deposit keep the amount and only change its
   * direction, and the cash tabs come back in the shape they were left in.
   */
  function selectTab(next: EntryTab) {
    if (busy || entry.saving || next === entryTab) return;
    if (entryTab === 'out' || entryTab === 'in') cashShape = tier === 'split' ? 'split' : 'simple';
    // A payment started as a Withdrawal that is really a transfer: an empty
    // Transfer amount starts from the amount leaving the same account. One
    // that was entered is never replaced.
    if (next === 'transfer' && !sentStr.trim() && cashShape === 'simple' && direction === 'out') {
      const amount = simpleAmount();
      if (amount && amount !== 'negative') sentStr = amountStr;
    }
    entryTab = next;
    direction = directionForTab(next) ?? direction;
    const shape = shapeForTab(next, cashShape);
    if (shape !== 'investment') tier = shape;
    // A kept split under the other cash tab is the same entry the other way:
    // its legs flip with the direction, so Deposit never saves a withdrawal.
    if (shape === 'split' && (next === 'out' || next === 'in')) {
      const moves = currentSplitDirection();
      if (moves && moves !== next) mirrorSplit();
    }
    entryMessage = '';
    transferMessage = '';
  }

  // A shown approval belongs to the entry as it was. A change of tab,
  // account, direction or shape closes it, and a closed review is never
  // reused: saving asks again (#506). The reconciliation override also
  // checks its reviewed request at confirmation.
  const entryMeaning = $derived(`${entryTab}|${tier}|${direction}|${accountID ?? ''}|${transferToID ?? ''}`);
  $effect(() => {
    void entryMeaning;
    untrack(() => {
      // A refusal about the entry as it was no longer describes it.
      entryMessage = '';
      transferMessage = '';
      if (payeeConfirm.open) payeeConfirm = { open: false, resolution: null };
      if (reconciliationModal.open) reconciliationModal = { open: false, impacts: [], pendingPayload: {} as TransactionRequest };
    });
  });

  // The split keeps the account it was entered with while another tab
  // changes the entry's account (Transfer's From, the simple form): its legs
  // follow, so the cash tab's label and the saved postings name the same
  // account (#506 review). Inside the split editor the legs lead instead.
  let splitBoundAccountID: number | undefined;
  $effect(() => {
    const next = accountID;
    untrack(() => {
      const previous = splitBoundAccountID;
      if (next !== undefined) splitBoundAccountID = next;
      if (!tabbed || tier === 'split' || cashShape !== 'split') return;
      if (previous === undefined || next === undefined || previous === next) return;
      splitLegs = rebindSplitAccount(splitLegs, previous, next, commodityForAccount);
    });
  });

  function swapTransferSides() {
    [accountID, transferToID] = [transferToID, accountID];
    [simpleCommodityID, transferToCommodityID] = [transferToCommodityID, simpleCommodityID];
    [leavesOn, arrivesOn] = [arrivesOn, leavesOn];
    if (transferIsExchange) [sentStr, receivedStr] = [receivedStr, sentStr];
    [transferLineKeys, transferMemos] = [
      { from: transferLineKeys.to, to: transferLineKeys.from, fee: transferLineKeys.fee },
      { ...transferMemos, from: transferMemos.to, to: transferMemos.from }
    ];
    feePaidBy = feePaidBy === 'sending' ? 'receiving' : 'sending';
    conversionSide = conversionSide === 'sending' ? 'receiving' : 'sending';
  }

  // ── Helpers ───────────────────────────────────────────────────────

  function selectCategory(cat: CategoryResponse) {
    categoryID = cat.id;
    categoryLabel = categoryDisplayName(cat);
    categorySearch = '';
    categoryDropdownOpen = false;
    categoryActive = -1;
  }

  function clearCategory() {
    categoryID = undefined;
    categoryLabel = '';
    categorySearch = '';
  }

  function addSplitLeg() {
    splitLegs = [...splitLegs, { accountID: '', commodityID: '', amountStr: '', memo: '', postedOn: '' }];
  }

  function removeSplitLeg(i: number) {
    const was = splitLegs[i]?.accountID ?? '';
    splitLegs = splitLegs.filter((_, idx) => idx !== i);
    if (tabbed) adoptSplitEntryAccount(splitLegs, undefined, was);
  }

  function switchToSplit() {
    // Copy existing simple fields into split legs.
    const parsed = accountID && amountStr ? simpleAmount() : null;
    if (parsed !== null && parsed !== 'negative') {
      const { value, scale } = parsed;
      const assetLedger = accountLedgerValue(direction, value);
      splitLegs = [
        {
          accountID: String(accountID),
          commodityID: effectiveCommodityID ? String(effectiveCommodityID) : '',
          amountStr: formatEditableAmount(assetLedger, scale),
          memo: '',
          // The same lines, so their reconciliation state stays theirs.
          lineKey: simpleLineKeys.account,
          postedOn: accountPostedOn
        },
        {
          accountID: categoryID ? String(categoryID) : '',
          // The category follows the account line while the account has a
          // currency of its own; a chosen one is copied (#511).
          commodityID: inferredCommodityID === undefined && effectiveCommodityID ? String(effectiveCommodityID) : '',
          amountStr: formatEditableAmount(negateCoefficient(assetLedger), scale),
          memo: '',
          lineKey: simpleLineKeys.category,
          postedOn: ''
        }
      ];
    }
    tier = 'split';
    if (tabbed) cashShape = 'split';
  }

  // The split as the simple form would show it, or null when it would drop
  // something; an untouched new split switches freely.
  const splitIsEmpty = $derived(splitLegs.every((leg) => leg.accountID === '' && leg.amountStr.trim() === ''));
  const simpleFit = $derived.by(() => {
    if (tier !== 'split' || preservePostings) return null;
    const postings = splitPostings();
    if (!postings) return null;
    const categoryIDs = new Set((categoriesQuery.data?.categories ?? []).map((category: CategoryResponse) => category.id));
    return simpleFormFits(postings, splitCarry, (id) => categoryIDs.has(id));
  });

  function switchToSimple() {
    const fit = simpleFit;
    if (fit) {
      accountID = fit.account.accountID;
      simpleCommodityID = fit.account.commodityID;
      const shown = directionOf(fit.account.value);
      direction = shown.direction;
      if (tabbed) entryTab = shown.direction;
      amountStr = formatEditableAmount(shown.magnitude, fit.account.scale);
      accountPostedOn = fit.account.postedOn;
      showAccountPostedOn = fit.account.postedOn !== '';
      categoryID = fit.category.accountID;
      const category = categoriesQuery.data?.categories.find((c: CategoryResponse) => c.id === fit.category.accountID);
      categoryLabel = category ? categoryDisplayName(category) : String(fit.category.accountID);
      simpleLineKeys = { account: fit.account.lineKey, category: fit.category.lineKey };
    } else if (!splitIsEmpty) {
      return;
    }
    tier = 'simple';
    if (tabbed) cashShape = 'simple';
  }

  // Currency choices include currencies used by clearing accounts with no default.
  const allActiveCommodities = $derived(currenciesQuery.data?.currencies ?? []);

  // All accounts for split leg account selector.
  const allPostingAccounts = $derived(
    (accountsQuery.data?.accounts ?? []).filter(
      (a: AccountResponse) => a.status === 'active' && a.allows_postings && !isSubledgerManagedAccountKind(a.account_kind)
    )
  );

  function postingAccountLabel(account: AccountResponse): string {
    if (account.system_role) return systemRoleLabel(account.system_role);
    const category = categoriesQuery.data?.categories.find(c => c.id === account.id);
    return category ? categoryDisplayName(category) : accountDisplayName(account);
  }

  // Commodity from a specific account (for split legs).
  function commodityForAccount(accIDStr: string): string {
    if (!accIDStr) return '';
    const acc = (accountsQuery.data?.accounts ?? []).find((a: AccountResponse) => a.id === Number(accIDStr));
    return acc?.default_commodity_id ? String(acc.default_commodity_id) : '';
  }

  function accountForLeg(accIDStr: string): AccountResponse | undefined {
    if (!accIDStr) return undefined;
    return (accountsQuery.data?.accounts ?? []).find((a: AccountResponse) => a.id === Number(accIDStr));
  }

  function setLegPostedOn(i: number, postedOn: string) {
    const legs = [...splitLegs];
    legs[i] = { ...legs[i], postedOn };
    splitLegs = legs;
  }

  function onSplitAccountChange(i: number, e: Event) {
    const val = (e.target as HTMLSelectElement).value;
    const newLegs = [...splitLegs];
    const was = newLegs[i].accountID;
    newLegs[i] = { ...newLegs[i], accountID: val, commodityID: commodityForAccount(val) };
    splitLegs = newLegs;
    if (tabbed) adoptSplitEntryAccount(newLegs, i, was);
  }

  /**
   * Inside the split editor the legs lead the entry's account, for the tab
   * labels, the direction and every other tab (#506 review). While a leg is
   * on the entry's account it stays; when none is — no account was chosen
   * yet (a split started first) or its leg moved away — the leg just changed
   * takes it if it is an account, else the first account leg in order.
   */
  function adoptSplitEntryAccount(legs: SplitLeg[], changed: number | undefined, was: string) {
    if (accountID !== undefined && legs.some((leg) => leg.accountID === String(accountID))) return;
    const asAccount = (leg: SplitLeg) => assetLiabilityAccounts.find((a: AccountResponse) => String(a.id) === leg.accountID);
    const next = (changed === undefined ? undefined : asAccount(legs[changed])) ?? legs.map(asAccount).find((account) => account !== undefined);
    if (next) accountID = next.id;
    else if (accountID !== undefined && was === String(accountID)) accountID = undefined;
  }
</script>

{#if discardOpen}
  <ModalDialog role="alertdialog" labelledby={`${uid}-discard-title`} describedby={`${uid}-discard-body`}
    onCancel={keepEditing} class="max-w-md">
    <div class="space-y-2 px-4 py-3">
      <h3 id={`${uid}-discard-title`} class="text-sm font-semibold text-foreground">{m.transactions_entry_discard_title()}</h3>
      <p id={`${uid}-discard-body`} class="text-sm text-muted">{m.transactions_entry_discard_body()}</p>
    </div>
    <div class="flex flex-wrap justify-end gap-2 border-t border-border px-4 py-3">
      <button type="button" data-initial-focus onclick={keepEditing}
        class="inline-flex min-h-11 items-center rounded-[var(--radius-control)] border border-border bg-control px-4 py-2 text-sm font-semibold text-foreground transition hover:bg-control-hover">
        {m.transactions_entry_discard_keep()}
      </button>
      <button type="button" onclick={confirmDiscard}
        class="inline-flex min-h-11 items-center rounded-[var(--radius-control)] bg-accent px-4 py-2 text-sm font-semibold text-accent-foreground transition hover:opacity-90">
        {m.transactions_entry_discard_confirm()}
      </button>
    </div>
  </ModalDialog>
{/if}

<!-- Reconciliation warning modal -->
{#if reconciliationModal.open}
  <ModalDialog role="alertdialog" labelledby="recon-modal-title" {pending}
    onCancel={() => (reconciliationModal = { ...reconciliationModal, open: false })} class="max-w-lg">
    <div class="flex items-start gap-3 border-b border-border px-4 py-3">
      <TriangleExclamationPoint size={18} class="mt-0.5 shrink-0 text-warning" aria-hidden="true" />
      <div class="min-w-0">
        <h3 id="recon-modal-title" class="text-sm font-semibold text-foreground">
          {m.transactions_reconciliation_warning_title()}
        </h3>
        <p class="mt-1 text-xs leading-5 text-muted">{m.transactions_reconciliation_warning_copy()}</p>
      </div>
    </div>

    <ul class="divide-y divide-border px-4 py-3 text-sm">
      {#each reconciliationModal.impacts as cp (cp.checkpoint_id)}
        <li class="py-2 text-foreground">
          {m.transactions_reconciliation_checkpoint_label({
            account: cp.account_label,
            commodity: cp.commodity_code,
            date: cp.statement_date
          })}
        </li>
      {/each}
    </ul>

    <div class="flex flex-wrap justify-end gap-2 border-t border-border px-4 py-3">
      <button
        type="button"
        class="inline-flex items-center gap-2 rounded-[var(--radius-control)] border border-border bg-control px-4 py-2.5 text-sm font-semibold text-foreground transition hover:bg-control-hover"
        onclick={() => (reconciliationModal = { ...reconciliationModal, open: false })}
      >
        {m.transactions_reconciliation_cancel()}
      </button>
      <button
        type="button"
        class="inline-flex items-center gap-2 rounded-[var(--radius-control)] bg-warning px-4 py-2.5 text-sm font-semibold text-background transition hover:ring-2 hover:ring-warning/40 disabled:opacity-60"
        onclick={confirmReconciliationOverride}
        disabled={pending}
      >
        <TriangleExclamationPoint size={14} aria-hidden="true" />
        {m.transactions_reconciliation_confirm()}
      </button>
    </div>
  </ModalDialog>
{/if}

<!-- A region named by its heading: the entry's header and tabs sit outside
     its form, which the Investment tab hides (#506). -->
<!-- editorHold: global search stays closed while this is open (#490). -->
<section class="space-y-4" aria-labelledby={`${uid}-title`} {@attach editorHold}>
  <!-- Header -->
  <div class="flex items-start justify-between gap-3">
    <div class="min-w-0">
      <h2 id={`${uid}-title`} class="text-base font-semibold text-foreground">{title}</h2>
      {#if mode === 'edit' && transaction}
        <p class="mt-1 truncate text-sm text-muted">
          {transactionTitle(transaction, systemLabelText, String(transaction.id))}
        </p>
      {/if}
    </div>
    <button
      type="button"
      class="inline-flex h-9 w-9 items-center justify-center rounded-[var(--radius-control)] border border-border bg-control text-foreground transition hover:bg-control-hover"
      onclick={requestCancel}
      disabled={busy}
      aria-label={m.transactions_form_cancel()}
      title={m.transactions_form_cancel()}
    >
      <X size={16} aria-hidden="true" />
    </button>
  </div>

  {#if tabbed}
    <EntryTabs
      tabs={entryTabList}
      active={entryTab}
      onSelect={selectTab}
      panelID={`${uid}-entry-panel`}
      idPrefix={uid}
      label={m.transactions_entry_tabs_label()}
      disabled={busy || entry.saving}
    />
  {/if}

  <div id={`${uid}-entry-panel`} role={tabbed ? 'tabpanel' : undefined} aria-labelledby={tabbed ? `${uid}-tab-${entryTab}` : undefined}>
  <!-- Hidden, not unmounted, on the Investment tab: its fields are the
       entry's shared context and working copies (#506). -->
  <form class="space-y-4" onsubmit={handleSubmit} aria-busy={busy} hidden={tabbed && entryTab === 'investment'}>
  <fieldset disabled={busy} class="space-y-4">
  {#if templateFields}{@render templateFields()}{/if}

  <!-- Correction banner -->
  {#if mode === 'correct'}
    <div class="flex items-start gap-2 rounded-[var(--radius-control)] border border-warning/40 bg-warning-soft px-3 py-2.5 text-sm text-foreground">
      <TriangleExclamationPoint size={15} class="mt-0.5 shrink-0" aria-hidden="true" />
      <span>{m.transactions_correction_banner()}</span>
    </div>
  {/if}

  <!-- Voided — blocked -->
  {#if isVoided && mode === 'edit'}
    <div class="rounded-[var(--radius-control)] border border-border bg-surface px-3 py-2.5 text-sm text-muted">
      {m.transactions_voided_edit_blocked()}
    </div>
  {/if}

  {#if reloadedNotice}
    <div class="rounded-[var(--radius-control)] border border-warning/50 bg-warning-soft px-3 py-2.5 text-sm text-foreground" role="status">
      {m.transactions_form_reloaded_newer_version()}
    </div>
  {/if}

  <APIFormError error={formError} id="transaction-form-error" />

  <!-- ── Tier 1 / shared fields ───────────────────────────────────── -->
  <div class="grid gap-3 sm:grid-cols-2">
    <!-- Date -->
    {#if mode !== 'template'}
    <div>
      <label>
        <span class={labelClass}>{m.transactions_field_date()}</span>
        <input
          type="date"
          bind:value={transactionDate}
          class={inputClass}
          required
          disabled={(isVoided && mode === 'edit') || tier === 'locked'}
        />
      </label>
      {#if tier === 'simple'}
        {#if showAccountPostedOn}
          <label class="mt-2 block">
            <span class={labelClass}>{m.transactions_field_posted_on()}</span>
            <input
              type="date"
              value={accountPostedOn || transactionDate}
              oninput={(e) => { accountPostedOn = e.currentTarget.value; }}
              class={inputClass}
              aria-describedby="transaction-posted-on-hint"
              disabled={isVoided && mode === 'edit'}
            />
            <span id="transaction-posted-on-hint" class="mt-1 block text-xs text-muted">{m.transactions_posted_on_hint()}</span>
          </label>
        {:else}
          <button
            type="button"
            class="mt-1 min-h-10 text-xs font-medium text-accent underline-offset-2 hover:underline"
            onclick={() => { showAccountPostedOn = true; }}
            disabled={isVoided && mode === 'edit'}
          >
            {m.transactions_posted_on_toggle()}
          </button>
        {/if}
      {/if}
    </div>
    {/if}
    <!-- Payee autocomplete -->
    <label class="relative">
      <span id={`${uid}-payee-label`} class={labelClass}>{m.transactions_field_payee()}</span>
      <input
        id={payeeInputID}
        type="text"
        aria-labelledby={`${uid}-payee-label`}
        role="combobox"
        aria-autocomplete="list"
        aria-expanded={payeeDropdownOpen && payeeMatches.length > 0}
        aria-controls={payeeDropdownOpen && payeeMatches.length > 0 ? `${uid}-payees` : undefined}
        aria-activedescendant={payeeDropdownOpen && payeeActive >= 0 ? `${uid}-payee-${payeeActive}` : undefined}
        value={payeeSearch}
        oninput={onPayeeInput}
        onkeydown={payeeKeydown}
        onfocus={() => { if (payeeSearch) payeeDropdownOpen = true; }}
        onblur={() => setTimeout(() => { payeeDropdownOpen = false; payeeActive = -1; }, 150)}
        class={inputClass}
        placeholder={m.transactions_field_payee_placeholder()}
        autocomplete="off"
        disabled={isVoided && mode === 'edit'}
      />
      {#if payeeDropdownOpen && payeeMatches.length > 0}
        <ul
          id={`${uid}-payees`}
          class="absolute z-20 mt-1 w-full rounded-[var(--radius-control)] border border-border bg-surface text-sm shadow-[var(--shadow-panel)]"
          role="listbox"
          aria-label={m.transactions_field_payee()}
        >
          {#each payeeMatches as payee, index (payee.id)}
            <li role="none">
              <button
                type="button"
                id={`${uid}-payee-${index}`}
                tabindex="-1"
                class="w-full px-3 py-2 text-left hover:bg-row-hover aria-selected:bg-row-hover"
                onmousedown={() => choosePayee(payee)}
                role="option"
                aria-selected={index === payeeActive}
              >
                {payee.name}
              </button>
            </li>
          {/each}
        </ul>
      {/if}
    </label>

    <!--
      No payee carries this name. Creating one is a deliberate act, so it is
      confirmed here rather than happening as a side effect of saving — and the
      near matches come first, because the usual cause is a typo or a different
      word order for a payee that already exists.
    -->
    {#if payeeConfirm.open && payeeConfirm.resolution?.kind === 'unknown'}
      <div
        class="rounded-(--radius-control) border border-border bg-control p-3 sm:col-span-2"
        role="group"
        aria-label={m.transactions_payee_confirm_title({ name: payeeConfirm.resolution.name })}
      >
        <p class="text-sm font-semibold text-foreground">
          {m.transactions_payee_confirm_title({ name: payeeConfirm.resolution.name })}
        </p>

        {#if payeeConfirm.resolution.suggestions.length > 0}
          <p class="mt-2 text-xs text-muted">{m.transactions_payee_confirm_suggestions()}</p>
          <ul class="mt-2 flex flex-wrap gap-2">
            {#each payeeConfirm.resolution.suggestions as suggestion (suggestion.id)}
              <li>
                <button
                  type="button"
                  class="rounded-(--radius-control) border border-border bg-surface px-3 py-1.5 text-sm text-foreground transition hover:bg-control-hover"
                  onclick={() => usePayeeSuggestion(suggestion)}
                >
                  {suggestion.name}
                </button>
              </li>
            {/each}
          </ul>
        {/if}

        <div class="mt-3 flex flex-wrap gap-2">
          <button
            type="button"
            class="rounded-(--radius-control) bg-accent px-3 py-1.5 text-sm font-semibold text-accent-foreground transition hover:opacity-90 disabled:opacity-60"
            disabled={payeeCreatePending}
            onclick={() =>
              payeeConfirm.resolution?.kind === 'unknown' &&
              createTypedPayee(payeeConfirm.resolution.name)}
          >
            {payeeCreatePending
              ? m.transactions_payee_confirm_creating()
              : m.transactions_payee_confirm_create({ name: payeeConfirm.resolution.name })}
          </button>
          <button
            type="button"
            class="rounded-(--radius-control) border border-border bg-surface px-3 py-1.5 text-sm font-semibold text-foreground transition hover:bg-control-hover"
            onclick={editPayeeName}
          >
            {m.transactions_payee_confirm_edit()}
          </button>
        </div>
      </div>
    {/if}

    <!-- Description -->
    <label class="sm:col-span-2">
      <span class={labelClass}>{m.transactions_field_description()}</span>
      <input
        type="text"
        bind:value={description}
        class={inputClass}
        maxlength="500"
        disabled={isVoided && mode === 'edit'}
      />
    </label>
  </div>

  {#if tier === 'transfer'}
    <!-- ── Transfer between two accounts (#481) ───────────────────── -->
    <div class="space-y-3" role="group" aria-label={m.transactions_transfer_title()}>
      {#if transferMessage}
        <p role="alert" class="flex items-start gap-2 rounded-[var(--radius-control)] border border-danger/40 bg-danger-soft px-3 py-2.5 text-sm text-foreground">
          <TriangleExclamationPoint size={15} class="mt-0.5 shrink-0 text-danger" aria-hidden="true" />
          <span>{transferMessage}</span>
        </p>
      {/if}
      <div class="grid gap-3 sm:grid-cols-[1fr_auto_1fr] sm:items-end">
        <label>
          <span class={labelClass}>{m.transactions_transfer_from()}</span>
          <select
            value={accountID ?? ''}
            onchange={(e) => { accountID = Number(e.currentTarget.value) || undefined; }}
            class={inputClass}
            required
            aria-invalid={accountProblem ? true : undefined}
            aria-describedby={accountProblem ? `${uid}-account-problem` : undefined}
            disabled={isVoided && mode === 'edit'}
          >
            <option value="">—</option>
            {#if accountProblem && contextAccount}
              <option value={contextAccount.id} disabled>{accountDisplayName(contextAccount)}</option>
            {/if}
            {#each assetLiabilityAccounts as a (a.id)}
              <option value={a.id}>{postingAccountLabel(a)}</option>
            {/each}
          </select>
        </label>
        <button
          type="button"
          class="inline-flex h-10 min-w-11 items-center justify-center rounded-[var(--radius-control)] border border-border bg-control px-3 text-sm text-foreground transition hover:bg-control-hover"
          onclick={swapTransferSides}
          aria-label={m.transactions_transfer_swap()}
          title={m.transactions_transfer_swap()}
          disabled={isVoided && mode === 'edit'}
        >
          <span aria-hidden="true">⇄</span>
        </button>
        <label>
          <span class={labelClass}>{m.transactions_transfer_to()}</span>
          <select
            value={transferToID ?? ''}
            onchange={(e) => { transferToID = Number(e.currentTarget.value) || undefined; transferToCommodityID = undefined; }}
            class={inputClass}
            required
            disabled={isVoided && mode === 'edit'}
          >
            <option value="">—</option>
            {#each assetLiabilityAccounts as a (a.id)}
              <option value={a.id}>{postingAccountLabel(a)}</option>
            {/each}
          </select>
        </label>
      </div>

      {#if accountProblem}
        <p id={`${uid}-account-problem`} class="text-sm text-foreground">{accountProblemText}</p>
      {/if}

      {#if (transferFromAccount && transferFromAccount.default_commodity_id == null) || (transferToID !== undefined && transferToAccount?.default_commodity_id == null)}
        <div class="grid gap-3 sm:grid-cols-2">
          {#if transferFromAccount && transferFromAccount.default_commodity_id == null}
            <label>
              <span class={labelClass}>{m.transactions_transfer_from_currency()}</span>
              <select value={simpleCommodityID ?? ''} onchange={(e) => { simpleCommodityID = Number(e.currentTarget.value) || undefined; }} class={inputClass} required>
                <option value="">—</option>
                {#each allActiveCommodities as c (c.id)}<option value={c.id}>{c.code}</option>{/each}
              </select>
            </label>
          {/if}
          {#if transferToID !== undefined && transferToAccount?.default_commodity_id == null}
            <label>
              <span class={labelClass}>{m.transactions_transfer_to_currency()}</span>
              <select value={transferToCommodityID ?? ''} onchange={(e) => { transferToCommodityID = Number(e.currentTarget.value) || undefined; }} class={inputClass} required>
                <option value="">—</option>
                {#each allActiveCommodities as c (c.id)}<option value={c.id}>{c.code}</option>{/each}
              </select>
            </label>
          {/if}
        </div>
      {/if}

      <div class="grid gap-3 sm:grid-cols-2">
        <label>
          <span class={labelClass}>
            {transferIsExchange ? m.transactions_transfer_sent({ currency: commodityCode(transferFromCommodity) }) : m.transactions_field_amount()}
          </span>
          <input type="text" bind:value={sentStr} class={inputClass} inputmode="decimal" placeholder={formatEditableAmount('0', 2)} required aria-describedby={transferIsExchange ? 'transfer-statements-hint' : undefined} disabled={isVoided && mode === 'edit'} />
        </label>
        {#if transferIsExchange}
          <label>
            <span class={labelClass}>{m.transactions_transfer_received({ currency: commodityCode(transferToCommodity) })}</span>
            <input type="text" bind:value={receivedStr} class={inputClass} inputmode="decimal" placeholder={formatEditableAmount('0', 2)} required aria-describedby="transfer-statements-hint transfer-rate" disabled={isVoided && mode === 'edit'} />
          </label>
          <p id="transfer-statements-hint" class="text-xs text-muted sm:col-span-2">{m.transactions_transfer_statements_hint()}</p>
          <p id="transfer-rate" class="text-xs text-muted sm:col-span-2" aria-live="polite">
            {#if transferConversion}
              {m.transactions_transfer_converted({ sent: transferConversion.sent, received: transferConversion.received })}
              · {m.transactions_transfer_rate({ rate: transferConversion.rate, from: commodityCode(transferFromCommodity), to: commodityCode(transferToCommodity) })}
            {:else}
              {m.transactions_transfer_rate_hint()}
            {/if}
          </p>
        {/if}
      </div>

      <div class="grid gap-3 sm:grid-cols-2">
        <div>
          <label>
            <span class={labelClass}>{m.transactions_transfer_fee()}</span>
            <input type="text" bind:value={feeStr} class={inputClass} inputmode="decimal" placeholder={formatEditableAmount('0', 2)} aria-describedby="transfer-fee-hint" disabled={isVoided && mode === 'edit'} />
          </label>
          <span id="transfer-fee-hint" class="mt-1 block text-xs text-muted">{m.transactions_transfer_fee_hint()}</span>
        </div>
        {#if feeStr.trim()}
          <label>
            <span class={labelClass}>{m.transactions_transfer_fee_paid_by()}</span>
            <select bind:value={feePaidBy} class={inputClass} disabled={isVoided && mode === 'edit'}>
              <option value="sending">{m.transactions_transfer_fee_paid_by_from({ currency: commodityCode(transferFromCommodity) })}</option>
              <option value="receiving">{m.transactions_transfer_fee_paid_by_to({ currency: commodityCode(transferToCommodity) })}</option>
            </select>
          </label>
        {/if}
      </div>

      {#if mode !== 'template'}
        {#if showTransferDays}
          <div class="grid gap-3 sm:grid-cols-2">
            <label>
              <span class={labelClass}>{m.transactions_transfer_leaves_on()}</span>
              <input type="date" value={leavesOn || transactionDate} oninput={(e) => { leavesOn = e.currentTarget.value; }} class={inputClass} disabled={isVoided && mode === 'edit'} />
            </label>
            <label>
              <span class={labelClass}>{m.transactions_transfer_arrives_on()}</span>
              <input type="date" value={arrivesOn || transactionDate} oninput={(e) => { arrivesOn = e.currentTarget.value; }} class={inputClass} aria-describedby="transfer-days-hint" disabled={isVoided && mode === 'edit'} />
            </label>
            <p id="transfer-days-hint" class="text-xs text-muted sm:col-span-2">{m.transactions_transfer_days_hint()}</p>
            {#if transferIsExchange}
              <label class="sm:col-span-2">
                <span class={labelClass}>{m.transactions_transfer_conversion_side()}</span>
                <select bind:value={conversionSide} class={inputClass} disabled={isVoided && mode === 'edit'}>
                  <option value="sending">{m.transactions_transfer_conversion_sending()}</option>
                  <option value="receiving">{m.transactions_transfer_conversion_receiving()}</option>
                </select>
              </label>
            {/if}
          </div>
        {:else}
          <button type="button" class="min-h-10 text-xs font-medium text-accent underline-offset-2 hover:underline" onclick={() => { showTransferDays = true; }} disabled={isVoided && mode === 'edit'}>
            {m.transactions_transfer_days_toggle()}
          </button>
        {/if}
      {/if}

      <div class="flex justify-end">
        <button type="button" class="min-h-10 text-xs font-medium text-accent underline-offset-2 hover:underline" onclick={switchTransferToSplit}>
          {m.transactions_tier_switch_split()}
        </button>
      </div>
    </div>
  {:else if tier === 'locked'}
    <div class="space-y-2 rounded-[var(--radius-control)] border border-border bg-surface px-3 py-3" role="note" aria-labelledby="locked-journal-title">
      <p id="locked-journal-title" class="text-sm font-semibold text-foreground">{m.transactions_locked_title()}</p>
      <p class="text-sm text-muted">{mode === 'correct' ? m.transactions_locked_correct() : m.transactions_locked_body()}</p>
      <ul class="space-y-1">
        {#each lockedPostings as row (row.key)}
          <li class="flex items-center justify-between gap-3 text-sm">
            <span class="min-w-0">
              <span class="block truncate text-foreground">{row.label}</span>
              <span class="block text-xs tabular-nums text-muted">{row.date}</span>
            </span>
            <span class="shrink-0 font-medium tabular-nums text-foreground">{row.amount}</span>
          </li>
        {/each}
      </ul>
    </div>
  {:else if tier === 'simple'}
    <!-- ── Tier 1: simple two-posting form ─────────────────────────── -->
    {#if entryMessage}
      <p role="alert" class="flex items-start gap-2 rounded-[var(--radius-control)] border border-danger/40 bg-danger-soft px-3 py-2.5 text-sm text-foreground">
        <TriangleExclamationPoint size={15} class="mt-0.5 shrink-0 text-danger" aria-hidden="true" />
        <span>{entryMessage}</span>
      </p>
    {/if}
    {#if !tabbed}
      <!-- Editing: the direction the positive amount moves the account (#506). -->
      <fieldset class="flex flex-wrap gap-x-4 gap-y-1">
        <legend class={labelClass}>{m.transactions_entry_direction()}</legend>
        {#each ['out', 'in'] as const as value (value)}
          <label class="flex min-h-10 items-center gap-2 text-sm text-foreground">
            <input type="radio" name={`${uid}-direction`} {value} bind:group={direction} disabled={isVoided && mode === 'edit'} />
            {directionLabel(value)}
          </label>
        {/each}
      </fieldset>
    {:else if entryTab === 'in' && isCreditCard(selectedAccount)}
      <p class="text-sm text-muted">{m.transactions_entry_card_credit_note()}</p>
    {/if}
    <div class="grid gap-3 sm:grid-cols-2">
      <!-- Account (asset/liability leg). Changing it keeps every other field (#506). -->
      <label>
        <span class={labelClass}>{m.transactions_field_account()}</span>
        <select
          value={accountID ?? ''}
          onchange={(e) => {
            accountID = Number((e.target as HTMLSelectElement).value) || undefined;
          }}
          class={inputClass}
          aria-invalid={accountProblem ? true : undefined}
          aria-describedby={accountProblem ? `${uid}-account-problem` : undefined}
          disabled={isVoided && mode === 'edit'}
        >
          <option value="">—</option>
          {#if accountProblem && contextAccount}
            <option value={contextAccount.id} disabled>{accountDisplayName(contextAccount)}</option>
          {/if}
          {#each assetLiabilityAccounts as a (a.id)}
            <option value={a.id}>{postingAccountLabel(a)}</option>
          {/each}
        </select>
      </label>

      <!-- Amount: positive; the tab or direction decides its sign (#506). -->
      <label>
        <span class={labelClass}>{m.transactions_field_amount()}</span>
        <input
          type="text"
          bind:value={amountStr}
          class={inputClass}
          inputmode="decimal"
          placeholder={formatEditableAmount('0', 2)}
          required={tier === 'simple'}
          disabled={isVoided && mode === 'edit'}
        />
      </label>

      {#if accountProblem}
        <p id={`${uid}-account-problem`} class="text-sm text-foreground sm:col-span-2">{accountProblemText}</p>
      {/if}

      <!-- Commodity selector — only shown when the selected account has no default commodity -->
      {#if selectedAccount && inferredCommodityID === undefined}
        <label class="sm:col-span-2">
          <span class={labelClass}>{m.transactions_field_posting_commodity()}</span>
          <select
            value={simpleCommodityID ?? ''}
            onchange={(e) => { simpleCommodityID = Number((e.target as HTMLSelectElement).value) || undefined; }}
            class={inputClass}
            required
            disabled={isVoided && mode === 'edit'}
          >
            <option value="">—</option>
            {#each allActiveCommodities as c (c.id)}
              <option value={c.id}>{c.code}</option>
            {/each}
          </select>
        </label>
      {/if}

      <!-- Category (income/expense) with searchable dropdown -->
      <label class="relative sm:col-span-2">
        <span id={`${uid}-category-label`} class={labelClass}>{m.transactions_field_category()}</span>
        {#if categoryID !== undefined}
          <div class="mt-1.5 flex items-center gap-2">
            <span class="flex-1 truncate rounded-[var(--radius-control)] border border-border bg-control px-3 py-2.5 text-sm text-foreground">
              {categoryLabel}
            </span>
            <button
              type="button"
              class="inline-flex h-10 w-10 shrink-0 items-center justify-center rounded-[var(--radius-control)] border border-border bg-control text-muted transition hover:bg-control-hover hover:text-foreground"
              id={`${uid}-category-clear`}
              onclick={async () => {
                clearCategory();
                await tick();
                document.getElementById(`${uid}-category`)?.focus();
              }}
              aria-label={m.reports_filter_clear_dimension()}
              disabled={isVoided && mode === 'edit'}
            >
              <X size={14} aria-hidden="true" />
            </button>
          </div>
        {:else}
          <div class="relative mt-1.5">
            <input
              id={`${uid}-category`}
              type="text"
              aria-labelledby={`${uid}-category-label`}
              role="combobox"
              aria-autocomplete="list"
              aria-expanded={categoryDropdownOpen && filteredCategories.length > 0}
              aria-controls={categoryDropdownOpen && filteredCategories.length > 0 ? `${uid}-categories` : undefined}
              aria-activedescendant={categoryDropdownOpen && categoryActive >= 0 ? `${uid}-category-${categoryActive}` : undefined}
              bind:value={categorySearch}
              oninput={() => { categoryDropdownOpen = true; categoryActive = -1; }}
              onkeydown={categoryKeydown}
              onfocus={() => { categoryDropdownOpen = true; }}
              onblur={() => setTimeout(() => { categoryDropdownOpen = false; categoryActive = -1; }, 150)}
              class={inputClass + ' mt-0'}
              placeholder={m.transactions_field_category_placeholder()}
              autocomplete="off"
              disabled={isVoided && mode === 'edit'}
            />
            {#if categoryDropdownOpen && filteredCategories.length > 0}
              <ul
                id={`${uid}-categories`}
                class="absolute z-20 mt-1 max-h-52 w-full overflow-auto rounded-[var(--radius-control)] border border-border bg-surface text-sm shadow-[var(--shadow-panel)]"
                role="listbox"
                aria-label={m.transactions_field_category()}
              >
                {#each filteredCategories as cat, index (cat.id)}
                  <li role="none">
                    <button
                      type="button"
                      id={`${uid}-category-${index}`}
                      tabindex="-1"
                      class="flex w-full items-center gap-2 px-3 py-2 text-left hover:bg-row-hover aria-selected:bg-row-hover"
                      onmousedown={() => selectCategory(cat)}
                      role="option"
                      aria-selected={index === categoryActive}
                    >
                      <span class="min-w-0 truncate text-foreground">{categoryDisplayName(cat)}</span>
                      <span class="ml-auto shrink-0 text-xs text-muted">{cat.category_type === 'income' ? '↑' : '↓'}</span>
                    </button>
                  </li>
                {/each}
              </ul>
            {/if}
          </div>
        {/if}
      </label>
    </div>

    <!-- Switch to transfer or split; New transaction has its Transfer tab -->
    <div class="flex flex-wrap justify-end gap-x-4">
      {#if mode !== 'template' && !tabbed}
        <button
          type="button"
          class="min-h-10 text-xs font-medium text-accent underline-offset-2 hover:underline"
          onclick={switchToTransfer}
        >
          {m.transactions_tier_switch_transfer()}
        </button>
      {/if}
      <button
        type="button"
        class="min-h-10 text-xs font-medium text-accent underline-offset-2 hover:underline"
        onclick={switchToSplit}
      >
        {m.transactions_tier_switch_split()}
      </button>
    </div>
  {:else}
    <!-- ── Tier 3: split posting editor ────────────────────────────── -->
    <div class="space-y-3">
      {#if entryMessage}
        <p role="alert" class="flex items-start gap-2 rounded-[var(--radius-control)] border border-danger/40 bg-danger-soft px-3 py-2.5 text-sm text-foreground">
          <TriangleExclamationPoint size={15} class="mt-0.5 shrink-0 text-danger" aria-hidden="true" />
          <span>{entryMessage}</span>
        </p>
      {/if}
      <div class="flex items-center justify-between">
        <span class={labelClass}>{m.transactions_split_title()}</span>
        {#if splitIsEmpty || simpleFit}
          <button
            type="button"
            class="text-xs font-medium text-accent underline-offset-2 hover:underline"
            onclick={switchToSimple}
          >
            {m.transactions_tier_switch_simple()}
          </button>
        {/if}
      </div>

      {#each splitLegs as leg, i (i)}
        <div class="rounded-[var(--radius-control)] border border-border bg-surface px-3 py-3">
          <div class="grid gap-2 sm:grid-cols-[1fr_1fr_auto]">
            <!-- Account -->
            <label>
              <span class={labelClass}>{m.transactions_field_posting_account()}</span>
              <select
                value={leg.accountID}
                onchange={(e) => onSplitAccountChange(i, e)}
                class={inputClass}
              >
                <option value="">—</option>
                {#each allPostingAccounts as a (a.id)}
                  <option value={String(a.id)}>{postingAccountLabel(a)}</option>
                {/each}
              </select>
            </label>

            <!-- Amount -->
            <label>
              <span class={labelClass}>{m.transactions_field_posting_amount()}</span>
              <input
                type="text"
                value={leg.amountStr}
                oninput={(e) => { const ls = [...splitLegs]; ls[i] = { ...ls[i], amountStr: (e.target as HTMLInputElement).value }; splitLegs = ls; }}
                class={inputClass}
                inputmode="decimal"
                placeholder={formatEditableAmount('0', 2)}
              />
            </label>

            <!-- Remove button -->
            <div class="flex items-end pb-0.5">
              <button
                type="button"
                class="inline-flex h-10 w-10 items-center justify-center rounded-[var(--radius-control)] border border-border bg-control text-muted transition hover:bg-control-hover hover:text-danger"
                onclick={() => removeSplitLeg(i)}
                aria-label={m.transactions_split_remove_leg()}
                disabled={splitLegs.length <= 2}
              >
                <Trash size={14} aria-hidden="true" />
              </button>
            </div>

            {#if mode !== 'template' && hasPostingDay(accountForLeg(leg.accountID))}
              {#if leg.postedOn !== '' || splitPostedOnOpen[i]}
                <label class="sm:col-span-3">
                  <span class={labelClass}>{m.transactions_field_posted_on()}</span>
                  <input
                    type="date"
                    value={leg.postedOn || transactionDate}
                    oninput={(e) => setLegPostedOn(i, e.currentTarget.value)}
                    class={inputClass}
                  />
                </label>
              {:else}
                <div class="sm:col-span-3">
                  <button
                    type="button"
                    class="min-h-10 text-xs font-medium text-accent underline-offset-2 hover:underline"
                    onclick={() => { splitPostedOnOpen = { ...splitPostedOnOpen, [i]: true }; }}
                  >
                    {m.transactions_posted_on_toggle()}
                  </button>
                </div>
              {/if}
            {/if}

            <!-- A line on an account without a currency of its own shows the
                 one it takes, and the owner can change it (#511). -->
            {#if mode === 'template' || preservePostings || (leg.accountID !== '' && commodityForAccount(leg.accountID) === '')}
              {@const lineCommodity = splitCommodities[i] ?? ''}
              {@const missingCurrency = splitProblem?.line === i + 1 && splitProblem.missing === 'currency'}
              <label class="sm:col-span-3">
                <span class={labelClass}>{m.transactions_field_posting_commodity()}</span>
                <select class={inputClass} value={lineCommodity} aria-required="true" aria-invalid={missingCurrency && entryMessage !== '' ? 'true' : undefined} onchange={(e) => { const legs=[...splitLegs]; legs[i]={...legs[i],commodityID:e.currentTarget.value}; splitLegs=legs; }}>
                  {#if lineCommodity === ''}<option value="">{m.transactions_split_no_commodity()}</option>{/if}
                  {#each allActiveCommodities as commodity (commodity.id)}<option value={String(commodity.id)}>{commodity.code}</option>{/each}
                </select>
              </label>
            {/if}

            <!-- Memo (full width) -->
            <label class="sm:col-span-3">
              <span class={labelClass}>{m.transactions_field_posting_memo()}</span>
              <input
                type="text"
                value={leg.memo}
                oninput={(e) => { const ls = [...splitLegs]; ls[i] = { ...ls[i], memo: (e.target as HTMLInputElement).value }; splitLegs = ls; }}
                class={inputClass}
                maxlength="500"
              />
            </label>
          </div>
        </div>
      {/each}

      <div class="flex items-center justify-between">
        <button
          type="button"
          class="inline-flex items-center gap-2 rounded-[var(--radius-control)] border border-border bg-control px-3 py-2 text-sm font-semibold text-foreground transition hover:bg-control-hover"
          onclick={addSplitLeg}
        >
          <Plus size={14} aria-hidden="true" />
          {m.transactions_split_add_leg()}
        </button>

        <!-- Balance indicator -->
        {#if splitImbalance}
          <span class="text-xs font-medium text-warning">
            {m.transactions_split_balance_off({ amount: splitImbalance.amount, commodity: commodityCode(Number(splitImbalance.commodityID)) || splitImbalance.commodityID })}
          </span>
        {:else if splitLegs.some((l) => l.amountStr.trim())}
          <span class="text-xs font-medium text-positive">{m.transactions_split_balance_ok()}</span>
        {/if}
      </div>
    </div>
  {/if}

  <fieldset class="rounded-(--radius-control) border border-border p-3">
    <legend class="px-1 text-sm font-semibold">{m.recurring_tags()}</legend>
    {#if tagsQuery.isPending}<p class="text-sm text-muted">{m.recurring_loading()}</p>
    {:else if tagsQuery.isError}<APIFormError error={tagsQuery.error} /><button type="button" onclick={() => tagsQuery.refetch()}>{m.recurring_refresh()}</button>
    {:else if !tagsQuery.data?.tags.length}<p class="text-sm text-muted">{m.recurring_no_tags()}</p>
    {:else}<div class="flex flex-wrap gap-3">
      {#each tagsQuery.data.tags as tag (tag.id)}
        <label class="flex min-h-10 items-center gap-2 text-sm"><input type="checkbox" value={tag.id} bind:group={tagIDs} disabled={busy} />{tag.name}</label>
      {/each}
    </div>{/if}
  </fieldset>

  <!-- ── Tier 2: Advanced section ───────────────────────────────── -->
  <details class="rounded-[var(--radius-control)] border border-border bg-surface">
    <summary class="cursor-pointer px-3 py-2 text-sm font-semibold text-foreground">{m.form_advanced()}</summary>
    <div class="grid gap-3 border-t border-border px-3 py-3 sm:grid-cols-2">
      <!-- Transaction kind; a transfer is always one -->
      <label class:hidden={tier === 'transfer'}>
        <span class={labelClass}>{m.transactions_field_kind()}</span>
        <select bind:value={transactionKind} class={inputClass} disabled={isVoided && mode === 'edit'}>
          <option value="ordinary">{m.transaction_kind_ordinary()}</option>
          {#if mode === 'template' || preservePostings}
            <option value="transfer">{m.transaction_kind_transfer()}</option>
          {:else}
            <option value="adjustment">{m.transaction_kind_adjustment()}</option>
          {/if}
        </select>
      </label>

      <!-- External reference -->
      {#if mode !== 'template'}
      <label>
        <span class={labelClass}>{m.transactions_field_external_ref()}</span>
        <input
          type="text"
          bind:value={externalRefHint}
          class={inputClass}
          maxlength="500"
          disabled={isVoided && mode === 'edit'}
        />
      </label>

      {/if}
      <!-- Note (full width) -->
      <label class="sm:col-span-2">
        <span class={labelClass}>{m.transactions_field_note()}</span>
        <textarea
          bind:value={noteMarkdown}
          class={textAreaClass}
          maxlength="5000"
          disabled={isVoided && mode === 'edit'}
        ></textarea>
      </label>
    </div>
  </details>

  <!-- ── Action buttons ────────────────────────────────────────── -->
  <div class="flex flex-wrap items-center justify-end gap-2">
    <button
      type="button"
      class="inline-flex items-center gap-2 rounded-[var(--radius-control)] border border-border bg-control px-4 py-2.5 text-sm font-semibold text-foreground transition hover:bg-control-hover"
      onclick={requestCancel}
      disabled={busy}
    >
      <X size={16} aria-hidden="true" />
      {m.transactions_form_cancel()}
    </button>

    {#if !(isVoided && mode === 'edit') && !(tier === 'locked' && mode === 'correct')}
      <button
        type="submit"
        class="inline-flex items-center gap-2 rounded-[var(--radius-control)] bg-accent px-4 py-2.5 text-sm font-semibold text-accent-foreground transition hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-60"
        disabled={busy}
      >
        <Save size={16} aria-hidden="true" />
        {submitLabel}
      </button>
    {/if}
  </div>
  </fieldset>
  </form>

  {#if tabbed && entryTab === 'investment'}
    <!-- Existing investment commands own their financial fields and reviews. -->
    <div class="space-y-3">
      {#if investmentMuted(contextAccount)}
        <p class="text-sm text-foreground">{m.transactions_entry_investment_holding_needed()}</p>
      {/if}
      <p class="text-sm text-muted">{m.transactions_entry_investment_intro()}</p>
      <InvestmentEntry prefill={{ ...createDefaults?.investment, startingAccountID: accountID ?? createDefaults?.accountID, transactionDate, memo: description }}
        onSaved={(kind) => { leaving = true; onInvestmentSaved?.(kind); }} onCancel={requestCancel} />
    </div>
  {/if}
  </div>
</section>
