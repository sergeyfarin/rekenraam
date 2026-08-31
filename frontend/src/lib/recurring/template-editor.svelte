<script lang="ts">
  import { createQuery } from '@tanstack/svelte-query';
  import { untrack } from 'svelte';
  import TransactionEditor from '$lib/transactions/transaction-editor.svelte';
  import APIFormError from '$lib/components/api-form-error.svelte';
  import { m } from '$lib/paraglide/messages.js';
  import { getLocale } from '$lib/paraglide/runtime.js';
  import { createRecurringTemplate, updateRecurringTemplate, previewRecurringSchedule, type RecurringTemplate, type RecurringTemplatePatch, type CreateRecurringTemplateRequest } from '$lib/api/recurring';
  import type { TransactionRequest } from '$lib/api/transactions';
  import { templateValues, templatePatch, weekdayLabel, displayDate } from './recurring-model';

  let { template, today, csrfToken, onSaved, onCancel, onPendingChange } = $props<{
    template?: RecurringTemplate; today: string; csrfToken: string;
    onSaved: () => Promise<void>; onCancel: () => void; onPendingChange: (pending:boolean) => void;
  }>();
  // The enclosing keyed dialog gives each editor one initial snapshot. Query
  // refreshes must never overwrite an in-progress form.
  const initial = untrack(() => templateValues(template, today));
  let name = $state(untrack(() => template?.name ?? ''));
  let enabled = $state(untrack(() => template?.enabled ?? true));
  let frequency = $state<RecurringTemplate['frequency']>(untrack(() => template?.frequency ?? 'monthly'));
  let interval = $state(untrack(() => template?.interval_count ?? 1));
  let weekday = $state(untrack(() => template?.by_weekday ?? 1));
  let day = $state(untrack(() => template?.day_of_month ?? 1));
  let lastDay = $state(untrack(() => template?.last_day_of_month ?? false));
  let month = $state(untrack(() => template?.month_of_year ?? 1));
  let starts = $state(untrack(() => template?.starts_on ?? today));
  let ends = $state(untrack(() => template?.ends_on ?? ''));
  let maxOccurrences = $state<number | undefined>(untrack(() => template?.max_occurrences ?? undefined));
  let lead = $state(untrack(() => template?.lead_days ?? 5));
  const locale = $derived(getLocale());
  const schedule = $derived<RecurringTemplatePatch>({
    frequency, interval_count: interval,
    by_weekday: frequency === 'weekly' ? weekday : null,
    day_of_month: ['monthly', 'yearly'].includes(frequency) && !lastDay ? day : null,
    last_day_of_month: ['monthly', 'yearly'].includes(frequency) && lastDay,
    month_of_year: frequency === 'yearly' ? month : null,
    starts_on: starts, ends_on: ends || null, max_occurrences: maxOccurrences ?? null
  });
  let previewSchedule = $state<RecurringTemplatePatch | undefined>();
  $effect(() => {
    const snapshot = schedule;
    const timer = setTimeout(() => { previewSchedule = snapshot; }, 250);
    return () => clearTimeout(timer);
  });
  const preview = createQuery(() => ({
    queryKey: ['api', 'recurring', 'preview', previewSchedule],
    queryFn: () => previewRecurringSchedule(previewSchedule!), enabled: !!previewSchedule,
    retry: false, staleTime: 30_000
  }));
  async function save(entry: TransactionRequest) {
    const patch = templatePatch({ ...schedule, name, enabled, lead_days: lead }, entry, template);
    if (template) await updateRecurringTemplate(template.id, patch, csrfToken);
    else await createRecurringTemplate(patch as CreateRecurringTemplateRequest, csrfToken);
    await onSaved();
  }
  const inputClass = 'mt-1 min-h-10 w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground focus:outline-accent';
</script>

<TransactionEditor mode="template" initialValues={initial} {csrfToken} saveTemplate={save} {onCancel} {onPendingChange}>
  {#snippet templateFields()}
    <fieldset class="space-y-4 rounded-(--radius-control) border border-border p-4">
      <legend class="px-1 font-semibold">{m.recurring_schedule()}</legend>
      <div class="grid gap-4 sm:grid-cols-2">
        <label class="text-sm">{m.recurring_name()}<input class={inputClass} bind:value={name} required maxlength="500" /></label>
        <label class="flex min-h-10 items-center gap-2 text-sm"><input type="checkbox" bind:checked={enabled} />{m.recurring_enabled()}</label>
        <label class="text-sm">{m.recurring_frequency()}<select class={inputClass} bind:value={frequency}>
          <option value="daily">{m.recurring_daily()}</option><option value="weekly">{m.recurring_weekly()}</option>
          <option value="monthly">{m.recurring_monthly()}</option><option value="yearly">{m.recurring_yearly()}</option>
        </select></label>
        <label class="text-sm">{m.recurring_interval()}<input class={inputClass} type="number" min="1" max="10000" step="1" bind:value={interval} required /></label>
        {#if frequency === 'weekly'}
          <label class="text-sm">{m.recurring_weekday()}<select class={inputClass} bind:value={weekday}>{#each [0,1,2,3,4,5,6] as value}<option value={value}>{weekdayLabel(value, locale)}</option>{/each}</select></label>
        {/if}
        {#if frequency === 'monthly' || frequency === 'yearly'}
          <label class="flex min-h-10 items-center gap-2 text-sm"><input type="checkbox" bind:checked={lastDay} />{m.recurring_last_day()}</label>
          {#if !lastDay}<label class="text-sm">{m.recurring_day()}<input class={inputClass} type="number" min="1" max="31" step="1" bind:value={day} required /></label>{/if}
        {/if}
        {#if frequency === 'yearly'}<label class="text-sm">{m.recurring_month()}<input class={inputClass} type="number" min="1" max="12" step="1" bind:value={month} required /></label>{/if}
        <label class="text-sm">{m.recurring_starts()}<input class={inputClass} type="date" bind:value={starts} required /></label>
        <label class="text-sm">{m.recurring_ends()}<input class={inputClass} type="date" bind:value={ends} min={starts} /></label>
        <label class="text-sm">{m.recurring_limit()}<input class={inputClass} type="number" min="1" step="1" bind:value={maxOccurrences} /></label>
        <label class="text-sm">{m.recurring_lead()}<input class={inputClass} type="number" min="0" max="90" step="1" bind:value={lead} required /></label>
      </div>
      <p class="text-sm text-muted">{m.recurring_schedule_help()}</p>
      <section aria-label={m.recurring_preview()} class="rounded-(--radius-control) bg-surface-strong p-3" aria-live="polite">
        <h3 class="text-sm font-semibold">{m.recurring_preview()}</h3>
        {#if preview.isPending || preview.isFetching}<p class="text-sm text-muted">{m.recurring_loading()}</p>
        {:else if preview.isError}<APIFormError error={preview.error} />
        {:else if !preview.data?.dates.length}<p class="text-sm text-muted">{m.recurring_no_dates()}</p>
        {:else}<ol class="mt-2 flex flex-wrap gap-x-5 gap-y-2 text-sm">{#each preview.data.dates as date}<li><time datetime={date}>{displayDate(date, locale)}</time></li>{/each}</ol>{/if}
      </section>
    </fieldset>
  {/snippet}
</TransactionEditor>
