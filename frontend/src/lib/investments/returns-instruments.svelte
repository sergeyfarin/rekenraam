<script lang="ts">
  import type { InstrumentReturns, ReturnFigure, ReturnsAmount, ReturnsAttributionFallback } from '#lib/api/investments.ts';
  import { m } from '#lib/paraglide/messages.js';
  import { fallbackCountedText, fallbackReasonText, formatReturn, returnDirection, returnReasonText, returnsCoverage } from './returns-model';

  // Per-instrument returns (instrument_returns_v1, R13 slice 9), live or
  // saved: every figure, reason and gap comes from the document; only the
  // instrument names are today's reference data. Cards on a phone, a table
  // from md up; each unavailable figure says why in words.
  let {
    instruments, fallbacks, locale, formatDate, formatAmount, commodityLabel
  }: {
    instruments: InstrumentReturns;
    /** The portfolio's own attribution fallbacks from the same document:
     * whether its figures count a payment no instrument carries. */
    fallbacks: ReturnsAttributionFallback[];
    locale: string;
    formatDate: (date: string) => string;
    /** The result's own amount text (live or saved scale) or its reason. */
    formatAmount: (amount: ReturnsAmount) => string;
    commodityLabel: (id: number) => string;
  } = $props();

  const idPrefix = $props.id();
  const rows = $derived(instruments.rows.map((row) => ({ row, name: commodityLabel(row.commodity_id), coverage: returnsCoverage(row.valuation) })));

  function figureText(figure: ReturnFigure): string {
    return figure.value !== null ? formatReturn(figure.value, locale) : '—';
  }

  function reasonText(figure: ReturnFigure): string {
    return returnReasonText(figure.reason, figure.date, formatDate);
  }

  function directionClass(figure: ReturnFigure): string {
    if (figure.value === null) return 'text-muted';
    const direction = returnDirection(figure.value);
    return direction === 'up' ? 'text-positive' : direction === 'down' ? 'text-danger' : 'text-foreground';
  }

  function amountClass(amount: ReturnsAmount): string {
    return amount.amount ? 'text-foreground' : 'text-muted';
  }
</script>

{#snippet returnFigure(period: ReturnFigure, annual: ReturnFigure, testid: string)}
  <span class="block font-medium tabular-nums {directionClass(period)}" data-testid={testid}>{figureText(period)}</span>
  {#if period.value === null}
    <span class="block text-xs text-muted">{reasonText(period)}</span>
  {:else if annual.value !== null}
    <span class="block text-xs text-muted tabular-nums">{m.returns_instruments_annual({ value: figureText(annual) })}</span>
  {/if}
{/snippet}

{#snippet coverageNotes(coverage: { missing: number[]; staleDates: string[] })}
  {#if coverage.missing.length > 0}
    <p role="status" class="text-xs text-warning">{m.returns_instruments_coverage({ commodities: coverage.missing.map(commodityLabel).join(', ') })}</p>
  {/if}
  {#if coverage.staleDates.length > 0}
    <p class="text-xs text-muted">{m.returns_instruments_coverage_stale({ dates: coverage.staleDates.map(formatDate).join(', ') })}</p>
  {/if}
{/snippet}

<section aria-labelledby={`${idPrefix}-instruments-title`} class="min-w-0 space-y-3" data-testid="returns-instruments">
  <h3 id={`${idPrefix}-instruments-title`} class="text-sm font-semibold text-foreground">{m.returns_instruments_title()}</h3>
  <p class="text-xs text-muted">{m.returns_instruments_help()}</p>
  <p class="text-xs text-muted" data-testid="returns-instruments-income-rule">{m.returns_instruments_income_rule()}</p>

  {#if rows.length === 0}
    <p class="text-sm text-muted" data-testid="returns-instruments-empty">{m.returns_instruments_empty()}</p>
  {:else}
    <!-- Phone: one card per instrument. -->
    <ul class="space-y-3 md:hidden">
      {#each rows as { row, name, coverage } (row.commodity_id)}
        <li class="rounded-(--radius-control) border border-border p-3" data-testid="returns-instrument-card">
          <h4 class="flex flex-wrap items-center gap-2 text-sm font-semibold text-foreground">
            {name}
            {#if row.short}<span class="rounded border border-border px-1.5 text-xs font-medium text-muted">{m.returns_instruments_short()}</span>{/if}
          </h4>
          <dl class="mt-2 grid grid-cols-2 gap-x-3 gap-y-2 text-sm">
            <div>
              <dt class="text-xs text-muted">{m.returns_instruments_twr()}</dt>
              <dd>{@render returnFigure(row.time_weighted, row.time_weighted_annualized, 'returns-instrument-twr')}</dd>
            </div>
            <div>
              <dt class="text-xs text-muted">{m.returns_instruments_mwr()}</dt>
              <dd>{@render returnFigure(row.money_weighted_period, row.money_weighted, 'returns-instrument-mwr')}</dd>
            </div>
            {#each [
              { key: 'opening', label: m.returns_instruments_opening(), amount: row.opening_value },
              { key: 'closing', label: m.returns_instruments_closing(), amount: row.closing_value },
              { key: 'contributions', label: m.returns_contributions(), amount: row.contributions },
              { key: 'withdrawals', label: m.returns_withdrawals(), amount: row.withdrawals },
              { key: 'income', label: m.returns_instruments_income(), amount: row.income },
              { key: 'gain', label: m.returns_gain(), amount: row.gain }
            ] as line (line.key)}
              <div class="min-w-0">
                <dt class="text-xs text-muted">{line.label}</dt>
                <dd class="break-words tabular-nums {amountClass(line.amount)}" data-testid="returns-instrument-{line.key}">{formatAmount(line.amount)}</dd>
              </div>
            {/each}
          </dl>
          <div class="mt-2 space-y-1">{@render coverageNotes(coverage)}</div>
        </li>
      {/each}
    </ul>

    <!-- Wider screens: one table, scrolling inside its own box if needed. -->
    <!-- svelte-ignore a11y_no_noninteractive_tabindex (keyboard users must be able to scroll the table when it overflows) -->
    <div class="hidden overflow-x-auto md:block" role="region" tabindex="0" aria-label={m.returns_instruments_table_region()}>
      <table class="w-full text-left text-sm">
        <caption class="sr-only">{m.returns_instruments_title()}</caption>
        <thead class="text-xs text-muted">
          <tr>
            <th scope="col" class="py-2 pr-3 font-medium">{m.returns_instruments_instrument()}</th>
            <th scope="col" class="py-2 pr-3 font-medium">{m.returns_instruments_twr()}</th>
            <th scope="col" class="py-2 pr-3 font-medium">{m.returns_instruments_mwr()}</th>
            <th scope="col" class="py-2 pr-3 font-medium">{m.returns_instruments_opening()}</th>
            <th scope="col" class="py-2 pr-3 font-medium">{m.returns_contributions()}</th>
            <th scope="col" class="py-2 pr-3 font-medium">{m.returns_withdrawals()}</th>
            <th scope="col" class="py-2 pr-3 font-medium">{m.returns_instruments_income()}</th>
            <th scope="col" class="py-2 pr-3 font-medium">{m.returns_instruments_closing()}</th>
            <th scope="col" class="py-2 font-medium">{m.returns_gain()}</th>
          </tr>
        </thead>
        <tbody>
          {#each rows as { row, name, coverage } (row.commodity_id)}
            <tr class="border-t border-border align-top" data-testid="returns-instrument-row">
              <th scope="row" class="py-2 pr-3 font-semibold text-foreground">
                {name}
                {#if row.short}<span class="ml-1 rounded border border-border px-1.5 text-xs font-medium text-muted">{m.returns_instruments_short()}</span>{/if}
                <div class="mt-1 max-w-48 space-y-1 font-normal">{@render coverageNotes(coverage)}</div>
              </th>
              <td class="py-2 pr-3">{@render returnFigure(row.time_weighted, row.time_weighted_annualized, 'returns-instrument-twr')}</td>
              <td class="py-2 pr-3">{@render returnFigure(row.money_weighted_period, row.money_weighted, 'returns-instrument-mwr')}</td>
              {#each [
                { key: 'opening', amount: row.opening_value },
                { key: 'contributions', amount: row.contributions },
                { key: 'withdrawals', amount: row.withdrawals },
                { key: 'income', amount: row.income },
                { key: 'closing', amount: row.closing_value },
                { key: 'gain', amount: row.gain }
              ] as cell, index (cell.key)}
                <td class="py-2 tabular-nums {index < 5 ? 'pr-3' : ''} {amountClass(cell.amount)}" data-testid="returns-instrument-{cell.key}">{formatAmount(cell.amount)}</td>
              {/each}
            </tr>
          {/each}
        </tbody>
      </table>
    </div>
  {/if}

  <div class="border-t border-border pt-3">
    <dl class="text-sm">
      <dt class="text-xs text-muted">{m.returns_instruments_residual()}</dt>
      <dd class="font-medium tabular-nums {amountClass(instruments.not_attributed_gain)}" data-testid="returns-instruments-residual">{formatAmount(instruments.not_attributed_gain)}</dd>
    </dl>
    <p class="mt-1 text-xs text-muted">{m.returns_instruments_residual_help()}</p>
  </div>

  {#if instruments.not_attributed.length > 0}
    <div class="border-t border-border pt-3" data-testid="returns-instruments-not-attributed">
      <h4 class="text-sm font-semibold text-foreground">{m.returns_instruments_not_attributed_title({ count: instruments.not_attributed.length })}</h4>
      <p class="mt-1 text-xs text-muted">{m.returns_instruments_not_attributed_help()}</p>
      <ul class="mt-2 space-y-2 text-sm">
        {#each instruments.not_attributed as item (`${item.transaction_id}-${item.date}`)}
          <li class="text-foreground">
            <a href={`/app/ledger/transactions?transaction_id=${item.transaction_id}`} class="inline-flex min-h-11 items-center font-semibold underline underline-offset-2">{formatDate(item.date)}</a>
            {#if item.instrument_id !== null}· {commodityLabel(item.instrument_id)}{/if}
            · {fallbackReasonText(item.reason)}
            {#if fallbackCountedText(fallbacks, item.transaction_id) !== null}· {fallbackCountedText(fallbacks, item.transaction_id)}{/if}
          </li>
        {/each}
      </ul>
    </div>
  {/if}
</section>
