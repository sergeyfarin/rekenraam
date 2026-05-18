<script lang="ts">
  import { onMount } from "svelte";
  import { Button } from "$lib/components/ui/button";
  import { Input } from "$lib/components/ui/input";
  import * as Card from "$lib/components/ui/card";
  import * as Tabs from "$lib/components/ui/tabs";
  import * as Table from "$lib/components/ui/table";
  import { listAccounts, type AccountSummary } from "$lib/api/accounts";
  import { listCategories, type CategorySummary } from "$lib/api/metadata";
  import {
    budgetVariance,
    createBudget,
    createLoan,
    createSchedule,
    listBudgets,
    listLoans,
    listSchedules,
    loanPaymentDraft,
    loanAmortization,
    postLoanPayment,
    postSchedule,
    projectedCash,
    projectedInstances,
    skipSchedule,
    type AmortizationRow,
    type Budget,
    type BudgetVarianceRow,
    type Loan,
    type ProjectedCashRow,
    type Schedule,
    type ScheduleOccurrence
  } from "$lib/api/planning";
  import {
    validateIntegerInRange,
    validateIsoDate,
    validateNonEmptyString,
  } from "$lib/forms/validators";
  import { requireActiveBookId } from "$lib/book-context";
  import { formatApiError } from "$lib/utils";

  const today = new Date().toISOString().slice(0, 10);
  const monthEnd = new Date(new Date().getFullYear(), new Date().getMonth() + 1, 0).toISOString().slice(0, 10);

  let loading = true;
  let error = "";
  let budgets: Budget[] = [];
  let schedules: Schedule[] = [];
  let occurrences: ScheduleOccurrence[] = [];
  let forecast: ProjectedCashRow[] = [];
  let loans: Loan[] = [];
  let amortization: AmortizationRow[] = [];
  let categories: CategorySummary[] = [];
  let accounts: AccountSummary[] = [];
  let variance: BudgetVarianceRow[] = [];
  let status = "";

  let budgetName = "Monthly budget";
  let budgetCategoryId = 0;
  let budgetAmount = 50000;
  let selectedBudgetId = 0;

  let scheduleName = "Recurring transfer";
  let scheduleStart = today;
  let scheduleFrequency: "daily" | "weekly" | "monthly" | "yearly" = "monthly";
  let scheduleFromAccountId = 2;
  let scheduleToAccountId = 3;
  let scheduleAmount = 10000;

  let loanName = "Mortgage";
  let loanAccountId = 0;
  let paymentAccountId = 2;
  let principal = 25000000;
  let rateBps = 650;
  let termMonths = 360;
  let paymentLoanId = 0;
  let paymentDate = today;
  let paymentMemo = "";
  let loanPaymentPreview = "";

  $: selectedBudget = budgets.find((budget) => budget.id === selectedBudgetId) ?? budgets[0];

  onMount(() => {
    void loadPlanning();
  });

  function money(value: number): string {
    return new Intl.NumberFormat(undefined, { style: "currency", currency: "USD" }).format(value / 100);
  }

  function accountName(id: number): string {
    return accounts.find((account) => account.id === id)?.name ?? `Account ${id}`;
  }

  function categoryName(id: number): string {
    return categories.find((category) => category.id === id)?.name ?? `Category ${id}`;
  }

  function bookId(): number {
    return requireActiveBookId();
  }

  function baseCommodityId(): number {
    return accounts.find((account) => !account.is_system)?.commodity_id ?? accounts[0]?.commodity_id ?? 1;
  }

  async function loadPlanning() {
    loading = true;
    error = "";
    try {
      [accounts, categories, budgets, schedules, loans] = await Promise.all([
        listAccounts(bookId()),
        listCategories(bookId()),
        listBudgets(bookId()),
        listSchedules(bookId()),
        listLoans(bookId())
      ]);
      budgetCategoryId = categories[0]?.id ?? 0;
      const usableAccounts = accounts.filter((account) => !account.is_system);
      scheduleFromAccountId = usableAccounts[0]?.id ?? 2;
      scheduleToAccountId = usableAccounts[1]?.id ?? usableAccounts[0]?.id ?? 2;
      loanAccountId = accounts.find((account) => account.account_type === "loan" || account.account_type === "liability")?.id ?? usableAccounts[0]?.id ?? 2;
      paymentAccountId = usableAccounts[0]?.id ?? 2;
      selectedBudgetId = budgets[0]?.id ?? 0;
      paymentLoanId = loans[0]?.id ?? 0;
      await refreshDerived();
    } catch (caught) {
      error = formatApiError(caught);
    } finally {
      loading = false;
    }
  }

  async function refreshDerived() {
    [occurrences, forecast] = await Promise.all([
      projectedInstances(bookId(), today, monthEnd),
      projectedCash(bookId(), today, monthEnd)
    ]);
    if (selectedBudgetId) variance = await budgetVariance(selectedBudgetId, today);
    if (loans[0]) amortization = await loanAmortization(loans[0].id);
  }

  async function submitBudget() {
    if (!budgetCategoryId) return;
    const nameResult = validateNonEmptyString(budgetName, "Budget name");
    if (!nameResult.ok) { error = nameResult.error; return; }
    const amountResult = validateIntegerInRange(budgetAmount, { min: 1, fieldName: "Target amount" });
    if (!amountResult.ok) { error = amountResult.error; return; }
    error = "";
    status = "";
    try {
      const created = await createBudget({
        book_id: bookId(),
        name: nameResult.value,
        period: "monthly",
        starts_on: today.slice(0, 8) + "01",
        ends_on: null,
        commodity_id: baseCommodityId(),
        is_active: true,
        targets: [{ category_id: budgetCategoryId, amount_minor: amountResult.value, rollover_enabled: true }]
      });
      budgets = [...budgets, created];
      selectedBudgetId = created.id;
      variance = await budgetVariance(created.id, today);
    } catch (caught) {
      error = formatApiError(caught);
    }
  }

  async function submitSchedule() {
    const nameResult = validateNonEmptyString(scheduleName, "Schedule name");
    if (!nameResult.ok) { error = nameResult.error; return; }
    const startResult = validateIsoDate(scheduleStart, "Start date");
    if (!startResult.ok) { error = startResult.error; return; }
    const amountResult = validateIntegerInRange(scheduleAmount, { min: 1, fieldName: "Schedule amount" });
    if (!amountResult.ok) { error = amountResult.error; return; }
    error = "";
    status = "";
    try {
      const created = await createSchedule({
        book_id: bookId(),
        name: nameResult.value,
        payee_id: null,
        memo: nameResult.value,
        status: "uncleared",
        reference: null,
        frequency: scheduleFrequency,
        interval: 1,
        start_date: startResult.value,
        end_date: null,
        reminder_days: 3,
        enabled: true,
        splits: [
          {
            account_id: scheduleFromAccountId,
            commodity_id: accounts.find((account) => account.id === scheduleFromAccountId)?.commodity_id ?? baseCommodityId(),
            amount_minor: -amountResult.value,
            category_id: null,
            memo: "Source"
          },
          {
            account_id: scheduleToAccountId,
            commodity_id: accounts.find((account) => account.id === scheduleToAccountId)?.commodity_id ?? baseCommodityId(),
            amount_minor: amountResult.value,
            category_id: null,
            memo: "Destination"
          }
        ]
      });
      schedules = [...schedules, created];
      await refreshDerived();
    } catch (caught) {
      error = formatApiError(caught);
    }
  }

  async function skipOccurrence(row: ScheduleOccurrence) {
    try {
      await skipSchedule(row.schedule_id, row.occurrence_date);
      await refreshDerived();
    } catch (caught) {
      error = formatApiError(caught);
    }
  }

  async function postOccurrence(row: ScheduleOccurrence) {
    try {
      await postSchedule(row.schedule_id, row.occurrence_date);
      await refreshDerived();
    } catch (caught) {
      error = formatApiError(caught);
    }
  }

  async function submitLoan() {
    const nameResult = validateNonEmptyString(loanName, "Loan name");
    if (!nameResult.ok) { error = nameResult.error; return; }
    const principalResult = validateIntegerInRange(principal, { min: 1, fieldName: "Principal" });
    if (!principalResult.ok) { error = principalResult.error; return; }
    const rateResult = validateIntegerInRange(rateBps, { min: 0, max: 100000, fieldName: "Rate (bps)" });
    if (!rateResult.ok) { error = rateResult.error; return; }
    const termResult = validateIntegerInRange(termMonths, { min: 1, max: 600, fieldName: "Term (months)" });
    if (!termResult.ok) { error = termResult.error; return; }
    error = "";
    status = "";
    try {
      const created = await createLoan({
        book_id: bookId(),
        account_id: loanAccountId,
        name: nameResult.value,
        principal_minor: principalResult.value,
        annual_rate_bps: rateResult.value,
        term_months: termResult.value,
        payment_frequency: "monthly",
        start_date: today,
        payment_account_id: paymentAccountId,
        interest_category_id: null
      });
      loans = [...loans, created];
      paymentLoanId = created.id;
      amortization = await loanAmortization(created.id);
    } catch (caught) {
      error = formatApiError(caught);
    }
  }

  async function previewLoanPayment() {
    if (!paymentLoanId) return;
    error = "";
    status = "";
    loanPaymentPreview = "";
    try {
      const draft = await loanPaymentDraft(paymentLoanId, {
        payment_date: paymentDate,
        payment_account_id: paymentAccountId || null,
        interest_category_id: null,
        memo: paymentMemo || null,
      });
      const splits = draft.transaction.splits.length;
      loanPaymentPreview = `Draft ready for ${draft.transaction.txn_date} with ${splits} splits.`;
    } catch (caught) {
      error = formatApiError(caught);
    }
  }

  async function submitLoanPayment() {
    if (!paymentLoanId) return;
    error = "";
    status = "";
    try {
      const tx = await postLoanPayment(paymentLoanId, {
        payment_date: paymentDate,
        payment_account_id: paymentAccountId || null,
        interest_category_id: null,
        memo: paymentMemo || null,
      });
      status = `Loan payment posted as transaction #${tx.id}.`;
      loanPaymentPreview = "";
      amortization = await loanAmortization(paymentLoanId);
      await refreshDerived();
    } catch (caught) {
      error = formatApiError(caught);
    }
  }
</script>

<main class="py-6">
  <div class="container mx-auto px-6 space-y-6">
    <div>
      <h1 class="text-3xl font-bold tracking-tight">Planning</h1>
      <p class="text-muted-foreground">Budgets, recurring activity, cash projection, and personal liabilities.</p>
    </div>

    {#if error}
      <p class="rounded-md border border-destructive/40 p-3 text-sm text-destructive">{error}</p>
    {/if}
    {#if status}
      <p class="rounded-md border border-border bg-muted p-3 text-sm">{status}</p>
    {/if}

    {#if loading}
      <p class="text-sm text-muted-foreground">Loading planning workspace</p>
    {:else}
      <Tabs.Root value="budgets" class="space-y-4">
        <Tabs.List>
          <Tabs.Trigger value="budgets">Budgets</Tabs.Trigger>
          <Tabs.Trigger value="schedules">Schedules</Tabs.Trigger>
          <Tabs.Trigger value="forecast">Forecast</Tabs.Trigger>
          <Tabs.Trigger value="loans">Loans</Tabs.Trigger>
        </Tabs.List>

        <Tabs.Content value="budgets" class="space-y-4">
          <Card.Root>
            <Card.Header><Card.Title>Budget Targets</Card.Title></Card.Header>
            <Card.Content class="grid gap-3 md:grid-cols-5">
              <Input bind:value={budgetName} aria-label="Budget name" />
              <select bind:value={budgetCategoryId} class="rounded-md border bg-background px-3 py-2 text-sm">
                {#each categories as category}<option value={category.id}>{category.name}</option>{/each}
              </select>
              <Input type="number" bind:value={budgetAmount} aria-label="Target amount minor" />
              <Button onclick={submitBudget}>Add Target</Button>
            </Card.Content>
          </Card.Root>
          <Card.Root>
            <Card.Header><Card.Title>Planned vs Actual</Card.Title></Card.Header>
            <Card.Content>
              <Table.Root>
                <Table.Header><Table.Row><Table.Head>Category</Table.Head><Table.Head>Target</Table.Head><Table.Head>Actual</Table.Head><Table.Head>Rollover</Table.Head><Table.Head>Remaining</Table.Head></Table.Row></Table.Header>
                <Table.Body>
                  {#each variance as row}
                    <Table.Row><Table.Cell>{categoryName(row.category_id)}</Table.Cell><Table.Cell>{money(row.target_minor)}</Table.Cell><Table.Cell>{money(row.actual_minor)}</Table.Cell><Table.Cell>{money(row.rollover_minor)}</Table.Cell><Table.Cell>{money(row.remaining_minor)}</Table.Cell></Table.Row>
                  {:else}
                    <Table.Row><Table.Cell colspan={5}>No budget variance yet</Table.Cell></Table.Row>
                  {/each}
                </Table.Body>
              </Table.Root>
            </Card.Content>
          </Card.Root>
        </Tabs.Content>

        <Tabs.Content value="schedules" class="space-y-4">
          <Card.Root>
            <Card.Header><Card.Title>Scheduled Transaction</Card.Title></Card.Header>
            <Card.Content class="grid gap-3 md:grid-cols-6">
              <Input bind:value={scheduleName} aria-label="Schedule name" />
              <Input type="date" bind:value={scheduleStart} aria-label="Start date" />
              <select bind:value={scheduleFrequency} class="rounded-md border bg-background px-3 py-2 text-sm"><option>daily</option><option>weekly</option><option>monthly</option><option>yearly</option></select>
              <select bind:value={scheduleFromAccountId} class="rounded-md border bg-background px-3 py-2 text-sm">{#each accounts as account}<option value={account.id}>{account.name}</option>{/each}</select>
              <Input type="number" bind:value={scheduleAmount} aria-label="Amount minor" />
              <Button onclick={submitSchedule}>Create</Button>
            </Card.Content>
          </Card.Root>
          <Card.Root>
            <Card.Header><Card.Title>Upcoming Instances</Card.Title></Card.Header>
            <Card.Content>
              <Table.Root>
                <Table.Header><Table.Row><Table.Head>Date</Table.Head><Table.Head>Schedule</Table.Head><Table.Head>Status</Table.Head><Table.Head></Table.Head></Table.Row></Table.Header>
                <Table.Body>
                  {#each occurrences as row}
                    <Table.Row>
                      <Table.Cell>{row.occurrence_date}</Table.Cell>
                      <Table.Cell>{schedules.find((schedule) => schedule.id === row.schedule_id)?.name}</Table.Cell>
                      <Table.Cell>{row.status}</Table.Cell>
                      <Table.Cell class="space-x-2">{#if row.status === "pending"}<Button size="sm" variant="outline" onclick={() => skipOccurrence(row)}>Skip</Button><Button size="sm" onclick={() => postOccurrence(row)}>Post</Button>{/if}</Table.Cell>
                    </Table.Row>
                  {:else}
                    <Table.Row><Table.Cell colspan={4}>No projected instances</Table.Cell></Table.Row>
                  {/each}
                </Table.Body>
              </Table.Root>
            </Card.Content>
          </Card.Root>
        </Tabs.Content>

        <Tabs.Content value="forecast">
          <Card.Root>
            <Card.Header><Card.Title>Projected Cash</Card.Title></Card.Header>
            <Card.Content>
              <Table.Root>
                <Table.Header><Table.Row><Table.Head>Date</Table.Head><Table.Head>Account</Table.Head><Table.Head>Delta</Table.Head><Table.Head>Projected Balance</Table.Head></Table.Row></Table.Header>
                <Table.Body>
                  {#each forecast as row}
                    <Table.Row><Table.Cell>{row.projection_date}</Table.Cell><Table.Cell>{accountName(row.account_id)}</Table.Cell><Table.Cell>{money(row.scheduled_delta_minor)}</Table.Cell><Table.Cell>{money(row.projected_balance_minor)}</Table.Cell></Table.Row>
                  {:else}
                    <Table.Row><Table.Cell colspan={4}>No scheduled cash movement in this window</Table.Cell></Table.Row>
                  {/each}
                </Table.Body>
              </Table.Root>
            </Card.Content>
          </Card.Root>
        </Tabs.Content>

        <Tabs.Content value="loans" class="space-y-4">
          <Card.Root>
            <Card.Header><Card.Title>Loan Setup</Card.Title></Card.Header>
            <Card.Content class="grid gap-3 md:grid-cols-6">
              <Input bind:value={loanName} aria-label="Loan name" />
              <select bind:value={loanAccountId} class="rounded-md border bg-background px-3 py-2 text-sm">{#each accounts as account}<option value={account.id}>{account.name}</option>{/each}</select>
              <Input type="number" bind:value={principal} aria-label="Principal minor" />
              <Input type="number" bind:value={rateBps} aria-label="Rate basis points" />
              <Input type="number" bind:value={termMonths} aria-label="Term months" />
              <Button onclick={submitLoan}>Create Loan</Button>
            </Card.Content>
          </Card.Root>
          <Card.Root>
            <Card.Header><Card.Title>Amortization</Card.Title></Card.Header>
            <Card.Content>
              <Table.Root>
                <Table.Header><Table.Row><Table.Head>#</Table.Head><Table.Head>Date</Table.Head><Table.Head>Payment</Table.Head><Table.Head>Principal</Table.Head><Table.Head>Interest</Table.Head><Table.Head>Remaining</Table.Head></Table.Row></Table.Header>
                <Table.Body>
                  {#each amortization.slice(0, 24) as row}
                    <Table.Row><Table.Cell>{row.period}</Table.Cell><Table.Cell>{row.payment_date}</Table.Cell><Table.Cell>{money(row.payment_minor)}</Table.Cell><Table.Cell>{money(row.principal_minor)}</Table.Cell><Table.Cell>{money(row.interest_minor)}</Table.Cell><Table.Cell>{money(row.remaining_principal_minor)}</Table.Cell></Table.Row>
                  {:else}
                    <Table.Row><Table.Cell colspan={6}>No loan schedule yet</Table.Cell></Table.Row>
                  {/each}
                </Table.Body>
              </Table.Root>
            </Card.Content>
          </Card.Root>
          <Card.Root>
            <Card.Header><Card.Title>Loan Payment Assistant</Card.Title></Card.Header>
            <Card.Content class="space-y-4">
              <div class="grid gap-3 md:grid-cols-5">
                <select bind:value={paymentLoanId} class="rounded-md border bg-background px-3 py-2 text-sm">
                  <option value={0}>Select loan</option>
                  {#each loans as loan}<option value={loan.id}>{loan.name}</option>{/each}
                </select>
                <Input type="date" bind:value={paymentDate} aria-label="Payment date" />
                <select bind:value={paymentAccountId} class="rounded-md border bg-background px-3 py-2 text-sm">
                  {#each accounts as account}<option value={account.id}>{account.name}</option>{/each}
                </select>
                <Input bind:value={paymentMemo} aria-label="Payment memo" />
                <div class="flex gap-2">
                  <Button variant="outline" onclick={previewLoanPayment} disabled={!paymentLoanId}>Draft</Button>
                  <Button onclick={submitLoanPayment} disabled={!paymentLoanId}>Post</Button>
                </div>
              </div>
              {#if loanPaymentPreview}
                <p class="text-sm text-muted-foreground">{loanPaymentPreview}</p>
              {/if}
            </Card.Content>
          </Card.Root>
        </Tabs.Content>
      </Tabs.Root>
    {/if}
  </div>
</main>
