<script lang="ts">
  import { onMount } from "svelte";
  import { invoke } from "@tauri-apps/api/core";
  import * as Card from "$lib/components/ui/card";
  import { Button } from "$lib/components/ui/button";
  import { Input } from "$lib/components/ui/input";
  import { Label } from "$lib/components/ui/label";
  import * as Tabs from "$lib/components/ui/tabs";
  import * as Table from "$lib/components/ui/table";
  import * as Alert from "$lib/components/ui/alert";

  // Tab state
  let activeTab: "cashflow" | "spending" | "payees" | "gains" = "cashflow";

  // Date filters
  let dateFrom = "";
  let dateTo = "";
  let groupBy = "month";

  // Loading state
  let busy = false;
  let error = "";

  // --- Cashflow report ---
  type CashflowRow = {
    period_start: string;
    inflow_minor: number;
    outflow_minor: number;
    net_minor: number;
  };

  let cashflowData: CashflowRow[] = [];
  let cashflowTotals = { inflow: 0, outflow: 0, net: 0 };

  // --- Category spend report ---
  type CategorySpendRow = {
    category_id: number;
    category_name: string;
    total_minor: number;
  };

  let categorySpendData: CategorySpendRow[] = [];
  let categorySpendTotal = 0;

  // --- Payee totals report ---
  type PayeeTotalRow = {
    payee_id: number;
    payee_name: string;
    total_minor: number;
  };

  let payeeTotalsData: PayeeTotalRow[] = [];
  let payeeTotalsTotal = 0;

  // --- Gains reports ---
  type RealizedGainEntry = {
    tx_id: number;
    txn_date: string;
    commodity_id: number;
    quantity_minor: number;
    proceeds_minor: number;
    quote_commodity_id: number | null;
    cost_basis_minor: number;
    gain_loss_minor: number;
    proceeds_missing: boolean;
  };

  type UnrealizedGainEntry = {
    account_id: number;
    account_name: string;
    account_type: string;
    commodity_id: number;
    commodity_name: string;
    value_minor: number;
    cost_basis_minor: number;
    unrealized_gain_minor: number;
    price_missing: boolean;
  };

  type Commodity = {
    id: number;
    book_id: number;
    kind: string;
    symbol: string | null;
    name: string;
    scale: number;
  };

  let realizedGains: RealizedGainEntry[] = [];
  let unrealizedGains: UnrealizedGainEntry[] = [];
  let commodities: Commodity[] = [];
  let selectedCommodityId: number | null = null;
  let realizedGainsTotal = 0;
  let unrealizedGainsTotal = 0;

  onMount(async () => {
    // Set default date range to current year
    const now = new Date();
    dateFrom = `${now.getFullYear()}-01-01`;
    dateTo = now.toISOString().split("T")[0];

    await loadCommodities();
    await loadCashflow();
  });

  async function loadCommodities() {
    try {
      commodities = await invoke<Commodity[]>("list_commodities", { bookId: 1 });
      // Default to first currency if available
      const currencies = commodities.filter((c) => c.kind === "currency");
      if (currencies.length > 0) {
        selectedCommodityId = currencies[0].id;
      }
    } catch (e) {
      error = `Failed to load commodities: ${String(e)}`;
    }
  }

  async function loadCashflow() {
    error = "";
    busy = true;
    try {
      cashflowData = await invoke<CashflowRow[]>("report_cashflow", {
        input: {
          book_id: 1,
          date_from: dateFrom || null,
          date_to: dateTo || null,
          group_by: groupBy,
        },
      });

      // Calculate totals
      cashflowTotals = cashflowData.reduce(
        (acc, row) => ({
          inflow: acc.inflow + row.inflow_minor,
          outflow: acc.outflow + row.outflow_minor,
          net: acc.net + row.net_minor,
        }),
        { inflow: 0, outflow: 0, net: 0 }
      );
    } catch (e) {
      error = `Failed to load cashflow report: ${String(e)}`;
    } finally {
      busy = false;
    }
  }

  async function loadCategorySpend() {
    error = "";
    busy = true;
    try {
      categorySpendData = await invoke<CategorySpendRow[]>("report_category_spend", {
        input: {
          book_id: 1,
          date_from: dateFrom || null,
          date_to: dateTo || null,
          category_ids: null,
        },
      });

      categorySpendTotal = categorySpendData.reduce((sum, row) => sum + row.total_minor, 0);
    } catch (e) {
      error = `Failed to load category spend report: ${String(e)}`;
    } finally {
      busy = false;
    }
  }

  async function loadPayeeTotals() {
    error = "";
    busy = true;
    try {
      payeeTotalsData = await invoke<PayeeTotalRow[]>("report_payee_totals", {
        input: {
          book_id: 1,
          date_from: dateFrom || null,
          date_to: dateTo || null,
          payee_ids: null,
        },
      });

      payeeTotalsTotal = payeeTotalsData.reduce((sum, row) => sum + row.total_minor, 0);
    } catch (e) {
      error = `Failed to load payee totals report: ${String(e)}`;
    } finally {
      busy = false;
    }
  }

  async function loadRealizedGains() {
    error = "";
    busy = true;
    try {
      realizedGains = await invoke<RealizedGainEntry[]>("realized_gains_report", {
        dateFrom: dateFrom || null,
        dateTo: dateTo || null,
      });

      realizedGainsTotal = realizedGains.reduce((sum, row) => sum + row.gain_loss_minor, 0);
    } catch (e) {
      error = `Failed to load realized gains report: ${String(e)}`;
    } finally {
      busy = false;
    }
  }

  async function loadUnrealizedGains() {
    if (!selectedCommodityId) return;

    error = "";
    busy = true;
    try {
      unrealizedGains = await invoke<UnrealizedGainEntry[]>("unrealized_gains_report", {
        baseCommodityId: selectedCommodityId,
        asOfDate: dateTo || null,
      });

      unrealizedGainsTotal = unrealizedGains.reduce((sum, row) => sum + row.unrealized_gain_minor, 0);
    } catch (e) {
      error = `Failed to load unrealized gains report: ${String(e)}`;
    } finally {
      busy = false;
    }
  }

  async function loadGains() {
    await loadRealizedGains();
    await loadUnrealizedGains();
  }

  function formatCurrency(minor: number, scale = 100) {
    const value = minor / scale;
    return value.toLocaleString(undefined, { minimumFractionDigits: 2, maximumFractionDigits: 2 });
  }

  function formatDate(dateStr: string) {
    if (!dateStr) return "—";
    const date = new Date(dateStr);
    return date.toLocaleDateString();
  }

  function formatPeriod(periodStart: string) {
    if (!periodStart) return "—";
    const date = new Date(periodStart);
    if (groupBy === "year") {
      return date.getFullYear().toString();
    } else if (groupBy === "quarter") {
      const q = Math.ceil((date.getMonth() + 1) / 3);
      return `Q${q} ${date.getFullYear()}`;
    } else if (groupBy === "month") {
      return date.toLocaleDateString(undefined, { year: "numeric", month: "short" });
    }
    return date.toLocaleDateString();
  }

  async function handleTabChange(tab: typeof activeTab) {
    activeTab = tab;
    if (tab === "cashflow" && cashflowData.length === 0) {
      await loadCashflow();
    } else if (tab === "spending" && categorySpendData.length === 0) {
      await loadCategorySpend();
    } else if (tab === "payees" && payeeTotalsData.length === 0) {
      await loadPayeeTotals();
    } else if (tab === "gains" && realizedGains.length === 0) {
      await loadGains();
    }
  }

  async function refreshReport() {
    if (activeTab === "cashflow") {
      await loadCashflow();
    } else if (activeTab === "spending") {
      await loadCategorySpend();
    } else if (activeTab === "payees") {
      await loadPayeeTotals();
    } else if (activeTab === "gains") {
      await loadGains();
    }
  }
</script>

<main class="py-6">
  <div class="container mx-auto px-6 space-y-6">
    <div>
      <h1 class="text-3xl font-bold tracking-tight">Reports</h1>
      <p class="text-muted-foreground">Generate financial reports and insights.</p>
    </div>

    <!-- Filters -->
    <Card.Root>
      <Card.Content class="pt-6">
        <div class="flex flex-wrap items-center gap-4">
          <div class="flex items-center gap-2">
            <Label for="date-from">From:</Label>
            <Input id="date-from" type="date" bind:value={dateFrom} class="w-40" />
          </div>
          <div class="flex items-center gap-2">
            <Label for="date-to">To:</Label>
            <Input id="date-to" type="date" bind:value={dateTo} class="w-40" />
          </div>
          {#if activeTab === "cashflow"}
            <div class="flex items-center gap-2">
              <Label for="group-by">Group by:</Label>
              <select id="group-by" class="flex h-9 rounded-md border border-input bg-transparent px-3 py-1 text-sm" bind:value={groupBy}>
                <option value="day">Day</option>
                <option value="month">Month</option>
                <option value="quarter">Quarter</option>
                <option value="year">Year</option>
              </select>
            </div>
          {/if}
          {#if activeTab === "gains"}
            <div class="flex items-center gap-2">
              <Label for="quote-currency">Quote currency:</Label>
              <select id="quote-currency" class="flex h-9 rounded-md border border-input bg-transparent px-3 py-1 text-sm" bind:value={selectedCommodityId}>
                {#each commodities.filter((c) => c.kind === "currency") as c}
                  <option value={c.id}>{c.symbol || c.name}</option>
                {/each}
              </select>
            </div>
          {/if}
          <Button onclick={refreshReport} disabled={busy}>
            {busy ? "Loading..." : "Refresh"}
          </Button>
        </div>
      </Card.Content>
    </Card.Root>

    {#if error}
      <Alert.Root variant="destructive">
        <Alert.Title>Error</Alert.Title>
        <Alert.Description>{error}</Alert.Description>
      </Alert.Root>
    {/if}

    <!-- Tabs -->
    <Tabs.Root value={activeTab} onValueChange={(v) => handleTabChange(v as typeof activeTab)}>
      <Tabs.List>
        <Tabs.Trigger value="cashflow">Cash Flow</Tabs.Trigger>
        <Tabs.Trigger value="spending">Spending by Category</Tabs.Trigger>
        <Tabs.Trigger value="payees">Spending by Payee</Tabs.Trigger>
        <Tabs.Trigger value="gains">Investment Gains</Tabs.Trigger>
      </Tabs.List>

      <!-- Cashflow Tab -->
      <Tabs.Content value="cashflow">
        <Card.Root>
          <Card.Header>
            <Card.Title>Cash Flow Report</Card.Title>
          </Card.Header>
          <Card.Content>
            {#if cashflowData.length === 0}
              <p class="text-muted-foreground">No data for selected period.</p>
            {:else}
              <div class="grid gap-4 md:grid-cols-3 mb-6">
                <Card.Root class="border-green-200 bg-green-50">
                  <Card.Header class="pb-2">
                    <Card.Description>Total Inflow</Card.Description>
                  </Card.Header>
                  <Card.Content>
                    <div class="text-2xl font-bold text-green-700">{formatCurrency(cashflowTotals.inflow)}</div>
                  </Card.Content>
                </Card.Root>
                <Card.Root class="border-red-200 bg-red-50">
                  <Card.Header class="pb-2">
                    <Card.Description>Total Outflow</Card.Description>
                  </Card.Header>
                  <Card.Content>
                    <div class="text-2xl font-bold text-red-700">{formatCurrency(cashflowTotals.outflow)}</div>
                  </Card.Content>
                </Card.Root>
                <Card.Root class={cashflowTotals.net >= 0 ? "border-green-200 bg-green-50" : "border-red-200 bg-red-50"}>
                  <Card.Header class="pb-2">
                    <Card.Description>Net Cash Flow</Card.Description>
                  </Card.Header>
                  <Card.Content>
                    <div class="text-2xl font-bold">{formatCurrency(cashflowTotals.net)}</div>
                  </Card.Content>
                </Card.Root>
              </div>

              <Table.Root>
                <Table.Header>
                  <Table.Row>
                    <Table.Head>Period</Table.Head>
                    <Table.Head class="text-right">Inflow</Table.Head>
                    <Table.Head class="text-right">Outflow</Table.Head>
                    <Table.Head class="text-right">Net</Table.Head>
                  </Table.Row>
                </Table.Header>
                <Table.Body>
                  {#each cashflowData as row}
                    <Table.Row>
                      <Table.Cell>{formatPeriod(row.period_start)}</Table.Cell>
                      <Table.Cell class="text-right text-green-600">{formatCurrency(row.inflow_minor)}</Table.Cell>
                      <Table.Cell class="text-right text-red-600">{formatCurrency(row.outflow_minor)}</Table.Cell>
                      <Table.Cell class="text-right {row.net_minor >= 0 ? 'text-green-600' : 'text-red-600'}">
                        {formatCurrency(row.net_minor)}
                      </Table.Cell>
                    </Table.Row>
                  {/each}
                </Table.Body>
              </Table.Root>
            {/if}
          </Card.Content>
        </Card.Root>
      </Tabs.Content>

      <!-- Category Spend Tab -->
      <Tabs.Content value="spending">
        <Card.Root>
          <Card.Header>
            <Card.Title>Spending by Category</Card.Title>
          </Card.Header>
          <Card.Content>
            {#if categorySpendData.length === 0}
              <p class="text-muted-foreground">No spending data for selected period.</p>
            {:else}
              <div class="mb-6">
                <Card.Root class="border-red-200 bg-red-50 w-fit">
                  <Card.Header class="pb-2">
                    <Card.Description>Total Spending</Card.Description>
                  </Card.Header>
                  <Card.Content>
                    <div class="text-2xl font-bold text-red-700">{formatCurrency(Math.abs(categorySpendTotal))}</div>
                  </Card.Content>
                </Card.Root>
              </div>

              <Table.Root>
                <Table.Header>
                  <Table.Row>
                    <Table.Head>Category</Table.Head>
                    <Table.Head class="text-right">Amount</Table.Head>
                    <Table.Head class="text-right">% of Total</Table.Head>
                  </Table.Row>
                </Table.Header>
                <Table.Body>
                  {#each categorySpendData.sort((a, b) => Math.abs(b.total_minor) - Math.abs(a.total_minor)) as row}
                    <Table.Row>
                      <Table.Cell>{row.category_name || "(No category)"}</Table.Cell>
                      <Table.Cell class="text-right {row.total_minor < 0 ? 'text-red-600' : 'text-green-600'}">
                        {formatCurrency(row.total_minor)}
                      </Table.Cell>
                      <Table.Cell class="text-right text-muted-foreground">
                        {categorySpendTotal !== 0 ? ((Math.abs(row.total_minor) / Math.abs(categorySpendTotal)) * 100).toFixed(1) : 0}%
                      </Table.Cell>
                    </Table.Row>
                  {/each}
                </Table.Body>
              </Table.Root>
            {/if}
          </Card.Content>
        </Card.Root>
      </Tabs.Content>

      <!-- Payee Totals Tab -->
      <Tabs.Content value="payees">
        <Card.Root>
          <Card.Header>
            <Card.Title>Spending by Payee</Card.Title>
          </Card.Header>
          <Card.Content>
            {#if payeeTotalsData.length === 0}
              <p class="text-muted-foreground">No payee data for selected period.</p>
            {:else}
              <div class="mb-6">
                <Card.Root class="w-fit">
                  <Card.Header class="pb-2">
                    <Card.Description>Total</Card.Description>
                  </Card.Header>
                  <Card.Content>
                    <div class="text-2xl font-bold">{formatCurrency(payeeTotalsTotal)}</div>
                  </Card.Content>
                </Card.Root>
              </div>

              <Table.Root>
                <Table.Header>
                  <Table.Row>
                    <Table.Head>Payee</Table.Head>
                    <Table.Head class="text-right">Amount</Table.Head>
                    <Table.Head class="text-right">% of Total</Table.Head>
                  </Table.Row>
                </Table.Header>
                <Table.Body>
                  {#each payeeTotalsData.sort((a, b) => Math.abs(b.total_minor) - Math.abs(a.total_minor)) as row}
                    <Table.Row>
                      <Table.Cell>{row.payee_name || "(No payee)"}</Table.Cell>
                      <Table.Cell class="text-right {row.total_minor < 0 ? 'text-red-600' : 'text-green-600'}">
                        {formatCurrency(row.total_minor)}
                      </Table.Cell>
                      <Table.Cell class="text-right text-muted-foreground">
                        {payeeTotalsTotal !== 0 ? ((Math.abs(row.total_minor) / Math.abs(payeeTotalsTotal)) * 100).toFixed(1) : 0}%
                      </Table.Cell>
                    </Table.Row>
                  {/each}
                </Table.Body>
              </Table.Root>
            {/if}
          </Card.Content>
        </Card.Root>
      </Tabs.Content>

      <!-- Investment Gains Tab -->
      <Tabs.Content value="gains">
        <div class="space-y-6">
          <Card.Root>
            <Card.Header>
              <Card.Title>Realized Gains/Losses</Card.Title>
            </Card.Header>
            <Card.Content>
              {#if realizedGains.length === 0}
                <p class="text-muted-foreground">No realized gains for selected period.</p>
              {:else}
                <div class="mb-6">
                  <Card.Root class={realizedGainsTotal >= 0 ? "border-green-200 bg-green-50 w-fit" : "border-red-200 bg-red-50 w-fit"}>
                    <Card.Header class="pb-2">
                      <Card.Description>Total Realized Gain/Loss</Card.Description>
                    </Card.Header>
                    <Card.Content>
                      <div class="text-2xl font-bold">{formatCurrency(realizedGainsTotal)}</div>
                    </Card.Content>
                  </Card.Root>
                </div>

                <Table.Root>
                  <Table.Header>
                    <Table.Row>
                      <Table.Head>Date</Table.Head>
                      <Table.Head class="text-right">Qty</Table.Head>
                      <Table.Head class="text-right">Proceeds</Table.Head>
                      <Table.Head class="text-right">Cost Basis</Table.Head>
                      <Table.Head class="text-right">Gain/Loss</Table.Head>
                    </Table.Row>
                  </Table.Header>
                  <Table.Body>
                    {#each realizedGains as row}
                      <Table.Row>
                        <Table.Cell>{formatDate(row.txn_date)}</Table.Cell>
                        <Table.Cell class="text-right">{(row.quantity_minor / 1000000).toFixed(4)}</Table.Cell>
                        <Table.Cell class="text-right">{formatCurrency(row.proceeds_minor)}</Table.Cell>
                        <Table.Cell class="text-right">{formatCurrency(row.cost_basis_minor)}</Table.Cell>
                        <Table.Cell class="text-right {row.gain_loss_minor >= 0 ? 'text-green-600' : 'text-red-600'}">
                          {formatCurrency(row.gain_loss_minor)}
                        </Table.Cell>
                      </Table.Row>
                    {/each}
                  </Table.Body>
                </Table.Root>
              {/if}
            </Card.Content>
          </Card.Root>

          <Card.Root>
            <Card.Header>
              <Card.Title>Unrealized Gains/Losses</Card.Title>
            </Card.Header>
            <Card.Content>
              {#if unrealizedGains.length === 0}
                <p class="text-muted-foreground">No unrealized gains data available.</p>
              {:else}
                <div class="mb-6">
                  <Card.Root class={unrealizedGainsTotal >= 0 ? "border-green-200 bg-green-50 w-fit" : "border-red-200 bg-red-50 w-fit"}>
                    <Card.Header class="pb-2">
                      <Card.Description>Total Unrealized Gain/Loss</Card.Description>
                    </Card.Header>
                    <Card.Content>
                      <div class="text-2xl font-bold">{formatCurrency(unrealizedGainsTotal)}</div>
                    </Card.Content>
                  </Card.Root>
                </div>

                <Table.Root>
                  <Table.Header>
                    <Table.Row>
                      <Table.Head>Account</Table.Head>
                      <Table.Head>Commodity</Table.Head>
                      <Table.Head class="text-right">Value</Table.Head>
                      <Table.Head class="text-right">Cost Basis</Table.Head>
                      <Table.Head class="text-right">Gain/Loss</Table.Head>
                    </Table.Row>
                  </Table.Header>
                  <Table.Body>
                    {#each unrealizedGains as row}
                      <Table.Row>
                        <Table.Cell>{row.account_name}</Table.Cell>
                        <Table.Cell>{row.commodity_name}</Table.Cell>
                        <Table.Cell class="text-right">{formatCurrency(row.value_minor)}</Table.Cell>
                        <Table.Cell class="text-right">{formatCurrency(row.cost_basis_minor)}</Table.Cell>
                        <Table.Cell class="text-right {row.unrealized_gain_minor >= 0 ? 'text-green-600' : 'text-red-600'}">
                          {formatCurrency(row.unrealized_gain_minor)}
                          {#if row.price_missing}
                            <span class="text-yellow-600" title="Price missing">⚠</span>
                          {/if}
                        </Table.Cell>
                      </Table.Row>
                    {/each}
                  </Table.Body>
                </Table.Root>
              {/if}
            </Card.Content>
          </Card.Root>
        </div>
      </Tabs.Content>
    </Tabs.Root>
  </div>
</main>
