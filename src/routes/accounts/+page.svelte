<script lang="ts">
  import { onMount } from "svelte";
  import { invoke } from "@tauri-apps/api/core";
  import AccountTreeItem from "./AccountTreeItem.svelte";

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
  let formCountryId: number | null = null;
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
    formCommodityId = commodities[0]?.id ?? null;
    formInstitutionId = null;
    formCountryId = null;
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
    formCountryId = account.country_id ?? null;
    newInstitutionName = "";
    newInstitutionKind = "bank";
    newInstitutionCountryId = account.country_id ?? null;
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
      error = "Select a commodity.";
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
            country_id: normalizeOptionalId(formCountryId),
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
            country_id: normalizeOptionalId(formCountryId),
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

<main class="page">
  <div class="page-grid container">
    <div class="page-row">
      <div class="page-col">
        <h1 class="page-title">Accounts</h1>
        <p class="page-subtitle">View, filter, and organize your accounts.</p>
      </div>
      <div class="page-col page-col-actions header-actions">
        <button class="btn btn-primary" type="button" on:click={openCreateDialog}>
          New account
        </button>
      </div>
    </div>

    <div class="page-row">
      <div class="page-col">
        <div class="card account-tree">
          <div class="card-header">
            <h2 class="section-title">Account tree</h2>
            <button
              class="btn btn-ghost btn-sm"
              type="button"
              on:click={() => (showAccountTree = !showAccountTree)}
            >
              {showAccountTree ? "Hide" : "Show"}
            </button>
          </div>
          {#if showAccountTree}
            {#if accountTree.length === 0}
              <p class="text-sm text-muted">No accounts yet.</p>
            {:else}
              <ul class="tree">
                {#each accountTree as node}
                  <AccountTreeItem {node} {formatMinorWithScale} />
                {/each}
              </ul>
            {/if}
          {/if}
        </div>
      </div>
    </div>

    <div class="page-row controls">
      <div class="page-col">
        <div class="form-field">
          <label class="label" for="account-search">Search</label>
          <input
            id="account-search"
            class="input"
            type="text"
            placeholder="Search by name, institution, country, type, or last4"
            bind:value={search}
          />
        </div>
      </div>

      <div class="page-col">
        <div class="form-field checkbox-field">
          <label class="checkbox" for="include-closed">
            <input id="include-closed" type="checkbox" bind:checked={includeClosed} />
            <span>Include closed accounts</span>
          </label>
        </div>
      </div>

      <div class="page-col page-col-actions controls-actions">
        <button
          class="btn btn-ghost"
          type="button"
          on:click={() => (showAdvancedFilters = !showAdvancedFilters)}
        >
          {showAdvancedFilters ? "Hide filters" : "Show filters"}
        </button>
      </div>
    </div>

    {#if showAdvancedFilters}
      <div class="page-row advanced-filters">
        <div class="page-col">
          <div class="form-field">
            <label class="label" for="group-by">Group by</label>
              <select id="group-by" class="select" bind:value={groupBy}>
                <option value="institution">Institution</option>
                <option value="type">Account type</option>
                <option value="none">None</option>
              </select>
          </div>
        </div>

        <div class="page-col">
          <div class="form-field">
            <label class="label" for="sort-by">Sort by</label>
              <select id="sort-by" class="select" bind:value={sortBy}>
                <option value="name">Name</option>
                <option value="balance">Balance</option>
                <option value="institution">Institution</option>
                <option value="type">Account type</option>
              </select>
          </div>
        </div>

        <div class="page-col">
          <div class="form-field">
            <label class="label" for="sort-dir">Direction</label>
              <select id="sort-dir" class="select" bind:value={sortDir}>
                <option value="asc">Ascending</option>
                <option value="desc">Descending</option>
              </select>
          </div>
        </div>
      </div>
    {/if}

    {#if loading}
      <div class="page-row">
        <div class="page-col">
          <p class="text-sm text-muted">Loading accounts…</p>
        </div>
      </div>
    {:else if error}
      <div class="page-row">
        <div class="page-col">
          <p class="text-sm text-error">{error}</p>
        </div>
      </div>
    {:else}
      {#each grouped as group}
        <section class="page-row group" aria-label={group.label}>
          <div class="page-col">
            <div class="card">
              <div class="data-table">
                <div class="data-row header">
                  <div class="data-cell heading">{group.label}</div>
                  <div class="data-cell heading">Institution</div>
                  <div class="data-cell heading amount">Balance</div>
                  <div class="data-cell heading action">Actions</div>
                </div>
                {#if group.accounts.length === 0}
                  <div class="data-row">
                    <div class="data-cell">No accounts found.</div>
                    <div class="data-cell"></div>
                    <div class="data-cell"></div>
                    <div class="data-cell"></div>
                  </div>
                {:else}
                  {#each group.accounts as account}
                    <div class="data-row">
                      <div class="data-cell">
                        <a class="link" href={`/accounts/${account.id}`}>{account.name}</a>
                        <div class="text-sm text-muted meta">
                          {account.account_type}
                          {#if account.number_last4}
                            ••••{account.number_last4}
                          {/if}
                        </div>
                      </div>
                      <div class="data-cell">
                        {account.institution_name ?? "—"}
                        {#if account.country_name}
                          <div class="text-sm text-muted meta">{account.country_name}</div>
                        {/if}
                      </div>
                      <div class="data-cell amount">
                        {formatMinor(accountBalance(account))}
                      </div>
                      <div class="data-cell action">
                        <button
                          class="btn btn-ghost btn-sm"
                          type="button"
                          on:click={() => openEditDialog(account)}
                        >
                          Edit
                        </button>
                      </div>
                    </div>
                  {/each}
                {/if}
              </div>
            </div>
          </div>
        </section>
      {/each}
    {/if}
  </div>

  {#if dialogOpen}
    <div class="dialog-backdrop" role="presentation" on:click={closeDialog}></div>
    <div class="dialog" role="dialog" aria-modal="true" aria-label="Account dialog">
      <form on:submit={submitAccount}>
        <header class="dialog-header">
          <h2 class="section-title">
            {dialogMode === "create" ? "Create account" : "Edit account"}
          </h2>
        </header>

        <div class="dialog-body">
          <div class="form-field">
            <label class="label" for="account-name">Name</label>
            <input
              id="account-name"
              class="input"
              type="text"
              bind:value={formName}
              required
            />
          </div>

          <div class="form-field">
            <label class="label" for="account-type">Account type</label>
              <select id="account-type" class="select" bind:value={formType}>
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

          <div class="form-field">
            <label class="label" for="account-commodity">Commodity</label>
              <select
                id="account-commodity"
                class="select"
                bind:value={formCommodityId}
              >
                {#each commodities as commodity}
                  <option value={commodity.id}>
                    {commodity.symbol ?? commodity.name}
                  </option>
                {/each}
              </select>
          </div>

          <div class="form-field">
            <label class="label" for="account-country">Country</label>
              <select id="account-country" class="select" bind:value={formCountryId}>
                <option value="">None</option>
                {#each countries as country}
                  <option value={country.id}>{country.name} ({country.code})</option>
                {/each}
              </select>
          </div>

          <div class="form-field">
            <label class="label" for="account-institution">Institution</label>
              <select id="account-institution" class="select" bind:value={formInstitutionId}>
                <option value="">None</option>
                {#each institutions as institution}
                  <option value={institution.id}>{institution.name}</option>
                {/each}
              </select>
          </div>

          <div class="form-field">
            <label class="label" for="new-institution-name">Create new institution</label>
            <div class="inline-create">
              <input
                id="new-institution-name"
                class="input"
                type="text"
                placeholder="Institution name"
                bind:value={newInstitutionName}
              />
                <select class="select" bind:value={newInstitutionKind}>
                  <option value="bank">Bank</option>
                  <option value="broker">Broker</option>
                  <option value="credit_union">Credit union</option>
                  <option value="other">Other</option>
                </select>
                <select class="select" bind:value={newInstitutionCountryId}>
                  <option value="">Country</option>
                  {#each countries as country}
                    <option value={country.id}>{country.name}</option>
                  {/each}
                </select>
              <button
                class="btn btn-secondary"
                type="button"
                on:click={createInstitutionInline}
                disabled={creatingInstitution}
              >
                {creatingInstitution ? "Adding…" : "Add"}
              </button>
            </div>
          </div>

          <div class="form-field">
            <label class="label" for="account-last4">Last 4 digits</label>
            <input
              id="account-last4"
              class="input"
              type="text"
              maxlength="4"
              bind:value={formLast4}
            />
          </div>

          <div class="form-field checkbox-field">
            <label class="checkbox" for="account-closed">
              <input id="account-closed" type="checkbox" bind:checked={formIsClosed} />
              <span>Closed</span>
            </label>
          </div>
        </div>

        <footer class="dialog-footer">
          <button class="btn btn-secondary" type="button" on:click={closeDialog}>
            Cancel
          </button>
          <button class="btn btn-primary" type="submit" disabled={submitting}>
            {submitting ? "Saving…" : "Save"}
          </button>
        </footer>
      </form>
    </div>
  {/if}
</main>

<style>
  .controls {
    margin-top: 1rem;
    margin-bottom: 1.5rem;
  }

  .controls-actions {
    display: flex;
    align-items: flex-end;
    justify-content: flex-end;
    padding-bottom: 0.5rem;
  }

  .advanced-filters {
    margin-top: -0.5rem;
    margin-bottom: 1rem;
  }

  .header-actions {
    display: flex;
    justify-content: flex-end;
    align-items: center;
  }

  .group {
    margin-bottom: 1.5rem;
  }

  .heading {
    font-weight: 600;
  }

  .amount {
    text-align: right;
  }

  .action {
    text-align: right;
  }

  .meta {
    opacity: 0.7;
  }

  .checkbox-field {
    padding-top: 1.5rem;
  }

  .account-tree {
    margin-top: 1rem;
    margin-bottom: 1.5rem;
  }

  :global(.tree) {
    list-style: none;
    margin: 0.75rem 0 0 0;
    padding-left: 0;
  }

  :global(.tree-item) {
    padding: 0.25rem 0;
  }

  :global(.tree-item > .tree) {
    padding-left: 1rem;
    border-left: 1px solid #e0e0e0;
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
    color: #6f6f6f;
  }

  :global(.tree-balances) {
    display: flex;
    gap: 0.75rem;
    font-size: 0.875rem;
  }

  .dialog-backdrop {
    position: fixed;
    inset: 0;
    background: rgba(0, 0, 0, 0.4);
    z-index: 30;
  }

  .dialog {
    position: fixed;
    top: 50%;
    left: 50%;
    transform: translate(-50%, -50%);
    background: var(--card);
    width: min(560px, 90vw);
    border-radius: 8px;
    border: 1px solid var(--border);
    box-shadow: 0 8px 24px rgba(0, 0, 0, 0.2);
    z-index: 31;
  }

  .dialog-header {
    padding: 1.5rem 1.5rem 0 1.5rem;
  }

  .dialog-body {
    padding: 1rem 1.5rem;
    display: grid;
    gap: 1rem;
  }

  .dialog-footer {
    padding: 0 1.5rem 1.5rem 1.5rem;
    display: flex;
    justify-content: flex-end;
    gap: 0.75rem;
  }

  .inline-create {
    display: grid;
    grid-template-columns: 1.5fr 1fr 1fr auto;
    gap: 0.5rem;
    align-items: center;
  }
</style>
