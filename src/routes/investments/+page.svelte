<script lang="ts">
  import { onMount } from "svelte";
  import { invoke } from "@tauri-apps/api/core";

  // Tab state
  let activeTab: "holdings" | "transactions" | "lots" = "holdings";

  // Loading state
  let busy = false;
  let error = "";
  let status = "";

  // Holdings data
  type PositionLot = {
    lot_id: number;
    opened_date: string | null;
    quantity_minor: number;
    cost_basis_minor: number;
    remaining_cost_basis_minor: number;
    converted_value_minor: number | null;
    converted_cost_basis_minor: number | null;
    price_missing: boolean | null;
  };

  type Position = {
    account_id: number;
    account_name: string;
    account_type: string;
    commodity_id: number;
    commodity_name: string;
    commodity_scale: number;
    balance_minor: number;
    lots: PositionLot[];
  };

  type ConvertedPosition = Position & {
    value_minor: number;
    price_missing: boolean;
  };

  type LotWithHoldingPeriod = {
    lot_id: number;
    account_id: number;
    account_name: string;
    commodity_id: number;
    commodity_name: string;
    opened_date: string | null;
    quantity_minor: number;
    cost_basis_minor: number;
    commodity_scale: number;
    holding_days: number | null;
    is_long_term: boolean;
  };

  type Account = {
    id: number;
    name: string;
    type: string;
    commodity_id: number;
  };

  type Commodity = {
    id: number;
    kind: string;
    symbol: string | null;
    name: string;
    scale: number;
  };

  let positions: Position[] = [];
  let convertedPositions: ConvertedPosition[] = [];
  let lotsWithHolding: LotWithHoldingPeriod[] = [];
  let accounts: Account[] = [];
  let commodities: Commodity[] = [];
  let quoteCommodityId: number | null = null;
  let asOfDate: string = "";

  // Totals
  let totalValue = 0;
  let totalCostBasis = 0;
  let totalGainLoss = 0;

  // Buy/Sell dialog
  let showTradeDialog = false;
  let tradeType: "buy" | "sell" = "buy";
  let tradeForm = {
    txn_date: "",
    investment_account_id: null as number | null,
    cash_account_id: null as number | null,
    commodity_id: null as number | null,
    quantity: "",
    cash_amount: "",
    memo: "",
  };

  // Dividend dialog
  let showDividendDialog = false;
  let dividendForm = {
    txn_date: "",
    investment_account_id: null as number | null,
    cash_account_id: null as number | null,
    income_account_id: null as number | null,
    commodity_id: null as number | null,
    amount: "",
    memo: "",
  };

  onMount(async () => {
    asOfDate = new Date().toISOString().split("T")[0];
    await loadAccounts();
    await loadCommodities();
    await loadPositions();
  });

  async function loadAccounts() {
    try {
      accounts = await invoke<Account[]>("list_accounts", { bookId: 1, includeClosed: false });
    } catch (e) {
      error = `Failed to load accounts: ${String(e)}`;
    }
  }

  async function loadCommodities() {
    try {
      commodities = await invoke<Commodity[]>("list_commodities", { bookId: 1 });
      const currencies = commodities.filter((c) => c.kind === "currency");
      if (currencies.length > 0 && !quoteCommodityId) {
        quoteCommodityId = currencies[0].id;
      }
    } catch (e) {
      error = `Failed to load commodities: ${String(e)}`;
    }
  }

  async function loadPositions() {
    error = "";
    busy = true;
    try {
      positions = await invoke<Position[]>("get_positions", {
        bookId: 1,
        asOfDate: asOfDate || null,
      });

      // Convert positions to base currency if quote commodity is selected
      if (quoteCommodityId) {
        convertedPositions = await invoke<ConvertedPosition[]>("convert_positions", {
          bookId: 1,
          quoteCommodityId: quoteCommodityId,
          asOfDate: asOfDate || null,
        });

        // Calculate totals
        totalValue = convertedPositions.reduce((sum, p) => sum + p.value_minor, 0);
        totalCostBasis = convertedPositions.reduce((sum, p) => {
          return sum + p.lots.reduce((lotSum, lot) => lotSum + (lot.converted_cost_basis_minor || lot.remaining_cost_basis_minor), 0);
        }, 0);
        totalGainLoss = totalValue - totalCostBasis;
      }
    } catch (e) {
      error = `Failed to load positions: ${String(e)}`;
    } finally {
      busy = false;
    }
  }

  async function loadLots() {
    error = "";
    busy = true;
    try {
      lotsWithHolding = await invoke<LotWithHoldingPeriod[]>("list_lots_with_holding_period", {
        bookId: 1,
        asOfDate: asOfDate || null,
      });
    } catch (e) {
      error = `Failed to load lots: ${String(e)}`;
    } finally {
      busy = false;
    }
  }

  async function handleTabChange(tab: typeof activeTab) {
    activeTab = tab;
    if (tab === "lots" && lotsWithHolding.length === 0) {
      await loadLots();
    }
  }

  // Trade dialog functions
  function openBuyDialog() {
    tradeType = "buy";
    tradeForm = {
      txn_date: new Date().toISOString().split("T")[0],
      investment_account_id: null,
      cash_account_id: null,
      commodity_id: null,
      quantity: "",
      cash_amount: "",
      memo: "",
    };
    showTradeDialog = true;
  }

  function openSellDialog() {
    tradeType = "sell";
    tradeForm = {
      txn_date: new Date().toISOString().split("T")[0],
      investment_account_id: null,
      cash_account_id: null,
      commodity_id: null,
      quantity: "",
      cash_amount: "",
      memo: "",
    };
    showTradeDialog = true;
  }

  function closeTradeDialog() {
    showTradeDialog = false;
  }

  async function submitTrade() {
    if (!tradeForm.investment_account_id || !tradeForm.cash_account_id || !tradeForm.commodity_id) {
      error = "Please fill in all required fields";
      return;
    }

    const commodity = commodities.find((c) => c.id === tradeForm.commodity_id);
    const quantityMinor = Math.round(parseFloat(tradeForm.quantity) * (commodity?.scale || 1000000));
    const cashAmountMinor = Math.round(parseFloat(tradeForm.cash_amount) * 100);

    error = "";
    status = "";
    busy = true;

    try {
      if (tradeType === "buy") {
        await invoke("buy_commodity", {
          input: {
            txn_date: tradeForm.txn_date,
            investment_account_id: tradeForm.investment_account_id,
            cash_account_id: tradeForm.cash_account_id,
            commodity_id: tradeForm.commodity_id,
            quantity_minor: quantityMinor,
            cash_amount_minor: cashAmountMinor,
            memo: tradeForm.memo || null,
            payee_id: null,
            status: null,
          },
        });
        status = "Buy transaction created successfully";
      } else {
        await invoke("sell_commodity", {
          input: {
            txn_date: tradeForm.txn_date,
            investment_account_id: tradeForm.investment_account_id,
            cash_account_id: tradeForm.cash_account_id,
            commodity_id: tradeForm.commodity_id,
            quantity_minor: quantityMinor,
            proceeds_minor: cashAmountMinor,
            lot_allocation_method: "FIFO",
            lot_allocations: null,
            memo: tradeForm.memo || null,
            payee_id: null,
            status: null,
          },
        });
        status = "Sell transaction created successfully";
      }

      closeTradeDialog();
      await loadPositions();
      if (activeTab === "lots") {
        await loadLots();
      }
    } catch (e) {
      error = `Failed to create ${tradeType} transaction: ${String(e)}`;
    } finally {
      busy = false;
    }
  }

  // Dividend dialog functions
  function openDividendDialog() {
    dividendForm = {
      txn_date: new Date().toISOString().split("T")[0],
      investment_account_id: null,
      cash_account_id: null,
      income_account_id: null,
      commodity_id: null,
      amount: "",
      memo: "",
    };
    showDividendDialog = true;
  }

  function closeDividendDialog() {
    showDividendDialog = false;
  }

  async function submitDividend() {
    if (!dividendForm.investment_account_id || !dividendForm.cash_account_id || !dividendForm.income_account_id) {
      error = "Please fill in all required fields";
      return;
    }

    const amountMinor = Math.round(parseFloat(dividendForm.amount) * 100);

    error = "";
    status = "";
    busy = true;

    try {
      await invoke("create_dividend_transaction", {
        input: {
          txn_date: dividendForm.txn_date,
          investment_account_id: dividendForm.investment_account_id,
          cash_account_id: dividendForm.cash_account_id,
          income_account_id: dividendForm.income_account_id,
          amount_minor: amountMinor,
          commodity_id: dividendForm.commodity_id,
          memo: dividendForm.memo || null,
          payee_id: null,
          status: null,
        },
      });
      status = "Dividend transaction created successfully";
      closeDividendDialog();
      await loadPositions();
    } catch (e) {
      error = `Failed to create dividend transaction: ${String(e)}`;
    } finally {
      busy = false;
    }
  }

  function formatCurrency(minor: number, scale = 100) {
    return (minor / scale).toLocaleString(undefined, { minimumFractionDigits: 2, maximumFractionDigits: 2 });
  }

  function formatQuantity(minor: number, scale: number) {
    return (minor / scale).toLocaleString(undefined, { minimumFractionDigits: 2, maximumFractionDigits: 6 });
  }

  function formatDate(dateStr: string | null) {
    if (!dateStr) return "—";
    return new Date(dateStr).toLocaleDateString();
  }

  $: investmentAccounts = accounts.filter((a) => a.type === "investment" || a.type === "brokerage" || a.type === "stock");
  $: cashAccounts = accounts.filter((a) => a.type === "checking" || a.type === "savings" || a.type === "cash" || a.type === "asset");
  $: incomeAccounts = accounts.filter((a) => a.type === "income");
  $: securities = commodities.filter((c) => c.kind === "security" || c.kind === "stock" || c.kind === "mutual_fund");
  $: currencies = commodities.filter((c) => c.kind === "currency");
</script>

<main class="page">
  <div class="page-grid container">
    <div class="page-row">
      <div class="page-col">
        <h1 class="page-title">Investments</h1>
        <p class="page-subtitle">Track holdings and portfolio performance.</p>
      </div>
    </div>

    <!-- Actions & Filters -->
    <div class="page-row">
      <div class="page-col">
        <div class="actions-bar">
          <div class="actions-group">
            <button class="btn btn-primary" on:click={openBuyDialog} disabled={busy}>Buy</button>
            <button class="btn btn-secondary" on:click={openSellDialog} disabled={busy}>Sell</button>
            <button class="btn btn-secondary" on:click={openDividendDialog} disabled={busy}>Dividend</button>
          </div>
          <div class="filters-group">
            <label class="label-inline">
              <span>As of:</span>
              <input type="date" class="input input-sm" bind:value={asOfDate} />
            </label>
            <label class="label-inline">
              <span>Quote:</span>
              <select class="select select-sm" bind:value={quoteCommodityId}>
                {#each currencies as c}
                  <option value={c.id}>{c.symbol || c.name}</option>
                {/each}
              </select>
            </label>
            <button class="btn btn-ghost btn-sm" on:click={loadPositions} disabled={busy}>
              {busy ? "Loading..." : "Refresh"}
            </button>
          </div>
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

    {#if status}
      <div class="page-row">
        <div class="page-col">
          <p class="text-success">{status}</p>
        </div>
      </div>
    {/if}

    <!-- Summary Cards -->
    <div class="page-row">
      <div class="page-col">
        <div class="summary-cards">
          <div class="summary-card">
            <span class="summary-label">Total Value</span>
            <span class="summary-value">{formatCurrency(totalValue)}</span>
          </div>
          <div class="summary-card">
            <span class="summary-label">Total Cost Basis</span>
            <span class="summary-value">{formatCurrency(totalCostBasis)}</span>
          </div>
          <div class="summary-card" class:summary-card--positive={totalGainLoss >= 0} class:summary-card--negative={totalGainLoss < 0}>
            <span class="summary-label">Unrealized Gain/Loss</span>
            <span class="summary-value">{formatCurrency(totalGainLoss)}</span>
          </div>
        </div>
      </div>
    </div>

    <!-- Tabs -->
    <div class="page-row">
      <div class="page-col">
        <div class="tabs">
          <button class="tab" class:active={activeTab === "holdings"} on:click={() => handleTabChange("holdings")}>
            Holdings
          </button>
          <button class="tab" class:active={activeTab === "lots"} on:click={() => handleTabChange("lots")}>
            Tax Lots
          </button>
        </div>
      </div>
    </div>

    <!-- Holdings Tab -->
    {#if activeTab === "holdings"}
      <div class="page-row">
        <div class="page-col">
          <div class="card">
            {#if convertedPositions.length === 0 && positions.length === 0}
              <p class="text-muted">No holdings found. Use Buy to add securities.</p>
            {:else}
              <div class="holdings-table">
                <div class="holdings-table__header">
                  <span>Account</span>
                  <span>Security</span>
                  <span class="text-right">Quantity</span>
                  <span class="text-right">Value</span>
                  <span class="text-right">Cost Basis</span>
                  <span class="text-right">Gain/Loss</span>
                </div>
                {#each convertedPositions.length > 0 ? convertedPositions : positions as pos}
                  {@const costBasis = pos.lots.reduce((sum, lot) => sum + (lot.converted_cost_basis_minor || lot.remaining_cost_basis_minor), 0)}
                  {@const value = 'value_minor' in pos ? pos.value_minor : 0}
                  {@const gainLoss = value - costBasis}
                  <div class="holdings-table__row">
                    <span>{pos.account_name}</span>
                    <span>{pos.commodity_name}</span>
                    <span class="text-right">{formatQuantity(pos.balance_minor, pos.commodity_scale)}</span>
                    <span class="text-right">
                      {formatCurrency(value)}
                      {#if 'price_missing' in pos && pos.price_missing}
                        <span class="text-warning" title="Price missing">⚠</span>
                      {/if}
                    </span>
                    <span class="text-right">{formatCurrency(costBasis)}</span>
                    <span class="text-right" class:text-income={gainLoss >= 0} class:text-expense={gainLoss < 0}>
                      {formatCurrency(gainLoss)}
                    </span>
                  </div>
                {/each}
              </div>
            {/if}
          </div>
        </div>
      </div>
    {/if}

    <!-- Lots Tab -->
    {#if activeTab === "lots"}
      <div class="page-row">
        <div class="page-col">
          <div class="card">
            {#if lotsWithHolding.length === 0}
              <p class="text-muted">No tax lots found.</p>
            {:else}
              <div class="lots-table">
                <div class="lots-table__header">
                  <span>Account</span>
                  <span>Security</span>
                  <span>Opened</span>
                  <span class="text-right">Quantity</span>
                  <span class="text-right">Cost Basis</span>
                  <span class="text-right">Days Held</span>
                  <span>Term</span>
                </div>
                {#each lotsWithHolding as lot}
                  <div class="lots-table__row">
                    <span>{lot.account_name}</span>
                    <span>{lot.commodity_name}</span>
                    <span>{formatDate(lot.opened_date)}</span>
                    <span class="text-right">{formatQuantity(lot.quantity_minor, lot.commodity_scale)}</span>
                    <span class="text-right">{formatCurrency(lot.cost_basis_minor)}</span>
                    <span class="text-right">{lot.holding_days ?? "—"}</span>
                    <span>
                      <span class="badge" class:badge-long={lot.is_long_term} class:badge-short={!lot.is_long_term}>
                        {lot.is_long_term ? "Long" : "Short"}
                      </span>
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

<!-- Buy/Sell Dialog -->
{#if showTradeDialog}
  <button class="dialog-backdrop" type="button" aria-label="Close" on:click={closeTradeDialog}></button>
  <div class="dialog" role="dialog" aria-modal="true">
    <div class="dialog-header">
      <h2 class="section-title">{tradeType === "buy" ? "Buy Security" : "Sell Security"}</h2>
    </div>
    <div class="dialog-body">
      <label class="label">
        Date
        <input class="input" type="date" bind:value={tradeForm.txn_date} />
      </label>
      <label class="label">
        Investment Account
        <select class="select" bind:value={tradeForm.investment_account_id}>
          <option value={null}>Select account...</option>
          {#each investmentAccounts as a}
            <option value={a.id}>{a.name}</option>
          {/each}
        </select>
      </label>
      <label class="label">
        Cash Account
        <select class="select" bind:value={tradeForm.cash_account_id}>
          <option value={null}>Select account...</option>
          {#each cashAccounts as a}
            <option value={a.id}>{a.name}</option>
          {/each}
        </select>
      </label>
      <label class="label">
        Security
        <select class="select" bind:value={tradeForm.commodity_id}>
          <option value={null}>Select security...</option>
          {#each securities as c}
            <option value={c.id}>{c.symbol || c.name}</option>
          {/each}
        </select>
      </label>
      <label class="label">
        Quantity
        <input class="input" type="number" step="0.000001" bind:value={tradeForm.quantity} placeholder="Number of shares" />
      </label>
      <label class="label">
        {tradeType === "buy" ? "Total Cost" : "Proceeds"}
        <input class="input" type="number" step="0.01" bind:value={tradeForm.cash_amount} placeholder="Amount" />
      </label>
      <label class="label">
        Memo (optional)
        <input class="input" type="text" bind:value={tradeForm.memo} placeholder="Description" />
      </label>
    </div>
    <div class="dialog-footer">
      <button class="btn btn-secondary" type="button" on:click={closeTradeDialog}>Cancel</button>
      <button class="btn btn-primary" type="button" on:click={submitTrade} disabled={busy}>
        {tradeType === "buy" ? "Buy" : "Sell"}
      </button>
    </div>
  </div>
{/if}

<!-- Dividend Dialog -->
{#if showDividendDialog}
  <button class="dialog-backdrop" type="button" aria-label="Close" on:click={closeDividendDialog}></button>
  <div class="dialog" role="dialog" aria-modal="true">
    <div class="dialog-header">
      <h2 class="section-title">Record Dividend</h2>
    </div>
    <div class="dialog-body">
      <label class="label">
        Date
        <input class="input" type="date" bind:value={dividendForm.txn_date} />
      </label>
      <label class="label">
        Investment Account (source)
        <select class="select" bind:value={dividendForm.investment_account_id}>
          <option value={null}>Select account...</option>
          {#each investmentAccounts as a}
            <option value={a.id}>{a.name}</option>
          {/each}
        </select>
      </label>
      <label class="label">
        Cash Account (destination)
        <select class="select" bind:value={dividendForm.cash_account_id}>
          <option value={null}>Select account...</option>
          {#each cashAccounts as a}
            <option value={a.id}>{a.name}</option>
          {/each}
        </select>
      </label>
      <label class="label">
        Income Account
        <select class="select" bind:value={dividendForm.income_account_id}>
          <option value={null}>Select account...</option>
          {#each incomeAccounts as a}
            <option value={a.id}>{a.name}</option>
          {/each}
        </select>
      </label>
      <label class="label">
        Security (optional)
        <select class="select" bind:value={dividendForm.commodity_id}>
          <option value={null}>None</option>
          {#each securities as c}
            <option value={c.id}>{c.symbol || c.name}</option>
          {/each}
        </select>
      </label>
      <label class="label">
        Amount
        <input class="input" type="number" step="0.01" bind:value={dividendForm.amount} placeholder="Dividend amount" />
      </label>
      <label class="label">
        Memo (optional)
        <input class="input" type="text" bind:value={dividendForm.memo} placeholder="Description" />
      </label>
    </div>
    <div class="dialog-footer">
      <button class="btn btn-secondary" type="button" on:click={closeDividendDialog}>Cancel</button>
      <button class="btn btn-primary" type="button" on:click={submitDividend} disabled={busy}>
        Record Dividend
      </button>
    </div>
  </div>
{/if}

<style>
  .actions-bar {
    display: flex;
    justify-content: space-between;
    align-items: center;
    flex-wrap: wrap;
    gap: 1rem;
    padding: 1rem;
    background: var(--bg-muted);
    border-radius: 0.5rem;
  }

  .actions-group,
  .filters-group {
    display: flex;
    gap: 0.5rem;
    align-items: center;
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

  .summary-cards {
    display: flex;
    gap: 1rem;
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
  }

  .summary-value {
    font-size: 1.5rem;
    font-weight: 600;
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

  .holdings-table,
  .lots-table {
    display: flex;
    flex-direction: column;
  }

  .holdings-table__header,
  .holdings-table__row {
    display: grid;
    grid-template-columns: 1fr 1fr 100px 100px 100px 100px;
    gap: 1rem;
    padding: 0.75rem;
    align-items: center;
  }

  .lots-table__header,
  .lots-table__row {
    display: grid;
    grid-template-columns: 1fr 1fr 100px 100px 100px 80px 80px;
    gap: 1rem;
    padding: 0.75rem;
    align-items: center;
  }

  .holdings-table__header,
  .lots-table__header {
    font-weight: 600;
    background: var(--bg-muted);
    border-bottom: 1px solid var(--border);
  }

  .holdings-table__row,
  .lots-table__row {
    border-bottom: 1px solid var(--border-light, #eee);
  }

  .holdings-table__row:hover,
  .lots-table__row:hover {
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

  .text-warning {
    color: #d97706;
  }

  .badge {
    display: inline-block;
    padding: 0.125rem 0.5rem;
    border-radius: 0.25rem;
    font-size: 0.75rem;
    background: var(--bg-muted);
  }

  .badge-long {
    background: #dcfce7;
    color: #16a34a;
  }

  .badge-short {
    background: #fef3c7;
    color: #d97706;
  }
</style>
