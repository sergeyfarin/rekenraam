<script lang="ts">
  import { Button } from "$lib/components/ui/button";
  import { Input } from "$lib/components/ui/input";
  import { Label } from "$lib/components/ui/label";
  import { Badge } from "$lib/components/ui/badge";
  import * as Card from "$lib/components/ui/card";
  import * as Dialog from "$lib/components/ui/dialog";
  import * as Table from "$lib/components/ui/table";
  import type {
    Currency,
    FxAssignmentForm,
    FxRateRefreshState,
    FxRateSettings,
    FxRateSource,
    FxRateSourceAssignment,
    FxRefreshExecutionStatus,
    FxRefreshRunSummary,
  } from "./types";

  export let currencies: Currency[] = [];
  export let fxRateSources: FxRateSource[] = [];
  export let fxRateSettings: FxRateSettings | null = null;
  export let fxRateAssignments: FxRateSourceAssignment[] = [];
  export let fxRateRefreshState: FxRateRefreshState[] = [];
  export let fxRefreshExecutionStatus: FxRefreshExecutionStatus | null = null;
  export let fxRefreshRunHistory: FxRefreshRunSummary[] = [];
  export let pricingAutomationManagedByServer = false;
  export let busy = false;
  export let fxRateStatus = "";
  export let fxRateError = "";
  export let showFxAssignmentDialog = false;
  export let editingAssignment: FxRateSourceAssignment | null = null;
  export let newFxAssignment: FxAssignmentForm = {
    from_currency_id: 0,
    to_currency_id: 0,
    source_id: 0,
    effective_from: "",
    effective_to: "",
  };
  export let onRefreshNow: () => void | Promise<void> = () => {};
  export let onSaveSettings: () => void | Promise<void> = () => {};
  export let onOpenNewAssignment: () => void = () => {};
  export let onOpenEditAssignment: (assignment: FxRateSourceAssignment) => void = () => {};
  export let onCloseAssignmentDialog: () => void = () => {};
  export let onSaveAssignment: () => void | Promise<void> = () => {};
  export let onDeleteAssignment: (assignment: FxRateSourceAssignment) => void | Promise<void> = () => {};
  export let onReload: () => void | Promise<void> = () => {};
</script>

<div class="flex items-center justify-between mb-4 gap-4">
  <div>
    <p class="text-sm text-muted-foreground">Configure pricing refresh policy and manage source assignments. In the web app, scheduled execution is owned by a backend worker rather than the browser session.</p>
  </div>
  <div class="flex gap-2">
    <Button onclick={onRefreshNow} disabled={busy || (pricingAutomationManagedByServer && fxRefreshExecutionStatus?.is_running)} size="sm">
      {pricingAutomationManagedByServer ? "Run Refresh Now" : "Refresh Now"}
    </Button>
    <Button onclick={onSaveSettings} disabled={busy || !fxRateSettings} size="sm" variant="secondary">Save Settings</Button>
  </div>
</div>

{#if pricingAutomationManagedByServer}
  <p class="text-sm text-muted-foreground mb-4">Refresh jobs are owned by the backend scheduler using the policy and assignments below. Manual runs request the backend worker directly and do not start browser-side automation.</p>
{/if}

{#if fxRateStatus}
  <p class="text-sm text-status-success mb-2">{fxRateStatus}</p>
{/if}
{#if fxRateError}
  <p class="text-sm text-destructive mb-2">{fxRateError}</p>
{/if}

{#if fxRateSettings}
  <div class="grid gap-4 lg:grid-cols-3 mb-6">
    <Card.Root>
      <Card.Header>
        <Card.Title>Base Currency</Card.Title>
      </Card.Header>
      <Card.Content class="space-y-3">
        <div class="grid gap-2">
          <Label for="fx-base-currency">Base currency</Label>
          <select id="fx-base-currency" class="flex h-9 w-full rounded-md border border-input bg-transparent px-3 py-1 text-sm shadow-sm transition-colors focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring" bind:value={fxRateSettings.base_currency_id}>
            {#each currencies as currency}
              <option value={currency.id}>{currency.display_symbol || ""} {currency.symbol} - {currency.name}</option>
            {/each}
          </select>
        </div>
        <div class="grid gap-2">
          <Label for="fx-default-source">Default source</Label>
          <select id="fx-default-source" class="flex h-9 w-full rounded-md border border-input bg-transparent px-3 py-1 text-sm shadow-sm transition-colors focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring" bind:value={fxRateSettings.default_source_id}>
            <option value="">Select source</option>
            {#each fxRateSources as source}
              <option value={source.id}>{source.name}</option>
            {/each}
          </select>
        </div>
      </Card.Content>
    </Card.Root>

    <Card.Root>
      <Card.Header>
        <Card.Title>Refresh Schedule</Card.Title>
      </Card.Header>
      <Card.Content class="space-y-3">
        <div class="flex items-center justify-between">
          <Label for="fx-refresh-enabled">Enable refresh</Label>
          <input id="fx-refresh-enabled" type="checkbox" class="h-4 w-4" bind:checked={fxRateSettings.refresh_enabled} />
        </div>
        <div class="grid grid-cols-2 gap-3">
          <div class="grid gap-2">
            <Label for="fx-refresh-hour">Hour (UTC)</Label>
            <Input id="fx-refresh-hour" type="number" min="0" max="23" bind:value={fxRateSettings.refresh_hour_utc} />
          </div>
          <div class="grid gap-2">
            <Label for="fx-refresh-minute">Minute (UTC)</Label>
            <Input id="fx-refresh-minute" type="number" min="0" max="59" bind:value={fxRateSettings.refresh_minute_utc} />
          </div>
        </div>
      </Card.Content>
    </Card.Root>

    <Card.Root>
      <Card.Header>
        <Card.Title>Backfill Policy</Card.Title>
      </Card.Header>
      <Card.Content class="space-y-3">
        <div class="grid gap-2">
          <Label for="fx-backfill-days">Max backfill days</Label>
          <Input id="fx-backfill-days" type="number" min="1" bind:value={fxRateSettings.max_backfill_days} />
        </div>
        <div class="grid gap-2">
          <Label for="fx-weekend-policy">Weekend policy</Label>
          <select id="fx-weekend-policy" class="flex h-9 w-full rounded-md border border-input bg-transparent px-3 py-1 text-sm shadow-sm transition-colors focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring" bind:value={fxRateSettings.weekend_policy}>
            <option value="skip">Skip</option>
            <option value="fill_previous">Fill previous</option>
            <option value="download">Download</option>
          </select>
        </div>
      </Card.Content>
    </Card.Root>
  </div>
{/if}

{#if pricingAutomationManagedByServer && fxRefreshExecutionStatus}
  <div class="grid gap-4 lg:grid-cols-3 mb-6">
    <Card.Root>
      <Card.Header>
        <Card.Title>Worker Status</Card.Title>
      </Card.Header>
      <Card.Content class="space-y-2 text-sm">
        <div class="flex items-center justify-between">
          <span class="text-muted-foreground">Scheduler</span>
          <Badge variant={fxRefreshExecutionStatus.scheduler_enabled ? "secondary" : "outline"}>
            {fxRefreshExecutionStatus.scheduler_enabled ? "Enabled" : "Disabled"}
          </Badge>
        </div>
        <div class="flex items-center justify-between">
          <span class="text-muted-foreground">Run state</span>
          <Badge variant={fxRefreshExecutionStatus.is_running ? "secondary" : "outline"}>
            {fxRefreshExecutionStatus.is_running ? "Running" : "Idle"}
          </Badge>
        </div>
        <div>
          <span class="text-muted-foreground">Poll interval:</span>
          <span class="ml-2">{fxRefreshExecutionStatus.scheduler_poll_seconds}s</span>
        </div>
        <div>
          <span class="text-muted-foreground">Next scheduled:</span>
          <span class="ml-2">{fxRefreshExecutionStatus.next_scheduled_at || "—"}</span>
        </div>
        <div>
          <span class="text-muted-foreground">Worker started:</span>
          <span class="ml-2">{fxRefreshExecutionStatus.worker_started_at || "—"}</span>
        </div>
      </Card.Content>
    </Card.Root>

    <Card.Root>
      <Card.Header>
        <Card.Title>Last Run</Card.Title>
      </Card.Header>
      <Card.Content class="space-y-2 text-sm">
        {#if fxRefreshExecutionStatus.last_run}
          <div>
            <span class="text-muted-foreground">Trigger:</span>
            <span class="ml-2 capitalize">{fxRefreshExecutionStatus.last_run.trigger}</span>
          </div>
          <div>
            <span class="text-muted-foreground">Finished:</span>
            <span class="ml-2">{fxRefreshExecutionStatus.last_run.finished_at}</span>
          </div>
          <div>
            <span class="text-muted-foreground">Pairs:</span>
            <span class="ml-2">{fxRefreshExecutionStatus.last_run.pairs_success}/{fxRefreshExecutionStatus.last_run.pairs_total}</span>
          </div>
          <div>
            <span class="text-muted-foreground">Rates inserted:</span>
            <span class="ml-2">{fxRefreshExecutionStatus.last_run.rates_inserted}</span>
          </div>
        {:else}
          <p class="text-muted-foreground">No execution run recorded yet.</p>
        {/if}
      </Card.Content>
    </Card.Root>

    <Card.Root>
      <Card.Header>
        <Card.Title>Backend Ownership</Card.Title>
      </Card.Header>
      <Card.Content class="space-y-2 text-sm text-muted-foreground">
        <p>Policy changes update the server-owned schedule.</p>
        <p>Manual runs queue work on the backend worker instead of the browser.</p>
        <p>Refresh-state rows below reflect pair-level outcomes from those backend runs.</p>
      </Card.Content>
    </Card.Root>
  </div>
{/if}

<div class="flex items-center justify-between mb-2">
  <h3 class="text-lg font-semibold">Source Assignments</h3>
  <Button onclick={onOpenNewAssignment} disabled={busy} size="sm">Add Assignment</Button>
</div>
<Table.Root>
  <Table.Header>
    <Table.Row>
      <Table.Head>Pair</Table.Head>
      <Table.Head>Source</Table.Head>
      <Table.Head>Effective</Table.Head>
      <Table.Head class="text-right">Actions</Table.Head>
    </Table.Row>
  </Table.Header>
  <Table.Body>
    {#each fxRateAssignments as assignment}
      <Table.Row>
        <Table.Cell class="font-mono">{assignment.from_currency_symbol}/{assignment.to_currency_symbol}</Table.Cell>
        <Table.Cell>{assignment.source_name}</Table.Cell>
        <Table.Cell>{assignment.effective_from}{assignment.effective_to ? ` → ${assignment.effective_to}` : ""}</Table.Cell>
        <Table.Cell class="text-right">
          <Button variant="ghost" size="sm" onclick={() => onOpenEditAssignment(assignment)}>Edit</Button>
          <Button variant="ghost" size="sm" class="text-destructive" onclick={() => onDeleteAssignment(assignment)}>Delete</Button>
        </Table.Cell>
      </Table.Row>
    {:else}
      <Table.Row>
        <Table.Cell colspan={4} class="text-muted-foreground">No source assignments configured.</Table.Cell>
      </Table.Row>
    {/each}
  </Table.Body>
</Table.Root>

{#if pricingAutomationManagedByServer}
  <div class="flex items-center justify-between mt-6 mb-2">
    <h3 class="text-lg font-semibold">Run History</h3>
    <Button variant="secondary" size="sm" onclick={onReload} disabled={busy}>Reload Status</Button>
  </div>
  <Table.Root>
    <Table.Header>
      <Table.Row>
        <Table.Head>Finished</Table.Head>
        <Table.Head>Trigger</Table.Head>
        <Table.Head>Pairs</Table.Head>
        <Table.Head>Rates</Table.Head>
        <Table.Head>Status</Table.Head>
      </Table.Row>
    </Table.Header>
    <Table.Body>
      {#each fxRefreshRunHistory as run}
        <Table.Row>
          <Table.Cell>{run.finished_at}</Table.Cell>
          <Table.Cell class="capitalize">{run.trigger}</Table.Cell>
          <Table.Cell>{run.pairs_success}/{run.pairs_total}</Table.Cell>
          <Table.Cell>{run.rates_inserted}</Table.Cell>
          <Table.Cell>
            {#if run.last_error}
              <Badge variant="destructive">Error</Badge>
            {:else}
              <Badge variant="secondary">OK</Badge>
            {/if}
          </Table.Cell>
        </Table.Row>
        {#if run.last_error}
          <Table.Row>
            <Table.Cell colspan={5} class="text-xs text-destructive">{run.last_error}</Table.Cell>
          </Table.Row>
        {/if}
      {:else}
        <Table.Row>
          <Table.Cell colspan={5} class="text-muted-foreground">No refresh runs recorded yet.</Table.Cell>
        </Table.Row>
      {/each}
    </Table.Body>
  </Table.Root>
{/if}

<div class="flex items-center justify-between mt-6 mb-2">
  <h3 class="text-lg font-semibold">Refresh Status</h3>
  <Button variant="secondary" size="sm" onclick={onReload} disabled={busy}>Reload</Button>
</div>
<Table.Root>
  <Table.Header>
    <Table.Row>
      <Table.Head>Pair</Table.Head>
      <Table.Head>Source</Table.Head>
      <Table.Head>Last Success</Table.Head>
      <Table.Head>Last Attempt</Table.Head>
      <Table.Head>Status</Table.Head>
    </Table.Row>
  </Table.Header>
  <Table.Body>
    {#each fxRateRefreshState as state}
      <Table.Row>
        <Table.Cell class="font-mono">{state.from_currency_symbol}/{state.to_currency_symbol}</Table.Cell>
        <Table.Cell>{state.source_name || "—"}</Table.Cell>
        <Table.Cell>{state.last_success_date || "—"}</Table.Cell>
        <Table.Cell>{state.last_attempt_at || "—"}</Table.Cell>
        <Table.Cell>
          {#if state.last_error}
            <Badge variant="destructive">Error</Badge>
          {:else}
            <Badge variant="secondary">OK</Badge>
          {/if}
        </Table.Cell>
      </Table.Row>
      {#if state.last_error}
        <Table.Row>
          <Table.Cell colspan={5} class="text-xs text-destructive">{state.last_error}</Table.Cell>
        </Table.Row>
      {/if}
    {:else}
      <Table.Row>
        <Table.Cell colspan={5} class="text-muted-foreground">No refresh status yet.</Table.Cell>
      </Table.Row>
    {/each}
  </Table.Body>
</Table.Root>

<Dialog.Root bind:open={showFxAssignmentDialog}>
  <Dialog.Content>
    <Dialog.Header>
      <Dialog.Title>{editingAssignment ? "Edit FX Source Assignment" : "Add FX Source Assignment"}</Dialog.Title>
    </Dialog.Header>
    <div class="grid gap-4 py-4">
      <div class="grid gap-2">
        <Label for="fx-assignment-from">From Currency</Label>
        <select id="fx-assignment-from" class="flex h-9 w-full rounded-md border border-input bg-transparent px-3 py-1 text-sm shadow-sm transition-colors focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring" bind:value={newFxAssignment.from_currency_id}>
          {#each currencies as currency}
            <option value={currency.id}>{currency.display_symbol || ""} {currency.symbol} - {currency.name}</option>
          {/each}
        </select>
      </div>
      <div class="grid gap-2">
        <Label for="fx-assignment-to">To Currency</Label>
        <select id="fx-assignment-to" class="flex h-9 w-full rounded-md border border-input bg-transparent px-3 py-1 text-sm shadow-sm transition-colors focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring" bind:value={newFxAssignment.to_currency_id}>
          {#each currencies as currency}
            <option value={currency.id}>{currency.display_symbol || ""} {currency.symbol} - {currency.name}</option>
          {/each}
        </select>
      </div>
      <div class="grid gap-2">
        <Label for="fx-assignment-source">Source</Label>
        <select id="fx-assignment-source" class="flex h-9 w-full rounded-md border border-input bg-transparent px-3 py-1 text-sm shadow-sm transition-colors focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring" bind:value={newFxAssignment.source_id}>
          {#each fxRateSources as source}
            <option value={source.id}>{source.name}</option>
          {/each}
        </select>
      </div>
      <div class="grid grid-cols-2 gap-4">
        <div class="grid gap-2">
          <Label for="fx-assignment-from-date">Effective From</Label>
          <Input id="fx-assignment-from-date" type="date" bind:value={newFxAssignment.effective_from} />
        </div>
        <div class="grid gap-2">
          <Label for="fx-assignment-to-date">Effective To</Label>
          <Input id="fx-assignment-to-date" type="date" bind:value={newFxAssignment.effective_to} />
        </div>
      </div>
    </div>
    <Dialog.Footer>
      <Button variant="secondary" onclick={onCloseAssignmentDialog}>Cancel</Button>
      <Button onclick={onSaveAssignment} disabled={busy || !newFxAssignment.source_id || !newFxAssignment.effective_from}>
        {editingAssignment ? "Update" : "Create"}
      </Button>
    </Dialog.Footer>
  </Dialog.Content>
</Dialog.Root>