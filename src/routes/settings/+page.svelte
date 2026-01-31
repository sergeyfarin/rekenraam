<script lang="ts">
  import { onMount } from "svelte";
  import { invoke } from "@tauri-apps/api/core";

  let dbPath = "";
  let error = "";
  let status = "";
  let busy = false;

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

  let backupSettings: BackupSettings = {
    enabled: false,
    interval_minutes: 60,
    retention_count: 10,
    backup_path: null,
    backup_on_close: true,
  };
  let backups: BackupInfo[] = [];
  let backupStatus = "";
  let backupError = "";

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

  let dbStats: DbStats | null = null;
  let migrationStatus: MigrationStatus | null = null;
  let integrityStatus = "";
  let maintenanceStatus = "";
  let maintenanceError = "";

  onMount(async () => {
    await loadDbPath();
    await loadBackupSettings();
    await refreshBackups();
    await refreshMaintenance();
  });

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
    } catch (e) {
      backupError = `Failed to load backup settings: ${String(e)}`;
    }
  }

  async function saveBackupSettings() {
    backupError = "";
    backupStatus = "";
    try {
      const normalized = {
        ...backupSettings,
        interval_minutes: Number(backupSettings.interval_minutes),
        retention_count: Number(backupSettings.retention_count),
      };
      backupSettings = await invoke<BackupSettings>("set_backup_settings", { settings: normalized });
      backupStatus = "Backup settings saved.";
      await refreshBackups();
    } catch (e) {
      backupError = `Failed to save backup settings: ${String(e)}`;
    }
  }

  async function chooseBackupFolder() {
    backupError = "";
    backupStatus = "";
    const selected = await invoke<string | null>("pick_backup_folder");
    if (!selected) return;
    backupSettings = { ...backupSettings, backup_path: selected };
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
    const confirmed = window.confirm("Restore will overwrite the current database. Continue?");
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

<main class="page">
  <div class="page-grid container">
    <div class="page-row">
      <div class="page-col">
        <h1 class="page-title">Settings</h1>
        <p class="page-subtitle">Configure preferences and storage options.</p>
      </div>
    </div>

    <div class="page-row">
      <div class="page-col">
        <div class="card">
          <h2 class="section-title">Storage</h2>
          <p class="text-sm text-muted">Current database path</p>
          <p class="mono">{dbPath || "—"}</p>

          <div class="actions">
            <button class="btn btn-secondary" type="button" on:click={openDatabaseFile} disabled={busy}>
              Open database file…
            </button>
            <button class="btn btn-secondary" type="button" on:click={openDatabaseFolder} disabled={busy}>
              Open database folder…
            </button>
            <button class="btn btn-primary" type="button" on:click={createNewDatabase} disabled={busy}>
              Create new database…
            </button>
            <button class="btn btn-secondary" type="button" on:click={moveDatabase} disabled={busy}>
              Move database…
            </button>
            <button class="btn btn-secondary" type="button" on:click={duplicateDatabase} disabled={busy}>
              Save As / Duplicate…
            </button>
            <button class="btn btn-danger" type="button" on:click={restoreFromBackup} disabled={busy}>
              Restore from backup…
            </button>
          </div>

          {#if status}
            <p class="text-sm text-success">{status}</p>
          {/if}
          {#if error}
            <p class="text-sm text-error">{error}</p>
          {/if}
        </div>
      </div>
    </div>

    <div class="page-row">
      <div class="page-col">
        <div class="card">
          <h2 class="section-title">Backups</h2>

          <div class="backup-grid">
            <label class="checkbox">
              <input type="checkbox" bind:checked={backupSettings.enabled} />
              <span>Enable scheduled backups</span>
            </label>

            <label class="checkbox">
              <input type="checkbox" bind:checked={backupSettings.backup_on_close} />
              <span>Backup on close</span>
            </label>

            <label class="label">
              Interval (minutes)
              <select class="select" bind:value={backupSettings.interval_minutes}>
                <option value={5}>5</option>
                <option value={10}>10</option>
                <option value={15}>15</option>
                <option value={30}>30</option>
                <option value={60}>60</option>
                <option value={120}>120</option>
              </select>
            </label>

            <label class="label">
              Retention count
              <input
                class="input"
                type="number"
                min="0"
                bind:value={backupSettings.retention_count}
              />
            </label>

            <div class="backup-path">
              <p class="text-sm text-muted">Backup folder</p>
              <p class="mono">{backupSettings.backup_path || "(Default: <db>/backups)"}</p>
              <button class="btn btn-secondary" type="button" on:click={chooseBackupFolder} disabled={busy}>
                Choose backup folder…
              </button>
            </div>
          </div>

          <div class="actions">
            <button class="btn btn-primary" type="button" on:click={saveBackupSettings} disabled={busy}>
              Save backup settings
            </button>
            <button class="btn btn-secondary" type="button" on:click={createBackupNow} disabled={busy}>
              Create backup now
            </button>
            <button class="btn btn-ghost" type="button" on:click={refreshBackups} disabled={busy}>
              Refresh list
            </button>
          </div>

          {#if backupStatus}
            <p class="text-sm text-success">{backupStatus}</p>
          {/if}
          {#if backupError}
            <p class="text-sm text-error">{backupError}</p>
          {/if}

          <div class="backup-list">
            <div class="backup-list__header">
              <span>File</span>
              <span>Size</span>
              <span>Modified</span>
            </div>
            {#if backups.length === 0}
              <p class="text-sm text-muted">No backups yet.</p>
            {:else}
              {#each backups as backup}
                <div class="backup-list__row">
                  <span class="mono">{backup.path}</span>
                  <span>{formatBytes(backup.size_bytes)}</span>
                  <span>{formatDate(backup.modified_unix)}</span>
                </div>
              {/each}
            {/if}
          </div>
        </div>
      </div>
    </div>

    <div class="page-row">
      <div class="page-col">
        <div class="card">
          <h2 class="section-title">Maintenance &amp; Health</h2>

          {#if dbStats && !dbStats.writable}
            <p class="text-sm text-warning">
              Database is read-only. Write operations and migrations are disabled.
            </p>
          {/if}

          <div class="health-grid">
            <div>
              <p class="text-sm text-muted">Schema version</p>
              <p>
                {migrationStatus ? `${migrationStatus.current_version} / ${migrationStatus.latest_version}` : "—"}
              </p>
              {#if migrationStatus && migrationStatus.pending_versions.length > 0}
                <p class="text-sm text-warning">
                  Pending migrations: {migrationStatus.pending_versions.join(", ")}
                </p>
              {/if}
            </div>
            <div>
              <p class="text-sm text-muted">DB size</p>
              <p>{dbStats ? formatBytes(dbStats.size_bytes) : "—"}</p>
            </div>
            <div>
              <p class="text-sm text-muted">Last modified</p>
              <p>{dbStats ? formatDate(dbStats.modified_unix) : "—"}</p>
            </div>
            <div>
              <p class="text-sm text-muted">Journal mode</p>
              <p>{dbStats?.journal_mode || "—"}</p>
            </div>
          </div>

          <div class="actions">
            <button class="btn btn-secondary" type="button" on:click={refreshMaintenance} disabled={busy}>
              Refresh status
            </button>
            <button class="btn btn-secondary" type="button" on:click={runIntegrityCheck} disabled={busy}>
              Run integrity check
            </button>
            <button class="btn btn-secondary" type="button" on:click={runVacuum} disabled={busy}>
              Vacuum / Optimize
            </button>
          </div>

          {#if integrityStatus}
            <p class="text-sm text-muted">Integrity status: {integrityStatus}</p>
          {/if}
          {#if maintenanceStatus}
            <p class="text-sm text-success">{maintenanceStatus}</p>
          {/if}
          {#if maintenanceError}
            <p class="text-sm text-error">{maintenanceError}</p>
          {/if}
        </div>
      </div>
    </div>
  </div>
</main>

<style>
  .actions {
    margin-top: 1rem;
    display: flex;
    flex-wrap: wrap;
    gap: 0.75rem;
  }

  .mono {
    font-family: "SFMono-Regular", Consolas, "Liberation Mono", Menlo, monospace;
    font-size: 0.875rem;
  }

  .health-grid {
    margin-top: 1rem;
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
    gap: 1rem;
  }

  .backup-grid {
    margin-top: 1rem;
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
    gap: 1rem;
  }

  .backup-path {
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
  }

  .backup-list {
    margin-top: 1.5rem;
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
  }

  .backup-list__header,
  .backup-list__row {
    display: grid;
    grid-template-columns: 1fr 120px 180px;
    gap: 1rem;
    align-items: center;
  }

  .backup-list__header {
    font-weight: 600;
  }
</style>
