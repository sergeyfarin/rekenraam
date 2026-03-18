<script lang="ts">
  import { onMount } from "svelte";
  import { page } from "$app/state";
  import { goto } from "$app/navigation";
  import { invoke } from "@tauri-apps/api/core";
  import { Button } from "$lib/components/ui/button";
  import * as Alert from "$lib/components/ui/alert";

  type Account = {
    id: number;
    name: string;
    account_type: string;
    commodity_id: number;
  };

  type Commodity = {
    id: number;
    symbol: string | null;
    name: string;
    scale: number;
  };

  type AccountBalancing = {
    id: number;
    account_id: number;
    as_of_date: string;
    balance_minor: number;
    memo: string | null;
  };

  type Transaction = {
    id: number;
    txn_date: string;
    payee_id: number | null;
    memo: string | null;
    status: string;
    reference: string | null;
  };

  type Split = {
    id: number;
    tx_id: number;
    account_id: number;
    commodity_id: number;
    amount_minor: number;
    category_id: number | null;
    tag_id: number | null;
    person_id: number | null;
    project_id: number | null;
    share_bps: number | null;
    memo: string | null;
  };

  type TxRow = {
    transaction: Transaction;
    splits: Split[];
  };

  type Payee = { id: number; name: string };

  // ── State ────────────────────────────────────────────────────────────────────
  let accountId: number | null = null;
  let account: Account | null = null;
  let commodity: Commodity | null = null;
  let payees: Payee[] = [];
  let lastBalancing: AccountBalancing | null = null;

  let step: "setup" | "working" = "setup";
  let statementDate = new Date().toISOString().slice(0, 10);
  let statementBalanceInput = "";

  let candidates: TxRow[] = [];
  let checkedIds = new Set<number>();

  let loading = false;
  let finishing = false;
  let error = "";
  let finishError = "";

  // ── Derived ──────────────────────────────────────────────────────────────────
  $: scale = commodity?.scale ?? 2;
  $: factor = 10 ** scale;

  $: openingBalanceMinor = lastBalancing?.balance_minor ?? 0;

  $: statementBalanceMinor = (() => {
    // Strip thousand-separators (commas) then parse
    const raw = statementBalanceInput.replace(/,/g, "").trim();
    if (!raw || isNaN(Number(raw))) return null;
    return Math.round(parseFloat(raw) * factor);
  })();

  function splitAmount(tx: TxRow): number {
    return tx.splits.find((s) => s.account_id === accountId)?.amount_minor ?? 0;
  }

  $: checkedAmountMinor = candidates
    .filter((tx) => checkedIds.has(tx.transaction.id))
    .reduce((sum, tx) => sum + splitAmount(tx), 0);

  $: clearedBalanceMinor = openingBalanceMinor + checkedAmountMinor;

  $: differenceMinor =
    statementBalanceMinor !== null ? clearedBalanceMinor - statementBalanceMinor : null;

  $: balanced = differenceMinor === 0;

  // Sort candidates oldest-first (matches bank statement order)
  $: sorted = [...candidates].sort((a, b) =>
    a.transaction.txn_date.localeCompare(b.transaction.txn_date)
  );

  // ── Lifecycle ────────────────────────────────────────────────────────────────
  onMount(async () => {
    const parsed = Number(page.params.id);
    if (!Number.isFinite(parsed)) {
      error = "Invalid account.";
      return;
    }
    accountId = parsed;
    await loadSetupData();
  });

  async function loadSetupData() {
    if (!accountId) return;
    loading = true;
    error = "";
    try {
      const [acct, comms, payeeList, bals] = await Promise.all([
        invoke<Account>("get_account", { id: accountId }),
        invoke<Commodity[]>("list_commodities", { bookId: 1 }),
        invoke<Payee[]>("list_payees", { bookId: 1 }),
        invoke<AccountBalancing[]>("list_account_balancings", { accountId }),
      ]);
      account = acct;
      commodity = comms.find((c) => c.id === acct.commodity_id) ?? null;
      payees = payeeList;
      // list_account_balancings returns DESC by date — first entry is most recent
      lastBalancing = bals.length > 0 ? bals[0] : null;
    } catch (e) {
      error = `Failed to load account: ${String(e)}`;
    } finally {
      loading = false;
    }
  }

  async function startReconciliation() {
    if (!accountId || statementBalanceMinor === null) return;
    loading = true;
    error = "";
    try {
      const allTx = await invoke<TxRow[]>("list_transactions", {
        filter: {
          book_id: 1,
          account_id: accountId,
          date_to: statementDate,
          limit: 10000,
          offset: 0,
        },
      });
      // Only show uncleared + cleared (not void or already reconciled)
      candidates = allTx.filter(
        (tx) => tx.transaction.status === "uncleared" || tx.transaction.status === "cleared"
      );
      // Pre-check already-cleared transactions
      checkedIds = new Set(
        candidates
          .filter((tx) => tx.transaction.status === "cleared")
          .map((tx) => tx.transaction.id)
      );
      step = "working";
    } catch (e) {
      error = `Failed to load transactions: ${String(e)}`;
    } finally {
      loading = false;
    }
  }

  function toggle(txId: number) {
    const next = new Set(checkedIds);
    if (next.has(txId)) {
      next.delete(txId);
    } else {
      next.add(txId);
    }
    checkedIds = next;
  }

  function selectAll() {
    checkedIds = new Set(candidates.map((tx) => tx.transaction.id));
  }

  function selectNone() {
    checkedIds = new Set();
  }

  async function finish() {
    if (!accountId || statementBalanceMinor === null) return;
    finishing = true;
    finishError = "";
    try {
      // Mark all checked transactions as "reconciled"
      const toReconcile = candidates.filter(
        (tx) => checkedIds.has(tx.transaction.id) && tx.transaction.status !== "reconciled"
      );
      for (const tx of toReconcile) {
        await invoke("update_transaction_with_splits", {
          input: {
            id: tx.transaction.id,
            book_id: 1,
            txn_date: tx.transaction.txn_date,
            payee_id: tx.transaction.payee_id,
            memo: tx.transaction.memo,
            status: "reconciled",
            reference: tx.transaction.reference,
            import_id: null,
            splits: tx.splits.map((s) => ({
              account_id: s.account_id,
              commodity_id: s.commodity_id,
              amount_minor: s.amount_minor,
              category_id: s.category_id,
              tag_id: s.tag_id,
              person_id: s.person_id,
              project_id: s.project_id,
              share_bps: s.share_bps,
              memo: s.memo,
            })),
          },
        });
      }
      // Create account balancing (locks the account through this date)
      await invoke("create_account_balancing", {
        input: {
          book_id: 1,
          account_id: accountId,
          as_of_date: statementDate,
          balance_minor: statementBalanceMinor,
          memo: null,
        },
      });
      goto(`/accounts/${accountId}`);
    } catch (e) {
      finishError = `Failed to finish reconciliation: ${String(e)}`;
    } finally {
      finishing = false;
    }
  }

  // ── Formatting ───────────────────────────────────────────────────────────────
  function formatMinor(minor: number): string {
    if (!commodity) return String(minor);
    const s = commodity.scale;
    const f = 10 ** s;
    const sign = minor < 0 ? "-" : "";
    const abs = Math.abs(minor);
    const whole = Math.floor(abs / f);
    const frac = String(abs % f).padStart(s, "0");
    const sym = commodity.symbol ?? commodity.name;
    return `${sign}${whole}.${frac} ${sym}`;
  }

  function payeeName(id: number | null): string {
    if (!id) return "—";
    return payees.find((p) => p.id === id)?.name ?? "—";
  }
</script>

<main class="py-6">
  <div class="container mx-auto px-6 space-y-6 max-w-5xl">

    {#if loading && !account}
      <p class="text-sm text-muted-foreground animate-pulse">Loading…</p>
    {:else if error}
      <Alert.Root variant="destructive">
        <Alert.Title>Error</Alert.Title>
        <Alert.Description>{error}</Alert.Description>
      </Alert.Root>
    {:else if account}

      <!-- ── STEP 1: SETUP ──────────────────────────────────────────────── -->
      {#if step === "setup"}
        <div>
          <h1 class="text-2xl font-bold tracking-tight">Reconcile: {account.name}</h1>
          <p class="text-sm text-muted-foreground mt-1">
            Match your transactions to a bank statement.
          </p>
        </div>

        <div class="card max-w-lg">
          <!-- Last reconciliation info -->
          {#if lastBalancing}
            <div class="mb-5 p-3 rounded-md bg-muted/40 text-sm space-y-0.5">
              <p class="font-medium">Previous reconciliation</p>
              <p class="text-muted-foreground">
                {lastBalancing.as_of_date} · Opening balance: <strong>{formatMinor(lastBalancing.balance_minor)}</strong>
              </p>
            </div>
          {:else}
            <div class="mb-5 p-3 rounded-md bg-muted/40 text-sm">
              <p class="text-muted-foreground">No previous reconciliation — opening balance is <strong>0</strong>.</p>
            </div>
          {/if}

          <form
            class="space-y-4"
            onsubmit={(e) => { e.preventDefault(); startReconciliation(); }}
          >
            <div class="form-field">
              <label class="label" for="stmt-date">Statement ending date</label>
              <input
                id="stmt-date"
                class="input"
                type="date"
                bind:value={statementDate}
                required
              />
            </div>

            <div class="form-field">
              <label class="label" for="stmt-balance">Statement ending balance</label>
              <input
                id="stmt-balance"
                class="input"
                type="text"
                inputmode="decimal"
                placeholder="0.00"
                bind:value={statementBalanceInput}
                required
              />
              {#if statementBalanceMinor !== null}
                <p class="text-xs text-muted-foreground mt-1">
                  Parsed: {formatMinor(statementBalanceMinor)}
                </p>
              {:else if statementBalanceInput}
                <p class="text-xs text-red-500 mt-1">Enter a valid number</p>
              {/if}
            </div>

            <div class="flex gap-3 pt-2">
              <Button
                type="submit"
                disabled={loading || statementBalanceMinor === null}
              >
                {loading ? "Loading…" : "Start Reconciliation"}
              </Button>
              <Button variant="outline" onclick={() => goto(`/accounts/${accountId}`)}>
                Cancel
              </Button>
            </div>
          </form>
        </div>

      <!-- ── STEP 2: WORKING ────────────────────────────────────────────── -->
      {:else}
        <!-- Sticky balance summary -->
        <div class="reconcile-summary sticky top-0 z-10 border rounded-lg p-4 bg-background shadow-sm">
          <div class="grid grid-cols-2 sm:grid-cols-4 gap-4">
            <div>
              <p class="text-xs text-muted-foreground uppercase tracking-wide">Opening</p>
              <p class="font-mono font-medium">{formatMinor(openingBalanceMinor)}</p>
            </div>
            <div>
              <p class="text-xs text-muted-foreground uppercase tracking-wide">+ Cleared</p>
              <p class="font-mono font-medium">{formatMinor(checkedAmountMinor)}</p>
            </div>
            <div>
              <p class="text-xs text-muted-foreground uppercase tracking-wide">Cleared Balance</p>
              <p class="font-mono font-semibold">{formatMinor(clearedBalanceMinor)}</p>
            </div>
            <div>
              <p class="text-xs text-muted-foreground uppercase tracking-wide">Statement</p>
              <p class="font-mono font-medium">
                {statementBalanceMinor !== null ? formatMinor(statementBalanceMinor) : "—"}
              </p>
            </div>
          </div>

          <div class="mt-3 flex items-center gap-3">
            {#if differenceMinor === null}
              <span class="text-sm text-muted-foreground">Enter statement balance to see difference.</span>
            {:else if balanced}
              <span class="text-sm font-semibold text-green-600">✓ Balanced — ready to finish!</span>
            {:else}
              <span class="text-sm font-semibold text-red-500">
                Difference: {formatMinor(differenceMinor)}
              </span>
              <span class="text-xs text-muted-foreground">Keep checking or unchecking transactions.</span>
            {/if}
          </div>
        </div>

        <!-- Toolbar -->
        <div class="flex flex-wrap items-center justify-between gap-3">
          <div class="flex items-center gap-2">
            <span class="text-sm text-muted-foreground">
              {checkedIds.size} / {candidates.length} selected
            </span>
            <Button variant="ghost" size="sm" onclick={selectAll}>Select all</Button>
            <Button variant="ghost" size="sm" onclick={selectNone}>Select none</Button>
          </div>
          <div class="flex items-center gap-2">
            <Button
              variant="outline"
              size="sm"
              onclick={() => { step = "setup"; finishError = ""; }}
            >
              ← Back
            </Button>
          </div>
        </div>

        <!-- Transaction checklist -->
        {#if candidates.length === 0}
          <div class="card py-12 text-center">
            <p class="text-muted-foreground">
              No unreconciled transactions up to {statementDate}.
            </p>
            <p class="text-sm text-muted-foreground mt-1">
              All transactions in this period are already reconciled.
            </p>
          </div>
        {:else}
          <div class="data-table striped compact reconcile-table">
            <!-- Header -->
            <div class="data-row header reconcile-row">
              <div class="data-cell heading check-col">✓</div>
              <div class="data-cell heading">Date</div>
              <div class="data-cell heading">Payee</div>
              <div class="data-cell heading">Memo</div>
              <div class="data-cell heading">Status</div>
              <div class="data-cell heading amount">Amount</div>
            </div>

            {#each sorted as tx (tx.transaction.id)}
              {@const checked = checkedIds.has(tx.transaction.id)}
              {@const amount = splitAmount(tx)}
              <label
                class="data-row reconcile-row reconcile-row-label {checked ? 'is-checked' : ''}"
              >
                <div class="data-cell check-col">
                  <input
                    type="checkbox"
                    checked={checked}
                    onchange={() => toggle(tx.transaction.id)}
                    aria-label="Mark as cleared"
                  />
                </div>
                <div class="data-cell">{tx.transaction.txn_date}</div>
                <div class="data-cell">{payeeName(tx.transaction.payee_id)}</div>
                <div class="data-cell text-muted-foreground">{tx.transaction.memo ?? "—"}</div>
                <div class="data-cell">
                  <span class="badge {tx.transaction.status === 'cleared' ? 'warning' : ''}">
                    {tx.transaction.status}
                  </span>
                </div>
                <div class="data-cell amount {amount >= 0 ? 'text-green-600' : 'text-red-600'}">
                  {formatMinor(amount)}
                </div>
              </label>
            {/each}
          </div>
        {/if}

        <!-- Finish actions -->
        {#if finishError}
          <Alert.Root variant="destructive">
            <Alert.Title>Error</Alert.Title>
            <Alert.Description>{finishError}</Alert.Description>
          </Alert.Root>
        {/if}

        <div class="flex flex-wrap items-center gap-3 pb-8">
          {#if balanced}
            <Button onclick={finish} disabled={finishing}>
              {finishing ? "Finishing…" : "✓ Finish Reconciliation"}
            </Button>
          {:else}
            <div class="flex items-center gap-3">
              <Button variant="outline" onclick={finish} disabled={finishing}>
                {finishing ? "Finishing…" : "Finish Anyway"}
              </Button>
              <span class="text-sm text-muted-foreground">
                The {differenceMinor !== null ? formatMinor(Math.abs(differenceMinor)) : "—"} difference will be recorded.
              </span>
            </div>
          {/if}
          <Button variant="ghost" onclick={() => goto(`/accounts/${accountId}`)}>
            Cancel
          </Button>
        </div>

      {/if}
    {/if}
  </div>
</main>

<style>
  .reconcile-row {
    grid-template-columns: 2.5rem 8rem 1fr 1fr 7rem 9rem;
  }

  .check-col {
    display: flex;
    align-items: center;
    justify-content: center;
  }

  .reconcile-row-label {
    cursor: pointer;
    display: grid;
    transition: background-color 0.1s;
  }

  .reconcile-row-label.is-checked {
    background-color: color-mix(in srgb, var(--color-primary, #2563eb) 8%, transparent);
  }

  .reconcile-row-label:hover {
    background-color: color-mix(in srgb, currentColor 5%, transparent);
  }

  .reconcile-table input[type="checkbox"] {
    width: 1rem;
    height: 1rem;
    cursor: pointer;
  }

  .amount {
    text-align: right;
    font-variant-numeric: tabular-nums;
  }
</style>
