<script lang="ts">
  import { createQuery } from '@tanstack/svelte-query';
  import Plus from '@lucide/svelte/icons/plus';
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
  import { m } from '#lib/paraglide/messages.js';

  // Investment entry (#186): the record buttons and their forms, shared by
  // the Investments screen and New transaction's Investment tab, which a
  // holding account's register and an instrument's view open (#506). The
  // screen passes what it already knows as `prefill`; every form takes that
  // one shape.
  let {
    prefill,
    onSaved
  }: {
    prefill?: InvestmentPrefill;
    /** A form saved; New transaction's Investment tab closes the entry (#506). */
    onSaved?: () => void;
  } = $props();

  const sessionQuery = createQuery(() => authSessionQueryOptions());
  const csrfToken = $derived(sessionQuery.data?.csrf_token ?? '');

  type Activity = 'write-off' | 'buy' | 'sell' | 'short-open' | 'short-cover' | 'payment-in-lieu' | 'dividend' | 'withholding' | 'reinvested' |
    'external-transfer-in' | 'external-transfer-out' | 'internal-transfer' | 'split' | 'share-exchange' |
    'cash-and-stock-merger' | 'spin-off' | 'capital-return';

  const activities: { id: Activity; label: () => string; titleID?: string }[] = [
    { id: 'buy', label: m.investments_record_buy },
    { id: 'sell', label: m.investments_record_sell },
    { id: 'write-off', label: m.investments_record_write_off },
    { id: 'short-open', label: m.investments_record_short_sale, titleID: 'short-open-title' },
    { id: 'short-cover', label: m.investments_record_short_cover, titleID: 'short-cover-title' },
    { id: 'payment-in-lieu', label: m.investments_record_payment_in_lieu, titleID: 'payment-in-lieu-title' },
    { id: 'dividend', label: m.investments_record_dividend, titleID: 'dividend-title' },
    { id: 'withholding', label: m.investments_record_withholding, titleID: 'withholding-title' },
    { id: 'reinvested', label: m.investments_record_reinvested, titleID: 'reinvested-dividend-title' },
    { id: 'external-transfer-in', label: m.investments_record_external_transfer_in, titleID: 'external-transfer-in-title' },
    { id: 'internal-transfer', label: m.investments_record_internal_transfer, titleID: 'internal-transfer-title' },
    { id: 'external-transfer-out', label: m.investments_record_external_transfer_out, titleID: 'external-transfer-out-title' },
    { id: 'split', label: m.investments_record_split, titleID: 'split-title' },
    { id: 'share-exchange', label: m.investments_record_share_exchange, titleID: 'share-exchange-title' },
    { id: 'cash-and-stock-merger', label: m.investments_record_merger, titleID: 'merger-title' },
    { id: 'spin-off', label: m.investments_record_spin_off, titleID: 'spin-off-title' },
    { id: 'capital-return', label: m.investments_record_capital_return, titleID: 'capital-return-title' }
  ];

  let active = $state<Activity | null>(null);
  let opener: HTMLElement | null = null;
  let writeOffSaved = $state(false);
  const titleID = $derived(activities.find((activity) => activity.id === active)?.titleID);

  function open(activity: Activity, event: MouseEvent) {
    writeOffSaved = false;
    opener = event.currentTarget as HTMLElement;
    active = activity;
  }

  function close() {
    active = null;
  }

  // Each form refreshes every investment read itself (invalidateInvestmentReads),
  // which includes the transaction lists and registers.
  function saved() {
    writeOffSaved = active === 'write-off';
    close();
    onSaved?.();
  }
</script>

{#if writeOffSaved}
  <p role="status" class="w-full text-sm text-foreground">{m.investments_write_off_success()}</p>
{/if}

<div class="flex w-full flex-wrap gap-2">
  {#each activities as activity, index (activity.id)}
    <button
      type="button"
      onclick={(event) => open(activity.id, event)}
      class={index === 0
        ? 'inline-flex min-h-10 items-center gap-2 rounded-(--radius-control) bg-foreground px-3 py-2 text-sm font-semibold text-background transition hover:opacity-90'
        : 'inline-flex min-h-10 items-center gap-2 rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm font-semibold text-foreground transition hover:bg-control-hover'}
    >
      {#if index === 0}<Plus size={14} aria-hidden="true" />{/if}
      {activity.label()}
    </button>
  {/each}
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
