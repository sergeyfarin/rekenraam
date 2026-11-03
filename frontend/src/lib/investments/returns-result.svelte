<script lang="ts">
  import Panel from '#lib/components/panel.svelte';
  import type { CurrencyResponse } from '#lib/api/currencies.ts';
  import type { InvestmentReturnsResponse, ReturnFigure, ReturnsAmount } from '#lib/api/investments.ts';
  import { formatMoney, formatQuantity, joinCommodityAmount } from '#lib/money/format.ts';
  import { m } from '#lib/paraglide/messages.js';
  import {
    benchmarkBasisText, benchmarkGaps, fallbackReasonText, formatReturn, returnDirection, returnReasonText, returnsCoverage
  } from './returns-model';
  import ReturnsChart from './returns-chart.svelte';
  import ReturnsAllocation from './returns-allocation.svelte';
  import ReturnsInstruments from './returns-instruments.svelte';

  // One returns_v1 result, live or saved (R13 slices 2-6, and slice 9's
  // per-instrument panel when the read asked for it). Every figure comes
  // from the result; names come from the lookups, which are today's
  // reference data, so a saved run fixes only ids, dates and figures.
  // Amounts were rounded to the reporting currency's standard scale when
  // read, so a saved one is shown at its own stored scale: a later change
  // to the currency's precision can never round a frozen figure again.
  let {
    result, reportingCurrencyID, currencies, locale, accountName, currencyOrCommodity, fetching = false, saved = false
  }: {
    result: InvestmentReturnsResponse;
    reportingCurrencyID: number;
    currencies: CurrencyResponse[];
    locale: string;
    accountName: (id: number) => string;
    currencyOrCommodity: (id: number) => string;
    fetching?: boolean;
    saved?: boolean;
  } = $props();

  // A saved run can sit beside the live result, so element ids stay unique.
  const idPrefix = $props.id();
  const dates = $derived(new Intl.DateTimeFormat(locale, { dateStyle: 'medium', timeZone: 'UTC' }));
  const instants = $derived(new Intl.DateTimeFormat(locale, { dateStyle: 'medium', timeStyle: 'short' }));
  const coverage = $derived(returnsCoverage(result.valuation));
  const seriesCoverage = $derived(returnsCoverage(result.time_weighted_series.valuation));
  const benchmarkCoverage = $derived(returnsCoverage(result.benchmark?.valuation));
  const benchmarkMissing = $derived(benchmarkGaps(result.benchmark?.valuation));
  const benchmarkName = $derived(result.benchmark ? currencyOrCommodity(result.benchmark.commodity_id) : '');

  function currencyCode(id: number): string {
    return currencies.find((currency) => currency.id === id)?.code ?? `#${id}`;
  }

  function day(date: string): string {
    return dates.format(new Date(`${date}T00:00:00Z`));
  }

  function amountText(amount: ReturnsAmount): string {
    if (!amount.amount) return returnReasonText(amount.reason, amount.date, day);
    const currency = currencies.find((candidate) => candidate.id === amount.amount?.commodity_id);
    const { quantity_value: value, quantity_scale: scale } = amount.amount;
    const formatted = saved ? formatQuantity(value, scale, locale) : formatMoney(value, scale, currency?.standard_scale ?? scale, locale);
    return joinCommodityAmount(currency?.code ?? `#${amount.amount.commodity_id}`, formatted);
  }

  function directionClass(figure: ReturnFigure): string {
    if (figure.value === null) return 'text-muted';
    const direction = returnDirection(figure.value);
    return direction === 'up' ? 'text-positive' : direction === 'down' ? 'text-danger' : 'text-foreground';
  }
</script>

<section aria-labelledby={`${idPrefix}-summary-title`} class="space-y-3" data-testid={saved ? 'returns-run-summary' : 'returns-summary'} aria-busy={fetching}>
  {#if fetching}
    <p role="status" class="text-sm text-muted">{m.returns_updating()}</p>
  {/if}
  <h2 id={`${idPrefix}-summary-title`} class="text-base font-semibold text-foreground">
    {m.returns_period({ from: day(result.from), to: day(result.to), currency: currencyCode(reportingCurrencyID), days: result.days })}
  </h2>
  <div class="grid grid-cols-1 gap-3 md:grid-cols-2">
    {#each [
      { key: 'twr', title: m.returns_twr_title(), help: m.returns_twr_help(), period: result.time_weighted, annual: result.time_weighted_annualized },
      { key: 'mwr', title: m.returns_mwr_title(), help: m.returns_mwr_help(), period: result.money_weighted_period, annual: result.money_weighted }
    ] as card (card.key)}
      <Panel>
        <h3 class="text-sm font-semibold text-foreground">{card.title}</h3>
        <p class="mt-1 text-xs text-muted">{card.help}</p>
        <dl class="mt-3 grid grid-cols-2 gap-3">
          <div>
            <dt class="text-xs text-muted">{m.returns_figure_period()}</dt>
            <dd class="text-xl font-semibold tabular-nums {directionClass(card.period)}" data-testid="returns-{card.key}-period">
              {card.period.value !== null ? formatReturn(card.period.value, locale) : '—'}
            </dd>
            {#if card.period.value === null}<dd class="text-xs text-muted">{returnReasonText(card.period.reason, card.period.date, day)}</dd>{/if}
          </div>
          <div>
            <dt class="text-xs text-muted">{m.returns_figure_annual()}</dt>
            <dd class="text-xl font-semibold tabular-nums {directionClass(card.annual)}" data-testid="returns-{card.key}-annual">
              {card.annual.value !== null ? formatReturn(card.annual.value, locale) : '—'}
            </dd>
            {#if card.annual.value === null}<dd class="text-xs text-muted">{returnReasonText(card.annual.reason, card.annual.date, day)}</dd>{/if}
          </div>
        </dl>
      </Panel>
    {/each}
  </div>

  {#if result.benchmark}
    {@const benchmark = result.benchmark}
    <Panel>
      <section aria-labelledby={`${idPrefix}-benchmark-title`} data-testid="returns-benchmark">
        <h3 id={`${idPrefix}-benchmark-title`} class="text-sm font-semibold text-foreground">{m.returns_benchmark_title({ name: benchmarkName })}</h3>
        <p class="mt-1 text-xs text-muted">{benchmarkBasisText(benchmark.basis)} · {m.returns_benchmark_card_help()}</p>
        <dl class="mt-3 grid grid-cols-2 gap-3">
          {#each [
            { key: 'period', label: m.returns_figure_period(), figure: benchmark.time_weighted },
            { key: 'annual', label: m.returns_figure_annual(), figure: benchmark.time_weighted_annualized }
          ] as line (line.key)}
            <div>
              <dt class="text-xs text-muted">{line.label}</dt>
              <dd class="text-xl font-semibold tabular-nums {directionClass(line.figure)}" data-testid="returns-benchmark-{line.key}">
                {line.figure.value !== null ? formatReturn(line.figure.value, locale) : '—'}
              </dd>
              {#if line.figure.value === null}<dd class="text-xs text-muted">{returnReasonText(line.figure.reason, line.figure.date, day)}</dd>{/if}
            </div>
          {/each}
        </dl>
        {#if benchmark.splits.length > 0}
          <p class="mt-3 text-sm text-muted" data-testid="returns-benchmark-splits">
            {m.returns_benchmark_splits({ splits: benchmark.splits.map((split) => m.returns_benchmark_split({
              ratio: `${split.ratio_numerator}:${split.ratio_denominator}`, date: day(split.effective_on) })).join(', ') })}
          </p>
        {/if}
        {#each benchmarkMissing as gap (gap.kind === 'exchange_rate' ? `fx-${gap.currencyID}` : 'price')}
          <p role="status" class="mt-3 text-sm text-warning">
            {#if gap.kind === 'exchange_rate'}
              {m.returns_benchmark_coverage_fx({ name: benchmarkName, currency: currencyCode(gap.currencyID) })}
            {:else if benchmark.basis === 'total_return'}
              {m.returns_benchmark_coverage_total({ name: benchmarkName })}
            {:else}
              {m.returns_benchmark_coverage({ name: benchmarkName })}
            {/if}
          </p>
        {/each}
        {#if benchmarkCoverage.staleDates.length > 0}
          <p class="mt-2 text-sm text-muted">{m.returns_benchmark_coverage_stale({ dates: benchmarkCoverage.staleDates.map(day).join(', ') })}</p>
        {/if}
      </section>
    </Panel>
  {/if}

  <Panel>
    <ReturnsChart series={result.time_weighted_series} {locale} formatDate={day}
      benchmark={result.benchmark ? { name: benchmarkName, points: result.benchmark.points } : null}
      missingCommodities={seriesCoverage.missing.map(currencyOrCommodity)} staleDates={seriesCoverage.staleDates} />
  </Panel>

  <Panel>
    <h3 class="text-sm font-semibold text-foreground">{m.returns_values_title()}</h3>
    <dl class="mt-3 grid grid-cols-1 gap-3 text-sm sm:grid-cols-2 lg:grid-cols-5">
      {#each [
        { key: 'opening', label: m.returns_opening_value({ date: day(result.opening_date) }), amount: result.opening_value },
        { key: 'contributions', label: m.returns_contributions(), amount: result.contributions },
        { key: 'withdrawals', label: m.returns_withdrawals(), amount: result.withdrawals },
        { key: 'gain', label: m.returns_gain(), amount: result.gain },
        { key: 'closing', label: m.returns_closing_value({ date: day(result.to) }), amount: result.closing_value }
      ] as line (line.key)}
        <div>
          <dt class="text-xs text-muted">{line.label}</dt>
          <dd class="font-medium tabular-nums {line.amount.amount ? 'text-foreground' : 'text-muted'}" data-testid="returns-{line.key}">{amountText(line.amount)}</dd>
        </div>
      {/each}
    </dl>
  </Panel>

  <Panel>
    <ReturnsAllocation allocation={result.allocation} {locale} formatDate={day}
      formatAmount={(amount) => amountText({ amount, reason: null, date: null })}
      {accountName} {currencyCode} commodityLabel={currencyOrCommodity} />
  </Panel>

  {#if result.instrument_returns}
    <Panel>
      <ReturnsInstruments instruments={result.instrument_returns} fallbacks={result.attribution_fallbacks} {locale} formatDate={day} formatAmount={amountText}
        commodityLabel={currencyOrCommodity} />
    </Panel>
  {/if}

  {#if coverage.missing.length > 0}
    <p role="status" class="text-sm text-warning">{m.returns_coverage_incomplete({ commodities: coverage.missing.map(currencyOrCommodity).join(', ') })}</p>
  {/if}
  {#if coverage.staleDates.length > 0}
    <p class="text-sm text-muted">{m.returns_coverage_stale({ dates: coverage.staleDates.map(day).join(', ') })}</p>
  {/if}

  {#if result.attribution_fallbacks.length > 0}
    <Panel>
      <h3 class="text-sm font-semibold text-foreground">{m.returns_fallbacks_title({ count: result.attribution_fallbacks.length })}</h3>
      <p class="mt-1 text-xs text-muted">{m.returns_fallbacks_help()}</p>
      <ul class="mt-3 space-y-2 text-sm">
        {#each result.attribution_fallbacks as fallback (`${fallback.transaction_id}-${fallback.date}`)}
          <li class="text-foreground">
            <a href={`/app/ledger/transactions?transaction_id=${fallback.transaction_id}`} class="inline-flex min-h-11 items-center font-semibold underline underline-offset-2">{day(fallback.date)}</a>
            · {fallbackReasonText(fallback.reason)}
            · {fallback.counted ? m.returns_fallback_counted() : m.returns_fallback_not_counted()}
          </li>
        {/each}
      </ul>
    </Panel>
  {/if}

  <Panel>
    <h3 class="text-sm font-semibold text-foreground">{m.returns_scope_title()}</h3>
    <p class="mt-1 text-sm text-muted">
      {result.account_scope === 'default_investment_accounts' ? m.returns_scope_default() : m.returns_scope_selected()}
    </p>
    <p class="mt-2 text-sm text-foreground">{result.resolved_account_ids.map(accountName).join(', ')}</p>
    <p class="mt-3 text-xs text-muted">{saved ? m.returns_run_frozen({ cutoff: instants.format(new Date(result.knowledge_cutoff)) }) : m.returns_read_only({ cutoff: instants.format(new Date(result.knowledge_cutoff)) })}</p>
  </Panel>
</section>
