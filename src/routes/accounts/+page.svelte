<script lang="ts">
  import { onMount } from "svelte";
  import { invoke } from "@tauri-apps/api/core";

  type Account = {
    id: number;
    book_id: number;
    parent_id: number | null;
    account_type: string;
    name: string;
    commodity_id: number;
    institution: string | null;
    number_last4: string | null;
    is_closed: boolean;
    created_at: string;
    updated_at: string;
  };

  type AccountBalance = {
    account_id: number;
    balance_minor: number;
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

  const bookId = 1;

  let accounts: Account[] = [];
  let balances = new Map<number, number>();
  let commodities: Commodity[] = [];
  let loading = true;
  let error = "";

  let dialogOpen = false;
  let dialogMode: "create" | "edit" = "create";
  let submitting = false;

  let formId: number | null = null;
  let formName = "";
  let formType: Account["account_type"] = "checking";
  let formCommodityId: number | null = null;
  let formInstitution = "";
  let formLast4 = "";
  let formIsClosed = false;
  let formParentId: number | null = null;

  let groupBy: "institution" | "type" | "none" = "institution";
  let sortBy: "name" | "balance" | "type" | "institution" = "name";
  let sortDir: "asc" | "desc" = "asc";
  let search = "";
  let includeClosed = false;

  onMount(async () => {
    await loadAccounts();
    await loadCommodities();
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

  function groupLabel(account: Account) {
    if (groupBy === "institution") {
      return account.institution?.trim() || "Unknown institution";
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
    formInstitution = "";
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
    formInstitution = account.institution ?? "";
    formLast4 = account.number_last4 ?? "";
    formIsClosed = account.is_closed;
    formParentId = account.parent_id ?? null;
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
            institution: formInstitution.trim() || null,
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
            institution: formInstitution.trim() || null,
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
      account.institution ?? "",
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
      return direction * (a.institution ?? "").localeCompare(b.institution ?? "");
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

<main>
  <div class="bx--grid">
    <div class="bx--row">
      <div class="bx--col-lg-8 bx--col-md-8 bx--col-sm-4">
        <h1 class="bx--type-productive-heading-04">Accounts</h1>
        <p class="bx--type-body-long-02">View, filter, and organize your accounts.</p>
      </div>
      <div class="bx--col-lg-4 bx--col-md-8 bx--col-sm-4 header-actions">
        <button class="bx--btn bx--btn--primary" type="button" on:click={openCreateDialog}>
          New account
        </button>
      </div>
    </div>

    <div class="bx--row controls">
      <div class="bx--col-lg-4 bx--col-md-4 bx--col-sm-4">
        <div class="bx--form-item">
          <label class="bx--label" for="account-search">Search</label>
          <input
            id="account-search"
            class="bx--text-input"
            type="text"
            placeholder="Search by name, institution, type, or last4"
            bind:value={search}
          />
        </div>
      </div>

      <div class="bx--col-lg-3 bx--col-md-4 bx--col-sm-4">
        <div class="bx--form-item">
          <label class="bx--label" for="group-by">Group by</label>
          <div class="bx--select">
            <select id="group-by" class="bx--select-input" bind:value={groupBy}>
              <option value="institution">Institution</option>
              <option value="type">Account type</option>
              <option value="none">None</option>
            </select>
          </div>
        </div>
      </div>

      <div class="bx--col-lg-3 bx--col-md-4 bx--col-sm-4">
        <div class="bx--form-item">
          <label class="bx--label" for="sort-by">Sort by</label>
          <div class="bx--select">
            <select id="sort-by" class="bx--select-input" bind:value={sortBy}>
              <option value="name">Name</option>
              <option value="balance">Balance</option>
              <option value="institution">Institution</option>
              <option value="type">Account type</option>
            </select>
          </div>
        </div>
      </div>

      <div class="bx--col-lg-2 bx--col-md-4 bx--col-sm-4">
        <div class="bx--form-item">
          <label class="bx--label" for="sort-dir">Direction</label>
          <div class="bx--select">
            <select id="sort-dir" class="bx--select-input" bind:value={sortDir}>
              <option value="asc">Ascending</option>
              <option value="desc">Descending</option>
            </select>
          </div>
        </div>
      </div>

      <div class="bx--col-lg-3 bx--col-md-4 bx--col-sm-4">
        <div class="bx--form-item checkbox">
          <label class="bx--checkbox-label" for="include-closed">
            <input id="include-closed" type="checkbox" class="bx--checkbox" bind:checked={includeClosed} />
            <span class="bx--checkbox-label-text">Include closed accounts</span>
          </label>
        </div>
      </div>
    </div>

    {#if loading}
      <div class="bx--row">
        <div class="bx--col-lg-12">
          <p class="bx--type-body-short-01">Loading accounts…</p>
        </div>
      </div>
    {:else if error}
      <div class="bx--row">
        <div class="bx--col-lg-12">
          <p class="bx--type-body-short-01 error">{error}</p>
        </div>
      </div>
    {:else}
      {#each grouped as group}
        <section class="bx--row group" aria-label={group.label}>
          <div class="bx--col-lg-12">
            <div class="bx--tile">
              <div class="bx--structured-list">
                <div class="bx--structured-list-thead">
                  <div class="bx--structured-list-row bx--structured-list-row--header-row">
                    <div class="bx--structured-list-cell heading">{group.label}</div>
                    <div class="bx--structured-list-cell heading">Institution</div>
                    <div class="bx--structured-list-cell heading amount">Balance</div>
                    <div class="bx--structured-list-cell heading action">Actions</div>
                  </div>
                </div>
                <div class="bx--structured-list-tbody">
                  {#if group.accounts.length === 0}
                    <div class="bx--structured-list-row">
                      <div class="bx--structured-list-cell">No accounts found.</div>
                      <div class="bx--structured-list-cell"></div>
                      <div class="bx--structured-list-cell"></div>
                      <div class="bx--structured-list-cell"></div>
                    </div>
                  {:else}
                    {#each group.accounts as account}
                      <div class="bx--structured-list-row">
                        <div class="bx--structured-list-cell">
                          <a class="bx--link" href={`/accounts/${account.id}`}>{account.name}</a>
                          <div class="bx--type-body-short-01 meta">
                            {account.account_type}
                            {#if account.number_last4}
                              ••••{account.number_last4}
                            {/if}
                          </div>
                        </div>
                        <div class="bx--structured-list-cell">
                          {account.institution ?? "—"}
                        </div>
                        <div class="bx--structured-list-cell amount">
                          {formatMinor(accountBalance(account))}
                        </div>
                        <div class="bx--structured-list-cell action">
                          <button
                            class="bx--btn bx--btn--ghost bx--btn--sm"
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
          </div>
        </section>
      {/each}
    {/if}
  </div>

  {#if dialogOpen}
    <div class="dialog-backdrop" role="presentation" on:click={closeDialog}></div>
    <div class="dialog" role="dialog" aria-modal="true" aria-label="Account dialog">
      <form class="bx--form" on:submit={submitAccount}>
        <header class="dialog-header">
          <h2 class="bx--type-productive-heading-03">
            {dialogMode === "create" ? "Create account" : "Edit account"}
          </h2>
        </header>

        <div class="dialog-body">
          <div class="bx--form-item">
            <label class="bx--label" for="account-name">Name</label>
            <input
              id="account-name"
              class="bx--text-input"
              type="text"
              bind:value={formName}
              required
            />
          </div>

          <div class="bx--form-item">
            <label class="bx--label" for="account-type">Account type</label>
            <div class="bx--select">
              <select id="account-type" class="bx--select-input" bind:value={formType}>
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
          </div>

          <div class="bx--form-item">
            <label class="bx--label" for="account-commodity">Commodity</label>
            <div class="bx--select">
              <select
                id="account-commodity"
                class="bx--select-input"
                bind:value={formCommodityId}
              >
                {#each commodities as commodity}
                  <option value={commodity.id}>
                    {commodity.symbol ?? commodity.name}
                  </option>
                {/each}
              </select>
            </div>
          </div>

          <div class="bx--form-item">
            <label class="bx--label" for="account-institution">Institution</label>
            <input
              id="account-institution"
              class="bx--text-input"
              type="text"
              bind:value={formInstitution}
            />
          </div>

          <div class="bx--form-item">
            <label class="bx--label" for="account-last4">Last 4 digits</label>
            <input
              id="account-last4"
              class="bx--text-input"
              type="text"
              maxlength="4"
              bind:value={formLast4}
            />
          </div>

          <div class="bx--form-item checkbox">
            <label class="bx--checkbox-label" for="account-closed">
              <input id="account-closed" type="checkbox" class="bx--checkbox" bind:checked={formIsClosed} />
              <span class="bx--checkbox-label-text">Closed</span>
            </label>
          </div>
        </div>

        <footer class="dialog-footer">
          <button class="bx--btn bx--btn--secondary" type="button" on:click={closeDialog}>
            Cancel
          </button>
          <button class="bx--btn bx--btn--primary" type="submit" disabled={submitting}>
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

  .error {
    color: #da1e28;
  }

  .checkbox {
    padding-top: 1.5rem;
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
    background: #fff;
    width: min(560px, 90vw);
    border-radius: 8px;
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
</style>
