# Receipts & Attachments Plan (proposed R14)

Status: **proposed** (2026-07-19). Feature plan for receipt capture,
automatic recognition, and attachment storage. Resolves the open product
decision "attachment storage, retention, access-control, backup, and
encryption model" (`docs/product-requirements.md`), which currently lists
attachments as deliberately out of scope. Sequencing is decided in
`docs/roadmap.md`; this plan proposes the slot.

## Goal

The user photographs or uploads a receipt; Rekenraam stores the file
durably in a human-browsable folder, recognizes date / amount / currency /
merchant, and either attaches it to the matching existing transaction or
creates a draft transaction for review. Receipts without a match wait in an
inbox.

## Constraints (non-negotiable, from existing decisions)

- **Single binary stays single.** No cgo OCR dependency (Tesseract via
  gosseract would break "download one file, run it"). Recognition must be
  browser-side or user-provided.
- **Privacy-first by default.** No receipt image leaves the user's machines
  unless the user explicitly configures an external endpoint (BYO, same
  spirit as provider keys).
- **Financial documents are financial records.** Storage gets the same
  care as the ledger: dedupe, integrity hashes, inclusion in the backup
  story, no silent deletion.
- **Drafts are producer-owned.** Receipt-generated transactions use the
  reserved system-only `draft` status — this is the second planned producer
  for that workflow (R9 recurring is the first). A receipt never posts a
  transaction without review.

## Design

### Storage (slice R14a — the foundation, also useful alone)

- Files live under `ATTACHMENTS_DIR` (default `<data dir>/attachments/`),
  organized human-browsable as `YYYY/MM/<yyyy-mm-dd>-<slug>-<id>.<ext>`
  (slug from merchant/description once known; renamed on recognition). The
  folder is the user's — browsable, rsync-able, theirs.
- DB carries the index and integrity, not the bytes:
  - `attachments`: id, sha256, byte_size, mime_type, original_filename,
    stored_path, source (`upload` | `camera` | future `email`), created_at,
    created_by_user_id, audit linkage.
  - `attachment_links`: attachment_id → transaction_id (nullable —
    NULL = inbox). One receipt may attach to one transaction v1;
    many-to-many stays open.
- Content-hash (sha256) dedupe: re-uploading the same receipt links the
  existing file. Deletion is soft (unlink + trash semantics) consistent
  with ledger recovery philosophy; hard delete only from the inbox.
- Limits: configurable max file size (default ~10 MB), allowlisted types
  (JPEG/PNG/WebP/HEIC?/PDF). HEIC likely needs client-side conversion —
  decide during R14a.
- **Backup story extension (required, same slice):** documented procedure
  becomes SQLite `VACUUM INTO` **plus** attachments-dir copy; the
  trial-balance/self-check proposal gains an attachments integrity pass
  (every DB row's file exists and hashes match). README backup section
  updated.
- Access control: existing session auth; files served only through an
  authenticated endpoint, never as static paths.
- Encryption at rest: deferred with the same reasoning as SQLite encryption
  (documented, revisit before higher-risk deployment recommendations) —
  keep the two decisions aligned.

### Capture & inbox (slice R14b)

- Upload on the transaction editor (attach directly) and a **Receipts
  inbox** page: drag-drop / file picker / mobile camera capture
  (`<input capture>` — the existing responsive-web mobile story, no native
  app).
- Inbox shows thumbnail, recognized fields once available, and actions:
  attach-to-transaction (search assisted), create-draft, dismiss.
- Transaction read models gain attachment counts; the register/table shows
  a paperclip affordance.

### Recognition (slice R14c)

Two engines, one interface, both optional:

1. **Default: in-browser OCR** (tesseract.js WASM at capture/review time).
   Zero server dependencies, zero privacy exposure, works offline on LAN.
   Quality is adequate for printed register receipts; the user corrects
   fields in the inbox UI (corrections are cheap; totals get first-class
   treatment).
2. **Optional: BYO vision endpoint** — user-configured OpenAI-compatible
   endpoint (which may be a local Ollama/llava instance — still private) for
   markedly better extraction on crumpled/handwritten/foreign receipts.
   Same BYO-key rules as providers: off by default, explicit opt-in,
   endpoint + key sealed in secretbox.

Extraction targets, in priority order: **total amount + currency** (exact
decimal parse — the no-float rule applies to OCR output too), **date**,
**merchant**, optional line items/VAT later. Every extraction carries a
confidence and is user-editable before anything touches the ledger.

### Matching & drafts (slice R14c, continued)

- Matcher: candidate posted transactions within ±3 days and amount within
  rounding tolerance in the same currency (exact comparison at aligned
  scales — reuse ledger arithmetic, no floats). One candidate → suggest
  attach; several → pick list; none → offer **create draft** with payee =
  merchant (payee matching against existing payees), amount, date, and the
  user's default account prefilled.
- Draft creation goes through the existing producer-owned draft semantics:
  system-created, excluded from ledger, promoted via the normal post flow
  (which, per the 2026-07-13 fixes, runs the full reconciliation guard).
- Import-rules synergy: once rules exist (R5 proposal), merchant → category
  rules apply to receipt drafts too — one rules engine, three producers
  (file import, bank feeds, receipts).

## Roadmap placement (proposal)

- **R14a (storage + manual attach)** is independent and small — it can slot
  any time after R3, and pairs naturally with R3's backup work (both touch
  the "your data is safe" story). Doing R14a early also de-risks the open
  product decision cheaply.
- **R14b (capture + inbox)** after R5 — it reuses the review-queue UX
  muscle the import preview builds.
- **R14c (recognition + match/draft)** after or alongside R9, because
  R9 builds the draft-producer plumbing both features share; whichever
  ships first builds it.
- Must not displace R2/R3/R5 (the trust loop and announcement gates come
  first).

## Non-goals (v1)

- Email-in ingestion, bulk document management, full-text search over
  receipts, warranty/return tracking, multi-page statement parsing (that's
  the import pipeline's job), server-side OCR binaries, any cloud OCR
  default.

## Open questions to resolve at slice start

- HEIC handling (convert client-side vs reject with guidance).
- Whether attachments join the trash/restore UI or get their own.
- Retention policy surface (none v1 — keep forever — but decide the
  deletion audit shape).
- Whether R14a should also cover *statement* PDFs attached to
  reconciliation sessions (cheap adjacency, same table).
