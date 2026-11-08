import { execFileSync } from 'node:child_process';

/**
 * Inserts `count` active asset accounts named `${prefix} 00000`… straight
 * into an owned book's database (#518). A catalog above the account list's
 * page size would otherwise cost thousands of API requests. Only disposable
 * e2e databases are touched; each account gets the one version the API would
 * have written.
 */
export function seedBulkAccounts(databasePath: string, prefix: string, count: number): void {
  execFileSync('python3', ['-c', `
import sqlite3, sys
path, prefix, count = sys.argv[1], sys.argv[2], int(sys.argv[3])
with sqlite3.connect(path, timeout=10) as db:
    db.execute('PRAGMA foreign_keys=ON')
    book_id, owner_id = db.execute('SELECT id, owner_user_id FROM books').fetchone()
    first = db.execute('SELECT COALESCE(MAX(id), 0) + 1 FROM accounts').fetchone()[0]
    db.executemany("INSERT INTO accounts (id, book_id, created_at, created_by_user_id) VALUES (?, ?, '2020-01-01T00:00:00Z', ?)",
                   [(first + n, book_id, owner_id) for n in range(count)])
    db.executemany("""INSERT INTO account_versions (
        account_id, version_seq, effective_from, recorded_at, changed_by_user_id, change_reason,
        status, opened_on, name, account_class, account_kind, allows_postings
      ) VALUES (?, 1, '2020-01-01', '2020-01-01T00:00:00Z', ?, 'e2e bulk seed', 'active', '2020-01-01', ?, 'asset', 'checking', 0)""",
                   [(first + n, owner_id, '%s %05d' % (prefix, n)) for n in range(count)])
`, databasePath, prefix, String(count)]);
}
