<script lang="ts">
  import { onMount } from "svelte";
  import { page } from "$app/state";
  import { invoke } from "@tauri-apps/api/core";

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

  type RegisterEntry = {
    tx_id: number;
    txn_date: string;
    payee_id: number | null;
    payee_name: string | null;
    memo: string | null;
    status: string;
    split_id: number;
    account_id: number;
    account_name: string;
    amount_minor: number;
    commodity_id: number;
    category_id: number | null;
    category_name: string | null;
    running_balance_minor: number;
  };

  type AccountDirective = {
    id: number;
    book_id: number;
    account_id: number;
    directive_type: string;
    directive_date: string;
    note: string | null;
    metadata: string | null;
    created_at: string;
  };

  type AccountBalance = {
    account_id: number;
    balance_minor: number;
  };

  let accountId: number | null = null;
  let account: Account | null = null;
  let entries: RegisterEntry[] = [];
  let directives: AccountDirective[] = [];
  let balanceMinor = 0;
  let bookingPolicy = "fifo";
  let savingPolicy = false;
  let loading = true;
  let error = "";

  onMount(async () => {
    const idParam = page.params.id;
    const parsed = Number(idParam);
    if (!Number.isFinite(parsed)) {
      error = "Invalid account id.";
      loading = false;
      return;
    }
    accountId = parsed;
    await loadAccountDetails();
  });

  async function loadAccountDetails() {
    if (accountId === null) return;
    loading = true;
    error = "";
    try {
      const fetched = await invoke<Account | null>("get_account", { id: accountId });
      if (!fetched) {
        error = "Account not found.";
        return;
      }
      account = fetched;

      const balanceRows = await invoke<AccountBalance[]>("list_account_balances", { bookId: 1 });
      const balanceMap = new Map(balanceRows.map((row) => [row.account_id, row.balance_minor] as const));
      balanceMinor = balanceMap.get(accountId) ?? 0;

      directives = await invoke<AccountDirective[]>("list_account_directives", { accountId });
      if (account.account_type === "investment") {
        bookingPolicy = await invoke<string>("get_account_booking_policy", { accountId });
      }

      const rows = await invoke<RegisterEntry[]>("list_account_register_with_balance", {
        accountId,
        limit: 500,
        offset: 0
      });
      entries = rows;
    } catch (e) {
      error = `Failed to load account: ${String(e)}`;
    } finally {
      loading = false;
    }
  }

  function formatMinor(amountMinor: number) {
    const formatter = new Intl.NumberFormat(undefined, {
      minimumFractionDigits: 2,
      maximumFractionDigits: 2
    });
    return formatter.format(amountMinor / 100);
  }

  function findDirectiveDate(kind: string) {
    const matches = directives.filter((d) => d.directive_type === kind);
    if (matches.length === 0) return null;
    return matches[matches.length - 1].directive_date;
  }

  async function saveBookingPolicy() {
    if (!accountId) return;
    savingPolicy = true;
    try {
      bookingPolicy = await invoke<string>("set_account_booking_policy", {
        input: { account_id: accountId, booking_policy: bookingPolicy }
      });
    } catch (e) {
      error = `Failed to update booking policy: ${String(e)}`;
    } finally {
      savingPolicy = false;
    }
  }
</script>

<main>
  <div class="bx--grid">
    {#if loading}
      <div class="bx--row">
        <div class="bx--col-lg-12">
          <p class="bx--type-body-short-01">Loading account…</p>
        </div>
      </div>
    {:else if error}
      <div class="bx--row">
        <div class="bx--col-lg-12">
          <p class="bx--type-body-short-01 error">{error}</p>
        </div>
      </div>
    {:else if account}
      <div class="bx--row">
        <div class="bx--col-lg-12">
          <div class="bx--tile account-summary">
            <h1 class="bx--type-productive-heading-04">{account.name}</h1>
            <p class="bx--type-body-long-02">
              {account.account_type}
              {#if account.institution_name}
                · {account.institution_name}
              {/if}
              {#if account.country_name}
                · {account.country_name}
              {/if}
              {#if account.number_last4}
                · ••••{account.number_last4}
              {/if}
              {#if account.is_closed}
                · Closed
              {/if}
            </p>
            <div class="account-meta">
              <span>Balance: {formatMinor(balanceMinor)}</span>
              {#if findDirectiveDate("open")}
                <span>Opened: {findDirectiveDate("open")}</span>
              {/if}
              {#if findDirectiveDate("close")}
                <span>Closed: {findDirectiveDate("close")}</span>
              {/if}
            </div>
            {#if account?.account_type === "investment"}
              <div class="booking-policy">
                <label class="bx--label" for="booking-policy">Booking policy</label>
                <div class="bx--select">
                  <select
                    id="booking-policy"
                    class="bx--select-input"
                    bind:value={bookingPolicy}
                    on:change={saveBookingPolicy}
                    disabled={savingPolicy}
                  >
                    <option value="fifo">FIFO</option>
                    <option value="lifo">LIFO</option>
                    <option value="average">Average</option>
                    <option value="strict">Strict</option>
                  </select>
                </div>
              </div>
            {/if}
          </div>
        </div>
      </div>

      <div class="bx--row">
        <div class="bx--col-lg-12">
          <div class="bx--tile">
            <h2 class="bx--type-productive-heading-03">Transactions</h2>
            <div class="bx--structured-list">
              <div class="bx--structured-list-thead">
                <div class="bx--structured-list-row bx--structured-list-row--header-row">
                  <div class="bx--structured-list-cell heading">Date</div>
                  <div class="bx--structured-list-cell heading">Payee</div>
                  <div class="bx--structured-list-cell heading">Memo</div>
                  <div class="bx--structured-list-cell heading">Category</div>
                  <div class="bx--structured-list-cell heading amount">Amount</div>
                  <div class="bx--structured-list-cell heading amount">Balance</div>
                </div>
              </div>
              <div class="bx--structured-list-tbody">
                {#if entries.length === 0}
                  <div class="bx--structured-list-row">
                    <div class="bx--structured-list-cell">No transactions yet.</div>
                    <div class="bx--structured-list-cell"></div>
                    <div class="bx--structured-list-cell"></div>
                    <div class="bx--structured-list-cell"></div>
                    <div class="bx--structured-list-cell"></div>
                    <div class="bx--structured-list-cell"></div>
                  </div>
                {:else}
                  {#each entries as entry}
                    <div class="bx--structured-list-row">
                      <div class="bx--structured-list-cell">
                        {entry.txn_date}
                      </div>
                      <div class="bx--structured-list-cell">
                        {entry.payee_name ?? "—"}
                      </div>
                      <div class="bx--structured-list-cell">
                        {entry.memo ?? "—"}
                      </div>
                      <div class="bx--structured-list-cell">
                        {entry.category_name ?? "—"}
                      </div>
                      <div class="bx--structured-list-cell amount">
                        {formatMinor(entry.amount_minor)}
                      </div>
                      <div class="bx--structured-list-cell amount">
                        {formatMinor(entry.running_balance_minor)}
                      </div>
                    </div>
                  {/each}
                {/if}
              </div>
            </div>
          </div>
        </div>
      </div>
    {/if}
  </div>
</main>

<style>
  .account-summary {
    margin-bottom: 1.5rem;
  }

  .account-meta {
    margin-top: 0.5rem;
    display: flex;
    flex-wrap: wrap;
    gap: 1rem;
    font-size: 0.875rem;
    color: #525252;
  }

  .booking-policy {
    margin-top: 1rem;
    max-width: 240px;
  }

  .heading {
    font-weight: 600;
  }

  .amount {
    text-align: right;
  }

  .error {
    color: #da1e28;
  }
</style>
