<script lang="ts">
  import { Button } from "$lib/components/ui/button";
  import { Input } from "$lib/components/ui/input";
  import { Label } from "$lib/components/ui/label";
  import * as Dialog from "$lib/components/ui/dialog";
  import {
    createCrossCurrencyTransfer,
    type CrossCurrencyTransferInput,
  } from "$lib/api/transactions";
  import { parseAmountToMinor } from "$lib/money";
  import { validateIsoDate, validateScaleAwareAmountMinor } from "$lib/forms/validators";
  import { formatError } from "$lib/utils";

  type AccountLookup = {
    id: number;
    name: string;
    commodity_id: number;
    account_type: string;
  };

  type CommodityLookup = {
    id: number;
    name: string;
    symbol: string | null;
    scale: number;
  };

  let {
    open = $bindable(false),
    bookId,
    accounts,
    commodities,
    onPosted,
  }: {
    open?: boolean;
    bookId: number;
    accounts: AccountLookup[];
    commodities: CommodityLookup[];
    onPosted: () => void | Promise<void>;
  } = $props();

  let txnDate = $state(new Date().toISOString().slice(0, 10));
  let sourceAccountId = $state<number | null>(null);
  let destinationAccountId = $state<number | null>(null);
  let sourceAmount = $state("");
  let destinationAmount = $state("");
  let fxRate = $state("");
  let memo = $state("");
  let submitting = $state(false);
  let error = $state("");

  const sourceAccount = $derived(accounts.find((a) => a.id === sourceAccountId));
  const destinationAccount = $derived(accounts.find((a) => a.id === destinationAccountId));
  const sourceCommodity = $derived(commodities.find((c) => c.id === sourceAccount?.commodity_id));
  const destinationCommodity = $derived(commodities.find((c) => c.id === destinationAccount?.commodity_id));

  // Crude implied-rate hint when the user has typed both amounts.
  const impliedRate = $derived(() => {
    if (!sourceCommodity || !destinationCommodity) return null;
    const src = parseAmountToMinor(sourceAmount, sourceCommodity.scale);
    const dst = parseAmountToMinor(destinationAmount, destinationCommodity.scale);
    if (!src || !dst || src <= 0) return null;
    // implied destination-per-source unit, in real-number space
    const srcFloat = src / 10 ** sourceCommodity.scale;
    const dstFloat = dst / 10 ** destinationCommodity.scale;
    if (srcFloat === 0) return null;
    return (dstFloat / srcFloat).toFixed(6);
  });

  function reset() {
    txnDate = new Date().toISOString().slice(0, 10);
    sourceAccountId = null;
    destinationAccountId = null;
    sourceAmount = "";
    destinationAmount = "";
    fxRate = "";
    memo = "";
    error = "";
    submitting = false;
  }

  async function submit() {
    submitting = true;
    error = "";
    try {
      const dateResult = validateIsoDate(txnDate, "Date");
      if (!dateResult.ok) throw new Error(dateResult.error);
      if (!sourceAccount || !destinationAccount) {
        throw new Error("Source and destination accounts are required");
      }
      if (sourceAccount.id === destinationAccount.id) {
        throw new Error("Source and destination accounts must differ");
      }
      if (!sourceCommodity || !destinationCommodity) {
        throw new Error("Cannot resolve commodity for one of the selected accounts");
      }
      const srcResult = validateScaleAwareAmountMinor(sourceAmount, sourceCommodity.scale, "Source amount");
      if (!srcResult.ok) throw new Error(srcResult.error);
      if (srcResult.value <= 0) throw new Error("Source amount must be greater than zero");
      const dstResult = validateScaleAwareAmountMinor(destinationAmount, destinationCommodity.scale, "Destination amount");
      if (!dstResult.ok) throw new Error(dstResult.error);
      if (dstResult.value <= 0) throw new Error("Destination amount must be greater than zero");
      const rate = Number(fxRate);
      if (!Number.isFinite(rate) || rate <= 0) throw new Error("FX rate must be greater than zero");

      const input: CrossCurrencyTransferInput = {
        book_id: bookId,
        txn_date: dateResult.value,
        source_account_id: sourceAccount.id,
        destination_account_id: destinationAccount.id,
        source_amount_minor: srcResult.value,
        destination_amount_minor: dstResult.value,
        fx_rate: rate,
        fx_gain_loss_account_id: null,
        payee_id: null,
        memo: memo.trim() || null,
        status: "cleared",
        reference: null,
        source_memo: null,
        destination_memo: null,
        fx_gain_loss_memo: null,
      };
      await createCrossCurrencyTransfer(input);
      open = false;
      reset();
      await onPosted();
    } catch (e) {
      error = formatError(e);
    } finally {
      submitting = false;
    }
  }

  $effect(() => {
    if (!open) reset();
  });
</script>

<Dialog.Root bind:open>
  <Dialog.Content class="max-w-2xl">
    <form
      onsubmit={(e) => {
        e.preventDefault();
        void submit();
      }}
    >
      <Dialog.Header>
        <Dialog.Title>Cross-currency transfer</Dialog.Title>
        <Dialog.Description>
          Move funds between two accounts that hold different commodities. The
          API posts three splits: source debit, destination credit, FX
          gain/loss.
        </Dialog.Description>
      </Dialog.Header>

      <div class="space-y-4 py-4">
        <div class="space-y-2">
          <Label for="xfer-date">Date</Label>
          <Input id="xfer-date" type="date" bind:value={txnDate} required />
        </div>

        <div class="grid grid-cols-2 gap-4">
          <div class="space-y-2">
            <Label for="xfer-source">Source account</Label>
            <select id="xfer-source" class="flex h-9 w-full rounded-md border border-input bg-transparent px-3 py-1 text-sm" bind:value={sourceAccountId} required>
              <option value={null}>Select account</option>
              {#each accounts as account}
                <option value={account.id}>{account.name} ({commodities.find((c) => c.id === account.commodity_id)?.symbol ?? "?"})</option>
              {/each}
            </select>
          </div>

          <div class="space-y-2">
            <Label for="xfer-destination">Destination account</Label>
            <select id="xfer-destination" class="flex h-9 w-full rounded-md border border-input bg-transparent px-3 py-1 text-sm" bind:value={destinationAccountId} required>
              <option value={null}>Select account</option>
              {#each accounts as account}
                <option value={account.id}>{account.name} ({commodities.find((c) => c.id === account.commodity_id)?.symbol ?? "?"})</option>
              {/each}
            </select>
          </div>
        </div>

        <div class="grid grid-cols-2 gap-4">
          <div class="space-y-2">
            <Label for="xfer-source-amount">Source amount {sourceCommodity ? `(${sourceCommodity.symbol ?? sourceCommodity.name})` : ""}</Label>
            <Input id="xfer-source-amount" placeholder="0.00" bind:value={sourceAmount} required />
          </div>

          <div class="space-y-2">
            <Label for="xfer-destination-amount">Destination amount {destinationCommodity ? `(${destinationCommodity.symbol ?? destinationCommodity.name})` : ""}</Label>
            <Input id="xfer-destination-amount" placeholder="0.00" bind:value={destinationAmount} required />
          </div>
        </div>

        <div class="space-y-2">
          <Label for="xfer-rate">FX rate (source per destination)</Label>
          <Input id="xfer-rate" type="number" step="0.000001" placeholder="1.000000" bind:value={fxRate} required />
          {#if impliedRate()}
            <p class="text-xs text-muted-foreground">Implied from amounts: {impliedRate()}</p>
          {/if}
        </div>

        <div class="space-y-2">
          <Label for="xfer-memo">Memo</Label>
          <Input id="xfer-memo" bind:value={memo} placeholder="Optional" />
        </div>

        {#if error}
          <p class="text-sm text-destructive">{error}</p>
        {/if}
      </div>

      <Dialog.Footer>
        <Button variant="outline" type="button" onclick={() => (open = false)}>Cancel</Button>
        <Button type="submit" disabled={submitting}>
          {submitting ? "Posting…" : "Post transfer"}
        </Button>
      </Dialog.Footer>
    </form>
  </Dialog.Content>
</Dialog.Root>
