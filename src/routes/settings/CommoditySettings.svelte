<script lang="ts">
  import { onDestroy, onMount } from "svelte";
  import { hasApiBaseUrl } from "$lib/api/client";
  import { requireActiveBookId } from "$lib/book-context";
  import {
    createFxRateDaily,
    createFxRateOfficial,
    deleteFxRateDaily,
    deleteFxRateOfficial,
    deleteFxRateSourceAssignment,
    getFxRefreshExecutionStatus,
    getFxRateSettings,
    listCurrencies,
    listFxRefreshRunHistory,
    listFxRateRefreshState,
    listFxRateSourceAssignments,
    listFxRateSources,
    listFxRatesDaily,
    listFxRatesOfficial,
    refreshFxRatesNow as refreshFxRatesNowCommand,
    renameCommoditySymbol,
    restartFxRateScheduler,
    saveCurrency as saveCurrencyCommand,
    saveFxRateSourceAssignment,
    setDefaultCurrency as setDefaultCurrencyCommand,
    setFxRateSettings,
    toggleCurrencyActive as toggleCurrencyActiveCommand,
  } from "$lib/api/commodities";
  import { listCommodities, type CommoditySummary } from "$lib/api/metadata";
  import * as Card from "$lib/components/ui/card";
  import { formatApiError } from "$lib/utils";
  import CommoditySection from "./commodity-settings/CommoditySection.svelte";
  import CurrencySection from "./commodity-settings/CurrencySection.svelte";
  import FxDailySection from "./commodity-settings/FxDailySection.svelte";
  import FxOfficialSection from "./commodity-settings/FxOfficialSection.svelte";
  import FxSettingsSection from "./commodity-settings/FxSettingsSection.svelte";
  import {
    createEmptyCurrencyForm,
    createFxAssignmentDraft,
    createFxDailyDraft,
    createFxOfficialDraft,
    type Commodity,
    type CommoditySettingsTab,
    type Currency,
    type CurrencyForm,
    type FxAssignmentForm,
    type FxDailyForm,
    type FxOfficialForm,
    type FxRateDaily,
    type FxRateOfficial,
    type FxRateRefreshState,
    type FxRateSettings,
    type FxRateSource,
    type FxRateSourceAssignment,
    type FxRefreshExecutionStatus,
    type FxRefreshRunSummary,
  } from "./commodity-settings/types";

  export let commodities: Commodity[] = [];
  export let busy = false;

  let commodityError = "";
  let commodityStatus = "";
  let editingCommodity: Commodity | null = null;
  let showCommodityDialog = false;

  let currencies: Currency[] = [];
  let currencyError = "";
  let currencyStatus = "";
  let showCurrencyDialog = false;
  let editingCurrency: Currency | null = null;
  let newCurrency: CurrencyForm = createEmptyCurrencyForm();

  $: sortedCurrencies = [...currencies].sort((a, b) => {
    if (a.is_active !== b.is_active) return a.is_active ? -1 : 1;
    if (a.is_default !== b.is_default) return a.is_default ? -1 : 1;
    return (a.symbol || "").localeCompare(b.symbol || "");
  });

  let fxRatesDaily: FxRateDaily[] = [];
  let fxRatesOfficial: FxRateOfficial[] = [];
  let fxRateSources: FxRateSource[] = [];
  let fxRateSettings: FxRateSettings | null = null;
  let fxRateAssignments: FxRateSourceAssignment[] = [];
  let fxRateRefreshState: FxRateRefreshState[] = [];
  let fxRefreshExecutionStatus: FxRefreshExecutionStatus | null = null;
  let fxRefreshRunHistory: FxRefreshRunSummary[] = [];
  let fxRateError = "";
  let fxRateStatus = "";
  const pricingAutomationManagedByServer = hasApiBaseUrl();
  let fxExecutionPollTimer: ReturnType<typeof setInterval> | null = null;
  let showFxDailyDialog = false;
  let showFxOfficialDialog = false;
  let showFxAssignmentDialog = false;
  let newFxDaily: FxDailyForm = createFxDailyDraft(currencies);
  let newFxOfficial: FxOfficialForm = createFxOfficialDraft(currencies, fxRateSources);
  let editingAssignment: FxRateSourceAssignment | null = null;
  let newFxAssignment: FxAssignmentForm = createFxAssignmentDraft(currencies, fxRateSources);
  let commoditiesSubTab: CommoditySettingsTab = "currencies";

  $: if (pricingAutomationManagedByServer && commoditiesSubTab === "fx-settings" && fxRefreshExecutionStatus?.is_running) {
    startFxExecutionPolling();
  } else {
    stopFxExecutionPolling();
  }

  onMount(() => {
    void initialize();
  });

  onDestroy(() => {
    stopFxExecutionPolling();
  });

  function getBookId(): number {
    return requireActiveBookId();
  }

  export async function initialize() {
    await Promise.all([loadCommodities(), loadCurrencies()]);
  }

  export async function loadCommodities() {
    try {
      const summaries = await listCommodities(getBookId());
      commodities = summaries.map((commodity: CommoditySummary) => ({
        id: commodity.id,
        book_id: commodity.book_id,
        kind: commodity.kind,
        symbol: commodity.symbol,
        name: commodity.name,
        scale: commodity.scale,
        metadata: commodity.metadata,
        created_at: commodity.created_at,
        updated_at: commodity.updated_at,
      }));
    } catch (error) {
      commodityError = `Failed to load commodities: ${formatApiError(error)}`;
    }
  }

  function openEditCommodity(commodity: Commodity) {
    editingCommodity = { ...commodity };
    showCommodityDialog = true;
  }

  function closeCommodityDialog() {
    showCommodityDialog = false;
    editingCommodity = null;
  }

  async function saveCommodity() {
    if (!editingCommodity) return;
    commodityError = "";
    commodityStatus = "";
    busy = true;
    try {
      await renameCommoditySymbol({
        id: editingCommodity.id,
        book_id: getBookId(),
        symbol: editingCommodity.symbol,
        name: editingCommodity.name,
        metadata: editingCommodity.metadata,
      });
      commodityStatus = "Commodity updated.";
      closeCommodityDialog();
      await loadCommodities();
    } catch (error) {
      commodityError = `Failed to save commodity: ${formatApiError(error)}`;
    } finally {
      busy = false;
    }
  }

  async function loadCurrencies() {
    try {
      currencies = await listCurrencies<Currency[]>(getBookId());
    } catch (error) {
      currencyError = `Failed to load currencies: ${formatApiError(error)}`;
    }
  }

  async function toggleCurrencyActive(currency: Currency) {
    currencyError = "";
    currencyStatus = "";
    busy = true;
    try {
      await toggleCurrencyActiveCommand(getBookId(), currency.id, !currency.is_active);
      currencyStatus = `${currency.symbol} ${currency.is_active ? "deactivated" : "activated"}.`;
      await loadCurrencies();
    } catch (error) {
      currencyError = `Failed to ${currency.is_active ? "deactivate" : "activate"} currency: ${formatApiError(error)}`;
    } finally {
      busy = false;
    }
  }

  async function setDefaultCurrency(currency: Currency) {
    currencyError = "";
    currencyStatus = "";
    try {
      await setDefaultCurrencyCommand(getBookId(), currency.id);
      currencyStatus = `${currency.symbol} set as default currency.`;
      await loadCurrencies();
    } catch (error) {
      currencyError = `Failed to set default currency: ${formatApiError(error)}`;
    }
  }

  function openNewCurrency() {
    editingCurrency = null;
    newCurrency = createEmptyCurrencyForm();
    showCurrencyDialog = true;
  }

  function openEditCurrency(currency: Currency) {
    editingCurrency = currency;
    newCurrency = {
      symbol: currency.symbol || "",
      display_symbol: currency.display_symbol || "",
      name: currency.name,
      scale: currency.scale,
    };
    showCurrencyDialog = true;
  }

  function closeCurrencyDialog() {
    showCurrencyDialog = false;
    editingCurrency = null;
  }

  async function saveCurrency() {
    currencyError = "";
    currencyStatus = "";
    busy = true;
    try {
      if (editingCurrency) {
        await saveCurrencyCommand(
          {
            id: editingCurrency.id,
            book_id: getBookId(),
            symbol: newCurrency.symbol,
            display_symbol: newCurrency.display_symbol || null,
            name: newCurrency.name,
            scale: newCurrency.scale,
          },
          true
        );
        currencyStatus = "Currency updated.";
      } else {
        await saveCurrencyCommand(
          {
            book_id: getBookId(),
            symbol: newCurrency.symbol,
            display_symbol: newCurrency.display_symbol || null,
            name: newCurrency.name,
            scale: newCurrency.scale,
          },
          false
        );
        currencyStatus = "Currency created.";
      }
      closeCurrencyDialog();
      await loadCurrencies();
    } catch (error) {
      currencyError = `Failed to ${editingCurrency ? "update" : "create"} currency: ${formatApiError(error)}`;
    } finally {
      busy = false;
    }
  }

  async function loadFxRatesDaily() {
    try {
      fxRatesDaily = await listFxRatesDaily<FxRateDaily[]>(getBookId(), 100);
    } catch (error) {
      fxRateError = `Failed to load daily FX rates: ${formatApiError(error)}`;
    }
  }

  async function loadFxRatesOfficial() {
    try {
      fxRatesOfficial = await listFxRatesOfficial<FxRateOfficial[]>(getBookId());
    } catch (error) {
      fxRateError = `Failed to load official FX rates: ${formatApiError(error)}`;
    }
  }

  async function loadFxRateSources() {
    try {
      fxRateSources = await listFxRateSources<FxRateSource[]>(getBookId());
    } catch (error) {
      fxRateError = `Failed to load FX rate sources: ${formatApiError(error)}`;
    }
  }

  async function loadFxRateSettings() {
    try {
      fxRateSettings = await getFxRateSettings<FxRateSettings | null>(getBookId());
    } catch (error) {
      fxRateError = `Failed to load FX settings: ${formatApiError(error)}`;
    }
  }

  async function saveFxRateSettings() {
    if (!fxRateSettings) return;
    fxRateError = "";
    fxRateStatus = "";
    busy = true;
    try {
      const normalizedDefaultSource = fxRateSettings.default_source_id ? Number(fxRateSettings.default_source_id) : null;
      fxRateSettings = await setFxRateSettings<FxRateSettings>({
        book_id: getBookId(),
        base_currency_id: Number(fxRateSettings.base_currency_id),
        default_source_id: normalizedDefaultSource,
        refresh_enabled: fxRateSettings.refresh_enabled,
        refresh_hour_utc: Number(fxRateSettings.refresh_hour_utc),
        refresh_minute_utc: Number(fxRateSettings.refresh_minute_utc),
        max_backfill_days: Number(fxRateSettings.max_backfill_days),
        weekend_policy: fxRateSettings.weekend_policy,
      });
      if (!pricingAutomationManagedByServer) {
        await restartFxRateScheduler();
      } else {
        await reloadFxExecutionData();
      }
      fxRateStatus = "FX settings saved.";
    } catch (error) {
      fxRateError = `Failed to save FX settings: ${formatApiError(error)}`;
    } finally {
      busy = false;
    }
  }

  async function loadFxRateAssignments() {
    try {
      fxRateAssignments = await listFxRateSourceAssignments<FxRateSourceAssignment[]>(getBookId());
    } catch (error) {
      fxRateError = `Failed to load FX source assignments: ${formatApiError(error)}`;
    }
  }

  function openNewFxAssignment() {
    newFxAssignment = createFxAssignmentDraft(currencies, fxRateSources);
    editingAssignment = null;
    showFxAssignmentDialog = true;
  }

  function openEditFxAssignment(assignment: FxRateSourceAssignment) {
    editingAssignment = assignment;
    newFxAssignment = {
      from_currency_id: assignment.from_currency_id,
      to_currency_id: assignment.to_currency_id,
      source_id: assignment.source_id,
      effective_from: assignment.effective_from,
      effective_to: assignment.effective_to || "",
    };
    showFxAssignmentDialog = true;
  }

  function closeFxAssignmentDialog() {
    showFxAssignmentDialog = false;
    editingAssignment = null;
  }

  async function saveFxAssignment() {
    fxRateError = "";
    fxRateStatus = "";
    busy = true;
    try {
      if (editingAssignment) {
        await saveFxRateSourceAssignment(
          {
            id: editingAssignment.id,
            book_id: getBookId(),
            from_currency_id: Number(newFxAssignment.from_currency_id),
            to_currency_id: Number(newFxAssignment.to_currency_id),
            source_id: Number(newFxAssignment.source_id),
            effective_from: newFxAssignment.effective_from,
            effective_to: newFxAssignment.effective_to || null,
          },
          true
        );
        fxRateStatus = "FX source assignment updated.";
      } else {
        await saveFxRateSourceAssignment(
          {
            book_id: getBookId(),
            from_currency_id: Number(newFxAssignment.from_currency_id),
            to_currency_id: Number(newFxAssignment.to_currency_id),
            source_id: Number(newFxAssignment.source_id),
            effective_from: newFxAssignment.effective_from,
            effective_to: newFxAssignment.effective_to || null,
          },
          false
        );
        fxRateStatus = "FX source assignment created.";
      }
      closeFxAssignmentDialog();
      await loadFxRateAssignments();
    } catch (error) {
      fxRateError = `Failed to save FX source assignment: ${formatApiError(error)}`;
    } finally {
      busy = false;
    }
  }

  async function deleteFxAssignment(assignment: FxRateSourceAssignment) {
    if (!confirm("Delete this FX source assignment?")) return;
    fxRateError = "";
    fxRateStatus = "";
    try {
      await deleteFxRateSourceAssignment(assignment.id);
      fxRateStatus = "FX source assignment deleted.";
      await loadFxRateAssignments();
    } catch (error) {
      fxRateError = `Failed to delete FX source assignment: ${formatApiError(error)}`;
    }
  }

  async function loadFxRefreshState() {
    try {
      fxRateRefreshState = await listFxRateRefreshState<FxRateRefreshState[]>(getBookId());
    } catch (error) {
      fxRateError = `Failed to load FX refresh status: ${formatApiError(error)}`;
    }
  }

  async function loadFxRefreshExecutionStatus() {
    if (!pricingAutomationManagedByServer) {
      fxRefreshExecutionStatus = null;
      return;
    }

    try {
      fxRefreshExecutionStatus = await getFxRefreshExecutionStatus<FxRefreshExecutionStatus>(getBookId());
    } catch (error) {
      fxRateError = `Failed to load FX execution status: ${formatApiError(error)}`;
    }
  }

  async function loadFxRefreshRunHistory() {
    if (!pricingAutomationManagedByServer) {
      fxRefreshRunHistory = [];
      return;
    }

    try {
      fxRefreshRunHistory = await listFxRefreshRunHistory<FxRefreshRunSummary[]>(getBookId(), 10);
    } catch (error) {
      fxRateError = `Failed to load FX refresh history: ${formatApiError(error)}`;
    }
  }

  async function reloadFxExecutionData() {
    await Promise.all([loadFxRefreshState(), loadFxRefreshExecutionStatus(), loadFxRefreshRunHistory()]);
  }

  function startFxExecutionPolling() {
    if (fxExecutionPollTimer !== null) {
      return;
    }

    fxExecutionPollTimer = setInterval(() => {
      void reloadFxExecutionData();
    }, 5000);
  }

  function stopFxExecutionPolling() {
    if (fxExecutionPollTimer === null) {
      return;
    }

    clearInterval(fxExecutionPollTimer);
    fxExecutionPollTimer = null;
  }

  async function refreshFxRatesNow() {
    fxRateError = "";
    fxRateStatus = "";
    busy = true;
    try {
      const summary = await refreshFxRatesNowCommand<FxRefreshRunSummary>(getBookId());
      fxRateStatus = `FX refresh complete. ${summary.pairs_success}/${summary.pairs_total} pairs updated, ${summary.rates_inserted} rates.`;
      if (summary.last_error) {
        fxRateError = `Last error: ${summary.last_error}`;
      }
      await reloadFxExecutionData();
    } catch (error) {
      fxRateError = `Failed to refresh FX rates: ${formatApiError(error)}`;
    } finally {
      busy = false;
    }
  }

  function openNewFxDaily() {
    newFxDaily = createFxDailyDraft(currencies);
    showFxDailyDialog = true;
  }

  function closeFxDailyDialog() {
    showFxDailyDialog = false;
  }

  async function saveFxDaily() {
    fxRateError = "";
    fxRateStatus = "";
    busy = true;
    try {
      await createFxRateDaily({
        book_id: getBookId(),
        from_currency_id: newFxDaily.from_currency_id,
        to_currency_id: newFxDaily.to_currency_id,
        rate_date: newFxDaily.rate_date,
        rate: newFxDaily.rate,
        source: newFxDaily.source || null,
      });
      fxRateStatus = "Daily FX rate added.";
      closeFxDailyDialog();
      await loadFxRatesDaily();
    } catch (error) {
      fxRateError = `Failed to add FX rate: ${formatApiError(error)}`;
    } finally {
      busy = false;
    }
  }

  async function deleteFxDaily(rate: FxRateDaily) {
    if (!confirm(`Delete FX rate ${rate.from_currency_symbol}/${rate.to_currency_symbol} on ${rate.rate_date}?`)) return;
    fxRateError = "";
    fxRateStatus = "";
    try {
      await deleteFxRateDaily(rate.id);
      fxRateStatus = "FX rate deleted.";
      await loadFxRatesDaily();
    } catch (error) {
      fxRateError = `Failed to delete FX rate: ${formatApiError(error)}`;
    }
  }

  function openNewFxOfficial() {
    newFxOfficial = createFxOfficialDraft(currencies, fxRateSources);
    showFxOfficialDialog = true;
  }

  function closeFxOfficialDialog() {
    showFxOfficialDialog = false;
  }

  async function saveFxOfficial() {
    fxRateError = "";
    fxRateStatus = "";
    busy = true;
    try {
      await createFxRateOfficial({
        book_id: getBookId(),
        from_currency_id: newFxOfficial.from_currency_id,
        to_currency_id: newFxOfficial.to_currency_id,
        period_type: newFxOfficial.period_type,
        period_year: newFxOfficial.period_year,
        period_month: newFxOfficial.period_type === "monthly" ? newFxOfficial.period_month : null,
        rate: newFxOfficial.rate,
        source_name: newFxOfficial.source_name,
        source_url: newFxOfficial.source_url || null,
        source_date: null,
        notes: newFxOfficial.notes || null,
      });
      fxRateStatus = "Official FX rate added.";
      closeFxOfficialDialog();
      await loadFxRatesOfficial();
    } catch (error) {
      fxRateError = `Failed to add official FX rate: ${formatApiError(error)}`;
    } finally {
      busy = false;
    }
  }

  async function deleteFxOfficial(rate: FxRateOfficial) {
    if (!confirm(`Delete official FX rate ${rate.from_currency_symbol}/${rate.to_currency_symbol} for ${rate.period_type === "monthly" ? `${rate.period_year}-${String(rate.period_month).padStart(2, "0")}` : String(rate.period_year)}?`)) return;
    fxRateError = "";
    fxRateStatus = "";
    try {
      await deleteFxRateOfficial(rate.id);
      fxRateStatus = "Official FX rate deleted.";
      await loadFxRatesOfficial();
    } catch (error) {
      fxRateError = `Failed to delete official FX rate: ${formatApiError(error)}`;
    }
  }

  async function activateTab(tab: CommoditySettingsTab) {
    commoditiesSubTab = tab;
    if (tab === "fx-daily") {
      await Promise.all([loadFxRateSources(), loadFxRatesDaily()]);
      return;
    }
    if (tab === "fx-official") {
      await Promise.all([loadFxRateSources(), loadFxRatesOfficial()]);
      return;
    }
    if (tab === "fx-settings") {
      await Promise.all([loadFxRateSources(), loadFxRateSettings(), loadFxRateAssignments(), reloadFxExecutionData()]);
    }
  }
</script>

<Card.Root>
  <Card.Header>
    <Card.Title>Currencies &amp; Commodities</Card.Title>
  </Card.Header>
  <Card.Content>
    <div class="flex gap-2 mb-4 border-b">
      <button
        class="px-4 py-2 text-sm font-medium {commoditiesSubTab === 'currencies' ? 'border-b-2 border-primary text-primary' : 'text-muted-foreground hover:text-foreground'}"
        onclick={() => void activateTab("currencies")}
      >
        Currencies
      </button>
      <button
        class="px-4 py-2 text-sm font-medium {commoditiesSubTab === 'commodities' ? 'border-b-2 border-primary text-primary' : 'text-muted-foreground hover:text-foreground'}"
        onclick={() => void activateTab("commodities")}
      >
        Other Commodities
      </button>
      <button
        class="px-4 py-2 text-sm font-medium {commoditiesSubTab === 'fx-daily' ? 'border-b-2 border-primary text-primary' : 'text-muted-foreground hover:text-foreground'}"
        onclick={() => void activateTab("fx-daily")}
      >
        Daily FX Rates
      </button>
      <button
        class="px-4 py-2 text-sm font-medium {commoditiesSubTab === 'fx-official' ? 'border-b-2 border-primary text-primary' : 'text-muted-foreground hover:text-foreground'}"
        onclick={() => void activateTab("fx-official")}
      >
        Official FX Rates
      </button>
      <button
        class="px-4 py-2 text-sm font-medium {commoditiesSubTab === 'fx-settings' ? 'border-b-2 border-primary text-primary' : 'text-muted-foreground hover:text-foreground'}"
        onclick={() => void activateTab("fx-settings")}
      >
        FX Settings
      </button>
    </div>

    {#if commoditiesSubTab === "currencies"}
      <CurrencySection
        {sortedCurrencies}
        {busy}
        {currencyStatus}
        {currencyError}
        bind:showCurrencyDialog
        {editingCurrency}
        bind:newCurrency
        onOpenNew={openNewCurrency}
        onOpenEdit={openEditCurrency}
        onToggleActive={toggleCurrencyActive}
        onSetDefault={setDefaultCurrency}
        onCloseDialog={closeCurrencyDialog}
        onSave={saveCurrency}
      />
    {/if}

    {#if commoditiesSubTab === "commodities"}
      <CommoditySection
        {commodities}
        {busy}
        {commodityStatus}
        {commodityError}
        bind:showCommodityDialog
        bind:editingCommodity
        onOpenEdit={openEditCommodity}
        onCloseDialog={closeCommodityDialog}
        onSave={saveCommodity}
      />
    {/if}

    {#if commoditiesSubTab === "fx-daily"}
      <FxDailySection
        {fxRatesDaily}
        {currencies}
        {busy}
        {fxRateStatus}
        {fxRateError}
        bind:showFxDailyDialog
        bind:newFxDaily
        onOpenNew={openNewFxDaily}
        onCloseDialog={closeFxDailyDialog}
        onSave={saveFxDaily}
        onDelete={deleteFxDaily}
      />
    {/if}

    {#if commoditiesSubTab === "fx-official"}
      <FxOfficialSection
        {fxRatesOfficial}
        {currencies}
        {fxRateSources}
        {busy}
        {fxRateStatus}
        {fxRateError}
        bind:showFxOfficialDialog
        bind:newFxOfficial
        onOpenNew={openNewFxOfficial}
        onCloseDialog={closeFxOfficialDialog}
        onSave={saveFxOfficial}
        onDelete={deleteFxOfficial}
      />
    {/if}

    {#if commoditiesSubTab === "fx-settings"}
      <FxSettingsSection
        {currencies}
        {fxRateSources}
        bind:fxRateSettings
        {fxRateAssignments}
        {fxRateRefreshState}
        {fxRefreshExecutionStatus}
        {fxRefreshRunHistory}
        {pricingAutomationManagedByServer}
        {busy}
        {fxRateStatus}
        {fxRateError}
        bind:showFxAssignmentDialog
        {editingAssignment}
        bind:newFxAssignment
        onRefreshNow={refreshFxRatesNow}
        onSaveSettings={saveFxRateSettings}
        onOpenNewAssignment={openNewFxAssignment}
        onOpenEditAssignment={openEditFxAssignment}
        onCloseAssignmentDialog={closeFxAssignmentDialog}
        onSaveAssignment={saveFxAssignment}
        onDeleteAssignment={deleteFxAssignment}
        onReload={reloadFxExecutionData}
      />
    {/if}
  </Card.Content>
</Card.Root>