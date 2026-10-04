<script lang="ts">
  import { m } from '$lib/paraglide/messages.js';
  import type { AccountRegisterEntryResponse } from '$lib/api/transactions';
  import type { AccountClass } from './transaction-labels';
  import { commodityDisplay, formatSignedAmount } from './transaction-labels';
  import {
    correctionBadge,
    isEffectiveMember,
    type RegisterCorrectionChain,
    type RegisterCorrectionRole
  } from './correction-chain';

  // The expandable explanation for a register row that belongs to a
  // correction chain (T-120). The rows stay separate in the register; this
  // names every member, including those on other pages, and the chain's net
  // effect on this account, counted once.
  let {
    entry,
    chain,
    locale
  }: {
    entry: AccountRegisterEntryResponse;
    chain: RegisterCorrectionChain;
    locale: string;
  } = $props();

  const badge = $derived(correctionBadge(chain, entry.transaction_id));

  function badgeText(): string {
    switch (badge) {
      case 'reversal':
        return m.register_correction_badge_reversal();
      case 'current':
        return m.register_correction_badge_current();
      case 'corrected':
        return m.register_correction_badge_corrected();
    }
  }

  function roleText(role: RegisterCorrectionRole): string {
    switch (role) {
      case 'original':
        return m.register_correction_role_original();
      case 'reversal':
        return m.register_correction_role_reversal();
      case 'replacement':
        return m.register_correction_role_replacement();
    }
  }

  // Amounts use this account's perspective and commodity, like the row itself.
  function amountText(amount: { quantity_value: string; quantity_scale: number }): string {
    const posting = { ...entry.posting, quantity_value: amount.quantity_value, quantity_scale: amount.quantity_scale };
    return `${commodityDisplay(posting)}${formatSignedAmount(posting, entry.posting.account_class as AccountClass, locale)}`;
  }
</script>

<!-- svelte-ignore a11y_no_noninteractive_element_interactions (the listeners only stop the row's open-on-click and open-on-Enter from firing while the native summary toggles this disclosure) -->
<details
  class="mt-1 text-xs"
  onclick={(e) => e.stopPropagation()}
  onkeydown={(e) => e.stopPropagation()}
>
  <summary class="inline-flex cursor-pointer flex-wrap items-center gap-1.5 rounded text-muted hover:text-foreground focus-visible:outline focus-visible:outline-2 focus-visible:outline-accent">
    <!-- Bordered foreground text, not a tinted StatusBadge: the warning and
         accent badge tones fall below 4.5:1 at this size. The words carry the
         state, so no colour cue is needed. -->
    <span class="rounded-(--radius-control) border border-border px-1.5 py-0.5 font-semibold text-foreground">{badgeText()}</span>
    <span>{m.register_correction_history({ count: chain.members.length })}</span>
  </summary>
  <div class="mt-2 space-y-2 rounded-(--radius-control) border border-border bg-surface-strong px-3 py-2 text-foreground">
    <ol class="space-y-1.5">
      {#each chain.members as member (member.transaction_id)}
        <li class="flex flex-wrap items-baseline gap-x-2 gap-y-0.5">
          <span class="font-medium">{roleText(member.role)}</span>
          <span class="tabular-nums text-muted">{member.transaction_date}</span>
          {#if member.amount}
            <span class="tabular-nums">{amountText(member.amount)}</span>
          {:else}
            <span class="text-muted">{m.register_correction_elsewhere()}</span>
          {/if}
          {#if member.transaction_id === entry.transaction_id}
            <span class="text-muted">({m.register_correction_this_row()})</span>
          {/if}
          {#if isEffectiveMember(chain, member)}
            <span class="text-accent">({m.register_correction_current()})</span>
          {/if}
          {#if member.reason}
            <span class="basis-full text-muted">{m.register_correction_reason({ reason: member.reason })}</span>
          {/if}
        </li>
      {/each}
    </ol>
    <p class="font-medium tabular-nums">{m.register_correction_net({ amount: amountText(chain.net_effect) })}</p>
    <p class="text-muted">{m.register_correction_net_hint()}</p>
    {#if chain.effective_transaction_id === null}
      <p class="text-muted">{m.register_correction_fully_reversed()}</p>
    {/if}
  </div>
</details>
