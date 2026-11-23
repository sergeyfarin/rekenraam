-- +goose Up
-- A create's request key (#660). The browser sends the same Idempotency-Key
-- with every retry of one entry; the write that commits it stores the key in
-- its own transaction (through its audit event), so a repeat after a lost
-- response finds the key and is answered with the stored result instead of
-- writing again. The response is stored once the request finishes; a row
-- without one means the write committed but its result was never recorded.
-- Rows are request bookkeeping, not financial records: they are pruned after
-- the retention window when new keys are claimed.
CREATE TABLE request_keys (
  id INTEGER PRIMARY KEY,
  book_id INTEGER NOT NULL REFERENCES books(id) ON DELETE RESTRICT,
  request_key TEXT NOT NULL CHECK (
    instr(request_key, char(0)) = 0
    AND length(request_key) BETWEEN 1 AND 128
    AND request_key NOT GLOB '*[^!-~]*'
  ),
  route TEXT NOT NULL CHECK (instr(route, char(0)) = 0 AND length(trim(route)) > 0),
  request_hash TEXT NOT NULL CHECK (
    instr(request_hash, char(0)) = 0
    AND length(request_hash) = 64
    AND request_hash NOT GLOB '*[^0-9a-f]*'
  ),
  -- Random per request: a second audit event of the same request finds its
  -- own row by this token rather than mistaking it for an earlier request.
  claim_token TEXT NOT NULL CHECK (instr(claim_token, char(0)) = 0 AND length(claim_token) = 32),
  audit_event_id INTEGER NOT NULL REFERENCES audit_events(id) ON DELETE RESTRICT,
  -- A real UTC RFC 3339 second, in the text the retention cutoff compares.
  created_at TEXT NOT NULL CHECK (strftime('%Y-%m-%dT%H:%M:%SZ', created_at) IS created_at),
  response_status INTEGER CHECK (response_status BETWEEN 200 AND 299),
  response_body TEXT CHECK (response_body IS NULL OR json_valid(response_body)),
  CHECK ((response_status IS NULL) = (response_body IS NULL)),
  UNIQUE (book_id, request_key)
);

CREATE INDEX request_keys_by_created_at ON request_keys (book_id, created_at);

-- +goose Down
DROP INDEX request_keys_by_created_at;
DROP TABLE request_keys;
