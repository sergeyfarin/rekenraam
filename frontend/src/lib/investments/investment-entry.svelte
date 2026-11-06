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

  type Activity = 'write-off' | 'buy' | 'sell' | 'short-open' | 'short-cover' | 'payment-in-lieu' | 'dividend' | 'withholding' | 'reinvested' |
    'external-transfer-in' | 'external-transfer-out' | 'internal-transfer' | 'split' | 'share-exchange' |
    'cash-and-stock-merger' | 'spin-off' | 'capital-return';

  const activities: { id: Activity; label: () => string; titleID?: string; group: 'trades' | 'income' | 'shares' | 'corporate' }[] = [
    { id: 'buy', group: 'trades', label: m.investments_activity_buy, titleID: 'buy-title' },
    { id: 'sell', group: 'trades', label: m.investments_activity_sell, titleID: 'sell-title' },
    { id: 'write-off', group: 'shares', label: m.investments_activity_write_off },
    { id: 'short-open', group: 'trades', label: m.investments_activity_short_open, titleID: 'short-open-title' },
    { id: 'short-cover', group: 'trades', label: m.investments_activity_short_cover, titleID: 'short-cover-title' },
    { id: 'payment-in-lieu', group: 'income', label: m.investments_activity_payment_in_lieu, titleID: 'payment-in-lieu-title' },
    { id: 'dividend', group: 'income', label: m.investments_activity_dividend, titleID: 'dividend-title' },
    { id: 'withholding', group: 'income', label: m.investments_activity_withholding, titleID: 'withholding-title' },
    { id: 'reinvested', group: 'income', label: m.investments_activity_reinvested, titleID: 'reinvested-dividend-title' },
    { id: 'external-transfer-in', group: 'shares', label: m.investments_activity_external_transfer_in, titleID: 'external-transfer-in-title' },
    { id: 'internal-transfer', group: 'shares', label: m.investments_activity_internal_transfer, titleID: 'internal-transfer-title' },
    { id: 'external-transfer-out', group: 'shares', label: m.investments_activity_external_transfer_out, titleID: 'external-transfer-out-title' },
    { id: 'split', group: 'corporate', label: m.investments_activity_split, titleID: 'split-title' },
    { id: 'share-exchange', group: 'corporate', label: m.investments_activity_share_exchange, titleID: 'share-exchange-title' },
    { id: 'cash-and-stock-merger', group: 'corporate', label: m.investments_activity_cash_and_stock_merger, titleID: 'merger-title' },
    { id: 'spin-off', group: 'corporate', label: m.investments_activity_spin_off, titleID: 'spin-off-title' },
    { id: 'capital-return', group: 'corporate', label: m.investments_activity_capital_return, titleID: 'capital-return-title' }
  ];

  let active = $state<Activity | null>(null);
  let selected = $state('');
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
  <select id="investment-activity" bind:value={selected}
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
  <button type="button" onclick={open} disabled={!selected}
    class="min-h-11 rounded-(--radius-control) bg-foreground px-4 py-2 text-sm font-semibold text-background disabled:opacity-50">{m.investments_activity_continue()}</button>
  <p class="text-xs text-muted">{m.investments_activity_help()}</p>
  {#if onCancel}<button type="button" onclick={onCancel} class="min-h-11 rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground">{m.transactions_form_cancel()}</button>{/if}
</div>

{#if active === 'write-off'}
  <WriteOffForm {csrfToken} {prefill} onSaved={saved} onCancel={close} />
{:else if active !== null}
  <ModalDialog labelledby={titleID} onCancel={close} closeOnBackdrop fallbackFocus={() => opener} class="max-w-lg">
  <div class="p-6">
    {#if active === 'buy'}
      <BuyForm {csrfToken} {prefill} onSaved={saved} onCancel={close} />
    {:else if active === 'sell'}
      <SellForm {csrfToken} {prefill} onSaved={saved} onCancel={close} />
    {:else if active === 'short-open'}
      <ShortTradeForm mode="open" {csrfToken} {prefill} onSaved={saved} onCancel={close} />
    {:else if active === 'short-cover'}
      <ShortTradeForm mode="cover" {csrfToken} {prefill} onSaved={saved} onCancel={close} />
    {:else if active === 'payment-in-lieu'}
      <PaymentInLieuForm {csrfToken} {prefill} onSaved={saved} onCancel={close} />
    {:else if active === 'dividend'}
      <DividendForm mode="cash" {csrfToken} {prefill} onSaved={saved} onCancel={close} />
    {:else if active === 'withholding'}
      <DividendForm mode="withholding" {csrfToken} {prefill} onSaved={saved} onCancel={close} />
    {:else if active === 'reinvested'}
      <DividendForm mode="reinvested" {csrfToken} {prefill} onSaved={saved} onCancel={close} />
    {:else if active === 'external-transfer-in'}
      <ExternalTransferInForm {csrfToken} {prefill} onSaved={saved} onCancel={close} />
    {:else if active === 'internal-transfer'}
      <InternalTransferForm {csrfToken} {prefill} onSaved={saved} onCancel={close} />
    {:else if active === 'external-transfer-out'}
      <ExternalTransferOutForm {csrfToken} {prefill} onSaved={saved} onCancel={close} />
    {:else if active === 'split'}
      <SplitForm {csrfToken} {prefill} onSaved={saved} onCancel={close} />
    {:else if active === 'share-exchange'}
      <ShareExchangeForm {csrfToken} {prefill} onSaved={saved} onCancel={close} />
    {:else if active === 'cash-and-stock-merger'}
      <CashAndStockMergerForm {csrfToken} {prefill} onSaved={saved} onCancel={close} />
    {:else if active === 'spin-off'}
      <SpinOffForm {csrfToken} {prefill} onSaved={saved} onCancel={close} />
    {:else if active === 'capital-return'}
      <CapitalReturnForm {csrfToken} {prefill} onSaved={saved} onCancel={close} />
    {/if}
  </div>
  </ModalDialog>
{/if}
