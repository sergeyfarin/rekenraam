// Learn more about Tauri commands at https://tauri.app/develop/calling-rust/
use serde::{Deserialize, Serialize};
use std::{
    fs,
    path::PathBuf,
    sync::{Arc, Mutex},
    time::Duration,
};
use tauri::{
    async_runtime::{self, JoinHandle},
    Manager, PhysicalPosition, PhysicalSize, Position, Size, Window, WindowEvent, WebviewWindow,
};
use tokio::time::sleep;
use rfd::FileDialog;
mod db;
mod commands;
mod state;

use crate::db::{
    create_db_placeholder, db_accessible, default_storage_dir, load_storage_path,
    open_and_migrate, resolve_accessible_db, save_storage_path,
};
use crate::state::DbState;

#[derive(Serialize, Deserialize, Debug)]
struct WindowState {
    x: i32,
    y: i32,
    width: u32,
    height: u32,
}

fn load_window_state() -> Option<WindowState> {
    let path = state_file()?;
    let content = std::fs::read_to_string(path).ok()?;
    serde_json::from_str(&content).ok()
}

fn state_file() -> Option<PathBuf> {
    let mut path = dirs_next::data_local_dir()?;
    path.push("rekenraam");
    std::fs::create_dir_all(&path).ok()?;
    path.push("window-state.json");
    Some(path)
}

fn save_window_state(window: &Window) {
    if let Some(path) = state_file() {
        let pos = window.outer_position().ok();
        let size = window.inner_size().unwrap_or_else(|_| PhysicalSize::new(800, 600));
        let state = if let Some(pos) = pos {
            WindowState { x: pos.x, y: pos.y, width: size.width, height: size.height }
        } else {
            WindowState { x: 0, y: 0, width: size.width, height: size.height }
        };
        if let Ok(serialized) = serde_json::to_string_pretty(&state) {
            let _ = std::fs::write(path, serialized);
        }
    }
}

type DebounceState = Arc<Mutex<Option<JoinHandle<()>>>>;
const MIN_SIZE: (u32, u32) = (400, 300);
const MAX_MARGIN: u32 = 100;
const DEBOUNCE_MS: Duration = Duration::from_millis(500);

fn fit_window_state_to_monitor(window: &WebviewWindow, mut state: WindowState) -> WindowState {
    let mut width = state.width.max(MIN_SIZE.0);
    let mut height = state.height.max(MIN_SIZE.1);

    if let Ok(Some(monitor)) = window.current_monitor() {
        let mon_pos = monitor.position().to_owned();
        let mon_size = monitor.size().to_owned();
        let mon_left = mon_pos.x;
        let mon_top = mon_pos.y;
        let mon_right = mon_left + mon_size.width as i32;
        let mon_bottom = mon_top + mon_size.height as i32;

        let max_w = mon_size.width.saturating_sub(MAX_MARGIN);
        let max_h = mon_size.height.saturating_sub(MAX_MARGIN);
        width = width.min(max_w);
        height = height.min(max_h);

        let win_area = (width as i64) * (height as i64);
        let win_left = state.x;
        let win_top = state.y;
        let win_right = state.x + width as i32;
        let win_bottom = state.y + height as i32;

        let inter_left = win_left.max(mon_left);
        let inter_top = win_top.max(mon_top);
        let inter_right = win_right.min(mon_right);
        let inter_bottom = win_bottom.min(mon_bottom);

        let inter_w = (inter_right - inter_left).max(0) as i64;
        let inter_h = (inter_bottom - inter_top).max(0) as i64;
        let inter_area = inter_w * inter_h;

        if win_area == 0 || (inter_area as f64) < 0.3 * (win_area as f64) {
            state.width = width;
            state.height = height;
            state.x = mon_left + ((mon_size.width as i32 - width as i32) / 2);
            state.y = mon_top + ((mon_size.height as i32 - height as i32) / 2);
            return state;
        }

        let mut x = state.x;
        let mut y = state.y;
        if x < mon_left {
            x = mon_left;
        }
        if y < mon_top {
            y = mon_top;
        }
        if x + width as i32 > mon_right {
            x = mon_right - width as i32;
        }
        if y + height as i32 > mon_bottom {
            y = mon_bottom - height as i32;
        }

        state.x = x;
        state.y = y;
        state.width = width;
        state.height = height;
        return state;
    }

    state.width = width;
    state.height = height;
    state
}

fn cancel_pending_save(debounce: DebounceState) {
    if let Ok(mut guard) = debounce.lock() {
        if let Some(task) = guard.take() {
            task.abort();
        }
    }
}

fn schedule_debounced_save(window: &Window, debounce: DebounceState) {
    if let Ok(mut guard) = debounce.lock() {
        if let Some(task) = guard.take() {
            task.abort();
        }

        let preview = window.clone();
        let handle = async_runtime::spawn(async move {
            sleep(DEBOUNCE_MS).await;
            save_window_state(&preview);
        });

        *guard = Some(handle);
    }
}

fn extract_launch_file() -> Option<PathBuf> {
    std::env::args().skip(1).find_map(|arg| {
        let p = PathBuf::from(arg);
        let is_rekenraam = p
            .extension()
            .and_then(|s| s.to_str())
            .map(|s| s.eq_ignore_ascii_case("rekenraam"))
            .unwrap_or(false);
        if p.exists() && is_rekenraam {
            Some(p)
        } else {
            None
        }
    })
}

fn register_launch_file(file: &PathBuf) {
    if let Some(parent) = file.parent() {
        let _ = fs::create_dir_all(parent);
    }
    save_storage_path(file);
    create_db_placeholder(file);
}

fn initialize_storage_location() -> PathBuf {
    let default = default_storage_dir();
    let picked = FileDialog::new()
        .set_title("Select storage folder (Cancel to use default)")
        .set_directory(default.clone())
        .pick_folder();

    let chosen = picked.unwrap_or_else(|| default.clone());
    if fs::create_dir_all(&chosen).is_err() {
        let _ = fs::create_dir_all(&default);
        save_storage_path(&default);
        create_db_placeholder(&default);
        default
    } else {
        save_storage_path(&chosen);
        create_db_placeholder(&chosen);
        chosen
    }
}

fn ensure_accessible_storage(mut storage: PathBuf) -> PathBuf {
    if !db_accessible(&storage) {
        storage = resolve_accessible_db(storage.clone());
        save_storage_path(&storage);
    }
    storage
}

#[cfg_attr(mobile, tauri::mobile_entry_point)]
pub fn run() {
    let debounce_state: DebounceState = Arc::new(Mutex::new(None));
    let db_state = DbState::default();

    tauri::Builder::default()
        .manage(debounce_state.clone())
        .manage(db_state)
        .setup(|app| {
            // If app was opened with a file argument (double-clicking an associated file), use it
            if let Some(file) = extract_launch_file() {
                register_launch_file(&file);
            }

            // Ensure storage path exists or ask user on first run
            let storage = match load_storage_path() {
                Some(path) => path,
                None => initialize_storage_location(),
            };

            // Ensure the saved storage path points to an accessible sqlite DB, otherwise resolve it
            let storage = ensure_accessible_storage(storage);

            // Open DB connection, run migrations, store in app state
            let (conn, db_path) = open_and_migrate(&storage).map_err(|e| e.to_string())?;
            {
                let db_state = app.state::<DbState>();
                let mut guard = db_state.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
                guard.db_path = Some(db_path);
                guard.conn = Some(conn);
            }

            if let Some(window) = app.get_webview_window("main") {
                if let Some(state) = load_window_state() {
                    let state = fit_window_state_to_monitor(&window, state);
                    let _ = window.set_size(Size::Physical(PhysicalSize::new(state.width, state.height)));
                    let _ = window.set_position(Position::Physical(PhysicalPosition::new(state.x, state.y)));
                }

                let _ = window.show();
                let _ = window.set_focus();
            }
            Ok(())
        })
        .on_window_event(move |window, event| match event {
            WindowEvent::CloseRequested { .. } => {
                cancel_pending_save(debounce_state.clone());
                save_window_state(window);
            }
            WindowEvent::Resized(_) | WindowEvent::Moved(_) => {
                schedule_debounced_save(window, debounce_state.clone());
            }
            _ => {}
        })
        .plugin(tauri_plugin_opener::init())
        .invoke_handler(tauri::generate_handler![
            commands::validate_and_set_storage_location,
            commands::get_storage_location,
            commands::greet,
            commands::get_db_path,
            commands::db_health,
            commands::get_schema_version,
            commands::create_account,
            commands::get_account,
            commands::list_accounts,
            commands::list_categories,
            commands::list_payees,
            commands::list_account_register,
            commands::list_corporate_actions,
            commands::update_account,
            commands::delete_account,
            commands::create_transaction_with_splits,
            commands::get_transaction_with_splits,
            commands::apply_corporate_action_split_merge,
            commands::create_price_source,
            commands::list_price_sources,
            commands::update_price_source,
            commands::delete_price_source,
            commands::create_commodity_price_source,
            commands::list_commodity_price_sources,
            commands::update_commodity_price_source,
            commands::delete_commodity_price_source,
            commands::create_commodity_price,
            commands::list_commodity_prices,
            commands::update_commodity_price,
            commands::delete_commodity_price,
        ])
        .run(tauri::generate_context!())
        .expect("error while running tauri application");
}

#[cfg(test)]
mod tests {
    use std::fs;
    use std::time::{SystemTime, UNIX_EPOCH};
    use crate::db::{open_and_migrate, normalize_db_path};

    #[test]
    fn test_journal_mode_and_schema_version() {
        // Create a unique temporary folder under the OS temp dir
        let start = SystemTime::now().duration_since(UNIX_EPOCH).unwrap().as_millis();
        let mut temp = std::env::temp_dir();
        temp.push(format!("rekenraam_test_{}", start));
        let _ = fs::remove_dir_all(&temp);
        fs::create_dir_all(&temp).expect("create temp dir");

        // Open DB and run migrations
        let (conn, _db_path) = open_and_migrate(&temp).expect("open and migrate");

        // Check PRAGMA journal_mode
        let journal: String = conn
            .query_row("PRAGMA journal_mode;", [], |row| row.get(0))
            .expect("pragma journal_mode");
        assert!(journal.to_lowercase().contains("wal"), "journal_mode should be WAL");

        // Check schema version (MAX(version))
        let version_opt: Option<i32> = conn
            .query_row(
                "SELECT MAX(version) FROM schema_migrations",
                [],
                |row| row.get(0),
            )
            .expect("query max version");
        let version = version_opt.unwrap_or(0);
        assert!(version >= 1, "expected at least one migration applied");

        // Clean up
        let _ = fs::remove_file(normalize_db_path(&temp));
        let _ = fs::remove_dir_all(&temp);
    }
}
