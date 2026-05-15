<script lang="ts">
  import { Button } from "$lib/components/ui/button";
  import { Input } from "$lib/components/ui/input";
  import { Label } from "$lib/components/ui/label";
  import type { FilterColumn } from "$lib/transactions/saved-views";

  type AccountLookup = { id: number; name: string };

  let {
    activeFilter,
    dateFrom = $bindable(""),
    dateTo = $bindable(""),
    statusFilter = $bindable(""),
    accountFilterId = $bindable<number | null>(null),
    amountMin = $bindable(""),
    amountMax = $bindable(""),
    search = $bindable(""),
    accounts,
    onApply,
    onSearchInput,
  }: {
    activeFilter: FilterColumn | null;
    dateFrom?: string;
    dateTo?: string;
    statusFilter?: string;
    accountFilterId?: number | null;
    amountMin?: string;
    amountMax?: string;
    search?: string;
    accounts: AccountLookup[];
    onApply: () => void | Promise<void>;
    onSearchInput: () => void;
  } = $props();

  function clear(column: FilterColumn) {
    if (column === "date") { dateFrom = ""; dateTo = ""; }
    else if (column === "payee") { search = ""; }
    else if (column === "status") { statusFilter = ""; }
    else if (column === "account") { accountFilterId = null; }
    else if (column === "amount") { amountMin = ""; amountMax = ""; }
    void onApply();
  }
</script>

{#if activeFilter}
  <div class="mb-4 p-4 bg-muted/50 rounded-lg border">
    {#if activeFilter === "date"}
      <div class="flex flex-wrap items-end gap-4">
        <div class="space-y-1">
          <Label for="tx-date-from">From</Label>
          <Input id="tx-date-from" type="date" bind:value={dateFrom} class="w-40" />
        </div>
        <div class="space-y-1">
          <Label for="tx-date-to">To</Label>
          <Input id="tx-date-to" type="date" bind:value={dateTo} class="w-40" />
        </div>
        <div class="flex gap-2">
          <Button variant="secondary" size="sm" onclick={() => onApply()}>Apply</Button>
          <Button variant="ghost" size="sm" onclick={() => clear("date")}>Clear</Button>
        </div>
      </div>
    {:else if activeFilter === "payee"}
      <div class="flex flex-wrap items-end gap-4">
        <div class="space-y-1 flex-1 max-w-xs">
          <Label for="tx-payee-search">Search payee / memo</Label>
          <Input id="tx-payee-search" placeholder="Type to search…" bind:value={search} oninput={onSearchInput} />
        </div>
        <div class="flex gap-2">
          <Button variant="ghost" size="sm" onclick={() => clear("payee")}>Clear</Button>
        </div>
      </div>
    {:else if activeFilter === "status"}
      <div class="flex flex-wrap items-end gap-4">
        <div class="space-y-1">
          <Label for="tx-status-filter">Status</Label>
          <select id="tx-status-filter" class="flex h-9 rounded-md border border-input bg-transparent px-3 py-1 text-sm" bind:value={statusFilter} onchange={() => onApply()}>
            <option value="">Active (non-void)</option>
            <option value="uncleared">Uncleared</option>
            <option value="cleared">Cleared</option>
            <option value="reconciled">Reconciled</option>
            <option value="void">Void only</option>
            <option value="all">All (including void)</option>
          </select>
        </div>
        <Button variant="ghost" size="sm" onclick={() => clear("status")}>Clear</Button>
      </div>
    {:else if activeFilter === "account"}
      <div class="flex flex-wrap items-end gap-4">
        <div class="space-y-1">
          <Label for="tx-account-filter">Account</Label>
          <select id="tx-account-filter" class="flex h-9 rounded-md border border-input bg-transparent px-3 py-1 text-sm" bind:value={accountFilterId} onchange={() => onApply()}>
            <option value={null}>All accounts</option>
            {#each accounts as account}
              <option value={account.id}>{account.name}</option>
            {/each}
          </select>
        </div>
        <Button variant="ghost" size="sm" onclick={() => clear("account")}>Clear</Button>
      </div>
    {:else if activeFilter === "amount"}
      <div class="flex flex-wrap items-end gap-4">
        <div class="space-y-1">
          <Label for="tx-amount-min">Min amount</Label>
          <Input id="tx-amount-min" type="number" step="0.01" placeholder="0.00" bind:value={amountMin} class="w-32" />
        </div>
        <div class="space-y-1">
          <Label for="tx-amount-max">Max amount</Label>
          <Input id="tx-amount-max" type="number" step="0.01" placeholder="0.00" bind:value={amountMax} class="w-32" />
        </div>
        <div class="flex gap-2">
          <Button variant="secondary" size="sm" onclick={() => onApply()}>Apply</Button>
          <Button variant="ghost" size="sm" onclick={() => clear("amount")}>Clear</Button>
        </div>
      </div>
    {/if}
  </div>
{/if}
