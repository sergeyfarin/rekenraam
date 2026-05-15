<script lang="ts">
  import { onMount } from "svelte";
  import { page } from "$app/state";
  import { goto } from "$app/navigation";
  import { createAccount, listAccounts, unlockAccountBalancings } from "$lib/api/accounts";
  import {
    createCategory,
    listCategories,
    listCommodities,
    createPayee,
    listPayees,
    createPerson,
    listPeople,
    createProject,
    listProjects,
    createTag,
    listTags,
    type CategorySummary,
    type CommoditySummary,
    type PayeeSummary,
    type PersonSummary,
    type ProjectSummary,
    type TagSummary,
  } from "$lib/api/metadata";
  import {
    bulkDeleteTransactions,
    bulkVoidTransactions,
    createTransaction,
    deleteTransaction,
    duplicateTransaction as duplicateTransactionCommand,
    getPayeeDefaults,
    listTransactions as fetchTransactions,
    updateTransaction,
  } from "$lib/api/transactions";
  import { createTransactionView, listTransactionViews, type TransactionSavedView } from "$lib/api/search";
  import {
    listTransactionTemplates,
    postTransactionTemplate,
    type TransactionTemplate,
  } from "$lib/api/templates";
  import * as Card from "$lib/components/ui/card";
  import { Button } from "$lib/components/ui/button";
  import { Input } from "$lib/components/ui/input";
  import { Label } from "$lib/components/ui/label";
  import * as Dialog from "$lib/components/ui/dialog";
  import * as Table from "$lib/components/ui/table";
  import * as Alert from "$lib/components/ui/alert";
  import ConfirmDialog from "$lib/components/ConfirmDialog.svelte";
  import { formatError } from "$lib/utils";
  import { formatMinorWithScale, parseAmountToMinor } from "$lib/money";
  import { parseSmartDate } from "$lib/dates";
  import { exactMatchByName, fuzzyOptions } from "$lib/search/fuzzy";
  import { applyViewToState, buildFilterFromState, type FilterColumn, type FilterFormState } from "$lib/transactions/saved-views";
  import { emptySplitDraft, type SplitDraft } from "$lib/transactions/split-draft";
  import TransactionSplitEditor from "$lib/components/TransactionSplitEditor.svelte";
  import TransactionFilters from "$lib/components/TransactionFilters.svelte";
  import SavedViewsBar from "$lib/components/SavedViewsBar.svelte";
  import TransactionRow from "$lib/components/TransactionRow.svelte";
  import CrossCurrencyTransferDialog from "$lib/components/CrossCurrencyTransferDialog.svelte";
  import { validateIsoDate } from "$lib/forms/validators";

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

  const bookId = 1;

  let transactions: TransactionWithSplits[] = [];
  let accounts: Account[] = [];
  let categories: CategorySummary[] = [];
  let payees: PayeeSummary[] = [];
  let tags: TagSummary[] = [];
  let people: PersonSummary[] = [];
  let projects: ProjectSummary[] = [];
  let commodities: CommoditySummary[] = [];
  let savedViews: TransactionSavedView[] = [];
  let templates: TransactionTemplate[] = [];
  let selectedSavedViewId: number | null = null;
  let selectedTemplateId: number | null = null;
  let newSavedViewName = "";
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
  let crossCurrencyDialogOpen = false;
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
      const created = await createCategory({ book_id: bookId, parent_id: null, name: createCategoryName, kind: createCategoryKind, color: null });
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
    await loadErgonomics();
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
        listAccounts(bookId),
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

  async function loadErgonomics() {
    try {
      [savedViews, templates] = await Promise.all([
        listTransactionViews(bookId),
        listTransactionTemplates(bookId),
      ]);
    } catch {
      savedViews = [];
      templates = [];
    }
  }

  function currentFilterState(): FilterFormState {
    return {
      search,
      dateFrom,
      dateTo,
      statusFilter,
      accountFilterId,
      amountMin,
      amountMax,
      sortBy,
      sortDir,
    };
  }

  function buildFilter(offset: number) {
    return buildFilterFromState(currentFilterState(), { bookId, limit: BATCH_SIZE, offset });
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
      const result = await fetchTransactions(buildFilter(append ? currentOffset : 0));
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

  async function saveCurrentView() {
    if (!newSavedViewName.trim()) return;
    try {
      await createTransactionView({
        book_id: bookId,
        name: newSavedViewName.trim(),
        filters: buildFilter(0),
        is_shared: false,
      });
      newSavedViewName = "";
      await loadErgonomics();
    } catch (e) {
      error = `Failed to save view: ${formatError(e)}`;
    }
  }

  async function applySavedView() {
    const view = savedViews.find((item) => item.id === selectedSavedViewId);
    if (!view) return;
    const next = applyViewToState(view.filters);
    search = next.search;
    dateFrom = next.dateFrom;
    dateTo = next.dateTo;
    statusFilter = next.statusFilter;
    accountFilterId = next.accountFilterId;
    amountMin = next.amountMin;
    amountMax = next.amountMax;
    sortBy = next.sortBy;
    sortDir = next.sortDir;
    await loadTransactions(false);
  }

  async function postSelectedTemplate() {
    if (!selectedTemplateId) return;
    try {
      await postTransactionTemplate(selectedTemplateId, new Date().toISOString().slice(0, 10));
      await loadTransactions(false);
    } catch (e) {
      error = `Failed to post template: ${formatError(e)}`;
    }
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
      { ...emptySplitDraft(), account_id: accountFilterId },
      emptySplitDraft(),
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
        { ...emptySplitDraft(), account_id: formAccountId, category_id: formCategoryId, category_input: formCategoryInput, amount: formAmount },
        { ...emptySplitDraft(), account_id: formTransferAccountId, amount: formAmount ? `-${formAmount}` : "" },
      ];
    }
    splitEditorOpen = true;
  }

  function toNullableInt(value: number | string | null | undefined): number | null {
    if (value === null || value === undefined || value === "") return null;
    const n = typeof value === "number" ? value : Number(value);
    return Number.isFinite(n) ? Math.trunc(n) : null;
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
          const defaults = await getPayeeDefaults(matched.id, formAccountId ?? undefined);
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
      const created = await createPayee({ book_id: bookId, name: trimmed, kind: "person", metadata: null });
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
      const created = await createTag({ book_id: bookId, name: trimmed, color: null });
      tags = [...tags, created];
      return created.id;
    }

    if (kind === "person") {
      const existing = exactMatchByName(people, trimmed);
      if (existing) return existing.id;
      if (!await askConfirm(`Create new person "${trimmed}"?`, { label: "Create" })) throw new Error("Person creation cancelled");
      const created = await createPerson({ book_id: bookId, name: trimmed, role: "member", metadata: null });
      people = [...people, created];
      return created.id;
    }

    const existing = exactMatchByName(projects, trimmed);
    if (existing) return existing.id;
    if (!await askConfirm(`Create new project "${trimmed}"?`, { label: "Create" })) throw new Error("Project creation cancelled");
    const created = await createProject({ book_id: bookId, name: trimmed, status: "active", metadata: null });
    projects = [...projects, created];
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

    const created = await createAccount({
      book_id: bookId,
      parent_id: null,
      account_type: targetKind,
      name: targetName,
      commodity_id: commodityId,
      institution_id: null,
      country_id: null,
      number_last4: null,
      is_closed: false,
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
        unlockAccountBalancings(account_id, tx.transaction.txn_date, reason || null, true)
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
      const dateResult = validateIsoDate(formDate, "Date");
      if (!dateResult.ok) throw new Error(dateResult.error);
      const payeeId = await ensureEntityId("payee", formPayeeInput, formPayeeId);
      formPayeeId = payeeId;
      const splits = await buildSplitsForSubmit();

      const total = splits.reduce((sum, s) => sum + s.amount_minor, 0);
      if (total !== 0) {
        throw new Error("Splits must balance to zero");
      }

      if (dialogMode === "create") {
        await createTransaction({
          book_id: bookId,
          txn_date: formDate,
          payee_id: payeeId,
          memo: formMemo || null,
          status: formStatus,
          reference: formReference || null,
          import_id: null,
          splits,
        });
      } else if (formId) {
        await updateTransaction({
          id: formId,
          book_id: bookId,
          txn_date: formDate,
          payee_id: payeeId,
          memo: formMemo || null,
          status: formStatus,
          reference: formReference || null,
          import_id: null,
          splits,
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
      await updateTransaction({
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
          memo: split.memo,
        })),
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
      await deleteTransaction(tx.transaction.id);
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
      await duplicateTransactionCommand(tx.transaction.id, today);
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
    const requested = selectedIds.size;
    if (requested === 0) return;
    const confirmed = await askConfirm(`Void ${requested} selected transaction(s)?`, { label: "Void", destructive: true });
    if (!confirmed) return;
    bulkActionRunning = true;
    error = "";
    try {
      const ids = Array.from(selectedIds);
      const voided = await bulkVoidTransactions(ids);
      clearSelection();
      await loadTransactions();
      if (voided < requested) {
        const skipped = requested - voided;
        error = `${voided} voided, ${skipped} skipped (likely locked by reconciliation).`;
      }
    } catch (e) {
      error = `Bulk void failed: ${formatError(e)}`;
    } finally {
      bulkActionRunning = false;
    }
  }

  async function bulkDelete() {
    const requested = selectedIds.size;
    if (requested === 0) return;
    const confirmed = await askConfirm(`Permanently delete ${requested} selected transaction(s)? This cannot be undone.`, { label: "Delete", destructive: true });
    if (!confirmed) return;
    bulkActionRunning = true;
    error = "";
    try {
      const ids = Array.from(selectedIds);
      const deleted = await bulkDeleteTransactions(ids);
      clearSelection();
      await loadTransactions();
      if (deleted < requested) {
        const skipped = requested - deleted;
        error = `${deleted} deleted, ${skipped} skipped (likely locked by reconciliation).`;
      }
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
      <div class="flex gap-2">
        <Button variant="outline" onclick={() => (crossCurrencyDialogOpen = true)}>Cross-currency transfer…</Button>
        <Button onclick={openCreateDialog}>New transaction</Button>
      </div>
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

        <SavedViewsBar
          {savedViews}
          {templates}
          bind:selectedSavedViewId
          bind:selectedTemplateId
          bind:newSavedViewName
          onApplyView={applySavedView}
          onSaveView={saveCurrentView}
          onPostTemplate={postSelectedTemplate}
        />

        {#if loading}
          <p class="text-sm text-muted-foreground">Loading transactions…</p>
        {:else}
          <TransactionFilters
            {activeFilter}
            {accounts}
            bind:dateFrom
            bind:dateTo
            bind:statusFilter
            bind:accountFilterId
            bind:amountMin
            bind:amountMax
            bind:search
            onApply={applyFilters}
            {onSearchInput}
          />

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
                    <TransactionRow
                      {tx}
                      {accountFilterId}
                      selected={selectedIds.has(tx.transaction.id)}
                      {accounts}
                      {payees}
                      {commodities}
                      onToggleSelect={toggleSelect}
                      onEdit={openEditDialog}
                      onDuplicate={duplicateTransaction}
                      onUpdateStatus={updateStatus}
                      onRemove={removeTransaction}
                    />
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
              class={!formDate && formDateRaw ? "border-status-danger" : ""}
              required
            />
            {#if formDateHint}
              <p class="text-xs text-muted-foreground">→ {formDateHint}</p>
            {:else if !formDate && formDateRaw}
              <p class="text-xs text-status-danger">Date not recognized</p>
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

  <TransactionSplitEditor
    bind:open={splitEditorOpen}
    bind:splits={formSplits}
    {accounts}
    {categories}
    {tags}
    {people}
    {projects}
    {commodities}
  />

  <CrossCurrencyTransferDialog
    bind:open={crossCurrencyDialogOpen}
    {bookId}
    {accounts}
    {commodities}
    onPosted={() => loadTransactions(false)}
  />

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
