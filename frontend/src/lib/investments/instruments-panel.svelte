<script lang="ts">
  import { createQuery, useQueryClient } from '@tanstack/svelte-query';
  import Panel from '#lib/components/panel.svelte';
  import APIFormError from '#lib/components/api-form-error.svelte';
  import { investmentInstrumentsQueryOptions } from '#lib/api/investments.ts';
  import { currenciesQueryOptions } from '#lib/api/currencies.ts';
  import { pricingSourcesQueryOptions, pricingSourceAssignmentsQueryOptions, pricingSourceHealthQueryOptions, queueQuoteRefresh, createPricingSourceAssignment, updatePricingSourceAssignment } from '#lib/api/pricing.ts';
  import { saveInstrument, createInstrumentHolding, type Instrument, type InstrumentType } from '#lib/api/instruments.ts';
  import { accountsQueryKey } from '#lib/api/accounts.ts';
  import { invalidateInvestmentReads } from '#lib/investments/invalidate.ts';
  import { useOwnerToday } from '#lib/dates/owner-date-context.ts';
  import { m } from '#lib/paraglide/messages.js';
  const ownerToday = useOwnerToday();
  let { csrfToken }: { csrfToken: string } = $props();
  const client = useQueryClient();
  const instrumentsQuery = createQuery(() => investmentInstrumentsQueryOptions());
  const currenciesQuery = createQuery(() => currenciesQueryOptions());
  const sourcesQuery = createQuery(() => pricingSourcesQueryOptions());
  const healthQuery = createQuery(() => ({ ...pricingSourceHealthQueryOptions(), refetchInterval: 5000 }));
  const assignmentsQuery = createQuery(() => pricingSourceAssignmentsQueryOptions());
  let healthSignature = $state('');
  $effect(() => {
    const signature = JSON.stringify(healthQuery.data?.sources.map(source => [source.id, source.updated_at]) ?? []);
    if (healthSignature && signature !== healthSignature) void invalidateInvestmentReads(client);
    healthSignature = signature;
  });
  let selected = $state<Instrument | null>(null);
  let editing = $state(false);
  let name = $state(''); let code = $state(''); let symbol = $state('');
  let kind = $state<InstrumentType>('crypto'); let scale = $state(24); let quote = $state(0);
  let mic = $state(''); let coingecko = $state(''); let yahoo = $state(''); let twelve = $state('');
  let sourceID = $state(0); let holdingName = $state(''); let openedOn = $state('');
  let queuedCount = $state<number | null>(null);
  let pending = $state(false); let error = $state<unknown>(); let success = $state(false);
  const instruments = $derived(instrumentsQuery.data?.instruments ?? []);
  const sources = $derived((sourcesQuery.data?.sources ?? []).filter(s => s.status === 'active' && ['coingecko','yahoo_finance','twelve_data'].includes(s.code)));
  function start(instrument: Instrument | null) {
    selected = instrument; editing = true; error = undefined; success = false;
    name = instrument?.display_name ?? ''; code = instrument?.commodity_code ?? ''; symbol = instrument?.symbol ?? '';
    kind = (instrument?.instrument_type as InstrumentType) ?? 'crypto'; scale = instrument?.quantity_scale ?? 24;
    quote = instrument?.quote_commodity_id ?? 0; mic = instrument?.mic ?? '';
    coingecko = String(instrument?.identifiers.coingecko_id ?? ''); yahoo = String(instrument?.identifiers.yahoo_symbol ?? ''); twelve = String(instrument?.identifiers.twelve_data_symbol ?? '');
    sourceID = 0; holdingName = instrument?.display_name ?? ''; openedOn = ownerToday();
  }
  async function save() {
    pending = true; error = undefined; success = false;
    try {
      selected = await saveInstrument(selected?.id ?? null, {
        commodity_code: code, display_name: name, symbol, instrument_type: kind, quantity_scale: scale,
        price_scale: selected?.price_scale ?? 12, quote_commodity_id: quote || undefined, mic,
        quote_identifiers: { coingecko_id: coingecko.trim(), yahoo_symbol: yahoo.trim(), twelve_data_symbol: twelve.trim() },
        exchange_code: selected?.exchange_code, issuer: selected?.issuer, country_code: selected?.country_code,
        trading_commodity_id: selected?.trading_commodity_id, effective_from: ownerToday()
      }, csrfToken);
      await invalidateInvestmentReads(client); success = true;
    } catch (err) { error = err; } finally { pending = false; }
  }
  async function configureQuotes() {
    if (!selected || !quote || !sourceID) return;
    pending = true; error = undefined; success = false;
    try {
      const current = assignmentsQuery.data?.assignments.find(a => a.commodity_id === selected!.commodity_id && a.quote_commodity_id === quote && a.source_id === sourceID && a.status === 'active');
      const body = { commodity_id: selected.commodity_id, quote_commodity_id: quote, source_id: sourceID, priority: 100, status: 'active' as const, effective_from: ownerToday() };
      if (current) await updatePricingSourceAssignment(current.id, body, csrfToken); else await createPricingSourceAssignment(body, csrfToken);
      await assignmentsQuery.refetch(); success = true;
    } catch (err) { error = err; } finally { pending = false; }
  }
  async function refreshQuotes() {
    pending = true; error = undefined;
    try { const accepted = await queueQuoteRefresh(csrfToken); queuedCount = accepted.queued_count; await healthQuery.refetch(); }
    catch (err) { error = err; } finally { pending = false; }
  }

  async function archiveQuote(id: number) {
    const assignment = assignmentsQuery.data?.assignments.find(a => a.id === id); if (!assignment) return;
    pending = true; error = undefined; success = false;
    try { await updatePricingSourceAssignment(id, { commodity_id: assignment.commodity_id, quote_commodity_id: assignment.quote_commodity_id, source_id: assignment.source_id, priority: assignment.priority, effective_from: assignment.effective_from, effective_to: assignment.effective_to, status: 'archived' }, csrfToken); await assignmentsQuery.refetch(); success = true; }
    catch (err) { error = err; } finally { pending = false; }
  }
  async function holding() {
    if (!selected) return; pending = true; error = undefined; success = false;
    try {
      await createInstrumentHolding({ instrument_id: selected.id, name: holdingName, opened_on: openedOn, effective_from: openedOn }, csrfToken);
      await client.invalidateQueries({ queryKey: accountsQueryKey }); holdingName = ''; success = true;
    } catch (err) { error = err; } finally { pending = false; }
  }
</script>
<div class="space-y-5">
  <Panel>
    <h2 class="text-lg font-semibold">{m.quotes_instruments()}</h2>
    <p class="mt-2 text-sm text-muted">{m.quotes_scope()}</p>
    <button type="button" onclick={() => start(null)} class="mt-4 min-h-11 rounded-(--radius-control) bg-foreground px-4 text-sm font-semibold text-background">{m.quotes_add_instrument()}</button>
    {#if instrumentsQuery.isPending}<p role="status" class="mt-3">{m.investments_loading()}</p>
    {:else if instrumentsQuery.isError}<APIFormError error={instrumentsQuery.error} />
    {:else if instruments.length === 0}<p class="mt-3 text-sm text-muted">{m.quotes_empty()}</p>
    {:else}<ul class="mt-4 space-y-2">{#each instruments as instrument}<li class="flex flex-wrap items-center gap-x-4"><button type="button" onclick={() => start(instrument)} class="min-h-11 text-left font-semibold underline underline-offset-4">{instrument.display_name} ({instrument.commodity_code})</button><a href={`/app/investments/instruments/${instrument.commodity_id}`} aria-label={m.investments_instrument_view_open_aria({ name: instrument.display_name })} class="inline-flex min-h-11 items-center text-sm underline underline-offset-4">{m.investments_instrument_view_open()}</a></li>{/each}</ul>{/if}
  </Panel>
  {#if editing}
  <Panel>
    <form aria-label={selected ? m.quotes_edit_instrument() : m.quotes_add_instrument()} onsubmit={(event) => { event.preventDefault(); void save(); }} class="space-y-4">
      <h3 class="font-semibold">{selected ? m.quotes_edit_instrument() : m.quotes_add_instrument()}</h3>
      <div class="grid gap-4 sm:grid-cols-2">
        <label class="grid gap-1 text-sm">{m.quotes_name()}<input required maxlength="200" bind:value={name} class="min-h-11 rounded-(--radius-control) border border-border bg-control px-3" /></label>
        <label class="grid gap-1 text-sm">{m.quotes_code()}<input required maxlength="64" bind:value={code} disabled={selected !== null} class="min-h-11 rounded-(--radius-control) border border-border bg-control px-3" /></label>
        <label class="grid gap-1 text-sm">{m.quotes_symbol()}<input maxlength="64" bind:value={symbol} class="min-h-11 rounded-(--radius-control) border border-border bg-control px-3" /></label>
        <label class="grid gap-1 text-sm">{m.quotes_type()}<select bind:value={kind} disabled={selected !== null} onchange={(event) => { kind = event.currentTarget.value as InstrumentType; scale = kind === 'crypto' ? 24 : 12; }} class="min-h-11 rounded-(--radius-control) border border-border bg-control px-3"><option value="crypto">{m.quotes_type_crypto()}</option><option value="stock">{m.quotes_type_stock()}</option><option value="etf">{m.quotes_type_etf()}</option><option value="fund">{m.quotes_type_fund()}</option><option value="bond">{m.quotes_type_bond()}</option><option value="option">{m.quotes_type_option()}</option><option value="future">{m.quotes_type_future()}</option><option value="other">{m.quotes_type_other()}</option></select></label>
        <label class="grid gap-1 text-sm">{m.quotes_precision()}<input type="number" min="0" max={kind === 'crypto' ? 24 : 12} required bind:value={scale} class="min-h-11 rounded-(--radius-control) border border-border bg-control px-3" /></label>
        <label class="grid gap-1 text-sm">{m.quotes_currency()}<select bind:value={quote} class="min-h-11 rounded-(--radius-control) border border-border bg-control px-3"><option value={0}>{m.pricing_choose()}</option>{#each currenciesQuery.data?.currencies ?? [] as currency}<option value={currency.id}>{currency.code}</option>{/each}</select></label>
        <label class="grid gap-1 text-sm">{m.quotes_coingecko_id()}<input maxlength="128" bind:value={coingecko} class="min-h-11 rounded-(--radius-control) border border-border bg-control px-3" /></label>
        <label class="grid gap-1 text-sm">{m.quotes_yahoo_symbol()}<input maxlength="128" bind:value={yahoo} class="min-h-11 rounded-(--radius-control) border border-border bg-control px-3" /></label>
        <label class="grid gap-1 text-sm">{m.quotes_twelve_symbol()}<input maxlength="128" bind:value={twelve} class="min-h-11 rounded-(--radius-control) border border-border bg-control px-3" /></label>
        <label class="grid gap-1 text-sm">{m.quotes_mic()}<input maxlength="16" bind:value={mic} class="min-h-11 rounded-(--radius-control) border border-border bg-control px-3" /></label>
      </div>
      <p class="text-sm text-muted">{m.quotes_identity_help()}</p>
      <button type="submit" disabled={pending || !csrfToken} class="min-h-11 rounded-(--radius-control) bg-foreground px-4 text-sm font-semibold text-background disabled:opacity-50">{m.quotes_save()}</button>
    </form>
    {#if currenciesQuery.isError}<APIFormError error={currenciesQuery.error} />{/if}
    {#if selected}
      <form onsubmit={(event) => { event.preventDefault(); void configureQuotes(); }} class="mt-6 space-y-3 border-t border-border pt-5">
        <h3 class="font-semibold">{m.quotes_enable()}</h3><p class="text-sm text-muted">{m.quotes_provider_help()}</p>
        <label class="grid gap-1 text-sm">{m.quotes_provider()}<select required bind:value={sourceID} class="min-h-11 rounded-(--radius-control) border border-border bg-control px-3"><option value={0}>{m.pricing_choose()}</option>{#each sources as source}<option value={source.id}>{source.name}</option>{/each}</select></label>
        <button disabled={pending || !quote || !sourceID || assignmentsQuery.isPending || assignmentsQuery.isError || sourcesQuery.isError} class="min-h-11 rounded-(--radius-control) border border-border bg-control px-4 text-sm font-semibold disabled:opacity-50">{m.quotes_enable()}</button>
        {#if sourcesQuery.isError}<APIFormError error={sourcesQuery.error} />{/if}
        {#if assignmentsQuery.isError}<APIFormError error={assignmentsQuery.error} />{/if}
        <ul>{#each (assignmentsQuery.data?.assignments ?? []).filter(a => a.commodity_id === selected?.commodity_id && a.status === 'active') as assignment}<li class="flex flex-wrap items-center gap-3 py-2"><span>{sources.find(s => s.id === assignment.source_id)?.name ?? ''} · {currenciesQuery.data?.currencies.find(c => c.id === assignment.quote_commodity_id)?.code ?? ''}</span><button type="button" disabled={pending} onclick={() => archiveQuote(assignment.id)} class="min-h-11 px-3 text-sm font-semibold underline disabled:opacity-50">{m.quotes_disable()}</button></li>{/each}</ul>
        <button type="button" onclick={refreshQuotes} disabled={pending} class="min-h-11 rounded-(--radius-control) border border-border bg-control px-4 text-sm font-semibold disabled:opacity-50">{m.quotes_refresh()}</button>
        {#if queuedCount !== null}<p role="status" class="text-sm">{m.quotes_queued({ count: String(queuedCount) })}</p>{/if}
        {#if healthQuery.isError}<APIFormError error={healthQuery.error} />{/if}
        <ul class="space-y-2">{#each (healthQuery.data?.sources ?? []).filter(h => h.commodity_id === selected?.commodity_id) as health}<li class="text-sm">{sources.find(s => s.id === health.source_id)?.name ?? ''} · {health.last_success_date || '—'}{#if health.last_error}<p role="status" class="text-warning">{health.last_error}</p>{/if}</li>{/each}</ul>
        <a href="/app/settings/currencies" class="inline-block min-h-11 text-sm font-semibold underline">{m.quotes_refresh_health()}</a>
      </form>
      <form aria-label={m.quotes_add_holding()} onsubmit={(event) => { event.preventDefault(); void holding(); }} class="mt-6 space-y-3 border-t border-border pt-5">
        <h3 class="font-semibold">{m.quotes_add_holding()}</h3>
        <p class="text-sm text-muted">{m.quotes_opened_help()}</p>
        <label class="grid gap-1 text-sm">{m.quotes_name()}<input required bind:value={holdingName} class="min-h-11 rounded-(--radius-control) border border-border bg-control px-3" /></label>
        <label class="grid gap-1 text-sm">{m.quotes_opened()}<input type="date" required bind:value={openedOn} class="min-h-11 rounded-(--radius-control) border border-border bg-control px-3" /></label>
        <button disabled={pending} class="min-h-11 rounded-(--radius-control) border border-border bg-control px-4 text-sm font-semibold disabled:opacity-50">{m.quotes_add_holding()}</button>
      </form>
    {/if}
    {#if error}<div class="mt-4"><APIFormError {error} /></div>{/if}
    {#if success}<p role="status" class="mt-4 text-sm">{m.quotes_saved()}</p>{/if}
  </Panel>
  {/if}
  <a href="/app/ledger/import" class="inline-block min-h-11 text-sm font-semibold underline">{m.quotes_csv()}</a>
</div>
