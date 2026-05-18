<script lang="ts">
  import { onMount } from "svelte";
  import { listCommodities, type CommoditySummary } from "$lib/api/metadata";
  import { requireActiveBookId } from "$lib/book-context";
  import { formatApiError } from "$lib/utils";
  import {
    accountValuationReport,
    currencyExposureReport,
    deleteReportDefinition,
    investmentPerformanceReport,
    listReportDefinitions,
    listReportRuns,
    realizedGainsReport,
    reportAccountTrends,
    reportCashflow,
    reportCategorySpend,
    reportNetWorth,
    reportPayeeTotals,
    saveReportDefinition,
    unrealizedGainsReport,
    type AccountTrendRow,
    type AccountValuationRow,
    type CashflowRow,
    type CategorySpendRow,
    type CurrencyExposureRow,
    type GroupBy,
    type NetWorthRow,
    type PayeeTotalRow,
    type PortfolioPerformanceSummary,
    type RealizedGainEntry,
    type ReportDefinitionSummary,
    type ReportRunSummary,
    type UnrealizedGainEntry,
  } from "$lib/api/reports";
  import * as Tabs from "$lib/components/ui/tabs";
  import * as Alert from "$lib/components/ui/alert";
  import ReportFilters from "$lib/components/ReportFilters.svelte";
  import CashflowReport from "$lib/components/CashflowReport.svelte";
  import CategorySpendReport from "$lib/components/CategorySpendReport.svelte";
  import PayeeTotalsReport from "$lib/components/PayeeTotalsReport.svelte";
  import InvestmentGainsReport from "$lib/components/InvestmentGainsReport.svelte";
  import AccountTrendsReport from "./AccountTrendsReport.svelte";
  import AccountValuationReport from "./AccountValuationReport.svelte";
  import CurrencyExposureReport from "./CurrencyExposureReport.svelte";
  import NetWorthReport from "./NetWorthReport.svelte";
  import PortfolioPerformanceReport from "./PortfolioPerformanceReport.svelte";
  import SavedReportsPanel from "./SavedReportsPanel.svelte";
  import { buildReportCsv, hasExportableReportData, saveCsv, type ReportsExportState } from "./export";
  import {
    createReportDefinitionForm,
    createTodayDate,
    createYearStartDate,
    type ReportDefinitionForm,
    type ReportsTab,
  } from "./types";

  let activeTab: ReportsTab = "cashflow";
  let dateFrom = "";
  let dateTo = "";
  let groupBy: GroupBy = "month";
  let busy = false;
  let error = "";
  let status = "";
  let commodities: CommoditySummary[] = [];
  let selectedCommodityId: number | null = null;

  let cashflowData: CashflowRow[] = [];
  let cashflowTotals = { inflow: 0, outflow: 0, net: 0 };
  let netWorthData: NetWorthRow[] = [];
  let accountTrendData: AccountTrendRow[] = [];
  let categorySpendData: CategorySpendRow[] = [];
  let categorySpendTotal = 0;
  let payeeTotalsData: PayeeTotalRow[] = [];
  let payeeTotalsTotal = 0;
  let realizedGains: RealizedGainEntry[] = [];
  let realizedGainsTotal = 0;
  let unrealizedGains: UnrealizedGainEntry[] = [];
  let unrealizedGainsTotal = 0;
  let performanceSummary: PortfolioPerformanceSummary | null = null;
  let accountValuationRows: AccountValuationRow[] = [];
  let currencyExposureRows: CurrencyExposureRow[] = [];
  let reportDefinitions: ReportDefinitionSummary[] = [];
  let reportRuns: ReportRunSummary[] = [];
  let showDefinitionDialog = false;
  let editingDefinition: ReportDefinitionSummary | null = null;
  let definitionForm: ReportDefinitionForm = createReportDefinitionForm();

  $: exportState = {
    activeTab,
    dateFrom,
    dateTo,
    cashflowData,
    netWorthData,
    accountTrendData,
    categorySpendData,
    payeeTotalsData,
    realizedGains,
    unrealizedGains,
    performanceSummary,
    accountValuationRows,
    currencyExposureRows,
    reportDefinitions,
    reportRuns,
  } satisfies ReportsExportState;
  $: exportEnabled = hasExportableReportData(exportState);

  onMount(() => {
    void initialize();
  });

  function getBookId(): number {
    return requireActiveBookId();
  }

  function getSelectedCommodity(): number | null {
    return selectedCommodityId;
  }

  function resetMessages() {
    error = "";
    status = "";
  }

  async function initialize() {
    const now = new Date();
    dateFrom = createYearStartDate(now);
    dateTo = createTodayDate(now);
    await loadCommodities();
    await loadActiveTab(true);
  }

  async function loadCommodities() {
    try {
      commodities = await listCommodities(getBookId());
      const currencies = commodities.filter((commodity) => commodity.kind === "currency");
      if (!getSelectedCommodity() && currencies.length > 0) {
        selectedCommodityId = currencies[0].id;
      }
    } catch (caught) {
      error = `Failed to load commodities: ${formatApiError(caught)}`;
    }
  }

  async function loadCashflow() {
    const rows = await reportCashflow({
      book_id: getBookId(),
      date_from: dateFrom || null,
      date_to: dateTo || null,
      group_by: groupBy,
    });
    cashflowData = rows;
    cashflowTotals = rows.reduce(
      (acc, row) => ({
        inflow: acc.inflow + row.inflow_minor,
        outflow: acc.outflow + row.outflow_minor,
        net: acc.net + row.net_minor,
      }),
      { inflow: 0, outflow: 0, net: 0 }
    );
  }

  async function loadNetWorth() {
    netWorthData = await reportNetWorth({
      book_id: getBookId(),
      date_from: dateFrom || null,
      date_to: dateTo || null,
      group_by: groupBy,
    });
  }

  async function loadAccountTrends() {
    accountTrendData = await reportAccountTrends({
      book_id: getBookId(),
      account_ids: null,
      date_from: dateFrom || null,
      date_to: dateTo || null,
      group_by: groupBy,
    });
  }

  async function loadCategorySpend() {
    categorySpendData = await reportCategorySpend({
      book_id: getBookId(),
      date_from: dateFrom || null,
      date_to: dateTo || null,
      category_ids: null,
    });
    categorySpendTotal = categorySpendData.reduce((sum, row) => sum + row.total_minor, 0);
  }

  async function loadPayeeTotals() {
    payeeTotalsData = await reportPayeeTotals({
      book_id: getBookId(),
      date_from: dateFrom || null,
      date_to: dateTo || null,
      payee_ids: null,
    });
    payeeTotalsTotal = payeeTotalsData.reduce((sum, row) => sum + row.total_minor, 0);
  }

  async function loadGains() {
    const selectedCommodity = getSelectedCommodity();
    realizedGains = await realizedGainsReport(getBookId(), dateFrom || null, dateTo || null);
    realizedGainsTotal = realizedGains.reduce((sum, row) => sum + row.gain_loss_minor, 0);
    unrealizedGains = selectedCommodity
      ? await unrealizedGainsReport(getBookId(), selectedCommodity, dateTo || null)
      : [];
    unrealizedGainsTotal = unrealizedGains.reduce((sum, row) => sum + row.unrealized_gain_minor, 0);
  }

  async function loadPerformance() {
    const selectedCommodity = getSelectedCommodity();
    performanceSummary = selectedCommodity
      ? await investmentPerformanceReport(getBookId(), selectedCommodity, dateTo || null)
      : null;
  }

  async function loadAccountValuation() {
    const selectedCommodity = getSelectedCommodity();
    accountValuationRows = selectedCommodity
      ? await accountValuationReport(getBookId(), selectedCommodity, dateTo || null)
      : [];
  }

  async function loadCurrencyExposure() {
    currencyExposureRows = await currencyExposureReport(getBookId(), dateTo || null);
  }

  async function loadSavedReports() {
    [reportDefinitions, reportRuns] = await Promise.all([
      listReportDefinitions(getBookId()),
      listReportRuns(getBookId()),
    ]);
  }

  async function loadActiveTab(force = false) {
    resetMessages();
    busy = true;
    try {
      if (activeTab === "cashflow") {
        if (!force && cashflowData.length > 0) return;
        await loadCashflow();
      } else if (activeTab === "net-worth") {
        if (!force && netWorthData.length > 0) return;
        await loadNetWorth();
      } else if (activeTab === "account-trends") {
        if (!force && accountTrendData.length > 0) return;
        await loadAccountTrends();
      } else if (activeTab === "spending") {
        if (!force && categorySpendData.length > 0) return;
        await loadCategorySpend();
      } else if (activeTab === "payees") {
        if (!force && payeeTotalsData.length > 0) return;
        await loadPayeeTotals();
      } else if (activeTab === "gains") {
        if (!force && (realizedGains.length > 0 || unrealizedGains.length > 0)) return;
        await loadGains();
      } else if (activeTab === "performance") {
        if (!force && performanceSummary) return;
        await loadPerformance();
      } else if (activeTab === "valuation") {
        if (!force && accountValuationRows.length > 0) return;
        await loadAccountValuation();
      } else if (activeTab === "exposure") {
        if (!force && currencyExposureRows.length > 0) return;
        await loadCurrencyExposure();
      } else if (activeTab === "saved") {
        if (!force && (reportDefinitions.length > 0 || reportRuns.length > 0)) return;
        await loadSavedReports();
      }
    } catch (caught) {
      error = `Failed to load ${activeTab.replace("-", " ")} report: ${formatApiError(caught)}`;
    } finally {
      busy = false;
    }
  }

  async function handleTabChange(tab: ReportsTab) {
    activeTab = tab;
    await loadActiveTab(false);
  }

  async function refreshReport() {
    await loadActiveTab(true);
  }

  function openNewDefinition() {
    editingDefinition = null;
    definitionForm = createReportDefinitionForm();
    showDefinitionDialog = true;
  }

  function openEditDefinition(definition: ReportDefinitionSummary) {
    editingDefinition = definition;
    definitionForm = createReportDefinitionForm(definition);
    showDefinitionDialog = true;
  }

  function closeDefinitionDialog() {
    showDefinitionDialog = false;
    editingDefinition = null;
    definitionForm = createReportDefinitionForm();
  }

  async function saveCurrentDefinition() {
    resetMessages();
    busy = true;
    try {
      const payload = {
        book_id: getBookId(),
        name: definitionForm.name.trim(),
        kind: definitionForm.kind,
        query_type: definitionForm.query_type,
        query_text: definitionForm.query_text.trim(),
        params_schema: definitionForm.params_schema.trim() || null,
      };
      if (editingDefinition) {
        await saveReportDefinition({ id: editingDefinition.id, ...payload });
        status = "Report definition updated.";
      } else {
        await saveReportDefinition(payload);
        status = "Report definition created.";
      }
      closeDefinitionDialog();
      activeTab = "saved";
      await loadSavedReports();
    } catch (caught) {
      error = `Failed to save report definition: ${formatApiError(caught)}`;
    } finally {
      busy = false;
    }
  }

  async function deleteDefinitionRecord(definition: ReportDefinitionSummary) {
    if (!confirm(`Delete report definition \"${definition.name}\"?`)) {
      return;
    }

    resetMessages();
    busy = true;
    try {
      await deleteReportDefinition(definition.id);
      status = "Report definition deleted.";
      await loadSavedReports();
    } catch (caught) {
      error = `Failed to delete report definition: ${formatApiError(caught)}`;
    } finally {
      busy = false;
    }
  }

  function exportActiveReport() {
    resetMessages();
    const csv = buildReportCsv(exportState);
    if (!csv || !exportEnabled) {
      error = "No report data available to export.";
      return;
    }
    saveCsv(csv.filename, csv.content);
    status = `${csv.filename} downloaded.`;
  }

  function printActiveReport() {
    if (!exportEnabled) {
      error = "No report data available to print.";
      return;
    }
    window.print();
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
      showGroupBy={activeTab === "cashflow" || activeTab === "net-worth" || activeTab === "account-trends"}
      showQuoteCurrency={activeTab === "gains" || activeTab === "performance" || activeTab === "valuation"}
      {commodities}
      {busy}
      onRefresh={refreshReport}
      onExport={exportActiveReport}
      onPrint={printActiveReport}
      exportDisabled={!exportEnabled}
      printDisabled={!exportEnabled}
    />

    {#if status}
      <Alert.Root>
        <Alert.Title>Status</Alert.Title>
        <Alert.Description>{status}</Alert.Description>
      </Alert.Root>
    {/if}

    {#if error}
      <Alert.Root variant="destructive">
        <Alert.Title>Error</Alert.Title>
        <Alert.Description>{error}</Alert.Description>
      </Alert.Root>
    {/if}

    <!-- Tabs -->
    <Tabs.Root value={activeTab} onValueChange={(v) => handleTabChange(v as ReportsTab)}>
      <Tabs.List>
        <Tabs.Trigger value="cashflow">Cash Flow</Tabs.Trigger>
        <Tabs.Trigger value="net-worth">Net Worth</Tabs.Trigger>
        <Tabs.Trigger value="account-trends">Account Trends</Tabs.Trigger>
        <Tabs.Trigger value="spending">Spending by Category</Tabs.Trigger>
        <Tabs.Trigger value="payees">Spending by Payee</Tabs.Trigger>
        <Tabs.Trigger value="gains">Investment Gains</Tabs.Trigger>
        <Tabs.Trigger value="performance">Performance</Tabs.Trigger>
        <Tabs.Trigger value="valuation">Account Valuation</Tabs.Trigger>
        <Tabs.Trigger value="exposure">Currency Exposure</Tabs.Trigger>
        <Tabs.Trigger value="saved">Saved Reports</Tabs.Trigger>
      </Tabs.List>

      <Tabs.Content value="cashflow">
        <CashflowReport rows={cashflowData} totals={cashflowTotals} {groupBy} />
      </Tabs.Content>

      <Tabs.Content value="net-worth">
        <NetWorthReport rows={netWorthData} {groupBy} />
      </Tabs.Content>

      <Tabs.Content value="account-trends">
        <AccountTrendsReport rows={accountTrendData} {groupBy} />
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

      <Tabs.Content value="performance">
        <PortfolioPerformanceReport summary={performanceSummary} />
      </Tabs.Content>

      <Tabs.Content value="valuation">
        <AccountValuationReport rows={accountValuationRows} />
      </Tabs.Content>

      <Tabs.Content value="exposure">
        <CurrencyExposureReport rows={currencyExposureRows} />
      </Tabs.Content>

      <Tabs.Content value="saved">
        <SavedReportsPanel
          definitions={reportDefinitions}
          runs={reportRuns}
          {busy}
          {status}
          {error}
          bind:showDefinitionDialog
          {editingDefinition}
          bind:definitionForm
          onOpenNew={openNewDefinition}
          onOpenEdit={openEditDefinition}
          onCloseDialog={closeDefinitionDialog}
          onSave={saveCurrentDefinition}
          onDelete={deleteDefinitionRecord}
        />
      </Tabs.Content>
    </Tabs.Root>
  </div>
</main>
