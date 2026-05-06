<script lang="ts">
  import { onMount } from "svelte";
  import { page } from "$app/state";
  import {
    createAccount,
    getAccountBookingPolicy,
    getAccountById,
    listAccounts,
    listAccountBalancings,
    listAccountBalances,
    listAccountDirectives,
    setAccountBookingPolicy,
    unlockAccountBalancings,
    type AccountSummary,
    type AccountBalancingSummary,
    type AccountDirectiveSummary,
  } from "$lib/api/accounts";
  import {
    createCategory,
    createPayee,
    createPerson,
    createProject,
    createTag,
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
  import {
    createTransaction,
    deleteTransaction,
    getTransactionById,
    listAccountRegister,
    updateTransaction,
    type AccountRegisterEntry,
    type TransactionMutationInput,
    type TransactionWithSplits,
  } from "$lib/api/transactions";
  import * as Card from "$lib/components/ui/card";
  import { Button } from "$lib/components/ui/button";
  import { Input } from "$lib/components/ui/input";
  import { Label } from "$lib/components/ui/label";
  import * as Table from "$lib/components/ui/table";
  import * as Dialog from "$lib/components/ui/dialog";
  import { Badge } from "$lib/components/ui/badge";
  import ConfirmDialog from "$lib/components/ConfirmDialog.svelte";
  import NotesPanel from "$lib/components/NotesPanel.svelte";
  import { formatError } from "$lib/utils";

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

  let accountId: number | null = null;
  let account: AccountSummary | null = null;
  let registerEntries: AccountRegisterEntry[] = [];
  let directives: AccountDirectiveSummary[] = [];
  let balanceMinor = 0;
  let lastBalancing: AccountBalancingSummary | null = null;
  let bookingPolicy = "fifo";
  let savingPolicy = false;
  let loading = true;
  let error = "";
  let accounts: AccountSummary[] = [];
  let categories: CategorySummary[] = [];
  let payees: PayeeSummary[] = [];
  let tags: TagSummary[] = [];
  let people: PersonSummary[] = [];
  let projects: ProjectSummary[] = [];
  let commodities: CommoditySummary[] = [];

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
  const transactionDetailCache = new Map<number, TransactionWithSplits>();

  $: selectedCategoryKind = formCategoryId
    ? categories.find((category) => category.id === formCategoryId)?.kind ?? null
    : null;
  $: transferRequired = selectedCategoryKind === "transfer" || !selectedCategoryKind;
  $: if (selectedCategoryKind && selectedCategoryKind !== "transfer") {
    formTransferAccountId = null;
  }

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
      const created = await createCategory({
        book_id: 1,
        parent_id: null,
        name: createCategoryName,
        kind: createCategoryKind,
        color: null,
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
      const fetched = await getAccountById(accountId);
      if (!fetched) {
        error = "Account not found.";
        return;
      }
      account = fetched;

      const [balanceRows, balancings] = await Promise.all([
        listAccountBalances(1),
        listAccountBalancings(accountId),
      ]);
      const balanceMap = new Map(balanceRows.map((row) => [row.account_id, row.balance_minor] as const));
      balanceMinor = balanceMap.get(accountId) ?? 0;
      // list_account_balancings returns DESC — first entry is most recent
      lastBalancing = balancings.length > 0 ? balancings[0] : null;

      directives = await listAccountDirectives(accountId);
      if (account.account_type === "investment") {
        bookingPolicy = await getAccountBookingPolicy(accountId);
      }
      await loadTransactions();
    } catch (e) {
      error = `Failed to load account: ${formatError(e)}`;
    } finally {
      loading = false;
    }
  }

  async function loadLookups() {
    const [accountList, categoryList, payeeList, tagList, peopleList, projectList, commodityList] = await Promise.all([
      listAccounts(1),
      listCategories(1),
      listPayees(1),
      listTags(1),
      listPeople(1),
      listProjects(1),
      listCommodities(1)
    ]);
    accounts = accountList;
    categories = categoryList;
    payees = payeeList;
    tags = tagList;
    people = peopleList;
    projects = projectList;
    commodities = commodityList;
  }

  async function loadTransactions() {
    if (!accountId) return;
    registerEntries = await listAccountRegister(accountId);
    transactionDetailCache.clear();
  }

  async function getTransactionDetails(transactionId: number): Promise<TransactionWithSplits> {
    const cached = transactionDetailCache.get(transactionId);
    if (cached) {
      return cached;
    }

    const fetched = await getTransactionById(transactionId);
    if (!fetched) {
      throw new Error("Transaction not found.");
    }

    transactionDetailCache.set(transactionId, fetched);
    return fetched;
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
      bookingPolicy = await setAccountBookingPolicy(accountId, bookingPolicy);
    } catch (e) {
      error = `Failed to update booking policy: ${formatError(e)}`;
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
      { account_id: accountId, category_id: null, category_input: "", tag_id: null, tag_input: "", person_id: null, person_input: "", project_id: null, project_input: "", share_bps: null, amount: "", memo: "" },
      { account_id: null, category_id: null, category_input: "", tag_id: null, tag_input: "", person_id: null, person_input: "", project_id: null, project_input: "", share_bps: null, amount: "", memo: "" }
    ];
    dialogOpen = true;
  }

  async function openEditDialog(transactionId: number) {
    const tx = await getTransactionDetails(transactionId);
    dialogMode = "edit";
    formId = tx.transaction.id;
    formDate = tx.transaction.txn_date;
    const primary = accountSplit(tx);
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

  function syncTopLevelInput(kind: "payee" | "category", value: string) {
    if (kind === "payee") {
      formPayeeInput = value;
      formPayeeId = exactMatchByName(payees, value)?.id ?? null;
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
      const created = await createPayee({ book_id: 1, name: trimmed, kind: "person", metadata: null });
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
      const created = await createTag({ book_id: 1, name: trimmed, color: null });
      tags = [...tags, created];
      return created.id;
    }

    if (kind === "person") {
      const existing = exactMatchByName(people, trimmed);
      if (existing) return existing.id;
      if (!await askConfirm(`Create new person "${trimmed}"?`, { label: "Create" })) throw new Error("Person creation cancelled");
      const created = await createPerson({ book_id: 1, name: trimmed, role: "member", metadata: null });
      people = [...people, created];
      return created.id;
    }

    const existing = exactMatchByName(projects, trimmed);
    if (existing) return existing.id;
    if (!await askConfirm(`Create new project "${trimmed}"?`, { label: "Create" })) throw new Error("Project creation cancelled");
    const created = await createProject({ book_id: 1, name: trimmed, status: "active", metadata: null });
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

    const created = await createAccount({
      book_id: 1,
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
    if (!accountId) return;
    submitting = true;
    error = "";
    try {
      const payeeId = await ensureEntityId("payee", formPayeeInput, formPayeeId);
      formPayeeId = payeeId;
      const splits = await buildSplitsForSubmit();

      const total = splits.reduce((sum, s) => sum + s.amount_minor, 0);
      if (total !== 0) {
        throw new Error("Splits must balance to zero");
      }

      const input: TransactionMutationInput = {
        ...(dialogMode === "edit" && formId ? { id: formId } : {}),
        book_id: 1,
        txn_date: formDate,
        payee_id: payeeId,
        memo: formMemo || null,
        status: formStatus,
        reference: formReference || null,
        import_id: null,
        splits,
      };

      if (dialogMode === "create") {
        await createTransaction(input);
      } else if (formId) {
        await updateTransaction(input);
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

  async function updateRegisterEntryStatus(entry: AccountRegisterEntry, status: string) {
    const tx = await getTransactionDetails(entry.tx_id);
    await updateStatus(tx, status);
  }

  async function removeRegisterEntry(entry: AccountRegisterEntry) {
    const tx = await getTransactionDetails(entry.tx_id);
    await removeTransaction(tx);
  }

  $: filtered = registerEntries.filter((entry) => {
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
  });

  $: sorted = [...filtered].sort((a, b) => {
    const direction = sortDir === "asc" ? 1 : -1;
    if (sortBy === "date") {
      return direction * a.txn_date.localeCompare(b.txn_date);
    }
    if (sortBy === "payee") {
      return direction * payeeName(a.payee_id).localeCompare(payeeName(b.payee_id));
    }
    if (sortBy === "memo") {
      return direction * (a.memo ?? "").localeCompare(b.memo ?? "");
    }
    if (sortBy === "status") {
      return direction * a.status.localeCompare(b.status);
    }
    if (sortBy === "amount") {
      return direction * (a.amount_minor - b.amount_minor);
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
              {#if lastBalancing}
                <span>
                  Last reconciled: {lastBalancing.as_of_date}
                  · {formatMinor(lastBalancing.balance_minor, account.commodity_id)}
                </span>
              {/if}
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
                  onchange={saveBookingPolicy}
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

      {#if accountId}
        <NotesPanel targetType="account" targetId={accountId} />
      {/if}

      <div class="page-row">
        <div class="page-col">
          <div class="card">
            <div class="page-row">
              <div class="page-col">
                <h2 class="section-title">Transactions</h2>
              </div>
              <div class="page-col page-col-actions">
                <Button variant="secondary" href="/accounts/{accountId}/reconcile">Reconcile</Button>
                <Button onclick={openCreateDialog}>New transaction</Button>
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
                <Button variant="secondary" onclick={loadTransactions}>Apply filters</Button>
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
                {#each sorted as entry}
                  {#key entry.split_id}
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
                        <Button variant="ghost" size="sm" onclick={() => openEditDialog(entry.tx_id)}>Edit</Button>
                        {#if entry.status === "cleared"}
                          <Button variant="ghost" size="sm" onclick={() => updateRegisterEntryStatus(entry, "uncleared")}>Unflag</Button>
                        {:else}
                          <Button variant="ghost" size="sm" onclick={() => updateRegisterEntryStatus(entry, "cleared")}>Flag</Button>
                        {/if}
                        <Button variant="ghost" size="sm" onclick={() => updateRegisterEntryStatus(entry, "void")}>Void</Button>
                        <Button variant="ghost" size="sm" onclick={() => removeRegisterEntry(entry)}>Delete</Button>
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
    <button class="dialog-backdrop" type="button" aria-label="Close dialog" onclick={closeDialog}></button>
    <div class="dialog" role="dialog" aria-modal="true" aria-label="Transaction dialog">
      <form onsubmit={(e) => { e.preventDefault(); submitTransaction(); }}>
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
            <input
              id="acct-tx-payee"
              class="input"
              list="acct-tx-payee-options"
              placeholder="Search or enter payee"
              value={formPayeeInput}
              oninput={(event) => syncTopLevelInput("payee", (event.currentTarget as HTMLInputElement).value)}
            />
            <datalist id="acct-tx-payee-options">
              {#each fuzzyOptions(payees, formPayeeInput) as payee}
                <option value={payee.name}></option>
              {/each}
            </datalist>
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
            <input
              id="acct-tx-category"
              class="input"
              list="acct-tx-category-options"
              placeholder="Search or enter category"
              value={formCategoryInput}
              oninput={(event) => syncTopLevelInput("category", (event.currentTarget as HTMLInputElement).value)}
              disabled={splitMode}
            />
            <datalist id="acct-tx-category-options">
              {#each fuzzyOptions(categories, formCategoryInput) as category}
                <option value={category.name}></option>
              {/each}
            </datalist>
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
            <Button variant="secondary" onclick={openSplitEditor}>Split transaction…</Button>
            {#if splitMode}
              <p class="text-sm text-muted">Split mode enabled.</p>
            {/if}
          </div>
        </div>

        <footer class="dialog-footer">
          <Button variant="secondary" onclick={closeDialog}>Cancel</Button>
          <Button type="submit" disabled={submitting}>
            {submitting ? "Saving…" : "Save"}
          </Button>
        </footer>
      </form>
    </div>
  {/if}

  {#if dialogOpen && splitEditorOpen}
    <button class="dialog-backdrop" type="button" aria-label="Close split editor" onclick={closeSplitEditor}></button>
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
              <div class="data-cell heading">Tag</div>
              <div class="data-cell heading">Person</div>
              <div class="data-cell heading">Project</div>
              <div class="data-cell heading">Share (bps)</div>
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
                  <input
                    class="input"
                    list={`acct-split-category-options-${idx}`}
                    placeholder="Search/enter category"
                    value={split.category_input}
                    oninput={(event) => syncSplitInput(split, "category", (event.currentTarget as HTMLInputElement).value)}
                  />
                  <datalist id={`acct-split-category-options-${idx}`}>
                    {#each fuzzyOptions(categories, split.category_input) as category}
                      <option value={category.name}></option>
                    {/each}
                  </datalist>
                </div>
                <div class="data-cell">
                  <input
                    class="input"
                    list={`acct-split-tag-options-${idx}`}
                    placeholder="Search/enter tag"
                    value={split.tag_input}
                    oninput={(event) => syncSplitInput(split, "tag", (event.currentTarget as HTMLInputElement).value)}
                  />
                  <datalist id={`acct-split-tag-options-${idx}`}>
                    {#each fuzzyOptions(tags, split.tag_input) as tag}
                      <option value={tag.name}></option>
                    {/each}
                  </datalist>
                </div>
                <div class="data-cell">
                  <input
                    class="input"
                    list={`acct-split-person-options-${idx}`}
                    placeholder="Search/enter person"
                    value={split.person_input}
                    oninput={(event) => syncSplitInput(split, "person", (event.currentTarget as HTMLInputElement).value)}
                  />
                  <datalist id={`acct-split-person-options-${idx}`}>
                    {#each fuzzyOptions(people, split.person_input) as person}
                      <option value={person.name}></option>
                    {/each}
                  </datalist>
                </div>
                <div class="data-cell">
                  <input
                    class="input"
                    list={`acct-split-project-options-${idx}`}
                    placeholder="Search/enter project"
                    value={split.project_input}
                    oninput={(event) => syncSplitInput(split, "project", (event.currentTarget as HTMLInputElement).value)}
                  />
                  <datalist id={`acct-split-project-options-${idx}`}>
                    {#each fuzzyOptions(projects, split.project_input) as project}
                      <option value={project.name}></option>
                    {/each}
                  </datalist>
                </div>
                <div class="data-cell">
                  <input class="input" type="number" bind:value={split.share_bps} placeholder="e.g. 5000" />
                </div>
                <div class="data-cell amount">
                  <input class="input" bind:value={split.amount} placeholder="0.00" />
                </div>
                <div class="data-cell">
                  <input class="input" bind:value={split.memo} placeholder="Split memo" />
                </div>
                <div class="data-cell action">
                  <Button variant="ghost" size="sm" onclick={() => removeSplitRow(idx)}>
                    Remove
                  </Button>
                </div>
              </div>
            {/each}
          </div>
          <Button variant="ghost" size="sm" onclick={addSplitRow}>Add split</Button>
          {#if splitsTotalMinor() !== null}
            {@const total = splitsTotalMinor()!}
            {@const balanced = total === 0}
            <div class="flex items-center gap-2 mt-2">
              <span class="text-sm font-medium">Balance:</span>
              <span class="text-sm font-mono {balanced ? 'text-money-positive' : 'text-money-negative'}">
                {#if balanced}
                  ✓ Balanced
                {:else}
                  {@const firstAcc = accounts.find((a) => a.id === formSplits[0]?.account_id)}
                  {@const firstCom = commodities.find((c) => c.id === firstAcc?.commodity_id)}
                  {@const scale = firstCom?.scale ?? 2}
                  {total > 0 ? "+" : ""}{formatMinorWithScale(total, scale)} {firstCom?.symbol ?? ""} (unbalanced)
                {/if}
              </span>
            </div>
          {/if}
        </fieldset>
      </div>
      <div class="dialog-footer">
        <Button variant="secondary" onclick={closeSplitEditor}>Done</Button>
      </div>
    </div>
  {/if}

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
    color: var(--muted-foreground);
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
    outline: 2px solid var(--ring);
    outline-offset: 2px;
  }
</style>
