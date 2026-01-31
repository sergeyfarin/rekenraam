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

  type Category = {
    id: number;
    name: string;
    kind: string;
  };

  type Payee = {
    id: number;
    name: string;
  };

  type Commodity = {
    id: number;
    symbol: string | null;
    name: string;
    scale: number;
  };

  type Transaction = {
    id: number;
    txn_date: string;
    payee_id: number | null;
    memo: string | null;
    status: string;
    reference: string | null;
  };

  type Split = {
    id: number;
    tx_id: number;
    account_id: number;
    commodity_id: number;
    amount_minor: number;
    category_id: number | null;
    memo: string | null;
  };

  type TransactionWithSplits = {
    transaction: Transaction;
    splits: Split[];
  };

  type SplitDraft = {
    account_id: number | null;
    category_id: number | null;
    amount: string;
    memo: string;
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
  let transactions: TransactionWithSplits[] = [];
  let directives: AccountDirective[] = [];
  let balanceMinor = 0;
  let bookingPolicy = "fifo";
  let savingPolicy = false;
  let loading = true;
  let error = "";
  let accounts: Account[] = [];
  let categories: Category[] = [];
  let payees: Payee[] = [];
  let commodities: Commodity[] = [];

  let search = "";
  let dateFrom = "";
  let dateTo = "";
  let statusFilter = "";
  let sortBy: "date" | "payee" | "memo" | "status" | "amount" = "date";
  let sortDir: "asc" | "desc" = "desc";

  let dialogOpen = false;
  let dialogMode: "create" | "edit" = "create";
  let submitting = false;
  let formId: number | null = null;
  let formDate = "";
  let formAccountId: number | null = null;
  let formTransferAccountId: number | null = null;
  let formPayeeId: number | null = null;
  let formNewPayee = "";
  let formMemo = "";
  let formReference = "";
  let formStatus = "uncleared";
  let formCategoryId: number | null = null;
  let formAmount = "";
  let splitMode = false;
  let splitEditorOpen = false;
  let formSplits: SplitDraft[] = [];

  $: selectedCategoryKind = formCategoryId
    ? categories.find((category) => category.id === formCategoryId)?.kind ?? null
    : null;
  $: transferRequired = selectedCategoryKind === "transfer" || !selectedCategoryKind;
  $: if (selectedCategoryKind && selectedCategoryKind !== "transfer") {
    formTransferAccountId = null;
  }

  let tableRef: HTMLDivElement | null = null;
  let txColumnWidths = [12, 18, 22, 12, 16, 10, 10];
  let resizingIndex: number | null = null;
  let resizeStartX = 0;
  let resizeStartWidths: number[] = [];

  $: txGridTemplate = txColumnWidths.map((w) => `${w}%`).join(" ");

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
      await loadLookups();
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
      await loadTransactions();
    } catch (e) {
      error = `Failed to load account: ${String(e)}`;
    } finally {
      loading = false;
    }
  }

  async function loadLookups() {
    const [accountList, categoryList, payeeList, commodityList] = await Promise.all([
      invoke<Account[]>("list_accounts", { bookId: 1 }),
      invoke<Category[]>("list_categories", { bookId: 1 }),
      invoke<Payee[]>("list_payees", { bookId: 1 }),
      invoke<Commodity[]>("list_commodities", { bookId: 1 })
    ]);
    accounts = accountList;
    categories = categoryList;
    payees = payeeList;
    commodities = commodityList;
  }

  async function loadTransactions() {
    if (!accountId) return;
    const filter = {
      book_id: 1,
      account_id: accountId,
      date_from: dateFrom || undefined,
      date_to: dateTo || undefined,
      search: search || undefined,
      limit: 10000,
      offset: 0
    };
    transactions = await invoke<TransactionWithSplits[]>("list_transactions", { filter });
  }

  function formatMinorWithScale(amountMinor: number, scale: number): string {
    const sign = amountMinor < 0 ? "-" : "";
    const abs = Math.abs(amountMinor);
    if (scale <= 0) {
      return `${sign}${abs}`;
    }
    const factor = 10 ** scale;
    const whole = Math.floor(abs / factor);
    const fraction = String(abs % factor).padStart(scale, "0");
    return `${sign}${whole}.${fraction}`;
  }

  function formatMinor(amountMinor: number, commodityId: number) {
    const commodity = commodities.find((c) => c.id === commodityId);
    if (!commodity) return String(amountMinor);
    return `${formatMinorWithScale(amountMinor, commodity.scale)} ${commodity.symbol ?? commodity.name}`;
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

  function payeeName(id: number | null) {
    if (!id) return "—";
    return payees.find((p) => p.id === id)?.name ?? "—";
  }

  function categoryName(id: number | null) {
    if (!id) return "—";
    return categories.find((c) => c.id === id)?.name ?? "—";
  }

  function categoryKind(id: number | null) {
    if (!id) return null;
    return categories.find((c) => c.id === id)?.kind ?? null;
  }

  function startResize(event: PointerEvent, index: number) {
    if (!tableRef || index >= txColumnWidths.length - 1) return;
    event.preventDefault();
    resizingIndex = index;
    resizeStartX = event.clientX;
    resizeStartWidths = [...txColumnWidths];
    document.body.style.cursor = "col-resize";
    document.body.style.userSelect = "none";
    window.addEventListener("pointermove", handleResize);
    window.addEventListener("pointerup", stopResize);
  }

  function handleResize(event: PointerEvent) {
    if (resizingIndex === null || !tableRef) return;
    const deltaX = event.clientX - resizeStartX;
    const totalWidth = tableRef.clientWidth || 1;
    const deltaPercent = (deltaX / totalWidth) * 100;
    const minWidth = 6;
    const nextIndex = resizingIndex + 1;
    let newCurrent = resizeStartWidths[resizingIndex] + deltaPercent;
    let newNext = resizeStartWidths[nextIndex] - deltaPercent;

    if (newCurrent < minWidth) {
      const adjust = minWidth - newCurrent;
      newCurrent = minWidth;
      newNext -= adjust;
    }
    if (newNext < minWidth) {
      const adjust = minWidth - newNext;
      newNext = minWidth;
      newCurrent -= adjust;
    }

    const updated = [...resizeStartWidths];
    updated[resizingIndex] = Number(newCurrent.toFixed(2));
    updated[nextIndex] = Number(newNext.toFixed(2));
    txColumnWidths = updated;
  }

  function stopResize() {
    resizingIndex = null;
    document.body.style.cursor = "";
    document.body.style.userSelect = "";
    window.removeEventListener("pointermove", handleResize);
    window.removeEventListener("pointerup", stopResize);
  }

  function accountName(id: number) {
    return accounts.find((a) => a.id === id)?.name ?? "—";
  }

  function accountSplit(tx: TransactionWithSplits) {
    if (!accountId) return tx.splits[0];
    return tx.splits.find((s) => s.account_id === accountId) ?? tx.splits[0];
  }

  function formatAmountInput(amountMinor: number, commodityId: number): string {
    const commodity = commodities.find((c) => c.id === commodityId);
    if (!commodity) return String(amountMinor);
    return formatMinorWithScale(amountMinor, commodity.scale);
  }

  function parseAmountToMinor(value: string, scale: number): number | null {
    if (!value) return 0;
    const trimmed = value.trim();
    if (!/^[-+]?\d*(\.\d*)?$/.test(trimmed)) return null;
    const sign = trimmed.startsWith("-") ? -1 : 1;
    const [wholePart, fracPartRaw] = trimmed.replace("+", "").replace("-", "").split(".");
    const whole = wholePart ? parseInt(wholePart, 10) : 0;
    const fracPart = (fracPartRaw ?? "").padEnd(scale, "0").slice(0, scale);
    const fraction = fracPart ? parseInt(fracPart, 10) : 0;
    const factor = 10 ** scale;
    return sign * (whole * factor + fraction);
  }

  function statusBadgeClass(status: string) {
    if (status === "reconciled") return "badge success";
    if (status === "void") return "badge danger";
    if (status === "cleared") return "badge warning";
    return "badge";
  }

  function setSort(column: typeof sortBy) {
    if (sortBy === column) {
      sortDir = sortDir === "asc" ? "desc" : "asc";
    } else {
      sortBy = column;
      sortDir = column === "date" ? "desc" : "asc";
    }
  }

  function openCreateDialog() {
    dialogMode = "create";
    formId = null;
    formDate = new Date().toISOString().slice(0, 10);
    formAccountId = accountId;
    formTransferAccountId = null;
    formPayeeId = null;
    formNewPayee = "";
    formMemo = "";
    formReference = "";
    formStatus = "uncleared";
    formCategoryId = null;
    formAmount = "";
    splitMode = false;
    splitEditorOpen = false;
    formSplits = [
      { account_id: accountId, category_id: null, amount: "", memo: "" },
      { account_id: null, category_id: null, amount: "", memo: "" }
    ];
    dialogOpen = true;
  }

  function openEditDialog(tx: TransactionWithSplits) {
    dialogMode = "edit";
    formId = tx.transaction.id;
    formDate = tx.transaction.txn_date;
    const primary = accountSplit(tx);
    const transfer = tx.splits.find((s) => s.account_id !== primary.account_id) ?? tx.splits[0];
    formAccountId = primary.account_id;
    formTransferAccountId = transfer?.account_id ?? null;
    formPayeeId = tx.transaction.payee_id;
    formNewPayee = "";
    formMemo = tx.transaction.memo ?? "";
    formReference = tx.transaction.reference ?? "";
    formStatus = tx.transaction.status;
    formCategoryId = primary.category_id;
    formAmount = formatAmountInput(primary.amount_minor, primary.commodity_id);
    splitMode = tx.splits.length > 2;
    splitEditorOpen = false;
    formSplits = tx.splits.map((split) => ({
      account_id: split.account_id,
      category_id: split.category_id,
      amount: formatAmountInput(split.amount_minor, split.commodity_id),
      memo: split.memo ?? ""
    }));
    dialogOpen = true;
  }

  function closeDialog() {
    dialogOpen = false;
    splitEditorOpen = false;
  }

  function openSplitEditor() {
    if (!splitMode) {
      splitMode = true;
      formSplits = [
        { account_id: formAccountId, category_id: formCategoryId, amount: formAmount, memo: "" },
        { account_id: formTransferAccountId, category_id: null, amount: formAmount ? `-${formAmount}` : "", memo: "" }
      ];
    }
    splitEditorOpen = true;
  }

  function closeSplitEditor() {
    splitEditorOpen = false;
  }

  function addSplitRow() {
    formSplits = [...formSplits, { account_id: null, category_id: null, amount: "", memo: "" }];
  }

  function removeSplitRow(index: number) {
    if (formSplits.length <= 2) return;
    formSplits = formSplits.filter((_, idx) => idx !== index);
  }

  function splitsTotalMinor(): number | null {
    let total = 0;
    for (const split of formSplits) {
      if (!split.account_id) return null;
      const account = accounts.find((a) => a.id === split.account_id);
      if (!account) return null;
      const commodity = commodities.find((c) => c.id === account.commodity_id);
      if (!commodity) return null;
      const minor = parseAmountToMinor(split.amount, commodity.scale);
      if (minor === null) return null;
      total += minor;
    }
    return total;
  }

  async function ensurePayeeId(): Promise<number | null> {
    if (formPayeeId) return formPayeeId;
    if (!formNewPayee.trim()) return null;
    const created = await invoke<Payee>("create_payee", {
      input: { book_id: 1, name: formNewPayee.trim(), kind: "person", metadata: null }
    });
    payees = [...payees, created];
    return created.id;
  }

  async function ensureCategoryAccount(kind: string | null, commodityId: number): Promise<number> {
    const targetKind = kind === "income" ? "income" : "expense";
    const targetName = targetKind === "income" ? "Income" : "Expenses";
    const existing = accounts.find(
      (account) =>
        account.account_type === targetKind &&
        account.commodity_id === commodityId &&
        account.name.toLowerCase() === targetName.toLowerCase()
    );
    if (existing) return existing.id;

    const created = await invoke<Account>("create_account", {
      input: {
        book_id: 1,
        parent_id: null,
        account_type: targetKind,
        name: targetName,
        commodity_id: commodityId,
        institution_id: null,
        country_id: null,
        number_last4: null,
        is_closed: false
      }
    });
    accounts = [...accounts, created];
    return created.id;
  }

  async function unlockAndRetry(tx: TransactionWithSplits, action: () => Promise<void>) {
    const confirm = window.confirm(
      "This transaction is locked (reconciled). Unlocking will void balance checks from this date forward for affected accounts. Continue?"
    );
    if (!confirm) return;
    const reason = window.prompt("Unlock reason (optional):") ?? "";
    const accountIds = Array.from(new Set(tx.splits.map((s) => s.account_id)));
    await Promise.all(
      accountIds.map((account_id) =>
        invoke("unlock_account_balancings", {
          input: {
            account_id,
            from_date: tx.transaction.txn_date,
            reason: reason || null,
            confirm: true
          }
        })
      )
    );
    await action();
  }

  async function buildSplitsForSubmit() {
    if (splitMode) {
      return formSplits.map((split) => {
        if (!split.account_id) {
          throw new Error("All splits need an account");
        }
        const account = accounts.find((a) => a.id === split.account_id);
        if (!account) {
          throw new Error("Invalid account");
        }
        const commodity = commodities.find((c) => c.id === account.commodity_id);
        if (!commodity) {
          throw new Error("Missing commodity for account");
        }
        const amount_minor = parseAmountToMinor(split.amount, commodity.scale);
        if (amount_minor === null) {
          throw new Error("Invalid amount format");
        }
        return {
          account_id: split.account_id,
          commodity_id: account.commodity_id,
          amount_minor,
          category_id: split.category_id,
          memo: split.memo || null
        };
      });
    }

    if (!formAccountId) {
      throw new Error("Account is required");
    }
    const account = accounts.find((a) => a.id === formAccountId);
    if (!account) {
      throw new Error("Invalid account selection");
    }
    const commodity = commodities.find((c) => c.id === account.commodity_id);
    if (!commodity) {
      throw new Error("Missing commodity for account");
    }
    const amount_minor = parseAmountToMinor(formAmount, commodity.scale);
    if (amount_minor === null) {
      throw new Error("Invalid amount format");
    }

    let transferAccountId = formTransferAccountId;
    const kind = categoryKind(formCategoryId);
    if (!transferAccountId) {
      if (kind && kind !== "transfer") {
        transferAccountId = await ensureCategoryAccount(kind, account.commodity_id);
      } else {
        throw new Error("Transfer account required for transfer transactions");
      }
    }
    const transferAccount = accounts.find((a) => a.id === transferAccountId);
    if (!transferAccount) {
      throw new Error("Invalid transfer account");
    }

    return [
      {
        account_id: account.id,
        commodity_id: account.commodity_id,
        amount_minor,
        category_id: formCategoryId,
        memo: null
      },
      {
        account_id: transferAccount.id,
        commodity_id: transferAccount.commodity_id,
        amount_minor: -amount_minor,
        category_id: null,
        memo: null
      }
    ];
  }

  async function submitTransaction() {
    if (!accountId) return;
    submitting = true;
    error = "";
    try {
      const payeeId = await ensurePayeeId();
      const splits = await buildSplitsForSubmit();

      const total = splits.reduce((sum, s) => sum + s.amount_minor, 0);
      if (total !== 0) {
        throw new Error("Splits must balance to zero");
      }

      if (dialogMode === "create") {
        await invoke("create_transaction_with_splits", {
          input: {
            book_id: 1,
            txn_date: formDate,
            payee_id: payeeId,
            memo: formMemo || null,
            status: formStatus,
            reference: formReference || null,
            import_id: null,
            splits
          }
        });
      } else if (formId) {
        await invoke("update_transaction_with_splits", {
          input: {
            id: formId,
            book_id: 1,
            txn_date: formDate,
            payee_id: payeeId,
            memo: formMemo || null,
            status: formStatus,
            reference: formReference || null,
            import_id: null,
            splits
          }
        });
      }

      await loadTransactions();
      closeDialog();
    } catch (e) {
      error = `Failed to save transaction: ${String(e)}`;
    } finally {
      submitting = false;
    }
  }

  async function updateStatus(tx: TransactionWithSplits, status: string) {
    const action = async () => {
      await invoke("update_transaction_with_splits", {
        input: {
          id: tx.transaction.id,
          book_id: 1,
          txn_date: tx.transaction.txn_date,
          payee_id: tx.transaction.payee_id,
          memo: tx.transaction.memo,
          status,
          reference: tx.transaction.reference,
          import_id: null,
          splits: tx.splits.map((split) => ({
            account_id: split.account_id,
            commodity_id: split.commodity_id,
            amount_minor: split.amount_minor,
            category_id: split.category_id,
            memo: split.memo
          }))
        }
      });
    };

    try {
      await action();
      await loadTransactions();
    } catch (e) {
      const msg = String(e);
      if (msg.includes("locked accounts") || tx.transaction.status === "reconciled") {
        await unlockAndRetry(tx, async () => {
          await action();
          await loadTransactions();
        });
      } else {
        error = `Failed to update status: ${msg}`;
      }
    }
  }

  async function removeTransaction(tx: TransactionWithSplits) {
    const confirmed = window.confirm("Delete this transaction? This cannot be undone.");
    if (!confirmed) return;

    const action = async () => {
      await invoke("delete_transaction", { id: tx.transaction.id });
    };

    try {
      await action();
      await loadTransactions();
    } catch (e) {
      const msg = String(e);
      if (msg.includes("locked accounts") || tx.transaction.status === "reconciled") {
        await unlockAndRetry(tx, async () => {
          await action();
          await loadTransactions();
        });
      } else {
        error = `Failed to delete transaction: ${msg}`;
      }
    }
  }

  $: filtered = transactions.filter((tx) => {
    if (statusFilter && tx.transaction.status !== statusFilter) return false;
    if (dateFrom && tx.transaction.txn_date < dateFrom) return false;
    if (dateTo && tx.transaction.txn_date > dateTo) return false;
    if (search) {
      const term = search.toLowerCase();
      const payee = payeeName(tx.transaction.payee_id).toLowerCase();
      const memo = (tx.transaction.memo ?? "").toLowerCase();
      const reference = (tx.transaction.reference ?? "").toLowerCase();
      const accountsText = tx.splits.map((s) => accountName(s.account_id).toLowerCase()).join(" ");
      if (!payee.includes(term) && !memo.includes(term) && !reference.includes(term) && !accountsText.includes(term)) {
        return false;
      }
    }
    return true;
  });

  $: sorted = [...filtered].sort((a, b) => {
    const direction = sortDir === "asc" ? 1 : -1;
    if (sortBy === "date") {
      return direction * a.transaction.txn_date.localeCompare(b.transaction.txn_date);
    }
    if (sortBy === "payee") {
      return direction * payeeName(a.transaction.payee_id).localeCompare(payeeName(b.transaction.payee_id));
    }
    if (sortBy === "memo") {
      return direction * (a.transaction.memo ?? "").localeCompare(b.transaction.memo ?? "");
    }
    if (sortBy === "status") {
      return direction * a.transaction.status.localeCompare(b.transaction.status);
    }
    if (sortBy === "amount") {
      return direction * (accountSplit(a).amount_minor - accountSplit(b).amount_minor);
    }
    return 0;
  });
</script>

<main class="page">
  <div class="page-grid container">
    {#if loading}
      <div class="page-row">
        <div class="page-col">
          <p class="text-sm text-muted">Loading account…</p>
        </div>
      </div>
    {:else if error}
      <div class="page-row">
        <div class="page-col">
          <p class="text-sm text-error">{error}</p>
        </div>
      </div>
    {:else if account}
      <div class="page-row">
        <div class="page-col">
          <div class="card account-summary">
            <h1 class="page-title">{account.name}</h1>
            <p class="page-subtitle">
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
              <span>Balance: {formatMinor(balanceMinor, account.commodity_id)}</span>
              {#if findDirectiveDate("open")}
                <span>Opened: {findDirectiveDate("open")}</span>
              {/if}
              {#if findDirectiveDate("close")}
                <span>Closed: {findDirectiveDate("close")}</span>
              {/if}
            </div>
            {#if account?.account_type === "investment"}
              <div class="booking-policy">
                <label class="label" for="booking-policy">Booking policy</label>
                <select
                  id="booking-policy"
                  class="select"
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
            {/if}
          </div>
        </div>
      </div>

      <div class="page-row">
        <div class="page-col">
          <div class="card">
            <div class="page-row">
              <div class="page-col">
                <h2 class="section-title">Transactions</h2>
              </div>
              <div class="page-col page-col-actions">
                <button class="btn btn-primary" type="button" on:click={openCreateDialog}>New transaction</button>
              </div>
            </div>

            <div class="page-row">
              <div class="page-col">
                <div class="form-field">
                  <label class="label" for="acct-tx-search">Search</label>
                  <input id="acct-tx-search" class="input" placeholder="Search payee, memo, reference" bind:value={search} />
                </div>
              </div>
              <div class="page-col">
                <div class="form-field">
                  <label class="label" for="acct-tx-status">Status</label>
                  <select id="acct-tx-status" class="select" bind:value={statusFilter}>
                    <option value="">Any status</option>
                    <option value="uncleared">Uncleared</option>
                    <option value="cleared">Cleared</option>
                    <option value="reconciled">Reconciled</option>
                    <option value="void">Void</option>
                  </select>
                </div>
              </div>
            </div>
            <div class="page-row">
              <div class="page-col">
                <div class="form-field">
                  <label class="label" for="acct-date-from">From</label>
                  <input id="acct-date-from" class="input" type="date" bind:value={dateFrom} />
                </div>
              </div>
              <div class="page-col">
                <div class="form-field">
                  <label class="label" for="acct-date-to">To</label>
                  <input id="acct-date-to" class="input" type="date" bind:value={dateTo} />
                </div>
              </div>
              <div class="page-col page-col-actions">
                <button class="btn btn-secondary" type="button" on:click={loadTransactions}>Apply filters</button>
              </div>
            </div>

            <div class="data-table striped compact transaction-table" bind:this={tableRef}>
              <div class="data-row header" style={`grid-template-columns: ${txGridTemplate}`}>
                <button class="data-cell heading sort-button col-header" type="button" on:click={() => setSort("date")}>
                  Date
                  <span class="col-resizer" on:pointerdown={(event) => startResize(event, 0)}></span>
                </button>
                <button class="data-cell heading sort-button col-header" type="button" on:click={() => setSort("payee")}>
                  Payee
                  <span class="col-resizer" on:pointerdown={(event) => startResize(event, 1)}></span>
                </button>
                <button class="data-cell heading sort-button col-header" type="button" on:click={() => setSort("memo")}>
                  Memo
                  <span class="col-resizer" on:pointerdown={(event) => startResize(event, 2)}></span>
                </button>
                <button class="data-cell heading sort-button col-header" type="button" on:click={() => setSort("status")}>
                  Status
                  <span class="col-resizer" on:pointerdown={(event) => startResize(event, 3)}></span>
                </button>
                <div class="data-cell heading">Category</div>
                <button class="data-cell heading amount sort-button col-header" type="button" on:click={() => setSort("amount")}>
                  Amount
                  <span class="col-resizer" on:pointerdown={(event) => startResize(event, 5)}></span>
                </button>
                <div class="data-cell heading action">Actions</div>
              </div>
              {#if sorted.length === 0}
                <div class="data-row" style={`grid-template-columns: ${txGridTemplate}`}>
                  <div class="data-cell">No transactions yet.</div>
                  <div class="data-cell"></div>
                  <div class="data-cell"></div>
                  <div class="data-cell"></div>
                  <div class="data-cell"></div>
                  <div class="data-cell"></div>
                  <div class="data-cell"></div>
                </div>
              {:else}
                {#each sorted as tx}
                  {#key tx.transaction.id}
                    <div class="data-row" style={`grid-template-columns: ${txGridTemplate}`}>
                      <div class="data-cell">{tx.transaction.txn_date}</div>
                      <div class="data-cell">{payeeName(tx.transaction.payee_id)}</div>
                      <div class="data-cell">{tx.transaction.memo ?? "—"}</div>
                      <div class="data-cell">
                        <span class={statusBadgeClass(tx.transaction.status)}>{tx.transaction.status}</span>
                      </div>
                      <div class="data-cell">{categoryName(accountSplit(tx).category_id)}</div>
                      <div class="data-cell amount">{formatMinor(accountSplit(tx).amount_minor, accountSplit(tx).commodity_id)}</div>
                      <div class="data-cell action">
                        <button class="btn btn-ghost btn-sm" type="button" on:click={() => openEditDialog(tx)}>Edit</button>
                        {#if tx.transaction.status === "cleared"}
                          <button class="btn btn-ghost btn-sm" type="button" on:click={() => updateStatus(tx, "uncleared")}>Unflag</button>
                        {:else}
                          <button class="btn btn-ghost btn-sm" type="button" on:click={() => updateStatus(tx, "cleared")}>Flag</button>
                        {/if}
                        <button class="btn btn-ghost btn-sm" type="button" on:click={() => updateStatus(tx, "void")}>Void</button>
                        <button class="btn btn-ghost btn-sm" type="button" on:click={() => removeTransaction(tx)}>Delete</button>
                      </div>
                    </div>
                  {/key}
                {/each}
              {/if}
            </div>
          </div>
        </div>
      </div>
    {/if}
  </div>

  {#if dialogOpen}
    <button class="dialog-backdrop" type="button" aria-label="Close dialog" on:click={closeDialog}></button>
    <div class="dialog" role="dialog" aria-modal="true" aria-label="Transaction dialog">
      <form on:submit|preventDefault={submitTransaction}>
        <header class="dialog-header">
          <h2 class="section-title">
            {dialogMode === "create" ? "New transaction" : "Edit transaction"}
          </h2>
        </header>

        <div class="dialog-body">
          <div class="form-field">
            <label class="label" for="acct-tx-date">Date</label>
            <input id="acct-tx-date" class="input" type="date" bind:value={formDate} required />
          </div>

          <div class="form-field">
            <label class="label" for="acct-tx-account">Account</label>
            <select id="acct-tx-account" class="select" bind:value={formAccountId} disabled>
              {#each accounts as accountOption}
                <option value={accountOption.id}>{accountOption.name}</option>
              {/each}
            </select>
          </div>

          <div class="form-field">
            <label class="label" for="acct-tx-transfer-account">Transfer account</label>
            {#if transferRequired}
              <select id="acct-tx-transfer-account" class="select" bind:value={formTransferAccountId} disabled={splitMode}>
                <option value="">Select account</option>
                {#each accounts as accountOption}
                  <option value={accountOption.id}>{accountOption.name}</option>
                {/each}
              </select>
            {:else}
              <p class="text-sm text-muted">Transfer account not required for non-transfer categories.</p>
            {/if}
          </div>

          <div class="form-field">
            <label class="label" for="acct-tx-payee">Payee</label>
            <select id="acct-tx-payee" class="select" bind:value={formPayeeId}>
              <option value="">None</option>
              {#each payees as payee}
                <option value={payee.id}>{payee.name}</option>
              {/each}
            </select>
          </div>

          <div class="form-field">
            <label class="label" for="acct-tx-new-payee">New payee (optional)</label>
            <input id="acct-tx-new-payee" class="input" placeholder="Create payee" bind:value={formNewPayee} />
          </div>

          <div class="form-field">
            <label class="label" for="acct-tx-memo">Memo</label>
            <input id="acct-tx-memo" class="input" bind:value={formMemo} />
          </div>

          <div class="form-field">
            <label class="label" for="acct-tx-ref">Reference</label>
            <input id="acct-tx-ref" class="input" bind:value={formReference} />
          </div>

          <div class="form-field">
            <label class="label" for="acct-tx-category">Category</label>
            <select id="acct-tx-category" class="select" bind:value={formCategoryId} disabled={splitMode}>
              <option value="">None</option>
              {#each categories as category}
                <option value={category.id}>{category.name}</option>
              {/each}
            </select>
          </div>

          <div class="form-field">
            <label class="label" for="acct-tx-amount">Amount</label>
            <input id="acct-tx-amount" class="input" placeholder="0.00" bind:value={formAmount} disabled={splitMode} />
          </div>

          <div class="form-field">
            <label class="label" for="acct-tx-status-select">Status</label>
            <select id="acct-tx-status-select" class="select" bind:value={formStatus}>
              <option value="uncleared">Uncleared</option>
              <option value="cleared">Cleared</option>
              <option value="reconciled">Reconciled</option>
              <option value="void">Void</option>
            </select>
          </div>
          <div class="form-field">
            <button class="btn btn-secondary" type="button" on:click={openSplitEditor}>Split transaction…</button>
            {#if splitMode}
              <p class="text-sm text-muted">Split mode enabled.</p>
            {/if}
          </div>
        </div>

        <footer class="dialog-footer">
          <button class="btn btn-secondary" type="button" on:click={closeDialog}>Cancel</button>
          <button class="btn btn-primary" type="submit" disabled={submitting}>
            {submitting ? "Saving…" : "Save"}
          </button>
        </footer>
      </form>
    </div>
  {/if}

  {#if dialogOpen && splitEditorOpen}
    <button class="dialog-backdrop" type="button" aria-label="Close split editor" on:click={closeSplitEditor}></button>
    <div class="dialog" role="dialog" aria-modal="true" aria-label="Split editor">
      <div class="dialog-header">
        <h2 class="section-title">Split transaction</h2>
      </div>
      <div class="dialog-body">
        <fieldset class="form-field">
          <legend class="label">Splits</legend>
          <div class="data-table compact">
            <div class="data-row header">
              <div class="data-cell heading">Account</div>
              <div class="data-cell heading">Category</div>
              <div class="data-cell heading amount">Amount</div>
              <div class="data-cell heading">Memo</div>
              <div class="data-cell heading action">Action</div>
            </div>
            {#each formSplits as split, idx}
              <div class="data-row">
                <div class="data-cell">
                  <select class="select" bind:value={split.account_id}>
                    <option value="">Select account</option>
                    {#each accounts as accountOption}
                      <option value={accountOption.id}>{accountOption.name}</option>
                    {/each}
                  </select>
                </div>
                <div class="data-cell">
                  <select class="select" bind:value={split.category_id}>
                    <option value="">None</option>
                    {#each categories as category}
                      <option value={category.id}>{category.name}</option>
                    {/each}
                  </select>
                </div>
                <div class="data-cell amount">
                  <input class="input" bind:value={split.amount} placeholder="0.00" />
                </div>
                <div class="data-cell">
                  <input class="input" bind:value={split.memo} placeholder="Split memo" />
                </div>
                <div class="data-cell action">
                  <button class="btn btn-ghost btn-sm" type="button" on:click={() => removeSplitRow(idx)}>
                    Remove
                  </button>
                </div>
              </div>
            {/each}
          </div>
          <button class="btn btn-ghost btn-sm" type="button" on:click={addSplitRow}>Add split</button>
          {#if splitsTotalMinor() !== null}
            <p class="text-sm text-muted">Split total: {splitsTotalMinor()}</p>
          {/if}
        </fieldset>
      </div>
      <div class="dialog-footer">
        <button class="btn btn-secondary" type="button" on:click={closeSplitEditor}>Done</button>
      </div>
    </div>
  {/if}
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
    color: #64748b;
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

  .sort-button {
    background: none;
    border: none;
    padding: 0;
    text-align: left;
    cursor: pointer;
    font: inherit;
    color: inherit;
  }

  .col-header {
    position: relative;
    padding-right: 0.75rem;
  }

  .col-resizer {
    position: absolute;
    right: -4px;
    top: 0;
    height: 100%;
    width: 8px;
    cursor: col-resize;
    touch-action: none;
  }

  .sort-button:focus-visible {
    outline: 2px solid #2563eb;
    outline-offset: 2px;
  }
</style>
