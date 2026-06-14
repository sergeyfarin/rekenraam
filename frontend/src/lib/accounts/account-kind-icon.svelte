<script lang="ts">
  import Banknote from '@lucide/svelte/icons/banknote';
  import BriefcaseBusiness from '@lucide/svelte/icons/briefcase-business';
  import Car from '@lucide/svelte/icons/car';
  import CircleDollarSign from '@lucide/svelte/icons/circle-dollar-sign';
  import Ellipsis from '@lucide/svelte/icons/ellipsis';
  import Gift from '@lucide/svelte/icons/gift';
  import HandHeart from '@lucide/svelte/icons/hand-heart';
  import House from '@lucide/svelte/icons/house';
  import Landmark from '@lucide/svelte/icons/landmark';
  import Receipt from '@lucide/svelte/icons/receipt';
  import Shield from '@lucide/svelte/icons/shield';
  import Tags from '@lucide/svelte/icons/tags';
  import WalletCards from '@lucide/svelte/icons/wallet-cards';
  import type { AccountKind } from './account-labels';

  let {
    kind,
    className = 'size-4'
  } = $props<{
    kind: AccountKind;
    className?: string;
  }>();
</script>

{#if kind === 'cash'}
  <Banknote class={className} aria-hidden="true" />
{:else if kind === 'checking'}
  <Landmark class={className} aria-hidden="true" />
{:else if kind === 'savings'}
  <CircleDollarSign class={className} aria-hidden="true" />
{:else if kind === 'term_deposit'}
  <Shield class={className} aria-hidden="true" />
{:else if kind === 'other_money_account'}
  <WalletCards class={className} aria-hidden="true" />
{:else if kind === 'brokerage'}
  <BriefcaseBusiness class={className} aria-hidden="true" />
{:else if kind === 'brokerage_cash'}
  <Banknote class={className} aria-hidden="true" />
{:else if kind === 'security_holding'}
  <BriefcaseBusiness class={className} aria-hidden="true" />
{:else if kind === 'fund_holding'}
  <CircleDollarSign class={className} aria-hidden="true" />
{:else if kind === 'crypto_wallet'}
  <CircleDollarSign class={className} aria-hidden="true" />
{:else if kind === 'property'}
  <House class={className} aria-hidden="true" />
{:else if kind === 'vehicle'}
  <Car class={className} aria-hidden="true" />
{:else if kind === 'rewards_balance'}
  <Gift class={className} aria-hidden="true" />
{:else if kind === 'receivable'}
  <HandHeart class={className} aria-hidden="true" />
{:else if kind === 'other_asset'}
  <Tags class={className} aria-hidden="true" />
{:else if kind === 'credit_card'}
  <WalletCards class={className} aria-hidden="true" />
{:else if kind === 'line_of_credit'}
  <Receipt class={className} aria-hidden="true" />
{:else if kind === 'loan'}
  <Landmark class={className} aria-hidden="true" />
{:else if kind === 'mortgage'}
  <House class={className} aria-hidden="true" />
{:else if kind === 'tax_liability'}
  <Receipt class={className} aria-hidden="true" />
{:else if kind === 'payable'}
  <HandHeart class={className} aria-hidden="true" />
{:else if kind === 'other_liability'}
  <Receipt class={className} aria-hidden="true" />
{:else if kind === 'equity'}
  <CircleDollarSign class={className} aria-hidden="true" />
{:else if kind === 'income'}
  <Banknote class={className} aria-hidden="true" />
{:else if kind === 'expense'}
  <Receipt class={className} aria-hidden="true" />
{:else}
  <Ellipsis class={className} aria-hidden="true" />
{/if}
