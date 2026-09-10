<script lang="ts">
  import { createQuery, useQueryClient } from '@tanstack/svelte-query';
  import { authSessionQueryOptions } from '$lib/api/auth';
  import { budgetMonthQueryOptions, budgetQueryKey, setBudgetTarget, setBudgetTreatment, type BudgetTreatment } from '$lib/api/budgets';
  import APIFormError from '$lib/components/api-form-error.svelte';
  import PageHeader from '$lib/components/page-header.svelte';
  import Panel from '$lib/components/panel.svelte';
  import StatePanel from '$lib/components/state-panel.svelte';
  import { formatLedgerAmount, parseDecimalAmount } from '$lib/money/amount';
  import { formatQuantity, joinCommodityAmount } from '$lib/money/format';
  import { getLocale } from '$lib/paraglide/runtime.js';
  import { m } from '$lib/paraglide/messages.js';

  let month = $state('');
  let currencyID = $state(0);
  let drafts = $state<Record<number, string>>({});
  let savingID = $state<number | null>(null);
  let mutationError = $state<unknown>(undefined);
  const periodStart = $derived(month ? `${month}-01` : undefined);
  const query = createQuery(() => budgetMonthQueryOptions(periodStart));
  const session = createQuery(() => authSessionQueryOptions());
  const client = useQueryClient();
  const currency = $derived(query.data?.commodities.find((item) => item.id === currencyID) ?? query.data?.commodities[0]);
  const rows = $derived(query.data?.categories.filter((item) => item.allows_postings) ?? []);
  const selectedTotals = $derived(query.data?.totals.filter((item) => item.amount.commodity_id === currency?.id) ?? []);

  $effect(() => { if (!currencyID && query.data?.commodities[0]) currencyID = query.data.commodities[0].id; });
  $effect(() => { if (!month && query.data?.period_start) month = query.data.period_start.slice(0, 7); });
  $effect(() => { month; currencyID; drafts = {}; mutationError = undefined; });

  function amountFor(category: (typeof rows)[number]) {
    return category.amounts.find((amount) => amount.commodity_id === currency?.id);
  }
  function display(amount: { quantity_value: string; quantity_scale: number } | undefined) {
    if (!amount || !currency) return '—';
    return joinCommodityAmount(currency.symbol || currency.code, formatQuantity(amount.quantity_value, amount.quantity_scale, getLocale()));
  }
  function targetInput(category: (typeof rows)[number]) {
    if (drafts[category.id] !== undefined) return drafts[category.id];
    const target = amountFor(category)?.target;
    return target ? formatLedgerAmount(target.quantity_value, target.quantity_scale) : '';
  }
  function categoryLabel(category: (typeof rows)[number]) { return category.name || category.code || ''; }
  async function saveTarget(categoryID: number) {
    if (!currency || !periodStart || !session.data?.csrf_token) return;
    const parsed = parseDecimalAmount(drafts[categoryID] || '0', { maxScale: currency.scale });
    if (!parsed || parsed.value.startsWith('-')) { mutationError = new Error(m.budgets_invalid_amount()); return; }
    savingID = categoryID; mutationError = undefined;
    try {
      const data = await setBudgetTarget({ categoryID, commodityID: currency.id, periodStart, quantityValue: parsed.value, quantityScale: parsed.scale, csrfToken: session.data.csrf_token });
      client.setQueryData([...budgetQueryKey, periodStart], data);
      const nextDrafts = { ...drafts };
      delete nextDrafts[categoryID];
      drafts = nextDrafts;
    } catch (error) { mutationError = error; } finally { savingID = null; }
  }
  async function treatmentChanged(accountID: number, treatment: BudgetTreatment) {
    if (!periodStart || !session.data?.csrf_token) return;
    savingID = -accountID; mutationError = undefined;
    try {
      await setBudgetTreatment({ accountID, treatment, effectiveFrom: periodStart, csrfToken: session.data.csrf_token });
      await query.refetch();
    } catch (error) { mutationError = error; } finally { savingID = null; }
  }
</script>

<PageHeader title={m.budgets_title()} copy={m.budgets_copy()} />
<div class="mt-6 space-y-6">
  <Panel>
    <div class="grid gap-4 sm:grid-cols-2">
      <label class="text-sm font-semibold">{m.budgets_month()}<input class="mt-2 w-full rounded-(--radius-control) border border-border bg-surface px-3 py-2" type="month" bind:value={month} /></label>
      <label class="text-sm font-semibold">{m.budgets_currency()}<select class="mt-2 w-full rounded-(--radius-control) border border-border bg-surface px-3 py-2" bind:value={currencyID}>{#each query.data?.commodities ?? [] as item}<option value={item.id}>{item.code}</option>{/each}</select></label>
    </div>
  </Panel>

  {#if query.isPending}<StatePanel title={m.budgets_loading()} copy="" />
  {:else if query.isError}<StatePanel title={m.budgets_error()} copy=""><APIFormError error={query.error} /></StatePanel>
  {:else if !query.data?.commodities.length}<StatePanel title={m.budgets_no_currency()} copy="" />
  {:else if !rows.length}<StatePanel title={m.budgets_empty()} copy="" />
  {:else}
    <APIFormError error={mutationError} />
    <div class="grid gap-4 sm:grid-cols-2">
      {#each selectedTotals as total (total.category_type)}
        <Panel>
          <p class="text-sm font-semibold">{total.category_type === 'income' ? m.budgets_income() : m.budgets_expense()}</p>
          <dl class="mt-3 grid grid-cols-3 gap-2 text-sm">
            <div><dt class="text-muted">{m.budgets_target()}</dt><dd class="mt-1 font-semibold">{display(total.amount.target)}</dd></div>
            <div><dt class="text-muted">{m.budgets_actual()}</dt><dd class="mt-1 font-semibold">{display(total.amount.actual)}</dd></div>
            <div><dt class="text-muted">{m.budgets_remaining()}</dt><dd class="mt-1 font-semibold">{display(total.amount.remaining)}</dd></div>
          </dl>
        </Panel>
      {/each}
    </div>
    <Panel>
      <h2 class="text-lg font-semibold">{m.budgets_categories()}</h2>
      <p class="mt-1 text-sm text-muted">{m.budgets_no_rollover()}</p>
      <div class="mt-4 divide-y divide-border">
        {#each rows as category (category.id)}
          {@const amount = amountFor(category)}
          <form class="grid gap-3 py-4 md:grid-cols-[minmax(10rem,1fr)_repeat(3,minmax(8rem,0.7fr))_auto] md:items-end" onsubmit={(event) => { event.preventDefault(); void saveTarget(category.id); }}>
            <div><p class="font-semibold">{category.name || category.code}</p><p class="text-xs text-muted">{category.category_type === 'income' ? m.budgets_income() : m.budgets_expense()}</p></div>
            <label class="text-sm">{m.budgets_target()}<input aria-label={m.budgets_target_for({ name: categoryLabel(category) })} class="mt-1 w-full rounded-(--radius-control) border border-border bg-surface px-3 py-2" inputmode="decimal" value={targetInput(category)} oninput={(event) => { drafts = { ...drafts, [category.id]: event.currentTarget.value }; }} /></label>
            <div><p class="text-xs text-muted">{m.budgets_actual()}</p><p class="mt-2 font-semibold">{display(amount?.actual)}</p></div>
            <div><p class="text-xs text-muted">{m.budgets_remaining()}</p><p class="mt-2 font-semibold">{display(amount?.remaining)}</p></div>
            <button class="rounded-(--radius-control) bg-foreground px-4 py-2 text-sm font-semibold text-background disabled:opacity-50" disabled={savingID===category.id}>{savingID===category.id ? m.budgets_saving() : m.budgets_save()}</button>
          </form>
        {/each}
      </div>
    </Panel>

    <Panel>
      <h2 class="text-lg font-semibold">{m.budgets_accounts()}</h2><p class="mt-1 text-sm text-muted">{m.budgets_accounts_help()}</p>
      <div class="mt-4 grid gap-3 md:grid-cols-2">
        {#each query.data.accounts as account (account.id)}
          <label class="rounded-(--radius-control) border border-border p-3 text-sm"><span class="font-semibold">{account.name || account.code}</span>
            <select class="mt-2 w-full rounded-(--radius-control) border border-border bg-surface px-3 py-2" value={account.treatment} disabled={savingID===-account.id} onchange={(event)=>void treatmentChanged(account.id,(event.currentTarget as HTMLSelectElement).value as BudgetTreatment)}>
              <option value="on_budget">{m.budgets_on_budget()}</option><option value="off_budget">{m.budgets_off_budget()}</option><option value="excluded">{m.budgets_excluded()}</option>
            </select>
          </label>
        {/each}
      </div>
    </Panel>
  {/if}
</div>
