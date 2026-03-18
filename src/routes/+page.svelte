<script lang="ts">
  import { onMount } from "svelte";
  import { invoke } from "@tauri-apps/api/core";
  import * as Card from "$lib/components/ui/card";
  import { Button } from "$lib/components/ui/button";
  import * as Alert from "$lib/components/ui/alert";

  // Dashboard data
  type AccountBalance = {
    account_id: number;
    balance_minor: number;
    native_balance_minor: number;
    price_missing: boolean;
  };

  type Account = {
    id: number;
    name: string;
    type: string;
    commodity_id: number;
  };

  type TransactionWithSplits = {
    id: number;
    book_id: number;
    txn_date: string;
    memo: string | null;
    payee_id: number | null;
    payee_name: string | null;
    status: string | null;
    splits: Split[];
  };

  type Split = {
    id: number;
    tx_id: number;
    account_id: number;
    account_name: string;
    category_id: number | null;
    category_name: string | null;
    commodity_id: number;
    amount_minor: number;
    memo: string | null;
  };

  let busy = false;
  let error = "";
  let setupError = "";
  let dbReady: boolean | null = null;

  // Balances
  let accounts: Account[] = [];
  let balances: AccountBalance[] = [];
  let recentTransactions: TransactionWithSplits[] = [];

  // Computed totals
  let totalAssets = 0;
  let totalLiabilities = 0;
  let netWorth = 0;

  // By account type
  let assetAccounts: { account: Account; balance: number }[] = [];
  let liabilityAccounts: { account: Account; balance: number }[] = [];

  onMount(async () => {
    await checkDatabase();
  });

  async function checkDatabase() {
    busy = true;
    error = "";
    setupError = "";

    try {
      await invoke<string>("db_health");
      dbReady = true;
      await loadDashboardData();
    } catch (e) {
      const message = String(e);
      dbReady = false;
      if (!message.includes("db not initialized")) {
        setupError = `Database error: ${message}`;
      }
    } finally {
      busy = false;
    }
  }

  async function openDatabase() {
    setupError = "";
    busy = true;
    try {
      let selected = await invoke<string | null>("pick_storage_file");
      if (!selected) {
        selected = await invoke<string | null>("pick_storage_folder");
      }
      if (!selected) return;
      await invoke<string>("validate_and_set_storage_location", { path: selected });
      await checkDatabase();
    } catch (e) {
      setupError = `Failed to open database: ${String(e)}`;
    } finally {
      busy = false;
    }
  }

  async function createDatabase() {
    setupError = "";
    busy = true;
    try {
      const selected = await invoke<string | null>("pick_storage_folder");
      if (!selected) return;
      await invoke<string>("create_new_storage", { path: selected });
      await checkDatabase();
    } catch (e) {
      setupError = `Failed to create database: ${String(e)}`;
    } finally {
      busy = false;
    }
  }

  async function loadDashboardData() {
    busy = true;
    error = "";

    try {
      // Load accounts and balances in parallel
      const [accountsResult, balancesResult, transactionsResult] = await Promise.all([
        invoke<Account[]>("list_accounts", { bookId: 1, includeClosed: false }),
        invoke<AccountBalance[]>("list_account_balances", { bookId: 1 }),
        invoke<TransactionWithSplits[]>("list_transactions", {
          filter: {
            book_id: 1,
            limit: 10,
            offset: 0,
          },
        }),
      ]);

      accounts = accountsResult;
      balances = balancesResult;
      recentTransactions = transactionsResult;

      // Calculate totals
      calculateTotals();
    } catch (e) {
      error = `Failed to load dashboard data: ${String(e)}`;
    } finally {
      busy = false;
    }
  }

  function calculateTotals() {
    // Map balances by account ID
    const balanceMap = new Map<number, number>();
    for (const b of balances) {
      balanceMap.set(b.account_id, b.native_balance_minor);
    }

    // Asset types
    const assetTypes = ["checking", "savings", "cash", "asset", "investment", "brokerage", "stock"];
    // Liability types
    const liabilityTypes = ["credit", "credit_card", "loan", "liability"];

    totalAssets = 0;
    totalLiabilities = 0;
    assetAccounts = [];
    liabilityAccounts = [];

    for (const account of accounts) {
      const balance = balanceMap.get(account.id) || 0;

      if (assetTypes.includes(account.type)) {
        totalAssets += balance;
        assetAccounts.push({ account, balance });
      } else if (liabilityTypes.includes(account.type)) {
        totalLiabilities += balance;
        liabilityAccounts.push({ account, balance });
      }
    }

    netWorth = totalAssets + totalLiabilities; // liabilities are typically negative
  }

  function formatCurrency(minor: number) {
    return (minor / 100).toLocaleString(undefined, { minimumFractionDigits: 2, maximumFractionDigits: 2 });
  }

  function formatDate(dateStr: string) {
    return new Date(dateStr).toLocaleDateString();
  }

  function getTransactionAmount(tx: TransactionWithSplits): number {
    // Sum of positive splits (or first split amount)
    if (tx.splits.length === 0) return 0;
    const positive = tx.splits.filter((s) => s.amount_minor > 0);
    return positive.length > 0
      ? positive.reduce((sum, s) => sum + s.amount_minor, 0)
      : Math.abs(tx.splits[0].amount_minor);
  }

  function getTransactionDescription(tx: TransactionWithSplits): string {
    if (tx.memo) return tx.memo;
    if (tx.payee_name) return tx.payee_name;
    if (tx.splits.length > 0 && tx.splits[0].account_name) return tx.splits[0].account_name;
    return "(No description)";
  }
</script>

<main class="py-6">
  <div class="container mx-auto px-6 space-y-6">
    <!-- Header -->
    <div>
      <h1 class="text-3xl font-bold tracking-tight">Rekenraam</h1>
      <p class="text-muted-foreground">Personal finance tracking with a local-first database.</p>
    </div>

    {#if dbReady === true && error}
      <Alert.Root variant="destructive">
        <Alert.Title>Error</Alert.Title>
        <Alert.Description>{error}</Alert.Description>
      </Alert.Root>
    {/if}

    {#if dbReady === null}
      <div class="flex items-center justify-center py-16">
        <p class="text-muted-foreground animate-pulse">Starting up…</p>
      </div>
    {:else if dbReady === false}
      <!-- First-launch onboarding screen -->
      <div class="flex flex-col items-center justify-center py-16 space-y-8 text-center">
        <div class="space-y-3">
          <div class="text-5xl">🪙</div>
          <h2 class="text-3xl font-bold tracking-tight">Welcome to Rekenraam</h2>
          <p class="text-muted-foreground max-w-md">
            Your personal finance tracker — local-first, private, and built to last.
            Get started by creating a new book or opening an existing one.
          </p>
        </div>

        {#if setupError}
          <Alert.Root variant="destructive" class="max-w-md text-left">
            <Alert.Title>Error</Alert.Title>
            <Alert.Description>{setupError}</Alert.Description>
          </Alert.Root>
        {/if}

        <div class="flex flex-col sm:flex-row gap-4">
          <Button size="lg" disabled={busy} onclick={() => !busy && createDatabase()} class="min-w-44">
            <span class="mr-2 text-lg">✨</span>
            Create new book
          </Button>
          <Button size="lg" variant="outline" disabled={busy} onclick={() => !busy && openDatabase()} class="min-w-44">
            <span class="mr-2 text-lg">📂</span>
            Open existing file
          </Button>
        </div>

        <p class="text-xs text-muted-foreground max-w-xs">
          Your data is stored as a portable SQLite file on your computer.
          Nothing is sent to the cloud.
        </p>
      </div>
    {:else}

    <!-- Net Worth Summary Cards -->
    <div class="grid gap-4 md:grid-cols-3">
      <Card.Root class={netWorth >= 0 ? "border-green-200 bg-green-50" : "border-red-200 bg-red-50"}>
        <Card.Header class="pb-2">
          <Card.Description>Net Worth</Card.Description>
        </Card.Header>
        <Card.Content>
          <div class="text-3xl font-bold">{formatCurrency(netWorth)}</div>
        </Card.Content>
      </Card.Root>

      <Card.Root class="border-emerald-200 bg-emerald-50">
        <Card.Header class="pb-2">
          <Card.Description>Total Assets</Card.Description>
        </Card.Header>
        <Card.Content>
          <div class="text-2xl font-semibold text-emerald-700">{formatCurrency(totalAssets)}</div>
        </Card.Content>
      </Card.Root>

      <Card.Root class="border-rose-200 bg-rose-50">
        <Card.Header class="pb-2">
          <Card.Description>Total Liabilities</Card.Description>
        </Card.Header>
        <Card.Content>
          <div class="text-2xl font-semibold text-rose-700">{formatCurrency(Math.abs(totalLiabilities))}</div>
        </Card.Content>
      </Card.Root>
    </div>

    <!-- Quick Actions -->
    <Card.Root>
      <Card.Header>
        <Card.Title>Quick Actions</Card.Title>
      </Card.Header>
      <Card.Content>
        <div class="flex flex-wrap gap-3">
          <Button variant="outline" href="/transactions" class="flex flex-col h-auto py-3 px-4">
            <span class="text-xl mb-1">📝</span>
            <span>New Transaction</span>
          </Button>
          <Button variant="outline" href="/accounts" class="flex flex-col h-auto py-3 px-4">
            <span class="text-xl mb-1">🏦</span>
            <span>Accounts</span>
          </Button>
          <Button variant="outline" href="/investments" class="flex flex-col h-auto py-3 px-4">
            <span class="text-xl mb-1">📈</span>
            <span>Investments</span>
          </Button>
          <Button variant="outline" href="/reports" class="flex flex-col h-auto py-3 px-4">
            <span class="text-xl mb-1">📊</span>
            <span>Reports</span>
          </Button>
          <Button variant="outline" href="/settings" class="flex flex-col h-auto py-3 px-4">
            <span class="text-xl mb-1">⚙️</span>
            <span>Settings</span>
          </Button>
        </div>
      </Card.Content>
    </Card.Root>

    <!-- Account Balances and Recent Transactions -->
    <div class="grid gap-6 lg:grid-cols-2">
      <!-- Asset & Liability Accounts -->
      <div class="space-y-4">
        <Card.Root>
          <Card.Header>
            <Card.Title>Asset Accounts</Card.Title>
          </Card.Header>
          <Card.Content>
            {#if assetAccounts.length === 0}
              <p class="text-muted-foreground text-sm">No asset accounts.</p>
            {:else}
              <div class="space-y-1">
                {#each assetAccounts.sort((a, b) => b.balance - a.balance) as { account, balance }}
                  <a
                    href="/accounts/{account.id}"
                    class="flex justify-between items-center py-2 px-3 rounded-md hover:bg-accent transition-colors"
                  >
                    <span class="font-medium">{account.name}</span>
                    <span class="font-mono text-sm">{formatCurrency(balance)}</span>
                  </a>
                {/each}
              </div>
            {/if}
          </Card.Content>
        </Card.Root>

        <Card.Root>
          <Card.Header>
            <Card.Title>Liability Accounts</Card.Title>
          </Card.Header>
          <Card.Content>
            {#if liabilityAccounts.length === 0}
              <p class="text-muted-foreground text-sm">No liability accounts.</p>
            {:else}
              <div class="space-y-1">
                {#each liabilityAccounts.sort((a, b) => a.balance - b.balance) as { account, balance }}
                  <a
                    href="/accounts/{account.id}"
                    class="flex justify-between items-center py-2 px-3 rounded-md hover:bg-accent transition-colors"
                  >
                    <span class="font-medium">{account.name}</span>
                    <span class="font-mono text-sm text-rose-600">{formatCurrency(Math.abs(balance))}</span>
                  </a>
                {/each}
              </div>
            {/if}
          </Card.Content>
        </Card.Root>
      </div>

      <!-- Recent Transactions -->
      <Card.Root>
        <Card.Header class="flex flex-row items-center justify-between">
          <Card.Title>Recent Transactions</Card.Title>
          <Button variant="ghost" size="sm" href="/transactions">View All</Button>
        </Card.Header>
        <Card.Content>
          {#if recentTransactions.length === 0}
            <p class="text-muted-foreground text-sm">No recent transactions.</p>
          {:else}
            <div class="space-y-3">
              {#each recentTransactions as tx}
                <div class="flex justify-between items-center py-2 border-b border-border last:border-0">
                  <div class="flex flex-col gap-0.5">
                    <span class="text-xs text-muted-foreground">{formatDate(tx.txn_date)}</span>
                    <span class="font-medium">{getTransactionDescription(tx)}</span>
                  </div>
                  <span
                    class="font-mono text-sm {getTransactionAmount(tx) > 0
                      ? 'text-emerald-600'
                      : 'text-foreground'}"
                  >
                    {formatCurrency(getTransactionAmount(tx))}
                  </span>
                </div>
              {/each}
            </div>
          {/if}
        </Card.Content>
      </Card.Root>
    </div>
    {/if}
  </div>
</main>