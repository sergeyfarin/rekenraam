<script lang="ts">
  import { onMount } from "svelte";
  import { listCommodities, type CommoditySummary } from "$lib/api/metadata";
  import {
    realizedGainsReport,
    reportCashflow,
    reportCategorySpend,
    reportPayeeTotals,
    unrealizedGainsReport,
  } from "$lib/api/reports";
  import * as Tabs from "$lib/components/ui/tabs";
  import * as Alert from "$lib/components/ui/alert";
  import ReportFilters from "$lib/components/ReportFilters.svelte";
  import CashflowReport from "$lib/components/CashflowReport.svelte";
  import CategorySpendReport from "$lib/components/CategorySpendReport.svelte";
  import PayeeTotalsReport from "$lib/components/PayeeTotalsReport.svelte";
  import InvestmentGainsReport from "$lib/components/InvestmentGainsReport.svelte";

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
      const summaries = await listCommodities(1);
      commodities = summaries.map((commodity: CommoditySummary) => ({
        id: commodity.id,
        book_id: commodity.book_id,
        kind: commodity.kind,
        symbol: commodity.symbol,
        name: commodity.name,
        scale: commodity.scale,
      }));
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
      cashflowData = await reportCashflow<CashflowRow[]>({
        book_id: 1,
        date_from: dateFrom || null,
        date_to: dateTo || null,
        group_by: groupBy,
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
      categorySpendData = await reportCategorySpend<CategorySpendRow[]>({
        book_id: 1,
        date_from: dateFrom || null,
        date_to: dateTo || null,
        category_ids: null,
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
      payeeTotalsData = await reportPayeeTotals<PayeeTotalRow[]>({
        book_id: 1,
        date_from: dateFrom || null,
        date_to: dateTo || null,
        payee_ids: null,
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
      realizedGains = await realizedGainsReport<RealizedGainEntry[]>(dateFrom || null, dateTo || null);

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
      unrealizedGains = await unrealizedGainsReport<UnrealizedGainEntry[]>(selectedCommodityId, dateTo || null);

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

    <ReportFilters
      bind:dateFrom
      bind:dateTo
      bind:groupBy
      bind:selectedCommodityId
      showGroupBy={activeTab === "cashflow"}
      showQuoteCurrency={activeTab === "gains"}
      {commodities}
      {busy}
      onRefresh={refreshReport}
    />

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

      <Tabs.Content value="cashflow">
        <CashflowReport rows={cashflowData} totals={cashflowTotals} {groupBy} />
      </Tabs.Content>

      <Tabs.Content value="spending">
        <CategorySpendReport rows={categorySpendData} total={categorySpendTotal} />
      </Tabs.Content>

      <Tabs.Content value="payees">
        <PayeeTotalsReport rows={payeeTotalsData} total={payeeTotalsTotal} />
      </Tabs.Content>

      <Tabs.Content value="gains">
        <InvestmentGainsReport
          realized={realizedGains}
          realizedTotal={realizedGainsTotal}
          unrealized={unrealizedGains}
          unrealizedTotal={unrealizedGainsTotal}
        />
      </Tabs.Content>
    </Tabs.Root>
  </div>
</main>
