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

  // --- Currency management types and state ---
  type Currency = {
    id: number;
    book_id: number;
    symbol: string | null;
    display_symbol: string | null;
    name: string;
    scale: number;
    is_active: boolean;
    is_default: boolean;
    created_at: string;
    updated_at: string;
  };

  type FxRateDaily = {
    id: number;
    book_id: number;
    from_currency_id: number;
    from_currency_symbol: string | null;
    to_currency_id: number;
    to_currency_symbol: string | null;
    rate_date: string;
    rate: number;
    source: string | null;
    created_at: string;
  };

  type FxRateOfficial = {
    id: number;
    book_id: number;
    from_currency_id: number;
    from_currency_symbol: string | null;
    to_currency_id: number;
    to_currency_symbol: string | null;
    period_type: string;
    period_year: number;
    period_month: number | null;
    rate: number;
    source_name: string;
    source_url: string | null;
    source_date: string | null;
    notes: string | null;
    created_at: string;
    updated_at: string;
  };

  type FxRateSource = {
    id: number;
    book_id: number;
    name: string;
    country_code: string | null;
    website_url: string | null;
    notes: string | null;
    created_at: string;
    updated_at: string;
  };

  let currencies: Currency[] = [];
  let currencyError = "";
  let currencyStatus = "";
  let showCurrencyDialog = false;
  let newCurrency = { symbol: "", display_symbol: "", name: "", scale: 2 };

  let fxRatesDaily: FxRateDaily[] = [];
  let fxRatesOfficial: FxRateOfficial[] = [];
  let fxRateSources: FxRateSource[] = [];
  let fxRateError = "";
  let fxRateStatus = "";
  let showFxDailyDialog = false;
  let showFxOfficialDialog = false;
  let newFxDaily = { from_currency_id: 0, to_currency_id: 0, rate_date: "", rate: 0, source: "" };
  let newFxOfficial = { from_currency_id: 0, to_currency_id: 0, period_type: "yearly", period_year: new Date().getFullYear(), period_month: null as number | null, rate: 0, source_name: "", source_url: "", notes: "" };

  // Commodities sub-tabs
  let commoditiesSubTab: "currencies" | "commodities" | "fx-daily" | "fx-official" = "currencies";

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
    await loadCurrencies();
    await loadFxRateSources();
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

  // --- Currency management functions ---
  async function loadCurrencies() {
    try {
      currencies = await invoke<Currency[]>("list_currencies", { bookId: 1 });
    } catch (e) {
      currencyError = `Failed to load currencies: ${String(e)}`;
    }
  }

  async function toggleCurrencyActive(currency: Currency) {
    currencyError = "";
    currencyStatus = "";
    try {
      await invoke("toggle_currency_active", { currencyId: currency.id });
      currencyStatus = `Currency ${currency.symbol} ${currency.is_active ? "deactivated" : "activated"}.`;
      await loadCurrencies();
    } catch (e) {
      currencyError = `Failed to toggle currency: ${String(e)}`;
    }
  }

  async function setDefaultCurrency(currency: Currency) {
    currencyError = "";
    currencyStatus = "";
    try {
      await invoke("set_default_currency", { bookId: 1, currencyId: currency.id });
      currencyStatus = `${currency.symbol} set as default currency.`;
      await loadCurrencies();
    } catch (e) {
      currencyError = `Failed to set default currency: ${String(e)}`;
    }
  }

  function openNewCurrency() {
    newCurrency = { symbol: "", display_symbol: "", name: "", scale: 2 };
    showCurrencyDialog = true;
  }

  function closeCurrencyDialog() {
    showCurrencyDialog = false;
  }

  async function saveCurrency() {
    currencyError = "";
    currencyStatus = "";
    busy = true;
    try {
      await invoke("create_currency", {
        input: {
          book_id: 1,
          symbol: newCurrency.symbol,
          display_symbol: newCurrency.display_symbol || null,
          name: newCurrency.name,
          scale: newCurrency.scale,
          is_active: true,
        },
      });
      currencyStatus = "Currency created.";
      closeCurrencyDialog();
      await loadCurrencies();
    } catch (e) {
      currencyError = `Failed to create currency: ${String(e)}`;
    } finally {
      busy = false;
    }
  }

  // --- FX Rate functions ---
  async function loadFxRatesDaily() {
    try {
      fxRatesDaily = await invoke<FxRateDaily[]>("list_fx_rates_daily", { bookId: 1, limit: 100 });
    } catch (e) {
      fxRateError = `Failed to load daily FX rates: ${String(e)}`;
    }
  }

  async function loadFxRatesOfficial() {
    try {
      fxRatesOfficial = await invoke<FxRateOfficial[]>("list_fx_rates_official", { bookId: 1 });
    } catch (e) {
      fxRateError = `Failed to load official FX rates: ${String(e)}`;
    }
  }

  async function loadFxRateSources() {
    try {
      fxRateSources = await invoke<FxRateSource[]>("list_fx_rate_sources", { bookId: 1 });
    } catch (e) {
      fxRateError = `Failed to load FX rate sources: ${String(e)}`;
    }
  }

  function openNewFxDaily() {
    const activeCurrencies = currencies.filter(c => c.is_active);
    const defaultCurrency = currencies.find(c => c.is_default);
    newFxDaily = {
      from_currency_id: activeCurrencies[0]?.id || 0,
      to_currency_id: defaultCurrency?.id || 0,
      rate_date: new Date().toISOString().split("T")[0],
      rate: 1.0,
      source: "",
    };
    showFxDailyDialog = true;
  }

  function closeFxDailyDialog() {
    showFxDailyDialog = false;
  }

  async function saveFxDaily() {
    fxRateError = "";
    fxRateStatus = "";
    busy = true;
    try {
      await invoke("create_fx_rate_daily", {
        input: {
          book_id: 1,
          from_currency_id: newFxDaily.from_currency_id,
          to_currency_id: newFxDaily.to_currency_id,
          rate_date: newFxDaily.rate_date,
          rate: newFxDaily.rate,
          source: newFxDaily.source || null,
        },
      });
      fxRateStatus = "Daily FX rate added.";
      closeFxDailyDialog();
      await loadFxRatesDaily();
    } catch (e) {
      fxRateError = `Failed to add FX rate: ${String(e)}`;
    } finally {
      busy = false;
    }
  }

  async function deleteFxDaily(rate: FxRateDaily) {
    if (!confirm(`Delete FX rate ${rate.from_currency_symbol}/${rate.to_currency_symbol} on ${rate.rate_date}?`)) return;
    fxRateError = "";
    fxRateStatus = "";
    try {
      await invoke("delete_fx_rate_daily", { id: rate.id });
      fxRateStatus = "FX rate deleted.";
      await loadFxRatesDaily();
    } catch (e) {
      fxRateError = `Failed to delete FX rate: ${String(e)}`;
    }
  }

  function openNewFxOfficial() {
    const activeCurrencies = currencies.filter(c => c.is_active);
    const defaultCurrency = currencies.find(c => c.is_default);
    newFxOfficial = {
      from_currency_id: activeCurrencies[0]?.id || 0,
      to_currency_id: defaultCurrency?.id || 0,
      period_type: "yearly",
      period_year: new Date().getFullYear(),
      period_month: null,
      rate: 1.0,
      source_name: fxRateSources[0]?.name || "",
      source_url: "",
      notes: "",
    };
    showFxOfficialDialog = true;
  }

  function closeFxOfficialDialog() {
    showFxOfficialDialog = false;
  }

  async function saveFxOfficial() {
    fxRateError = "";
    fxRateStatus = "";
    busy = true;
    try {
      await invoke("create_fx_rate_official", {
        input: {
          book_id: 1,
          from_currency_id: newFxOfficial.from_currency_id,
          to_currency_id: newFxOfficial.to_currency_id,
          period_type: newFxOfficial.period_type,
          period_year: newFxOfficial.period_year,
          period_month: newFxOfficial.period_type === "monthly" ? newFxOfficial.period_month : null,
          rate: newFxOfficial.rate,
          source_name: newFxOfficial.source_name,
          source_url: newFxOfficial.source_url || null,
          source_date: null,
          notes: newFxOfficial.notes || null,
        },
      });
      fxRateStatus = "Official FX rate added.";
      closeFxOfficialDialog();
      await loadFxRatesOfficial();
    } catch (e) {
      fxRateError = `Failed to add official FX rate: ${String(e)}`;
    } finally {
      busy = false;
    }
  }

  async function deleteFxOfficial(rate: FxRateOfficial) {
    const period = rate.period_type === "monthly" ? `${rate.period_year}-${String(rate.period_month).padStart(2, "0")}` : String(rate.period_year);
    if (!confirm(`Delete official FX rate ${rate.from_currency_symbol}/${rate.to_currency_symbol} for ${period}?`)) return;
    fxRateError = "";
    fxRateStatus = "";
    try {
      await invoke("delete_fx_rate_official", { id: rate.id });
      fxRateStatus = "Official FX rate deleted.";
      await loadFxRatesOfficial();
    } catch (e) {
      fxRateError = `Failed to delete official FX rate: ${String(e)}`;
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

<main class="py-6">
  <div class="container mx-auto px-6 space-y-6">
    <div>
      <h1 class="text-3xl font-bold tracking-tight">Settings</h1>
      <p class="text-muted-foreground">Configure preferences and manage data.</p>
    </div>

    <!-- Tabs -->
    <Tabs.Root bind:value={activeTab}>
      <Tabs.List>
        <Tabs.Trigger value="database">Database</Tabs.Trigger>
        <Tabs.Trigger value="categories">Categories ({categories.length})</Tabs.Trigger>
        <Tabs.Trigger value="payees">Payees ({payees.length})</Tabs.Trigger>
        <Tabs.Trigger value="tags">Tags ({tags.length})</Tabs.Trigger>
        <Tabs.Trigger value="commodities">Currencies ({commodities.length})</Tabs.Trigger>
        <Tabs.Trigger value="institutions">Institutions ({institutions.length})</Tabs.Trigger>
      </Tabs.List>
    </Tabs.Root>

    <!-- Database Tab -->
    {#if activeTab === "database"}

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
    {/if}

    <!-- Categories Tab -->
    {#if activeTab === "categories"}
    <Card.Root>
      <Card.Header class="flex flex-row items-center justify-between">
        <Card.Title>Categories</Card.Title>
        <Button onclick={openNewCategory} disabled={busy}>
          Add Category
        </Button>
      </Card.Header>
      <Card.Content>
          {#if categoryStatus}
            <p class="text-sm text-green-600">{categoryStatus}</p>
          {/if}
          {#if categoryError}
            <p class="text-sm text-destructive">{categoryError}</p>
          {/if}

          <Table.Root>
            <Table.Header>
              <Table.Row>
                <Table.Head>Name</Table.Head>
                <Table.Head>Kind</Table.Head>
                <Table.Head>Color</Table.Head>
                <Table.Head class="text-right">Actions</Table.Head>
              </Table.Row>
            </Table.Header>
            <Table.Body>
              {#each categories as cat}
                <Table.Row>
                  <Table.Cell>{cat.name}</Table.Cell>
                  <Table.Cell>
                    <Badge variant={cat.kind === "expense" ? "destructive" : cat.kind === "income" ? "default" : "secondary"}>{cat.kind}</Badge>
                  </Table.Cell>
                  <Table.Cell>
                    {#if cat.color}
                      <span class="inline-block w-6 h-6 rounded border" style="background-color: {cat.color}"></span>
                    {:else}
                      —
                    {/if}
                  </Table.Cell>
                  <Table.Cell class="text-right">
                    <Button variant="ghost" size="sm" onclick={() => openEditCategory(cat)}>Edit</Button>
                    <Button variant="ghost" size="sm" class="text-destructive" onclick={() => deleteCategory(cat)}>Delete</Button>
                  </Table.Cell>
                </Table.Row>
              {:else}
                <Table.Row>
                  <Table.Cell colspan={4} class="text-muted-foreground">No categories found.</Table.Cell>
                </Table.Row>
              {/each}
            </Table.Body>
          </Table.Root>
      </Card.Content>
    </Card.Root>
    {/if}

    <!-- Payees Tab -->
    {#if activeTab === "payees"}
    <Card.Root>
      <Card.Header class="flex flex-row items-center justify-between">
        <Card.Title>Payees</Card.Title>
        <Button onclick={openNewPayee} disabled={busy}>
          Add Payee
        </Button>
      </Card.Header>
      <Card.Content>
          {#if payeeStatus}
            <p class="text-sm text-green-600">{payeeStatus}</p>
          {/if}
          {#if payeeError}
            <p class="text-sm text-destructive">{payeeError}</p>
          {/if}

          <Table.Root>
            <Table.Header>
              <Table.Row>
                <Table.Head>Name</Table.Head>
                <Table.Head>Kind</Table.Head>
                <Table.Head>Metadata</Table.Head>
                <Table.Head class="text-right">Actions</Table.Head>
              </Table.Row>
            </Table.Header>
            <Table.Body>
              {#each payees as p}
                <Table.Row>
                  <Table.Cell>{p.name}</Table.Cell>
                  <Table.Cell><Badge variant="outline">{p.kind}</Badge></Table.Cell>
                  <Table.Cell class="text-muted-foreground">{p.metadata || "—"}</Table.Cell>
                  <Table.Cell class="text-right">
                    <Button variant="ghost" size="sm" onclick={() => openEditPayee(p)}>Edit</Button>
                    <Button variant="ghost" size="sm" class="text-destructive" onclick={() => deletePayee(p)}>Delete</Button>
                  </Table.Cell>
                </Table.Row>
              {:else}
                <Table.Row>
                  <Table.Cell colspan={4} class="text-muted-foreground">No payees found.</Table.Cell>
                </Table.Row>
              {/each}
            </Table.Body>
          </Table.Root>
      </Card.Content>
    </Card.Root>
    {/if}

    <!-- Tags Tab -->
    {#if activeTab === "tags"}
    <Card.Root>
      <Card.Header class="flex flex-row items-center justify-between">
        <Card.Title>Tags</Card.Title>
        <Button onclick={openNewTag} disabled={busy}>
          Add Tag
        </Button>
      </Card.Header>
      <Card.Content>
          {#if tagStatus}
            <p class="text-sm text-green-600">{tagStatus}</p>
          {/if}
          {#if tagError}
            <p class="text-sm text-destructive">{tagError}</p>
          {/if}

          <Table.Root>
            <Table.Header>
              <Table.Row>
                <Table.Head>Name</Table.Head>
                <Table.Head>Color</Table.Head>
                <Table.Head class="text-right">Actions</Table.Head>
              </Table.Row>
            </Table.Header>
            <Table.Body>
              {#each tags as t}
                <Table.Row>
                  <Table.Cell>{t.name}</Table.Cell>
                  <Table.Cell>
                    {#if t.color}
                      <span class="inline-block w-6 h-6 rounded border" style="background-color: {t.color}"></span>
                    {:else}
                      —
                    {/if}
                  </Table.Cell>
                  <Table.Cell class="text-right">
                    <Button variant="ghost" size="sm" onclick={() => openEditTag(t)}>Edit</Button>
                    <Button variant="ghost" size="sm" class="text-destructive" onclick={() => deleteTag(t)}>Delete</Button>
                  </Table.Cell>
                </Table.Row>
              {:else}
                <Table.Row>
                  <Table.Cell colspan={3} class="text-muted-foreground">No tags found.</Table.Cell>
                </Table.Row>
              {/each}
            </Table.Body>
          </Table.Root>
      </Card.Content>
    </Card.Root>
    {/if}

    <!-- Commodities Tab -->
    {#if activeTab === "commodities"}
    <Card.Root>
      <Card.Header>
        <Card.Title>Currencies &amp; Commodities</Card.Title>
      </Card.Header>
      <Card.Content>
        <!-- Sub-tabs for currencies, commodities, fx rates -->
        <div class="flex gap-2 mb-4 border-b">
          <button
            class="px-4 py-2 text-sm font-medium {commoditiesSubTab === 'currencies' ? 'border-b-2 border-primary text-primary' : 'text-muted-foreground hover:text-foreground'}"
            onclick={() => commoditiesSubTab = 'currencies'}
          >
            Currencies
          </button>
          <button
            class="px-4 py-2 text-sm font-medium {commoditiesSubTab === 'commodities' ? 'border-b-2 border-primary text-primary' : 'text-muted-foreground hover:text-foreground'}"
            onclick={() => commoditiesSubTab = 'commodities'}
          >
            Other Commodities
          </button>
          <button
            class="px-4 py-2 text-sm font-medium {commoditiesSubTab === 'fx-daily' ? 'border-b-2 border-primary text-primary' : 'text-muted-foreground hover:text-foreground'}"
            onclick={() => { commoditiesSubTab = 'fx-daily'; loadFxRatesDaily(); }}
          >
            Daily FX Rates
          </button>
          <button
            class="px-4 py-2 text-sm font-medium {commoditiesSubTab === 'fx-official' ? 'border-b-2 border-primary text-primary' : 'text-muted-foreground hover:text-foreground'}"
            onclick={() => { commoditiesSubTab = 'fx-official'; loadFxRatesOfficial(); }}
          >
            Official FX Rates
          </button>
        </div>

        <!-- Currencies Sub-tab -->
        {#if commoditiesSubTab === 'currencies'}
          <div class="flex justify-between items-center mb-4">
            <p class="text-sm text-muted-foreground">Manage active currencies and set your default currency. The default currency is used for reports and conversions.</p>
            <Button onclick={openNewCurrency} disabled={busy} size="sm">Add Currency</Button>
          </div>
          {#if currencyStatus}
            <p class="text-sm text-green-600 mb-2">{currencyStatus}</p>
          {/if}
          {#if currencyError}
            <p class="text-sm text-destructive mb-2">{currencyError}</p>
          {/if}

          <Table.Root>
            <Table.Header>
              <Table.Row>
                <Table.Head>Code</Table.Head>
                <Table.Head>Symbol</Table.Head>
                <Table.Head>Name</Table.Head>
                <Table.Head>Scale</Table.Head>
                <Table.Head>Status</Table.Head>
                <Table.Head class="text-right">Actions</Table.Head>
              </Table.Row>
            </Table.Header>
            <Table.Body>
              {#each currencies as c}
                <Table.Row class={!c.is_active ? 'opacity-50' : ''}>
                  <Table.Cell class="font-mono font-semibold">{c.symbol || "—"}</Table.Cell>
                  <Table.Cell class="text-lg">{c.display_symbol || "—"}</Table.Cell>
                  <Table.Cell>{c.name}</Table.Cell>
                  <Table.Cell>{c.scale}</Table.Cell>
                  <Table.Cell>
                    {#if c.is_default}
                      <Badge variant="default">Default</Badge>
                    {:else if c.is_active}
                      <Badge variant="secondary">Active</Badge>
                    {:else}
                      <Badge variant="outline">Inactive</Badge>
                    {/if}
                  </Table.Cell>
                  <Table.Cell class="text-right">
                    {#if !c.is_default}
                      <Button variant="ghost" size="sm" onclick={() => setDefaultCurrency(c)}>Set Default</Button>
                      <Button variant="ghost" size="sm" onclick={() => toggleCurrencyActive(c)}>
                        {c.is_active ? 'Deactivate' : 'Activate'}
                      </Button>
                    {/if}
                  </Table.Cell>
                </Table.Row>
              {:else}
                <Table.Row>
                  <Table.Cell colspan={6} class="text-muted-foreground">No currencies found.</Table.Cell>
                </Table.Row>
              {/each}
            </Table.Body>
          </Table.Root>
        {/if}

        <!-- Other Commodities Sub-tab -->
        {#if commoditiesSubTab === 'commodities'}
          {#if commodityStatus}
            <p class="text-sm text-green-600 mb-2">{commodityStatus}</p>
          {/if}
          {#if commodityError}
            <p class="text-sm text-destructive mb-2">{commodityError}</p>
          {/if}

          <Table.Root>
            <Table.Header>
              <Table.Row>
                <Table.Head>Symbol</Table.Head>
                <Table.Head>Name</Table.Head>
                <Table.Head>Kind</Table.Head>
                <Table.Head>Scale</Table.Head>
                <Table.Head class="text-right">Actions</Table.Head>
              </Table.Row>
            </Table.Header>
            <Table.Body>
              {#each commodities.filter(c => c.kind !== 'currency') as c}
                <Table.Row>
                  <Table.Cell class="font-mono">{c.symbol || "—"}</Table.Cell>
                  <Table.Cell>{c.name}</Table.Cell>
                  <Table.Cell>
                    <Badge variant="default">{c.kind}</Badge>
                  </Table.Cell>
                  <Table.Cell>{c.scale}</Table.Cell>
                  <Table.Cell class="text-right">
                    <Button variant="ghost" size="sm" onclick={() => openEditCommodity(c)}>Edit</Button>
                  </Table.Cell>
                </Table.Row>
              {:else}
                <Table.Row>
                  <Table.Cell colspan={5} class="text-muted-foreground">No non-currency commodities found.</Table.Cell>
                </Table.Row>
              {/each}
            </Table.Body>
          </Table.Root>
        {/if}

        <!-- Daily FX Rates Sub-tab -->
        {#if commoditiesSubTab === 'fx-daily'}
          <div class="flex justify-between items-center mb-4">
            <p class="text-sm text-muted-foreground">Market exchange rates for daily currency conversion.</p>
            <Button onclick={openNewFxDaily} disabled={busy} size="sm">Add Rate</Button>
          </div>
          {#if fxRateStatus}
            <p class="text-sm text-green-600 mb-2">{fxRateStatus}</p>
          {/if}
          {#if fxRateError}
            <p class="text-sm text-destructive mb-2">{fxRateError}</p>
          {/if}

          <Table.Root>
            <Table.Header>
              <Table.Row>
                <Table.Head>Date</Table.Head>
                <Table.Head>From</Table.Head>
                <Table.Head>To</Table.Head>
                <Table.Head>Rate</Table.Head>
                <Table.Head>Source</Table.Head>
                <Table.Head class="text-right">Actions</Table.Head>
              </Table.Row>
            </Table.Header>
            <Table.Body>
              {#each fxRatesDaily as rate}
                <Table.Row>
                  <Table.Cell>{rate.rate_date}</Table.Cell>
                  <Table.Cell class="font-mono">{rate.from_currency_symbol || '?'}</Table.Cell>
                  <Table.Cell class="font-mono">{rate.to_currency_symbol || '?'}</Table.Cell>
                  <Table.Cell class="font-mono">{rate.rate.toFixed(6)}</Table.Cell>
                  <Table.Cell class="text-muted-foreground">{rate.source || '—'}</Table.Cell>
                  <Table.Cell class="text-right">
                    <Button variant="ghost" size="sm" class="text-destructive" onclick={() => deleteFxDaily(rate)}>Delete</Button>
                  </Table.Cell>
                </Table.Row>
              {:else}
                <Table.Row>
                  <Table.Cell colspan={6} class="text-muted-foreground">No daily FX rates found.</Table.Cell>
                </Table.Row>
              {/each}
            </Table.Body>
          </Table.Root>
        {/if}

        <!-- Official FX Rates Sub-tab -->
        {#if commoditiesSubTab === 'fx-official'}
          <div class="flex justify-between items-center mb-4">
            <p class="text-sm text-muted-foreground">Official exchange rates from tax authorities for tax reporting purposes.</p>
            <Button onclick={openNewFxOfficial} disabled={busy} size="sm">Add Official Rate</Button>
          </div>
          {#if fxRateStatus}
            <p class="text-sm text-green-600 mb-2">{fxRateStatus}</p>
          {/if}
          {#if fxRateError}
            <p class="text-sm text-destructive mb-2">{fxRateError}</p>
          {/if}

          <Table.Root>
            <Table.Header>
              <Table.Row>
                <Table.Head>Period</Table.Head>
                <Table.Head>From</Table.Head>
                <Table.Head>To</Table.Head>
                <Table.Head>Rate</Table.Head>
                <Table.Head>Source</Table.Head>
                <Table.Head class="text-right">Actions</Table.Head>
              </Table.Row>
            </Table.Header>
            <Table.Body>
              {#each fxRatesOfficial as rate}
                <Table.Row>
                  <Table.Cell>
                    {#if rate.period_type === 'monthly'}
                      {rate.period_year}-{String(rate.period_month).padStart(2, '0')}
                    {:else}
                      {rate.period_year}
                    {/if}
                    <Badge variant="outline" class="ml-2">{rate.period_type}</Badge>
                  </Table.Cell>
                  <Table.Cell class="font-mono">{rate.from_currency_symbol || '?'}</Table.Cell>
                  <Table.Cell class="font-mono">{rate.to_currency_symbol || '?'}</Table.Cell>
                  <Table.Cell class="font-mono">{rate.rate.toFixed(6)}</Table.Cell>
                  <Table.Cell>
                    {#if rate.source_url}
                      <a href={rate.source_url} target="_blank" rel="noopener" class="text-primary hover:underline">{rate.source_name}</a>
                    {:else}
                      {rate.source_name}
                    {/if}
                  </Table.Cell>
                  <Table.Cell class="text-right">
                    <Button variant="ghost" size="sm" class="text-destructive" onclick={() => deleteFxOfficial(rate)}>Delete</Button>
                  </Table.Cell>
                </Table.Row>
              {:else}
                <Table.Row>
                  <Table.Cell colspan={6} class="text-muted-foreground">No official FX rates found.</Table.Cell>
                </Table.Row>
              {/each}
            </Table.Body>
          </Table.Root>
        {/if}
      </Card.Content>
    </Card.Root>
    {/if}

    <!-- Institutions Tab -->
    {#if activeTab === "institutions"}
    <Card.Root>
      <Card.Header class="flex flex-row items-center justify-between">
        <Card.Title>Institutions</Card.Title>
        <Button onclick={openNewInstitution} disabled={busy}>
          Add Institution
        </Button>
      </Card.Header>
      <Card.Content>
          {#if institutionStatus}
            <p class="text-sm text-green-600">{institutionStatus}</p>
          {/if}
          {#if institutionError}
            <p class="text-sm text-destructive">{institutionError}</p>
          {/if}

          <Table.Root>
            <Table.Header>
              <Table.Row>
                <Table.Head>Name</Table.Head>
                <Table.Head>Kind</Table.Head>
                <Table.Head>Routing</Table.Head>
                <Table.Head>Website</Table.Head>
                <Table.Head class="text-right">Actions</Table.Head>
              </Table.Row>
            </Table.Header>
            <Table.Body>
              {#each institutions as inst}
                <Table.Row>
                  <Table.Cell>{inst.name}</Table.Cell>
                  <Table.Cell><Badge variant="outline">{inst.kind || "—"}</Badge></Table.Cell>
                  <Table.Cell class="font-mono">{inst.routing || "—"}</Table.Cell>
                  <Table.Cell>
                    {#if inst.website}
                      <a href={inst.website} target="_blank" rel="noopener" class="text-primary hover:underline">{inst.website}</a>
                    {:else}
                      —
                    {/if}
                  </Table.Cell>
                  <Table.Cell class="text-right">
                    <Button variant="ghost" size="sm" onclick={() => openEditInstitution(inst)}>Edit</Button>
                    <Button variant="ghost" size="sm" class="text-destructive" onclick={() => deleteInstitution(inst)}>Delete</Button>
                  </Table.Cell>
                </Table.Row>
              {:else}
                <Table.Row>
                  <Table.Cell colspan={5} class="text-muted-foreground">No institutions found.</Table.Cell>
                </Table.Row>
              {/each}
            </Table.Body>
          </Table.Root>
      </Card.Content>
    </Card.Root>
    {/if}

  </div>
</main>

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

<!-- Category Dialog -->
<Dialog.Root bind:open={showCategoryDialog}>
  <Dialog.Content>
    <Dialog.Header>
      <Dialog.Title>{editingCategory ? "Edit Category" : "New Category"}</Dialog.Title>
    </Dialog.Header>
    <div class="grid gap-4 py-4">
      <div class="grid gap-2">
        <Label for="category-name">Name</Label>
        <Input id="category-name" type="text" bind:value={newCategory.name} placeholder="Category name" />
      </div>
      <div class="grid gap-2">
        <Label for="category-kind">Kind</Label>
        <select id="category-kind" class="flex h-9 w-full rounded-md border border-input bg-transparent px-3 py-1 text-sm shadow-sm transition-colors focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring" bind:value={newCategory.kind}>
          <option value="expense">Expense</option>
          <option value="income">Income</option>
          <option value="transfer">Transfer</option>
          <option value="other">Other</option>
        </select>
      </div>
      <div class="grid gap-2">
        <Label for="category-parent">Parent Category</Label>
        <select id="category-parent" class="flex h-9 w-full rounded-md border border-input bg-transparent px-3 py-1 text-sm shadow-sm transition-colors focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring" bind:value={newCategory.parent_id}>
          <option value={null}>None (Top-level)</option>
          {#each categories.filter((c) => c.id !== editingCategory?.id) as cat}
            <option value={cat.id}>{cat.name}</option>
          {/each}
        </select>
      </div>
      <div class="grid gap-2">
        <Label for="category-color">Color</Label>
        <Input id="category-color" type="color" bind:value={newCategory.color} />
      </div>
    </div>
    <Dialog.Footer>
      <Button variant="secondary" onclick={closeCategoryDialog}>Cancel</Button>
      <Button onclick={saveCategory} disabled={busy || !newCategory.name}>
        {editingCategory ? "Update" : "Create"}
      </Button>
    </Dialog.Footer>
  </Dialog.Content>
</Dialog.Root>

<!-- Payee Dialog -->
<Dialog.Root bind:open={showPayeeDialog}>
  <Dialog.Content>
    <Dialog.Header>
      <Dialog.Title>{editingPayee ? "Edit Payee" : "New Payee"}</Dialog.Title>
    </Dialog.Header>
    <div class="grid gap-4 py-4">
      <div class="grid gap-2">
        <Label for="payee-name">Name</Label>
        <Input id="payee-name" type="text" bind:value={newPayee.name} placeholder="Payee name" />
      </div>
      <div class="grid gap-2">
        <Label for="payee-kind">Kind</Label>
        <select id="payee-kind" class="flex h-9 w-full rounded-md border border-input bg-transparent px-3 py-1 text-sm shadow-sm transition-colors focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring" bind:value={newPayee.kind}>
          <option value="person">Person</option>
          <option value="business">Business</option>
          <option value="government">Government</option>
          <option value="other">Other</option>
        </select>
      </div>
      <div class="grid gap-2">
        <Label for="payee-metadata">Metadata (optional)</Label>
        <Input id="payee-metadata" type="text" bind:value={newPayee.metadata} placeholder="Notes or additional info" />
      </div>
    </div>
    <Dialog.Footer>
      <Button variant="secondary" onclick={closePayeeDialog}>Cancel</Button>
      <Button onclick={savePayee} disabled={busy || !newPayee.name}>
        {editingPayee ? "Update" : "Create"}
      </Button>
    </Dialog.Footer>
  </Dialog.Content>
</Dialog.Root>

<!-- Tag Dialog -->
<Dialog.Root bind:open={showTagDialog}>
  <Dialog.Content>
    <Dialog.Header>
      <Dialog.Title>{editingTag ? "Edit Tag" : "New Tag"}</Dialog.Title>
    </Dialog.Header>
    <div class="grid gap-4 py-4">
      <div class="grid gap-2">
        <Label for="tag-name">Name</Label>
        <Input id="tag-name" type="text" bind:value={newTag.name} placeholder="Tag name" />
      </div>
      <div class="grid gap-2">
        <Label for="tag-color">Color</Label>
        <Input id="tag-color" type="color" bind:value={newTag.color} />
      </div>
    </div>
    <Dialog.Footer>
      <Button variant="secondary" onclick={closeTagDialog}>Cancel</Button>
      <Button onclick={saveTag} disabled={busy || !newTag.name}>
        {editingTag ? "Update" : "Create"}
      </Button>
    </Dialog.Footer>
  </Dialog.Content>
</Dialog.Root>

<!-- Commodity Dialog -->
<Dialog.Root bind:open={showCommodityDialog}>
  <Dialog.Content>
    <Dialog.Header>
      <Dialog.Title>Edit Commodity</Dialog.Title>
    </Dialog.Header>
    {#if editingCommodity}
    <div class="grid gap-4 py-4">
      <div class="grid gap-2">
        <Label for="commodity-symbol">Symbol</Label>
        <Input id="commodity-symbol" type="text" bind:value={editingCommodity.symbol} placeholder="e.g. USD, EUR, BTC" />
      </div>
      <div class="grid gap-2">
        <Label for="commodity-name">Name</Label>
        <Input id="commodity-name" type="text" bind:value={editingCommodity.name} placeholder="Full name" />
      </div>
      <div class="grid gap-2">
        <Label for="commodity-kind">Kind</Label>
        <Input id="commodity-kind" type="text" value={editingCommodity.kind} disabled />
        <p class="text-sm text-muted-foreground">Kind cannot be changed</p>
      </div>
      <div class="grid gap-2">
        <Label for="commodity-scale">Scale (decimal places)</Label>
        <Input id="commodity-scale" type="number" value={editingCommodity.scale} disabled />
        <p class="text-sm text-muted-foreground">Scale cannot be changed</p>
      </div>
    </div>
    {/if}
    <Dialog.Footer>
      <Button variant="secondary" onclick={closeCommodityDialog}>Cancel</Button>
      <Button onclick={saveCommodity} disabled={busy || !editingCommodity?.name}>
        Update
      </Button>
    </Dialog.Footer>
  </Dialog.Content>
</Dialog.Root>

<!-- Institution Dialog -->
<Dialog.Root bind:open={showInstitutionDialog}>
  <Dialog.Content>
    <Dialog.Header>
      <Dialog.Title>{editingInstitution ? "Edit Institution" : "New Institution"}</Dialog.Title>
    </Dialog.Header>
    <div class="grid gap-4 py-4">
      <div class="grid gap-2">
        <Label for="institution-name">Name</Label>
        <Input id="institution-name" type="text" bind:value={newInstitution.name} placeholder="Institution name" />
      </div>
      <div class="grid gap-2">
        <Label for="institution-kind">Kind</Label>
        <select id="institution-kind" class="flex h-9 w-full rounded-md border border-input bg-transparent px-3 py-1 text-sm shadow-sm transition-colors focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring" bind:value={newInstitution.kind}>
          <option value="">None</option>
          <option value="bank">Bank</option>
          <option value="credit_union">Credit Union</option>
          <option value="brokerage">Brokerage</option>
          <option value="insurance">Insurance</option>
          <option value="employer">Employer</option>
          <option value="other">Other</option>
        </select>
      </div>
      <div class="grid gap-2">
        <Label for="institution-routing">Routing Number (optional)</Label>
        <Input id="institution-routing" type="text" bind:value={newInstitution.routing} placeholder="e.g. 123456789" />
      </div>
      <div class="grid gap-2">
        <Label for="institution-website">Website (optional)</Label>
        <Input id="institution-website" type="url" bind:value={newInstitution.website} placeholder="https://..." />
      </div>
      <div class="grid gap-2">
        <Label for="institution-metadata">Metadata (optional)</Label>
        <Input id="institution-metadata" type="text" bind:value={newInstitution.metadata} placeholder="Additional notes" />
      </div>
    </div>
    <Dialog.Footer>
      <Button variant="secondary" onclick={closeInstitutionDialog}>Cancel</Button>
      <Button onclick={saveInstitution} disabled={busy || !newInstitution.name}>
        {editingInstitution ? "Update" : "Create"}
      </Button>
    </Dialog.Footer>
  </Dialog.Content>
</Dialog.Root>

<!-- New Currency Dialog -->
<Dialog.Root bind:open={showCurrencyDialog}>
  <Dialog.Content>
    <Dialog.Header>
      <Dialog.Title>Add Currency</Dialog.Title>
    </Dialog.Header>
    <div class="grid gap-4 py-4">
      <div class="grid grid-cols-2 gap-4">
        <div class="grid gap-2">
          <Label for="currency-symbol">Code (ISO)</Label>
          <Input id="currency-symbol" type="text" bind:value={newCurrency.symbol} placeholder="e.g. EUR, GBP, JPY" maxlength={10} />
          <p class="text-sm text-muted-foreground">ISO 4217 code</p>
        </div>
        <div class="grid gap-2">
          <Label for="currency-display-symbol">Display Symbol</Label>
          <Input id="currency-display-symbol" type="text" bind:value={newCurrency.display_symbol} placeholder="e.g. €, £, ¥, $" maxlength={5} />
          <p class="text-sm text-muted-foreground">Unicode symbol</p>
        </div>
      </div>
      <div class="grid gap-2">
        <Label for="currency-name">Name</Label>
        <Input id="currency-name" type="text" bind:value={newCurrency.name} placeholder="e.g. Euro, British Pound" />
      </div>
      <div class="grid gap-2">
        <Label for="currency-scale">Decimal Places</Label>
        <select id="currency-scale" class="flex h-9 w-full rounded-md border border-input bg-transparent px-3 py-1 text-sm shadow-sm transition-colors focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring" bind:value={newCurrency.scale}>
          <option value={0}>0 (JPY, KRW, etc.)</option>
          <option value={2}>2 (Most currencies)</option>
          <option value={3}>3 (BHD, KWD, etc.)</option>
        </select>
      </div>
    </div>
    <Dialog.Footer>
      <Button variant="secondary" onclick={closeCurrencyDialog}>Cancel</Button>
      <Button onclick={saveCurrency} disabled={busy || !newCurrency.symbol || !newCurrency.name}>
        Create
      </Button>
    </Dialog.Footer>
  </Dialog.Content>
</Dialog.Root>

<!-- Daily FX Rate Dialog -->
<Dialog.Root bind:open={showFxDailyDialog}>
  <Dialog.Content>
    <Dialog.Header>
      <Dialog.Title>Add Daily FX Rate</Dialog.Title>
    </Dialog.Header>
    <div class="grid gap-4 py-4">
      <div class="grid gap-2">
        <Label for="fx-daily-date">Date</Label>
        <Input id="fx-daily-date" type="date" bind:value={newFxDaily.rate_date} />
      </div>
      <div class="grid gap-2">
        <Label for="fx-daily-from">From Currency</Label>
        <select id="fx-daily-from" class="flex h-9 w-full rounded-md border border-input bg-transparent px-3 py-1 text-sm shadow-sm transition-colors focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring" bind:value={newFxDaily.from_currency_id}>
          {#each currencies.filter(c => c.is_active) as c}
            <option value={c.id}>{c.display_symbol || ''} {c.symbol} - {c.name}</option>
          {/each}
        </select>
      </div>
      <div class="grid gap-2">
        <Label for="fx-daily-to">To Currency</Label>
        <select id="fx-daily-to" class="flex h-9 w-full rounded-md border border-input bg-transparent px-3 py-1 text-sm shadow-sm transition-colors focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring" bind:value={newFxDaily.to_currency_id}>
          {#each currencies.filter(c => c.is_active) as c}
            <option value={c.id}>{c.display_symbol || ''} {c.symbol} - {c.name}</option>
          {/each}
        </select>
      </div>
      <div class="grid gap-2">
        <Label for="fx-daily-rate">Exchange Rate</Label>
        <Input id="fx-daily-rate" type="number" step="0.000001" bind:value={newFxDaily.rate} />
        <p class="text-sm text-muted-foreground">1 unit of "From" currency = this many units of "To" currency</p>
      </div>
      <div class="grid gap-2">
        <Label for="fx-daily-source">Source (optional)</Label>
        <Input id="fx-daily-source" type="text" bind:value={newFxDaily.source} placeholder="e.g. ECB, XE.com, bank" />
      </div>
    </div>
    <Dialog.Footer>
      <Button variant="secondary" onclick={closeFxDailyDialog}>Cancel</Button>
      <Button onclick={saveFxDaily} disabled={busy || !newFxDaily.rate_date || !newFxDaily.rate}>
        Add Rate
      </Button>
    </Dialog.Footer>
  </Dialog.Content>
</Dialog.Root>

<!-- Official FX Rate Dialog -->
<Dialog.Root bind:open={showFxOfficialDialog}>
  <Dialog.Content>
    <Dialog.Header>
      <Dialog.Title>Add Official FX Rate</Dialog.Title>
      <p class="text-sm text-muted-foreground">Official rates from tax authorities for tax reporting</p>
    </Dialog.Header>
    <div class="grid gap-4 py-4">
      <div class="grid grid-cols-2 gap-4">
        <div class="grid gap-2">
          <Label for="fx-official-type">Period Type</Label>
          <select id="fx-official-type" class="flex h-9 w-full rounded-md border border-input bg-transparent px-3 py-1 text-sm shadow-sm transition-colors focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring" bind:value={newFxOfficial.period_type}>
            <option value="yearly">Yearly Average</option>
            <option value="monthly">Monthly Average</option>
          </select>
        </div>
        <div class="grid gap-2">
          <Label for="fx-official-year">Year</Label>
          <Input id="fx-official-year" type="number" bind:value={newFxOfficial.period_year} min="1990" max="2100" />
        </div>
      </div>
      {#if newFxOfficial.period_type === 'monthly'}
        <div class="grid gap-2">
          <Label for="fx-official-month">Month</Label>
          <select id="fx-official-month" class="flex h-9 w-full rounded-md border border-input bg-transparent px-3 py-1 text-sm shadow-sm transition-colors focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring" bind:value={newFxOfficial.period_month}>
            <option value={1}>January</option>
            <option value={2}>February</option>
            <option value={3}>March</option>
            <option value={4}>April</option>
            <option value={5}>May</option>
            <option value={6}>June</option>
            <option value={7}>July</option>
            <option value={8}>August</option>
            <option value={9}>September</option>
            <option value={10}>October</option>
            <option value={11}>November</option>
            <option value={12}>December</option>
          </select>
        </div>
      {/if}
      <div class="grid gap-2">
        <Label for="fx-official-from">From Currency</Label>
        <select id="fx-official-from" class="flex h-9 w-full rounded-md border border-input bg-transparent px-3 py-1 text-sm shadow-sm transition-colors focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring" bind:value={newFxOfficial.from_currency_id}>
          {#each currencies.filter(c => c.is_active) as c}
            <option value={c.id}>{c.display_symbol || ''} {c.symbol} - {c.name}</option>
          {/each}
        </select>
      </div>
      <div class="grid gap-2">
        <Label for="fx-official-to">To Currency</Label>
        <select id="fx-official-to" class="flex h-9 w-full rounded-md border border-input bg-transparent px-3 py-1 text-sm shadow-sm transition-colors focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring" bind:value={newFxOfficial.to_currency_id}>
          {#each currencies.filter(c => c.is_active) as c}
            <option value={c.id}>{c.display_symbol || ''} {c.symbol} - {c.name}</option>
          {/each}
        </select>
      </div>
      <div class="grid gap-2">
        <Label for="fx-official-rate">Exchange Rate</Label>
        <Input id="fx-official-rate" type="number" step="0.000001" bind:value={newFxOfficial.rate} />
      </div>
      <div class="grid gap-2">
        <Label for="fx-official-source">Source Authority</Label>
        <select id="fx-official-source" class="flex h-9 w-full rounded-md border border-input bg-transparent px-3 py-1 text-sm shadow-sm transition-colors focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring" bind:value={newFxOfficial.source_name}>
          {#each fxRateSources as src}
            <option value={src.name}>{src.name}{src.country_code ? ` (${src.country_code})` : ''}</option>
          {/each}
          <option value="">Other</option>
        </select>
      </div>
      {#if !fxRateSources.find(s => s.name === newFxOfficial.source_name)}
        <div class="grid gap-2">
          <Label for="fx-official-source-custom">Custom Source Name</Label>
          <Input id="fx-official-source-custom" type="text" bind:value={newFxOfficial.source_name} placeholder="e.g. Tax Authority Name" />
        </div>
      {/if}
      <div class="grid gap-2">
        <Label for="fx-official-url">Source URL (optional)</Label>
        <Input id="fx-official-url" type="url" bind:value={newFxOfficial.source_url} placeholder="https://..." />
      </div>
      <div class="grid gap-2">
        <Label for="fx-official-notes">Notes (optional)</Label>
        <Input id="fx-official-notes" type="text" bind:value={newFxOfficial.notes} placeholder="Additional notes" />
      </div>
    </div>
    <Dialog.Footer>
      <Button variant="secondary" onclick={closeFxOfficialDialog}>Cancel</Button>
      <Button onclick={saveFxOfficial} disabled={busy || !newFxOfficial.rate || !newFxOfficial.source_name}>
        Add Rate
      </Button>
    </Dialog.Footer>
  </Dialog.Content>
</Dialog.Root>
