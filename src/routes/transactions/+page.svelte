<script lang="ts">
  import { onMount } from "svelte";
  import { invoke } from "@tauri-apps/api/core";
  import { page } from "$app/state";
  import { goto } from "$app/navigation";
  import {
    listCategories,
    listCommodities,
    listPayees,
    listPeople,
    listProjects,
    listTags,
    type CategorySummary,
    type CommoditySummary,
    type PayeeSummary,
    type PersonSummary,
    type ProjectSummary,
    type TagSummary,
  } from "$lib/api/metadata";
  import * as Card from "$lib/components/ui/card";
  import { Button } from "$lib/components/ui/button";
  import { Input } from "$lib/components/ui/input";
  import { Label } from "$lib/components/ui/label";
  import * as Dialog from "$lib/components/ui/dialog";
  import * as Table from "$lib/components/ui/table";
  import * as Alert from "$lib/components/ui/alert";
  import { Badge } from "$lib/components/ui/badge";
  import ConfirmDialog from "$lib/components/ConfirmDialog.svelte";
  import { formatError } from "$lib/utils";

  type Account = {
    id: number;
    name: string;
    commodity_id: number;
    account_type: string;
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
    tag_id: number | null;
    person_id: number | null;
    project_id: number | null;
    share_bps: number | null;
    memo: string | null;
  };

  type TransactionWithSplits = {
    transaction: Transaction;
    splits: Split[];
  };

  type SplitDraft = {
    account_id: number | null;
    category_id: number | null;
    category_input: string;
    tag_id: number | null;
    tag_input: string;
    person_id: number | null;
    person_input: string;
    project_id: number | null;
    project_input: string;
    share_bps: number | null;
    amount: string;
    memo: string;
  };

  const bookId = 1;

  let transactions: TransactionWithSplits[] = [];
  let accounts: Account[] = [];
  let categories: CategorySummary[] = [];
  let payees: PayeeSummary[] = [];
  let tags: TagSummary[] = [];
  let people: PersonSummary[] = [];
  let projects: ProjectSummary[] = [];
  let commodities: CommoditySummary[] = [];
  let loading = true;
  let error = "";

  // Filter state (all server-side)
  let search = "";
  let dateFrom = "";
  let dateTo = "";
  let statusFilter = "";
  let accountFilterId: number | null = null;
  let amountMin = "";
  let amountMax = "";
  type FilterColumn = "date" | "payee" | "status" | "account" | "amount";
  let activeFilter: FilterColumn | null = null;
  let sortBy: "date" | "payee" | "status" | "amount" = "date";
  let sortDir: "asc" | "desc" = "desc";

  // Infinite scroll state
  const BATCH_SIZE = 50;
  let currentOffset = 0;
  let hasMore = false;
  let loadingMore = false;
  let sentinelEl: HTMLDivElement;
  let scrollObserver: IntersectionObserver | null = null;

  // Search debounce
  let searchTimeout: ReturnType<typeof setTimeout>;

  let dialogOpen = false;
  let dialogMode: "create" | "edit" = "create";
  let submitting = false;
  let formId: number | null = null;
  // formDateRaw is what the user types; formDate is the resolved ISO YYYY-MM-DD
  let formDateRaw = "";
  let formDate = "";
  let formDateHint = ""; // shown when raw != resolved
  let formAccountId: number | null = null;
  let formTransferAccountId: number | null = null;
  let formPayeeId: number | null = null;
  let formPayeeInput = "";
  let formMemo = "";
  let formReference = "";
  let formStatus = "uncleared";
  let formCategoryId: number | null = null;
  let formCategoryInput = "";
  let formAmount = "";
  let splitMode = false;
  let splitEditorOpen = false;
  let formSplits: SplitDraft[] = [];

  // Multi-select state
  let selectedIds = new Set<number>();
  let bulkActionRunning = false;

  // ─── Confirm dialog state ──────────────────────────────────────────────────
  let confirmOpen = false;
  let confirmTitle = "Are you sure?";
  let confirmMessage = "";
  let confirmLabel = "Confirm";
  let confirmDestructive = false;
  let confirmResolve: ((v: boolean) => void) | null = null;

  function askConfirm(msg: string, opts?: { title?: string; label?: string; destructive?: boolean }): Promise<boolean> {
    confirmMessage = msg;
    confirmTitle = opts?.title ?? "Are you sure?";
    confirmLabel = opts?.label ?? "Confirm";
    confirmDestructive = opts?.destructive ?? false;
    confirmOpen = true;
    return new Promise<boolean>((resolve) => { confirmResolve = resolve; });
  }
  function onConfirmYes() { confirmOpen = false; confirmResolve?.(true); confirmResolve = null; }
  function onConfirmNo() { confirmOpen = false; confirmResolve?.(false); confirmResolve = null; }

  // ─── Create category dialog state ──────────────────────────────────────────
  let createCategoryDialogOpen = false;
  let createCategoryName = "";
  let createCategoryKind = "expense";
  let createCategoryResolve: ((id: number | null) => void) | null = null;

  function askCategoryCreate(name: string): Promise<number | null> {
    createCategoryName = name;
    createCategoryKind = "expense";
    createCategoryDialogOpen = true;
    return new Promise<number | null>((resolve) => { createCategoryResolve = resolve; });
  }
  async function onCreateCategoryConfirm() {
    createCategoryDialogOpen = false;
    try {
      const created = await invoke<CategorySummary>("create_category", {
        input: { book_id: bookId, parent_id: null, name: createCategoryName, kind: createCategoryKind, color: null }
      });
      categories = [...categories, created];
      createCategoryResolve?.(created.id);
    } catch (e) {
      error = `Failed to create category: ${formatError(e)}`;
      createCategoryResolve?.(null);
    }
    createCategoryResolve = null;
  }
  function onCreateCategoryCancel() {
    createCategoryDialogOpen = false;
    createCategoryResolve?.(null);
    createCategoryResolve = null;
  }

  // ─── Unlock reason dialog state ────────────────────────────────────────────
  let unlockDialogOpen = false;
  let unlockReason = "";
  let unlockResolve: ((reason: string | null) => void) | null = null;

  function askUnlockReason(): Promise<string | null> {
    unlockReason = "";
    unlockDialogOpen = true;
    return new Promise<string | null>((resolve) => { unlockResolve = resolve; });
  }
  function onUnlockConfirm() { unlockDialogOpen = false; unlockResolve?.(unlockReason); unlockResolve = null; }
  function onUnlockCancel() { unlockDialogOpen = false; unlockResolve?.(null); unlockResolve = null; }

  $: selectedCategoryKind = formCategoryId
    ? categories.find((category) => category.id === formCategoryId)?.kind ?? null
    : null;
  $: transferRequired = selectedCategoryKind === "transfer" || !selectedCategoryKind;
  $: if (selectedCategoryKind && selectedCategoryKind !== "transfer") {
    formTransferAccountId = null;
  }

  onMount(async () => {
    await loadLookups();
    await loadTransactions();
    // Auto-open create dialog if URL has ?new=1 (e.g. from global keyboard shortcut)
    if (page.url.searchParams.get("new") === "1") {
      await goto("/transactions", { replaceState: true });
      openCreateDialog();
    }
    // sentinel must be in DOM before observing — wait a tick
    setTimeout(setupInfiniteScroll, 0);
  });

  async function loadLookups() {
    try {
      const [accountList, categoryList, payeeList, tagList, peopleList, projectList, commodityList] = await Promise.all([
        invoke<Account[]>("list_accounts", { bookId }),
        listCategories(bookId),
        listPayees(bookId),
        listTags(bookId),
        listPeople(bookId),
        listProjects(bookId),
        listCommodities(bookId)
      ]);
      accounts = accountList;
      categories = categoryList;
      payees = payeeList;
      tags = tagList;
      people = peopleList;
      projects = projectList;
      commodities = commodityList;
    } catch (e) {
      error = `Failed to load lookup data: ${formatError(e)}`;
    }
  }

  function buildFilter(offset: number) {
    // Amount range: convert decimal input to minor units (scale 2 = cents, good for single currency)
    const amountMinMinor = amountMin && Number.isFinite(Number(amountMin))
      ? Math.round(Math.abs(Number(amountMin)) * 100) : undefined;
    const amountMaxMinor = amountMax && Number.isFinite(Number(amountMax))
      ? Math.round(Math.abs(Number(amountMax)) * 100) : undefined;

    return {
      book_id: bookId,
      account_id: accountFilterId ?? undefined,
      date_from: dateFrom || undefined,
      date_to: dateTo || undefined,
      search: search || undefined,
      status: statusFilter || undefined,
      amount_min: amountMinMinor,
      amount_max: amountMaxMinor,
      sort_by: sortBy,
      sort_dir: sortDir,
      limit: BATCH_SIZE,
      offset,
    };
  }

  async function loadTransactions(append = false) {
    if (append) {
      if (!hasMore || loadingMore) return;
      loadingMore = true;
    } else {
      loading = true;
      currentOffset = 0;
      transactions = [];
    }
    error = "";
    try {
      const result = await invoke<TransactionWithSplits[]>("list_transactions", {
        filter: buildFilter(append ? currentOffset : 0),
      });
      if (append) {
        transactions = [...transactions, ...result];
      } else {
        transactions = result;
      }
      hasMore = result.length === BATCH_SIZE;
      currentOffset = (append ? currentOffset : 0) + result.length;
    } catch (e) {
      error = `Failed to load transactions: ${formatError(e)}`;
    } finally {
      loading = false;
      loadingMore = false;
    }
  }

  function setupInfiniteScroll() {
    scrollObserver?.disconnect();
    scrollObserver = new IntersectionObserver(
      (entries) => {
        if (entries[0].isIntersecting && hasMore && !loadingMore) {
          loadTransactions(true);
        }
      },
      { rootMargin: "200px" }
    );
    if (sentinelEl) scrollObserver.observe(sentinelEl);
  }

  function onSearchInput() {
    clearTimeout(searchTimeout);
    searchTimeout = setTimeout(() => loadTransactions(false), 400);
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

  function formatAmount(amountMinor: number, commodityId: number): string {
    const commodity = commodities.find((c) => c.id === commodityId);
    if (!commodity) return String(amountMinor);
    const formatted = formatMinorWithScale(amountMinor, commodity.scale);
    return `${formatted} ${commodity.symbol ?? commodity.name}`;
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

  function amountToNumber(amountMinor: number, commodityId: number): number {
    const commodity = commodities.find((c) => c.id === commodityId);
    if (!commodity || commodity.scale === 0) return amountMinor;
    return amountMinor / 10 ** commodity.scale;
  }

  function accountName(id: number) {
    return accounts.find((a) => a.id === id)?.name ?? "—";
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

  function primarySplit(tx: TransactionWithSplits, accountId: number | null) {
    if (accountId) {
      return tx.splits.find((s) => s.account_id === accountId) ?? tx.splits[0];
    }
    return tx.splits.reduce((max, current) =>
      Math.abs(current.amount_minor) > Math.abs(max.amount_minor) ? current : max
    );
  }

  function handleHeaderClick(column: FilterColumn) {
    // "account" column is filterable but not sortable server-side
    if (column !== "account") setSort(column as typeof sortBy);
    toggleFilter(column);
  }

  function toggleFilter(column: FilterColumn) {
    activeFilter = activeFilter === column ? null : column;
  }

  function hasFilter(column: FilterColumn) {
    if (column === "date") return Boolean(dateFrom || dateTo);
    if (column === "payee") return Boolean(search);
    if (column === "status") return Boolean(statusFilter);
    if (column === "account") return Boolean(accountFilterId);
    if (column === "amount") return Boolean(amountMin || amountMax);
    return false;
  }

  async function applyFilters() {
    await loadTransactions(false);
  }

  async function clearFilter(column: FilterColumn) {
    if (column === "date") { dateFrom = ""; dateTo = ""; }
    else if (column === "payee") { search = ""; }
    else if (column === "status") { statusFilter = ""; }
    else if (column === "account") { accountFilterId = null; }
    else if (column === "amount") { amountMin = ""; amountMax = ""; }
    await loadTransactions(false);
  }

  async function openCreateDialog() {
    dialogMode = "create";
    formId = null;
    formDate = new Date().toISOString().slice(0, 10);
    formDateRaw = formDate;
    formDateHint = "";
    formAccountId = accountFilterId;
    formTransferAccountId = null;
    formPayeeId = null;
    formPayeeInput = "";
    formMemo = "";
    formReference = "";
    formStatus = "uncleared";
    formCategoryId = null;
    formCategoryInput = "";
    formAmount = "";
    splitMode = false;
    splitEditorOpen = false;
    formSplits = [
      { account_id: accountFilterId, category_id: null, category_input: "", tag_id: null, tag_input: "", person_id: null, person_input: "", project_id: null, project_input: "", share_bps: null, amount: "", memo: "" },
      { account_id: null, category_id: null, category_input: "", tag_id: null, tag_input: "", person_id: null, person_input: "", project_id: null, project_input: "", share_bps: null, amount: "", memo: "" }
    ];
    dialogOpen = true;
  }

  function openEditDialog(tx: TransactionWithSplits) {
    dialogMode = "edit";
    formId = tx.transaction.id;
    formDate = tx.transaction.txn_date;
    formDateRaw = formDate;
    formDateHint = "";
    const primary = primarySplit(tx, accountFilterId);
    const transfer = tx.splits.find((s) => s.account_id !== primary.account_id) ?? tx.splits[0];
    formAccountId = primary.account_id;
    formTransferAccountId = transfer?.account_id ?? null;
    formPayeeId = tx.transaction.payee_id;
    formPayeeInput = tx.transaction.payee_id ? payeeName(tx.transaction.payee_id) : "";
    formMemo = tx.transaction.memo ?? "";
    formReference = tx.transaction.reference ?? "";
    formStatus = tx.transaction.status;
    formCategoryId = primary.category_id;
    formCategoryInput = primary.category_id ? categoryName(primary.category_id) : "";
    formAmount = formatAmountInput(primary.amount_minor, primary.commodity_id);
    splitMode = tx.splits.length > 2;
    splitEditorOpen = false;
    formSplits = tx.splits.map((split) => ({
      account_id: split.account_id,
      category_id: split.category_id,
      category_input: split.category_id ? categoryName(split.category_id) : "",
      tag_id: split.tag_id,
      tag_input: split.tag_id ? tags.find((tag) => tag.id === split.tag_id)?.name ?? "" : "",
      person_id: split.person_id,
      person_input: split.person_id ? people.find((person) => person.id === split.person_id)?.name ?? "" : "",
      project_id: split.project_id,
      project_input: split.project_id ? projects.find((project) => project.id === split.project_id)?.name ?? "" : "",
      share_bps: split.share_bps,
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
        { account_id: formAccountId, category_id: formCategoryId, category_input: formCategoryInput, tag_id: null, tag_input: "", person_id: null, person_input: "", project_id: null, project_input: "", share_bps: null, amount: formAmount, memo: "" },
        { account_id: formTransferAccountId, category_id: null, category_input: "", tag_id: null, tag_input: "", person_id: null, person_input: "", project_id: null, project_input: "", share_bps: null, amount: formAmount ? `-${formAmount}` : "", memo: "" }
      ];
    }
    splitEditorOpen = true;
  }

  function closeSplitEditor() {
    splitEditorOpen = false;
  }

  function addSplitRow() {
    formSplits = [...formSplits, { account_id: null, category_id: null, category_input: "", tag_id: null, tag_input: "", person_id: null, person_input: "", project_id: null, project_input: "", share_bps: null, amount: "", memo: "" }];
  }

  function toNullableInt(value: number | string | null | undefined): number | null {
    if (value === null || value === undefined || value === "") return null;
    const n = typeof value === "number" ? value : Number(value);
    return Number.isFinite(n) ? Math.trunc(n) : null;
  }

  function normalizeName(value: string): string {
    return value.trim().toLowerCase();
  }

  function fuzzyMatch(query: string, candidate: string): boolean {
    const q = normalizeName(query);
    if (!q) return true;
    const c = normalizeName(candidate);
    let cursor = 0;
    for (const ch of c) {
      if (ch === q[cursor]) {
        cursor += 1;
        if (cursor === q.length) return true;
      }
    }
    return false;
  }

  function fuzzyOptions<T extends { id: number; name: string }>(items: T[], query: string): T[] {
    return items.filter((item) => fuzzyMatch(query, item.name)).slice(0, 30);
  }

  function exactMatchByName<T extends { id: number; name: string }>(items: T[], query: string): T | undefined {
    const needle = normalizeName(query);
    if (!needle) return undefined;
    return items.find((item) => normalizeName(item.name) === needle);
  }

  // ─── Smart date parsing ────────────────────────────────────────────────────

  function parseSmartDate(raw: string, todayIso: string): string | null {
    const s = raw.trim();
    if (!s) return null;

    // "t" or "today"
    if (s.toLowerCase() === "t" || s.toLowerCase() === "today") return todayIso;

    // Already ISO YYYY-MM-DD or YYYY-M-D
    if (/^\d{4}-\d{1,2}-\d{1,2}$/.test(s)) {
      const [y, m, d] = s.split("-").map(Number);
      return toIso(y, m, d);
    }

    const today = new Date(todayIso + "T12:00:00");

    function toIso(y: number, m: number, d: number): string | null {
      if (m < 1 || m > 12 || d < 1 || d > 31) return null;
      const dt = new Date(y, m - 1, d);
      if (dt.getMonth() !== m - 1) return null; // overflow (e.g. Feb 30)
      return `${y}-${String(m).padStart(2, "0")}-${String(d).padStart(2, "0")}`;
    }

    function guessYear(m: number, d: number): number {
      const y = today.getFullYear();
      const candidate = new Date(y, m - 1, d);
      const diffDays = (candidate.getTime() - today.getTime()) / 86400000;
      return diffDays > 60 ? y - 1 : y;
    }

    function resolveMonthDay(a: number, b: number, year: number | null): string | null {
      let month: number, day: number;
      if (a > 12 && b <= 12) { day = a; month = b; }
      else if (b > 12 && a <= 12) { day = b; month = a; }
      else { month = a; day = b; } // ambiguous: MM/DD convention
      const y = year ?? guessYear(month, day);
      return toIso(y, month, day);
    }

    // Separator-delimited: A/B or A/B/C (also - and .)
    const sepMatch = s.match(/^(\d{1,4})[\/\-\.](\d{1,2})(?:[\/\-\.](\d{2,4}))?$/);
    if (sepMatch) {
      const [, a, b, c] = sepMatch;
      const aNum = parseInt(a, 10), bNum = parseInt(b, 10);
      if (c) {
        const cNum = parseInt(c, 10);
        if (aNum > 31) {
          // YYYY/MM/DD
          return toIso(aNum, bNum, cNum);
        } else {
          // DD/MM/YY or MM/DD/YY
          const year = cNum < 100 ? 2000 + cNum : cNum;
          return resolveMonthDay(aNum, bNum, year);
        }
      } else {
        return resolveMonthDay(aNum, bNum, null);
      }
    }

    // No separator: 1–4 digits
    if (/^\d{1,4}$/.test(s)) {
      const n = parseInt(s, 10);
      if (s.length <= 2) {
        // Day only: use current month, step back if more than 2 days in future
        const d = n;
        if (d < 1 || d > 31) return null;
        let m = today.getMonth() + 1;
        let y = today.getFullYear();
        const candidate = new Date(y, m - 1, d);
        if (isNaN(candidate.getTime()) || candidate.getMonth() !== m - 1) {
          m -= 1; if (m === 0) { m = 12; y -= 1; }
        } else {
          const diff = (candidate.getTime() - today.getTime()) / 86400000;
          if (diff > 2) { m -= 1; if (m === 0) { m = 12; y -= 1; } }
        }
        return toIso(y, m, d);
      } else if (s.length === 3) {
        // MDD: first digit = month, last two = day
        return resolveMonthDay(parseInt(s[0], 10), parseInt(s.slice(1), 10), null);
      } else {
        // MMDD
        return resolveMonthDay(parseInt(s.slice(0, 2), 10), parseInt(s.slice(2), 10), null);
      }
    }

    return null;
  }

  function onDateInput(value: string) {
    formDateRaw = value;
    const todayIso = new Date().toISOString().slice(0, 10);
    const resolved = parseSmartDate(value, todayIso);
    if (resolved) {
      formDate = resolved;
      formDateHint = resolved !== value.trim() ? resolved : "";
    } else {
      formDate = "";
      formDateHint = "";
    }
  }

  function onDateBlur() {
    // On blur, if parsed, replace raw with the canonical ISO form
    if (formDate) {
      formDateRaw = formDate;
      formDateHint = "";
    }
  }

  // ─── Payee auto-fill ───────────────────────────────────────────────────────

  async function syncTopLevelInput(kind: "payee" | "category", value: string) {
    if (kind === "payee") {
      formPayeeInput = value;
      const matched = exactMatchByName(payees, value);
      formPayeeId = matched?.id ?? null;
      // When we get an exact payee match, auto-fill category + memo from last transaction
      if (matched) {
        try {
          const defaults = await invoke<{ category_id: number | null; memo: string | null }>(
            "get_payee_defaults",
            { payeeId: matched.id, accountId: formAccountId ?? undefined }
          );
          if (defaults.category_id && !formCategoryId) {
            formCategoryId = defaults.category_id;
            formCategoryInput = categoryName(defaults.category_id);
          }
          if (defaults.memo && !formMemo) {
            formMemo = defaults.memo;
          }
        } catch {
          // ignore auto-fill errors silently
        }
      }
      return;
    }
    formCategoryInput = value;
    formCategoryId = exactMatchByName(categories, value)?.id ?? null;
  }

  function syncSplitInput(
    split: SplitDraft,
    field: "category" | "tag" | "person" | "project",
    value: string
  ) {
    if (field === "category") {
      split.category_input = value;
      split.category_id = exactMatchByName(categories, value)?.id ?? null;
    } else if (field === "tag") {
      split.tag_input = value;
      split.tag_id = exactMatchByName(tags, value)?.id ?? null;
    } else if (field === "person") {
      split.person_input = value;
      split.person_id = exactMatchByName(people, value)?.id ?? null;
    } else {
      split.project_input = value;
      split.project_id = exactMatchByName(projects, value)?.id ?? null;
    }
    formSplits = [...formSplits];
  }

  async function ensureEntityId(
    kind: "payee" | "category" | "tag" | "person" | "project",
    value: string,
    currentId: number | null
  ): Promise<number | null> {
    const trimmed = value.trim();
    if (!trimmed) return null;
    if (currentId) return currentId;

    if (kind === "payee") {
      const existing = exactMatchByName(payees, trimmed);
      if (existing) return existing.id;
      if (!await askConfirm(`Create new payee "${trimmed}"?`, { label: "Create" })) throw new Error("Payee creation cancelled");
      const created = await invoke<PayeeSummary>("create_payee", {
        input: { book_id: bookId, name: trimmed, kind: "person", metadata: null }
      });
      payees = [...payees, created];
      return created.id;
    }

    if (kind === "category") {
      const existing = exactMatchByName(categories, trimmed);
      if (existing) return existing.id;
      const id = await askCategoryCreate(trimmed);
      if (id === null) throw new Error("Category creation cancelled");
      return id;
    }

    if (kind === "tag") {
      const existing = exactMatchByName(tags, trimmed);
      if (existing) return existing.id;
      if (!await askConfirm(`Create new tag "${trimmed}"?`, { label: "Create" })) throw new Error("Tag creation cancelled");
      const created = await invoke<TagSummary>("create_tag", {
        input: { book_id: bookId, name: trimmed, color: null }
      });
      tags = [...tags, created];
      return created.id;
    }

    if (kind === "person") {
      const existing = exactMatchByName(people, trimmed);
      if (existing) return existing.id;
      if (!await askConfirm(`Create new person "${trimmed}"?`, { label: "Create" })) throw new Error("Person creation cancelled");
      const created = await invoke<PersonSummary>("create_person", {
        input: { book_id: bookId, name: trimmed, role: "member", metadata: null }
      });
      people = [...people, created];
      return created.id;
    }

    const existing = exactMatchByName(projects, trimmed);
    if (existing) return existing.id;
    if (!await askConfirm(`Create new project "${trimmed}"?`, { label: "Create" })) throw new Error("Project creation cancelled");
    const created = await invoke<ProjectSummary>("create_project", {
      input: { book_id: bookId, name: trimmed, status: "active", metadata: null }
    });
    projects = [...projects, created];
    return created.id;
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
        book_id: bookId,
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
    const confirm = await askConfirm(
      "Unlocking will void balance checks from this date forward for affected accounts.",
      { title: "Unlock Reconciled Transaction", label: "Unlock", destructive: true }
    );
    if (!confirm) return;
    const reason = (await askUnlockReason()) ?? "";
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
      return Promise.all(formSplits.map(async (split) => {
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
        const category_id = await ensureEntityId("category", split.category_input, split.category_id);
        const tag_id = await ensureEntityId("tag", split.tag_input, split.tag_id);
        const person_id = await ensureEntityId("person", split.person_input, split.person_id);
        const project_id = await ensureEntityId("project", split.project_input, split.project_id);
        return {
          account_id: split.account_id,
          commodity_id: account.commodity_id,
          amount_minor,
          category_id,
          tag_id: toNullableInt(tag_id),
          person_id: toNullableInt(person_id),
          project_id: toNullableInt(project_id),
          share_bps: toNullableInt(split.share_bps),
          memo: split.memo || null
        };
      }));
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
    const formCategoryResolved = await ensureEntityId("category", formCategoryInput, formCategoryId);
    formCategoryId = formCategoryResolved;
    const kind = categoryKind(formCategoryResolved);
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
        category_id: formCategoryResolved,
        tag_id: null,
        person_id: null,
        project_id: null,
        share_bps: null,
        memo: null
      },
      {
        account_id: transferAccount.id,
        commodity_id: transferAccount.commodity_id,
        amount_minor: -amount_minor,
        category_id: null,
        tag_id: null,
        person_id: null,
        project_id: null,
        share_bps: null,
        memo: null
      }
    ];
  }

  async function submitTransaction() {
    submitting = true;
    error = "";
    try {
      if (!formDate) {
        throw new Error("Date is required and must be a valid date");
      }
      const payeeId = await ensureEntityId("payee", formPayeeInput, formPayeeId);
      formPayeeId = payeeId;
      const splits = await buildSplitsForSubmit();

      const total = splits.reduce((sum, s) => sum + s.amount_minor, 0);
      if (total !== 0) {
        throw new Error("Splits must balance to zero");
      }

      if (dialogMode === "create") {
        await invoke("create_transaction_with_splits", {
          input: {
            book_id: bookId,
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
            book_id: bookId,
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
      error = `Failed to save transaction: ${formatError(e)}`;
    } finally {
      submitting = false;
    }
  }

  async function updateStatus(tx: TransactionWithSplits, status: string) {
    const action = async () => {
      await invoke("update_transaction_with_splits", {
        input: {
          id: tx.transaction.id,
          book_id: bookId,
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
            tag_id: split.tag_id,
            person_id: split.person_id,
            project_id: split.project_id,
            share_bps: split.share_bps,
            memo: split.memo
          }))
        }
      });
    };

    try {
      await action();
      await loadTransactions();
    } catch (e) {
      const msg = formatError(e);
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
    const confirmed = await askConfirm("Delete this transaction? This cannot be undone.", { label: "Delete", destructive: true });
    if (!confirmed) return;

    const action = async () => {
      await invoke("delete_transaction", { id: tx.transaction.id });
    };

    try {
      await action();
      await loadTransactions();
    } catch (e) {
      const msg = formatError(e);
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

  function setSort(column: typeof sortBy) {
    if (sortBy === column) {
      sortDir = sortDir === "asc" ? "desc" : "asc";
    } else {
      sortBy = column;
      sortDir = column === "date" ? "desc" : "asc";
    }
    loadTransactions(false);
  }

  // ─── Duplicate transaction ─────────────────────────────────────────────────

  async function duplicateTransaction(tx: TransactionWithSplits) {
    const today = new Date().toISOString().slice(0, 10);
    try {
      await invoke("duplicate_transaction", { id: tx.transaction.id, today });
      await loadTransactions();
    } catch (e) {
      error = `Failed to duplicate: ${formatError(e)}`;
    }
  }

  // ─── Multi-select helpers ──────────────────────────────────────────────────

  function toggleSelect(id: number) {
    if (selectedIds.has(id)) selectedIds.delete(id);
    else selectedIds.add(id);
    selectedIds = new Set(selectedIds); // trigger reactivity
  }

  function toggleSelectAll() {
    if (selectedIds.size === displayedTx.length && displayedTx.length > 0) {
      selectedIds = new Set();
    } else {
      selectedIds = new Set(displayedTx.map((t) => t.transaction.id));
    }
  }

  function clearSelection() {
    selectedIds = new Set();
  }

  // ─── Bulk operations ───────────────────────────────────────────────────────

  async function bulkVoid() {
    if (selectedIds.size === 0) return;
    const confirmed = await askConfirm(`Void ${selectedIds.size} selected transaction(s)?`, { label: "Void", destructive: true });
    if (!confirmed) return;
    bulkActionRunning = true;
    error = "";
    try {
      const voided = await invoke<number>("bulk_void_transactions", { ids: Array.from(selectedIds) });
      clearSelection();
      await loadTransactions();
      if (voided < selectedIds.size) {
        // Some were skipped (locked)
      }
    } catch (e) {
      error = `Bulk void failed: ${formatError(e)}`;
    } finally {
      bulkActionRunning = false;
    }
  }

  async function bulkDelete() {
    if (selectedIds.size === 0) return;
    const confirmed = await askConfirm(`Permanently delete ${selectedIds.size} selected transaction(s)? This cannot be undone.`, { label: "Delete", destructive: true });
    if (!confirmed) return;
    bulkActionRunning = true;
    error = "";
    try {
      await invoke<number>("bulk_delete_transactions", { ids: Array.from(selectedIds) });
      clearSelection();
      await loadTransactions();
    } catch (e) {
      error = `Bulk delete failed: ${formatError(e)}`;
    } finally {
      bulkActionRunning = false;
    }
  }

  // Transactions are sorted and filtered server-side; just use the list directly.
  $: displayedTx = transactions;
</script>

<main class="py-6">
  <div class="container mx-auto px-6 space-y-6">
    <div class="flex items-center justify-between">
      <div>
        <h1 class="text-3xl font-bold tracking-tight">Transactions</h1>
        <p class="text-muted-foreground">Review and enter transactions.</p>
      </div>
      <Button onclick={openCreateDialog}>New transaction</Button>
    </div>

    {#if error}
      <Alert.Root variant="destructive">
        <Alert.Title>Error</Alert.Title>
        <Alert.Description>{error}</Alert.Description>
      </Alert.Root>
    {/if}

    <Card.Root>
      <Card.Content class="pt-6">
        <!-- Search bar (always visible) -->
        <div class="mb-4">
          <Input
            type="search"
            placeholder="Search payee, memo, reference…"
            bind:value={search}
            oninput={onSearchInput}
            class="max-w-sm"
          />
        </div>

        {#if loading}
          <p class="text-sm text-muted-foreground">Loading transactions…</p>
        {:else}
          <!-- Column filter panels (date, status, account, amount) -->
          {#if activeFilter}
            <div class="mb-4 p-4 bg-muted/50 rounded-lg border">
              {#if activeFilter === "date"}
                <div class="flex flex-wrap items-end gap-4">
                  <div class="space-y-1">
                    <Label for="tx-date-from">From</Label>
                    <Input id="tx-date-from" type="date" bind:value={dateFrom} class="w-40" />
                  </div>
                  <div class="space-y-1">
                    <Label for="tx-date-to">To</Label>
                    <Input id="tx-date-to" type="date" bind:value={dateTo} class="w-40" />
                  </div>
                  <div class="flex gap-2">
                    <Button variant="secondary" size="sm" onclick={applyFilters}>Apply</Button>
                    <Button variant="ghost" size="sm" onclick={() => clearFilter("date")}>Clear</Button>
                  </div>
                </div>
              {:else if activeFilter === "payee"}
                <div class="flex flex-wrap items-end gap-4">
                  <div class="space-y-1 flex-1 max-w-xs">
                    <Label for="tx-payee-search">Search payee / memo</Label>
                    <Input id="tx-payee-search" placeholder="Type to search…" bind:value={search} oninput={onSearchInput} />
                  </div>
                  <div class="flex gap-2">
                    <Button variant="ghost" size="sm" onclick={() => clearFilter("payee")}>Clear</Button>
                  </div>
                </div>
              {:else if activeFilter === "status"}
                <div class="flex flex-wrap items-end gap-4">
                  <div class="space-y-1">
                    <Label for="tx-status-filter">Status</Label>
                    <select id="tx-status-filter" class="flex h-9 rounded-md border border-input bg-transparent px-3 py-1 text-sm" bind:value={statusFilter} onchange={applyFilters}>
                      <option value="">Active (non-void)</option>
                      <option value="uncleared">Uncleared</option>
                      <option value="cleared">Cleared</option>
                      <option value="reconciled">Reconciled</option>
                      <option value="void">Void only</option>
                      <option value="all">All (including void)</option>
                    </select>
                  </div>
                  <Button variant="ghost" size="sm" onclick={() => clearFilter("status")}>Clear</Button>
                </div>
              {:else if activeFilter === "account"}
                <div class="flex flex-wrap items-end gap-4">
                  <div class="space-y-1">
                    <Label for="tx-account-filter">Account</Label>
                    <select id="tx-account-filter" class="flex h-9 rounded-md border border-input bg-transparent px-3 py-1 text-sm" bind:value={accountFilterId} onchange={applyFilters}>
                      <option value={null}>All accounts</option>
                      {#each accounts as account}
                        <option value={account.id}>{account.name}</option>
                      {/each}
                    </select>
                  </div>
                  <Button variant="ghost" size="sm" onclick={() => clearFilter("account")}>Clear</Button>
                </div>
              {:else if activeFilter === "amount"}
                <div class="flex flex-wrap items-end gap-4">
                  <div class="space-y-1">
                    <Label for="tx-amount-min">Min amount</Label>
                    <Input id="tx-amount-min" type="number" step="0.01" placeholder="0.00" bind:value={amountMin} class="w-32" />
                  </div>
                  <div class="space-y-1">
                    <Label for="tx-amount-max">Max amount</Label>
                    <Input id="tx-amount-max" type="number" step="0.01" placeholder="0.00" bind:value={amountMax} class="w-32" />
                  </div>
                  <div class="flex gap-2">
                    <Button variant="secondary" size="sm" onclick={applyFilters}>Apply</Button>
                    <Button variant="ghost" size="sm" onclick={() => clearFilter("amount")}>Clear</Button>
                  </div>
                </div>
              {/if}
            </div>
          {/if}

          <!-- Bulk action toolbar -->
          {#if selectedIds.size > 0}
            <div class="mb-3 flex items-center gap-3 p-2 bg-primary/5 border rounded-lg">
              <span class="text-sm font-medium">{selectedIds.size} selected</span>
              <Button variant="outline" size="sm" onclick={bulkVoid} disabled={bulkActionRunning}>Void all</Button>
              <Button variant="outline" size="sm" class="text-destructive" onclick={bulkDelete} disabled={bulkActionRunning}>Delete all</Button>
              <Button variant="ghost" size="sm" onclick={clearSelection}>Clear selection</Button>
            </div>
          {/if}

          <div class="rounded-md border">
            <Table.Root>
              <Table.Header>
                <Table.Row>
                  <Table.Head class="w-8">
                    <input
                      type="checkbox"
                      class="cursor-pointer"
                      checked={selectedIds.size === displayedTx.length && displayedTx.length > 0}
                      indeterminate={selectedIds.size > 0 && selectedIds.size < displayedTx.length}
                      onchange={toggleSelectAll}
                    />
                  </Table.Head>
                  <Table.Head class="cursor-pointer hover:bg-muted/50 w-28" onclick={() => handleHeaderClick("date")}>
                    <span class="flex items-center gap-1">
                      {#if hasFilter("date")}<span class="text-primary">⏷</span>{/if}
                      Date
                      {#if sortBy === "date"}<span class="text-xs">{sortDir === "asc" ? "↑" : "↓"}</span>{/if}
                    </span>
                  </Table.Head>
                  <Table.Head class="cursor-pointer hover:bg-muted/50" onclick={() => handleHeaderClick("payee")}>
                    <span class="flex items-center gap-1">
                      {#if hasFilter("payee")}<span class="text-primary">⏷</span>{/if}
                      Payee / Memo
                      {#if sortBy === "payee"}<span class="text-xs">{sortDir === "asc" ? "↑" : "↓"}</span>{/if}
                    </span>
                  </Table.Head>
                  <Table.Head class="cursor-pointer hover:bg-muted/50 w-28" onclick={() => handleHeaderClick("status")}>
                    <span class="flex items-center gap-1">
                      {#if hasFilter("status")}<span class="text-primary">⏷</span>{/if}
                      Status
                      {#if sortBy === "status"}<span class="text-xs">{sortDir === "asc" ? "↑" : "↓"}</span>{/if}
                    </span>
                  </Table.Head>
                  <Table.Head class="cursor-pointer hover:bg-muted/50" onclick={() => handleHeaderClick("account")}>
                    <span class="flex items-center gap-1">
                      {#if hasFilter("account")}<span class="text-primary">⏷</span>{/if}
                      Account
                    </span>
                  </Table.Head>
                  <Table.Head class="text-right cursor-pointer hover:bg-muted/50" onclick={() => handleHeaderClick("amount")}>
                    <span class="flex items-center justify-end gap-1">
                      {#if hasFilter("amount")}<span class="text-primary">⏷</span>{/if}
                      Amount
                      {#if sortBy === "amount"}<span class="text-xs">{sortDir === "asc" ? "↑" : "↓"}</span>{/if}
                    </span>
                  </Table.Head>
                  <Table.Head class="text-right w-44">Actions</Table.Head>
                </Table.Row>
              </Table.Header>
              <Table.Body>
                {#if displayedTx.length === 0}
                  <Table.Row>
                    <Table.Cell colspan={6} class="text-center text-muted-foreground py-8">
                      No transactions found.
                    </Table.Cell>
                  </Table.Row>
                {:else}
                  {#each displayedTx as tx (tx.transaction.id)}
                    <Table.Row class="hover:bg-muted/50 {selectedIds.has(tx.transaction.id) ? 'bg-muted/30' : ''}">
                      <Table.Cell class="w-8">
                        <input
                          type="checkbox"
                          class="cursor-pointer"
                          checked={selectedIds.has(tx.transaction.id)}
                          onchange={() => toggleSelect(tx.transaction.id)}
                        />
                      </Table.Cell>
                      <Table.Cell class="font-mono text-sm">{tx.transaction.txn_date}</Table.Cell>
                      <Table.Cell>
                        <div class="leading-snug">
                          <span class="font-medium">{payeeName(tx.transaction.payee_id)}</span>
                          {#if tx.transaction.memo}
                            <span class="block text-xs text-muted-foreground">{tx.transaction.memo}</span>
                          {/if}
                        </div>
                      </Table.Cell>
                      <Table.Cell>
                        <Badge variant={tx.transaction.status === "reconciled" ? "default" : tx.transaction.status === "cleared" ? "secondary" : tx.transaction.status === "void" ? "destructive" : "outline"}>
                          {tx.transaction.status}
                        </Badge>
                      </Table.Cell>
                      <Table.Cell class="text-sm">{accountName(primarySplit(tx, accountFilterId).account_id)}</Table.Cell>
                      <Table.Cell class="text-right font-mono {primarySplit(tx, accountFilterId).amount_minor < 0 ? 'text-red-600' : 'text-green-600'}">
                        {formatAmount(primarySplit(tx, accountFilterId).amount_minor, primarySplit(tx, accountFilterId).commodity_id)}
                      </Table.Cell>
                      <Table.Cell class="text-right">
                        <div class="flex justify-end gap-1">
                          <Button variant="ghost" size="sm" onclick={() => openEditDialog(tx)}>Edit</Button>
                          <Button variant="ghost" size="sm" onclick={() => duplicateTransaction(tx)}>Dup</Button>
                          {#if tx.transaction.status === "cleared"}
                            <Button variant="ghost" size="sm" onclick={() => updateStatus(tx, "uncleared")}>Unflag</Button>
                          {:else}
                            <Button variant="ghost" size="sm" onclick={() => updateStatus(tx, "cleared")}>Flag</Button>
                          {/if}
                          <Button variant="ghost" size="sm" onclick={() => updateStatus(tx, "void")}>Void</Button>
                          <Button variant="ghost" size="sm" class="text-destructive" onclick={() => removeTransaction(tx)}>Del</Button>
                        </div>
                      </Table.Cell>
                    </Table.Row>
                  {/each}
                {/if}
              </Table.Body>
            </Table.Root>
          </div>

          <!-- Infinite scroll sentinel -->
          <div bind:this={sentinelEl} class="py-2 text-center text-sm text-muted-foreground">
            {#if loadingMore}
              <span>Loading more…</span>
            {:else if !hasMore && displayedTx.length > 0}
              <span class="text-xs opacity-50">{displayedTx.length} transaction{displayedTx.length === 1 ? "" : "s"}</span>
            {/if}
          </div>
        {/if}
      </Card.Content>
      </Card.Root>
  </div>

  <Dialog.Root bind:open={dialogOpen}>
    <Dialog.Content class="max-w-2xl max-h-[90vh] overflow-y-auto">
      <form onsubmit={(e) => { e.preventDefault(); submitTransaction(); }}>
        <Dialog.Header>
          <Dialog.Title>
            {dialogMode === "create" ? "New transaction" : "Edit transaction"}
          </Dialog.Title>
        </Dialog.Header>

        <div class="space-y-4 py-4">
          <div class="space-y-2">
            <Label for="tx-date">Date</Label>
            <Input
              id="tx-date"
              type="text"
              placeholder="YYYY-MM-DD, 18, 3/18, today…"
              value={formDateRaw}
              oninput={(e) => onDateInput((e.currentTarget as HTMLInputElement).value)}
              onblur={onDateBlur}
              class={!formDate && formDateRaw ? "border-red-500" : ""}
              required
            />
            {#if formDateHint}
              <p class="text-xs text-muted-foreground">→ {formDateHint}</p>
            {:else if !formDate && formDateRaw}
              <p class="text-xs text-red-500">Date not recognized</p>
            {/if}
          </div>

          <div class="space-y-2">
            <Label for="tx-account">Account</Label>
            <select id="tx-account" class="flex h-9 w-full rounded-md border border-input bg-transparent px-3 py-1 text-sm shadow-sm transition-colors" bind:value={formAccountId} disabled={splitMode}>
              <option value="">Select account</option>
              {#each accounts as account}
                <option value={account.id}>{account.name}</option>
              {/each}
            </select>
          </div>

          <div class="space-y-2">
            <Label for="tx-transfer-account">Transfer account</Label>
            {#if transferRequired}
              <select id="tx-transfer-account" class="flex h-9 w-full rounded-md border border-input bg-transparent px-3 py-1 text-sm shadow-sm transition-colors" bind:value={formTransferAccountId} disabled={splitMode}>
                <option value="">Select account</option>
                {#each accounts as account}
                  <option value={account.id}>{account.name}</option>
                {/each}
              </select>
            {:else}
              <p class="text-sm text-muted-foreground">Transfer account not required for non-transfer categories.</p>
            {/if}
          </div>

          <div class="space-y-2">
            <Label for="tx-payee">Payee</Label>
            <Input
              id="tx-payee"
              list="tx-payee-options"
              placeholder="Search or enter payee"
              value={formPayeeInput}
              oninput={(event) => syncTopLevelInput("payee", (event.currentTarget as HTMLInputElement).value)}
            />
            <datalist id="tx-payee-options">
              {#each fuzzyOptions(payees, formPayeeInput) as payee}
                <option value={payee.name}></option>
              {/each}
            </datalist>
          </div>

          <div class="space-y-2">
            <Label for="tx-memo">Memo</Label>
            <Input id="tx-memo" bind:value={formMemo} />
          </div>

          <div class="space-y-2">
            <Label for="tx-ref">Reference</Label>
            <Input id="tx-ref" bind:value={formReference} />
          </div>

          <div class="space-y-2">
            <Label for="tx-category">Category</Label>
            <Input
              id="tx-category"
              list="tx-category-options"
              placeholder="Search or enter category"
              value={formCategoryInput}
              oninput={(event) => syncTopLevelInput("category", (event.currentTarget as HTMLInputElement).value)}
              disabled={splitMode}
            />
            <datalist id="tx-category-options">
              {#each fuzzyOptions(categories, formCategoryInput) as category}
                <option value={category.name}></option>
              {/each}
            </datalist>
          </div>

          <div class="space-y-2">
            <Label for="tx-amount">Amount</Label>
            <Input id="tx-amount" placeholder="0.00" bind:value={formAmount} disabled={splitMode} />
          </div>

          <div class="space-y-2">
            <Label for="tx-status-select">Status</Label>
            <select id="tx-status-select" class="flex h-9 w-full rounded-md border border-input bg-transparent px-3 py-1 text-sm shadow-sm transition-colors" bind:value={formStatus}>
              <option value="uncleared">Uncleared</option>
              <option value="cleared">Cleared</option>
              <option value="reconciled">Reconciled</option>
              <option value="void">Void</option>
            </select>
          </div>
          <div class="space-y-2">
            <Button variant="secondary" type="button" onclick={openSplitEditor}>Split transaction…</Button>
            {#if splitMode}
              <p class="text-sm text-muted-foreground">Split mode enabled.</p>
            {/if}
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

  <Dialog.Root bind:open={splitEditorOpen}>
    <Dialog.Content class="max-w-4xl">
      <Dialog.Header>
        <Dialog.Title>Split transaction</Dialog.Title>
      </Dialog.Header>
      <div class="space-y-4 py-4">
        <div class="space-y-2">
          <Label>Splits</Label>
          <Table.Root>
            <Table.Header>
              <Table.Row>
                <Table.Head>Account</Table.Head>
                <Table.Head>Category</Table.Head>
                <Table.Head>Tag</Table.Head>
                <Table.Head>Person</Table.Head>
                <Table.Head>Project</Table.Head>
                <Table.Head>Share (bps)</Table.Head>
                <Table.Head class="text-right">Amount</Table.Head>
                <Table.Head>Memo</Table.Head>
                <Table.Head class="w-24">Action</Table.Head>
              </Table.Row>
            </Table.Header>
            <Table.Body>
              {#each formSplits as split, idx}
                <Table.Row>
                  <Table.Cell>
                    <select class="flex h-9 w-full rounded-md border border-input bg-transparent px-3 py-1 text-sm" bind:value={split.account_id}>
                      <option value="">Select account</option>
                      {#each accounts as account}
                        <option value={account.id}>{account.name}</option>
                      {/each}
                    </select>
                  </Table.Cell>
                  <Table.Cell>
                    <Input
                      list={`split-category-options-${idx}`}
                      placeholder="Search/enter category"
                      value={split.category_input}
                      oninput={(event) => syncSplitInput(split, "category", (event.currentTarget as HTMLInputElement).value)}
                    />
                    <datalist id={`split-category-options-${idx}`}>
                      {#each fuzzyOptions(categories, split.category_input) as category}
                        <option value={category.name}></option>
                      {/each}
                    </datalist>
                  </Table.Cell>
                  <Table.Cell>
                    <Input
                      list={`split-tag-options-${idx}`}
                      placeholder="Search/enter tag"
                      value={split.tag_input}
                      oninput={(event) => syncSplitInput(split, "tag", (event.currentTarget as HTMLInputElement).value)}
                    />
                    <datalist id={`split-tag-options-${idx}`}>
                      {#each fuzzyOptions(tags, split.tag_input) as tag}
                        <option value={tag.name}></option>
                      {/each}
                    </datalist>
                  </Table.Cell>
                  <Table.Cell>
                    <Input
                      list={`split-person-options-${idx}`}
                      placeholder="Search/enter person"
                      value={split.person_input}
                      oninput={(event) => syncSplitInput(split, "person", (event.currentTarget as HTMLInputElement).value)}
                    />
                    <datalist id={`split-person-options-${idx}`}>
                      {#each fuzzyOptions(people, split.person_input) as person}
                        <option value={person.name}></option>
                      {/each}
                    </datalist>
                  </Table.Cell>
                  <Table.Cell>
                    <Input
                      list={`split-project-options-${idx}`}
                      placeholder="Search/enter project"
                      value={split.project_input}
                      oninput={(event) => syncSplitInput(split, "project", (event.currentTarget as HTMLInputElement).value)}
                    />
                    <datalist id={`split-project-options-${idx}`}>
                      {#each fuzzyOptions(projects, split.project_input) as project}
                        <option value={project.name}></option>
                      {/each}
                    </datalist>
                  </Table.Cell>
                  <Table.Cell>
                    <Input type="number" bind:value={split.share_bps} placeholder="e.g. 5000" class="w-28" />
                  </Table.Cell>
                  <Table.Cell class="text-right">
                    <Input bind:value={split.amount} placeholder="0.00" class="w-28 text-right" />
                  </Table.Cell>
                  <Table.Cell>
                    <Input bind:value={split.memo} placeholder="Split memo" />
                  </Table.Cell>
                  <Table.Cell>
                    <Button variant="ghost" size="sm" onclick={() => removeSplitRow(idx)}>
                      Remove
                    </Button>
                  </Table.Cell>
                </Table.Row>
              {/each}
            </Table.Body>
          </Table.Root>
          <Button variant="outline" size="sm" onclick={addSplitRow}>Add split</Button>
          {#if splitsTotalMinor() !== null}
            {@const total = splitsTotalMinor()!}
            {@const balanced = total === 0}
            <div class="flex items-center gap-2 mt-2">
              <span class="text-sm font-medium">Balance:</span>
              <span class="text-sm font-mono {balanced ? 'text-green-600' : 'text-red-600'}">
                {#if total === 0}
                  ✓ Balanced
                {:else}
                  {@const firstAccount = accounts.find((a) => a.id === formSplits[0]?.account_id)}
                  {@const firstCommodity = commodities.find((c) => c.id === firstAccount?.commodity_id)}
                  {@const scale = firstCommodity?.scale ?? 2}
                  {total > 0 ? "+" : ""}{formatMinorWithScale(total, scale)} {firstCommodity?.symbol ?? ""} (unbalanced)
                {/if}
              </span>
            </div>
          {/if}
        </div>
      </div>
      <Dialog.Footer>
        <Button variant="secondary" onclick={closeSplitEditor}>Done</Button>
      </Dialog.Footer>
    </Dialog.Content>
  </Dialog.Root>

  <!-- Generic confirm dialog -->
  <ConfirmDialog
    bind:open={confirmOpen}
    title={confirmTitle}
    message={confirmMessage}
    confirmLabel={confirmLabel}
    destructive={confirmDestructive}
    onConfirm={onConfirmYes}
    onCancel={onConfirmNo}
  />

  <!-- Create category dialog -->
  <Dialog.Root bind:open={createCategoryDialogOpen}>
    <Dialog.Content class="max-w-sm">
      <Dialog.Header>
        <Dialog.Title>Create Category</Dialog.Title>
        <Dialog.Description>New category: <strong>{createCategoryName}</strong></Dialog.Description>
      </Dialog.Header>
      <div class="py-2 space-y-2">
        <Label for="new-category-kind">Kind</Label>
        <select
          id="new-category-kind"
          bind:value={createCategoryKind}
          class="flex h-9 w-full rounded-md border border-input bg-transparent px-3 py-1 text-sm shadow-xs focus:outline-none focus:ring-1 focus:ring-ring"
        >
          <option value="expense">Expense</option>
          <option value="income">Income</option>
          <option value="transfer">Transfer</option>
          <option value="other">Other</option>
        </select>
      </div>
      <Dialog.Footer>
        <Button variant="outline" onclick={onCreateCategoryCancel}>Cancel</Button>
        <Button onclick={onCreateCategoryConfirm}>Create</Button>
      </Dialog.Footer>
    </Dialog.Content>
  </Dialog.Root>

  <!-- Unlock transaction dialog -->
  <Dialog.Root bind:open={unlockDialogOpen}>
    <Dialog.Content class="max-w-md">
      <Dialog.Header>
        <Dialog.Title>Unlock Reason</Dialog.Title>
        <Dialog.Description>Optional reason for unlocking this transaction.</Dialog.Description>
      </Dialog.Header>
      <div class="py-2">
        <Input bind:value={unlockReason} placeholder="Reason (optional)" />
      </div>
      <Dialog.Footer>
        <Button variant="outline" onclick={onUnlockCancel}>Cancel</Button>
        <Button variant="destructive" onclick={onUnlockConfirm}>Unlock</Button>
      </Dialog.Footer>
    </Dialog.Content>
  </Dialog.Root>
</main>