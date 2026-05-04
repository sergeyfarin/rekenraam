<script lang="ts">
  import {
    dbIntegrityCheck,
    getAdminRuntimeStatus,
    type AdminRuntimeStatus,
  } from "$lib/api/admin";
  import * as Card from "$lib/components/ui/card";
  import { Button } from "$lib/components/ui/button";
  import * as Alert from "$lib/components/ui/alert";

  export let busy = false;

  let error = "";
  let runtimeStatus: AdminRuntimeStatus | null = null;
  let integrityStatus = "";
  let maintenanceStatus = "";
  let maintenanceError = "";

  export async function initialize() {
    await refreshMaintenance();
  }

  function formatBytes(bytes: number) {
    if (!bytes) return "0 B";
    const units = ["B", "KB", "MB", "GB", "TB"];
    let size = bytes;
    let unitIndex = 0;
    while (size >= 1024 && unitIndex < units.length - 1) {
      size /= 1024;
      unitIndex += 1;
    }
    return `${size.toFixed(unitIndex === 0 ? 0 : 1)} ${units[unitIndex]}`;
  }

  async function refreshMaintenance() {
    maintenanceError = "";
    try {
      runtimeStatus = await getAdminRuntimeStatus();
    } catch (e) {
      maintenanceError = `Failed to load maintenance info: ${String(e)}`;
    }
  }

  async function runIntegrityCheck() {
    maintenanceError = "";
    maintenanceStatus = "";
    integrityStatus = "";
    busy = true;
    try {
      integrityStatus = await dbIntegrityCheck();
      maintenanceStatus = integrityStatus === "ok" ? "Integrity check passed." : "Integrity check reported issues.";
    } catch (e) {
      maintenanceError = `Integrity check failed: ${String(e)}`;
    } finally {
      busy = false;
    }
  }
</script>

<Card.Root>
  <Card.Header>
    <Card.Title>Runtime</Card.Title>
    <Card.Description>Server-managed database information for the web deployment</Card.Description>
  </Card.Header>
  <Card.Content>
    {#if runtimeStatus}
      <div class="space-y-3 text-sm">
        <div>
          <p class="text-muted-foreground">Connection</p>
          <p class="font-mono">{runtimeStatus.display_path}</p>
        </div>
        <div class="grid gap-4 md:grid-cols-2 lg:grid-cols-4">
          <div>
            <p class="text-muted-foreground">Engine</p>
            <p>{runtimeStatus.database_kind}</p>
          </div>
          <div>
            <p class="text-muted-foreground">Database</p>
            <p>{runtimeStatus.database_name}</p>
          </div>
          <div>
            <p class="text-muted-foreground">Writable</p>
            <p>{runtimeStatus.writable ? "Yes" : "No"}</p>
          </div>
          <div>
            <p class="text-muted-foreground">Size</p>
            <p>{runtimeStatus.size_bytes === null ? "—" : formatBytes(runtimeStatus.size_bytes)}</p>
          </div>
        </div>
      </div>
      <Alert.Root class="mt-4">
        <Alert.Title>Web semantics</Alert.Title>
        <Alert.Description>{runtimeStatus.note}</Alert.Description>
      </Alert.Root>
    {:else}
      <p class="text-sm text-muted-foreground">Runtime information unavailable.</p>
    {/if}

    {#if error}
      <p class="text-sm text-destructive mt-4">{error}</p>
    {/if}
  </Card.Content>
</Card.Root>

<Card.Root>
  <Card.Header>
    <Card.Title>Storage Operations</Card.Title>
  </Card.Header>
  <Card.Content>
    <Alert.Root>
      <Alert.Title>Server-managed operations</Alert.Title>
      <Alert.Description>
        Database file switching, restore-from-file, and backup-folder selection were desktop-only flows.
        In the web deployment, storage and backup policy must be managed on the server or platform level.
      </Alert.Description>
    </Alert.Root>
  </Card.Content>
</Card.Root>

<Card.Root>
  <Card.Header>
    <Card.Title>Maintenance &amp; Health</Card.Title>
  </Card.Header>
  <Card.Content>
    {#if runtimeStatus && !runtimeStatus.writable}
      <Alert.Root variant="destructive" class="mb-4">
        <Alert.Title>Read-only</Alert.Title>
        <Alert.Description>Database is read-only. Write operations and migrations are disabled.</Alert.Description>
      </Alert.Root>
    {/if}

    <div class="grid gap-4 md:grid-cols-2 lg:grid-cols-4">
      <div>
        <p class="text-sm text-muted-foreground">Schema version</p>
        <p>
          {runtimeStatus ? `${runtimeStatus.current_version ?? "—"} / ${runtimeStatus.latest_version}` : "—"}
        </p>
        {#if runtimeStatus && runtimeStatus.pending_versions.length > 0}
          <p class="text-sm text-yellow-600">
            Pending migrations: {runtimeStatus.pending_versions.join(", ")}
          </p>
        {/if}
      </div>
      <div>
        <p class="text-sm text-muted-foreground">DB size</p>
        <p>{runtimeStatus?.size_bytes === null || runtimeStatus === null ? "—" : formatBytes(runtimeStatus.size_bytes)}</p>
      </div>
      <div>
        <p class="text-sm text-muted-foreground">Foreign keys</p>
        <p>{runtimeStatus ? (runtimeStatus.foreign_keys ? "Enabled" : "Disabled") : "—"}</p>
      </div>
      <div>
        <p class="text-sm text-muted-foreground">Host</p>
        <p>{runtimeStatus?.database_host || "—"}</p>
      </div>
    </div>

    <div class="flex flex-wrap gap-2 mt-4">
      <Button variant="secondary" onclick={refreshMaintenance} disabled={busy}>
        Refresh status
      </Button>
      <Button variant="secondary" onclick={runIntegrityCheck} disabled={busy}>
        Run integrity check
      </Button>
    </div>

    {#if integrityStatus}
      <p class="text-sm text-muted-foreground mt-4">Integrity status: {integrityStatus}</p>
    {/if}
    {#if maintenanceStatus}
      <p class="text-sm text-green-600 mt-4">{maintenanceStatus}</p>
    {/if}
    {#if maintenanceError}
      <p class="text-sm text-destructive mt-4">{maintenanceError}</p>
    {/if}
  </Card.Content>
</Card.Root>
