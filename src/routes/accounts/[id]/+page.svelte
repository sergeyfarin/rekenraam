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
  import { formatMinorWithScale, parseAmountToMinor } from "$lib/money";
  import { exactMatchByName, fuzzyOptions } from "$lib/search/fuzzy";
  import { validateIsoDate } from "$lib/forms/validators";
  import { emptySplitDraft, type SplitDraft } from "$lib/transactions/split-draft";
  import TransactionSplitEditor from "$lib/components/TransactionSplitEditor.svelte";
  import AccountHeader from "$lib/components/AccountHeader.svelte";
  import AccountRegister from "$lib/components/AccountRegister.svelte";

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

  function accountSplit(tx: TransactionWithSplits) {
    if (!accountId) return tx.splits[0];
    return tx.splits.find((s) => s.account_id === accountId) ?? tx.splits[0];
  }

  function formatAmountInput(amountMinor: number, commodityId: number): string {
    const commodity = commodities.find((c) => c.id === commodityId);
    if (!commodity) return String(amountMinor);
    return formatMinorWithScale(amountMinor, commodity.scale);
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
      { ...emptySplitDraft(), account_id: accountId },
      emptySplitDraft(),
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


  function syncTopLevelInput(kind: "payee" | "category", value: string) {
    if (kind === "payee") {
      formPayeeInput = value;
      formPayeeId = exactMatchByName(payees, value)?.id ?? null;
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
      const dateResult = validateIsoDate(formDate, "Date");
      if (!dateResult.ok) throw new Error(dateResult.error);
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
          <AccountHeader
            {account}
            {balanceMinor}
            {lastBalancing}
            {directives}
            {commodities}
            bind:bookingPolicy
            {savingPolicy}
            onSaveBookingPolicy={saveBookingPolicy}
          />
        </div>
      </div>

      {#if accountId}
        <NotesPanel targetType="account" targetId={accountId} />
      {/if}

      {#if accountId !== null}
        <div class="page-row">
          <div class="page-col">
            <AccountRegister
              entries={registerEntries}
              accountId={accountId}
              {accounts}
              {payees}
              {categories}
              {commodities}
              bind:search
              bind:dateFrom
              bind:dateTo
              bind:statusFilter
              bind:sortBy
              bind:sortDir
              onApplyFilters={loadTransactions}
              onOpenCreate={openCreateDialog}
              onOpenEdit={openEditDialog}
              onUpdateStatus={updateRegisterEntryStatus}
              onRemove={removeRegisterEntry}
            />
          </div>
        </div>
      {/if}
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

  {#if dialogOpen}
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

