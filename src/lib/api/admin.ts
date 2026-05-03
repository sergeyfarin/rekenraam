import { invokeTauri } from "$lib/api/client";

export async function closeFiscalYear<T>(input: { close_date: string; memo: string | null }): Promise<T> {
  return invokeTauri<T>("close_fiscal_year", { input });
}

export async function getDbPath(): Promise<string | null> {
  return invokeTauri<string | null>("get_db_path");
}

export async function pickStorageFile(): Promise<string | null> {
  return invokeTauri<string | null>("pick_storage_file");
}

export async function pickStorageFolder(): Promise<string | null> {
  return invokeTauri<string | null>("pick_storage_folder");
}

export async function pickBackupFolder(): Promise<string | null> {
  return invokeTauri<string | null>("pick_backup_folder");
}

export async function pickBackupFile(): Promise<string | null> {
  return invokeTauri<string | null>("pick_backup_file");
}

export async function getBackupSettings<T>(): Promise<T> {
  return invokeTauri<T>("get_backup_settings");
}

export async function setBackupSettings<T>(settings: Record<string, unknown>): Promise<T> {
  return invokeTauri<T>("set_backup_settings", { settings });
}

export async function createBackup(): Promise<string> {
  return invokeTauri<string>("create_backup");
}

export async function listBackups<T>(): Promise<T> {
  return invokeTauri<T>("list_backups");
}

export async function dbStats<T>(): Promise<T> {
  return invokeTauri<T>("db_stats");
}

export async function getMigrationStatus<T>(): Promise<T> {
  return invokeTauri<T>("get_migration_status");
}

export async function dbIntegrityCheck(): Promise<string> {
  return invokeTauri<string>("db_integrity_check");
}

export async function dbVacuum(): Promise<string> {
  return invokeTauri<string>("db_vacuum");
}

export async function createNewStorage(path: string): Promise<string> {
  return invokeTauri<string>("create_new_storage", { path });
}

export async function moveStorage(path: string): Promise<string> {
  return invokeTauri<string>("move_storage", { path });
}

export async function copyStorage(path: string): Promise<string> {
  return invokeTauri<string>("copy_storage", { path });
}

export async function restoreFromBackup(backupPath: string): Promise<string> {
  return invokeTauri<string>("restore_from_backup", { backup_path: backupPath });
}

export async function validateAndSetStorageLocation(path: string): Promise<string> {
  return invokeTauri<string>("validate_and_set_storage_location", { path });
}