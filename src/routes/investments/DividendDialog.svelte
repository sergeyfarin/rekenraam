<script lang="ts">
  import type { AccountSummary } from "$lib/api/accounts";
  import { Button } from "$lib/components/ui/button";
  import * as Dialog from "$lib/components/ui/dialog";
  import { Input } from "$lib/components/ui/input";
  import { Label } from "$lib/components/ui/label";
  import type { DividendFormState } from "./forms";

  export let open = false;
  export let form: DividendFormState;
  export let cashAccounts: AccountSummary[] = [];
  export let incomeAccounts: AccountSummary[] = [];
  export let busy = false;
  export let onClose: () => void = () => {};
  export let onSubmit: () => void | Promise<void> = () => {};
</script>

<Dialog.Root bind:open>
  <Dialog.Content class="max-w-md">
    <Dialog.Header>
      <Dialog.Title>Record Dividend</Dialog.Title>
    </Dialog.Header>
    <div class="space-y-4 py-4">
      <div class="space-y-2">
        <Label for="div-date">Date</Label>
        <Input id="div-date" type="date" bind:value={form.txn_date} />
      </div>
      <div class="space-y-2">
        <Label for="div-cash-account">Cash Account</Label>
        <select id="div-cash-account" class="flex h-9 w-full rounded-md border border-input bg-transparent px-3 py-1 text-sm" bind:value={form.cash_account_id}>
          <option value={""}>Select account...</option>
          {#each cashAccounts as account}
            <option value={account.id}>{account.name}</option>
          {/each}
        </select>
      </div>
      <div class="space-y-2">
        <Label for="div-income-account">Income Account</Label>
        <select id="div-income-account" class="flex h-9 w-full rounded-md border border-input bg-transparent px-3 py-1 text-sm" bind:value={form.income_account_id}>
          <option value={""}>Select account...</option>
          {#each incomeAccounts as account}
            <option value={account.id}>{account.name}</option>
          {/each}
        </select>
      </div>
      <div class="space-y-2">
        <Label for="div-amount">Amount</Label>
        <Input id="div-amount" type="number" step="0.01" bind:value={form.amount} placeholder="Dividend amount" />
      </div>
      <div class="space-y-2">
        <Label for="div-memo">Memo (optional)</Label>
        <Input id="div-memo" bind:value={form.memo} placeholder="Description" />
      </div>
    </div>
    <Dialog.Footer>
      <Button variant="outline" onclick={onClose}>Cancel</Button>
      <Button onclick={onSubmit} disabled={busy}>Record Dividend</Button>
    </Dialog.Footer>
  </Dialog.Content>
</Dialog.Root>