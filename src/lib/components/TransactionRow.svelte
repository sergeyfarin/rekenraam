<script lang="ts">
  import * as Table from "$lib/components/ui/table";
  import { Button } from "$lib/components/ui/button";
  import { Badge } from "$lib/components/ui/badge";
  import type { TransactionWithSplits, SplitRecord } from "$lib/api/transactions";
  import type { CommoditySummary, PayeeSummary } from "$lib/api/metadata";
  import { formatMinorWithScale } from "$lib/money";

  type AccountLookup = { id: number; name: string };

  let {
    tx,
    accountFilterId,
    selected,
    accounts,
    payees,
    commodities,
    onToggleSelect,
    onEdit,
    onDuplicate,
    onUpdateStatus,
    onRemove,
  }: {
    tx: TransactionWithSplits;
    accountFilterId: number | null;
    selected: boolean;
    accounts: AccountLookup[];
    payees: PayeeSummary[];
    commodities: CommoditySummary[];
    onToggleSelect: (id: number) => void;
    onEdit: (tx: TransactionWithSplits) => void;
    onDuplicate: (tx: TransactionWithSplits) => void;
    onUpdateStatus: (tx: TransactionWithSplits, status: string) => void;
    onRemove: (tx: TransactionWithSplits) => void;
  } = $props();

  function primarySplit(transaction: TransactionWithSplits, accountId: number | null): SplitRecord {
    if (accountId) {
      return transaction.splits.find((s) => s.account_id === accountId) ?? transaction.splits[0];
    }
    return transaction.splits.reduce((max, current) =>
      Math.abs(current.amount_minor) > Math.abs(max.amount_minor) ? current : max,
    );
  }

  function payeeName(id: number | null): string {
    if (!id) return "—";
    return payees.find((p) => p.id === id)?.name ?? "—";
  }

  function accountName(id: number): string {
    return accounts.find((a) => a.id === id)?.name ?? "—";
  }

  function formatAmount(amountMinor: number, commodityId: number): string {
    const commodity = commodities.find((c) => c.id === commodityId);
    if (!commodity) return String(amountMinor);
    const formatted = formatMinorWithScale(amountMinor, commodity.scale);
    return `${formatted} ${commodity.symbol ?? commodity.name}`;
  }

  const primary = $derived(primarySplit(tx, accountFilterId));
  const statusVariant = $derived(
    tx.transaction.status === "reconciled" ? "default" :
    tx.transaction.status === "cleared" ? "secondary" :
    tx.transaction.status === "void" ? "destructive" : "outline",
  );
</script>

<Table.Row class="hover:bg-muted/50 {selected ? 'bg-muted/30' : ''}">
  <Table.Cell class="w-8">
    <input
      type="checkbox"
      class="cursor-pointer"
      checked={selected}
      onchange={() => onToggleSelect(tx.transaction.id)}
    />
  </Table.Cell>
  <Table.Cell class="font-mono text-sm">{tx.transaction.txn_date}</Table.Cell>
  <Table.Cell>
    <div class="leading-snug">
      <span class="font-medium">{payeeName(tx.transaction.payee_id)}</span>
      {#if tx.transaction.memo}
        <span class="block text-xs text-muted-foreground">{tx.transaction.memo}</span>
      {/if}
    </div>
  </Table.Cell>
  <Table.Cell>
    <Badge variant={statusVariant}>{tx.transaction.status}</Badge>
  </Table.Cell>
  <Table.Cell class="text-sm">{accountName(primary.account_id)}</Table.Cell>
  <Table.Cell class="text-right font-mono {primary.amount_minor < 0 ? 'text-money-negative' : 'text-money-positive'}">
    {formatAmount(primary.amount_minor, primary.commodity_id)}
  </Table.Cell>
  <Table.Cell class="text-right">
    <div class="flex justify-end gap-1">
      <Button variant="ghost" size="sm" onclick={() => onEdit(tx)}>Edit</Button>
      <Button variant="ghost" size="sm" onclick={() => onDuplicate(tx)}>Dup</Button>
      {#if tx.transaction.status === "cleared"}
        <Button variant="ghost" size="sm" onclick={() => onUpdateStatus(tx, "uncleared")}>Unflag</Button>
      {:else}
        <Button variant="ghost" size="sm" onclick={() => onUpdateStatus(tx, "cleared")}>Flag</Button>
      {/if}
      <Button variant="ghost" size="sm" onclick={() => onUpdateStatus(tx, "void")}>Void</Button>
      <Button variant="ghost" size="sm" class="text-destructive" onclick={() => onRemove(tx)}>Del</Button>
    </div>
  </Table.Cell>
</Table.Row>
