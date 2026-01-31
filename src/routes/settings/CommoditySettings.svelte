<script lang="ts">
  import { invoke } from "@tauri-apps/api/core";
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

  let fxRatesDaily: FxRateDaily[] = [];
  let fxRatesOfficial: FxRateOfficial[] = [];
  let fxRateSources: FxRateSource[] = [];
  let fxRateError = "";
  let fxRateStatus = "";
  let showFxDailyDialog = false;
  let showFxOfficialDialog = false;
  let newFxDaily = { from_currency_id: 0, to_currency_id: 0, rate_date: "", rate: 0, source: "" };
  let newFxOfficial = { from_currency_id: 0, to_currency_id: 0, period_type: "yearly", period_year: new Date().getFullYear(), period_month: null as number | null, rate: 0, source_name: "", source_url: "", notes: "" };

  // Sub-tabs
  let commoditiesSubTab: "currencies" | "commodities" | "fx-daily" | "fx-official" = "currencies";

  export async function initialize() {
    await loadCommodities();
    await loadCurrencies();
    await loadFxRateSources();
  }

  export async function loadCommodities() {
    try {
      commodities = await invoke<Commodity[]>("list_commodities", { bookId: 1 });
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
      await invoke("rename_commodity_symbol", {
        input: {
          id: editingCommodity.id,
          book_id: 1,
          symbol: editingCommodity.symbol,
          name: editingCommodity.name,
          metadata: editingCommodity.metadata,
        },
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
      currencies = await invoke<Currency[]>("list_currencies", { bookId: 1 });
    } catch (e) {
      currencyError = `Failed to load currencies: ${String(e)}`;
    }
  }

  async function toggleCurrencyActive(currency: Currency) {
    currencyError = "";
    currencyStatus = "";
    try {
      await invoke("toggle_currency_active", { currencyId: currency.id });
      currencyStatus = `Currency ${currency.symbol} ${currency.is_active ? "deactivated" : "activated"}.`;
      await loadCurrencies();
    } catch (e) {
      currencyError = `Failed to toggle currency: ${String(e)}`;
    }
  }

  async function setDefaultCurrency(currency: Currency) {
    currencyError = "";
    currencyStatus = "";
    try {
      await invoke("set_default_currency", { bookId: 1, currencyId: currency.id });
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
        await invoke("update_currency", {
          input: {
            id: editingCurrency.id,
            symbol: newCurrency.symbol,
            display_symbol: newCurrency.display_symbol || null,
            name: newCurrency.name,
            scale: newCurrency.scale,
          },
        });
        currencyStatus = "Currency updated.";
      } else {
        await invoke("create_currency", {
          input: {
            book_id: 1,
            symbol: newCurrency.symbol,
            display_symbol: newCurrency.display_symbol || null,
            name: newCurrency.name,
            scale: newCurrency.scale,
            is_active: true,
          },
        });
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
      fxRatesDaily = await invoke<FxRateDaily[]>("list_fx_rates_daily", { bookId: 1, limit: 100 });
    } catch (e) {
      fxRateError = `Failed to load daily FX rates: ${String(e)}`;
    }
  }

  async function loadFxRatesOfficial() {
    try {
      fxRatesOfficial = await invoke<FxRateOfficial[]>("list_fx_rates_official", { bookId: 1 });
    } catch (e) {
      fxRateError = `Failed to load official FX rates: ${String(e)}`;
    }
  }

  async function loadFxRateSources() {
    try {
      fxRateSources = await invoke<FxRateSource[]>("list_fx_rate_sources", { bookId: 1 });
    } catch (e) {
      fxRateError = `Failed to load FX rate sources: ${String(e)}`;
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
      await invoke("create_fx_rate_daily", {
        input: {
          book_id: 1,
          from_currency_id: newFxDaily.from_currency_id,
          to_currency_id: newFxDaily.to_currency_id,
          rate_date: newFxDaily.rate_date,
          rate: newFxDaily.rate,
          source: newFxDaily.source || null,
        },
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
      await invoke("delete_fx_rate_daily", { id: rate.id });
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
      await invoke("create_fx_rate_official", {
        input: {
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
        },
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
      await invoke("delete_fx_rate_official", { id: rate.id });
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
        onclick={() => { commoditiesSubTab = 'fx-daily'; loadFxRatesDaily(); }}
      >
        Daily FX Rates
      </button>
      <button
        class="px-4 py-2 text-sm font-medium {commoditiesSubTab === 'fx-official' ? 'border-b-2 border-primary text-primary' : 'text-muted-foreground hover:text-foreground'}"
        onclick={() => { commoditiesSubTab = 'fx-official'; loadFxRatesOfficial(); }}
      >
        Official FX Rates
      </button>
    </div>

    <!-- Currencies Sub-tab -->
    {#if commoditiesSubTab === 'currencies'}
      <div class="flex justify-between items-center mb-4">
        <p class="text-sm text-muted-foreground">Manage active currencies and set your default currency. The default currency is used for reports and conversions.</p>
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
          {#each currencies as c}
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
                  <Button variant="ghost" size="sm" onclick={() => setDefaultCurrency(c)}>Set Default</Button>
                  <Button variant="ghost" size="sm" onclick={() => toggleCurrencyActive(c)}>
                    {c.is_active ? 'Deactivate' : 'Activate'}
                  </Button>
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
