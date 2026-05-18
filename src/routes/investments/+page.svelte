<script lang="ts">
  import { onDestroy, onMount } from "svelte";
  import { autocompleteCommodities, type CommodityAutocompleteOption } from "$lib/api/commodities";
  import {
    buyCommodity,
    convertPositions,
    createDividendTransaction,
    getPositions,
    listLotsWithHoldingPeriod,
    sellCommodity,
    type ConvertedInvestmentPositionSummary,
    type InvestmentLotHoldingSummary,
    type InvestmentPositionSummary,
  } from "$lib/api/investments";
  import { listCommodities, type CommoditySummary } from "$lib/api/metadata";
  import { listAccounts, type AccountSummary } from "$lib/api/accounts";
  import { requireActiveBookId } from "$lib/book-context";
  import { Button } from "$lib/components/ui/button";
  import * as Card from "$lib/components/ui/card";
  import * as Tabs from "$lib/components/ui/tabs";
  import * as Alert from "$lib/components/ui/alert";
  import { Input } from "$lib/components/ui/input";
  import { Label } from "$lib/components/ui/label";
  import { formatApiError } from "$lib/utils";
  import DividendDialog from "./DividendDialog.svelte";
  import HoldingsTable from "./HoldingsTable.svelte";
  import LotsTable from "./LotsTable.svelte";
  import TradeDialog from "./TradeDialog.svelte";
  import {
    buildDividendPayload,
    buildTradePayload,
    calculatePositionTotals,
    createDividendFormState,
    createTodayDate,
    createTradeFormState,
    formatCurrency,
    getCashAccounts,
    getCommodityScale,
    getCurrencies,
    getIncomeAccounts,
    getInvestmentAccounts,
    getSecurities,
    getSelectedSecurity,
    type DividendFormState,
    type InvestmentsTab,
    type TradeFormState,
    type TradeType,
  } from "./forms";

  let activeTab: InvestmentsTab = "holdings";
  let busy = false;
  let error = "";
  let status = "";

  let positions: InvestmentPositionSummary[] = [];
  let convertedPositions: ConvertedInvestmentPositionSummary[] = [];
  let lotsWithHolding: InvestmentLotHoldingSummary[] = [];
  let accounts: AccountSummary[] = [];
  let commodities: CommoditySummary[] = [];
  let quoteCommodityId: number | string | null = null;
  let asOfDate = createTodayDate();

  let totalValue = 0;
  let totalCostBasis = 0;
  let totalGainLoss = 0;

  let showTradeDialog = false;
  let tradeType: TradeType = "buy";
  let tradeForm: TradeFormState = createTradeFormState(asOfDate);
  let tradeSecurityQuery = "";
  let tradeSecuritySuggestions: CommodityAutocompleteOption[] = [];
  let tradeSecuritySearching = false;
  let tradeSecurityDebounceTimer: ReturnType<typeof setTimeout> | null = null;

  let showDividendDialog = false;
  let dividendForm: DividendFormState = createDividendFormState(asOfDate);

  $: investmentAccounts = getInvestmentAccounts(accounts);
  $: cashAccounts = getCashAccounts(accounts);
  $: incomeAccounts = getIncomeAccounts(accounts);
  $: securities = getSecurities(commodities);
  $: currencies = getCurrencies(commodities);
  $: selectedTradeSecurity = getSelectedSecurity(securities, tradeForm.commodity_id);

  onMount(() => {
    void initialize();
  });

  onDestroy(() => {
    if (tradeSecurityDebounceTimer) {
      clearTimeout(tradeSecurityDebounceTimer);
    }
  });

  function getBookId(): number {
    return requireActiveBookId();
  }

  async function initialize() {
    asOfDate = createTodayDate();
    tradeForm = createTradeFormState(asOfDate);
    dividendForm = createDividendFormState(asOfDate);
    await Promise.all([loadAccounts(), loadCommodities()]);
    await loadPositions();
  }

  async function loadAccounts() {
    try {
      accounts = (await listAccounts(getBookId())).filter((account) => !account.is_closed);
    } catch (caught) {
      error = `Failed to load accounts: ${formatApiError(caught)}`;
    }
  }

  async function loadCommodities() {
    try {
      const loadedCommodities = await listCommodities(getBookId());
      commodities = loadedCommodities;
      const availableCurrencies = getCurrencies(loadedCommodities);
      if (availableCurrencies.length > 0 && !quoteCommodityId) {
        quoteCommodityId = availableCurrencies[0].id;
      }
    } catch (caught) {
      error = `Failed to load commodities: ${formatApiError(caught)}`;
    }
  }

  async function loadPositions() {
    error = "";
    busy = true;
    try {
      positions = await getPositions(getBookId(), asOfDate);
      const numericQuoteId = typeof quoteCommodityId === "string" ? Number(quoteCommodityId) : quoteCommodityId;

      if (numericQuoteId) {
        convertedPositions = await convertPositions(getBookId(), numericQuoteId, asOfDate);
        const totals = calculatePositionTotals(convertedPositions);
        totalValue = totals.totalValue;
        totalCostBasis = totals.totalCostBasis;
        totalGainLoss = totals.totalGainLoss;
      } else {
        convertedPositions = [];
        totalValue = 0;
        totalCostBasis = 0;
        totalGainLoss = 0;
      }
    } catch (caught) {
      error = `Failed to load positions: ${formatApiError(caught)}`;
    } finally {
      busy = false;
    }
  }

  async function loadLots() {
    error = "";
    busy = true;
    try {
      lotsWithHolding = await listLotsWithHoldingPeriod(getBookId(), null, null, asOfDate);
    } catch (caught) {
      error = `Failed to load lots: ${formatApiError(caught)}`;
    } finally {
      busy = false;
    }
  }

  async function handleTabChange(tab: string) {
    activeTab = tab as InvestmentsTab;
    if (activeTab === "lots" && lotsWithHolding.length === 0) {
      await loadLots();
    }
  }

  async function refreshActiveTab() {
    await loadPositions();
    if (activeTab === "lots") {
      await loadLots();
    }
  }

  function openBuyDialog() {
    tradeType = "buy";
    tradeForm = createTradeFormState(asOfDate || createTodayDate());
    tradeSecurityQuery = "";
    tradeSecuritySuggestions = [];
    showTradeDialog = true;
  }

  function openSellDialog() {
    tradeType = "sell";
    tradeForm = createTradeFormState(asOfDate || createTodayDate());
    tradeSecurityQuery = "";
    tradeSecuritySuggestions = [];
    showTradeDialog = true;
  }

  function closeTradeDialog() {
    if (tradeSecurityDebounceTimer) {
      clearTimeout(tradeSecurityDebounceTimer);
      tradeSecurityDebounceTimer = null;
    }
    tradeSecuritySuggestions = [];
    showTradeDialog = false;
  }

  async function loadTradeSecuritySuggestions(query: string) {
    if (!query.trim()) {
      tradeSecuritySuggestions = [];
      return;
    }

    const securityIds = new Set(securities.map((item) => item.id));
    tradeSecuritySearching = true;
    try {
      const options = await autocompleteCommodities({
        query,
        limit: 8,
        activeOnly: true,
        minQueryLength: 1,
      });
      tradeSecuritySuggestions = options.filter((item) => securityIds.has(item.id));
    } catch (caught) {
      error = `Failed to search securities: ${formatApiError(caught)}`;
      tradeSecuritySuggestions = [];
    } finally {
      tradeSecuritySearching = false;
    }
  }

  function onTradeSecurityInput(event: Event) {
    const target = event.currentTarget as HTMLInputElement;
    tradeSecurityQuery = target.value;

    if (tradeSecurityDebounceTimer) {
      clearTimeout(tradeSecurityDebounceTimer);
    }
    tradeSecurityDebounceTimer = setTimeout(() => {
      void loadTradeSecuritySuggestions(tradeSecurityQuery);
    }, 250);
  }

  function chooseTradeSecurity(option: CommodityAutocompleteOption) {
    tradeForm.commodity_id = option.id;
    tradeSecurityQuery = option.symbol ? `${option.symbol} — ${option.name}` : option.name;
    tradeSecuritySuggestions = [];
  }

  async function submitTrade() {
    error = "";
    status = "";
    busy = true;

    try {
      const commodityScale = getCommodityScale(commodities, tradeForm.commodity_id);
      if (tradeType === "buy") {
        const payload = buildTradePayload(getBookId(), "buy", tradeForm, commodityScale);
        await buyCommodity(payload);
        status = "Buy transaction created successfully";
      } else {
        const payload = buildTradePayload(getBookId(), "sell", tradeForm, commodityScale);
        await sellCommodity(payload);
        status = "Sell transaction created successfully";
      }

      closeTradeDialog();
      await refreshActiveTab();
    } catch (caught) {
      error = caught instanceof Error
        ? caught.message
        : `Failed to create ${tradeType} transaction: ${formatApiError(caught)}`;
    } finally {
      busy = false;
    }
  }

  function openDividendDialog() {
    dividendForm = createDividendFormState(asOfDate || createTodayDate());
    showDividendDialog = true;
  }

  function closeDividendDialog() {
    showDividendDialog = false;
  }

  async function submitDividend() {
    error = "";
    status = "";
    busy = true;
    try {
      await createDividendTransaction(buildDividendPayload(getBookId(), dividendForm));
      status = "Dividend transaction created successfully";
      closeDividendDialog();
      await refreshActiveTab();
    } catch (caught) {
      error = caught instanceof Error
        ? caught.message
        : `Failed to create dividend transaction: ${formatApiError(caught)}`;
    } finally {
      busy = false;
    }
  }
</script>

<main class="py-6">
  <div class="container mx-auto px-6 space-y-6">
    <div>
      <h1 class="text-3xl font-bold tracking-tight">Investments</h1>
      <p class="text-muted-foreground">Track holdings and portfolio performance.</p>
    </div>

    <Card.Root>
      <Card.Content class="pt-6">
        <div class="flex flex-wrap items-center justify-between gap-4">
          <div class="flex gap-2">
            <Button onclick={openBuyDialog} disabled={busy}>Buy</Button>
            <Button variant="secondary" onclick={openSellDialog} disabled={busy}>Sell</Button>
            <Button variant="secondary" onclick={openDividendDialog} disabled={busy}>Dividend</Button>
          </div>
          <div class="flex flex-wrap items-center gap-4">
            <div class="flex items-center gap-2">
              <Label for="as-of-date">As of:</Label>
              <Input id="as-of-date" type="date" bind:value={asOfDate} class="w-40" />
            </div>
            <div class="flex items-center gap-2">
              <Label for="quote-currency">Quote:</Label>
              <select id="quote-currency" class="flex h-9 rounded-md border border-input bg-transparent px-3 py-1 text-sm" bind:value={quoteCommodityId}>
                {#each currencies as currency}
                  <option value={currency.id}>{currency.symbol || currency.name}</option>
                {/each}
              </select>
            </div>
            <Button variant="outline" onclick={refreshActiveTab} disabled={busy}>
              {busy ? "Loading..." : "Refresh"}
            </Button>
          </div>
        </div>
      </Card.Content>
    </Card.Root>

    {#if error}
      <Alert.Root variant="destructive">
        <Alert.Title>Error</Alert.Title>
        <Alert.Description>{error}</Alert.Description>
      </Alert.Root>
    {/if}

    {#if status}
      <Alert.Root>
        <Alert.Title>Success</Alert.Title>
        <Alert.Description>{status}</Alert.Description>
      </Alert.Root>
    {/if}

    <div class="grid gap-4 md:grid-cols-3">
      <Card.Root>
        <Card.Header class="pb-2">
          <Card.Description>Total Value</Card.Description>
        </Card.Header>
        <Card.Content>
          <div class="text-2xl font-bold">{formatCurrency(totalValue)}</div>
        </Card.Content>
      </Card.Root>
      <Card.Root>
        <Card.Header class="pb-2">
          <Card.Description>Total Cost Basis</Card.Description>
        </Card.Header>
        <Card.Content>
          <div class="text-2xl font-bold">{formatCurrency(totalCostBasis)}</div>
        </Card.Content>
      </Card.Root>
      <Card.Root class={totalGainLoss >= 0 ? "surface-money-positive" : "surface-money-negative"}>
        <Card.Header class="pb-2">
          <Card.Description>Unrealized Gain/Loss</Card.Description>
        </Card.Header>
        <Card.Content>
          <div class="text-2xl font-bold">{formatCurrency(totalGainLoss)}</div>
        </Card.Content>
      </Card.Root>
    </div>

    <Tabs.Root value={activeTab} onValueChange={handleTabChange}>
      <Tabs.List>
        <Tabs.Trigger value="holdings">Holdings</Tabs.Trigger>
        <Tabs.Trigger value="lots">Tax Lots</Tabs.Trigger>
      </Tabs.List>

      <Tabs.Content value="holdings">
        <HoldingsTable positions={convertedPositions.length > 0 ? convertedPositions : positions} />
      </Tabs.Content>

      <Tabs.Content value="lots">
        <LotsTable lots={lotsWithHolding} />
      </Tabs.Content>
    </Tabs.Root>
  </div>
</main>

<TradeDialog
  bind:open={showTradeDialog}
  {tradeType}
  bind:form={tradeForm}
  {investmentAccounts}
  {cashAccounts}
  {securities}
  selectedSecurity={selectedTradeSecurity}
  bind:searchQuery={tradeSecurityQuery}
  searchSuggestions={tradeSecuritySuggestions}
  searching={tradeSecuritySearching}
  {busy}
  onClose={closeTradeDialog}
  onSearchInput={onTradeSecurityInput}
  onChooseSecurity={chooseTradeSecurity}
  onSubmit={submitTrade}
/>

<DividendDialog
  bind:open={showDividendDialog}
  bind:form={dividendForm}
  {cashAccounts}
  {incomeAccounts}
  {busy}
  onClose={closeDividendDialog}
  onSubmit={submitDividend}
/>