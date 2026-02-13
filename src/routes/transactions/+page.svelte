<script lang="ts">
  import { onMount } from "svelte";
  import { invoke } from "@tauri-apps/api/core";
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
    name: string;
    commodity_id: number;
    account_type: string;
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

  type Tag = {
    id: number;
    name: string;
  };

  type Person = {
    id: number;
    name: string;
  };

  type Project = {
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
  let categories: Category[] = [];
  let payees: Payee[] = [];
  let tags: Tag[] = [];
  let people: Person[] = [];
  let projects: Project[] = [];
  let commodities: Commodity[] = [];
  let loading = true;
  let error = "";

  let search = "";
  let dateFrom = "";
  let dateTo = "";
  let statusFilter = "";
  let accountFilterId: number | null = null;
  let payeeFilter = "";
  let memoFilter = "";
  let amountMin = "";
  let amountMax = "";
  type FilterColumn = "date" | "payee" | "memo" | "status" | "account" | "amount";
  let activeFilter: FilterColumn | null = null;
  let sortBy: "date" | "payee" | "memo" | "status" | "account" | "amount" = "date";
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
  });

  async function loadLookups() {
    try {
      const [accountList, categoryList, payeeList, tagList, peopleList, projectList, commodityList] = await Promise.all([
        invoke<Account[]>("list_accounts", { bookId }),
        invoke<Category[]>("list_categories", { bookId }),
        invoke<Payee[]>("list_payees", { bookId }),
        invoke<Tag[]>("list_tags", { bookId }),
        invoke<Person[]>("list_people", { bookId }),
        invoke<Project[]>("list_projects", { bookId }),
        invoke<Commodity[]>("list_commodities", { bookId })
      ]);
      accounts = accountList;
      categories = categoryList;
      payees = payeeList;
      tags = tagList;
      people = peopleList;
      projects = projectList;
      commodities = commodityList;
    } catch (e) {
      error = `Failed to load lookup data: ${String(e)}`;
    }
  }

  async function loadTransactions() {
    loading = true;
    error = "";
    try {
      const filter = {
        book_id: bookId,
        account_id: accountFilterId ?? undefined,
        date_from: dateFrom || undefined,
        date_to: dateTo || undefined,
        search: search || undefined,
        limit: 10000,
        offset: 0
      };
      transactions = await invoke<TransactionWithSplits[]>("list_transactions", { filter });
    } catch (e) {
      error = `Failed to load transactions: ${String(e)}`;
    } finally {
      loading = false;
    }
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
    setSort(column);
    toggleFilter(column);
  }

  function toggleFilter(column: FilterColumn) {
    activeFilter = activeFilter === column ? null : column;
  }

  function hasFilter(column: FilterColumn) {
    if (column === "date") return Boolean(dateFrom || dateTo);
    if (column === "payee") return Boolean(payeeFilter);
    if (column === "memo") return Boolean(memoFilter);
    if (column === "status") return Boolean(statusFilter);
    if (column === "account") return Boolean(accountFilterId);
    if (column === "amount") return Boolean(amountMin || amountMax);
    return false;
  }

  async function applyFilters() {
    await loadTransactions();
  }

  async function clearFilter(column: FilterColumn) {
    if (column === "date") {
      dateFrom = "";
      dateTo = "";
      await loadTransactions();
      return;
    }
    if (column === "payee") {
      payeeFilter = "";
      return;
    }
    if (column === "memo") {
      memoFilter = "";
      return;
    }
    if (column === "status") {
      statusFilter = "";
      return;
    }
    if (column === "account") {
      accountFilterId = null;
      await loadTransactions();
      return;
    }
    if (column === "amount") {
      amountMin = "";
      amountMax = "";
    }
  }

  async function openCreateDialog() {
    dialogMode = "create";
    formId = null;
    formDate = new Date().toISOString().slice(0, 10);
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
      if (!window.confirm(`Create new payee \"${trimmed}\"?`)) throw new Error("Payee creation cancelled");
      const created = await invoke<Payee>("create_payee", {
        input: { book_id: bookId, name: trimmed, kind: "person", metadata: null }
      });
      payees = [...payees, created];
      return created.id;
    }

    if (kind === "category") {
      const existing = exactMatchByName(categories, trimmed);
      if (existing) return existing.id;
      if (!window.confirm(`Create new category \"${trimmed}\"?`)) throw new Error("Category creation cancelled");
      const rawKind = (window.prompt("Category kind (income, expense, transfer, other)", "expense") ?? "")
        .trim()
        .toLowerCase();
      const categoryKind = ["income", "expense", "transfer", "other"].includes(rawKind)
        ? rawKind
        : "expense";
      const created = await invoke<Category>("create_category", {
        input: { book_id: bookId, parent_id: null, name: trimmed, kind: categoryKind, color: null }
      });
      categories = [...categories, created];
      return created.id;
    }

    if (kind === "tag") {
      const existing = exactMatchByName(tags, trimmed);
      if (existing) return existing.id;
      if (!window.confirm(`Create new tag \"${trimmed}\"?`)) throw new Error("Tag creation cancelled");
      const created = await invoke<Tag>("create_tag", {
        input: { book_id: bookId, name: trimmed, color: null }
      });
      tags = [...tags, created];
      return created.id;
    }

    if (kind === "person") {
      const existing = exactMatchByName(people, trimmed);
      if (existing) return existing.id;
      if (!window.confirm(`Create new person \"${trimmed}\"?`)) throw new Error("Person creation cancelled");
      const created = await invoke<Person>("create_person", {
        input: { book_id: bookId, name: trimmed, role: "member", metadata: null }
      });
      people = [...people, created];
      return created.id;
    }

    const existing = exactMatchByName(projects, trimmed);
    if (existing) return existing.id;
    if (!window.confirm(`Create new project \"${trimmed}\"?`)) throw new Error("Project creation cancelled");
    const created = await invoke<Project>("create_project", {
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

  function setSort(column: typeof sortBy) {
    if (sortBy === column) {
      sortDir = sortDir === "asc" ? "desc" : "asc";
    } else {
      sortBy = column;
      sortDir = column === "date" ? "desc" : "asc";
    }
  }

  $: filtered = transactions.filter((tx) => {
    if (statusFilter && tx.transaction.status !== statusFilter) return false;
    if (dateFrom && tx.transaction.txn_date < dateFrom) return false;
    if (dateTo && tx.transaction.txn_date > dateTo) return false;
    if (accountFilterId && !tx.splits.some((s) => s.account_id === accountFilterId)) return false;
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
    if (payeeFilter) {
      const payee = payeeName(tx.transaction.payee_id).toLowerCase();
      if (!payee.includes(payeeFilter.toLowerCase())) return false;
    }
    if (memoFilter) {
      const memo = (tx.transaction.memo ?? "").toLowerCase();
      if (!memo.includes(memoFilter.toLowerCase())) return false;
    }
    if (amountMin || amountMax) {
      const split = primarySplit(tx, accountFilterId);
      const value = amountToNumber(split.amount_minor, split.commodity_id);
      if (amountMin && Number.isFinite(Number(amountMin)) && value < Number(amountMin)) return false;
      if (amountMax && Number.isFinite(Number(amountMax)) && value > Number(amountMax)) return false;
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
    if (sortBy === "account") {
      return direction * accountName(primarySplit(a, accountFilterId).account_id).localeCompare(
        accountName(primarySplit(b, accountFilterId).account_id)
      );
    }
    if (sortBy === "amount") {
      return direction * (primarySplit(a, accountFilterId).amount_minor - primarySplit(b, accountFilterId).amount_minor);
    }
    return 0;
  });
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
        {#if loading}
          <p class="text-sm text-muted-foreground">Loading transactions…</p>
        {:else}
          <!-- Filter Panel -->
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
                    <Label for="tx-payee-filter">Payee contains</Label>
                    <Input id="tx-payee-filter" placeholder="Filter payee" bind:value={payeeFilter} />
                  </div>
                  <div class="flex gap-2">
                    <Button variant="secondary" size="sm" onclick={applyFilters}>Apply</Button>
                    <Button variant="ghost" size="sm" onclick={() => clearFilter("payee")}>Clear</Button>
                  </div>
                </div>
              {:else if activeFilter === "memo"}
                <div class="flex flex-wrap items-end gap-4">
                  <div class="space-y-1 flex-1 max-w-xs">
                    <Label for="tx-memo-filter">Memo contains</Label>
                    <Input id="tx-memo-filter" placeholder="Filter memo" bind:value={memoFilter} />
                  </div>
                  <div class="flex gap-2">
                    <Button variant="secondary" size="sm" onclick={applyFilters}>Apply</Button>
                    <Button variant="ghost" size="sm" onclick={() => clearFilter("memo")}>Clear</Button>
                  </div>
                </div>
              {:else if activeFilter === "status"}
                <div class="flex flex-wrap items-end gap-4">
                  <div class="space-y-1">
                    <Label for="tx-status-filter">Status</Label>
                    <select id="tx-status-filter" class="flex h-9 rounded-md border border-input bg-transparent px-3 py-1 text-sm" bind:value={statusFilter}>
                      <option value="">Any status</option>
                      <option value="uncleared">Uncleared</option>
                      <option value="cleared">Cleared</option>
                      <option value="reconciled">Reconciled</option>
                      <option value="void">Void</option>
                    </select>
                  </div>
                  <div class="flex gap-2">
                    <Button variant="secondary" size="sm" onclick={applyFilters}>Apply</Button>
                    <Button variant="ghost" size="sm" onclick={() => clearFilter("status")}>Clear</Button>
                  </div>
                </div>
              {:else if activeFilter === "account"}
                <div class="flex flex-wrap items-end gap-4">
                  <div class="space-y-1">
                    <Label for="tx-account-filter">Account</Label>
                    <select id="tx-account-filter" class="flex h-9 rounded-md border border-input bg-transparent px-3 py-1 text-sm" bind:value={accountFilterId}>
                      <option value="">All accounts</option>
                      {#each accounts as account}
                        <option value={account.id}>{account.name}</option>
                      {/each}
                    </select>
                  </div>
                  <div class="flex gap-2">
                    <Button variant="secondary" size="sm" onclick={applyFilters}>Apply</Button>
                    <Button variant="ghost" size="sm" onclick={() => clearFilter("account")}>Clear</Button>
                  </div>
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

          <div class="rounded-md border">
            <Table.Root>
              <Table.Header>
                <Table.Row>
                  <Table.Head class="cursor-pointer hover:bg-muted/50" onclick={() => handleHeaderClick("date")}>
                    <span class="flex items-center gap-1">
                      {#if hasFilter("date")}<span class="text-primary">⏷</span>{/if}
                      Date
                      {#if sortBy === "date"}<span class="text-xs">{sortDir === "asc" ? "↑" : "↓"}</span>{/if}
                    </span>
                  </Table.Head>
                  <Table.Head class="cursor-pointer hover:bg-muted/50" onclick={() => handleHeaderClick("payee")}>
                    <span class="flex items-center gap-1">
                      {#if hasFilter("payee")}<span class="text-primary">⏷</span>{/if}
                      Payee
                      {#if sortBy === "payee"}<span class="text-xs">{sortDir === "asc" ? "↑" : "↓"}</span>{/if}
                    </span>
                  </Table.Head>
                  <Table.Head class="cursor-pointer hover:bg-muted/50" onclick={() => handleHeaderClick("memo")}>
                    <span class="flex items-center gap-1">
                      {#if hasFilter("memo")}<span class="text-primary">⏷</span>{/if}
                      Memo
                      {#if sortBy === "memo"}<span class="text-xs">{sortDir === "asc" ? "↑" : "↓"}</span>{/if}
                    </span>
                  </Table.Head>
                  <Table.Head class="cursor-pointer hover:bg-muted/50" onclick={() => handleHeaderClick("status")}>
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
                      {#if sortBy === "account"}<span class="text-xs">{sortDir === "asc" ? "↑" : "↓"}</span>{/if}
                    </span>
                  </Table.Head>
                  <Table.Head class="text-right cursor-pointer hover:bg-muted/50" onclick={() => handleHeaderClick("amount")}>
                    <span class="flex items-center justify-end gap-1">
                      {#if hasFilter("amount")}<span class="text-primary">⏷</span>{/if}
                      Amount
                      {#if sortBy === "amount"}<span class="text-xs">{sortDir === "asc" ? "↑" : "↓"}</span>{/if}
                    </span>
                  </Table.Head>
                  <Table.Head class="text-right">Actions</Table.Head>
                </Table.Row>
              </Table.Header>
              <Table.Body>
                {#if sorted.length === 0}
                  <Table.Row>
                    <Table.Cell colspan={7} class="text-center text-muted-foreground py-8">
                      No transactions found.
                    </Table.Cell>
                  </Table.Row>
                {:else}
                  {#each sorted as tx (tx.transaction.id)}
                    <Table.Row class="hover:bg-muted/50">
                      <Table.Cell class="font-mono text-sm">{tx.transaction.txn_date}</Table.Cell>
                      <Table.Cell>{payeeName(tx.transaction.payee_id)}</Table.Cell>
                      <Table.Cell class="text-muted-foreground">{tx.transaction.memo ?? "—"}</Table.Cell>
                      <Table.Cell>
                        <Badge variant={tx.transaction.status === "reconciled" ? "default" : tx.transaction.status === "cleared" ? "secondary" : tx.transaction.status === "void" ? "destructive" : "outline"}>
                          {tx.transaction.status}
                        </Badge>
                      </Table.Cell>
                      <Table.Cell>{accountName(primarySplit(tx, accountFilterId).account_id)}</Table.Cell>
                      <Table.Cell class="text-right font-mono {primarySplit(tx, accountFilterId).amount_minor < 0 ? 'text-red-600' : 'text-green-600'}">
                        {formatAmount(primarySplit(tx, accountFilterId).amount_minor, primarySplit(tx, accountFilterId).commodity_id)}
                      </Table.Cell>
                      <Table.Cell class="text-right">
                        <div class="flex justify-end gap-1">
                          <Button variant="ghost" size="sm" onclick={() => openEditDialog(tx)}>Edit</Button>
                          {#if tx.transaction.status === "cleared"}
                            <Button variant="ghost" size="sm" onclick={() => updateStatus(tx, "uncleared")}>Unflag</Button>
                          {:else}
                            <Button variant="ghost" size="sm" onclick={() => updateStatus(tx, "cleared")}>Flag</Button>
                          {/if}
                          <Button variant="ghost" size="sm" onclick={() => updateStatus(tx, "void")}>Void</Button>
                          <Button variant="ghost" size="sm" class="text-destructive" onclick={() => removeTransaction(tx)}>Delete</Button>
                        </div>
                      </Table.Cell>
                    </Table.Row>
                  {/each}
                {/if}
              </Table.Body>
            </Table.Root>
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
            <Input id="tx-date" type="date" bind:value={formDate} required />
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
            <p class="text-sm text-muted-foreground">Split total: {splitsTotalMinor()}</p>
          {/if}
        </div>
      </div>
      <Dialog.Footer>
        <Button variant="secondary" onclick={closeSplitEditor}>Done</Button>
      </Dialog.Footer>
    </Dialog.Content>
  </Dialog.Root>
</main>