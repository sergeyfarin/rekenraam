<script lang="ts">
  import { onMount } from "svelte";
  import { invoke } from "@tauri-apps/api/core";

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
        quoteCommodityId: selectedCommodityId,
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

<main class="page">
  <div class="page-grid container">
    <div class="page-row">
      <div class="page-col">
        <h1 class="page-title">Reports</h1>
        <p class="page-subtitle">Generate financial reports and insights.</p>
      </div>
    </div>

    <!-- Filters -->
    <div class="page-row">
      <div class="page-col">
        <div class="filters-bar">
          <label class="label-inline">
            <span>From:</span>
            <input type="date" class="input input-sm" bind:value={dateFrom} />
          </label>
          <label class="label-inline">
            <span>To:</span>
            <input type="date" class="input input-sm" bind:value={dateTo} />
          </label>
          {#if activeTab === "cashflow"}
            <label class="label-inline">
              <span>Group by:</span>
              <select class="select select-sm" bind:value={groupBy}>
                <option value="day">Day</option>
                <option value="month">Month</option>
                <option value="quarter">Quarter</option>
                <option value="year">Year</option>
              </select>
            </label>
          {/if}
          {#if activeTab === "gains"}
            <label class="label-inline">
              <span>Quote currency:</span>
              <select class="select select-sm" bind:value={selectedCommodityId}>
                {#each commodities.filter((c) => c.kind === "currency") as c}
                  <option value={c.id}>{c.symbol || c.name}</option>
                {/each}
              </select>
            </label>
          {/if}
          <button class="btn btn-primary btn-sm" on:click={refreshReport} disabled={busy}>
            {busy ? "Loading..." : "Refresh"}
          </button>
        </div>
      </div>
    </div>

    {#if error}
      <div class="page-row">
        <div class="page-col">
          <p class="text-error">{error}</p>
        </div>
      </div>
    {/if}

    <!-- Tabs -->
    <div class="page-row">
      <div class="page-col">
        <div class="tabs">
          <button class="tab" class:active={activeTab === "cashflow"} on:click={() => handleTabChange("cashflow")}>
            Cash Flow
          </button>
          <button class="tab" class:active={activeTab === "spending"} on:click={() => handleTabChange("spending")}>
            Spending by Category
          </button>
          <button class="tab" class:active={activeTab === "payees"} on:click={() => handleTabChange("payees")}>
            Spending by Payee
          </button>
          <button class="tab" class:active={activeTab === "gains"} on:click={() => handleTabChange("gains")}>
            Investment Gains
          </button>
        </div>
      </div>
    </div>

    <!-- Cashflow Tab -->
    {#if activeTab === "cashflow"}
      <div class="page-row">
        <div class="page-col">
          <div class="card">
            <h2 class="section-title">Cash Flow Report</h2>

            {#if cashflowData.length === 0}
              <p class="text-muted">No data for selected period.</p>
            {:else}
              <div class="summary-cards">
                <div class="summary-card summary-card--income">
                  <span class="summary-label">Total Inflow</span>
                  <span class="summary-value">{formatCurrency(cashflowTotals.inflow)}</span>
                </div>
                <div class="summary-card summary-card--expense">
                  <span class="summary-label">Total Outflow</span>
                  <span class="summary-value">{formatCurrency(cashflowTotals.outflow)}</span>
                </div>
                <div class="summary-card" class:summary-card--positive={cashflowTotals.net >= 0} class:summary-card--negative={cashflowTotals.net < 0}>
                  <span class="summary-label">Net Cash Flow</span>
                  <span class="summary-value">{formatCurrency(cashflowTotals.net)}</span>
                </div>
              </div>

              <div class="report-table">
                <div class="report-table__header">
                  <span>Period</span>
                  <span class="text-right">Inflow</span>
                  <span class="text-right">Outflow</span>
                  <span class="text-right">Net</span>
                </div>
                {#each cashflowData as row}
                  <div class="report-table__row">
                    <span>{formatPeriod(row.period_start)}</span>
                    <span class="text-right text-income">{formatCurrency(row.inflow_minor)}</span>
                    <span class="text-right text-expense">{formatCurrency(row.outflow_minor)}</span>
                    <span class="text-right" class:text-income={row.net_minor >= 0} class:text-expense={row.net_minor < 0}>
                      {formatCurrency(row.net_minor)}
                    </span>
                  </div>
                {/each}
              </div>
            {/if}
          </div>
        </div>
      </div>
    {/if}

    <!-- Category Spend Tab -->
    {#if activeTab === "spending"}
      <div class="page-row">
        <div class="page-col">
          <div class="card">
            <h2 class="section-title">Spending by Category</h2>

            {#if categorySpendData.length === 0}
              <p class="text-muted">No spending data for selected period.</p>
            {:else}
              <div class="summary-cards">
                <div class="summary-card summary-card--expense">
                  <span class="summary-label">Total Spending</span>
                  <span class="summary-value">{formatCurrency(Math.abs(categorySpendTotal))}</span>
                </div>
              </div>

              <div class="report-table">
                <div class="report-table__header">
                  <span>Category</span>
                  <span class="text-right">Amount</span>
                  <span class="text-right">% of Total</span>
                </div>
                {#each categorySpendData.sort((a, b) => Math.abs(b.total_minor) - Math.abs(a.total_minor)) as row}
                  <div class="report-table__row">
                    <span>{row.category_name || "(No category)"}</span>
                    <span class="text-right" class:text-expense={row.total_minor < 0} class:text-income={row.total_minor > 0}>
                      {formatCurrency(row.total_minor)}
                    </span>
                    <span class="text-right text-muted">
                      {categorySpendTotal !== 0 ? ((Math.abs(row.total_minor) / Math.abs(categorySpendTotal)) * 100).toFixed(1) : 0}%
                    </span>
                  </div>
                {/each}
              </div>
            {/if}
          </div>
        </div>
      </div>
    {/if}

    <!-- Payee Totals Tab -->
    {#if activeTab === "payees"}
      <div class="page-row">
        <div class="page-col">
          <div class="card">
            <h2 class="section-title">Spending by Payee</h2>

            {#if payeeTotalsData.length === 0}
              <p class="text-muted">No payee data for selected period.</p>
            {:else}
              <div class="summary-cards">
                <div class="summary-card">
                  <span class="summary-label">Total</span>
                  <span class="summary-value">{formatCurrency(payeeTotalsTotal)}</span>
                </div>
              </div>

              <div class="report-table">
                <div class="report-table__header">
                  <span>Payee</span>
                  <span class="text-right">Amount</span>
                  <span class="text-right">% of Total</span>
                </div>
                {#each payeeTotalsData.sort((a, b) => Math.abs(b.total_minor) - Math.abs(a.total_minor)) as row}
                  <div class="report-table__row">
                    <span>{row.payee_name || "(No payee)"}</span>
                    <span class="text-right" class:text-expense={row.total_minor < 0} class:text-income={row.total_minor > 0}>
                      {formatCurrency(row.total_minor)}
                    </span>
                    <span class="text-right text-muted">
                      {payeeTotalsTotal !== 0 ? ((Math.abs(row.total_minor) / Math.abs(payeeTotalsTotal)) * 100).toFixed(1) : 0}%
                    </span>
                  </div>
                {/each}
              </div>
            {/if}
          </div>
        </div>
      </div>
    {/if}

    <!-- Investment Gains Tab -->
    {#if activeTab === "gains"}
      <div class="page-row">
        <div class="page-col">
          <div class="card">
            <h2 class="section-title">Realized Gains/Losses</h2>

            {#if realizedGains.length === 0}
              <p class="text-muted">No realized gains for selected period.</p>
            {:else}
              <div class="summary-cards">
                <div class="summary-card" class:summary-card--positive={realizedGainsTotal >= 0} class:summary-card--negative={realizedGainsTotal < 0}>
                  <span class="summary-label">Total Realized Gain/Loss</span>
                  <span class="summary-value">{formatCurrency(realizedGainsTotal)}</span>
                </div>
              </div>

              <div class="report-table">
                <div class="report-table__header report-table__header--gains">
                  <span>Date</span>
                  <span class="text-right">Qty</span>
                  <span class="text-right">Proceeds</span>
                  <span class="text-right">Cost Basis</span>
                  <span class="text-right">Gain/Loss</span>
                </div>
                {#each realizedGains as row}
                  <div class="report-table__row report-table__row--gains">
                    <span>{formatDate(row.txn_date)}</span>
                    <span class="text-right">{(row.quantity_minor / 1000000).toFixed(4)}</span>
                    <span class="text-right">{formatCurrency(row.proceeds_minor)}</span>
                    <span class="text-right">{formatCurrency(row.cost_basis_minor)}</span>
                    <span class="text-right" class:text-income={row.gain_loss_minor >= 0} class:text-expense={row.gain_loss_minor < 0}>
                      {formatCurrency(row.gain_loss_minor)}
                    </span>
                  </div>
                {/each}
              </div>
            {/if}
          </div>
        </div>
      </div>

      <div class="page-row">
        <div class="page-col">
          <div class="card">
            <h2 class="section-title">Unrealized Gains/Losses</h2>

            {#if unrealizedGains.length === 0}
              <p class="text-muted">No unrealized gains data available.</p>
            {:else}
              <div class="summary-cards">
                <div class="summary-card" class:summary-card--positive={unrealizedGainsTotal >= 0} class:summary-card--negative={unrealizedGainsTotal < 0}>
                  <span class="summary-label">Total Unrealized Gain/Loss</span>
                  <span class="summary-value">{formatCurrency(unrealizedGainsTotal)}</span>
                </div>
              </div>

              <div class="report-table">
                <div class="report-table__header report-table__header--unrealized">
                  <span>Account</span>
                  <span>Commodity</span>
                  <span class="text-right">Value</span>
                  <span class="text-right">Cost Basis</span>
                  <span class="text-right">Gain/Loss</span>
                </div>
                {#each unrealizedGains as row}
                  <div class="report-table__row report-table__row--unrealized">
                    <span>{row.account_name}</span>
                    <span>{row.commodity_name}</span>
                    <span class="text-right">{formatCurrency(row.value_minor)}</span>
                    <span class="text-right">{formatCurrency(row.cost_basis_minor)}</span>
                    <span class="text-right" class:text-income={row.unrealized_gain_minor >= 0} class:text-expense={row.unrealized_gain_minor < 0}>
                      {formatCurrency(row.unrealized_gain_minor)}
                      {#if row.price_missing}
                        <span class="text-warning" title="Price missing">⚠</span>
                      {/if}
                    </span>
                  </div>
                {/each}
              </div>
            {/if}
          </div>
        </div>
      </div>
    {/if}
  </div>
</main>

<style>
  .filters-bar {
    display: flex;
    gap: 1rem;
    align-items: center;
    flex-wrap: wrap;
    padding: 1rem;
    background: var(--bg-muted);
    border-radius: 0.5rem;
  }

  .label-inline {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    font-size: 0.875rem;
  }

  .input-sm,
  .select-sm {
    padding: 0.375rem 0.5rem;
    font-size: 0.875rem;
  }

  .btn-sm {
    padding: 0.375rem 0.75rem;
    font-size: 0.875rem;
  }

  .tabs {
    display: flex;
    gap: 0.5rem;
    border-bottom: 1px solid var(--border);
    margin-bottom: 1rem;
  }

  .tab {
    padding: 0.75rem 1rem;
    background: none;
    border: none;
    border-bottom: 2px solid transparent;
    cursor: pointer;
    font-size: 0.875rem;
    color: var(--text-muted);
  }

  .tab:hover {
    color: var(--text);
    background: var(--bg-hover);
  }

  .tab.active {
    color: var(--primary);
    border-bottom-color: var(--primary);
  }

  .summary-cards {
    display: flex;
    gap: 1rem;
    margin-bottom: 1.5rem;
    flex-wrap: wrap;
  }

  .summary-card {
    padding: 1rem 1.5rem;
    background: var(--bg-muted);
    border-radius: 0.5rem;
    display: flex;
    flex-direction: column;
    gap: 0.25rem;
    min-width: 150px;
  }

  .summary-card--income {
    background: #dcfce7;
  }

  .summary-card--expense {
    background: #fee2e2;
  }

  .summary-card--positive {
    background: #dcfce7;
  }

  .summary-card--negative {
    background: #fee2e2;
  }

  .summary-label {
    font-size: 0.75rem;
    color: var(--text-muted);
    text-transform: uppercase;
    letter-spacing: 0.05em;
  }

  .summary-value {
    font-size: 1.5rem;
    font-weight: 600;
  }

  .report-table {
    display: flex;
    flex-direction: column;
  }

  .report-table__header,
  .report-table__row {
    display: grid;
    grid-template-columns: 1fr 120px 120px 120px;
    gap: 1rem;
    padding: 0.75rem;
    align-items: center;
  }

  .report-table__header--gains,
  .report-table__row--gains {
    grid-template-columns: 100px 80px 120px 120px 120px;
  }

  .report-table__header--unrealized,
  .report-table__row--unrealized {
    grid-template-columns: 1fr 150px 120px 120px 120px;
  }

  .report-table__header {
    font-weight: 600;
    background: var(--bg-muted);
    border-bottom: 1px solid var(--border);
  }

  .report-table__row {
    border-bottom: 1px solid var(--border-light, #eee);
  }

  .report-table__row:hover {
    background: var(--bg-hover);
  }

  .text-right {
    text-align: right;
  }

  .text-income {
    color: #16a34a;
  }

  .text-expense {
    color: #dc2626;
  }

  .text-muted {
    color: var(--text-muted);
  }

  .text-warning {
    color: #d97706;
  }
</style>
