<script lang="ts">
  import { Button } from "$lib/components/ui/button";
  import type { AccountRegisterEntry } from "$lib/api/transactions";
  import type { CommoditySummary, PayeeSummary, CategorySummary } from "$lib/api/metadata";
  import { formatMinorWithScale } from "$lib/money";

  type AccountLookup = { id: number; name: string };

  type SortColumn = "date" | "payee" | "memo" | "status" | "amount";
  type SortDir = "asc" | "desc";

  let {
    entries,
    accountId,
    accounts,
    payees,
    categories,
    commodities,
    search = $bindable(""),
    dateFrom = $bindable(""),
    dateTo = $bindable(""),
    statusFilter = $bindable(""),
    sortBy = $bindable<SortColumn>("date"),
    sortDir = $bindable<SortDir>("desc"),
    onApplyFilters,
    onOpenCreate,
    onOpenEdit,
    onUpdateStatus,
    onRemove,
  }: {
    entries: AccountRegisterEntry[];
    accountId: number;
    accounts: AccountLookup[];
    payees: PayeeSummary[];
    categories: CategorySummary[];
    commodities: CommoditySummary[];
    search?: string;
    dateFrom?: string;
    dateTo?: string;
    statusFilter?: string;
    sortBy?: SortColumn;
    sortDir?: SortDir;
    onApplyFilters: () => void | Promise<void>;
    onOpenCreate: () => void;
    onOpenEdit: (txId: number) => void;
    onUpdateStatus: (entry: AccountRegisterEntry, status: string) => void | Promise<void>;
    onRemove: (entry: AccountRegisterEntry) => void | Promise<void>;
  } = $props();

  function payeeName(id: number | null): string {
    if (!id) return "—";
    return payees.find((p) => p.id === id)?.name ?? "—";
  }

  function accountName(id: number): string {
    return accounts.find((a) => a.id === id)?.name ?? "—";
  }

  function categoryName(id: number | null): string {
    if (!id) return "—";
    return categories.find((c) => c.id === id)?.name ?? "—";
  }

  function formatMinor(amountMinor: number, commodityId: number): string {
    const commodity = commodities.find((c) => c.id === commodityId);
    if (!commodity) return String(amountMinor);
    return `${formatMinorWithScale(amountMinor, commodity.scale)} ${commodity.symbol ?? commodity.name}`;
  }

  function statusBadgeClass(status: string): string {
    if (status === "reconciled") return "badge success";
    if (status === "void") return "badge danger";
    if (status === "cleared") return "badge warning";
    return "badge";
  }

  function setSort(column: SortColumn) {
    if (sortBy === column) {
      sortDir = sortDir === "asc" ? "desc" : "asc";
    } else {
      sortBy = column;
      sortDir = column === "date" ? "desc" : "asc";
    }
  }

  const filtered = $derived(entries.filter((entry) => {
    if (statusFilter && entry.status !== statusFilter) return false;
    if (dateFrom && entry.txn_date < dateFrom) return false;
    if (dateTo && entry.txn_date > dateTo) return false;
    if (search) {
      const term = search.toLowerCase();
      const payee = payeeName(entry.payee_id).toLowerCase();
      const memo = (entry.memo ?? "").toLowerCase();
      const reference = (entry.reference ?? "").toLowerCase();
      const accountText = accountName(entry.account_id).toLowerCase();
      if (!payee.includes(term) && !memo.includes(term) && !reference.includes(term) && !accountText.includes(term)) {
        return false;
      }
    }
    return true;
  }));

  const sorted = $derived([...filtered].sort((a, b) => {
    const direction = sortDir === "asc" ? 1 : -1;
    if (sortBy === "date") return direction * a.txn_date.localeCompare(b.txn_date);
    if (sortBy === "payee") return direction * payeeName(a.payee_id).localeCompare(payeeName(b.payee_id));
    if (sortBy === "memo") return direction * (a.memo ?? "").localeCompare(b.memo ?? "");
    if (sortBy === "status") return direction * a.status.localeCompare(b.status);
    if (sortBy === "amount") return direction * (a.amount_minor - b.amount_minor);
    return 0;
  }));

  // Column resize state (local to this component)
  let tableRef: HTMLDivElement | null = $state(null);
  let txColumnWidths = $state([12, 18, 22, 12, 16, 10, 10]);
  let resizingIndex: number | null = null;
  let resizeStartX = 0;
  let resizeStartWidths: number[] = [];

  const txGridTemplate = $derived(txColumnWidths.map((w) => `${w}%`).join(" "));

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
</script>

<div class="card">
  <div class="page-row">
    <div class="page-col">
      <h2 class="section-title">Transactions</h2>
    </div>
    <div class="page-col page-col-actions">
      <Button variant="secondary" href="/accounts/{accountId}/reconcile">Reconcile</Button>
      <Button onclick={() => onOpenCreate()}>New transaction</Button>
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
      <Button variant="secondary" onclick={() => onApplyFilters()}>Apply filters</Button>
    </div>
  </div>

  <div class="data-table striped compact transaction-table" bind:this={tableRef}>
    <div class="data-row header" style={`grid-template-columns: ${txGridTemplate}`}>
      <button class="data-cell heading sort-button col-header" type="button" onclick={() => setSort("date")}>
        Date
        <span class="col-resizer" role="separator" onpointerdown={(event) => startResize(event, 0)}></span>
      </button>
      <button class="data-cell heading sort-button col-header" type="button" onclick={() => setSort("payee")}>
        Payee
        <span class="col-resizer" role="separator" onpointerdown={(event) => startResize(event, 1)}></span>
      </button>
      <button class="data-cell heading sort-button col-header" type="button" onclick={() => setSort("memo")}>
        Memo
        <span class="col-resizer" role="separator" onpointerdown={(event) => startResize(event, 2)}></span>
      </button>
      <button class="data-cell heading sort-button col-header" type="button" onclick={() => setSort("status")}>
        Status
        <span class="col-resizer" role="separator" onpointerdown={(event) => startResize(event, 3)}></span>
      </button>
      <div class="data-cell heading">Category</div>
      <button class="data-cell heading amount sort-button col-header" type="button" onclick={() => setSort("amount")}>
        Amount
        <span class="col-resizer" role="separator" onpointerdown={(event) => startResize(event, 5)}></span>
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
      {#each sorted as entry (entry.split_id)}
        <div class="data-row" style={`grid-template-columns: ${txGridTemplate}`}>
          <div class="data-cell">{entry.txn_date}</div>
          <div class="data-cell">{payeeName(entry.payee_id)}</div>
          <div class="data-cell">{entry.memo ?? "—"}</div>
          <div class="data-cell">
            <span class={statusBadgeClass(entry.status)}>{entry.status}</span>
          </div>
          <div class="data-cell">{categoryName(entry.category_id)}</div>
          <div class="data-cell amount">{formatMinor(entry.amount_minor, entry.commodity_id)}</div>
          <div class="data-cell action">
            <Button variant="ghost" size="sm" onclick={() => onOpenEdit(entry.tx_id)}>Edit</Button>
            {#if entry.status === "cleared"}
              <Button variant="ghost" size="sm" onclick={() => onUpdateStatus(entry, "uncleared")}>Unflag</Button>
            {:else}
              <Button variant="ghost" size="sm" onclick={() => onUpdateStatus(entry, "cleared")}>Flag</Button>
            {/if}
            <Button variant="ghost" size="sm" onclick={() => onUpdateStatus(entry, "void")}>Void</Button>
            <Button variant="ghost" size="sm" onclick={() => onRemove(entry)}>Delete</Button>
          </div>
        </div>
      {/each}
    {/if}
  </div>
</div>

<style>
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
    outline: 2px solid var(--ring);
    outline-offset: 2px;
  }
</style>
