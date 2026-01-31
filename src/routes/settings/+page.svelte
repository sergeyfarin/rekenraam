<script lang="ts">
  import { onMount } from "svelte";
  import { invoke } from "@tauri-apps/api/core";
  import * as Card from "$lib/components/ui/card";
  import { Button } from "$lib/components/ui/button";
  import { Input } from "$lib/components/ui/input";
  import { Label } from "$lib/components/ui/label";
  import * as Tabs from "$lib/components/ui/tabs";
  import * as Table from "$lib/components/ui/table";
  import * as Dialog from "$lib/components/ui/dialog";
  import * as Alert from "$lib/components/ui/alert";
  import { Badge } from "$lib/components/ui/badge";

  // Tab state
  let activeTab: "database" | "categories" | "payees" | "tags" | "commodities" | "institutions" = "database";

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

  // --- Category types and state ---
  type Category = {
    id: number;
    book_id: number;
    parent_id: number | null;
    name: string;
    kind: string;
    color: string | null;
    created_at: string;
    updated_at: string;
  };

  let categories: Category[] = [];
  let categoryError = "";
  let categoryStatus = "";
  let editingCategory: Category | null = null;
  let newCategory = { name: "", kind: "expense", color: "", parent_id: null as number | null };
  let showCategoryDialog = false;

  // --- Payee types and state ---
  type Payee = {
    id: number;
    book_id: number;
    name: string;
    kind: string;
    metadata: string | null;
    created_at: string;
    updated_at: string;
  };

  let payees: Payee[] = [];
  let payeeError = "";
  let payeeStatus = "";
  let editingPayee: Payee | null = null;
  let newPayee = { name: "", kind: "person", metadata: "" };
  let showPayeeDialog = false;

  // --- Tag types and state ---
  type Tag = {
    id: number;
    book_id: number;
    name: string;
    color: string | null;
    created_at: string;
    updated_at: string;
  };

  let tags: Tag[] = [];
  let tagError = "";
  let tagStatus = "";
  let editingTag: Tag | null = null;
  let newTag = { name: "", color: "" };
  let showTagDialog = false;

  // --- Commodity types and state ---
  type Commodity = {
    id: number;
    book_id: number;
    kind: string;
    symbol: string | null;
    name: string;
    scale: number;
    metadata: string | null;
    created_at: string;
    updated_at: string;
  };

  let commodities: Commodity[] = [];
  let commodityError = "";
  let commodityStatus = "";
  let editingCommodity: Commodity | null = null;
  let showCommodityDialog = false;
  let defaultCurrencyId: number | null = null;

  // --- Institution types and state ---
  type Institution = {
    id: number;
    book_id: number;
    name: string;
    kind: string | null;
    routing: string | null;
    website: string | null;
    metadata: string | null;
    created_at: string;
    updated_at: string;
  };

  let institutions: Institution[] = [];
  let institutionError = "";
  let institutionStatus = "";
  let editingInstitution: Institution | null = null;
  let newInstitution = { name: "", kind: "", routing: "", website: "", metadata: "" };
  let showInstitutionDialog = false;

  onMount(async () => {
    await loadDbPath();
    await loadBackupSettings();
    await refreshBackups();
    await refreshMaintenance();
    await loadCategories();
    await loadPayees();
    await loadTags();
    await loadCommodities();
    await loadInstitutions();
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

  // --- Category functions ---
  async function loadCategories() {
    try {
      categories = await invoke<Category[]>("list_categories", { bookId: 1 });
    } catch (e) {
      categoryError = `Failed to load categories: ${String(e)}`;
    }
  }

  function openNewCategory() {
    editingCategory = null;
    newCategory = { name: "", kind: "expense", color: "", parent_id: null };
    showCategoryDialog = true;
  }

  function openEditCategory(cat: Category) {
    editingCategory = cat;
    newCategory = { name: cat.name, kind: cat.kind, color: cat.color || "", parent_id: cat.parent_id };
    showCategoryDialog = true;
  }

  function closeCategoryDialog() {
    showCategoryDialog = false;
    editingCategory = null;
  }

  async function saveCategory() {
    categoryError = "";
    categoryStatus = "";
    busy = true;
    try {
      if (editingCategory) {
        await invoke("update_category", {
          category: {
            id: editingCategory.id,
            book_id: 1,
            parent_id: newCategory.parent_id || null,
            name: newCategory.name,
            kind: newCategory.kind,
            color: newCategory.color || null,
          },
        });
        categoryStatus = "Category updated.";
      } else {
        await invoke("create_category", {
          category: {
            book_id: 1,
            parent_id: newCategory.parent_id || null,
            name: newCategory.name,
            kind: newCategory.kind,
            color: newCategory.color || null,
          },
        });
        categoryStatus = "Category created.";
      }
      closeCategoryDialog();
      await loadCategories();
    } catch (e) {
      categoryError = `Failed to save category: ${String(e)}`;
    } finally {
      busy = false;
    }
  }

  async function deleteCategory(cat: Category) {
    if (!confirm(`Delete category "${cat.name}"?`)) return;
    categoryError = "";
    categoryStatus = "";
    busy = true;
    try {
      await invoke("delete_category", { categoryId: cat.id, bookId: 1 });
      categoryStatus = "Category deleted.";
      await loadCategories();
    } catch (e) {
      categoryError = `Failed to delete category: ${String(e)}`;
    } finally {
      busy = false;
    }
  }

  // --- Payee functions ---
  async function loadPayees() {
    try {
      payees = await invoke<Payee[]>("list_payees", { bookId: 1 });
    } catch (e) {
      payeeError = `Failed to load payees: ${String(e)}`;
    }
  }

  function openNewPayee() {
    editingPayee = null;
    newPayee = { name: "", kind: "person", metadata: "" };
    showPayeeDialog = true;
  }

  function openEditPayee(p: Payee) {
    editingPayee = p;
    newPayee = { name: p.name, kind: p.kind, metadata: p.metadata || "" };
    showPayeeDialog = true;
  }

  function closePayeeDialog() {
    showPayeeDialog = false;
    editingPayee = null;
  }

  async function savePayee() {
    payeeError = "";
    payeeStatus = "";
    busy = true;
    try {
      if (editingPayee) {
        await invoke("update_payee", {
          payee: {
            id: editingPayee.id,
            book_id: 1,
            name: newPayee.name,
            kind: newPayee.kind,
            metadata: newPayee.metadata || null,
          },
        });
        payeeStatus = "Payee updated.";
      } else {
        await invoke("create_payee", {
          payee: {
            book_id: 1,
            name: newPayee.name,
            kind: newPayee.kind,
            metadata: newPayee.metadata || null,
          },
        });
        payeeStatus = "Payee created.";
      }
      closePayeeDialog();
      await loadPayees();
    } catch (e) {
      payeeError = `Failed to save payee: ${String(e)}`;
    } finally {
      busy = false;
    }
  }

  async function deletePayee(p: Payee) {
    if (!confirm(`Delete payee "${p.name}"?`)) return;
    payeeError = "";
    payeeStatus = "";
    busy = true;
    try {
      await invoke("delete_payee", { payeeId: p.id, bookId: 1 });
      payeeStatus = "Payee deleted.";
      await loadPayees();
    } catch (e) {
      payeeError = `Failed to delete payee: ${String(e)}`;
    } finally {
      busy = false;
    }
  }

  // --- Tag functions ---
  async function loadTags() {
    try {
      tags = await invoke<Tag[]>("list_tags", { bookId: 1 });
    } catch (e) {
      tagError = `Failed to load tags: ${String(e)}`;
    }
  }

  function openNewTag() {
    editingTag = null;
    newTag = { name: "", color: "" };
    showTagDialog = true;
  }

  function openEditTag(t: Tag) {
    editingTag = t;
    newTag = { name: t.name, color: t.color || "" };
    showTagDialog = true;
  }

  function closeTagDialog() {
    showTagDialog = false;
    editingTag = null;
  }

  async function saveTag() {
    tagError = "";
    tagStatus = "";
    busy = true;
    try {
      if (editingTag) {
        await invoke("update_tag", {
          tag: {
            id: editingTag.id,
            book_id: 1,
            name: newTag.name,
            color: newTag.color || null,
          },
        });
        tagStatus = "Tag updated.";
      } else {
        await invoke("create_tag", {
          tag: {
            book_id: 1,
            name: newTag.name,
            color: newTag.color || null,
          },
        });
        tagStatus = "Tag created.";
      }
      closeTagDialog();
      await loadTags();
    } catch (e) {
      tagError = `Failed to save tag: ${String(e)}`;
    } finally {
      busy = false;
    }
  }

  async function deleteTag(t: Tag) {
    if (!confirm(`Delete tag "${t.name}"?`)) return;
    tagError = "";
    tagStatus = "";
    busy = true;
    try {
      await invoke("delete_tag", { tagId: t.id, bookId: 1 });
      tagStatus = "Tag deleted.";
      await loadTags();
    } catch (e) {
      tagError = `Failed to delete tag: ${String(e)}`;
    } finally {
      busy = false;
    }
  }

  // --- Commodity functions ---
  async function loadCommodities() {
    try {
      commodities = await invoke<Commodity[]>("list_commodities", { bookId: 1 });
    } catch (e) {
      commodityError = `Failed to load commodities: ${String(e)}`;
    }
  }

  function openEditCommodity(c: Commodity) {
    editingCommodity = c;
    showCommodityDialog = true;
  }

  function closeCommodityDialog() {
    showCommodityDialog = false;
    editingCommodity = null;
  }

  async function saveCommodity() {
    if (!editingCommodity) return;
    commodityError = "";
    commodityStatus = "";
    busy = true;
    try {
      await invoke("rename_commodity_symbol", {
        input: {
          id: editingCommodity.id,
          book_id: 1,
          symbol: editingCommodity.symbol,
          name: editingCommodity.name,
          metadata: editingCommodity.metadata,
        },
      });
      commodityStatus = "Commodity updated.";
      closeCommodityDialog();
      await loadCommodities();
    } catch (e) {
      commodityError = `Failed to save commodity: ${String(e)}`;
    } finally {
      busy = false;
    }
  }

  // --- Institution functions ---
  async function loadInstitutions() {
    try {
      institutions = await invoke<Institution[]>("list_institutions", { bookId: 1 });
    } catch (e) {
      institutionError = `Failed to load institutions: ${String(e)}`;
    }
  }

  function openNewInstitution() {
    editingInstitution = null;
    newInstitution = { name: "", kind: "", routing: "", website: "", metadata: "" };
    showInstitutionDialog = true;
  }

  function openEditInstitution(inst: Institution) {
    editingInstitution = inst;
    newInstitution = {
      name: inst.name,
      kind: inst.kind || "",
      routing: inst.routing || "",
      website: inst.website || "",
      metadata: inst.metadata || "",
    };
    showInstitutionDialog = true;
  }

  function closeInstitutionDialog() {
    showInstitutionDialog = false;
    editingInstitution = null;
  }

  async function saveInstitution() {
    institutionError = "";
    institutionStatus = "";
    busy = true;
    try {
      if (editingInstitution) {
        await invoke("update_institution", {
          institution: {
            id: editingInstitution.id,
            book_id: 1,
            name: newInstitution.name,
            kind: newInstitution.kind || null,
            routing: newInstitution.routing || null,
            website: newInstitution.website || null,
            metadata: newInstitution.metadata || null,
          },
        });
        institutionStatus = "Institution updated.";
      } else {
        await invoke("create_institution", {
          institution: {
            book_id: 1,
            name: newInstitution.name,
            kind: newInstitution.kind || null,
            routing: newInstitution.routing || null,
            website: newInstitution.website || null,
            metadata: newInstitution.metadata || null,
          },
        });
        institutionStatus = "Institution created.";
      }
      closeInstitutionDialog();
      await loadInstitutions();
    } catch (e) {
      institutionError = `Failed to save institution: ${String(e)}`;
    } finally {
      busy = false;
    }
  }

  async function deleteInstitution(inst: Institution) {
    if (!confirm(`Delete institution "${inst.name}"?`)) return;
    institutionError = "";
    institutionStatus = "";
    busy = true;
    try {
      await invoke("delete_institution", { institutionId: inst.id, bookId: 1 });
      institutionStatus = "Institution deleted.";
      await loadInstitutions();
    } catch (e) {
      institutionError = `Failed to delete institution: ${String(e)}`;
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
        <p class="page-subtitle">Configure preferences and manage data.</p>
      </div>
    </div>

    <!-- Tabs -->
    <div class="page-row">
      <div class="page-col">
        <div class="tabs">
          <button
            class="tab"
            class:active={activeTab === "database"}
            on:click={() => (activeTab = "database")}
          >
            Database
          </button>
          <button
            class="tab"
            class:active={activeTab === "categories"}
            on:click={() => (activeTab = "categories")}
          >
            Categories ({categories.length})
          </button>
          <button
            class="tab"
            class:active={activeTab === "payees"}
            on:click={() => (activeTab = "payees")}
          >
            Payees ({payees.length})
          </button>
          <button
            class="tab"
            class:active={activeTab === "tags"}
            on:click={() => (activeTab = "tags")}
          >
            Tags ({tags.length})
          </button>
          <button
            class="tab"
            class:active={activeTab === "commodities"}
            on:click={() => (activeTab = "commodities")}
          >
            Currencies ({commodities.length})
          </button>
          <button
            class="tab"
            class:active={activeTab === "institutions"}
            on:click={() => (activeTab = "institutions")}
          >
            Institutions ({institutions.length})
          </button>
        </div>
      </div>
    </div>

    <!-- Database Tab -->
    {#if activeTab === "database"}

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
              <input type="checkbox" bind:checked={backupSettings.enabled} on:change={queueBackupSave} />
              <span>Enable scheduled backups</span>
            </label>

            <label class="checkbox">
              <input type="checkbox" bind:checked={backupSettings.backup_on_close} on:change={queueBackupSave} />
              <span>Backup on close</span>
            </label>

            <label class="label">
              Interval (minutes)
              <select class="select" bind:value={backupSettings.interval_minutes} on:change={queueBackupSave}>
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
                on:input={queueBackupSave}
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
            <button class="btn btn-primary" type="button" on:click={() => saveBackupSettings(true)} disabled={busy}>
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

          <div class="backup-summary">
            <p class="text-sm text-muted">Last backup</p>
            {#if backups.length === 0}
              <p class="text-sm text-muted">No backups yet.</p>
            {:else}
              <div class="backup-list__row">
                <span class="mono">{backups[0].path}</span>
                <span>{formatBytes(backups[0].size_bytes)}</span>
                <span>{formatDate(backups[0].modified_unix)}</span>
              </div>
            {/if}
            <button class="btn btn-ghost" type="button" on:click={openBackupList} disabled={backups.length === 0}>
              View all backups
            </button>
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
    {/if}

    <!-- Categories Tab -->
    {#if activeTab === "categories"}
    <div class="page-row">
      <div class="page-col">
        <div class="card">
          <div class="card-header">
            <h2 class="section-title">Categories</h2>
            <button class="btn btn-primary" type="button" on:click={openNewCategory} disabled={busy}>
              Add Category
            </button>
          </div>

          {#if categoryStatus}
            <p class="text-sm text-success">{categoryStatus}</p>
          {/if}
          {#if categoryError}
            <p class="text-sm text-error">{categoryError}</p>
          {/if}

          <div class="data-table">
            <div class="data-table__header">
              <span>Name</span>
              <span>Kind</span>
              <span>Color</span>
              <span>Actions</span>
            </div>
            {#each categories as cat}
              <div class="data-table__row">
                <span>{cat.name}</span>
                <span class="badge badge-{cat.kind}">{cat.kind}</span>
                <span>
                  {#if cat.color}
                    <span class="color-swatch" style="background-color: {cat.color}"></span>
                  {:else}
                    —
                  {/if}
                </span>
                <span class="row-actions">
                  <button class="btn btn-ghost btn-sm" on:click={() => openEditCategory(cat)}>Edit</button>
                  <button class="btn btn-ghost btn-sm text-error" on:click={() => deleteCategory(cat)}>Delete</button>
                </span>
              </div>
            {:else}
              <div class="data-table__row">
                <span class="text-muted">No categories found.</span>
              </div>
            {/each}
          </div>
        </div>
      </div>
    </div>
    {/if}

    <!-- Payees Tab -->
    {#if activeTab === "payees"}
    <div class="page-row">
      <div class="page-col">
        <div class="card">
          <div class="card-header">
            <h2 class="section-title">Payees</h2>
            <button class="btn btn-primary" type="button" on:click={openNewPayee} disabled={busy}>
              Add Payee
            </button>
          </div>

          {#if payeeStatus}
            <p class="text-sm text-success">{payeeStatus}</p>
          {/if}
          {#if payeeError}
            <p class="text-sm text-error">{payeeError}</p>
          {/if}

          <div class="data-table">
            <div class="data-table__header">
              <span>Name</span>
              <span>Kind</span>
              <span>Metadata</span>
              <span>Actions</span>
            </div>
            {#each payees as p}
              <div class="data-table__row">
                <span>{p.name}</span>
                <span class="badge">{p.kind}</span>
                <span class="text-muted">{p.metadata || "—"}</span>
                <span class="row-actions">
                  <button class="btn btn-ghost btn-sm" on:click={() => openEditPayee(p)}>Edit</button>
                  <button class="btn btn-ghost btn-sm text-error" on:click={() => deletePayee(p)}>Delete</button>
                </span>
              </div>
            {:else}
              <div class="data-table__row">
                <span class="text-muted">No payees found.</span>
              </div>
            {/each}
          </div>
        </div>
      </div>
    </div>
    {/if}

    <!-- Tags Tab -->
    {#if activeTab === "tags"}
    <div class="page-row">
      <div class="page-col">
        <div class="card">
          <div class="card-header">
            <h2 class="section-title">Tags</h2>
            <button class="btn btn-primary" type="button" on:click={openNewTag} disabled={busy}>
              Add Tag
            </button>
          </div>

          {#if tagStatus}
            <p class="text-sm text-success">{tagStatus}</p>
          {/if}
          {#if tagError}
            <p class="text-sm text-error">{tagError}</p>
          {/if}

          <div class="data-table">
            <div class="data-table__header">
              <span>Name</span>
              <span>Color</span>
              <span>Actions</span>
            </div>
            {#each tags as t}
              <div class="data-table__row">
                <span>{t.name}</span>
                <span>
                  {#if t.color}
                    <span class="color-swatch" style="background-color: {t.color}"></span>
                  {:else}
                    —
                  {/if}
                </span>
                <span class="row-actions">
                  <button class="btn btn-ghost btn-sm" on:click={() => openEditTag(t)}>Edit</button>
                  <button class="btn btn-ghost btn-sm text-error" on:click={() => deleteTag(t)}>Delete</button>
                </span>
              </div>
            {:else}
              <div class="data-table__row">
                <span class="text-muted">No tags found.</span>
              </div>
            {/each}
          </div>
        </div>
      </div>
    </div>
    {/if}

    <!-- Commodities Tab -->
    {#if activeTab === "commodities"}
    <div class="page-row">
      <div class="page-col">
        <div class="card">
          <div class="card-header">
            <h2 class="section-title">Currencies &amp; Commodities</h2>
          </div>

          {#if commodityStatus}
            <p class="text-sm text-success">{commodityStatus}</p>
          {/if}
          {#if commodityError}
            <p class="text-sm text-error">{commodityError}</p>
          {/if}

          <div class="data-table">
            <div class="data-table__header">
              <span>Symbol</span>
              <span>Name</span>
              <span>Kind</span>
              <span>Scale</span>
              <span>Actions</span>
            </div>
            {#each commodities as c}
              <div class="data-table__row">
                <span class="mono">{c.symbol || "—"}</span>
                <span>{c.name}</span>
                <span class="badge badge-{c.kind}">{c.kind}</span>
                <span>{c.scale}</span>
                <span class="row-actions">
                  <button class="btn btn-ghost btn-sm" on:click={() => openEditCommodity(c)}>Edit</button>
                </span>
              </div>
            {:else}
              <div class="data-table__row">
                <span class="text-muted">No commodities found.</span>
              </div>
            {/each}
          </div>
        </div>
      </div>
    </div>
    {/if}

    <!-- Institutions Tab -->
    {#if activeTab === "institutions"}
    <div class="page-row">
      <div class="page-col">
        <div class="card">
          <div class="card-header">
            <h2 class="section-title">Institutions</h2>
            <button class="btn btn-primary" type="button" on:click={openNewInstitution} disabled={busy}>
              Add Institution
            </button>
          </div>

          {#if institutionStatus}
            <p class="text-sm text-success">{institutionStatus}</p>
          {/if}
          {#if institutionError}
            <p class="text-sm text-error">{institutionError}</p>
          {/if}

          <div class="data-table">
            <div class="data-table__header">
              <span>Name</span>
              <span>Kind</span>
              <span>Routing</span>
              <span>Website</span>
              <span>Actions</span>
            </div>
            {#each institutions as inst}
              <div class="data-table__row">
                <span>{inst.name}</span>
                <span class="badge">{inst.kind || "—"}</span>
                <span class="mono">{inst.routing || "—"}</span>
                <span>
                  {#if inst.website}
                    <a href={inst.website} target="_blank" rel="noopener">{inst.website}</a>
                  {:else}
                    —
                  {/if}
                </span>
                <span class="row-actions">
                  <button class="btn btn-ghost btn-sm" on:click={() => openEditInstitution(inst)}>Edit</button>
                  <button class="btn btn-ghost btn-sm text-error" on:click={() => deleteInstitution(inst)}>Delete</button>
                </span>
              </div>
            {:else}
              <div class="data-table__row">
                <span class="text-muted">No institutions found.</span>
              </div>
            {/each}
          </div>
        </div>
      </div>
    </div>
    {/if}

  </div>
</main>

{#if showBackupList}
  <button class="dialog-backdrop" type="button" aria-label="Close backup list" on:click={closeBackupList}></button>
  <div class="dialog" role="dialog" aria-modal="true" aria-label="Backups list">
    <div class="dialog-header">
      <h2 class="section-title">All backups</h2>
    </div>
    <div class="dialog-body">
      <div class="backup-list">
        <div class="backup-list__header">
          <span>File</span>
          <span>Size</span>
          <span>Modified</span>
        </div>
        {#each backups as backup}
          <div class="backup-list__row">
            <span class="mono">{backup.path}</span>
            <span>{formatBytes(backup.size_bytes)}</span>
            <span>{formatDate(backup.modified_unix)}</span>
          </div>
        {/each}
      </div>
    </div>
    <div class="dialog-footer">
      <button class="btn btn-secondary" type="button" on:click={closeBackupList}>Close</button>
    </div>
  </div>
{/if}

<!-- Category Dialog -->
{#if showCategoryDialog}
  <button class="dialog-backdrop" type="button" aria-label="Close" on:click={closeCategoryDialog}></button>
  <div class="dialog" role="dialog" aria-modal="true" aria-label="Category editor">
    <div class="dialog-header">
      <h2 class="section-title">{editingCategory ? "Edit Category" : "New Category"}</h2>
    </div>
    <div class="dialog-body">
      <label class="label">
        Name
        <input class="input" type="text" bind:value={newCategory.name} placeholder="Category name" />
      </label>
      <label class="label">
        Kind
        <select class="select" bind:value={newCategory.kind}>
          <option value="expense">Expense</option>
          <option value="income">Income</option>
          <option value="transfer">Transfer</option>
          <option value="other">Other</option>
        </select>
      </label>
      <label class="label">
        Parent Category
        <select class="select" bind:value={newCategory.parent_id}>
          <option value={null}>None (Top-level)</option>
          {#each categories.filter((c) => c.id !== editingCategory?.id) as cat}
            <option value={cat.id}>{cat.name}</option>
          {/each}
        </select>
      </label>
      <label class="label">
        Color
        <input class="input" type="color" bind:value={newCategory.color} />
      </label>
    </div>
    <div class="dialog-footer">
      <button class="btn btn-secondary" type="button" on:click={closeCategoryDialog}>Cancel</button>
      <button class="btn btn-primary" type="button" on:click={saveCategory} disabled={busy || !newCategory.name}>
        {editingCategory ? "Update" : "Create"}
      </button>
    </div>
  </div>
{/if}

<!-- Payee Dialog -->
{#if showPayeeDialog}
  <button class="dialog-backdrop" type="button" aria-label="Close" on:click={closePayeeDialog}></button>
  <div class="dialog" role="dialog" aria-modal="true" aria-label="Payee editor">
    <div class="dialog-header">
      <h2 class="section-title">{editingPayee ? "Edit Payee" : "New Payee"}</h2>
    </div>
    <div class="dialog-body">
      <label class="label">
        Name
        <input class="input" type="text" bind:value={newPayee.name} placeholder="Payee name" />
      </label>
      <label class="label">
        Kind
        <select class="select" bind:value={newPayee.kind}>
          <option value="person">Person</option>
          <option value="business">Business</option>
          <option value="government">Government</option>
          <option value="other">Other</option>
        </select>
      </label>
      <label class="label">
        Metadata (optional)
        <input class="input" type="text" bind:value={newPayee.metadata} placeholder="Notes or additional info" />
      </label>
    </div>
    <div class="dialog-footer">
      <button class="btn btn-secondary" type="button" on:click={closePayeeDialog}>Cancel</button>
      <button class="btn btn-primary" type="button" on:click={savePayee} disabled={busy || !newPayee.name}>
        {editingPayee ? "Update" : "Create"}
      </button>
    </div>
  </div>
{/if}

<!-- Tag Dialog -->
{#if showTagDialog}
  <button class="dialog-backdrop" type="button" aria-label="Close" on:click={closeTagDialog}></button>
  <div class="dialog" role="dialog" aria-modal="true" aria-label="Tag editor">
    <div class="dialog-header">
      <h2 class="section-title">{editingTag ? "Edit Tag" : "New Tag"}</h2>
    </div>
    <div class="dialog-body">
      <label class="label">
        Name
        <input class="input" type="text" bind:value={newTag.name} placeholder="Tag name" />
      </label>
      <label class="label">
        Color
        <input class="input" type="color" bind:value={newTag.color} />
      </label>
    </div>
    <div class="dialog-footer">
      <button class="btn btn-secondary" type="button" on:click={closeTagDialog}>Cancel</button>
      <button class="btn btn-primary" type="button" on:click={saveTag} disabled={busy || !newTag.name}>
        {editingTag ? "Update" : "Create"}
      </button>
    </div>
  </div>
{/if}

<!-- Commodity Dialog -->
{#if showCommodityDialog && editingCommodity}
  <button class="dialog-backdrop" type="button" aria-label="Close" on:click={closeCommodityDialog}></button>
  <div class="dialog" role="dialog" aria-modal="true" aria-label="Commodity editor">
    <div class="dialog-header">
      <h2 class="section-title">Edit Commodity</h2>
    </div>
    <div class="dialog-body">
      <label class="label">
        Symbol
        <input class="input" type="text" bind:value={editingCommodity.symbol} placeholder="e.g. USD, EUR, BTC" />
      </label>
      <label class="label">
        Name
        <input class="input" type="text" bind:value={editingCommodity.name} placeholder="Full name" />
      </label>
      <label class="label">
        Kind
        <input class="input" type="text" value={editingCommodity.kind} disabled />
        <span class="text-sm text-muted">Kind cannot be changed</span>
      </label>
      <label class="label">
        Scale (decimal places)
        <input class="input" type="number" value={editingCommodity.scale} disabled />
        <span class="text-sm text-muted">Scale cannot be changed</span>
      </label>
    </div>
    <div class="dialog-footer">
      <button class="btn btn-secondary" type="button" on:click={closeCommodityDialog}>Cancel</button>
      <button class="btn btn-primary" type="button" on:click={saveCommodity} disabled={busy || !editingCommodity.name}>
        Update
      </button>
    </div>
  </div>
{/if}

<!-- Institution Dialog -->
{#if showInstitutionDialog}
  <button class="dialog-backdrop" type="button" aria-label="Close" on:click={closeInstitutionDialog}></button>
  <div class="dialog" role="dialog" aria-modal="true" aria-label="Institution editor">
    <div class="dialog-header">
      <h2 class="section-title">{editingInstitution ? "Edit Institution" : "New Institution"}</h2>
    </div>
    <div class="dialog-body">
      <label class="label">
        Name
        <input class="input" type="text" bind:value={newInstitution.name} placeholder="Institution name" />
      </label>
      <label class="label">
        Kind
        <select class="select" bind:value={newInstitution.kind}>
          <option value="">None</option>
          <option value="bank">Bank</option>
          <option value="credit_union">Credit Union</option>
          <option value="brokerage">Brokerage</option>
          <option value="insurance">Insurance</option>
          <option value="employer">Employer</option>
          <option value="other">Other</option>
        </select>
      </label>
      <label class="label">
        Routing Number (optional)
        <input class="input" type="text" bind:value={newInstitution.routing} placeholder="e.g. 123456789" />
      </label>
      <label class="label">
        Website (optional)
        <input class="input" type="url" bind:value={newInstitution.website} placeholder="https://..." />
      </label>
      <label class="label">
        Metadata (optional)
        <input class="input" type="text" bind:value={newInstitution.metadata} placeholder="Additional notes" />
      </label>
    </div>
    <div class="dialog-footer">
      <button class="btn btn-secondary" type="button" on:click={closeInstitutionDialog}>Cancel</button>
      <button class="btn btn-primary" type="button" on:click={saveInstitution} disabled={busy || !newInstitution.name}>
        {editingInstitution ? "Update" : "Create"}
      </button>
    </div>
  </div>
{/if}

<style>
  .tabs {
    display: flex;
    gap: 0.5rem;
    border-bottom: 1px solid var(--border);
    margin-bottom: 1rem;
    overflow-x: auto;
  }

  .tab {
    padding: 0.75rem 1rem;
    background: none;
    border: none;
    border-bottom: 2px solid transparent;
    cursor: pointer;
    font-size: 0.875rem;
    color: var(--text-muted);
    white-space: nowrap;
  }

  .tab:hover {
    color: var(--text);
    background: var(--bg-hover);
  }

  .tab.active {
    color: var(--primary);
    border-bottom-color: var(--primary);
  }

  .card-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    margin-bottom: 1rem;
  }

  .data-table {
    margin-top: 1rem;
    display: flex;
    flex-direction: column;
    gap: 0.25rem;
  }

  .data-table__header,
  .data-table__row {
    display: grid;
    grid-template-columns: 1fr 120px 120px 150px;
    gap: 1rem;
    align-items: center;
    padding: 0.5rem;
  }

  .data-table__header {
    font-weight: 600;
    border-bottom: 1px solid var(--border);
    background: var(--bg-muted);
  }

  .data-table__row {
    border-bottom: 1px solid var(--border-light, #eee);
  }

  .data-table__row:hover {
    background: var(--bg-hover);
  }

  .row-actions {
    display: flex;
    gap: 0.5rem;
  }

  .badge {
    display: inline-block;
    padding: 0.125rem 0.5rem;
    border-radius: 0.25rem;
    font-size: 0.75rem;
    background: var(--bg-muted);
  }

  .badge-expense {
    background: #fee2e2;
    color: #dc2626;
  }

  .badge-income {
    background: #dcfce7;
    color: #16a34a;
  }

  .badge-transfer {
    background: #dbeafe;
    color: #2563eb;
  }

  .badge-currency {
    background: #fef3c7;
    color: #d97706;
  }

  .badge-security {
    background: #f3e8ff;
    color: #9333ea;
  }

  .color-swatch {
    display: inline-block;
    width: 1.5rem;
    height: 1.5rem;
    border-radius: 0.25rem;
    border: 1px solid var(--border);
    vertical-align: middle;
  }

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

  .backup-summary {
    margin-top: 1rem;
    padding-top: 1rem;
    border-top: 1px solid var(--border);
  }
</style>
