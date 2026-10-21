<script lang="ts">
  import { useOwnerToday } from '#lib/dates/owner-date-context.ts';
  import { transactionTitle } from '#lib/transactions/transaction-title.ts';
  import { systemLabelText } from '#lib/transactions/system-label.ts';
  import { untrack, type Snippet } from 'svelte';
  import { tagsQueryOptions } from '#lib/api/tags.ts';
  import Plus from '@lucide/svelte/icons/plus';
  import Save from '@lucide/svelte/icons/save';
  import Trash2 from '@lucide/svelte/icons/trash-2';
  import X from '@lucide/svelte/icons/x';
  import AlertTriangle from '@lucide/svelte/icons/triangle-alert';
  import ModalDialog from '#lib/components/modal-dialog.svelte';
  import { createQuery, useQueryClient } from '@tanstack/svelte-query';
  import APIFormError from '#lib/components/api-form-error.svelte';
  import { isSubledgerManagedAccountKind } from '#lib/accounts/subledger-accounts.ts';
  import { isOrdinaryEntryAccount } from './entry-account';
  import { editablePostings, hasPostingDay, postedOnForRequest } from './posted-on';
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
    inflowPositiveAmount,
    negateCoefficient,
    parseDecimalAmount
  } from '#lib/money/amount.ts';

  const ownerToday = useOwnerToday();

  // ── Tier types ────────────────────────────────────────────────────
  type Tier = 'simple' | 'split';

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
    preservePostings = false
  }: {
    mode: EditorMode;
    transaction?: TransactionResponse;
    csrfToken?: string;
    onSaved?: (saved: TransactionResponse) => Promise<void> | void;
    initialValues?: TransactionRequest;
    /** Pre-selects the account or category a new transaction starts from,
     *  when it is entered from that account's or category's screen (#185). */
    createDefaults?: { accountID?: number; categoryID?: number };
    templateFields?: Snippet;
    saveTemplate?: (payload: TransactionRequest) => Promise<void>;
    preservePostings?: boolean;
    onPendingChange?: (pending: boolean) => void;
    onCancel: () => void;
  } = $props();

  // A save refused because the transaction changed elsewhere reloads it here,
  // so the form shows the newer version instead of re-sending a stale one
  // (#296). A newer version handed down by the parent wins.
  let reloaded = $state<TransactionResponse | undefined>(undefined);
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

  function onPayeeInput(e: Event) {
    const val = (e.target as HTMLInputElement).value;
    payeeSearch = val;
    payeeID = undefined; // clear resolved ID when user edits
    payeeDropdownOpen = val.length > 0;
  }

  const payeesQuery = createQuery(() => payeesQueryOptions({}));
  const allPayees = $derived<PayeeResponse[]>(payeesQuery.data?.payees ?? []);
  const payeeMatches = $derived(payeeSuggestions(payeeSearch, allPayees));
  const payeeNameChanged = $derived(payeeSearch.trim() !== initialPayeeName.trim());

  // ── Field state ───────────────────────────────────────────────────
  let tier = $state<Tier>('simple');

  // Tier 1 / shared fields
  let transactionDate = $state(ownerToday());
  let payeeID = $state<number | undefined>(undefined);
  let description = $state('');

  // Tier 1: simple two-posting form
  let categoryID = $state<number | undefined>(undefined);
  let categoryLabel = $state('');
  let amountStr = $state(''); // user-entered amount string, e.g. "25.00"
  let accountID = $state<number | undefined>(undefined);
  // The day the account posting posts on, '' when it follows the date (#187).
  let accountPostedOn = $state('');
  let showAccountPostedOn = $state(false);
  // Split legs whose posted-on field the user opened (by leg index).
  let splitPostedOnOpen = $state<Record<number, boolean>>({});
  // Manually chosen commodity when the selected account has no default_commodity_id.
  let simpleCommodityID = $state<number | undefined>(undefined);

  // Tier 2 fields (under Advanced)
  let transactionKind = $state<string>('ordinary');
  let noteMarkdown = $state('');
  let externalRefHint = $state('');

  // Tier 3: split legs
  let splitLegs = $state<SplitLeg[]>([
    { accountID: '', commodityID: '', amountStr: '', memo: '', postedOn: '' },
    { accountID: '', commodityID: '', amountStr: '', memo: '', postedOn: '' }
  ]);

  // ── Form state ────────────────────────────────────────────────────
  let pending = $state(false);
  $effect(() => { onPendingChange?.(pending); });
  let formError = $state<unknown>(undefined);

  // Reconciliation override modal
  let reconciliationModal = $state<{
    open: boolean;
    impacts: ReconciliationImpactResponse['affected_checkpoints'];
    pendingPayload: TransactionRequest;
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

  const currenciesByID = $derived(
    new Map<number, CurrencyResponse>(
      (currenciesQuery.data?.currencies ?? []).map((c: CurrencyResponse) => [c.id, c])
    )
  );

  // For Tier 3 imbalance check — summed per commodity, scale-aware, in #lib/money.
  const splitImbalance = $derived.by(() => commodityImbalance(splitLegs));

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

  // ── Initialise from existing transaction (edit mode) ──────────────
  $effect(() => {
    tagIDs = [...(transaction?.tag_ids ?? initialValues?.tag_ids ?? [])];
    splitPostedOnOpen = {};
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
      accountPostedOn = '';
      showAccountPostedOn = false;
      simpleCommodityID = undefined;
      transactionKind = 'ordinary';
      noteMarkdown = '';
      externalRefHint = '';
      tier = 'simple';
      splitLegs = [
        { accountID: '', commodityID: '', amountStr: '', memo: '', postedOn: '' },
        { accountID: '', commodityID: '', amountStr: '', memo: '', postedOn: '' }
      ];
      formError = undefined;
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

    // The simple form is one account and one category; anything else (a
    // transfer between two accounts included) opens as splits.
    if (!preservePostings && shown.length <= 2 && assetLegs.length <= 1 && categoryLegs.length <= 1 && assetLegs.length + categoryLegs.length === shown.length) {
      tier = 'simple';
      const assetLeg = assetLegs[0];
      const catLeg = categoryLegs[0]?.posting;
      if (assetLeg) {
        accountID = assetLeg.posting.account_id;
        simpleCommodityID = assetLeg.posting.commodity_id;
        // Format the amount with correct sign: positive = inflow to account.
        amountStr = inflowPositiveAmount(assetLeg.posting);
        accountPostedOn = assetLeg.postedOn;
        showAccountPostedOn = assetLeg.postedOn !== '';
      }
      if (catLeg) {
        categoryID = catLeg.account_id;
        const allCats = (categoriesQuery.data?.categories ?? []);
        const cat = allCats.find((c: CategoryResponse) => c.id === catLeg.account_id);
        categoryLabel = cat ? categoryDisplayName(cat) : String(catLeg.account_id);
      }
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
          reconciliationModal = { open: true, impacts: impact.affected_checkpoints, pendingPayload: payload };
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
    await savePayee();
  }

  function editPayeeName() {
    payeeConfirm = { open: false, resolution: null };
    payeeDropdownOpen = false;
    document.getElementById(payeeInputID)?.focus();
  }

  async function confirmReconciliationOverride() {
    if (!csrfToken) return;
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

    if (tier === 'simple') {
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

    // Convert user-entered display amount to ledger coefficient.
    // User enters inflow-positive relative to the asset/liability account.
    const parsed = parseDecimalAmount(amountStr);
    if (parsed === null) return [];
    const { value: assetValue, scale } = parsed;

    // Ledger sign: for asset → positive value = debit = inflow; credit = outflow.
    // For liability → positive debit = outflow; so we flip.
    const assetLedgerValue = account.account_class === 'liability' ? negateCoefficient(assetValue) : assetValue;
    // Category posting is the balancing leg — opposite sign.
    const catLedgerValue = negateCoefficient(assetLedgerValue);

    return [
      {
        entry_date: transactionDate,
        postings: [
          {
            account_id: accountID,
            posted_on: mode === 'template' ? undefined : postedOnForRequest(accountPostedOn, transactionDate, account),
            commodity_id: effectiveCommodityID,
            quantity_value: assetLedgerValue,
            quantity_scale: scale,
            memo: ''
          },
          {
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

  function buildSplitJournalEntries(): TransactionRequest['journal_entries'] {
    const legs = splitLegs.filter(
      (leg) => leg.accountID !== '' && leg.commodityID !== '' && leg.amountStr.trim() !== ''
    );

    const postings: JournalEntryPostingRequest[] = [];
    for (const leg of legs) {
      const parsed = parseDecimalAmount(leg.amountStr);
      // A leg the user filled in but we cannot parse must not silently become a
      // zero posting — refuse to build the payload, as the simple tier does.
      if (parsed === null) return [];
      postings.push({
        line_key: leg.lineKey,
        account_id: Number(leg.accountID),
        posted_on: mode === 'template' ? undefined : postedOnForRequest(leg.postedOn, transactionDate, accountForLeg(leg.accountID)),
        commodity_id: Number(leg.commodityID),
        quantity_value: parsed.value,
        quantity_scale: parsed.scale,
        memo: leg.memo.trim()
      });
    }

    if (postings.length === 0) return [];

    return [{ entry_date: transactionDate, postings }];
  }

  // ── Helpers ───────────────────────────────────────────────────────

  function selectCategory(cat: CategoryResponse) {
    categoryID = cat.id;
    categoryLabel = categoryDisplayName(cat);
    categorySearch = '';
    categoryDropdownOpen = false;
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
    splitLegs = splitLegs.filter((_, idx) => idx !== i);
  }

  function switchToSplit() {
    // Copy existing simple fields into split legs.
    const parsed = accountID && amountStr ? parseDecimalAmount(amountStr) : null;
    if (parsed !== null) {
      const { value, scale } = parsed;
      const account = assetLiabilityAccounts.find((a: AccountResponse) => a.id === accountID);
      const assetLedger = account?.account_class === 'liability' ? negateCoefficient(value) : value;
      splitLegs = [
        {
          accountID: String(accountID),
          commodityID: inferredCommodityID ? String(inferredCommodityID) : '',
          amountStr: formatEditableAmount(assetLedger, scale),
          memo: '',
          postedOn: accountPostedOn
        },
        {
          accountID: categoryID ? String(categoryID) : '',
          commodityID: inferredCommodityID ? String(inferredCommodityID) : '',
          amountStr: formatEditableAmount(negateCoefficient(assetLedger), scale),
          memo: '',
          postedOn: ''
        }
      ];
    }
    tier = 'split';
  }

  function switchToSimple() {
    tier = 'simple';
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
    newLegs[i] = { ...newLegs[i], accountID: val, commodityID: commodityForAccount(val) };
    splitLegs = newLegs;
  }
</script>

<!-- Reconciliation warning modal -->
{#if reconciliationModal.open}
  <ModalDialog role="alertdialog" labelledby="recon-modal-title" {pending}
    onCancel={() => (reconciliationModal = { ...reconciliationModal, open: false })} class="max-w-lg">
    <div class="flex items-start gap-3 border-b border-border px-4 py-3">
      <AlertTriangle size={18} class="mt-0.5 shrink-0 text-warning" aria-hidden="true" />
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
        <AlertTriangle size={14} aria-hidden="true" />
        {m.transactions_reconciliation_confirm()}
      </button>
    </div>
  </ModalDialog>
{/if}

<form class="space-y-4" onsubmit={handleSubmit} aria-busy={pending}>
  <!-- Header -->
  <div class="flex items-start justify-between gap-3">
    <div class="min-w-0">
      <h2 class="text-base font-semibold text-foreground">{title}</h2>
      {#if mode === 'edit' && transaction}
        <p class="mt-1 truncate text-sm text-muted">
          {transactionTitle(transaction, systemLabelText, String(transaction.id))}
        </p>
      {/if}
    </div>
    <button
      type="button"
      class="inline-flex h-9 w-9 items-center justify-center rounded-[var(--radius-control)] border border-border bg-control text-foreground transition hover:bg-control-hover"
      onclick={onCancel}
      disabled={pending}
      aria-label={m.transactions_form_cancel()}
      title={m.transactions_form_cancel()}
    >
      <X size={16} aria-hidden="true" />
    </button>
  </div>

  <fieldset disabled={pending} class="space-y-4">
  {#if templateFields}{@render templateFields()}{/if}

  <!-- Correction banner -->
  {#if mode === 'correct'}
    <div class="flex items-start gap-2 rounded-[var(--radius-control)] border border-warning/40 bg-warning-soft px-3 py-2.5 text-sm text-foreground">
      <AlertTriangle size={15} class="mt-0.5 shrink-0" aria-hidden="true" />
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
          disabled={isVoided && mode === 'edit'}
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
      <span class={labelClass}>{m.transactions_field_payee()}</span>
      <input
        id={payeeInputID}
        type="text"
        value={payeeSearch}
        oninput={onPayeeInput}
        onfocus={() => { if (payeeSearch) payeeDropdownOpen = true; }}
        onblur={() => setTimeout(() => { payeeDropdownOpen = false; }, 150)}
        class={inputClass}
        placeholder={m.transactions_field_payee_placeholder()}
        autocomplete="off"
        disabled={isVoided && mode === 'edit'}
      />
      {#if payeeDropdownOpen && payeeMatches.length > 0}
        <ul
          class="absolute z-20 mt-1 w-full rounded-[var(--radius-control)] border border-border bg-surface text-sm shadow-[var(--shadow-panel)]"
          role="listbox"
        >
          {#each payeeMatches as payee (payee.id)}
            <li>
              <button
                type="button"
                class="w-full px-3 py-2 text-left hover:bg-row-hover"
                onmousedown={() => {
                  payeeID = payee.id;
                  payeeSearch = payee.name;
                  payeeDropdownOpen = false;
                }}
                role="option"
                aria-selected={payeeID === payee.id}
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
            class="rounded-(--radius-control) bg-foreground px-3 py-1.5 text-sm font-semibold text-background transition hover:opacity-90 disabled:opacity-60"
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

  {#if tier === 'simple'}
    <!-- ── Tier 1: simple two-posting form ─────────────────────────── -->
    <div class="grid gap-3 sm:grid-cols-2">
      <!-- Account (asset/liability leg) -->
      <label>
        <span class={labelClass}>{m.transactions_field_account()}</span>
        <select
          value={accountID ?? ''}
          onchange={(e) => {
            accountID = Number((e.target as HTMLSelectElement).value) || undefined;
            simpleCommodityID = undefined;
          }}
          class={inputClass}
          disabled={isVoided && mode === 'edit'}
        >
          <option value="">—</option>
          {#each assetLiabilityAccounts as a (a.id)}
            <option value={a.id}>{postingAccountLabel(a)}</option>
          {/each}
        </select>
      </label>

      <!-- Amount (inflow positive relative to selected account) -->
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

      <!-- Commodity selector — only shown when the selected account has no default commodity -->
      {#if accountID !== undefined && inferredCommodityID === undefined}
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
        <span class={labelClass}>{m.transactions_field_category()}</span>
        {#if categoryID !== undefined}
          <div class="mt-1.5 flex items-center gap-2">
            <span class="flex-1 truncate rounded-[var(--radius-control)] border border-border bg-control px-3 py-2.5 text-sm text-foreground">
              {categoryLabel}
            </span>
            <button
              type="button"
              class="inline-flex h-10 w-10 shrink-0 items-center justify-center rounded-[var(--radius-control)] border border-border bg-control text-muted transition hover:bg-control-hover hover:text-foreground"
              onclick={clearCategory}
              aria-label={m.reports_filter_clear_dimension()}
              disabled={isVoided && mode === 'edit'}
            >
              <X size={14} aria-hidden="true" />
            </button>
          </div>
        {:else}
          <div class="relative mt-1.5">
            <input
              type="text"
              bind:value={categorySearch}
              onfocus={() => { categoryDropdownOpen = true; }}
              onblur={() => setTimeout(() => { categoryDropdownOpen = false; }, 150)}
              class={inputClass + ' mt-0'}
              placeholder={m.transactions_field_category_placeholder()}
              autocomplete="off"
              disabled={isVoided && mode === 'edit'}
            />
            {#if categoryDropdownOpen && filteredCategories.length > 0}
              <ul
                class="absolute z-20 mt-1 max-h-52 w-full overflow-auto rounded-[var(--radius-control)] border border-border bg-surface text-sm shadow-[var(--shadow-panel)]"
                role="listbox"
              >
                {#each filteredCategories as cat (cat.id)}
                  <li>
                    <button
                      type="button"
                      class="flex w-full items-center gap-2 px-3 py-2 text-left hover:bg-row-hover"
                      onmousedown={() => selectCategory(cat)}
                      role="option"
                      aria-selected={false}
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

    <!-- Switch to split -->
    <div class="flex justify-end">
      <button
        type="button"
        class="text-xs font-medium text-accent underline-offset-2 hover:underline"
        onclick={switchToSplit}
      >
        {m.transactions_tier_switch_split()}
      </button>
    </div>
  {:else}
    <!-- ── Tier 3: split posting editor ────────────────────────────── -->
    <div class="space-y-3">
      <div class="flex items-center justify-between">
        <span class={labelClass}>{m.transactions_split_title()}</span>
        <button
          type="button"
          class="text-xs font-medium text-accent underline-offset-2 hover:underline"
          onclick={switchToSimple}
        >
          {m.transactions_tier_switch_simple()}
        </button>
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
                <Trash2 size={14} aria-hidden="true" />
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

            {#if mode === 'template' || preservePostings}
              <label class="sm:col-span-3">
                <span class={labelClass}>{m.transactions_field_posting_commodity()}</span>
                <select class={inputClass} value={leg.commodityID} required onchange={(e) => { const legs=[...splitLegs]; legs[i]={...legs[i],commodityID:e.currentTarget.value}; splitLegs=legs; }}>
                  <option value="">—</option>
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
            {m.transactions_split_balance_off({ amount: splitImbalance.amount, commodity: splitImbalance.commodityID })}
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
        <label class="flex min-h-10 items-center gap-2 text-sm"><input type="checkbox" value={tag.id} bind:group={tagIDs} disabled={pending} />{tag.name}</label>
      {/each}
    </div>{/if}
  </fieldset>

  <!-- ── Tier 2: Advanced section ───────────────────────────────── -->
  <details class="rounded-[var(--radius-control)] border border-border bg-surface">
    <summary class="cursor-pointer px-3 py-2 text-sm font-semibold text-foreground">{m.form_advanced()}</summary>
    <div class="grid gap-3 border-t border-border px-3 py-3 sm:grid-cols-2">
      <!-- Transaction kind -->
      <label>
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
      onclick={onCancel}
      disabled={pending}
    >
      <X size={16} aria-hidden="true" />
      {m.transactions_form_cancel()}
    </button>

    {#if !(isVoided && mode === 'edit')}
      <button
        type="submit"
        class="inline-flex items-center gap-2 rounded-[var(--radius-control)] bg-foreground px-4 py-2.5 text-sm font-semibold text-background transition hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-60"
        disabled={pending}
      >
        <Save size={16} aria-hidden="true" />
        {submitLabel}
      </button>
    {/if}
  </div>
  </fieldset>
</form>
