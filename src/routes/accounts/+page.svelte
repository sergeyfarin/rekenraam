<script lang="ts">
  import { onMount } from "svelte";
  import { invoke } from "@tauri-apps/api/core";
  import AccountTreeItem from "./AccountTreeItem.svelte";
  import * as Card from "$lib/components/ui/card";
  import { Button } from "$lib/components/ui/button";
  import { Input } from "$lib/components/ui/input";
  import { Label } from "$lib/components/ui/label";
  import * as Dialog from "$lib/components/ui/dialog";
  import * as Table from "$lib/components/ui/table";
  import * as Alert from "$lib/components/ui/alert";
  import { Badge } from "$lib/components/ui/badge";

  type Account = {
    id: number;
    book_id: number;
    parent_id: number | null;
    account_type: string;
    name: string;
    commodity_id: number;
    institution_id: number | null;
    institution_name: string | null;
    country_id: number | null;
    country_name: string | null;
    number_last4: string | null;
    is_closed: boolean;
    created_at: string;
    updated_at: string;
  };

  type AccountBalance = {
    account_id: number;
    balance_minor: number;
  };

  type AccountTreeNode = {
    id: number;
    parent_id: number | null;
    name: string;
    account_type: string;
    commodity_id: number;
    commodity_name: string;
    commodity_scale: number;
    institution_name: string | null;
    country_name: string | null;
    balance_minor: number;
    rollup_balance_minor: number;
    children: AccountTreeNode[];
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

  type Country = {
    id: number;
    book_id: number;
    code: string;
    name: string;
    created_at: string;
    updated_at: string;
  };

  type Institution = {
    id: number;
    book_id: number;
    name: string;
    kind: string;
    country_id: number | null;
    country_name: string | null;
    created_at: string;
    updated_at: string;
  };

  const bookId = 1;

  let accounts: Account[] = [];
  let balances = new Map<number, number>();
  let accountTree: AccountTreeNode[] = [];
  let commodities: Commodity[] = [];
  let countries: Country[] = [];
  let institutions: Institution[] = [];
  let loading = true;
  let error = "";

  let dialogOpen = false;
  let dialogMode: "create" | "edit" = "create";
  let submitting = false;

  let formId: number | null = null;
  let formName = "";
  let formType: Account["account_type"] = "checking";
  let formCommodityId: number | null = null;
  let formInstitutionId: number | null = null;
  let newInstitutionName = "";
  let newInstitutionKind: "bank" | "broker" | "credit_union" | "other" = "bank";
  let newInstitutionCountryId: number | null = null;
  let creatingInstitution = false;
  let formLast4 = "";
  let formIsClosed = false;
  let formParentId: number | null = null;

  let groupBy: "institution" | "type" | "none" = "institution";
  let sortBy: "name" | "balance" | "type" | "institution" = "name";
  let sortDir: "asc" | "desc" = "asc";
  let search = "";
  let includeClosed = false;
  let showAdvancedFilters = false;
  let showAccountTree = true;

  onMount(async () => {
    await loadAccounts();
    await loadCommodities();
    await loadCountries();
    await loadInstitutions();
    await loadAccountTree();
  });

  async function loadAccounts() {
    loading = true;
    error = "";
    try {
      const list = await invoke<Account[]>("list_accounts", { bookId });
      accounts = list;
      await loadBalances();
    } catch (e) {
      error = `Failed to load accounts: ${String(e)}`;
    } finally {
      loading = false;
    }
  }

  async function loadBalances() {
    try {
      const rows = await invoke<AccountBalance[]>("list_account_balances", { bookId });
      balances = new Map(rows.map((row) => [row.account_id, row.balance_minor] as const));
    } catch (e) {
      error = `Failed to load balances: ${String(e)}`;
      balances = new Map();
    }
  }

  async function loadAccountTree() {
    try {
      accountTree = await invoke<AccountTreeNode[]>("get_account_tree", { bookId });
    } catch (e) {
      error = `Failed to load account tree: ${String(e)}`;
      accountTree = [];
    }
  }

  async function loadCommodities() {
    try {
      const rows = await invoke<Commodity[]>("list_commodities", { bookId });
      commodities = rows.filter((row) => row.book_id === bookId);
      if (!formCommodityId && commodities.length > 0) {
        formCommodityId = commodities[0].id;
      }
    } catch (e) {
      error = `Failed to load commodities: ${String(e)}`;
      commodities = [];
    }
  }

  async function loadCountries() {
    try {
      countries = await invoke<Country[]>("list_countries", { bookId });
    } catch (e) {
      error = `Failed to load countries: ${String(e)}`;
      countries = [];
    }
  }

  async function loadInstitutions() {
    try {
      institutions = await invoke<Institution[]>("list_institutions", { bookId });
    } catch (e) {
      error = `Failed to load institutions: ${String(e)}`;
      institutions = [];
    }
  }

  function accountBalance(account: Account) {
    return balances.get(account.id) ?? 0;
  }

  function formatMinor(amountMinor: number) {
    const formatter = new Intl.NumberFormat(undefined, {
      minimumFractionDigits: 2,
      maximumFractionDigits: 2
    });
    return formatter.format(amountMinor / 100);
  }

  function formatMinorWithScale(amountMinor: number, scale: number) {
    const formatter = new Intl.NumberFormat(undefined, {
      minimumFractionDigits: scale,
      maximumFractionDigits: scale
    });
    return formatter.format(amountMinor / Math.pow(10, scale));
  }

  function normalizeOptionalId(value: number | string | null) {
    if (value === null || value === "") return null;
    const num = Number(value);
    return Number.isFinite(num) ? num : null;
  }

  function groupLabel(account: Account) {
    if (groupBy === "institution") {
      return account.institution_name?.trim() || "Unknown institution";
    }
    if (groupBy === "type") {
      return account.account_type || "Unknown type";
    }
    return "All accounts";
  }

  function openCreateDialog() {
    dialogMode = "create";
    dialogOpen = true;
    formId = null;
    formName = "";
    formType = "checking";
    formCommodityId = commodities.filter(c => c.kind === 'currency')[0]?.id ?? commodities[0]?.id ?? null;
    formInstitutionId = null;
    newInstitutionName = "";
    newInstitutionKind = "bank";
    newInstitutionCountryId = null;
    formLast4 = "";
    formIsClosed = false;
    formParentId = null;
  }

  function openEditDialog(account: Account) {
    dialogMode = "edit";
    dialogOpen = true;
    formId = account.id;
    formName = account.name;
    formType = account.account_type;
    formCommodityId = account.commodity_id;
    formInstitutionId = account.institution_id ?? null;
    newInstitutionName = "";
    newInstitutionKind = "bank";
    newInstitutionCountryId = null;
    formLast4 = account.number_last4 ?? "";
    formIsClosed = account.is_closed;
    formParentId = account.parent_id ?? null;
  }

  async function createInstitutionInline() {
    if (!newInstitutionName.trim()) {
      error = "Institution name is required.";
      return;
    }

    creatingInstitution = true;
    error = "";

    try {
      const created = await invoke<Institution>("create_institution", {
        input: {
          book_id: bookId,
          name: newInstitutionName.trim(),
          kind: newInstitutionKind,
          country_id: newInstitutionCountryId
        }
      });
      await loadInstitutions();
      formInstitutionId = created.id;
      newInstitutionName = "";
    } catch (e) {
      error = `Failed to create institution: ${String(e)}`;
    } finally {
      creatingInstitution = false;
    }
  }

  function closeDialog() {
    dialogOpen = false;
    submitting = false;
  }

  async function submitAccount(event: Event) {
    event.preventDefault();
    if (!formName.trim()) {
      error = "Account name is required.";
      return;
    }
    if (!formCommodityId) {
      error = "Select a currency.";
      return;
    }

    submitting = true;
    error = "";

    try {
      if (dialogMode === "create") {
        await invoke<Account>("create_account", {
          input: {
            book_id: bookId,
            parent_id: formParentId,
            account_type: formType,
            name: formName.trim(),
            commodity_id: formCommodityId,
            institution_id: normalizeOptionalId(formInstitutionId),
            country_id: null,
            number_last4: formLast4.trim() || null,
            is_closed: formIsClosed
          }
        });
      } else if (formId !== null) {
        await invoke<Account>("update_account", {
          input: {
            id: formId,
            book_id: bookId,
            parent_id: formParentId,
            account_type: formType,
            name: formName.trim(),
            commodity_id: formCommodityId,
            institution_id: normalizeOptionalId(formInstitutionId),
            country_id: null,
            number_last4: formLast4.trim() || null,
            is_closed: formIsClosed
          }
        });
      }

      closeDialog();
      await loadAccounts();
    } catch (e) {
      error = `Failed to save account: ${String(e)}`;
    } finally {
      submitting = false;
    }
  }

  async function deleteAccount(account: Account) {
    if (!confirm(`Delete account "${account.name}"? This cannot be undone. Any transactions for this account will also be affected.`)) {
      return;
    }

    error = "";
    loading = true;

    try {
      await invoke("delete_account", { accountId: account.id, bookId });
      await loadAccounts();
    } catch (e) {
      error = `Failed to delete account: ${String(e)}`;
    } finally {
      loading = false;
    }
  }

  async function validateClosing(account: Account) {
    error = "";
    try {
      const result = await invoke<{ valid: boolean; issues: string[] }>("validate_account_closing", {
        accountId: account.id,
        bookId,
      });

      if (result.valid) {
        alert(`Account "${account.name}" can be safely closed. No issues found.`);
      } else {
        alert(`Account "${account.name}" has issues:\n\n${result.issues.join("\n")}`);
      }
    } catch (e) {
      error = `Failed to validate account closing: ${String(e)}`;
    }
  }

  $: filtered = accounts.filter((account) => {
    if (!includeClosed && account.is_closed) return false;
    if (!search.trim()) return true;
    const haystack = [
      account.name,
      account.institution_name ?? "",
      account.country_name ?? "",
      account.account_type,
      account.number_last4 ?? ""
    ]
      .join(" ")
      .toLowerCase();
    return haystack.includes(search.trim().toLowerCase());
  });

  $: sorted = [...filtered].sort((a, b) => {
    const direction = sortDir === "asc" ? 1 : -1;
    if (sortBy === "balance") {
      return direction * (accountBalance(a) - accountBalance(b));
    }
    if (sortBy === "institution") {
      return direction * (a.institution_name ?? "").localeCompare(b.institution_name ?? "");
    }
    if (sortBy === "type") {
      return direction * a.account_type.localeCompare(b.account_type);
    }
    return direction * a.name.localeCompare(b.name);
  });

  $: grouped = (() => {
    if (groupBy === "none") {
      return [{ key: "all", label: "All accounts", accounts: sorted }];
    }
    const map = new Map<string, Account[]>();
    for (const account of sorted) {
      const key = groupLabel(account);
      map.set(key, [...(map.get(key) ?? []), account]);
    }
    return Array.from(map.entries()).map(([key, groupAccounts]) => ({
      key,
      label: key,
      accounts: groupAccounts
    }));
  })();
</script>

<main class="py-6">
  <div class="container mx-auto px-6 space-y-6">
    <!-- Header -->
    <div class="flex items-center justify-between">
      <div>
        <h1 class="text-3xl font-bold tracking-tight">Accounts</h1>
        <p class="text-muted-foreground">View, filter, and organize your accounts.</p>
      </div>
      <Button onclick={openCreateDialog}>New account</Button>
    </div>

    {#if error}
      <Alert.Root variant="destructive">
        <Alert.Title>Error</Alert.Title>
        <Alert.Description>{error}</Alert.Description>
      </Alert.Root>
    {/if}

    <!-- Account Tree -->
    <Card.Root>
      <Card.Header class="flex flex-row items-center justify-between">
        <Card.Title>Account Tree</Card.Title>
        <Button variant="ghost" size="sm" onclick={() => (showAccountTree = !showAccountTree)}>
          {showAccountTree ? "Hide" : "Show"}
        </Button>
      </Card.Header>
      {#if showAccountTree}
        <Card.Content>
          {#if accountTree.length === 0}
            <p class="text-sm text-muted-foreground">No accounts yet.</p>
          {:else}
            <ul class="tree">
              {#each accountTree as node}
                <AccountTreeItem {node} {formatMinorWithScale} />
              {/each}
            </ul>
          {/if}
        </Card.Content>
      {/if}
    </Card.Root>

    <!-- Search and Filters -->
    <div class="flex flex-wrap items-end gap-4">
      <div class="flex-1 min-w-50">
        <Label for="account-search">Search</Label>
        <Input
          id="account-search"
          type="text"
          placeholder="Search by name, institution, country, type, or last4"
          bind:value={search}
        />
      </div>

      <div class="flex items-center gap-2">
        <input id="include-closed" type="checkbox" bind:checked={includeClosed} class="h-4 w-4" />
        <Label for="include-closed">Include closed accounts</Label>
      </div>

      <Button variant="outline" onclick={() => (showAdvancedFilters = !showAdvancedFilters)}>
        {showAdvancedFilters ? "Hide filters" : "Show filters"}
      </Button>
    </div>

    {#if showAdvancedFilters}
      <div class="flex flex-wrap gap-4">
        <div class="flex-1 min-w-37.5">
          <Label for="group-by">Group by</Label>
          <select id="group-by" class="flex h-9 w-full rounded-md border border-input bg-transparent px-3 py-1 text-base shadow-xs" bind:value={groupBy}>
            <option value="institution">Institution</option>
            <option value="type">Account type</option>
            <option value="none">None</option>
          </select>
        </div>

        <div class="flex-1 min-w-37.5">
          <Label for="sort-by">Sort by</Label>
          <select id="sort-by" class="flex h-9 w-full rounded-md border border-input bg-transparent px-3 py-1 text-base shadow-xs" bind:value={sortBy}>
            <option value="name">Name</option>
            <option value="balance">Balance</option>
            <option value="institution">Institution</option>
            <option value="type">Account type</option>
          </select>
        </div>

        <div class="flex-1 min-w-37.5">
          <Label for="sort-dir">Direction</Label>
          <select id="sort-dir" class="flex h-9 w-full rounded-md border border-input bg-transparent px-3 py-1 text-base shadow-xs" bind:value={sortDir}>
            <option value="asc">Ascending</option>
            <option value="desc">Descending</option>
          </select>
        </div>
      </div>
    {/if}

    {#if loading}
      <p class="text-sm text-muted-foreground">Loading accounts…</p>
    {:else}
      {#each grouped as group}
        <Card.Root>
          <Card.Header>
            <Card.Title>{group.label}</Card.Title>
          </Card.Header>
          <Card.Content>
            <Table.Root>
              <Table.Header>
                <Table.Row>
                  <Table.Head>Name</Table.Head>
                  <Table.Head>Institution</Table.Head>
                  <Table.Head class="text-right">Balance</Table.Head>
                  <Table.Head class="text-right">Actions</Table.Head>
                </Table.Row>
              </Table.Header>
              <Table.Body>
                {#if group.accounts.length === 0}
                  <Table.Row>
                    <Table.Cell colspan={4} class="text-muted-foreground">No accounts found.</Table.Cell>
                  </Table.Row>
                {:else}
                  {#each group.accounts as account}
                    <Table.Row>
                      <Table.Cell>
                        <a class="font-medium hover:underline" href={`/accounts/${account.id}`}>{account.name}</a>
                        <div class="text-xs text-muted-foreground">
                          <Badge variant="outline" class="mr-1">{account.account_type}</Badge>
                          {#if account.number_last4}
                            <span>••••{account.number_last4}</span>
                          {/if}
                        </div>
                      </Table.Cell>
                      <Table.Cell>
                        {account.institution_name ?? "—"}
                        {#if account.country_name}
                          <div class="text-xs text-muted-foreground">{account.country_name}</div>
                        {/if}
                      </Table.Cell>
                      <Table.Cell class="text-right font-mono">
                        {formatMinor(accountBalance(account))}
                      </Table.Cell>
                      <Table.Cell class="text-right">
                        <Button variant="ghost" size="sm" onclick={() => openEditDialog(account)}>Edit</Button>
                        <Button variant="ghost" size="sm" class="text-destructive" onclick={() => deleteAccount(account)}>Delete</Button>
                      </Table.Cell>
                    </Table.Row>
                  {/each}
                {/if}
              </Table.Body>
            </Table.Root>
          </Card.Content>
        </Card.Root>
      {/each}
    {/if}
  </div>

  <!-- Account Dialog -->
  <Dialog.Root bind:open={dialogOpen}>
    <Dialog.Content class="max-w-lg">
      <Dialog.Header>
        <Dialog.Title>{dialogMode === "create" ? "Create account" : "Edit account"}</Dialog.Title>
      </Dialog.Header>

      <form onsubmit={submitAccount} class="space-y-4">
        <div class="space-y-2">
          <Label for="account-name">Name</Label>
          <Input id="account-name" type="text" bind:value={formName} required />
        </div>

        <div class="space-y-2">
          <Label for="account-type">Account type</Label>
          <select id="account-type" class="flex h-9 w-full rounded-md border border-input bg-transparent px-3 py-1 text-base shadow-xs" bind:value={formType}>
            <option value="cash">Cash</option>
            <option value="checking">Checking</option>
            <option value="savings">Savings</option>
            <option value="credit">Credit</option>
            <option value="loan">Loan</option>
            <option value="investment">Investment</option>
            <option value="asset">Asset</option>
            <option value="liability">Liability</option>
            <option value="income">Income</option>
            <option value="expense">Expense</option>
            <option value="equity">Equity</option>
          </select>
        </div>

        <div class="space-y-2">
          <Label for="account-commodity">Currency</Label>
          <select id="account-commodity" class="flex h-9 w-full rounded-md border border-input bg-transparent px-3 py-1 text-base shadow-xs" bind:value={formCommodityId}>
            {#each commodities.filter(c => c.kind === 'currency') as commodity}
              <option value={commodity.id}>{commodity.symbol ? `${commodity.symbol} - ${commodity.name}` : commodity.name}</option>
            {/each}
          </select>
          {#if commodities.filter(c => c.kind === 'currency').length === 0}
            <p class="text-sm text-muted-foreground">No currencies available. Add currencies in Settings → Commodities.</p>
          {/if}
        </div>

        <div class="space-y-2">
          <Label for="account-institution">Institution</Label>
          <select id="account-institution" class="flex h-9 w-full rounded-md border border-input bg-transparent px-3 py-1 text-base shadow-xs" bind:value={formInstitutionId}>
            <option value="">None</option>
            {#each institutions as institution}
              <option value={institution.id}>{institution.name}{institution.country_name ? ` (${institution.country_name})` : ''}</option>
            {/each}
          </select>
        </div>

        <div class="space-y-2">
          <Label>Create new institution</Label>
          <div class="grid grid-cols-4 gap-2">
            <Input placeholder="Institution name" bind:value={newInstitutionName} />
            <select class="flex h-9 rounded-md border border-input bg-transparent px-3 py-1 text-sm shadow-xs" bind:value={newInstitutionKind}>
              <option value="bank">Bank</option>
              <option value="broker">Broker</option>
              <option value="credit_union">Credit union</option>
              <option value="other">Other</option>
            </select>
            <select class="flex h-9 rounded-md border border-input bg-transparent px-3 py-1 text-sm shadow-xs" bind:value={newInstitutionCountryId}>
              <option value="">Country</option>
              {#each countries as country}
                <option value={country.id}>{country.name}</option>
              {/each}
            </select>
            <Button type="button" variant="secondary" onclick={createInstitutionInline} disabled={creatingInstitution}>
              {creatingInstitution ? "Adding…" : "Add"}
            </Button>
          </div>
        </div>

        <div class="grid grid-cols-2 gap-4">
          <div class="space-y-2">
            <Label for="account-last4">Last 4 digits</Label>
            <Input id="account-last4" type="text" maxlength={4} bind:value={formLast4} />
          </div>

          <div class="flex items-center gap-2 pt-8">
            <input id="account-closed" type="checkbox" bind:checked={formIsClosed} class="h-4 w-4" />
            <Label for="account-closed">Closed</Label>
          </div>
        </div>

        <Dialog.Footer>
          <Button variant="outline" type="button" onclick={closeDialog}>Cancel</Button>
          <Button type="submit" disabled={submitting}>
            {submitting ? "Saving…" : "Save"}
          </Button>
        </Dialog.Footer>
      </form>
    </Dialog.Content>
  </Dialog.Root>
</main>

<style>
  :global(.tree) {
    list-style: none;
    margin: 0;
    padding-left: 0;
  }

  :global(.tree-item) {
    padding: 0.25rem 0;
  }

  :global(.tree-item > .tree) {
    padding-left: 1rem;
    border-left: 1px solid hsl(var(--border));
    margin-left: 0.25rem;
  }

  :global(.tree-row) {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 1rem;
  }

  :global(.tree-title) {
    display: flex;
    align-items: baseline;
    gap: 0.5rem;
  }

  :global(.tree-meta) {
    font-size: 0.75rem;
    color: hsl(var(--muted-foreground));
  }

  :global(.tree-balances) {
    display: flex;
    gap: 0.75rem;
    font-size: 0.875rem;
  }
</style>
