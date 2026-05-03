<script lang="ts">
  import { onMount } from "svelte";
  import { hasApiBaseUrl } from "$lib/api/client";
  import {
    createFxRateDaily,
    createFxRateOfficial,
    deleteFxRateDaily,
    deleteFxRateOfficial,
    deleteFxRateSourceAssignment,
    getFxRefreshExecutionStatus,
    getFxRateSettings,
    listCurrencies,
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
  import { Button } from "$lib/components/ui/button";
  import { Input } from "$lib/components/ui/input";
  import { Label } from "$lib/components/ui/label";
  import * as Dialog from "$lib/components/ui/dialog";
  import * as Table from "$lib/components/ui/table";
  import { Badge } from "$lib/components/ui/badge";

  type Commodity = {
    id: number;
    book_id: number;
    kind: string;
    symbol: string | null;
    name: string;
    scale: number;
    metadata: string | null;
    created_at: string;
    updated_at: string;
  };

  type Currency = {
    id: number;
    book_id: number;
    symbol: string | null;
    display_symbol: string | null;
    name: string;
    scale: number;
    is_active: boolean;
    is_default: boolean;
    created_at: string;
    updated_at: string;
  };

  type FxRateDaily = {
    id: number;
    book_id: number;
    from_currency_id: number;
    from_currency_symbol: string | null;
    to_currency_id: number;
    to_currency_symbol: string | null;
    rate_date: string;
    rate: number;
    source: string | null;
    source_id: number | null;
    is_derived: boolean;
    derived_via_currency_id: number | null;
    created_at: string;
  };

  type FxRateOfficial = {
    id: number;
    book_id: number;
    from_currency_id: number;
    from_currency_symbol: string | null;
    to_currency_id: number;
    to_currency_symbol: string | null;
    period_type: string;
    period_year: number;
    period_month: number | null;
    rate: number;
    source_name: string;
    source_url: string | null;
    source_date: string | null;
    notes: string | null;
    created_at: string;
    updated_at: string;
  };

  type FxRateSource = {
    id: number;
    book_id: number;
    name: string;
    country_code: string | null;
    website_url: string | null;
    notes: string | null;
    created_at: string;
    updated_at: string;
  };

  type FxRateSettings = {
    book_id: number;
    base_currency_id: number;
    base_currency_symbol: string | null;
    default_source_id: number | null;
    default_source_name: string | null;
    refresh_enabled: boolean;
    refresh_hour_utc: number;
    refresh_minute_utc: number;
    max_backfill_days: number;
    weekend_policy: string;
    created_at: string;
    updated_at: string;
  };

  type FxRateSourceAssignment = {
    id: number;
    book_id: number;
    from_currency_id: number;
    from_currency_symbol: string | null;
    to_currency_id: number;
    to_currency_symbol: string | null;
    source_id: number;
    source_name: string | null;
    effective_from: string;
    effective_to: string | null;
    created_at: string;
    updated_at: string;
  };

  type FxRateRefreshState = {
    id: number;
    book_id: number;
    from_currency_id: number;
    from_currency_symbol: string | null;
    to_currency_id: number;
    to_currency_symbol: string | null;
    source_id: number;
    source_name: string | null;
    last_success_date: string | null;
    last_attempt_at: string | null;
    last_error: string | null;
    created_at: string;
    updated_at: string;
  };

  type FxRefreshRunSummary = {
    book_id: number;
    trigger: string;
    started_at: string;
    finished_at: string;
    pairs_total: number;
    pairs_success: number;
    pairs_failed: number;
    rates_inserted: number;
    derived_inserted: number;
    last_error: string | null;
  };

  type FxRefreshExecutionStatus = {
    book_id: number;
    scheduler_enabled: boolean;
    scheduler_poll_seconds: number;
    worker_started_at: string | null;
    is_running: boolean;
    active_book_ids: number[];
    next_scheduled_at: string | null;
    last_run: FxRefreshRunSummary | null;
  };

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
  let newCurrency = { symbol: "", display_symbol: "", name: "", scale: 2 };

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
  let fxRateError = "";
  let fxRateStatus = "";
  const pricingAutomationManagedByServer = hasApiBaseUrl();
  let showFxDailyDialog = false;
  let showFxOfficialDialog = false;
  let showFxAssignmentDialog = false;
  let newFxDaily = { from_currency_id: 0, to_currency_id: 0, rate_date: "", rate: 0, source: "" };
  let newFxOfficial = { from_currency_id: 0, to_currency_id: 0, period_type: "yearly", period_year: new Date().getFullYear(), period_month: null as number | null, rate: 0, source_name: "", source_url: "", notes: "" };
  let editingAssignment: FxRateSourceAssignment | null = null;
  let newFxAssignment = {
    from_currency_id: 0,
    to_currency_id: 0,
    source_id: 0,
    effective_from: "",
    effective_to: "",
  };

  // Sub-tabs
  let commoditiesSubTab: "currencies" | "commodities" | "fx-daily" | "fx-official" | "fx-settings" = "currencies";

  onMount(() => {
    initialize();
  });

  export async function initialize() {
    await loadCommodities();
    await loadCurrencies();
  }

  export async function loadCommodities() {
    try {
      const summaries = await listCommodities(1);
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
    } catch (e) {
      commodityError = `Failed to load commodities: ${String(e)}`;
    }
  }

  function openEditCommodity(c: Commodity) {
    editingCommodity = c;
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
        book_id: 1,
        symbol: editingCommodity.symbol,
        name: editingCommodity.name,
        metadata: editingCommodity.metadata,
      });
      commodityStatus = "Commodity updated.";
      closeCommodityDialog();
      await loadCommodities();
    } catch (e) {
      commodityError = `Failed to save commodity: ${String(e)}`;
    } finally {
      busy = false;
    }
  }

  // --- Currency management functions ---
  async function loadCurrencies() {
    try {
      currencies = await listCurrencies<Currency[]>(1);
    } catch (e) {
      currencyError = `Failed to load currencies: ${String(e)}`;
    }
  }

  async function toggleCurrencyActive(currency: Currency) {
    currencyError = "";
    currencyStatus = "";
    busy = true;
    try {
      await toggleCurrencyActiveCommand(1, currency.id, !currency.is_active);
      currencyStatus = `${currency.symbol} ${currency.is_active ? 'deactivated' : 'activated'}.`;
      await loadCurrencies();
    } catch (e) {
      currencyError = `Failed to ${currency.is_active ? 'deactivate' : 'activate'} currency: ${String(e)}`;
    } finally {
      busy = false;
    }
  }

  async function setDefaultCurrency(currency: Currency) {
    currencyError = "";
    currencyStatus = "";
    try {
      await setDefaultCurrencyCommand(1, currency.id);
      currencyStatus = `${currency.symbol} set as default currency.`;
      await loadCurrencies();
    } catch (e) {
      currencyError = `Failed to set default currency: ${String(e)}`;
    }
  }

  function openNewCurrency() {
    editingCurrency = null;
    newCurrency = { symbol: "", display_symbol: "", name: "", scale: 2 };
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
        await saveCurrencyCommand({
          id: editingCurrency.id,
          book_id: 1,
          symbol: newCurrency.symbol,
          display_symbol: newCurrency.display_symbol || null,
          name: newCurrency.name,
          scale: newCurrency.scale,
        }, true);
        currencyStatus = "Currency updated.";
      } else {
        await saveCurrencyCommand({
          book_id: 1,
          symbol: newCurrency.symbol,
          display_symbol: newCurrency.display_symbol || null,
          name: newCurrency.name,
          scale: newCurrency.scale,
        }, false);
        currencyStatus = "Currency created.";
      }
      closeCurrencyDialog();
      await loadCurrencies();
    } catch (e) {
      currencyError = `Failed to ${editingCurrency ? 'update' : 'create'} currency: ${String(e)}`;
    } finally {
      busy = false;
    }
  }

  // --- FX Rate functions ---
  async function loadFxRatesDaily() {
    try {
      fxRatesDaily = await listFxRatesDaily<FxRateDaily[]>(1, 100);
    } catch (e) {
      fxRateError = `Failed to load daily FX rates: ${String(e)}`;
    }
  }

  async function loadFxRatesOfficial() {
    try {
      fxRatesOfficial = await listFxRatesOfficial<FxRateOfficial[]>(1);
    } catch (e) {
      fxRateError = `Failed to load official FX rates: ${String(e)}`;
    }
  }

  async function loadFxRateSources() {
    try {
      fxRateSources = await listFxRateSources<FxRateSource[]>(1);
    } catch (e) {
      fxRateError = `Failed to load FX rate sources: ${String(e)}`;
    }
  }

  async function loadFxRateSettings() {
    try {
      fxRateSettings = await getFxRateSettings<FxRateSettings | null>(1);
    } catch (e) {
      fxRateError = `Failed to load FX settings: ${String(e)}`;
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
        book_id: 1,
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
        await loadFxRefreshExecutionStatus();
      }
      fxRateStatus = "FX settings saved.";
    } catch (e) {
      fxRateError = `Failed to save FX settings: ${String(e)}`;
    } finally {
      busy = false;
    }
  }

  async function loadFxRateAssignments() {
    try {
      fxRateAssignments = await listFxRateSourceAssignments<FxRateSourceAssignment[]>(1);
    } catch (e) {
      fxRateError = `Failed to load FX source assignments: ${String(e)}`;
    }
  }

  function openNewFxAssignment() {
    const activeCurrencies = currencies.filter(c => c.is_active);
    const defaultCurrency = currencies.find(c => c.is_default);
    newFxAssignment = {
      from_currency_id: activeCurrencies[0]?.id || 0,
      to_currency_id: defaultCurrency?.id || 0,
      source_id: fxRateSources[0]?.id || 0,
      effective_from: new Date().toISOString().split("T")[0],
      effective_to: "",
    };
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
        await saveFxRateSourceAssignment({
          id: editingAssignment.id,
          book_id: 1,
          from_currency_id: Number(newFxAssignment.from_currency_id),
          to_currency_id: Number(newFxAssignment.to_currency_id),
          source_id: Number(newFxAssignment.source_id),
          effective_from: newFxAssignment.effective_from,
          effective_to: newFxAssignment.effective_to || null,
        }, true);
        fxRateStatus = "FX source assignment updated.";
      } else {
        await saveFxRateSourceAssignment({
          book_id: 1,
          from_currency_id: Number(newFxAssignment.from_currency_id),
          to_currency_id: Number(newFxAssignment.to_currency_id),
          source_id: Number(newFxAssignment.source_id),
          effective_from: newFxAssignment.effective_from,
          effective_to: newFxAssignment.effective_to || null,
        }, false);
        fxRateStatus = "FX source assignment created.";
      }
      closeFxAssignmentDialog();
      await loadFxRateAssignments();
    } catch (e) {
      fxRateError = `Failed to save FX source assignment: ${String(e)}`;
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
    } catch (e) {
      fxRateError = `Failed to delete FX source assignment: ${String(e)}`;
    }
  }

  async function loadFxRefreshState() {
    try {
      fxRateRefreshState = await listFxRateRefreshState<FxRateRefreshState[]>(1);
    } catch (e) {
      fxRateError = `Failed to load FX refresh status: ${String(e)}`;
    }
  }

  async function loadFxRefreshExecutionStatus() {
    if (!pricingAutomationManagedByServer) {
      fxRefreshExecutionStatus = null;
      return;
    }

    try {
      fxRefreshExecutionStatus = await getFxRefreshExecutionStatus<FxRefreshExecutionStatus>(1);
    } catch (e) {
      fxRateError = `Failed to load FX execution status: ${String(e)}`;
    }
  }

  async function refreshFxRatesNow() {
    fxRateError = "";
    fxRateStatus = "";
    busy = true;
    try {
      const summary = await refreshFxRatesNowCommand<FxRefreshRunSummary>();
      fxRateStatus = `FX refresh complete. ${summary.pairs_success}/${summary.pairs_total} pairs updated, ${summary.rates_inserted} rates.`;
      if (summary.last_error) {
        fxRateError = `Last error: ${summary.last_error}`;
      }
      await loadFxRefreshState();
      await loadFxRefreshExecutionStatus();
    } catch (e) {
      fxRateError = `Failed to refresh FX rates: ${String(e)}`;
    } finally {
      busy = false;
    }
  }

  function openNewFxDaily() {
    const activeCurrencies = currencies.filter(c => c.is_active);
    const defaultCurrency = currencies.find(c => c.is_default);
    newFxDaily = {
      from_currency_id: activeCurrencies[0]?.id || 0,
      to_currency_id: defaultCurrency?.id || 0,
      rate_date: new Date().toISOString().split("T")[0],
      rate: 1.0,
      source: "",
    };
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
        book_id: 1,
        from_currency_id: newFxDaily.from_currency_id,
        to_currency_id: newFxDaily.to_currency_id,
        rate_date: newFxDaily.rate_date,
        rate: newFxDaily.rate,
        source: newFxDaily.source || null,
      });
      fxRateStatus = "Daily FX rate added.";
      closeFxDailyDialog();
      await loadFxRatesDaily();
    } catch (e) {
      fxRateError = `Failed to add FX rate: ${String(e)}`;
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
    } catch (e) {
      fxRateError = `Failed to delete FX rate: ${String(e)}`;
    }
  }

  function openNewFxOfficial() {
    const activeCurrencies = currencies.filter(c => c.is_active);
    const defaultCurrency = currencies.find(c => c.is_default);
    newFxOfficial = {
      from_currency_id: activeCurrencies[0]?.id || 0,
      to_currency_id: defaultCurrency?.id || 0,
      period_type: "yearly",
      period_year: new Date().getFullYear(),
      period_month: null,
      rate: 1.0,
      source_name: fxRateSources[0]?.name || "",
      source_url: "",
      notes: "",
    };
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
        book_id: 1,
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
    } catch (e) {
      fxRateError = `Failed to add official FX rate: ${String(e)}`;
    } finally {
      busy = false;
    }
  }

  async function deleteFxOfficial(rate: FxRateOfficial) {
    const period = rate.period_type === "monthly" ? `${rate.period_year}-${String(rate.period_month).padStart(2, "0")}` : String(rate.period_year);
    if (!confirm(`Delete official FX rate ${rate.from_currency_symbol}/${rate.to_currency_symbol} for ${period}?`)) return;
    fxRateError = "";
    fxRateStatus = "";
    try {
      await deleteFxRateOfficial(rate.id);
      fxRateStatus = "Official FX rate deleted.";
      await loadFxRatesOfficial();
    } catch (e) {
      fxRateError = `Failed to delete official FX rate: ${String(e)}`;
    }
  }
</script>

<Card.Root>
  <Card.Header>
    <Card.Title>Currencies &amp; Commodities</Card.Title>
  </Card.Header>
  <Card.Content>
    <!-- Sub-tabs for currencies, commodities, fx rates -->
    <div class="flex gap-2 mb-4 border-b">
      <button
        class="px-4 py-2 text-sm font-medium {commoditiesSubTab === 'currencies' ? 'border-b-2 border-primary text-primary' : 'text-muted-foreground hover:text-foreground'}"
        onclick={() => commoditiesSubTab = 'currencies'}
      >
        Currencies
      </button>
      <button
        class="px-4 py-2 text-sm font-medium {commoditiesSubTab === 'commodities' ? 'border-b-2 border-primary text-primary' : 'text-muted-foreground hover:text-foreground'}"
        onclick={() => commoditiesSubTab = 'commodities'}
      >
        Other Commodities
      </button>
      <button
        class="px-4 py-2 text-sm font-medium {commoditiesSubTab === 'fx-daily' ? 'border-b-2 border-primary text-primary' : 'text-muted-foreground hover:text-foreground'}"
        onclick={() => { commoditiesSubTab = 'fx-daily'; loadFxRateSources(); loadFxRatesDaily(); }}
      >
        Daily FX Rates
      </button>
      <button
        class="px-4 py-2 text-sm font-medium {commoditiesSubTab === 'fx-official' ? 'border-b-2 border-primary text-primary' : 'text-muted-foreground hover:text-foreground'}"
        onclick={() => { commoditiesSubTab = 'fx-official'; loadFxRateSources(); loadFxRatesOfficial(); }}
      >
        Official FX Rates
      </button>
      <button
        class="px-4 py-2 text-sm font-medium {commoditiesSubTab === 'fx-settings' ? 'border-b-2 border-primary text-primary' : 'text-muted-foreground hover:text-foreground'}"
        onclick={() => {
          commoditiesSubTab = 'fx-settings';
          loadFxRateSources();
          loadFxRateSettings();
          loadFxRateAssignments();
          loadFxRefreshState();
          loadFxRefreshExecutionStatus();
        }}
      >
        FX Settings
      </button>
    </div>

    <!-- Currencies Sub-tab -->
    {#if commoditiesSubTab === 'currencies'}
      <div class="flex justify-between items-center mb-4">
        <p class="text-sm text-muted-foreground">Manage active currencies and set your default currency. Inactive currencies stay visible here but are excluded from FX entry and refresh flows. The default currency always remains active.</p>
        <Button onclick={openNewCurrency} disabled={busy} size="sm">Add Currency</Button>
      </div>
      {#if currencyStatus}
        <p class="text-sm text-green-600 mb-2">{currencyStatus}</p>
      {/if}
      {#if currencyError}
        <p class="text-sm text-destructive mb-2">{currencyError}</p>
      {/if}

      <Table.Root>
        <Table.Header>
          <Table.Row>
            <Table.Head>Code</Table.Head>
            <Table.Head>Symbol</Table.Head>
            <Table.Head>Name</Table.Head>
            <Table.Head>Scale</Table.Head>
            <Table.Head>Status</Table.Head>
            <Table.Head class="text-right">Actions</Table.Head>
          </Table.Row>
        </Table.Header>
        <Table.Body>
          {#each sortedCurrencies as c}
            <Table.Row class={!c.is_active ? 'opacity-50' : ''}>
              <Table.Cell class="font-mono font-semibold">{c.symbol || "—"}</Table.Cell>
              <Table.Cell class="text-lg">{c.display_symbol || "—"}</Table.Cell>
              <Table.Cell>{c.name}</Table.Cell>
              <Table.Cell>{c.scale}</Table.Cell>
              <Table.Cell>
                {#if c.is_default}
                  <Badge variant="default">Default</Badge>
                {:else if c.is_active}
                  <Badge variant="secondary">Active</Badge>
                {:else}
                  <Badge variant="outline">Inactive</Badge>
                {/if}
              </Table.Cell>
              <Table.Cell class="text-right">
                <Button variant="ghost" size="sm" onclick={() => openEditCurrency(c)}>Edit</Button>
                {#if !c.is_default}
                  <Button variant="ghost" size="sm" onclick={() => toggleCurrencyActive(c)}>{c.is_active ? 'Deactivate' : 'Activate'}</Button>
                {/if}
                {#if !c.is_default}
                  <Button variant="ghost" size="sm" onclick={() => setDefaultCurrency(c)}>Set Default</Button>
                {/if}
              </Table.Cell>
            </Table.Row>
          {:else}
            <Table.Row>
              <Table.Cell colspan={6} class="text-muted-foreground">No currencies found.</Table.Cell>
            </Table.Row>
          {/each}
        </Table.Body>
      </Table.Root>
    {/if}

    <!-- Other Commodities Sub-tab -->
    {#if commoditiesSubTab === 'commodities'}
      {#if commodityStatus}
        <p class="text-sm text-green-600 mb-2">{commodityStatus}</p>
      {/if}
      {#if commodityError}
        <p class="text-sm text-destructive mb-2">{commodityError}</p>
      {/if}

      <Table.Root>
        <Table.Header>
          <Table.Row>
            <Table.Head>Symbol</Table.Head>
            <Table.Head>Name</Table.Head>
            <Table.Head>Kind</Table.Head>
            <Table.Head>Scale</Table.Head>
            <Table.Head class="text-right">Actions</Table.Head>
          </Table.Row>
        </Table.Header>
        <Table.Body>
          {#each commodities.filter(c => c.kind !== 'currency') as c}
            <Table.Row>
              <Table.Cell class="font-mono">{c.symbol || "—"}</Table.Cell>
              <Table.Cell>{c.name}</Table.Cell>
              <Table.Cell>
                <Badge variant="default">{c.kind}</Badge>
              </Table.Cell>
              <Table.Cell>{c.scale}</Table.Cell>
              <Table.Cell class="text-right">
                <Button variant="ghost" size="sm" onclick={() => openEditCommodity(c)}>Edit</Button>
              </Table.Cell>
            </Table.Row>
          {:else}
            <Table.Row>
              <Table.Cell colspan={5} class="text-muted-foreground">No non-currency commodities found.</Table.Cell>
            </Table.Row>
          {/each}
        </Table.Body>
      </Table.Root>
    {/if}

    <!-- Daily FX Rates Sub-tab -->
    {#if commoditiesSubTab === 'fx-daily'}
      <div class="flex justify-between items-center mb-4">
        <p class="text-sm text-muted-foreground">Market exchange rates for daily currency conversion.</p>
        <Button onclick={openNewFxDaily} disabled={busy} size="sm">Add Rate</Button>
      </div>
      {#if fxRateStatus}
        <p class="text-sm text-green-600 mb-2">{fxRateStatus}</p>
      {/if}
      {#if fxRateError}
        <p class="text-sm text-destructive mb-2">{fxRateError}</p>
      {/if}

      <Table.Root>
        <Table.Header>
          <Table.Row>
            <Table.Head>Date</Table.Head>
            <Table.Head>From</Table.Head>
            <Table.Head>To</Table.Head>
            <Table.Head>Rate</Table.Head>
            <Table.Head>Source</Table.Head>
            <Table.Head class="text-right">Actions</Table.Head>
          </Table.Row>
        </Table.Header>
        <Table.Body>
          {#each fxRatesDaily as rate}
            <Table.Row>
              <Table.Cell>{rate.rate_date}</Table.Cell>
              <Table.Cell class="font-mono">{rate.from_currency_symbol || '?'}</Table.Cell>
              <Table.Cell class="font-mono">{rate.to_currency_symbol || '?'}</Table.Cell>
              <Table.Cell class="font-mono">{rate.rate.toFixed(6)}</Table.Cell>
              <Table.Cell class="text-muted-foreground">{rate.source || '—'}</Table.Cell>
              <Table.Cell class="text-right">
                <Button variant="ghost" size="sm" class="text-destructive" onclick={() => deleteFxDaily(rate)}>Delete</Button>
              </Table.Cell>
            </Table.Row>
          {:else}
            <Table.Row>
              <Table.Cell colspan={6} class="text-muted-foreground">No daily FX rates found.</Table.Cell>
            </Table.Row>
          {/each}
        </Table.Body>
      </Table.Root>
    {/if}

    <!-- Official FX Rates Sub-tab -->
    {#if commoditiesSubTab === 'fx-official'}
      <div class="flex justify-between items-center mb-4">
        <p class="text-sm text-muted-foreground">Official exchange rates from tax authorities for tax reporting purposes.</p>
        <Button onclick={openNewFxOfficial} disabled={busy} size="sm">Add Official Rate</Button>
      </div>
      {#if fxRateStatus}
        <p class="text-sm text-green-600 mb-2">{fxRateStatus}</p>
      {/if}
      {#if fxRateError}
        <p class="text-sm text-destructive mb-2">{fxRateError}</p>
      {/if}

      <Table.Root>
        <Table.Header>
          <Table.Row>
            <Table.Head>Period</Table.Head>
            <Table.Head>From</Table.Head>
            <Table.Head>To</Table.Head>
            <Table.Head>Rate</Table.Head>
            <Table.Head>Source</Table.Head>
            <Table.Head class="text-right">Actions</Table.Head>
          </Table.Row>
        </Table.Header>
        <Table.Body>
          {#each fxRatesOfficial as rate}
            <Table.Row>
              <Table.Cell>
                {#if rate.period_type === 'monthly'}
                  {rate.period_year}-{String(rate.period_month).padStart(2, '0')}
                {:else}
                  {rate.period_year}
                {/if}
                <Badge variant="outline" class="ml-2">{rate.period_type}</Badge>
              </Table.Cell>
              <Table.Cell class="font-mono">{rate.from_currency_symbol || '?'}</Table.Cell>
              <Table.Cell class="font-mono">{rate.to_currency_symbol || '?'}</Table.Cell>
              <Table.Cell class="font-mono">{rate.rate.toFixed(6)}</Table.Cell>
              <Table.Cell>
                {#if rate.source_url}
                  <a href={rate.source_url} target="_blank" rel="noopener" class="text-primary hover:underline">{rate.source_name}</a>
                {:else}
                  {rate.source_name}
                {/if}
              </Table.Cell>
              <Table.Cell class="text-right">
                <Button variant="ghost" size="sm" class="text-destructive" onclick={() => deleteFxOfficial(rate)}>Delete</Button>
              </Table.Cell>
            </Table.Row>
          {:else}
            <Table.Row>
              <Table.Cell colspan={6} class="text-muted-foreground">No official FX rates found.</Table.Cell>
            </Table.Row>
          {/each}
        </Table.Body>
      </Table.Root>
    {/if}

    <!-- FX Settings Sub-tab -->
    {#if commoditiesSubTab === 'fx-settings'}
      <div class="flex items-center justify-between mb-4">
        <div>
          <p class="text-sm text-muted-foreground">Configure pricing refresh policy and manage source assignments. In the web app, scheduled execution is owned by a backend worker rather than the browser session.</p>
        </div>
        <div class="flex gap-2">
          <Button onclick={refreshFxRatesNow} disabled={busy || (pricingAutomationManagedByServer && fxRefreshExecutionStatus?.is_running)} size="sm">
            {pricingAutomationManagedByServer ? 'Run Refresh Now' : 'Refresh Now'}
          </Button>
          <Button onclick={saveFxRateSettings} disabled={busy || !fxRateSettings} size="sm" variant="secondary">Save Settings</Button>
        </div>
      </div>

      {#if pricingAutomationManagedByServer}
        <p class="text-sm text-muted-foreground mb-4">Refresh jobs are owned by the backend scheduler using the policy and assignments below. Manual runs request the backend worker directly and do not start browser-side automation.</p>
      {/if}

      {#if fxRateStatus}
        <p class="text-sm text-green-600 mb-2">{fxRateStatus}</p>
      {/if}
      {#if fxRateError}
        <p class="text-sm text-destructive mb-2">{fxRateError}</p>
      {/if}

      {#if fxRateSettings}
          <div class="grid gap-4 lg:grid-cols-3 mb-6">
            <Card.Root>
              <Card.Header>
                <Card.Title>Base Currency</Card.Title>
              </Card.Header>
              <Card.Content class="space-y-3">
                <div class="grid gap-2">
                  <Label for="fx-base-currency">Base currency</Label>
                  <select id="fx-base-currency" class="flex h-9 w-full rounded-md border border-input bg-transparent px-3 py-1 text-sm shadow-sm transition-colors focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring" bind:value={fxRateSettings.base_currency_id}>
                    {#each currencies as c}
                      <option value={c.id}>{c.display_symbol || ''} {c.symbol} - {c.name}</option>
                    {/each}
                  </select>
                </div>
                <div class="grid gap-2">
                  <Label for="fx-default-source">Default source</Label>
                  <select id="fx-default-source" class="flex h-9 w-full rounded-md border border-input bg-transparent px-3 py-1 text-sm shadow-sm transition-colors focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring" bind:value={fxRateSettings.default_source_id}>
                    <option value="">Select source</option>
                    {#each fxRateSources as s}
                      <option value={s.id}>{s.name}</option>
                    {/each}
                  </select>
                </div>
              </Card.Content>
            </Card.Root>

            <Card.Root>
              <Card.Header>
                <Card.Title>Refresh Schedule</Card.Title>
              </Card.Header>
              <Card.Content class="space-y-3">
                <div class="flex items-center justify-between">
                  <Label for="fx-refresh-enabled">Enable refresh</Label>
                  <input id="fx-refresh-enabled" type="checkbox" class="h-4 w-4" bind:checked={fxRateSettings.refresh_enabled} />
                </div>
                <div class="grid grid-cols-2 gap-3">
                  <div class="grid gap-2">
                    <Label for="fx-refresh-hour">Hour (UTC)</Label>
                    <Input id="fx-refresh-hour" type="number" min="0" max="23" bind:value={fxRateSettings.refresh_hour_utc} />
                  </div>
                  <div class="grid gap-2">
                    <Label for="fx-refresh-minute">Minute (UTC)</Label>
                    <Input id="fx-refresh-minute" type="number" min="0" max="59" bind:value={fxRateSettings.refresh_minute_utc} />
                  </div>
                </div>
              </Card.Content>
            </Card.Root>

            <Card.Root>
              <Card.Header>
                <Card.Title>Backfill Policy</Card.Title>
              </Card.Header>
              <Card.Content class="space-y-3">
                <div class="grid gap-2">
                  <Label for="fx-backfill-days">Max backfill days</Label>
                  <Input id="fx-backfill-days" type="number" min="1" bind:value={fxRateSettings.max_backfill_days} />
                </div>
                <div class="grid gap-2">
                  <Label for="fx-weekend-policy">Weekend policy</Label>
                  <select id="fx-weekend-policy" class="flex h-9 w-full rounded-md border border-input bg-transparent px-3 py-1 text-sm shadow-sm transition-colors focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring" bind:value={fxRateSettings.weekend_policy}>
                    <option value="skip">Skip</option>
                    <option value="fill_previous">Fill previous</option>
                    <option value="download">Download</option>
                  </select>
                </div>
              </Card.Content>
            </Card.Root>
          </div>
      {/if}

      {#if pricingAutomationManagedByServer && fxRefreshExecutionStatus}
        <div class="grid gap-4 lg:grid-cols-3 mb-6">
          <Card.Root>
            <Card.Header>
              <Card.Title>Worker Status</Card.Title>
            </Card.Header>
            <Card.Content class="space-y-2 text-sm">
              <div class="flex items-center justify-between">
                <span class="text-muted-foreground">Scheduler</span>
                <Badge variant={fxRefreshExecutionStatus.scheduler_enabled ? 'secondary' : 'outline'}>
                  {fxRefreshExecutionStatus.scheduler_enabled ? 'Enabled' : 'Disabled'}
                </Badge>
              </div>
              <div class="flex items-center justify-between">
                <span class="text-muted-foreground">Run state</span>
                <Badge variant={fxRefreshExecutionStatus.is_running ? 'secondary' : 'outline'}>
                  {fxRefreshExecutionStatus.is_running ? 'Running' : 'Idle'}
                </Badge>
              </div>
              <div>
                <span class="text-muted-foreground">Poll interval:</span>
                <span class="ml-2">{fxRefreshExecutionStatus.scheduler_poll_seconds}s</span>
              </div>
              <div>
                <span class="text-muted-foreground">Next scheduled:</span>
                <span class="ml-2">{fxRefreshExecutionStatus.next_scheduled_at || '—'}</span>
              </div>
            </Card.Content>
          </Card.Root>

          <Card.Root>
            <Card.Header>
              <Card.Title>Last Run</Card.Title>
            </Card.Header>
            <Card.Content class="space-y-2 text-sm">
              {#if fxRefreshExecutionStatus.last_run}
                <div>
                  <span class="text-muted-foreground">Trigger:</span>
                  <span class="ml-2 capitalize">{fxRefreshExecutionStatus.last_run.trigger}</span>
                </div>
                <div>
                  <span class="text-muted-foreground">Finished:</span>
                  <span class="ml-2">{fxRefreshExecutionStatus.last_run.finished_at}</span>
                </div>
                <div>
                  <span class="text-muted-foreground">Pairs:</span>
                  <span class="ml-2">{fxRefreshExecutionStatus.last_run.pairs_success}/{fxRefreshExecutionStatus.last_run.pairs_total}</span>
                </div>
                <div>
                  <span class="text-muted-foreground">Rates inserted:</span>
                  <span class="ml-2">{fxRefreshExecutionStatus.last_run.rates_inserted}</span>
                </div>
              {:else}
                <p class="text-muted-foreground">No execution run recorded yet.</p>
              {/if}
            </Card.Content>
          </Card.Root>

          <Card.Root>
            <Card.Header>
              <Card.Title>Backend Ownership</Card.Title>
            </Card.Header>
            <Card.Content class="space-y-2 text-sm text-muted-foreground">
              <p>Policy changes update the server-owned schedule.</p>
              <p>Manual runs queue work on the backend worker instead of the browser.</p>
              <p>Refresh-state rows below reflect pair-level outcomes from those backend runs.</p>
            </Card.Content>
          </Card.Root>
        </div>
      {/if}

      <div class="flex items-center justify-between mb-2">
          <h3 class="text-lg font-semibold">Source Assignments</h3>
          <Button onclick={openNewFxAssignment} disabled={busy} size="sm">Add Assignment</Button>
      </div>
      <Table.Root>
          <Table.Header>
            <Table.Row>
              <Table.Head>Pair</Table.Head>
              <Table.Head>Source</Table.Head>
              <Table.Head>Effective</Table.Head>
              <Table.Head class="text-right">Actions</Table.Head>
            </Table.Row>
          </Table.Header>
          <Table.Body>
            {#each fxRateAssignments as assignment}
              <Table.Row>
                <Table.Cell class="font-mono">{assignment.from_currency_symbol}/{assignment.to_currency_symbol}</Table.Cell>
                <Table.Cell>{assignment.source_name}</Table.Cell>
                <Table.Cell>{assignment.effective_from}{assignment.effective_to ? ` → ${assignment.effective_to}` : ''}</Table.Cell>
                <Table.Cell class="text-right">
                  <Button variant="ghost" size="sm" onclick={() => openEditFxAssignment(assignment)}>Edit</Button>
                  <Button variant="ghost" size="sm" class="text-destructive" onclick={() => deleteFxAssignment(assignment)}>Delete</Button>
                </Table.Cell>
              </Table.Row>
            {:else}
              <Table.Row>
                <Table.Cell colspan={4} class="text-muted-foreground">No source assignments configured.</Table.Cell>
              </Table.Row>
            {/each}
          </Table.Body>
      </Table.Root>

      <div class="flex items-center justify-between mt-6 mb-2">
          <h3 class="text-lg font-semibold">Refresh Status</h3>
          <Button variant="secondary" size="sm" onclick={loadFxRefreshState} disabled={busy}>Reload</Button>
      </div>
      <Table.Root>
          <Table.Header>
            <Table.Row>
              <Table.Head>Pair</Table.Head>
              <Table.Head>Source</Table.Head>
              <Table.Head>Last Success</Table.Head>
              <Table.Head>Last Attempt</Table.Head>
              <Table.Head>Status</Table.Head>
            </Table.Row>
          </Table.Header>
          <Table.Body>
            {#each fxRateRefreshState as state}
              <Table.Row>
                <Table.Cell class="font-mono">{state.from_currency_symbol}/{state.to_currency_symbol}</Table.Cell>
                <Table.Cell>{state.source_name || '—'}</Table.Cell>
                <Table.Cell>{state.last_success_date || '—'}</Table.Cell>
                <Table.Cell>{state.last_attempt_at || '—'}</Table.Cell>
                <Table.Cell>
                  {#if state.last_error}
                    <Badge variant="destructive">Error</Badge>
                  {:else}
                    <Badge variant="secondary">OK</Badge>
                  {/if}
                </Table.Cell>
              </Table.Row>
              {#if state.last_error}
                <Table.Row>
                  <Table.Cell colspan={5} class="text-xs text-destructive">{state.last_error}</Table.Cell>
                </Table.Row>
              {/if}
            {:else}
              <Table.Row>
                <Table.Cell colspan={5} class="text-muted-foreground">No refresh status yet.</Table.Cell>
              </Table.Row>
            {/each}
          </Table.Body>
      </Table.Root>
    {/if}
  </Card.Content>
</Card.Root>

<!-- Commodity Dialog -->
<Dialog.Root bind:open={showCommodityDialog}>
  <Dialog.Content>
    <Dialog.Header>
      <Dialog.Title>Edit Commodity</Dialog.Title>
    </Dialog.Header>
    {#if editingCommodity}
    <div class="grid gap-4 py-4">
      <div class="grid gap-2">
        <Label for="commodity-symbol">Symbol</Label>
        <Input id="commodity-symbol" type="text" bind:value={editingCommodity.symbol} placeholder="e.g. USD, EUR, BTC" />
      </div>
      <div class="grid gap-2">
        <Label for="commodity-name">Name</Label>
        <Input id="commodity-name" type="text" bind:value={editingCommodity.name} placeholder="Full name" />
      </div>
      <div class="grid gap-2">
        <Label for="commodity-kind">Kind</Label>
        <Input id="commodity-kind" type="text" value={editingCommodity.kind} disabled />
        <p class="text-sm text-muted-foreground">Kind cannot be changed</p>
      </div>
      <div class="grid gap-2">
        <Label for="commodity-scale">Scale (decimal places)</Label>
        <Input id="commodity-scale" type="number" value={editingCommodity.scale} disabled />
        <p class="text-sm text-muted-foreground">Scale cannot be changed</p>
      </div>
    </div>
    {/if}
    <Dialog.Footer>
      <Button variant="secondary" onclick={closeCommodityDialog}>Cancel</Button>
      <Button onclick={saveCommodity} disabled={busy || !editingCommodity?.name}>
        Update
      </Button>
    </Dialog.Footer>
  </Dialog.Content>
</Dialog.Root>

<!-- Currency Dialog -->
<Dialog.Root bind:open={showCurrencyDialog}>
  <Dialog.Content>
    <Dialog.Header>
      <Dialog.Title>{editingCurrency ? "Edit Currency" : "Add Currency"}</Dialog.Title>
    </Dialog.Header>
    <div class="grid gap-4 py-4">
      <div class="grid grid-cols-2 gap-4">
        <div class="grid gap-2">
          <Label for="currency-symbol">Code (ISO)</Label>
          <Input id="currency-symbol" type="text" bind:value={newCurrency.symbol} placeholder="e.g. EUR, GBP, JPY" maxlength={10} />
          <p class="text-sm text-muted-foreground">ISO 4217 code</p>
        </div>
        <div class="grid gap-2">
          <Label for="currency-display-symbol">Display Symbol</Label>
          <Input id="currency-display-symbol" type="text" bind:value={newCurrency.display_symbol} placeholder="e.g. €, £, ¥, $" maxlength={5} />
          <p class="text-sm text-muted-foreground">Unicode symbol</p>
        </div>
      </div>
      <div class="grid gap-2">
        <Label for="currency-name">Name</Label>
        <Input id="currency-name" type="text" bind:value={newCurrency.name} placeholder="e.g. Euro, British Pound" />
      </div>
      <div class="grid gap-2">
        <Label for="currency-scale">Decimal Places</Label>
        <select id="currency-scale" class="flex h-9 w-full rounded-md border border-input bg-transparent px-3 py-1 text-sm shadow-sm transition-colors focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring" bind:value={newCurrency.scale}>
          <option value={0}>0 (JPY, KRW, etc.)</option>
          <option value={2}>2 (Most currencies)</option>
          <option value={3}>3 (BHD, KWD, etc.)</option>
        </select>
      </div>
    </div>
    <Dialog.Footer>
      <Button variant="secondary" onclick={closeCurrencyDialog}>Cancel</Button>
      <Button onclick={saveCurrency} disabled={busy || !newCurrency.symbol || !newCurrency.name}>
        {editingCurrency ? "Update" : "Create"}
      </Button>
    </Dialog.Footer>
  </Dialog.Content>
</Dialog.Root>

<!-- Daily FX Rate Dialog -->
<Dialog.Root bind:open={showFxDailyDialog}>
  <Dialog.Content>
    <Dialog.Header>
      <Dialog.Title>Add Daily FX Rate</Dialog.Title>
    </Dialog.Header>
    <div class="grid gap-4 py-4">
      <div class="grid gap-2">
        <Label for="fx-daily-date">Date</Label>
        <Input id="fx-daily-date" type="date" bind:value={newFxDaily.rate_date} />
      </div>
      <div class="grid gap-2">
        <Label for="fx-daily-from">From Currency</Label>
        <select id="fx-daily-from" class="flex h-9 w-full rounded-md border border-input bg-transparent px-3 py-1 text-sm shadow-sm transition-colors focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring" bind:value={newFxDaily.from_currency_id}>
          {#each currencies.filter(c => c.is_active) as c}
            <option value={c.id}>{c.display_symbol || ''} {c.symbol} - {c.name}</option>
          {/each}
        </select>
      </div>
      <div class="grid gap-2">
        <Label for="fx-daily-to">To Currency</Label>
        <select id="fx-daily-to" class="flex h-9 w-full rounded-md border border-input bg-transparent px-3 py-1 text-sm shadow-sm transition-colors focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring" bind:value={newFxDaily.to_currency_id}>
          {#each currencies.filter(c => c.is_active) as c}
            <option value={c.id}>{c.display_symbol || ''} {c.symbol} - {c.name}</option>
          {/each}
        </select>
      </div>
      <div class="grid gap-2">
        <Label for="fx-daily-rate">Exchange Rate</Label>
        <Input id="fx-daily-rate" type="number" step="0.000001" bind:value={newFxDaily.rate} />
        <p class="text-sm text-muted-foreground">1 unit of "From" currency = this many units of "To" currency</p>
      </div>
      <div class="grid gap-2">
        <Label for="fx-daily-source">Source (optional)</Label>
        <Input id="fx-daily-source" type="text" bind:value={newFxDaily.source} placeholder="e.g. ECB, XE.com, bank" />
      </div>
    </div>
    <Dialog.Footer>
      <Button variant="secondary" onclick={closeFxDailyDialog}>Cancel</Button>
      <Button onclick={saveFxDaily} disabled={busy || !newFxDaily.rate_date || !newFxDaily.rate}>
        Add Rate
      </Button>
    </Dialog.Footer>
  </Dialog.Content>
</Dialog.Root>

<!-- Official FX Rate Dialog -->
<Dialog.Root bind:open={showFxOfficialDialog}>
  <Dialog.Content>
    <Dialog.Header>
      <Dialog.Title>Add Official FX Rate</Dialog.Title>
      <p class="text-sm text-muted-foreground">Official rates from tax authorities for tax reporting</p>
    </Dialog.Header>
    <div class="grid gap-4 py-4">
      <div class="grid grid-cols-2 gap-4">
        <div class="grid gap-2">
          <Label for="fx-official-type">Period Type</Label>
          <select id="fx-official-type" class="flex h-9 w-full rounded-md border border-input bg-transparent px-3 py-1 text-sm shadow-sm transition-colors focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring" bind:value={newFxOfficial.period_type}>
            <option value="yearly">Yearly Average</option>
            <option value="monthly">Monthly Average</option>
          </select>
        </div>
        <div class="grid gap-2">
          <Label for="fx-official-year">Year</Label>
          <Input id="fx-official-year" type="number" bind:value={newFxOfficial.period_year} min="1990" max="2100" />
        </div>
      </div>
      {#if newFxOfficial.period_type === 'monthly'}
        <div class="grid gap-2">
          <Label for="fx-official-month">Month</Label>
          <select id="fx-official-month" class="flex h-9 w-full rounded-md border border-input bg-transparent px-3 py-1 text-sm shadow-sm transition-colors focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring" bind:value={newFxOfficial.period_month}>
            <option value={1}>January</option>
            <option value={2}>February</option>
            <option value={3}>March</option>
            <option value={4}>April</option>
            <option value={5}>May</option>
            <option value={6}>June</option>
            <option value={7}>July</option>
            <option value={8}>August</option>
            <option value={9}>September</option>
            <option value={10}>October</option>
            <option value={11}>November</option>
            <option value={12}>December</option>
          </select>
        </div>
      {/if}
      <div class="grid gap-2">
        <Label for="fx-official-from">From Currency</Label>
        <select id="fx-official-from" class="flex h-9 w-full rounded-md border border-input bg-transparent px-3 py-1 text-sm shadow-sm transition-colors focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring" bind:value={newFxOfficial.from_currency_id}>
          {#each currencies.filter(c => c.is_active) as c}
            <option value={c.id}>{c.display_symbol || ''} {c.symbol} - {c.name}</option>
          {/each}
        </select>
      </div>
      <div class="grid gap-2">
        <Label for="fx-official-to">To Currency</Label>
        <select id="fx-official-to" class="flex h-9 w-full rounded-md border border-input bg-transparent px-3 py-1 text-sm shadow-sm transition-colors focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring" bind:value={newFxOfficial.to_currency_id}>
          {#each currencies.filter(c => c.is_active) as c}
            <option value={c.id}>{c.display_symbol || ''} {c.symbol} - {c.name}</option>
          {/each}
        </select>
      </div>
      <div class="grid gap-2">
        <Label for="fx-official-rate">Exchange Rate</Label>
        <Input id="fx-official-rate" type="number" step="0.000001" bind:value={newFxOfficial.rate} />
      </div>
      <div class="grid gap-2">
        <Label for="fx-official-source">Source Authority</Label>
        <select id="fx-official-source" class="flex h-9 w-full rounded-md border border-input bg-transparent px-3 py-1 text-sm shadow-sm transition-colors focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring" bind:value={newFxOfficial.source_name}>
          {#each fxRateSources as src}
            <option value={src.name}>{src.name}{src.country_code ? ` (${src.country_code})` : ''}</option>
          {/each}
          <option value="">Other</option>
        </select>
      </div>
      {#if !fxRateSources.find(s => s.name === newFxOfficial.source_name)}
        <div class="grid gap-2">
          <Label for="fx-official-source-custom">Custom Source Name</Label>
          <Input id="fx-official-source-custom" type="text" bind:value={newFxOfficial.source_name} placeholder="e.g. Tax Authority Name" />
        </div>
      {/if}
      <div class="grid gap-2">
        <Label for="fx-official-url">Source URL (optional)</Label>
        <Input id="fx-official-url" type="url" bind:value={newFxOfficial.source_url} placeholder="https://..." />
      </div>
      <div class="grid gap-2">
        <Label for="fx-official-notes">Notes (optional)</Label>
        <Input id="fx-official-notes" type="text" bind:value={newFxOfficial.notes} placeholder="Additional notes" />
      </div>
    </div>
    <Dialog.Footer>
      <Button variant="secondary" onclick={closeFxOfficialDialog}>Cancel</Button>
      <Button onclick={saveFxOfficial} disabled={busy || !newFxOfficial.rate || !newFxOfficial.source_name}>
        Add Rate
      </Button>
    </Dialog.Footer>
  </Dialog.Content>
</Dialog.Root>

<!-- FX Source Assignment Dialog -->
<Dialog.Root bind:open={showFxAssignmentDialog}>
  <Dialog.Content>
    <Dialog.Header>
      <Dialog.Title>{editingAssignment ? "Edit FX Source Assignment" : "Add FX Source Assignment"}</Dialog.Title>
    </Dialog.Header>
    <div class="grid gap-4 py-4">
      <div class="grid gap-2">
        <Label for="fx-assignment-from">From Currency</Label>
        <select id="fx-assignment-from" class="flex h-9 w-full rounded-md border border-input bg-transparent px-3 py-1 text-sm shadow-sm transition-colors focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring" bind:value={newFxAssignment.from_currency_id}>
          {#each currencies as c}
            <option value={c.id}>{c.display_symbol || ''} {c.symbol} - {c.name}</option>
          {/each}
        </select>
      </div>
      <div class="grid gap-2">
        <Label for="fx-assignment-to">To Currency</Label>
        <select id="fx-assignment-to" class="flex h-9 w-full rounded-md border border-input bg-transparent px-3 py-1 text-sm shadow-sm transition-colors focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring" bind:value={newFxAssignment.to_currency_id}>
          {#each currencies as c}
            <option value={c.id}>{c.display_symbol || ''} {c.symbol} - {c.name}</option>
          {/each}
        </select>
      </div>
      <div class="grid gap-2">
        <Label for="fx-assignment-source">Source</Label>
        <select id="fx-assignment-source" class="flex h-9 w-full rounded-md border border-input bg-transparent px-3 py-1 text-sm shadow-sm transition-colors focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring" bind:value={newFxAssignment.source_id}>
          {#each fxRateSources as s}
            <option value={s.id}>{s.name}</option>
          {/each}
        </select>
      </div>
      <div class="grid grid-cols-2 gap-4">
        <div class="grid gap-2">
          <Label for="fx-assignment-from-date">Effective From</Label>
          <Input id="fx-assignment-from-date" type="date" bind:value={newFxAssignment.effective_from} />
        </div>
        <div class="grid gap-2">
          <Label for="fx-assignment-to-date">Effective To</Label>
          <Input id="fx-assignment-to-date" type="date" bind:value={newFxAssignment.effective_to} />
        </div>
      </div>
    </div>
    <Dialog.Footer>
      <Button variant="secondary" onclick={closeFxAssignmentDialog}>Cancel</Button>
      <Button onclick={saveFxAssignment} disabled={busy || !newFxAssignment.source_id || !newFxAssignment.effective_from}>
        {editingAssignment ? "Update" : "Create"}
      </Button>
    </Dialog.Footer>
  </Dialog.Content>
</Dialog.Root>
