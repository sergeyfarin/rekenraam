<script lang="ts">
  import { onMount } from "svelte";
  import { listAccountBalances, listAccounts, type AccountSummary, type AccountBalanceSummary } from "$lib/api/accounts";
  import { getHealthStatus, listBooks } from "$lib/api/books";
  import { listPayees, type PayeeSummary } from "$lib/api/metadata";
  import { listTransactions, type TransactionWithSplits } from "$lib/api/transactions";
  import { formatError } from "$lib/utils";
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

  type DefaultCurrency = { scale: number; symbol: string | null; display_symbol: string | null };

  let busy = false;
  let error = "";
  let setupError = "";
  let dbReady: boolean | null = null;

  // Balances
  let accounts: AccountSummary[] = [];
  let balances: AccountBalance[] = [];
  let recentTransactions: TransactionWithSplits[] = [];
  let payees: PayeeSummary[] = [];
  let nativeCurrency: DefaultCurrency | null = null;

  // Computed totals
  let totalAssets = 0;
  let totalLiabilities = 0;
  let netWorth = 0;

  // By account type
  let assetAccounts: { account: AccountSummary; balance: number }[] = [];
  let liabilityAccounts: { account: AccountSummary; balance: number }[] = [];

  onMount(async () => {
    await checkDatabase();
  });

  async function checkDatabase() {
    busy = true;
    error = "";
    setupError = "";

    try {
      await getHealthStatus();
      dbReady = true;
      await loadDashboardData();
    } catch (e) {
      dbReady = false;
      setupError = `Backend unavailable: ${formatError(e)}`;
    } finally {
      busy = false;
    }
  }

  async function loadDashboardData() {
    busy = true;
    error = "";

    try {
      // Load accounts, balances, recent transactions, and native currency in parallel
      const nativeCurrencyPromise = listBooks()
        .then((books) => {
          const primaryBook = books[0] ?? null;
          if (primaryBook === null) {
            return null;
          }

          return {
            scale: 2,
            symbol: primaryBook.base_currency_code,
            display_symbol: primaryBook.base_currency_code,
          } satisfies DefaultCurrency;
        })
        .catch(() => null);

      const [accountsResult, balancesResult, transactionsResult, payeesResult, nativeCurrencyResult] = await Promise.all([
        listAccounts(1),
        listAccountBalances(1),
        listTransactions({
          book_id: 1,
          limit: 10,
          offset: 0,
        }),
        listPayees(1),
        nativeCurrencyPromise,
      ]);

      accounts = accountsResult;
      balances = balancesResult.map((balance: AccountBalanceSummary) => ({
        account_id: balance.account_id,
        balance_minor: balance.balance_minor,
        native_balance_minor: balance.balance_minor,
        price_missing: false,
      }));
      recentTransactions = transactionsResult;
      payees = payeesResult;
      nativeCurrency = nativeCurrencyResult;

      // Calculate totals
      calculateTotals();
    } catch (e) {
      error = `Failed to load dashboard data: ${formatError(e)}`;
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

    // Asset types should stay aligned with backend account validation.
    const assetTypes = ["cash", "checking", "savings", "asset", "investment"];
    // Liability types
    const liabilityTypes = ["credit", "loan", "liability"];

    totalAssets = 0;
    totalLiabilities = 0;
    assetAccounts = [];
    liabilityAccounts = [];

    for (const account of accounts) {
      const balance = balanceMap.get(account.id) || 0;

      if (assetTypes.includes(account.account_type)) {
        totalAssets += balance;
        assetAccounts.push({ account, balance });
      } else if (liabilityTypes.includes(account.account_type)) {
        totalLiabilities += balance;
        liabilityAccounts.push({ account, balance });
      }
    }

    netWorth = totalAssets + totalLiabilities; // liabilities are typically negative
  }

  function formatCurrency(minor: number): string {
    const scale = nativeCurrency?.scale ?? 2;
    const symbol = nativeCurrency?.display_symbol ?? nativeCurrency?.symbol ?? "";
    if (scale <= 0) return `${symbol}${minor}`.trim();
    const factor = 10 ** scale;
    const sign = minor < 0 ? "-" : "";
    const abs = Math.abs(minor);
    const whole = Math.floor(abs / factor);
    const fraction = String(abs % factor).padStart(scale, "0");
    return `${sign}${symbol}${whole}.${fraction}`.trim();
  }

  function formatDate(dateStr: string) {
    return new Date(dateStr).toLocaleDateString();
  }

  function payeeName(payeeId: number | null): string | null {
    if (!payeeId) return null;
    return payees.find((payee) => payee.id === payeeId)?.name ?? null;
  }

  function accountName(accountId: number): string | null {
    return accounts.find((account) => account.id === accountId)?.name ?? null;
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
    if (tx.transaction.memo) return tx.transaction.memo;
    const payee = payeeName(tx.transaction.payee_id);
    if (payee) return payee;
    if (tx.splits.length > 0) {
      const name = accountName(tx.splits[0].account_id);
      if (name) return name;
    }
    return "(No description)";
  }
</script>

<main class="py-6">
  <div class="container mx-auto px-6 space-y-6">
    <!-- Header -->
    <div>
      <h1 class="text-3xl font-bold tracking-tight">Rekenraam</h1>
      <p class="text-muted-foreground">Self-hosted personal finance tracking through the web app.</p>
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
      <div class="flex flex-col items-center justify-center py-16 space-y-6 text-center">
        <div class="space-y-3 max-w-md">
          <h2 class="text-3xl font-bold tracking-tight">Backend unavailable</h2>
          <p class="text-muted-foreground">
            The web app could not reach the configured API backend. Check the server and API base URL,
            then retry.
          </p>
        </div>

        {#if setupError}
          <Alert.Root variant="destructive" class="max-w-md text-left">
            <Alert.Title>Error</Alert.Title>
            <Alert.Description>{setupError}</Alert.Description>
          </Alert.Root>
        {/if}

        <Button size="lg" disabled={busy} onclick={() => !busy && checkDatabase()} class="min-w-44">
          Retry connection
        </Button>
      </div>
    {:else}

    <!-- Net Worth Summary Cards -->
    <div class="grid gap-4 md:grid-cols-3">
      <Card.Root class={netWorth >= 0 ? "surface-money-positive" : "surface-money-negative"}>
        <Card.Header class="pb-2">
          <Card.Description>Net Worth</Card.Description>
        </Card.Header>
        <Card.Content>
          <div class="text-3xl font-bold">{formatCurrency(netWorth)}</div>
        </Card.Content>
      </Card.Root>

      <Card.Root class="surface-money-asset">
        <Card.Header class="pb-2">
          <Card.Description>Total Assets</Card.Description>
        </Card.Header>
        <Card.Content>
          <div class="text-2xl font-semibold text-money-asset-strong">{formatCurrency(totalAssets)}</div>
        </Card.Content>
      </Card.Root>

      <Card.Root class="surface-money-liability">
        <Card.Header class="pb-2">
          <Card.Description>Total Liabilities</Card.Description>
        </Card.Header>
        <Card.Content>
          <div class="text-2xl font-semibold text-money-liability-strong">{formatCurrency(Math.abs(totalLiabilities))}</div>
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
                    <span class="font-mono text-sm text-money-liability">{formatCurrency(Math.abs(balance))}</span>
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
                    <span class="text-xs text-muted-foreground">{formatDate(tx.transaction.txn_date)}</span>
                    <span class="font-medium">{getTransactionDescription(tx)}</span>
                  </div>
                  <span
                    class="font-mono text-sm {getTransactionAmount(tx) > 0
                      ? 'text-money-positive'
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
