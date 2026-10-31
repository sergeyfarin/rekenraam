<script lang="ts">
  import { createQuery } from '@tanstack/svelte-query';
  import { goto } from '$app/navigation';
  import { page } from '$app/state';
  import PageHeader from '#lib/components/page-header.svelte';
  import Panel from '#lib/components/panel.svelte';
  import StatePanel from '#lib/components/state-panel.svelte';
  import APIFormError from '#lib/components/api-form-error.svelte';
  import { accountsQueryOptions, type AccountResponse } from '#lib/api/accounts.ts';
  import { currentBookQueryOptions } from '#lib/api/books.ts';
  import { currenciesQueryOptions } from '#lib/api/currencies.ts';
  import {
    investmentInstrumentsQueryOptions, investmentReturnsQueryOptions, type BenchmarkBasis, type InvestmentInstrumentResponse, type ReturnsRunRequest
  } from '#lib/api/investments.ts';
  import { authSessionQueryOptions } from '#lib/api/auth.ts';
  import { useOwnerToday } from '#lib/dates/owner-date-context.ts';
  import { m } from '#lib/paraglide/messages.js';
  import { getLocale } from '#lib/paraglide/runtime.js';
  import {
    parseReturnsSelection, presetRange, returnsSelectionParams, unavailableAccountIDs, validRange, type ReturnsPreset
  } from './returns-model';
  import ReturnsResult from './returns-result.svelte';
  import ReturnsRuns from './returns-runs.svelte';

  // R13 portfolio returns (#119, slice 2): a read-only view of the returns_v1
  // live read. Every missing figure says why; nothing here posts or moves
  // money. Saving a run (slice 6) writes only the frozen document.
  const ownerToday = useOwnerToday();
  const today = ownerToday();
  const locale = $derived(getLocale());

  // Archived accounts stay selectable: a past period can need them.
  // The reference queries notify on every result change: the screen gates its
  // form on their pending flags, and property-tracked observers can drop the
  // settling update when a fast response arrives before the first render reads
  // `isPending`, leaving the spinner up forever.
  const accounts = createQuery(() => ({ ...accountsQueryOptions(true), notifyOnChangeProps: 'all' as const }));
  const currencies = createQuery(() => ({ ...currenciesQueryOptions(), notifyOnChangeProps: 'all' as const }));
  const book = createQuery(() => ({ ...currentBookQueryOptions(), notifyOnChangeProps: 'all' as const }));
  const instruments = createQuery(() => investmentInstrumentsQueryOptions());
  const session = createQuery(() => authSessionQueryOptions());
  const csrfToken = $derived(session.data?.csrf_token ?? '');

  const selection = $derived(parseReturnsSelection(page.url.searchParams, today));
  // A saved run a link names opens beside the live read (#479).
  const linkedRunID = $derived.by(() => {
    const raw = page.url.searchParams.get('run_id');
    return raw !== null && /^[1-9][0-9]{0,15}$/.test(raw) ? Number(raw) : null;
  });
  const activeCurrencies = $derived((currencies.data?.currencies ?? []).filter((currency) => currency.status === 'active'));
  // The owner's choice wins; otherwise propose the book's default currency for
  // new UI, then the first active currency. The response names whichever is used.
  const reportingCurrencyID = $derived(
    selection.reportingCurrencyID
      ?? book.data?.default_currency_commodity_id
      ?? activeCurrencies[0]?.id
      ?? null
  );
  const portfolioAccounts = $derived(
    (accounts.data?.accounts ?? [])
      .filter((account) => !account.is_system && (account.account_class === 'asset' || account.account_class === 'liability'))
      .sort((a, b) => accountLabel(a).localeCompare(accountLabel(b), locale))
  );

  // Any instrument with a price series can be a benchmark, held or not.
  const benchmarkOptions = $derived(
    (instruments.data?.instruments ?? []).toSorted((a, b) => instrumentLabel(a).localeCompare(instrumentLabel(b), locale))
  );
  // A linked benchmark that is not an instrument here would fail the read; it
  // is disclosed and left out, like an unavailable account.
  const benchmarkUnavailable = $derived(
    selection.benchmarkCommodityID !== null && instruments.isSuccess
      && !benchmarkOptions.some((instrument) => instrument.commodity_id === selection.benchmarkCommodityID)
  );
  const benchmarkCommodityID = $derived(
    instruments.isSuccess && !benchmarkUnavailable ? selection.benchmarkCommodityID : null
  );

  // Draft controls, applied to the URL on submit; reset when the URL moves
  // (back/forward, a link), so the form always describes what is shown.
  let fromDraft = $state('');
  let toDraft = $state('');
  let currencyDraft = $state('');
  let accountDraft = $state<number[]>([]);
  let benchmarkDraft = $state('');
  let basisDraft = $state<BenchmarkBasis>('price_return');
  let instrumentsDraft = $state(false);
  $effect(() => {
    fromDraft = selection.from;
    toDraft = selection.to;
    accountDraft = [...selection.accountIDs];
    benchmarkDraft = selection.benchmarkCommodityID === null ? '' : String(selection.benchmarkCommodityID);
    basisDraft = selection.benchmarkBasis;
    instrumentsDraft = selection.instruments;
  });
  $effect(() => {
    currencyDraft = reportingCurrencyID === null ? '' : String(reportingCurrencyID);
  });
  const rangeInvalid = $derived(!validRange(fromDraft, toDraft, today));
  const unavailableDraft = $derived(unavailableAccountIDs(accountDraft, portfolioAccounts.map((account) => account.id)));

  const returns = createQuery(() => ({
    ...investmentReturnsQueryOptions({
      from: selection.from,
      to: selection.to,
      reportingCurrencyID: reportingCurrencyID ?? 0,
      accountIDs: selection.accountIDs,
      benchmarkCommodityID,
      benchmarkBasis: selection.benchmarkBasis,
      instruments: selection.instruments
    }),
    // A linked benchmark waits for the instruments that confirm it exists.
    enabled: reportingCurrencyID !== null && (selection.benchmarkCommodityID === null || !instruments.isPending)
  }));
  const result = $derived(returns.data);
  // Saving recomputes the applied selection on the server; it is offered
  // only beside a live result for that same selection.
  const savedRunRequest = $derived<ReturnsRunRequest | null>(
    result && result.resolved_account_ids.length > 0 && reportingCurrencyID !== null
      ? {
        from: selection.from, to: selection.to, reporting_currency_id: reportingCurrencyID, account_ids: selection.accountIDs,
        benchmark_commodity_id: benchmarkCommodityID, benchmark_basis: benchmarkCommodityID !== null ? selection.benchmarkBasis : null,
        ...(selection.instruments ? { instruments: true } : {})
      }
      : null
  );
  const referencesLoading = $derived(accounts.isPending || currencies.isPending || book.isPending);
  const referenceError = $derived(accounts.error ?? currencies.error ?? book.error);
  // The loading flag stays the first branch so its query observer is tracked
  // before a fast response settles; a known error then outranks it instead of
  // staying hidden behind a sibling reference query still loading.
  const showLoading = $derived.by(() => {
    const loading = referencesLoading;
    return !referenceError && loading;
  });

  function apply(range?: { from: string; to: string }) {
    const from = range?.from ?? fromDraft;
    const to = range?.to ?? toDraft;
    if (!validRange(from, to, today) || currencyDraft === '') return;
    // An id that is not a selectable account here would fail every request;
    // it is listed as unavailable above the button and left out.
    const selectable = accountDraft.filter((id) => !unavailableDraft.includes(id));
    const benchmark = benchmarkDraft === '' ? null : Number(benchmarkDraft);
    const params = returnsSelectionParams({
      from, to, reportingCurrencyID: Number(currencyDraft), accountIDs: [...selectable].sort((a, b) => a - b),
      benchmarkCommodityID: benchmark !== null && benchmarkOptions.some((instrument) => instrument.commodity_id === benchmark) ? benchmark : null,
      benchmarkBasis: basisDraft,
      instruments: instrumentsDraft
    });
    void goto(`/app/investments/returns?${params.toString()}`, { reset: false });
  }

  function choosePreset(preset: ReturnsPreset) {
    apply(presetRange(preset, today));
  }

  function toggleAccount(id: number, checked: boolean) {
    accountDraft = checked ? [...accountDraft, id] : accountDraft.filter((existing) => existing !== id);
  }

  function accountLabel(account: AccountResponse): string {
    const name = account.name || account.code || `#${account.id}`;
    if (account.status === 'archived') return m.returns_account_archived({ name });
    if (account.status === 'closed') return m.returns_account_closed({ name });
    return name;
  }

  function accountName(id: number): string {
    const account = accounts.data?.accounts.find((candidate) => candidate.id === id);
    return account ? accountLabel(account) : `#${id}`;
  }

  function instrumentLabel(instrument: InvestmentInstrumentResponse): string {
    const symbol = instrument.symbol || instrument.commodity_code;
    return instrument.display_name && instrument.display_name !== symbol ? `${symbol} · ${instrument.display_name}` : symbol;
  }

  // A coverage gap names a currency or a priced instrument.
  function currencyOrCommodity(id: number): string {
    const currency = currencies.data?.currencies.find((candidate) => candidate.id === id);
    if (currency) return currency.code;
    const instrument = instruments.data?.instruments.find((candidate) => candidate.commodity_id === id);
    return instrument ? instrument.symbol || instrument.commodity_code : `#${id}`;
  }
</script>

<PageHeader title={m.returns_title()} copy={m.returns_help()} />
<div class="mt-6 max-w-5xl space-y-5" data-testid="returns-screen">
  <a href="/app/investments" class="inline-flex min-h-11 items-center text-sm font-semibold text-foreground underline underline-offset-2">{m.investments_instrument_view_back()}</a>

  <!-- A known failure outranks a sibling reference query still loading: an
       errored accounts, currencies or book request must not stay hidden behind
       the spinner while another one settles. -->
  {#if showLoading}
    <Panel><p role="status" class="text-sm text-muted">{m.returns_loading()}</p></Panel>
  {:else if referenceError}
    <StatePanel title={m.returns_error_title()} copy={m.returns_error_copy()} alert>
      <APIFormError error={referenceError} />
      <button type="button" class="min-h-11 text-sm text-foreground underline" onclick={() => { accounts.refetch(); currencies.refetch(); book.refetch(); }}>{m.install_gate_retry()}</button>
    </StatePanel>
  {:else if activeCurrencies.length === 0}
    <StatePanel title={m.returns_no_currency_title()} copy={m.returns_no_currency_copy()}>
      <a href="/app/settings" class="inline-flex min-h-11 items-center text-sm font-semibold text-foreground underline underline-offset-2">{m.returns_no_currency_action()}</a>
    </StatePanel>
  {:else}
    <Panel>
      <form
        class="space-y-4"
        aria-labelledby="returns-controls-title"
        onsubmit={(event) => { event.preventDefault(); apply(); }}
      >
        <h2 id="returns-controls-title" class="text-sm font-semibold text-foreground">{m.returns_controls_title()}</h2>
        <div class="flex flex-wrap gap-2">
          <button type="button" class="min-h-11 rounded-(--radius-control) border border-border bg-control px-3 text-sm text-foreground hover:bg-control-hover" onclick={() => choosePreset('ytd')}>{m.returns_preset_ytd()}</button>
          <button type="button" class="min-h-11 rounded-(--radius-control) border border-border bg-control px-3 text-sm text-foreground hover:bg-control-hover" onclick={() => choosePreset('last_year')}>{m.returns_preset_last_year()}</button>
        </div>
        <div class="grid grid-cols-1 gap-3 sm:grid-cols-3">
          <label class="flex flex-col gap-1 text-sm text-muted">
            {m.returns_from()}
            <input type="date" bind:value={fromDraft} max={today} aria-invalid={rangeInvalid}
              aria-describedby={rangeInvalid ? 'returns-range-error' : undefined}
              class="min-h-11 rounded border border-border bg-control px-2 text-foreground focus:outline-none focus:ring-1 focus:ring-foreground/30" />
          </label>
          <label class="flex flex-col gap-1 text-sm text-muted">
            {m.returns_to()}
            <input type="date" bind:value={toDraft} max={today} aria-invalid={rangeInvalid}
              aria-describedby={rangeInvalid ? 'returns-range-error' : undefined}
              class="min-h-11 rounded border border-border bg-control px-2 text-foreground focus:outline-none focus:ring-1 focus:ring-foreground/30" />
          </label>
          <label class="flex flex-col gap-1 text-sm text-muted">
            {m.returns_reporting_currency()}
            <select bind:value={currencyDraft}
              class="min-h-11 rounded border border-border bg-control px-2 text-foreground focus:outline-none focus:ring-1 focus:ring-foreground/30">
              {#each activeCurrencies as currency (currency.id)}
                <option value={String(currency.id)}>{currency.code}</option>
              {/each}
            </select>
          </label>
        </div>
        {#if rangeInvalid}
          <p id="returns-range-error" class="text-sm text-danger">{m.returns_range_invalid()}</p>
        {/if}

        <fieldset class="space-y-2" data-testid="returns-benchmark-controls">
          <legend class="text-sm font-medium text-foreground">{m.returns_benchmark_label()}</legend>
          <p id="returns-benchmark-help" class="text-xs text-muted">{m.returns_benchmark_help()}</p>
          <div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
            <label class="flex flex-col gap-1 text-sm text-muted">
              {m.returns_benchmark_instrument()}
              <select bind:value={benchmarkDraft} aria-describedby="returns-benchmark-help" disabled={instruments.isPending}
                class="min-h-11 rounded border border-border bg-control px-2 text-foreground focus:outline-none focus:ring-1 focus:ring-foreground/30">
                <option value="">{m.returns_benchmark_none()}</option>
                {#each benchmarkOptions as instrument (instrument.commodity_id)}
                  <option value={String(instrument.commodity_id)}>{instrumentLabel(instrument)}</option>
                {/each}
              </select>
            </label>
            {#if benchmarkDraft !== ''}
              <label class="flex flex-col gap-1 text-sm text-muted">
                {m.returns_benchmark_basis_label()}
                <select bind:value={basisDraft}
                  class="min-h-11 rounded border border-border bg-control px-2 text-foreground focus:outline-none focus:ring-1 focus:ring-foreground/30">
                  <option value="price_return">{m.returns_benchmark_basis_price()}</option>
                  <option value="total_return">{m.returns_benchmark_basis_total()}</option>
                </select>
              </label>
            {/if}
          </div>
          {#if instruments.isError}
            <p role="status" class="text-sm text-warning">{m.returns_benchmark_instruments_error()}</p>
          {/if}
          {#if benchmarkUnavailable}
            <p role="status" class="text-sm text-warning">{m.returns_benchmark_unavailable({ benchmark: `#${selection.benchmarkCommodityID}` })}</p>
          {/if}
        </fieldset>

        <fieldset class="space-y-2">
          <legend class="text-sm font-medium text-foreground">{m.returns_accounts_title()}</legend>
          <p class="text-xs text-muted">{m.returns_accounts_help()}</p>
          {#if portfolioAccounts.length === 0}
            <p class="text-sm text-muted">{m.returns_accounts_none()}</p>
          {:else}
            <ul class="grid grid-cols-1 gap-1 sm:grid-cols-2">
              {#each portfolioAccounts as account (account.id)}
                <li>
                  <label class="flex min-h-11 items-center gap-2 text-sm text-foreground">
                    <input type="checkbox" checked={accountDraft.includes(account.id)}
                      onchange={(event) => toggleAccount(account.id, event.currentTarget.checked)} />
                    {accountLabel(account)}
                  </label>
                </li>
              {/each}
            </ul>
          {/if}
          {#if unavailableDraft.length > 0}
            <div role="status" class="space-y-1 text-sm text-warning">
              <p>{m.returns_accounts_unavailable()}</p>
              <ul class="flex flex-wrap gap-2">
                {#each unavailableDraft as id (id)}
                  <li>
                    <button type="button" class="min-h-11 rounded-(--radius-control) border border-border bg-control px-3 text-foreground hover:bg-control-hover"
                      onclick={() => toggleAccount(id, false)}>{m.returns_accounts_remove_unavailable({ account: `#${id}` })}</button>
                  </li>
                {/each}
              </ul>
            </div>
          {/if}
          {#if accountDraft.length > 0}
            <button type="button" class="min-h-11 text-sm text-foreground underline underline-offset-2" onclick={() => { accountDraft = []; }}>{m.returns_accounts_reset()}</button>
          {/if}
        </fieldset>

        <div class="space-y-1">
          <label class="flex min-h-11 items-center gap-2 text-sm text-foreground">
            <input type="checkbox" bind:checked={instrumentsDraft} aria-describedby="returns-instruments-help" data-testid="returns-instruments-toggle" />
            {m.returns_instruments_toggle()}
          </label>
          <p id="returns-instruments-help" class="text-xs text-muted">{m.returns_instruments_toggle_help()}</p>
        </div>

        <button type="submit" disabled={rangeInvalid || currencyDraft === ''}
          class="min-h-11 rounded-(--radius-control) border border-border bg-control px-4 text-sm font-medium text-foreground hover:bg-control-hover disabled:opacity-50">
          {m.returns_apply()}
        </button>
      </form>
    </Panel>

    {#if returns.isPending}
      <Panel><p role="status" class="text-sm text-muted">{m.returns_loading()}</p></Panel>
    {:else if returns.isError}
      <StatePanel title={m.returns_error_title()} copy={m.returns_error_copy()} alert>
        <APIFormError error={returns.error} />
        <button type="button" class="min-h-11 text-sm text-foreground underline" onclick={() => returns.refetch()}>{m.install_gate_retry()}</button>
      </StatePanel>
    {:else if result && result.resolved_account_ids.length === 0}
      <StatePanel title={m.returns_empty_title()} copy={m.returns_empty_copy()}>
        <a href="/app/ledger/accounts" class="inline-flex min-h-11 items-center text-sm font-semibold text-foreground underline underline-offset-2">{m.returns_empty_action()}</a>
      </StatePanel>
    {:else if result}
      <ReturnsResult {result} reportingCurrencyID={reportingCurrencyID ?? 0} currencies={currencies.data?.currencies ?? []} {locale}
        {accountName} {currencyOrCommodity} fetching={returns.isFetching} />
    {/if}

    <ReturnsRuns {csrfToken} request={returns.isFetching ? null : savedRunRequest} currencies={currencies.data?.currencies ?? []} {locale}
      {accountName} {currencyOrCommodity} {linkedRunID} />
  {/if}
</div>
