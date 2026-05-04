<script lang="ts">
  import { onMount } from "svelte";
  import { Download, Upload } from "@lucide/svelte";
  import * as Card from "$lib/components/ui/card";
  import { Button } from "$lib/components/ui/button";
  import { Input } from "$lib/components/ui/input";
  import { Label } from "$lib/components/ui/label";
  import { listAccounts, type AccountSummary } from "$lib/api/accounts";
  import { commitImport, previewImport, type ImportPreviewResult } from "$lib/api/imports";
  import { exportAccountsCsv, exportRegisterQif, exportTransactionsCsv } from "$lib/api/exports";

  let accounts: AccountSummary[] = [];
  let accountId = 0;
  let format = "auto";
  let csvDelimiter = ",";
  let decimalSeparator = ".";
  let thousandsSeparator = "";
  let dateFormat = "";
  let createPayees = true;
  let mode = "match_and_enrich";
  let selectedFile: File | null = null;
  let preview: ImportPreviewResult | null = null;
  let busy = false;
  let error = "";
  let result = "";

  onMount(async () => {
    accounts = await listAccounts();
    accountId = accounts.find((account) => !account.is_closed)?.id ?? accounts[0]?.id ?? 0;
  });

  async function readFilePayload(file: File): Promise<{ content?: string; content_base64?: string }> {
    if (file.name.toLowerCase().endsWith(".xlsx") || file.name.toLowerCase().endsWith(".xls")) {
      const buffer = await file.arrayBuffer();
      let binary = "";
      for (const byte of new Uint8Array(buffer)) {
        binary += String.fromCharCode(byte);
      }
      return { content_base64: btoa(binary) };
    }
    return { content: await file.text() };
  }

  function locale() {
    return {
      csv_delimiter: csvDelimiter || null,
      decimal_separator: decimalSeparator || null,
      thousands_separator: thousandsSeparator || null,
      date_format: dateFormat || null
    };
  }

  async function runPreview() {
    if (!selectedFile || !accountId) return;
    busy = true;
    error = "";
    result = "";
    try {
      preview = await previewImport({
        book_id: 1,
        account_id: accountId,
        format,
        file_name: selectedFile.name,
        locale: locale(),
        ...(await readFilePayload(selectedFile))
      });
      if (preview.file_error) {
        error = preview.file_error;
      }
    } catch (e) {
      error = e instanceof Error ? e.message : String(e);
    } finally {
      busy = false;
    }
  }

  async function runCommit() {
    if (!preview || !accountId) return;
    busy = true;
    error = "";
    result = "";
    try {
      const committed = await commitImport({
        book_id: 1,
        account_id: accountId,
        drafts: preview.rows.filter((row) => row.draft && !row.error).map((row) => row.draft),
        create_payees: createPayees,
        mode,
        source: "web",
        file_name: selectedFile?.name ?? null,
        file_size: selectedFile?.size ?? null
      });
      result = `Session ${committed.session.id}: ${committed.batch.created_tx_ids.length} created, ${committed.batch.matched_tx_ids.length} matched, ${committed.batch.updated_tx_ids.length} updated, ${committed.batch.skipped} skipped.`;
    } catch (e) {
      error = e instanceof Error ? e.message : String(e);
    } finally {
      busy = false;
    }
  }

  function saveText(filename: string, content: string) {
    const url = URL.createObjectURL(new Blob([content], { type: "text/plain;charset=utf-8" }));
    const link = document.createElement("a");
    link.href = url;
    link.download = filename;
    link.click();
    URL.revokeObjectURL(url);
  }

  async function download(kind: "accounts" | "transactions" | "qif") {
    busy = true;
    error = "";
    try {
      if (kind === "accounts") {
        saveText("accounts.csv", await exportAccountsCsv());
      } else if (kind === "transactions") {
        saveText("transactions.csv", await exportTransactionsCsv());
      } else if (accountId) {
        saveText(`register-${accountId}.qif`, await exportRegisterQif(accountId));
      }
    } catch (e) {
      error = e instanceof Error ? e.message : String(e);
    } finally {
      busy = false;
    }
  }

  $: validRows = preview?.rows.filter((row) => row.draft && !row.error).length ?? 0;
  $: duplicateRows = preview?.rows.filter((row) => row.is_duplicate).length ?? 0;
  $: errorRows = preview?.rows.filter((row) => row.error).length ?? 0;
</script>

<main class="py-6">
  <div class="container mx-auto space-y-6 px-6">
    <div>
      <h1 class="text-3xl font-bold tracking-tight">Import/Export</h1>
      <p class="text-muted-foreground">Move bank files and core data through the server API.</p>
    </div>

    {#if error}
      <div class="rounded-md border border-destructive/40 bg-destructive/10 px-4 py-3 text-sm text-destructive">
        {error}
      </div>
    {/if}
    {#if result}
      <div class="rounded-md border border-border bg-muted px-4 py-3 text-sm">{result}</div>
    {/if}

    <div class="grid gap-6 lg:grid-cols-[minmax(0,1fr)_320px]">
      <section class="space-y-4">
        <Card.Root>
          <Card.Header>
            <Card.Title>Import</Card.Title>
            <Card.Description>CSV, XLS/XLSX, QIF, and OFX/QFX</Card.Description>
          </Card.Header>
          <Card.Content class="space-y-4">
            <div class="grid gap-4 md:grid-cols-2">
              <div class="space-y-2">
                <Label for="account">Account</Label>
                <select id="account" bind:value={accountId} class="h-10 rounded-md border border-input bg-background px-3 text-sm">
                  {#each accounts as account}
                    <option value={account.id}>{account.name}</option>
                  {/each}
                </select>
              </div>
              <div class="space-y-2">
                <Label for="format">Format</Label>
                <select id="format" bind:value={format} class="h-10 rounded-md border border-input bg-background px-3 text-sm">
                  <option value="auto">Auto</option>
                  <option value="csv">CSV</option>
                  <option value="xlsx">XLS/XLSX</option>
                  <option value="qif">QIF</option>
                  <option value="ofx">OFX/QFX</option>
                </select>
              </div>
            </div>

            <Input type="file" accept=".csv,.qif,.ofx,.qfx,.xls,.xlsx" onchange={(e) => selectedFile = e.currentTarget.files?.[0] ?? null} />

            <div class="grid gap-4 md:grid-cols-4">
              <div class="space-y-2">
                <Label for="delimiter">Delimiter</Label>
                <Input id="delimiter" bind:value={csvDelimiter} maxlength={1} />
              </div>
              <div class="space-y-2">
                <Label for="decimal">Decimal</Label>
                <Input id="decimal" bind:value={decimalSeparator} maxlength={1} />
              </div>
              <div class="space-y-2">
                <Label for="thousands">Thousands</Label>
                <Input id="thousands" bind:value={thousandsSeparator} maxlength={1} />
              </div>
              <div class="space-y-2">
                <Label for="date-format">Date Format</Label>
                <Input id="date-format" bind:value={dateFormat} placeholder="YYYY-MM-DD" />
              </div>
            </div>

            <div class="flex flex-wrap items-center gap-3">
              <Button onclick={runPreview} disabled={busy || !selectedFile || !accountId}>
                <Upload class="mr-2 size-4" /> Preview
              </Button>
              <Button onclick={runCommit} disabled={busy || !preview || validRows === 0}>
                Commit
              </Button>
              <label class="flex items-center gap-2 text-sm">
                <input type="checkbox" bind:checked={createPayees} />
                Create payees
              </label>
              <select bind:value={mode} class="h-10 rounded-md border border-input bg-background px-3 text-sm">
                <option value="match_and_enrich">Match and enrich</option>
                <option value="deduplicate">Deduplicate</option>
                <option value="always_create">Always create</option>
                <option value="review">Review</option>
                <option value="update_only">Update only</option>
              </select>
            </div>
          </Card.Content>
        </Card.Root>

        {#if preview}
          <Card.Root>
            <Card.Header>
              <Card.Title>Preview</Card.Title>
              <Card.Description>{validRows} valid, {duplicateRows} duplicate, {errorRows} errors</Card.Description>
            </Card.Header>
            <Card.Content>
              <div class="overflow-x-auto">
                <table class="w-full text-sm">
                  <thead class="border-b text-left">
                    <tr>
                      <th class="py-2 pr-3">Row</th>
                      <th class="py-2 pr-3">Date</th>
                      <th class="py-2 pr-3">Amount</th>
                      <th class="py-2 pr-3">Payee</th>
                      <th class="py-2 pr-3">Status</th>
                    </tr>
                  </thead>
                  <tbody>
                    {#each preview.rows as row}
                      <tr class="border-b last:border-0">
                        <td class="py-2 pr-3">{row.row_number}</td>
                        <td class="py-2 pr-3">{row.draft?.txn_date ?? ""}</td>
                        <td class="py-2 pr-3">{row.draft?.amount_minor ?? ""}</td>
                        <td class="py-2 pr-3">{row.draft?.payee_name ?? ""}</td>
                        <td class="py-2 pr-3">
                          {#if row.error}
                            <span class="text-destructive">{row.error}</span>
                          {:else if row.is_duplicate}
                            matched #{row.matched_tx_id}
                          {:else}
                            ready
                          {/if}
                        </td>
                      </tr>
                    {/each}
                  </tbody>
                </table>
              </div>
            </Card.Content>
          </Card.Root>
        {/if}
      </section>

      <aside>
        <Card.Root>
          <Card.Header>
            <Card.Title>Export</Card.Title>
            <Card.Description>CSV and QIF downloads</Card.Description>
          </Card.Header>
          <Card.Content class="grid gap-3">
            <Button variant="outline" onclick={() => download("accounts")} disabled={busy}>
              <Download class="mr-2 size-4" /> Accounts CSV
            </Button>
            <Button variant="outline" onclick={() => download("transactions")} disabled={busy}>
              <Download class="mr-2 size-4" /> Transactions CSV
            </Button>
            <Button variant="outline" onclick={() => download("qif")} disabled={busy || !accountId}>
              <Download class="mr-2 size-4" /> Register QIF
            </Button>
          </Card.Content>
        </Card.Root>
      </aside>
    </div>
  </div>
</main>
