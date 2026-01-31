# Storage Management: Feature List + Implementation Plan

This document enumerates storage-related features and an implementation plan for robust database management (open/move/copy/backup/restore), including migrations and safety checks.

---

## A. Current Capabilities (Observed)
- Storage location persistence via `storage-path.txt`.
- Default storage directory selection on first run.
- Resolve DB path and run migrations on open.
- Basic DB accessibility checks.

---

## B. Feature List (Requested + Recommended)

### 1) Open a Different Database Folder
- **Browse & select** a folder or `.rekenraam` file.
- **Validate** read/write access.
- **Auto-migrate** if needed.
- **Update config** to persist selection.
- **Show current DB path** in settings/status.

### 2) Move Existing Folder
- **Move DB file** and any related metadata to new folder.
- **Lock + close DB** before move.
- **Atomic move** with fallback copy/delete.
- **Update config** to new location.

### 3) Create New Folder/Database
- **Create empty DB** in selected folder and run migrations.
- **Create from template** (seeded book, base currency, starter accounts).
- **Copy existing DB** (“Save As / Duplicate”).

### 4) Automatic Backups
- **On app close** backup.
- **Interval-based** backups (every N minutes).
- **Retention policy** (keep last X backups or last Y days).
- **Backup location** configurable (same folder, subfolder, external path).

### 5) Restore From Backup
- **Browse backup file** and restore to active DB.
- **Safe restore flow**: close DB, copy backup to new file, re-open, migrate if needed.
- **Optional: restore to new location** (“Restore As”).

### 6) Additional Recommended Features
- **DB health check** (quick `SELECT 1` + PRAGMA integrity_check on demand).
- **Manual “Vacuum/Optimize”** action.
- **Read-only mode** when DB is locked or on error.
- **Recent files list** and quick switcher.
- **Export current DB path** and size info.
- **Conflict detection** if DB is open elsewhere.
- **Migration status UI** (current schema version, pending migrations).
- **Prompt on incompatible schema** (future version).

---

## C. Backend Commands (Proposed)

### Storage Commands
- `get_storage_location()` (exists)
- `set_storage_location(path)` (validate + migrate)
- `open_storage_path(path)` (alias with migration)
- `move_storage(old_path, new_path)`
- `create_new_storage(path)`
- `copy_storage(src_path, dest_path)`
- `get_db_path()` (exists)
- `get_schema_version()` (exists)

### Backup Commands
- `create_backup(dest_path?)`
- `list_backups(path?)`
- `restore_backup(backup_path, target_path?)`
- `set_backup_settings({ enabled, interval_minutes, retention_count, backup_path })`
- `get_backup_settings()`

### Health/Maintenance
- `db_integrity_check()`
- `db_vacuum()`
- `db_stats()` (file size, last modified)

---

## D. UI/UX Plan

### Settings → Storage Section
- Current DB path (copy button)
- “Open Database…” (folder/file picker)
- “Move Database…” (destination picker)
- “Create New Database…” (empty/template/copy)
- Schema version + migration status
- DB health check button

### Settings → Backups Section
- Enable auto-backups toggle
- Interval picker (5/10/15/30/60 min)
- Retention count input
- Backup location picker
- Manual “Create Backup Now” button
- Restore from backup flow

---

## E. Implementation Plan (Phased)

### Phase 1: Storage Switching + Creation
1. Add backend commands to open/migrate selected path.
2. Add UI picker for “Open Database”.
3. Add “Create New Database” (empty) flow.

### Phase 2: Move/Copy + Restore
1. ✅ Implement move/copy with safe close/reopen.
2. ✅ Add restore flow from backup.
3. ✅ Add “Save As / Duplicate” UI.

### Phase 3: Backups
1. ✅ Add backup settings + scheduler (interval + on close).
2. ✅ Implement retention policy and backup listing.
3. ✅ UI: backup controls + history list.

### Phase 4: Maintenance + Health
1. ✅ Add integrity checks and vacuum.
2. ✅ Display stats, schema version, migration status.
3. ✅ Add error handling + read-only mode prompts.

---

## F. Acceptance Criteria
- Switching databases never corrupts or loses data.
- Backups are created reliably on close and interval.
- Restore flow is safe and reversible.
- All commands report clear errors and status.
- UI clearly shows current path, schema version, and backup status.
