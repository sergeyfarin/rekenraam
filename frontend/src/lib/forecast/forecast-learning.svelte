<script lang="ts">
  import { m } from '$lib/paraglide/messages.js';
  import StatusBadge from '$lib/components/status-badge.svelte';
  import {
    forecastLearningPatterns,
    forecastLearningMaxCategories,
    type ForecastLearningPattern,
    type ForecastLearnedSpending
  } from './forecast-model';
  import type { ForecastLearningOption } from '$lib/api/forecast';

  let {
    learned,
    enabled,
    historyCompleteFrom,
    historyError,
    expenseCategoryIDs,
    expensePatterns,
    options,
    formatAmount,
    formatDate,
    onToggle,
    onHistoryChange,
    onCategoryToggle,
    onPatternChange
  }: {
    learned: ForecastLearnedSpending | null | undefined;
    enabled: boolean;
    historyCompleteFrom: string;
    historyError: boolean;
    expenseCategoryIDs: number[];
    expensePatterns: Record<number, ForecastLearningPattern>;
    options: ForecastLearningOption[];
    formatAmount: (value: string, scale: number) => string;
    formatDate: (date: string) => string;
    onToggle: (value: boolean) => void;
    onHistoryChange: (value: string) => void;
    onCategoryToggle: (id: number, selected: boolean) => void;
    onPatternChange: (id: number, pattern: ForecastLearningPattern) => void;
  } = $props();

  const patternLabels: Record<ForecastLearningPattern, () => string> = {
    daily: m.forecast_learning_pattern_daily,
    weekly: m.forecast_learning_pattern_weekly,
    monthly: m.forecast_learning_pattern_monthly,
    annual_seasonal: m.forecast_learning_pattern_annual_seasonal
  };

  // Every backend code is translated here; a raw code is never shown.
  function reasonText(reason: string): string {
    const table: Record<string, () => string> = {
      insufficient_history: m.forecast_learning_reason_insufficient_history,
      insufficient_seasonal_history: m.forecast_learning_reason_insufficient_seasonal_history,
      sparse_history: m.forecast_learning_reason_sparse_history,
      recurring_overlap: m.forecast_learning_reason_recurring_overlap,
      ambiguous_known_spending: m.forecast_learning_reason_ambiguous_known_spending,
      resource_limit: m.forecast_learning_reason_resource_limit,
      busy: m.forecast_learning_reason_busy,
      calendar_limit: m.forecast_learning_reason_calendar_limit,
      computation_timeout: m.forecast_learning_reason_computation_timeout
    };
    return (table[reason] ?? m.forecast_learning_reason_resource_limit)();
  }

  function warningText(warning: string): string {
    const table: Record<string, () => string> = {
      uniform_timing_fallback: m.forecast_learning_warning_uniform_timing_fallback,
      seasonality_not_better_than_flat: m.forecast_learning_warning_seasonality_not_better_than_flat,
      sparse_annual_observations: m.forecast_learning_warning_sparse_annual_observations
    };
    return (table[warning] ?? (() => warning))();
  }

  function fallbackText(reason: string | null): string {
    if (!reason) return '';
    return reason === 'perfect_baseline'
      ? m.forecast_learning_fallback_perfect_baseline()
      : m.forecast_learning_fallback_no_validated_improvement();
  }

  const unitLabel = (unit: string) => (unit === 'month' ? m.forecast_learning_unit_month() : m.forecast_learning_unit_week());
  const optionName = (option: ForecastLearningOption) => option.name ?? option.code ?? `#${option.account_id}`;
  const categoryName = (id: number) => {
    const option = options.find((candidate) => candidate.account_id === id);
    return option ? optionName(option) : `#${id}`;
  };
  const monthNames = $derived(
    Array.from({ length: 12 }, (_, index) =>
      new Intl.DateTimeFormat(undefined, { month: 'short' }).format(new Date(Date.UTC(2024, index, 1)))
    )
  );
  const statusTone = $derived(
    learned?.status === 'ready' ? 'positive' : learned?.status === 'partial' ? 'warning' : 'neutral'
  );
  const selectionFull = $derived(expenseCategoryIDs.length >= forecastLearningMaxCategories);
</script>

<section class="mt-6 rounded-(--radius-panel) border border-border bg-surface p-4 sm:p-5" aria-labelledby="forecast-learning-heading">
  <div class="flex flex-wrap items-start justify-between gap-3">
    <div>
      <h3 id="forecast-learning-heading" class="text-base font-semibold text-foreground">{m.forecast_learning_heading()}</h3>
      <p class="mt-1 max-w-prose text-sm leading-6 text-muted">{m.forecast_learning_intro()}</p>
    </div>
    {#if enabled && learned}
      <StatusBadge tone={statusTone}>{m.forecast_learning_heading()}</StatusBadge>
    {/if}
  </div>

  <label class="mt-4 flex items-center gap-2 text-sm text-foreground">
    <input
      type="checkbox"
      class="size-4 rounded border-border"
      checked={enabled}
      onchange={(event) => onToggle(event.currentTarget.checked)}
      data-testid="forecast-learning-toggle"
    />
    {m.forecast_learning_enable()}
  </label>

  {#if enabled}
    <p class="mt-2 max-w-prose text-xs leading-5 text-muted">{m.forecast_learning_estimate_notice()}</p>

    <div class="mt-4 grid gap-4 sm:max-w-md">
      <label class="grid gap-1 text-sm">
        <span class="font-medium text-foreground">{m.forecast_learning_history_label()}</span>
        <input
          type="date"
          class="rounded-(--radius-control) border border-border bg-surface-strong px-3 py-2 text-sm"
          value={historyCompleteFrom}
          aria-invalid={historyError}
          aria-describedby="forecast-learning-history-help"
          onchange={(event) => onHistoryChange(event.currentTarget.value)}
          data-testid="forecast-learning-history"
        />
        <span id="forecast-learning-history-help" class="text-xs leading-5 text-muted">{m.forecast_learning_history_help()}</span>
        {#if historyError}
          <span class="text-xs text-danger" role="alert">{m.forecast_learning_history_error()}</span>
        {/if}
      </label>
    </div>

    {#if options.length > 0}
      <fieldset class="mt-5">
        <legend class="text-sm font-medium text-foreground">{m.forecast_learning_categories()}</legend>
        <p class="mt-1 text-xs leading-5 text-muted">{m.forecast_learning_categories_help()}</p>
        <ul class="mt-3 grid gap-2">
          {#each options as option (option.account_id)}
            {@const selected = expenseCategoryIDs.includes(option.account_id)}
            <li class="flex flex-wrap items-center gap-3">
              <label class="flex flex-1 items-center gap-2 text-sm text-foreground">
                <input
                  type="checkbox"
                  class="size-4 rounded border-border"
                  checked={selected}
                  disabled={!selected && selectionFull}
                  onchange={(event) => onCategoryToggle(option.account_id, event.currentTarget.checked)}
                />
                {optionName(option)}
              </label>
              <label class="flex items-center gap-2 text-xs text-muted">
                <span>{m.forecast_learning_pattern()}</span>
                <select
                  class="rounded-(--radius-control) border border-border bg-surface-strong px-2 py-1 text-xs"
                  value={expensePatterns[option.account_id] ?? 'weekly'}
                  aria-label={`${m.forecast_learning_pattern()} — ${optionName(option)}`}
                  onchange={(event) => onPatternChange(option.account_id, event.currentTarget.value as ForecastLearningPattern)}
                >
                  {#each forecastLearningPatterns as pattern (pattern)}
                    <option value={pattern}>{patternLabels[pattern]()}</option>
                  {/each}
                </select>
              </label>
            </li>
          {/each}
        </ul>
      </fieldset>
    {/if}

    {#if learned}
      <p class="mt-5 text-sm leading-6 text-foreground" data-testid="forecast-learning-status">
        {#if learned.status === 'ready'}
          {m.forecast_learning_status_ready()}
        {:else if learned.status === 'partial'}
          {m.forecast_learning_status_partial({ eligible: learned.eligible_group_count, requested: learned.requested_group_count })}
        {:else}
          {m.forecast_learning_status_unavailable()}
          {#if learned.reason}<span class="block text-muted">{reasonText(learned.reason)}</span>{/if}
        {/if}
      </p>

      {#if learned.groups.length === 0 && learned.status !== 'unavailable'}
        <p class="mt-2 text-sm text-muted">{m.forecast_learning_no_groups()}</p>
      {/if}

      {#each learned.groups as group (`${group.funding_account_id}:${group.category_account_id}:${group.commodity_id}`)}
        <article class="mt-4 rounded-(--radius-control) border border-border bg-surface-strong/40 p-3">
          <h4 class="text-sm font-semibold text-foreground">
            {categoryName(group.category_account_id)} · {patternLabels[group.pattern]()}
          </h4>
          <p class="mt-1 text-xs leading-5 text-muted">{m.forecast_learning_model_label({ method: group.selected_method })}</p>
          {#if group.fallback_reason}
            <p class="mt-1 text-xs leading-5 text-muted">{fallbackText(group.fallback_reason)}</p>
          {/if}
          <p class="mt-1 text-xs leading-5 text-muted">
            {m.forecast_learning_training_window({
              count: group.complete_periods,
              unit: unitLabel(group.period_unit),
              start: formatDate(group.training_start_date),
              end: formatDate(group.training_end_date),
              positive: group.positive_periods
            })}
          </p>
          {#if group.test_start_date && group.test_end_date}
            <p class="mt-1 text-xs leading-5 text-muted">
              {m.forecast_learning_tested_horizon({
                count: group.tested_horizon_periods,
                unit: unitLabel(group.period_unit),
                start: formatDate(group.test_start_date),
                end: formatDate(group.test_end_date)
              })}
            </p>
          {/if}
          <p class="mt-1 text-xs leading-5 text-muted">
            {m.forecast_learning_variation({
              minimum: formatAmount(group.minimum_observed.quantity_value, group.minimum_observed.quantity_scale),
              maximum: formatAmount(group.maximum_observed.quantity_value, group.maximum_observed.quantity_scale)
            })}
          </p>
          <p class="mt-1 text-xs leading-5 text-foreground">
            {m.forecast_learning_estimated_total({
              amount: formatAmount(group.estimated_total.quantity_value, group.estimated_total.quantity_scale)
            })}
          </p>
          {#each group.warnings as warning (warning)}
            <p class="mt-2 rounded-(--radius-control) border border-warning/40 bg-warning/10 px-2.5 py-1.5 text-xs leading-5 text-warning-foreground">
              {warningText(warning)}
            </p>
          {/each}

          {#if group.month_profile.length > 0}
            <h5 class="mt-3 text-xs font-semibold text-foreground">{m.forecast_learning_month_profile()}</h5>
            <div class="mt-1 overflow-x-auto">
              <table class="w-full min-w-[28rem] text-left text-xs">
                <thead>
                  <tr class="text-muted">
                    <th scope="col" class="py-1 pr-3 font-medium">{m.forecast_learning_month_profile()}</th>
                    <th scope="col" class="py-1 pr-3 font-medium">{m.forecast_learning_estimated_total({ amount: '' })}</th>
                    <th scope="col" class="py-1 pr-3 font-medium">{m.forecast_learning_month_observations({ count: '', positive: '' })}</th>
                  </tr>
                </thead>
                <tbody>
                  {#each group.month_profile as month (month.month)}
                    <tr class="border-t border-border">
                      <th scope="row" class="py-1 pr-3 font-normal text-foreground">{monthNames[month.month - 1]}</th>
                      <td class="py-1 pr-3 tabular-nums text-foreground">{formatAmount(month.estimate.quantity_value, month.estimate.quantity_scale)}</td>
                      <td class="py-1 pr-3 text-muted">
                        {m.forecast_learning_month_observations({ count: month.observations, positive: month.positive_observations })}
                      </td>
                    </tr>
                  {/each}
                </tbody>
              </table>
            </div>
          {/if}
        </article>
      {/each}

      {#if learned.exclusions.length > 0}
        <h4 class="mt-5 text-sm font-semibold text-foreground">{m.forecast_learning_excluded_heading()}</h4>
        <ul class="mt-2 grid gap-2">
          {#each learned.exclusions as exclusion (`${exclusion.funding_account_id}:${exclusion.category_account_id}:${exclusion.commodity_id}:${exclusion.reason}`)}
            <li class="text-xs leading-5 text-muted">
              <span class="font-medium text-foreground">{categoryName(exclusion.category_account_id)}</span>
              — {reasonText(exclusion.reason)}
            </li>
          {/each}
        </ul>
      {/if}
    {/if}
  {/if}
</section>
