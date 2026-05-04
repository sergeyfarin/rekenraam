<script lang="ts">
  import { onMount } from "svelte";
  import { closeFiscalYear } from "$lib/api/admin";
  import * as Tabs from "$lib/components/ui/tabs";
  import * as Card from "$lib/components/ui/card";
  import * as Alert from "$lib/components/ui/alert";
  import { Button } from "$lib/components/ui/button";
  import { Input } from "$lib/components/ui/input";
  import { Label } from "$lib/components/ui/label";
  import DatabaseSettings from "./DatabaseSettings.svelte";
  import CategorySettings from "./CategorySettings.svelte";
  import PayeeSettings from "./PayeeSettings.svelte";
  import TagSettings from "./TagSettings.svelte";
  import CommoditySettings from "./CommoditySettings.svelte";
  import InstitutionSettings from "./InstitutionSettings.svelte";

  // Types matching the component types
  type Category = {
    id: number;
    book_id: number;
    parent_id: number | null;
    name: string;
    kind: string;
    color: string | null;
    created_at: string;
    updated_at: string;
  };

  type Payee = {
    id: number;
    book_id: number;
    name: string;
    kind: string;
    metadata: string | null;
    created_at: string;
    updated_at: string;
  };

  type Tag = {
    id: number;
    book_id: number;
    name: string;
    color: string | null;
    created_at: string;
    updated_at: string;
  };

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

  type Institution = {
    id: number;
    book_id: number;
    name: string;
    kind: string | null;
    routing: string | null;
    website: string | null;
    metadata: string | null;
    created_at: string;
    updated_at: string;
  };

  // Tab state
  let activeTab: "database" | "categories" | "payees" | "tags" | "commodities" | "institutions" | "year-end" = "database";
  let loadedTabs = new Set<string>();

  // Year-End Close state
  type FiscalYearCloseResult = {
    tx_id: number | null;
    closed_accounts_count: number;
    retained_earnings_delta_minor: number;
    close_date: string;
  };
  let yearEndDate = new Date().getFullYear() - 1 + "-12-31";
  let yearEndMemo = "";
  let yearEndRunning = false;
  let yearEndResult: FiscalYearCloseResult | null = null;
  let yearEndError = "";

  // Shared busy state
  let busy = false;

  // Component references for data
  let databaseSettings: DatabaseSettings;
  let categorySettings: CategorySettings;
  let payeeSettings: PayeeSettings;
  let tagSettings: TagSettings;
  let commoditySettings: CommoditySettings;
  let institutionSettings: InstitutionSettings;

  // Data counts for tab badges
  let categories: Category[] = [];
  let payees: Payee[] = [];
  let tags: Tag[] = [];
  let commodities: Commodity[] = [];
  let institutions: Institution[] = [];

  onMount(async () => {
    await ensureActiveTabLoaded(activeTab);
  });

  $: void ensureActiveTabLoaded(activeTab);

  async function ensureActiveTabLoaded(tab: typeof activeTab) {
    if (loadedTabs.has(tab)) {
      return;
    }

    if (tab === "database" && databaseSettings) {
      await databaseSettings.initialize?.();
      loadedTabs.add(tab);
      return;
    }

    if (tab === "categories" && categorySettings) {
      await categorySettings.loadCategories?.();
      loadedTabs.add(tab);
      return;
    }

    if (tab === "payees" && payeeSettings) {
      await payeeSettings.loadPayees?.();
      loadedTabs.add(tab);
      return;
    }

    if (tab === "tags" && tagSettings) {
      await tagSettings.loadTags?.();
      loadedTabs.add(tab);
      return;
    }

    if (tab === "commodities" && commoditySettings) {
      await commoditySettings.initialize?.();
      loadedTabs.add(tab);
      return;
    }

    if (tab === "institutions" && institutionSettings) {
      await institutionSettings.loadInstitutions?.();
      loadedTabs.add(tab);
      return;
    }

    if (tab === "year-end") {
      loadedTabs.add(tab);
    }
  }

  async function runYearEndClose() {
    yearEndError = "";
    yearEndResult = null;
    yearEndRunning = true;
    try {
      yearEndResult = await closeFiscalYear({ close_date: yearEndDate, memo: yearEndMemo || null });
    } catch (e) {
      yearEndError = String(e);
    } finally {
      yearEndRunning = false;
    }
  }

  function formatCurrency(minor: number) {
    const abs = Math.abs(minor) / 100;
    const sign = minor < 0 ? "-" : minor > 0 ? "+" : "";
    return `${sign}${abs.toLocaleString(undefined, { minimumFractionDigits: 2, maximumFractionDigits: 2 })}`;
  }
</script>

<main class="py-6">
  <div class="container mx-auto px-6 space-y-6">
    <div>
      <h1 class="text-3xl font-bold tracking-tight">Settings</h1>
      <p class="text-muted-foreground">Configure preferences and manage data.</p>
    </div>

    <!-- Tabs -->
    <Tabs.Root bind:value={activeTab}>
      <Tabs.List class="flex-wrap">
        <Tabs.Trigger value="database">Database</Tabs.Trigger>
        <Tabs.Trigger value="categories">Categories ({categories.length})</Tabs.Trigger>
        <Tabs.Trigger value="payees">Payees ({payees.length})</Tabs.Trigger>
        <Tabs.Trigger value="tags">Tags ({tags.length})</Tabs.Trigger>
        <Tabs.Trigger value="commodities">Currencies ({commodities.length})</Tabs.Trigger>
        <Tabs.Trigger value="institutions">Institutions ({institutions.length})</Tabs.Trigger>
        <Tabs.Trigger value="year-end">Year-End</Tabs.Trigger>
      </Tabs.List>
    </Tabs.Root>

    <!-- Database Tab -->
    {#if activeTab === "database"}
      <DatabaseSettings bind:this={databaseSettings} bind:busy />
    {/if}

    <!-- Categories Tab -->
    {#if activeTab === "categories"}
      <CategorySettings bind:this={categorySettings} bind:categories bind:busy />
    {/if}

    <!-- Payees Tab -->
    {#if activeTab === "payees"}
      <PayeeSettings bind:this={payeeSettings} bind:payees bind:busy />
    {/if}

    <!-- Tags Tab -->
    {#if activeTab === "tags"}
      <TagSettings bind:this={tagSettings} bind:tags bind:busy />
    {/if}

    <!-- Commodities Tab -->
    {#if activeTab === "commodities"}
      <CommoditySettings bind:this={commoditySettings} bind:commodities bind:busy />
    {/if}

    <!-- Institutions Tab -->
    {#if activeTab === "institutions"}
      <InstitutionSettings bind:this={institutionSettings} bind:institutions bind:busy />
    {/if}

    <!-- Year-End Close Tab -->
    {#if activeTab === "year-end"}
      <Card.Root>
        <Card.Header>
          <Card.Title>Year-End Close</Card.Title>
          <Card.Description>
            Close a fiscal year: zero out income and expense accounts and roll the net result
            into retained earnings. This creates a single closing transaction that cannot be undone.
          </Card.Description>
        </Card.Header>
        <Card.Content class="space-y-4">
          {#if yearEndError}
            <Alert.Root variant="destructive">
              <Alert.Title>Error</Alert.Title>
              <Alert.Description>{yearEndError}</Alert.Description>
            </Alert.Root>
          {/if}

          {#if yearEndResult}
            <Alert.Root class="border-green-200 bg-green-50">
              <Alert.Title>Year closed successfully</Alert.Title>
              <Alert.Description class="space-y-1">
                <p>Close date: <strong>{yearEndResult.close_date}</strong></p>
                <p>Accounts closed: <strong>{yearEndResult.closed_accounts_count}</strong></p>
                <p>Retained earnings change: <strong>{formatCurrency(yearEndResult.retained_earnings_delta_minor)}</strong></p>
                {#if yearEndResult.tx_id}
                  <p class="text-xs text-muted-foreground">Closing transaction ID: {yearEndResult.tx_id}</p>
                {/if}
              </Alert.Description>
            </Alert.Root>
          {/if}

          <div class="space-y-2 max-w-xs">
            <Label for="year-end-date">Close date</Label>
            <Input
              id="year-end-date"
              type="date"
              bind:value={yearEndDate}
            />
            <p class="text-xs text-muted-foreground">
              All income and expense postings up to and including this date will be closed.
            </p>
          </div>

          <div class="space-y-2 max-w-xs">
            <Label for="year-end-memo">Memo (optional)</Label>
            <Input
              id="year-end-memo"
              placeholder="e.g. FY2025 year-end close"
              bind:value={yearEndMemo}
            />
          </div>

          <Button
            onclick={runYearEndClose}
            disabled={yearEndRunning || !yearEndDate}
            variant="destructive"
          >
            {yearEndRunning ? "Running…" : "Run year-end close"}
          </Button>
        </Card.Content>
      </Card.Root>
    {/if}
  </div>
</main>
