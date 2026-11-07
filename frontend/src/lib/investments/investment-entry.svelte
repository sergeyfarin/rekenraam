<script lang="ts">
  import { createQuery } from '@tanstack/svelte-query';
  import ModalDialog from '#lib/components/modal-dialog.svelte';
  import { authSessionQueryOptions } from '#lib/api/auth.ts';
  import BuyForm from '#lib/investments/buy-form.svelte';
  import SellForm from '#lib/investments/sell-form.svelte';
  import WriteOffForm from '#lib/investments/write-off-form.svelte';
  import DividendForm from '#lib/investments/dividend-form.svelte';
  import ExternalTransferInForm from '#lib/investments/external-transfer-in-form.svelte';
  import InternalTransferForm from '#lib/investments/internal-transfer-form.svelte';
  import ExternalTransferOutForm from '#lib/investments/external-transfer-out-form.svelte';
  import CapitalReturnForm from '#lib/investments/capital-return-form.svelte';
  import SplitForm from '#lib/investments/split-form.svelte';
  import ShareExchangeForm from '#lib/investments/share-exchange-form.svelte';
  import SpinOffForm from '#lib/investments/spin-off-form.svelte';
  import CashAndStockMergerForm from '#lib/investments/cash-and-stock-merger-form.svelte';
  import ShortTradeForm from '#lib/investments/short-trade-form.svelte';
  import PaymentInLieuForm from '#lib/investments/payment-in-lieu-form.svelte';
  import type { InvestmentPrefill } from '#lib/investments/investment-prefill.ts';
  import { investmentActivities, isInvestmentActivity, type InvestmentActivity } from '#lib/investments/activities.ts';
  import { entryContext } from '#lib/transactions/entry-context.svelte.ts';
  import type { InvestmentSaveKind } from '#lib/transactions/add-transaction.svelte.ts';
  import { m } from '#lib/paraglide/messages.js';

  // Investment entry (#507): one Activity selector and the existing forms, shared by
  // the Investments screen and New transaction's Investment tab, which a
  // holding account's register and an instrument's view open (#506). The
  // screen passes what it already knows as `prefill`; every form takes that
  // one shape.
  let {
    prefill,
    onSaved,
    onCancel
  }: {
    prefill?: InvestmentPrefill;
    /** A form saved; New transaction's Investment tab closes the entry (#506). */
    onSaved?: (kind?: InvestmentSaveKind) => void;
    onCancel?: () => void;
  } = $props();

  const sessionQuery = createQuery(() => authSessionQueryOptions());
  const csrfToken = $derived(sessionQuery.data?.csrf_token ?? '');

  const activities = investmentActivities;
  // Inside New transaction the entry remembers the chosen activity and the
  // shared selections (#508); each form opens on the shared context and its
  // own working copy, so switching activity loses nothing.
  const entry = entryContext();

  let active = $state<InvestmentActivity | null>(null);
  let selected = $state<string>(entry?.activity ?? '');
  $effect(() => {
    if (entry) entry.activity = isInvestmentActivity(selected) ? selected : '';
  });
  // While a save is in flight the activity cannot change (#508).
  const locked = $derived(entry?.saving ?? false);
  /** What the opened form starts from: the page's prefill, then the entry's shared context. */
  let openedPrefill = $state.raw<InvestmentPrefill | undefined>(undefined);
  function currentPrefill(): InvestmentPrefill | undefined {
    if (!entry) return prefill;
    const shared = entry.shared;
    return {
      ...prefill,
      holdingAccountID: shared.holdingAccountID ?? prefill?.holdingAccountID,
      commodityID: shared.commodityID ?? prefill?.commodityID,
      currencyID: shared.currencyID,
      transactionDate: entry.date || prefill?.transactionDate,
      memo: entry.memo,
      reference: entry.reference
    };
  }
  const groups = [
    { id: 'trades', label: m.investments_activity_trades },
    { id: 'income', label: m.investments_activity_income },
    { id: 'shares', label: m.investments_activity_shares },
    { id: 'corporate', label: m.investments_activity_corporate }
  ] as const;
  let opener: HTMLElement | null = null;
  let writeOffSaved = $state(false);
  const titleID = $derived(activities.find((activity) => activity.id === active)?.titleID);

  function open(event: MouseEvent) {
    opener = event.currentTarget as HTMLElement;
    if (locked) return;
    openedPrefill = currentPrefill();
    active = activities.find((activity) => activity.id === selected)?.id ?? null;
    writeOffSaved = false;
  }

  function close() {
    active = null;
  }

  // Each form refreshes every investment read itself (invalidateInvestmentReads),
  // which includes the transaction lists and registers.
  function saved() {
    writeOffSaved = active === 'write-off';
    close();
    onSaved?.(writeOffSaved ? 'write-off' : 'activity');
  }
</script>

{#if writeOffSaved}
  <p role="status" class="w-full text-sm text-foreground">{m.investments_write_off_success()}</p>
{/if}

<div class="space-y-2">
  <label for="investment-activity" class="block text-sm font-medium text-foreground">{m.investments_activity()}</label>
  <select id="investment-activity" bind:value={selected} disabled={locked}
    class="min-h-11 w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground">
    <option value="">{m.investments_activity_choose()}</option>
    {#each groups as group (group.id)}
      <optgroup label={group.label()}>
        {#each activities.filter((activity) => activity.group === group.id) as activity (activity.id)}
          <option value={activity.id}>{activity.label()}</option>
        {/each}
      </optgroup>
    {/each}
  </select>
  <button type="button" onclick={open} disabled={!selected || locked}
    class="min-h-11 rounded-(--radius-control) bg-foreground px-4 py-2 text-sm font-semibold text-background disabled:opacity-50">{m.investments_activity_continue()}</button>
  <p class="text-xs text-muted">{m.investments_activity_help()}</p>
  {#if onCancel}<button type="button" onclick={onCancel} class="min-h-11 rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground">{m.transactions_form_cancel()}</button>{/if}
</div>

{#if active === 'write-off'}
  <WriteOffForm {csrfToken} prefill={openedPrefill} onSaved={saved} onCancel={close} />
{:else if active !== null}
  <ModalDialog labelledby={titleID} onCancel={close} closeOnBackdrop fallbackFocus={() => opener} class="max-w-lg">
  <div class="p-6">
    {#if active === 'buy'}
      <BuyForm {csrfToken} prefill={openedPrefill} onSaved={saved} onCancel={close} />
    {:else if active === 'sell'}
      <SellForm {csrfToken} prefill={openedPrefill} onSaved={saved} onCancel={close} />
    {:else if active === 'short-open'}
      <ShortTradeForm mode="open" {csrfToken} prefill={openedPrefill} onSaved={saved} onCancel={close} />
    {:else if active === 'short-cover'}
      <ShortTradeForm mode="cover" {csrfToken} prefill={openedPrefill} onSaved={saved} onCancel={close} />
    {:else if active === 'payment-in-lieu'}
      <PaymentInLieuForm {csrfToken} prefill={openedPrefill} onSaved={saved} onCancel={close} />
    {:else if active === 'dividend'}
      <DividendForm mode="cash" {csrfToken} prefill={openedPrefill} onSaved={saved} onCancel={close} />
    {:else if active === 'withholding'}
      <DividendForm mode="withholding" {csrfToken} prefill={openedPrefill} onSaved={saved} onCancel={close} />
    {:else if active === 'reinvested'}
      <DividendForm mode="reinvested" {csrfToken} prefill={openedPrefill} onSaved={saved} onCancel={close} />
    {:else if active === 'external-transfer-in'}
      <ExternalTransferInForm {csrfToken} prefill={openedPrefill} onSaved={saved} onCancel={close} />
    {:else if active === 'internal-transfer'}
      <InternalTransferForm {csrfToken} prefill={openedPrefill} onSaved={saved} onCancel={close} />
    {:else if active === 'external-transfer-out'}
      <ExternalTransferOutForm {csrfToken} prefill={openedPrefill} onSaved={saved} onCancel={close} />
    {:else if active === 'split'}
      <SplitForm {csrfToken} prefill={openedPrefill} onSaved={saved} onCancel={close} />
    {:else if active === 'share-exchange'}
      <ShareExchangeForm {csrfToken} prefill={openedPrefill} onSaved={saved} onCancel={close} />
    {:else if active === 'cash-and-stock-merger'}
      <CashAndStockMergerForm {csrfToken} prefill={openedPrefill} onSaved={saved} onCancel={close} />
    {:else if active === 'spin-off'}
      <SpinOffForm {csrfToken} prefill={openedPrefill} onSaved={saved} onCancel={close} />
    {:else if active === 'capital-return'}
      <CapitalReturnForm {csrfToken} prefill={openedPrefill} onSaved={saved} onCancel={close} />
    {/if}
  </div>
  </ModalDialog>
{/if}
