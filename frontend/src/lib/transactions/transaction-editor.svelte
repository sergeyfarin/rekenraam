<script lang="ts">
  import Plus from '@lucide/svelte/icons/plus';
  import Save from '@lucide/svelte/icons/save';
  import Trash2 from '@lucide/svelte/icons/trash-2';
  import X from '@lucide/svelte/icons/x';
  import AlertTriangle from '@lucide/svelte/icons/triangle-alert';
  import { createQuery } from '@tanstack/svelte-query';
  import APIFormError from '$lib/components/api-form-error.svelte';
  import { m } from '$lib/paraglide/messages.js';
  import { accountsQueryOptions } from '$lib/api/accounts';
  import { categoriesQueryOptions } from '$lib/api/categories';
  import { payeesQueryOptions } from '$lib/api/payees';
  import { currenciesQueryOptions, type CurrencyResponse } from '$lib/api/currencies';
  import {
    createTransaction,
    updateTransaction,
    correctTransaction,
    reconciliationImpactForCreate,
    reconciliationImpactForUpdate,
    type TransactionRequest,
    type TransactionResponse,
    type ReconciliationImpactResponse
  } from '$lib/api/transactions';
  import type { AccountResponse } from '$lib/api/accounts';
  import type { CategoryResponse } from '$lib/api/categories';
  import type { components } from '$lib/api/schema';

  type JournalEntryResponse = components['schemas']['JournalEntryResponse'];
  type PostingResponse = components['schemas']['PostingResponse'];
  type JournalEntryPostingRequest = NonNullable<
    NonNullable<TransactionRequest['journal_entries']>[number]['postings']
  >[number];
  import { categoryDisplayName } from '$lib/categories/category-labels';
  import { APIClientError } from '$lib/api/client';
  import {
    commodityImbalance,
    formatLedgerAmount,
    inflowPositiveAmount,
    negateCoefficient,
    parseDecimalAmount
  } from '$lib/money/amount';

  // ── Tier types ────────────────────────────────────────────────────
  type Tier = 'simple' | 'split';

  // A single editable posting leg in the split editor.
  type SplitLeg = {
    accountID: string;       // '' means unset
    commodityID: string;     // '' means unset
    amountStr: string;       // user-entered, e.g. "25.00" or "-25.00"
    memo: string;
  };

  // ── Props ─────────────────────────────────────────────────────────
  type EditorMode = 'create' | 'edit' | 'correct';

  let {
    mode,
    transaction,
    csrfToken,
    onSaved,
    onCancel
  } = $props<{
    mode: EditorMode;
    transaction?: TransactionResponse;
    csrfToken?: string;
    onSaved: (saved: TransactionResponse) => Promise<void> | void;
    onCancel: () => void;
  }>();

  // ── Queries ───────────────────────────────────────────────────────
  // Load accounts (asset/liability for the account leg; income/expense categories loaded separately)
  const accountsQuery = createQuery(() => accountsQueryOptions(false, false));
  const categoriesQuery = createQuery(() => categoriesQueryOptions());
  const currenciesQuery = createQuery(() => currenciesQueryOptions());

  // Payee autocomplete — only triggered when user types
  let payeeSearch = $state('');
  let payeeSearchDebounced = $state('');
  let payeeDebounceTimer: ReturnType<typeof setTimeout> | undefined;
  let payeeDropdownOpen = $state(false);
  let categoryDropdownOpen = $state(false);
  let categorySearch = $state('');

  function onPayeeInput(e: Event) {
    const val = (e.target as HTMLInputElement).value;
    payeeSearch = val;
    payeeID = undefined; // clear resolved ID when user edits
    clearTimeout(payeeDebounceTimer);
    payeeDebounceTimer = setTimeout(() => { payeeSearchDebounced = val; }, 250);
    payeeDropdownOpen = val.length > 0;
  }

  const payeesQuery = createQuery(() => payeesQueryOptions({ q: payeeSearchDebounced || undefined }));

  // ── Field state ───────────────────────────────────────────────────
  let tier = $state<Tier>('simple');

  // Tier 1 / shared fields
  let transactionDate = $state(todayISO());
  let payeeID = $state<number | undefined>(undefined);
  let description = $state('');

  // Tier 1: simple two-posting form
  let categoryID = $state<number | undefined>(undefined);
  let categoryLabel = $state('');
  let amountStr = $state(''); // user-entered amount string, e.g. "25.00"
  let accountID = $state<number | undefined>(undefined);
  // Manually chosen commodity when the selected account has no default_commodity_id.
  let simpleCommodityID = $state<number | undefined>(undefined);

  // Tier 2 fields (under Advanced)
  let transactionKind = $state<string>('ordinary');
  let noteMarkdown = $state('');
  let externalRefHint = $state('');

  // Tier 3: split legs
  let splitLegs = $state<SplitLeg[]>([
    { accountID: '', commodityID: '', amountStr: '', memo: '' },
    { accountID: '', commodityID: '', amountStr: '', memo: '' }
  ]);

  // ── Form state ────────────────────────────────────────────────────
  let pending = $state(false);
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
      (a: AccountResponse) =>
        (a.account_class === 'asset' || a.account_class === 'liability') &&
        a.status === 'active' &&
        a.allows_postings
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

  // For Tier 3 imbalance check — summed per commodity, scale-aware, in $lib/money.
  const splitImbalance = $derived.by(() => commodityImbalance(splitLegs));

  const title = $derived(
    mode === 'correct'
      ? m.transactions_form_correction_title()
      : mode === 'edit'
        ? m.transactions_form_edit_title()
        : m.transactions_form_create_title()
  );

  const submitLabel = $derived(
    pending
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
    if (!transaction) {
      transactionDate = todayISO();
      payeeID = undefined;
      payeeSearch = '';
      description = '';
      categoryID = undefined;
      categoryLabel = '';
      amountStr = '';
      accountID = undefined;
      simpleCommodityID = undefined;
      transactionKind = 'ordinary';
      noteMarkdown = '';
      externalRefHint = '';
      tier = 'simple';
      splitLegs = [
        { accountID: '', commodityID: '', amountStr: '', memo: '' },
        { accountID: '', commodityID: '', amountStr: '', memo: '' }
      ];
      formError = undefined;
      return;
    }

    transactionDate = transaction.transaction_date;
    payeeID = transaction.payee_id;
    payeeSearch = transaction.payee_name ?? '';
    description = transaction.description;
    transactionKind = transaction.transaction_kind ?? 'ordinary';
    noteMarkdown = transaction.note_markdown ?? '';
    externalRefHint = transaction.external_ref_hint ?? '';
    formError = undefined;

    // Determine tier based on journal entry complexity.
    const entries: JournalEntryResponse[] = transaction.journal_entries ?? [];
    const postings: PostingResponse[] = entries.flatMap((e: JournalEntryResponse) => e.postings ?? []);
    const nonSystemPostings: PostingResponse[] = postings.filter((p: PostingResponse) => !p.account_system_role);

    if (nonSystemPostings.length <= 2) {
      tier = 'simple';
      // Find the asset/liability leg and the income/expense leg.
      const assetLeg = nonSystemPostings.find(
        (p: PostingResponse) => p.account_class === 'asset' || p.account_class === 'liability'
      );
      const catLeg = nonSystemPostings.find(
        (p: PostingResponse) => p.account_class === 'income' || p.account_class === 'expense'
      );
      if (assetLeg) {
        accountID = assetLeg.account_id;
        simpleCommodityID = assetLeg.commodity_id;
        // Format the amount with correct sign: positive = inflow to account.
        amountStr = inflowPositiveAmount(assetLeg);
      }
      if (catLeg) {
        categoryID = catLeg.account_id;
        const allCats = (categoriesQuery.data?.categories ?? []);
        const cat = allCats.find((c: CategoryResponse) => c.id === catLeg.account_id);
        categoryLabel = cat ? categoryDisplayName(cat) : String(catLeg.account_id);
      }
    } else {
      tier = 'split';
      splitLegs = nonSystemPostings.map((p: PostingResponse) => ({
        accountID: String(p.account_id),
        commodityID: String(p.commodity_id),
        amountStr: formatLedgerAmount(p.quantity_value, p.quantity_scale),
        memo: p.memo ?? ''
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

    pending = true;
    formError = undefined;

    try {
      const payload = buildPayload();
      await submitPayload(payload, false);
    } catch (error) {
      if (isReconciliationOverrideRequired(error)) {
        // Fetch impact details and open modal.
        try {
          const payload = buildPayload();
          const impact = await fetchImpact(payload);
          reconciliationModal = { open: true, impacts: impact.affected_checkpoints, pendingPayload: payload };
        } catch (impactErr) {
          formError = impactErr;
        }
      } else {
        formError = error;
      }
    } finally {
      pending = false;
    }
  }

  async function confirmReconciliationOverride() {
    if (!csrfToken) return;
    reconciliationModal = { ...reconciliationModal, open: false };
    pending = true;
    formError = undefined;
    try {
      await submitPayload(reconciliationModal.pendingPayload, true);
    } catch (error) {
      formError = error;
    } finally {
      pending = false;
    }
  }

  async function submitPayload(payload: TransactionRequest, withOverride: boolean) {
    const finalPayload = withOverride
      ? { ...payload, reconciliation_override: true }
      : payload;

    let saved: TransactionResponse;

    if (mode === 'correct' && transaction) {
      saved = await correctTransaction(transaction.id, finalPayload, csrfToken!);
    } else if (mode === 'edit' && transaction) {
      saved = await updateTransaction(transaction.id, finalPayload, csrfToken!);
    } else {
      saved = await createTransaction(finalPayload, csrfToken!);
    }

    await onSaved(saved);
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
      status: 'posted',
      transaction_date: transactionDate || undefined,
      description: description.trim() || undefined,
      note_markdown: noteMarkdown.trim() || undefined,
      external_ref_hint: externalRefHint.trim() || undefined
    };

    if (transactionKind && transactionKind !== 'ordinary') {
      payload.transaction_kind = transactionKind as TransactionRequest['transaction_kind'];
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
        account_id: Number(leg.accountID),
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
  function todayISO(): string {
    return new Date().toISOString().slice(0, 10);
  }

  function isReconciliationOverrideRequired(error: unknown): boolean {
    return (
      error instanceof APIClientError &&
      error.code === 'CONFLICT' &&
      (error.detail?.includes('reconciliation override') ?? false)
    );
  }

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
    splitLegs = [...splitLegs, { accountID: '', commodityID: '', amountStr: '', memo: '' }];
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
          amountStr: formatLedgerAmount(assetLedger, scale),
          memo: ''
        },
        {
          accountID: categoryID ? String(categoryID) : '',
          commodityID: inferredCommodityID ? String(inferredCommodityID) : '',
          amountStr: formatLedgerAmount(negateCoefficient(assetLedger), scale),
          memo: ''
        }
      ];
    }
    tier = 'split';
  }

  function switchToSimple() {
    tier = 'simple';
  }

  // For split leg commodity options — all active account commodities from accounts list.
  const allActiveCommodities = $derived.by(() => {
    const seen = new Map<number, { id: number; code: string }>();
    for (const a of (accountsQuery.data?.accounts ?? [])) {
      if (a.default_commodity_id && !seen.has(a.default_commodity_id)) {
        const code = currenciesByID.get(a.default_commodity_id)?.code ?? String(a.default_commodity_id);
        seen.set(a.default_commodity_id, { id: a.default_commodity_id, code });
      }
    }
    return [...seen.values()];
  });

  // All accounts for split leg account selector.
  const allPostingAccounts = $derived(
    (accountsQuery.data?.accounts ?? []).filter(
      (a: AccountResponse) => a.status === 'active' && a.allows_postings
    )
  );

  // Commodity from a specific account (for split legs).
  function commodityForAccount(accIDStr: string): string {
    if (!accIDStr) return '';
    const acc = (accountsQuery.data?.accounts ?? []).find((a: AccountResponse) => a.id === Number(accIDStr));
    return acc?.default_commodity_id ? String(acc.default_commodity_id) : '';
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
  <div class="fixed inset-0 z-50 flex items-center justify-center bg-background/70 px-4 py-6 backdrop-blur-sm">
    <div
      class="w-full max-w-lg rounded-[var(--radius-panel)] border border-border bg-surface shadow-[var(--shadow-panel)]"
      role="alertdialog"
      aria-modal="true"
      aria-labelledby="recon-modal-title"
    >
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
          class="inline-flex items-center gap-2 rounded-[var(--radius-control)] bg-warning px-4 py-2.5 text-sm font-semibold text-warning-foreground transition hover:opacity-90 disabled:opacity-60"
          onclick={confirmReconciliationOverride}
          disabled={pending}
        >
          <AlertTriangle size={14} aria-hidden="true" />
          {m.transactions_reconciliation_confirm()}
        </button>
      </div>
    </div>
  </div>
{/if}

<form class="space-y-4" onsubmit={handleSubmit}>
  <!-- Header -->
  <div class="flex items-start justify-between gap-3">
    <div class="min-w-0">
      <h2 class="text-base font-semibold text-foreground">{title}</h2>
      {#if mode === 'edit' && transaction}
        <p class="mt-1 truncate text-sm text-muted">
          {transaction.payee_name || transaction.description || String(transaction.id)}
        </p>
      {/if}
    </div>
    <button
      type="button"
      class="inline-flex h-9 w-9 items-center justify-center rounded-[var(--radius-control)] border border-border bg-control text-foreground transition hover:bg-control-hover"
      onclick={onCancel}
      aria-label={m.transactions_form_cancel()}
      title={m.transactions_form_cancel()}
    >
      <X size={16} aria-hidden="true" />
    </button>
  </div>

  <!-- Correction banner -->
  {#if mode === 'correct'}
    <div class="flex items-start gap-2 rounded-[var(--radius-control)] border border-warning/40 bg-warning/10 px-3 py-2.5 text-sm text-warning-foreground">
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

  <APIFormError error={formError} id="transaction-form-error" />

  <!-- ── Tier 1 / shared fields ───────────────────────────────────── -->
  <div class="grid gap-3 sm:grid-cols-2">
    <!-- Date -->
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

    <!-- Payee autocomplete -->
    <label class="relative">
      <span class={labelClass}>{m.transactions_field_payee()}</span>
      <input
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
      {#if payeeDropdownOpen && (payeesQuery.data?.payees?.length ?? 0) > 0}
        <ul
          class="absolute z-20 mt-1 w-full rounded-[var(--radius-control)] border border-border bg-surface text-sm shadow-[var(--shadow-panel)]"
          role="listbox"
        >
          {#each payeesQuery.data?.payees ?? [] as payee (payee.id)}
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
            <option value={a.id}>{a.name ?? a.code ?? String(a.id)}</option>
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
          placeholder="0.00"
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
              aria-label="Clear category"
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
                  <option value={String(a.id)}>{a.name ?? a.code ?? String(a.id)}</option>
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
                placeholder="0.00"
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

  <!-- ── Tier 2: Advanced section ───────────────────────────────── -->
  <details class="rounded-[var(--radius-control)] border border-border bg-surface">
    <summary class="cursor-pointer px-3 py-2 text-sm font-semibold text-foreground">{m.form_advanced()}</summary>
    <div class="grid gap-3 border-t border-border px-3 py-3 sm:grid-cols-2">
      <!-- Transaction kind -->
      <label>
        <span class={labelClass}>{m.transactions_field_kind()}</span>
        <select bind:value={transactionKind} class={inputClass} disabled={isVoided && mode === 'edit'}>
          <option value="ordinary">{m.transaction_kind_ordinary()}</option>
          <option value="adjustment">{m.transaction_kind_adjustment()}</option>
        </select>
      </label>

      <!-- External reference -->
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
</form>
