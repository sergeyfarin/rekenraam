<script lang="ts">
  import { onMount } from "svelte";
  import { goto } from "$app/navigation";
  import { page } from "$app/state";
  import { listAccounts, type AccountSummary } from "$lib/api/accounts";
  import { listPayees, type PayeeSummary } from "$lib/api/metadata";
  import {
    finishReconciliation,
    getReconciliationHistory,
    startReconciliation,
    type ReconciliationHistoryResponse,
  } from "$lib/api/reconciliation";
  import type { TransactionWithSplits } from "$lib/api/transactions";
  import { Button } from "$lib/components/ui/button";
  import * as Alert from "$lib/components/ui/alert";

  let accountId: number | null = null;
  let account: AccountSummary | null = null;
  let accounts: AccountSummary[] = [];
  let payees: PayeeSummary[] = [];
  let history: ReconciliationHistoryResponse | null = null;

  let commoditySymbol = "";
  let commodityScale = 2;
  let statementDate = new Date().toISOString().slice(0, 10);
  let statementStartDate = "";
  let statementBalanceInput = "";
  let memo = "";
  let offsetAccountId = "";
  let rememberOffsetAccount = true;

  let candidates: TransactionWithSplits[] = [];
  let checkedIds = new Set<number>();
  let loading = false;
  let finishing = false;
  let error = "";
  let finishError = "";
  let step: "setup" | "working" = "setup";

  $: factor = 10 ** commodityScale;
  $: statementBalanceMinor = parseAmount(statementBalanceInput);
  $: openingBalanceMinor = history?.balancings.find((row) => !row.voided_at)?.balance_minor ?? 0;
  $: checkedAmountMinor = candidates
    .filter((tx) => checkedIds.has(tx.transaction.id))
    .reduce((sum, tx) => sum + splitAmount(tx), 0);
  $: clearedBalanceMinor = openingBalanceMinor + checkedAmountMinor;
  $: differenceMinor = statementBalanceMinor === null ? null : clearedBalanceMinor - statementBalanceMinor;
  $: needsOffset = differenceMinor !== null && differenceMinor !== 0;
  $: sorted = [...candidates].sort((a, b) => a.transaction.txn_date.localeCompare(b.transaction.txn_date) || a.transaction.id - b.transaction.id);
  $: offsetChoices = accounts.filter((entry) => account && entry.book_id === account.book_id && entry.commodity_id === account.commodity_id && !entry.is_closed && entry.id !== account.id);

  onMount(async () => {
    const parsed = Number(page.params.id);
    if (!Number.isFinite(parsed)) {
      error = "Invalid account.";
      return;
    }
    accountId = parsed;
    await loadBaseData();
  });

  async function loadBaseData() {
    if (!accountId) return;
    loading = true;
    error = "";
    try {
      const [accountRows, payeeRows, historyRows] = await Promise.all([
        listAccounts(),
        listPayees(1),
        getReconciliationHistory(accountId),
      ]);
      accounts = accountRows;
      payees = payeeRows;
      history = historyRows;
      account = accountRows.find((entry) => entry.id === accountId) ?? null;
      if (!account) {
        error = "Account not found.";
      }
    } catch (e) {
      error = `Failed to load reconciliation data: ${String(e)}`;
    } finally {
      loading = false;
    }
  }

  async function loadCandidates() {
    if (!accountId || statementBalanceMinor === null) return;
    loading = true;
    error = "";
    try {
      const response = await startReconciliation(accountId, statementDate, statementStartDate || null);
      account = response.account;
      commoditySymbol = response.commodity.symbol ?? response.commodity.name;
      commodityScale = response.commodity.scale;
      statementStartDate = response.statement_start_date ?? "";
      candidates = response.candidates;
      checkedIds = new Set(
        candidates
          .filter((tx) => tx.transaction.status === "cleared")
          .map((tx) => tx.transaction.id),
      );
      if (response.remembered_offset_account_id) {
        offsetAccountId = String(response.remembered_offset_account_id);
      }
      history = await getReconciliationHistory(accountId);
      step = "working";
    } catch (e) {
      error = `Failed to start reconciliation: ${String(e)}`;
    } finally {
      loading = false;
    }
  }

  async function finish() {
    if (!accountId || !account || statementBalanceMinor === null) return;
    finishing = true;
    finishError = "";
    try {
      await finishReconciliation(accountId, {
        book_id: account.book_id,
        as_of_date: statementDate,
        statement_start_date: statementStartDate || null,
        statement_balance_minor: statementBalanceMinor,
        selected_transaction_ids: Array.from(checkedIds),
        offset_account_id: needsOffset ? Number(offsetAccountId) || null : null,
        remember_offset_account: needsOffset && rememberOffsetAccount,
        memo: memo.trim() || null,
        confirm_unbalanced: needsOffset,
        downgrade_unselected_cleared: true,
      });
      goto(`/accounts/${accountId}`);
    } catch (e) {
      finishError = `Failed to finish reconciliation: ${String(e)}`;
    } finally {
      finishing = false;
    }
  }

  function parseAmount(value: string): number | null {
    const raw = value.replace(/,/g, "").trim();
    if (!raw || Number.isNaN(Number(raw))) return null;
    return Math.round(Number(raw) * factor);
  }

  function splitAmount(tx: TransactionWithSplits): number {
    return tx.splits.find((split) => split.account_id === accountId)?.amount_minor ?? 0;
  }

  function toggle(txId: number) {
    const next = new Set(checkedIds);
    if (next.has(txId)) next.delete(txId);
    else next.add(txId);
    checkedIds = next;
  }

  function formatMinor(minor: number): string {
    const sign = minor < 0 ? "-" : "";
    const abs = Math.abs(minor);
    const whole = Math.floor(abs / factor);
    const frac = String(abs % factor).padStart(commodityScale, "0");
    return `${sign}${whole}.${frac} ${commoditySymbol || ""}`.trim();
  }

  function payeeName(id: number | null): string {
    if (id === null) return "-";
    return payees.find((payee) => payee.id === id)?.name ?? "-";
  }
</script>

<main class="py-6">
  <div class="container mx-auto px-6 space-y-6 max-w-5xl">
    {#if loading && !account}
      <p class="text-sm text-muted-foreground animate-pulse">Loading...</p>
    {:else if error}
      <Alert.Root variant="destructive">
        <Alert.Title>Error</Alert.Title>
        <Alert.Description>{error}</Alert.Description>
      </Alert.Root>
    {:else if account}
      {#if step === "setup"}
        <div>
          <h1 class="text-2xl font-bold tracking-tight">Reconcile: {account.name}</h1>
          <p class="text-sm text-muted-foreground mt-1">Match transactions to a statement and create a new balance lock.</p>
        </div>

        <form class="card max-w-xl space-y-4" onsubmit={(e) => { e.preventDefault(); loadCandidates(); }}>
          <div class="rounded-md bg-muted/40 p-3 text-sm">
            {#if history?.balancings.length}
              {@const latest = history.balancings.find((row) => !row.voided_at)}
              {#if latest}
                <p class="font-medium">Previous reconciliation</p>
                <p class="text-muted-foreground">{latest.as_of_date} at {formatMinor(latest.balance_minor)}</p>
              {:else}
                <p class="text-muted-foreground">No active reconciliation lock.</p>
              {/if}
            {:else}
              <p class="text-muted-foreground">No previous reconciliation.</p>
            {/if}
          </div>

          <div class="form-field">
            <label class="label" for="statement-start">Statement start date</label>
            <input id="statement-start" class="input" type="date" bind:value={statementStartDate} />
          </div>

          <div class="form-field">
            <label class="label" for="statement-end">Statement ending date</label>
            <input id="statement-end" class="input" type="date" bind:value={statementDate} required />
          </div>

          <div class="form-field">
            <label class="label" for="statement-balance">Statement ending balance</label>
            <input id="statement-balance" class="input" type="text" inputmode="decimal" bind:value={statementBalanceInput} placeholder="0.00" required />
          </div>

          <div class="flex gap-3 pt-2">
            <Button type="submit" disabled={loading || statementBalanceMinor === null}>{loading ? "Loading..." : "Start"}</Button>
            <Button variant="outline" onclick={() => goto(`/accounts/${accountId}`)}>Cancel</Button>
          </div>
        </form>
      {:else}
        <div class="sticky top-0 z-10 rounded-md border bg-background p-4 shadow-sm">
          <div class="grid grid-cols-2 gap-4 sm:grid-cols-4">
            <div>
              <p class="text-xs uppercase text-muted-foreground">Opening</p>
              <p class="font-mono font-medium">{formatMinor(openingBalanceMinor)}</p>
            </div>
            <div>
              <p class="text-xs uppercase text-muted-foreground">Selected</p>
              <p class="font-mono font-medium">{formatMinor(checkedAmountMinor)}</p>
            </div>
            <div>
              <p class="text-xs uppercase text-muted-foreground">Cleared</p>
              <p class="font-mono font-semibold">{formatMinor(clearedBalanceMinor)}</p>
            </div>
            <div>
              <p class="text-xs uppercase text-muted-foreground">Difference</p>
              <p class="font-mono font-semibold {needsOffset ? 'text-red-600' : 'text-green-600'}">{differenceMinor === null ? "-" : formatMinor(differenceMinor)}</p>
            </div>
          </div>
        </div>

        <div class="flex flex-wrap items-center justify-between gap-3">
          <p class="text-sm text-muted-foreground">{checkedIds.size} / {candidates.length} selected</p>
          <div class="flex gap-2">
            <Button variant="ghost" size="sm" onclick={() => (checkedIds = new Set(candidates.map((tx) => tx.transaction.id)))}>Select all</Button>
            <Button variant="ghost" size="sm" onclick={() => (checkedIds = new Set())}>Select none</Button>
            <Button variant="outline" size="sm" onclick={() => (step = "setup")}>Back</Button>
          </div>
        </div>

        <div class="data-table striped compact">
          <div class="data-row header reconcile-row">
            <div class="data-cell heading"></div>
            <div class="data-cell heading">Date</div>
            <div class="data-cell heading">Payee</div>
            <div class="data-cell heading">Memo</div>
            <div class="data-cell heading">Status</div>
            <div class="data-cell heading amount">Amount</div>
          </div>
          {#each sorted as tx (tx.transaction.id)}
            {@const checked = checkedIds.has(tx.transaction.id)}
            {@const amount = splitAmount(tx)}
            <label class="data-row reconcile-row reconcile-row-label {checked ? 'is-checked' : ''}">
              <div class="data-cell check-col">
                <input type="checkbox" checked={checked} onchange={() => toggle(tx.transaction.id)} aria-label="Mark as reconciled" />
              </div>
              <div class="data-cell">{tx.transaction.txn_date}</div>
              <div class="data-cell">{payeeName(tx.transaction.payee_id)}</div>
              <div class="data-cell text-muted-foreground">{tx.transaction.memo ?? "-"}</div>
              <div class="data-cell"><span class="badge">{tx.transaction.status}</span></div>
              <div class="data-cell amount {amount >= 0 ? 'text-green-600' : 'text-red-600'}">{formatMinor(amount)}</div>
            </label>
          {/each}
        </div>

        {#if needsOffset}
          <div class="card max-w-xl space-y-4">
            <div class="form-field">
              <label class="label" for="offset-account">Offset account</label>
              <select id="offset-account" class="input" bind:value={offsetAccountId} required>
                <option value="">Select account</option>
                {#each offsetChoices as option}
                  <option value={option.id}>{option.name}</option>
                {/each}
              </select>
            </div>
            <label class="flex items-center gap-2 text-sm">
              <input type="checkbox" bind:checked={rememberOffsetAccount} />
              Remember this offset account
            </label>
          </div>
        {/if}

        <div class="form-field max-w-xl">
          <label class="label" for="memo">Memo</label>
          <input id="memo" class="input" bind:value={memo} placeholder="Statement memo" />
        </div>

        {#if finishError}
          <Alert.Root variant="destructive">
            <Alert.Title>Error</Alert.Title>
            <Alert.Description>{finishError}</Alert.Description>
          </Alert.Root>
        {/if}

        <div class="flex flex-wrap gap-3">
          <Button onclick={finish} disabled={finishing || (needsOffset && !offsetAccountId)}>{finishing ? "Finishing..." : "Finish reconciliation"}</Button>
          <Button variant="ghost" onclick={() => goto(`/accounts/${accountId}`)}>Cancel</Button>
        </div>

        {#if history}
          <section class="space-y-3">
            <h2 class="text-lg font-semibold">History</h2>
            <div class="data-table compact">
              <div class="data-row header history-row">
                <div class="data-cell heading">Date</div>
                <div class="data-cell heading">Balance</div>
                <div class="data-cell heading">State</div>
              </div>
              {#each history.balancings.slice(0, 6) as row}
                <div class="data-row history-row">
                  <div class="data-cell">{row.as_of_date}</div>
                  <div class="data-cell font-mono">{formatMinor(row.balance_minor)}</div>
                  <div class="data-cell">{row.voided_at ? "Unlocked" : "Active"}</div>
                </div>
              {/each}
            </div>
          </section>
        {/if}
      {/if}
    {/if}
  </div>
</main>

<style>
  .reconcile-row {
    grid-template-columns: 2.5rem 8rem 1fr 1fr 7rem 9rem;
  }

  .history-row {
    grid-template-columns: 8rem 1fr 8rem;
  }

  .check-col {
    display: flex;
    align-items: center;
    justify-content: center;
  }

  .reconcile-row-label {
    cursor: pointer;
    display: grid;
  }

  .reconcile-row-label.is-checked {
    background: color-mix(in srgb, var(--color-primary, #2563eb) 8%, transparent);
  }

  .amount {
    text-align: right;
    font-variant-numeric: tabular-nums;
  }
</style>
