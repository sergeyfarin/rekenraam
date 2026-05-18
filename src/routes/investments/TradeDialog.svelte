<script lang="ts">
  import type { AccountSummary } from "$lib/api/accounts";
  import type { CommodityAutocompleteOption } from "$lib/api/commodities";
  import type { CommoditySummary } from "$lib/api/metadata";
  import { Button } from "$lib/components/ui/button";
  import * as Dialog from "$lib/components/ui/dialog";
  import { Input } from "$lib/components/ui/input";
  import { Label } from "$lib/components/ui/label";
  import type { TradeFormState, TradeType } from "./forms";

  export let open = false;
  export let tradeType: TradeType = "buy";
  export let form: TradeFormState;
  export let investmentAccounts: AccountSummary[] = [];
  export let cashAccounts: AccountSummary[] = [];
  export let securities: CommoditySummary[] = [];
  export let selectedSecurity: CommoditySummary | null = null;
  export let searchQuery = "";
  export let searchSuggestions: CommodityAutocompleteOption[] = [];
  export let searching = false;
  export let busy = false;
  export let onClose: () => void = () => {};
  export let onSearchInput: (event: Event) => void = () => {};
  export let onChooseSecurity: (option: CommodityAutocompleteOption) => void = () => {};
  export let onSubmit: () => void | Promise<void> = () => {};
</script>

<Dialog.Root bind:open>
  <Dialog.Content class="max-w-md">
    <Dialog.Header>
      <Dialog.Title>{tradeType === "buy" ? "Buy Security" : "Sell Security"}</Dialog.Title>
    </Dialog.Header>
    <div class="space-y-4 py-4">
      <div class="space-y-2">
        <Label for="trade-date">Date</Label>
        <Input id="trade-date" type="date" bind:value={form.txn_date} />
      </div>
      <div class="space-y-2">
        <Label for="trade-inv-account">Investment Account</Label>
        <select id="trade-inv-account" class="flex h-9 w-full rounded-md border border-input bg-transparent px-3 py-1 text-sm" bind:value={form.investment_account_id}>
          <option value={""}>Select account...</option>
          {#each investmentAccounts as account}
            <option value={account.id}>{account.name}</option>
          {/each}
        </select>
      </div>
      <div class="space-y-2">
        <Label for="trade-cash-account">Cash Account</Label>
        <select id="trade-cash-account" class="flex h-9 w-full rounded-md border border-input bg-transparent px-3 py-1 text-sm" bind:value={form.cash_account_id}>
          <option value={""}>Select account...</option>
          {#each cashAccounts as account}
            <option value={account.id}>{account.name}</option>
          {/each}
        </select>
      </div>
      <div class="space-y-2">
        <Label for="trade-security-search">Security</Label>
        <Input id="trade-security-search" value={searchQuery} oninput={onSearchInput} placeholder="Type symbol or name..." />
        {#if searching}
          <p class="text-xs text-muted-foreground">Searching…</p>
        {/if}
        {#if searchSuggestions.length > 0}
          <div class="max-h-40 overflow-auto rounded-md border border-input">
            {#each searchSuggestions as option}
              <button type="button" class="w-full px-3 py-2 text-left text-sm hover:bg-accent" onclick={() => onChooseSecurity(option)}>
                {option.symbol ? `${option.symbol} — ${option.name}` : option.name}
              </button>
            {/each}
          </div>
        {/if}
        <select id="trade-security-select" class="flex h-9 w-full rounded-md border border-input bg-transparent px-3 py-1 text-sm" bind:value={form.commodity_id}>
          <option value={""}>Select security...</option>
          {#each securities as security}
            <option value={security.id}>{security.symbol || security.name}</option>
          {/each}
        </select>
        {#if selectedSecurity}
          <p class="text-xs text-muted-foreground">Selected: {selectedSecurity.symbol || selectedSecurity.name}</p>
        {/if}
      </div>
      <div class="space-y-2">
        <Label for="trade-quantity">Quantity</Label>
        <Input id="trade-quantity" type="number" step="0.000001" bind:value={form.quantity} placeholder="Number of shares" />
      </div>
      <div class="space-y-2">
        <Label for="trade-amount">{tradeType === "buy" ? "Total Cost" : "Proceeds"}</Label>
        <Input id="trade-amount" type="number" step="0.01" bind:value={form.cash_amount} placeholder="Amount" />
      </div>
      <div class="space-y-2">
        <Label for="trade-memo">Memo (optional)</Label>
        <Input id="trade-memo" bind:value={form.memo} placeholder="Description" />
      </div>
    </div>
    <Dialog.Footer>
      <Button variant="outline" onclick={onClose}>Cancel</Button>
      <Button onclick={onSubmit} disabled={busy}>{tradeType === "buy" ? "Buy" : "Sell"}</Button>
    </Dialog.Footer>
  </Dialog.Content>
</Dialog.Root>