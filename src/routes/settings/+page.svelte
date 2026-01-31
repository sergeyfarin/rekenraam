<script lang="ts">
  import { onMount } from "svelte";
  import * as Tabs from "$lib/components/ui/tabs";
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
  let activeTab: "database" | "categories" | "payees" | "tags" | "commodities" | "institutions" = "database";

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
    // Initialize all settings components
    await databaseSettings?.initialize?.();
    await categorySettings?.loadCategories?.();
    await payeeSettings?.loadPayees?.();
    await tagSettings?.loadTags?.();
    await commoditySettings?.initialize?.();
    await institutionSettings?.loadInstitutions?.();
  });
</script>

<main class="py-6">
  <div class="container mx-auto px-6 space-y-6">
    <div>
      <h1 class="text-3xl font-bold tracking-tight">Settings</h1>
      <p class="text-muted-foreground">Configure preferences and manage data.</p>
    </div>

    <!-- Tabs -->
    <Tabs.Root bind:value={activeTab}>
      <Tabs.List>
        <Tabs.Trigger value="database">Database</Tabs.Trigger>
        <Tabs.Trigger value="categories">Categories ({categories.length})</Tabs.Trigger>
        <Tabs.Trigger value="payees">Payees ({payees.length})</Tabs.Trigger>
        <Tabs.Trigger value="tags">Tags ({tags.length})</Tabs.Trigger>
        <Tabs.Trigger value="commodities">Currencies ({commodities.length})</Tabs.Trigger>
        <Tabs.Trigger value="institutions">Institutions ({institutions.length})</Tabs.Trigger>
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
  </div>
</main>
