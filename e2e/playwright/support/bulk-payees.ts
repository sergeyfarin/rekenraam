import { execFileSync } from 'node:child_process';

/** Seeds an owned test book with active payees named `Vendor 0000` onwards. */
export function seedBulkPayees(databasePath: string, count: number): void {
  execFileSync('python3', ['-c', `
import sqlite3, sys
path, count = sys.argv[1], int(sys.argv[2])
with sqlite3.connect(path, timeout=10) as db:
    db.execute('PRAGMA foreign_keys=ON')
    book_id, owner_id = db.execute('SELECT id, owner_user_id FROM books').fetchone()
    first = db.execute('SELECT COALESCE(MAX(id), 0) + 1 FROM payees').fetchone()[0]
    db.executemany("INSERT INTO payees (id, book_id, created_at, created_by_user_id) VALUES (?, ?, '2020-01-01T00:00:00Z', ?)",
                   [(first + n, book_id, owner_id) for n in range(count)])
    db.executemany("""INSERT INTO payee_versions (
        payee_id, version_seq, effective_from, recorded_at, changed_by_user_id, change_reason,
        status, name, normalized_name
      ) VALUES (?, 1, '2020-01-01', '2020-01-01T00:00:00Z', ?, 'e2e bulk seed', 'active', ?, ?)""",
                   [(first + n, owner_id, f'Vendor {n:04}', f'vendor {n:04}') for n in range(count)])
`, databasePath, String(count)]);
}
