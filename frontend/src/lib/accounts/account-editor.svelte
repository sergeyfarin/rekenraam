<script lang="ts">
  import Plus from '@lucide/svelte/icons/plus';
  import Save from '@lucide/svelte/icons/save';
  import X from '@lucide/svelte/icons/x';
  import APIFormError from '$lib/components/api-form-error.svelte';
  import {
    createAccountAdvanced,
    updateAccount,
    type AccountRequest,
    type AccountResponse
  } from '$lib/api/accounts';
  import type { CurrencyResponse } from '$lib/api/currencies';
  import type { InstitutionResponse } from '$lib/api/institutions';
  import { m } from '$lib/paraglide/messages.js';
  import { deriveRecordCode } from '$lib/record-code';
  import type { LocalizedCurrencyCatalogEntry } from '$lib/install-gate/currency-options';
  import {
    accountDisplayName,
    accountKindClass,
    accountKindGroups,
    accountKindLabel,
    type ManagedAccountClass,
    type AccountKind
  } from './account-labels';

  type AccountEditorMode = 'create' | 'edit';

  let {
    mode,
    account,
    accounts,
    institutions,
    currencies,
    currencyCatalog,
    defaultCurrencyCommodityID,
    csrfToken,
    onSaved,
    onCancel,
    onQuickInstitution
  } = $props<{
    mode: AccountEditorMode;
    account?: AccountResponse;
    accounts: AccountResponse[];
    institutions: InstitutionResponse[];
    currencies: CurrencyResponse[];
    currencyCatalog: LocalizedCurrencyCatalogEntry[];
    defaultCurrencyCommodityID?: number;
    csrfToken?: string;
    onSaved: () => Promise<void> | void;
    onCancel: () => void;
    onQuickInstitution: () => void;
  }>();

  const inputClass =
    'mt-1.5 h-10 w-full rounded-[var(--radius-control)] border border-border bg-control px-3 text-sm text-foreground shadow-sm outline-none transition placeholder:text-muted hover:bg-control-hover focus:border-accent disabled:cursor-not-allowed disabled:opacity-70';
  const textAreaClass =
    'mt-1.5 min-h-20 w-full rounded-[var(--radius-control)] border border-border bg-control px-3 py-2 text-sm text-foreground shadow-sm outline-none transition placeholder:text-muted hover:bg-control-hover focus:border-accent';
  const labelClass = 'block text-xs font-semibold uppercase tracking-[0.12em] text-muted';
  const otherCurrencyOptionValue = '__other_currency__';

  let accountID = $state<number | undefined>(undefined);
  let name = $state('');
  let code = $state('');
  let accountKind = $state<AccountKind>('checking');
  const accountClass = $derived(accountKindClass(accountKind));
  let parentAccountID = $state('');
  let institutionID = $state('');
  let countryCode = $state('');
  let defaultCommodityID = $state('');
  let defaultCommodityCode = $state('');
  let allowsPostings = $state(true);
  let numberLast4 = $state('');
  let openedOn = $state('');
  let effectiveFrom = $state('');
  let changeReason = $state('');
  let commentMarkdown = $state('');
  let pending = $state(false);
  let formError = $state<unknown>(undefined);
  let codeEdited = $state(false);
  let currencyDialogOpen = $state(false);
  let currencySearch = $state('');

  const parentOptions = $derived.by(() => {
    return accounts.filter(
      (candidate: AccountResponse) =>
        candidate.status !== 'archived' &&
        candidate.account_class === accountClass &&
        candidate.id !== accountID &&
        !candidate.is_system
    );
  });
  const selectedParent = $derived.by(() => {
    return parentOptions.find((parent: AccountResponse) => String(parent.id) === parentAccountID);
  });
  const selectedParentInstitution = $derived.by(() => {
    if (!selectedParent?.institution_id) {
      return undefined;
    }

    return institutions.find((institution: InstitutionResponse) => institution.id === selectedParent.institution_id);
  });
  const activeInstitutions = $derived.by(() => {
    return institutions.filter(
      (institution: InstitutionResponse) =>
        institution.status === 'active' ||
        institution.id === account?.institution_id ||
        institution.id === selectedParent?.institution_id
    );
  });
  const accountTypeGroups = $derived(accountKindGroups());
  const selectableAccountKinds = $derived(accountTypeGroups.flatMap((group) => group.kinds));
  const currencyRequired = $derived(accountKindRequiresDefaultCurrency(accountClass, accountKind));
  const currencyOptions = $derived.by<CurrencyResponse[]>(() => {
    const currencyByID = new Map<number, CurrencyResponse>();
    for (const currency of currencies) {
      currencyByID.set(currency.id, currency);
    }
    const ids: number[] = [];

    addCurrencyID(ids, currencyByID, defaultCurrencyCommodityID);
    for (const candidate of accounts) {
      if (candidate.status === 'active') {
        addCurrencyID(ids, currencyByID, candidate.default_commodity_id);
      }
    }
    addCurrencyID(ids, currencyByID, account?.default_commodity_id);
    addCurrencyID(ids, currencyByID, defaultCommodityID === '' ? undefined : Number(defaultCommodityID));

    return ids
      .map((id) => currencyByID.get(id))
      .filter((currency: CurrencyResponse | undefined): currency is CurrencyResponse => currency !== undefined);
  });
  const selectedCatalogCurrency = $derived.by(() => {
    if (defaultCommodityCode === '') {
      return undefined;
    }

    return currencyCatalog.find((currency: LocalizedCurrencyCatalogEntry) => currency.code === defaultCommodityCode);
  });
  const currencyPlaceholder = $derived(
    selectedCatalogCurrency
      ? m.accounts_field_pending_currency({ code: selectedCatalogCurrency.code, name: selectedCatalogCurrency.name })
      : currencyRequired
        ? m.accounts_field_choose_currency()
        : m.accounts_field_no_currency()
  );
  const filteredCurrencyCatalog = $derived.by(() => {
    const query = currencySearch.trim().toLocaleLowerCase();
    const values = query
      ? currencyCatalog.filter((currency: LocalizedCurrencyCatalogEntry) =>
          [currency.code, currency.name, currency.english_name].join(' ').toLocaleLowerCase().includes(query)
        )
      : currencyCatalog;

    return values.slice(0, 30);
  });
  const institutionLockedByParent = $derived(!!selectedParent);
  const institutionInheritanceHint = $derived.by(() => {
    if (!selectedParent) {
      return '';
    }

    if (!selectedParent.institution_id) {
      return m.accounts_inherited_no_institution_hint({ parent: accountDisplayName(selectedParent) });
    }

    return m.accounts_inherited_institution_hint({
      parent: accountDisplayName(selectedParent),
      institution: selectedParentInstitution?.name ?? m.accounts_group_unknown_institution()
    });
  });
  const title = $derived(mode === 'edit' ? m.accounts_form_edit_title() : m.accounts_form_create_title());
  const submitLabel = $derived(
    pending
      ? mode === 'edit'
        ? m.accounts_form_save_pending()
        : m.accounts_form_create_pending()
      : mode === 'edit'
        ? m.accounts_form_save()
        : m.accounts_form_create()
  );

  $effect(() => {
    accountID = account?.id;
    name = account?.name ?? '';
    code = account?.code ?? deriveRecordCode(account?.name ?? '');
    codeEdited = mode === 'edit' && !!account?.code;
    const nextAccountKind = managedAccountKind(account?.account_kind);
    const nextAccountClass = accountKindClass(nextAccountKind);
    accountKind = nextAccountKind;
    parentAccountID = account?.parent_account_id ? String(account.parent_account_id) : '';
    institutionID = account?.institution_id ? String(account.institution_id) : '';
    countryCode = account?.country_code ?? '';
    defaultCommodityID = account?.default_commodity_id
      ? String(account.default_commodity_id)
      : defaultCurrencyID(nextAccountClass, nextAccountKind, currencies, defaultCurrencyCommodityID);
    defaultCommodityCode = '';
    allowsPostings = account?.allows_postings ?? true;
    numberLast4 = account?.number_last4 ?? '';
    openedOn = account?.opened_on ?? '';
    effectiveFrom = mode === 'edit' ? '' : (account?.effective_from ?? '');
    changeReason = '';
    commentMarkdown = account?.comment_markdown ?? '';
    formError = undefined;
  });

  $effect(() => {
    if (!selectableAccountKinds.includes(accountKind)) {
      accountKind = defaultAccountKind();
    }
  });

  $effect(() => {
    if (mode !== 'create') {
      return;
    }

    if (currencyRequired) {
      if (defaultCommodityID === '' && defaultCommodityCode === '') {
        defaultCommodityID = defaultCurrencyID(accountClass, accountKind, currencies, defaultCurrencyCommodityID);
      }
    } else {
      defaultCommodityID = '';
      defaultCommodityCode = '';
    }
  });

  $effect(() => {
    if (parentAccountID !== '' && !parentOptions.some((parent: AccountResponse) => String(parent.id) === parentAccountID)) {
      parentAccountID = '';
    }
  });

  $effect(() => {
    if (selectedParent) {
      institutionID = selectedParent.institution_id ? String(selectedParent.institution_id) : '';
    }
  });

  async function handleSubmit(event: SubmitEvent) {
    event.preventDefault();

    if (!csrfToken) {
      formError = new Error(m.accounts_form_missing_session());
      return;
    }

    pending = true;
    formError = undefined;

    try {
      const input = requestFromForm();

      if (mode === 'edit' && accountID) {
        await updateAccount(accountID, input, csrfToken);
      } else {
        await createAccountAdvanced(input, csrfToken);
      }

      await onSaved();
    } catch (error) {
      formError = error;
    } finally {
      pending = false;
    }
  }

  function requestFromForm(): AccountRequest {
    const request: AccountRequest = {
      name: name.trim(),
      account_class: accountClass,
      account_kind: accountKind,
      allows_postings: allowsPostings
    };

    assignString(request, 'code', code);
    assignNumber(request, 'parent_account_id', parentAccountID);
    assignNumber(request, 'institution_id', institutionID);
    assignString(request, 'country_code', countryCode.toUpperCase());
    assignNumber(request, 'default_commodity_id', defaultCommodityID);
    assignString(request, 'default_commodity_code', defaultCommodityCode);
    assignString(request, 'number_last4', numberLast4);
    assignString(request, 'opened_on', openedOn);
    assignString(request, 'effective_from', effectiveFrom);
    assignString(request, 'change_reason', changeReason);
    assignString(request, 'comment_markdown', commentMarkdown);

    return request;
  }

  function handleNameInput(event: Event) {
    name = event.currentTarget instanceof HTMLInputElement ? event.currentTarget.value : '';
    if (!codeEdited) {
      code = deriveRecordCode(name);
    }
  }

  function handleCodeInput(event: Event) {
    code = event.currentTarget instanceof HTMLInputElement ? event.currentTarget.value : '';
    codeEdited = true;
  }

  function handleCurrencySelect(event: Event) {
    if (!(event.currentTarget instanceof HTMLSelectElement)) {
      return;
    }

    if (event.currentTarget.value === otherCurrencyOptionValue) {
      event.currentTarget.value = defaultCommodityID;
      currencyDialogOpen = true;
      return;
    }

    defaultCommodityID = event.currentTarget.value;
    defaultCommodityCode = '';
  }

  function chooseCatalogCurrency(currency: LocalizedCurrencyCatalogEntry) {
    defaultCommodityID = '';
    defaultCommodityCode = currency.code;
    currencyDialogOpen = false;
    currencySearch = '';
  }

  function assignString<T extends Record<string, unknown>>(target: T, key: keyof T, value: string) {
    const trimmed = value.trim();
    if (trimmed !== '') {
      target[key] = trimmed as T[keyof T];
    }
  }

  function assignNumber<T extends Record<string, unknown>>(target: T, key: keyof T, value: string) {
    if (value !== '') {
      target[key] = Number(value) as T[keyof T];
    }
  }

  function addCurrencyID(ids: number[], currencyByID: ReadonlyMap<number, CurrencyResponse>, value?: number) {
    if (!value || !currencyByID.has(value) || ids.includes(value)) {
      return;
    }

    ids.push(value);
  }

  function defaultCurrencyID(
    accountClassValue: ManagedAccountClass,
    accountKindValue: AccountKind,
    values: CurrencyResponse[],
    preferredID?: number
  ): string {
    if (!accountKindRequiresDefaultCurrency(accountClassValue, accountKindValue)) {
      return '';
    }

    if (preferredID && values.some((currency: CurrencyResponse) => currency.id === preferredID)) {
      return String(preferredID);
    }

    return values[0]?.id ? String(values[0].id) : '';
  }

  function defaultAccountKind(): AccountKind {
    return 'checking';
  }

  function managedAccountKind(value?: AccountKind): AccountKind {
    if (value && accountKindGroups().some((group) => group.kinds.includes(value))) {
      return value;
    }

    return defaultAccountKind();
  }

  function accountKindRequiresDefaultCurrency(accountClassValue: ManagedAccountClass, accountKindValue: AccountKind): boolean {
    if (accountClassValue !== 'asset' && accountClassValue !== 'liability') {
      return false;
    }

    return accountKindValue !== 'receivable' && accountKindValue !== 'payable';
  }

</script>

<form class="space-y-4" onsubmit={handleSubmit}>
  <div class="flex items-start justify-between gap-3">
    <div class="min-w-0">
      <h2 class="text-base font-semibold text-foreground">{title}</h2>
      {#if mode === 'edit' && account}
        <p class="mt-1 truncate text-sm text-muted">{accountDisplayName(account)}</p>
      {/if}
    </div>
    <button
      type="button"
      class="inline-flex h-9 w-9 items-center justify-center rounded-[var(--radius-control)] border border-border bg-control text-foreground transition hover:bg-control-hover"
      onclick={onCancel}
      aria-label={m.accounts_form_cancel()}
      title={m.accounts_form_cancel()}
    >
      <X size={16} aria-hidden="true" />
    </button>
  </div>

  <APIFormError error={formError} id="account-form-error" />

  <div class="grid gap-3 sm:grid-cols-2">
    <label>
      <span class={labelClass}>{m.accounts_field_name()}</span>
      <input value={name} class={inputClass} required maxlength="200" oninput={handleNameInput} />
    </label>

    <label>
      <span class={labelClass}>{m.accounts_field_kind()}</span>
      <select bind:value={accountKind} class={inputClass}>
        {#each accountTypeGroups as group (group.label)}
          <optgroup label={group.label}>
            {#each group.kinds as kind (kind)}
              <option value={kind}>{accountKindLabel(kind)}</option>
            {/each}
          </optgroup>
        {/each}
      </select>
    </label>

    <label>
      <span class={labelClass}>{m.accounts_field_parent()}</span>
      <select bind:value={parentAccountID} class={inputClass}>
        <option value="">{m.accounts_field_no_parent()}</option>
        {#each parentOptions as parent (parent.id)}
          <option value={String(parent.id)}>{accountDisplayName(parent)}</option>
        {/each}
      </select>
    </label>

    <label>
      <span class={labelClass}>{m.accounts_field_institution()}</span>
      <select bind:value={institutionID} class={inputClass} disabled={institutionLockedByParent}>
        <option value="">{m.accounts_field_no_institution()}</option>
        {#each activeInstitutions as institution (institution.id)}
          <option value={String(institution.id)}>{institution.name}</option>
        {/each}
      </select>
      {#if institutionInheritanceHint}
        <p class="mt-1.5 text-xs leading-5 text-muted">{institutionInheritanceHint}</p>
      {/if}
    </label>

    <label>
      <span class={labelClass}>{m.accounts_field_currency()}</span>
      <select value={defaultCommodityID} class={inputClass} onchange={handleCurrencySelect}>
        {#if selectedCatalogCurrency}
          <option value="">{currencyPlaceholder}</option>
        {:else if currencyRequired}
          <option value="" disabled>{currencyPlaceholder}</option>
        {:else}
          <option value="">{currencyPlaceholder}</option>
        {/if}
        {#each currencyOptions as currency (currency.id)}
          <option value={String(currency.id)}>{currency.code} {currency.display_symbol}</option>
        {/each}
        <option value={otherCurrencyOptionValue}>{m.accounts_field_other_currency()}</option>
      </select>
    </label>

  </div>

  <label class="flex items-center gap-3 rounded-[var(--radius-control)] border border-border bg-control px-3 py-2 text-sm text-foreground">
    <input bind:checked={allowsPostings} type="checkbox" class="h-4 w-4 accent-[var(--color-accent)]" />
    {m.accounts_field_allows_postings()}
  </label>

  <details class="rounded-[var(--radius-control)] border border-border bg-surface">
    <summary class="cursor-pointer px-3 py-2 text-sm font-semibold text-foreground">{m.form_advanced()}</summary>
    <div class="grid gap-3 border-t border-border px-3 py-3 sm:grid-cols-2">
      <label>
        <span class={labelClass}>{m.accounts_field_code()}</span>
        <input value={code} class={inputClass} maxlength="100" oninput={handleCodeInput} />
      </label>

      <label>
        <span class={labelClass}>{m.accounts_field_country()}</span>
        <input bind:value={countryCode} class={inputClass} maxlength="2" autocapitalize="characters" />
      </label>

      <label>
        <span class={labelClass}>{m.accounts_field_number_last4()}</span>
        <input bind:value={numberLast4} class={inputClass} maxlength="8" inputmode="numeric" />
      </label>

      <label>
        <span class={labelClass}>{m.accounts_field_opened_on()}</span>
        <input bind:value={openedOn} class={inputClass} type="date" />
      </label>

      <label>
        <span class={labelClass}>{m.accounts_field_effective_from()}</span>
        <input bind:value={effectiveFrom} class={inputClass} type="date" />
      </label>

      <label>
        <span class={labelClass}>{m.accounts_field_reason()}</span>
        <input bind:value={changeReason} class={inputClass} maxlength="500" />
      </label>

      <label class="sm:col-span-2">
        <span class={labelClass}>{m.accounts_field_comment()}</span>
        <textarea bind:value={commentMarkdown} class={textAreaClass} maxlength="5000"></textarea>
      </label>
    </div>
  </details>

  {#if activeInstitutions.length === 0 && !selectedParent}
    <button
      type="button"
      class="inline-flex items-center gap-2 rounded-[var(--radius-control)] border border-border bg-control px-3 py-2 text-sm font-semibold text-foreground transition hover:bg-control-hover"
      onclick={onQuickInstitution}
    >
      <Plus size={16} aria-hidden="true" />
      {m.accounts_quick_create_institution()}
    </button>
  {/if}

  <div class="flex flex-wrap items-center justify-end gap-2">
    <button
      type="button"
      class="inline-flex items-center gap-2 rounded-[var(--radius-control)] border border-border bg-control px-4 py-2.5 text-sm font-semibold text-foreground transition hover:bg-control-hover"
      onclick={onCancel}
    >
      <X size={16} aria-hidden="true" />
      {m.accounts_form_cancel()}
    </button>
    <button
      type="submit"
      class="inline-flex items-center gap-2 rounded-[var(--radius-control)] bg-foreground px-4 py-2.5 text-sm font-semibold text-background transition hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-60"
      disabled={pending}
    >
      <Save size={16} aria-hidden="true" />
      {submitLabel}
    </button>
  </div>

  {#if currencyDialogOpen}
    <div class="fixed inset-0 z-50 flex items-center justify-center bg-background/70 px-4 py-6 backdrop-blur-sm">
      <section class="w-full max-w-lg rounded-[var(--radius-panel)] border border-border bg-surface shadow-[var(--shadow-panel)]">
        <div class="flex items-start justify-between gap-3 border-b border-border px-4 py-3">
          <div class="min-w-0">
            <h3 class="text-sm font-semibold text-foreground">{m.accounts_currency_dialog_title()}</h3>
            <p class="mt-1 text-xs leading-5 text-muted">{m.accounts_currency_dialog_copy()}</p>
          </div>
          <button
            type="button"
            class="inline-flex h-8 w-8 items-center justify-center rounded-[var(--radius-control)] border border-border bg-control text-foreground transition hover:bg-control-hover"
            onclick={() => (currencyDialogOpen = false)}
            aria-label={m.accounts_currency_dialog_close()}
            title={m.accounts_currency_dialog_close()}
          >
            <X size={15} aria-hidden="true" />
          </button>
        </div>

        <div class="space-y-3 px-4 py-4">
          <label>
            <span class={labelClass}>{m.accounts_currency_dialog_search()}</span>
            <input
              bind:value={currencySearch}
              class={inputClass}
              autocomplete="off"
              placeholder={m.accounts_currency_dialog_search_placeholder()}
            />
          </label>

          <div class="max-h-72 overflow-auto rounded-[var(--radius-control)] border border-border">
            {#each filteredCurrencyCatalog as currency (currency.code)}
              <button
                type="button"
                class="flex w-full items-center justify-between gap-3 border-b border-border px-3 py-2 text-left text-sm transition last:border-b-0 hover:bg-row-hover"
                onclick={() => chooseCatalogCurrency(currency)}
              >
                <span class="min-w-0">
                  <span class="block font-semibold text-foreground">{currency.code}</span>
                  <span class="block truncate text-xs text-muted">{currency.name}</span>
                </span>
                <span class="shrink-0 text-muted">{currency.symbol}</span>
              </button>
            {:else}
              <p class="px-3 py-6 text-center text-sm text-muted">{m.accounts_currency_dialog_empty()}</p>
            {/each}
          </div>
        </div>
      </section>
    </div>
  {/if}
</form>
