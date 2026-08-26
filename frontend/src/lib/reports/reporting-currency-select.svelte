<script lang="ts">
  import { createQuery } from '@tanstack/svelte-query';
  import { m } from '$lib/paraglide/messages.js';
  import { currenciesQueryOptions } from '$lib/api/currencies';
  import { getLocale } from '$lib/paraglide/runtime.js';

  let {
    selected,
    onChange
  }: {
    selected: number | null;
    onChange: (commodityID: number | null) => void;
  } = $props();

  const locale = $derived(getLocale());
  const collator = $derived(new Intl.Collator(locale, { sensitivity: 'base' }));
  const currenciesQuery = createQuery(() => currenciesQueryOptions());

  // Currencies only. A report can be *filtered* to a security, but denominating
  // in one would ask what a month of groceries is worth in shares — the backend
  // refuses a non-currency quote, and offering it here would only produce that
  // refusal one click later.
  //
  // Archived currencies stay listed: a report looks backwards, and a range that
  // predates an archival is exactly when its rates still matter.
  const currencies = $derived(
    [...(currenciesQuery.data?.currencies ?? [])].sort((a, b) => collator.compare(a.code, b.code))
  );

  // A selection the catalog has no entry for is still honoured rather than
  // silently reset to "none": the report behind it is already denominated, and
  // resetting the control would make the screen disagree with its own figures.
  const unknownSelection = $derived(
    selected !== null && !currencies.some((currency) => currency.id === selected)
  );
</script>

<label class="block min-w-44 flex-1 sm:flex-none">
  <span class="text-xs font-semibold uppercase tracking-[0.12em] text-muted">
    {m.reports_reporting_currency()}
  </span>
  <select
    value={selected === null ? '' : String(selected)}
    disabled={currenciesQuery.isPending}
    onchange={(event) => {
      const raw = event.currentTarget.value;
      onChange(raw === '' ? null : Number(raw));
    }}
    class="mt-1.5 h-10 w-full rounded-(--radius-control) border border-border bg-control px-3 text-sm text-foreground shadow-sm outline-none transition hover:bg-control-hover focus:border-accent disabled:opacity-60"
  >
    <option value="">
      {currenciesQuery.isPending
        ? m.reports_reporting_currency_loading()
        : m.reports_reporting_currency_none()}
    </option>
    {#if unknownSelection}
      <option value={String(selected)}>{m.reports_commodity_unknown({ commodityId: selected ?? 0 })}</option>
    {/if}
    {#each currencies as currency (currency.id)}
      <option value={String(currency.id)}>{currency.code}</option>
    {/each}
  </select>
</label>
