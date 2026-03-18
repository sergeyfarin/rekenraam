<script lang="ts">
  import { invoke } from "@tauri-apps/api/core";
  import * as Card from "$lib/components/ui/card";
  import { Button } from "$lib/components/ui/button";
  import { Input } from "$lib/components/ui/input";
  import { Label } from "$lib/components/ui/label";
  import * as Dialog from "$lib/components/ui/dialog";
  import * as Table from "$lib/components/ui/table";
  import * as Alert from "$lib/components/ui/alert";
  import ConfirmDialog from "$lib/components/ConfirmDialog.svelte";

  type BackupSettings = {
    enabled: boolean;
    interval_minutes: number;
    retention_count: number;
    backup_path: string | null;
    backup_on_close: boolean;
  };

  type BackupInfo = {
    path: string;
    size_bytes: number;
    modified_unix: number | null;
  };

  type DbStats = {
    path: string;
    size_bytes: number;
    modified_unix: number | null;
    writable: boolean;
    journal_mode: string | null;
    foreign_keys: boolean;
  };

  type MigrationStatus = {
    current_version: number;
    latest_version: number;
    pending_versions: number[];
  };

  export let busy = false;

  let dbPath = "";
  let error = "";
  let status = "";

  let backupSettings: BackupSettings = {
    enabled: false,
    interval_minutes: 60,
    retention_count: 60,
    backup_path: null,
    backup_on_close: true,
  };
  let backups: BackupInfo[] = [];
  let backupStatus = "";
  let backupError = "";
  let backupSettingsLoaded = false;
  let backupSaveTimer: ReturnType<typeof setTimeout> | null = null;
  let showBackupList = false;

  let confirmOpen = false;
  let confirmMessage = "";
  let confirmResolve: ((v: boolean) => void) | null = null;
  function askConfirm(msg: string): Promise<boolean> {
    confirmMessage = msg;
    confirmOpen = true;
    return new Promise<boolean>((resolve) => { confirmResolve = resolve; });
  }
  function onConfirmYes() { confirmOpen = false; confirmResolve?.(true); confirmResolve = null; }
  function onConfirmNo() { confirmOpen = false; confirmResolve?.(false); confirmResolve = null; }

  let dbStats: DbStats | null = null;
  let migrationStatus: MigrationStatus | null = null;
  let integrityStatus = "";
  let maintenanceStatus = "";
  let maintenanceError = "";

  export async function initialize() {
    await loadDbPath();
    await loadBackupSettings();
    await refreshBackups();
    await refreshMaintenance();
  }

  async function loadDbPath() {
    try {
      const path = await invoke<string | null>("get_db_path");
      dbPath = path ?? "";
    } catch (e) {
      error = `Failed to load DB path: ${String(e)}`;
    }
  }

  async function openDatabaseFile() {
    error = "";
    status = "";
    const selected = await invoke<string | null>("pick_storage_file");
    if (!selected) return;
    await switchDatabase(selected);
  }

  async function openDatabaseFolder() {
    error = "";
    status = "";
    const selected = await invoke<string | null>("pick_storage_folder");
    if (!selected) return;
    await switchDatabase(selected);
  }

  async function loadBackupSettings() {
    try {
      backupSettings = await invoke<BackupSettings>("get_backup_settings");
      backupSettingsLoaded = true;
    } catch (e) {
      backupError = `Failed to load backup settings: ${String(e)}`;
    }
  }

  async function saveBackupSettings(showStatus = true) {
    backupError = "";
    if (showStatus) {
      backupStatus = "";
    }
    try {
      const normalized = {
        ...backupSettings,
        interval_minutes: Number(backupSettings.interval_minutes),
        retention_count: Number(backupSettings.retention_count),
      };
      backupSettings = await invoke<BackupSettings>("set_backup_settings", { settings: normalized });
      if (showStatus) {
        backupStatus = "Backup settings saved.";
      }
      await refreshBackups();
    } catch (e) {
      backupError = `Failed to save backup settings: ${String(e)}`;
    }
  }

  function queueBackupSave() {
    if (!backupSettingsLoaded) return;
    if (backupSaveTimer) {
      clearTimeout(backupSaveTimer);
    }
    backupSaveTimer = setTimeout(() => {
      saveBackupSettings(false);
    }, 400);
  }

  async function chooseBackupFolder() {
    backupError = "";
    backupStatus = "";
    const selected = await invoke<string | null>("pick_backup_folder");
    if (!selected) return;
    backupSettings = { ...backupSettings, backup_path: selected };
    queueBackupSave();
  }

  async function createBackupNow() {
    backupError = "";
    backupStatus = "";
    busy = true;
    try {
      await invoke<string>("create_backup");
      backupStatus = "Backup created.";
      await refreshBackups();
    } catch (e) {
      backupError = `Failed to create backup: ${String(e)}`;
    } finally {
      busy = false;
    }
  }

  async function refreshBackups() {
    try {
      backups = await invoke<BackupInfo[]>("list_backups");
    } catch (e) {
      backupError = `Failed to list backups: ${String(e)}`;
    }
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

  function formatDate(ts: number | null) {
    if (!ts) return "—";
    return new Date(ts * 1000).toLocaleString();
  }

  function openBackupList() {
    showBackupList = true;
  }

  function closeBackupList() {
    showBackupList = false;
  }

  async function refreshMaintenance() {
    maintenanceError = "";
    try {
      dbStats = await invoke<DbStats>("db_stats");
      migrationStatus = await invoke<MigrationStatus>("get_migration_status");
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
      integrityStatus = await invoke<string>("db_integrity_check");
      maintenanceStatus = integrityStatus === "ok" ? "Integrity check passed." : "Integrity check reported issues.";
    } catch (e) {
      maintenanceError = `Integrity check failed: ${String(e)}`;
    } finally {
      busy = false;
    }
  }

  async function runVacuum() {
    maintenanceError = "";
    maintenanceStatus = "";
    busy = true;
    try {
      await invoke<string>("db_vacuum");
      maintenanceStatus = "Database vacuum complete.";
      await refreshMaintenance();
    } catch (e) {
      maintenanceError = `Vacuum failed: ${String(e)}`;
    } finally {
      busy = false;
    }
  }

  async function createNewDatabase() {
    error = "";
    status = "";
    const selected = await invoke<string | null>("pick_storage_folder");
    if (!selected) return;
    busy = true;
    try {
      const created = await invoke<string>("create_new_storage", { path: selected });
      dbPath = created;
      status = "New database created.";
      setTimeout(() => {
        window.location.reload();
      }, 300);
    } catch (e) {
      error = `Failed to create database: ${String(e)}`;
    } finally {
      busy = false;
    }
  }

  async function moveDatabase() {
    error = "";
    status = "";
    const selected = await invoke<string | null>("pick_storage_folder");
    if (!selected) return;
    busy = true;
    try {
      const moved = await invoke<string>("move_storage", { path: selected });
      dbPath = moved;
      status = "Database moved.";
      setTimeout(() => {
        window.location.reload();
      }, 300);
    } catch (e) {
      error = `Failed to move database: ${String(e)}`;
    } finally {
      busy = false;
    }
  }

  async function duplicateDatabase() {
    error = "";
    status = "";
    const selected = await invoke<string | null>("pick_storage_folder");
    if (!selected) return;
    busy = true;
    try {
      const copied = await invoke<string>("copy_storage", { path: selected });
      dbPath = copied;
      status = "Database duplicated.";
      setTimeout(() => {
        window.location.reload();
      }, 300);
    } catch (e) {
      error = `Failed to duplicate database: ${String(e)}`;
    } finally {
      busy = false;
    }
  }

  async function restoreFromBackup() {
    error = "";
    status = "";
    const selected = await invoke<string | null>("pick_backup_file");
    if (!selected) return;
    const confirmed = await askConfirm("Restore will overwrite the current database. This cannot be undone.");
    if (!confirmed) return;
    busy = true;
    try {
      const restored = await invoke<string>("restore_from_backup", { backup_path: selected });
      dbPath = restored;
      status = "Database restored from backup.";
      setTimeout(() => {
        window.location.reload();
      }, 300);
    } catch (e) {
      error = `Failed to restore database: ${String(e)}`;
    } finally {
      busy = false;
    }
  }

  async function switchDatabase(path: string) {
    busy = true;
    try {
      const updated = await invoke<string>("validate_and_set_storage_location", { path });
      dbPath = updated;
      status = "Storage location updated.";
      setTimeout(() => {
        window.location.reload();
      }, 300);
    } catch (e) {
      error = `Failed to switch database: ${String(e)}`;
    } finally {
      busy = false;
    }
  }
</script>

<Card.Root>
  <Card.Header>
    <Card.Title>Storage</Card.Title>
    <Card.Description>Current database path</Card.Description>
  </Card.Header>
  <Card.Content>
    <p class="font-mono text-sm mb-4">{dbPath || "—"}</p>

    <div class="flex flex-wrap gap-2">
      <Button variant="secondary" onclick={openDatabaseFile} disabled={busy}>
        Open database file…
      </Button>
      <Button variant="secondary" onclick={openDatabaseFolder} disabled={busy}>
        Open database folder…
      </Button>
      <Button onclick={createNewDatabase} disabled={busy}>
        Create new database…
      </Button>
      <Button variant="secondary" onclick={moveDatabase} disabled={busy}>
        Move database…
      </Button>
      <Button variant="secondary" onclick={duplicateDatabase} disabled={busy}>
        Save As / Duplicate…
      </Button>
      <Button variant="destructive" onclick={restoreFromBackup} disabled={busy}>
        Restore from backup…
      </Button>
    </div>

    {#if status}
      <p class="text-sm text-green-600 mt-4">{status}</p>
    {/if}
    {#if error}
      <p class="text-sm text-destructive mt-4">{error}</p>
    {/if}
  </Card.Content>
</Card.Root>

<Card.Root>
  <Card.Header>
    <Card.Title>Backups</Card.Title>
  </Card.Header>
  <Card.Content>
    <div class="grid gap-4 md:grid-cols-2 lg:grid-cols-3">
      <div class="flex items-center gap-2">
        <input type="checkbox" id="backup-enabled" bind:checked={backupSettings.enabled} onchange={queueBackupSave} class="h-4 w-4" />
        <Label for="backup-enabled">Enable scheduled backups</Label>
      </div>

      <div class="flex items-center gap-2">
        <input type="checkbox" id="backup-on-close" bind:checked={backupSettings.backup_on_close} onchange={queueBackupSave} class="h-4 w-4" />
        <Label for="backup-on-close">Backup on close</Label>
      </div>

      <div class="space-y-1">
        <Label for="backup-interval">Interval (minutes)</Label>
        <select id="backup-interval" class="flex h-9 w-full rounded-md border border-input bg-transparent px-3 py-1 text-sm" bind:value={backupSettings.interval_minutes} onchange={queueBackupSave}>
          <option value={5}>5</option>
          <option value={10}>10</option>
          <option value={15}>15</option>
          <option value={30}>30</option>
          <option value={60}>60</option>
          <option value={120}>120</option>
        </select>
      </div>

      <div class="space-y-1">
        <Label for="retention-count">Retention count</Label>
        <Input
          id="retention-count"
          type="number"
          min={0}
          bind:value={backupSettings.retention_count}
          oninput={queueBackupSave}
        />
      </div>

      <div class="space-y-2 md:col-span-2">
        <Label>Backup folder</Label>
        <p class="font-mono text-sm">{backupSettings.backup_path || "(Default: <db>/backups)"}</p>
        <Button variant="secondary" onclick={chooseBackupFolder} disabled={busy}>
          Choose backup folder…
        </Button>
      </div>
    </div>

    <div class="flex flex-wrap gap-2 mt-4">
      <Button onclick={() => saveBackupSettings(true)} disabled={busy}>
        Save backup settings
      </Button>
      <Button variant="secondary" onclick={createBackupNow} disabled={busy}>
        Create backup now
      </Button>
      <Button variant="ghost" onclick={refreshBackups} disabled={busy}>
        Refresh list
      </Button>
    </div>

    {#if backupStatus}
      <p class="text-sm text-green-600 mt-4">{backupStatus}</p>
    {/if}
    {#if backupError}
      <p class="text-sm text-destructive mt-4">{backupError}</p>
    {/if}

    <div class="mt-4 pt-4 border-t">
      <p class="text-sm text-muted-foreground">Last backup</p>
      {#if backups.length === 0}
        <p class="text-sm text-muted-foreground">No backups yet.</p>
      {:else}
        <div class="flex items-center gap-4 text-sm mt-2">
          <span class="font-mono">{backups[0].path}</span>
          <span>{formatBytes(backups[0].size_bytes)}</span>
          <span>{formatDate(backups[0].modified_unix)}</span>
        </div>
      {/if}
      <Button variant="ghost" class="mt-2" onclick={openBackupList} disabled={backups.length === 0}>
        View all backups
      </Button>
    </div>
  </Card.Content>
</Card.Root>

<Card.Root>
  <Card.Header>
    <Card.Title>Maintenance &amp; Health</Card.Title>
  </Card.Header>
  <Card.Content>
    {#if dbStats && !dbStats.writable}
      <Alert.Root variant="destructive" class="mb-4">
        <Alert.Title>Read-only</Alert.Title>
        <Alert.Description>Database is read-only. Write operations and migrations are disabled.</Alert.Description>
      </Alert.Root>
    {/if}

    <div class="grid gap-4 md:grid-cols-2 lg:grid-cols-4">
      <div>
        <p class="text-sm text-muted-foreground">Schema version</p>
        <p>
          {migrationStatus ? `${migrationStatus.current_version} / ${migrationStatus.latest_version}` : "—"}
        </p>
        {#if migrationStatus && migrationStatus.pending_versions.length > 0}
          <p class="text-sm text-yellow-600">
            Pending migrations: {migrationStatus.pending_versions.join(", ")}
          </p>
        {/if}
      </div>
      <div>
        <p class="text-sm text-muted-foreground">DB size</p>
        <p>{dbStats ? formatBytes(dbStats.size_bytes) : "—"}</p>
      </div>
      <div>
        <p class="text-sm text-muted-foreground">Last modified</p>
        <p>{dbStats ? formatDate(dbStats.modified_unix) : "—"}</p>
      </div>
      <div>
        <p class="text-sm text-muted-foreground">Journal mode</p>
        <p>{dbStats?.journal_mode || "—"}</p>
      </div>
    </div>

    <div class="flex flex-wrap gap-2 mt-4">
      <Button variant="secondary" onclick={refreshMaintenance} disabled={busy}>
        Refresh status
      </Button>
      <Button variant="secondary" onclick={runIntegrityCheck} disabled={busy}>
        Run integrity check
      </Button>
      <Button variant="secondary" onclick={runVacuum} disabled={busy}>
        Vacuum / Optimize
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

<!-- Backup List Dialog -->
<Dialog.Root bind:open={showBackupList}>
  <Dialog.Content class="max-w-3xl">
    <Dialog.Header>
      <Dialog.Title>All backups</Dialog.Title>
    </Dialog.Header>
    <Table.Root>
      <Table.Header>
        <Table.Row>
          <Table.Head>File</Table.Head>
          <Table.Head>Size</Table.Head>
          <Table.Head>Modified</Table.Head>
        </Table.Row>
      </Table.Header>
      <Table.Body>
        {#each backups as backup}
          <Table.Row>
            <Table.Cell class="font-mono">{backup.path}</Table.Cell>
            <Table.Cell>{formatBytes(backup.size_bytes)}</Table.Cell>
            <Table.Cell>{formatDate(backup.modified_unix)}</Table.Cell>
          </Table.Row>
        {/each}
      </Table.Body>
    </Table.Root>
    <Dialog.Footer>
      <Button variant="secondary" onclick={closeBackupList}>Close</Button>
    </Dialog.Footer>
  </Dialog.Content>
</Dialog.Root>

<ConfirmDialog
  bind:open={confirmOpen}
  title="Restore Database"
  message={confirmMessage}
  confirmLabel="Restore"
  destructive={true}
  onConfirm={onConfirmYes}
  onCancel={onConfirmNo}
/>
