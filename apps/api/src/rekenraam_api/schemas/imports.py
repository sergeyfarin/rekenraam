from __future__ import annotations

from datetime import date, datetime
from typing import Literal

from pydantic import BaseModel, ConfigDict, model_validator

ImportFormat = Literal["auto", "csv", "xls", "xlsx", "qif", "ofx", "qfx", "hbci", "mt940"]
ImportMode = Literal["always_create", "match_and_enrich", "deduplicate", "review", "update_only"]


class ImportLocaleOptions(BaseModel):
    model_config = ConfigDict(frozen=True)

    decimal_separator: str | None = None
    thousands_separator: str | None = None
    date_format: str | None = None
    csv_delimiter: str | None = None


class ImportDraft(BaseModel):
    model_config = ConfigDict(frozen=True)

    payee_name: str | None = None
    memo: str | None = None
    txn_date: date | None = None
    amount_minor: int | None = None
    reference: str | None = None
    import_id: str | None = None
    currency_code: str | None = None
    account_id: int | None = None
    category_id: int | None = None
    payee_id: int | None = None


class ImportPreviewRequest(BaseModel):
    model_config = ConfigDict(frozen=True)

    book_id: int = 1
    account_id: int
    format: ImportFormat = "auto"
    content: str | None = None
    content_base64: str | None = None
    file_name: str | None = None
    locale: ImportLocaleOptions | None = None

    @model_validator(mode="after")
    def validate_content(self) -> ImportPreviewRequest:
        if self.content is None and self.content_base64 is None:
            raise ValueError("content or content_base64 is required")
        return self


class ImportPreviewRow(BaseModel):
    model_config = ConfigDict(frozen=True)

    row_number: int
    raw_values: tuple[str, ...] = ()
    draft: ImportDraft | None = None
    error: str | None = None
    matched_tx_id: int | None = None
    is_duplicate: bool = False


class ImportPreviewResult(BaseModel):
    model_config = ConfigDict(frozen=True)

    format: str
    rows: tuple[ImportPreviewRow, ...]
    file_error: str | None = None


class ImportRuleCreate(BaseModel):
    model_config = ConfigDict(frozen=True)

    book_id: int = 1
    rule_kind: Literal["payee", "memo", "amount", "date", "account"]
    match_type: Literal["contains", "equals"] = "contains"
    match_text: str
    priority: int = 100
    amount_min_minor: int | None = None
    amount_max_minor: int | None = None
    date_from: date | None = None
    date_to: date | None = None
    match_account_id: int | None = None
    target_account_id: int | None = None
    target_category_id: int | None = None
    target_payee_id: int | None = None

    @model_validator(mode="after")
    def validate_ranges(self) -> ImportRuleCreate:
        if (
            self.amount_min_minor is not None
            and self.amount_max_minor is not None
            and self.amount_min_minor > self.amount_max_minor
        ):
            raise ValueError("amount_min_minor must be <= amount_max_minor")
        if (
            self.date_from is not None
            and self.date_to is not None
            and self.date_from > self.date_to
        ):
            raise ValueError("date_from must be <= date_to")
        return self


class ImportRuleSummary(ImportRuleCreate):
    id: int
    previous_import_rule_id: int | None = None
    deleted_at: datetime | None = None
    created_at: datetime


class ImportRulesApplyRequest(BaseModel):
    model_config = ConfigDict(frozen=True)

    book_id: int = 1
    drafts: tuple[ImportDraft, ...]


class ImportMatchResult(BaseModel):
    model_config = ConfigDict(frozen=True)

    draft: ImportDraft
    matched_tx_id: int | None


class ImportSessionSummary(BaseModel):
    model_config = ConfigDict(frozen=True)

    id: int
    book_id: int
    source: str | None
    file_name: str | None
    file_hash: str | None
    file_size: int | None
    status: str
    started_at: datetime
    committed_at: datetime | None
    notes: str | None


class ImportSessionTransactionSummary(BaseModel):
    model_config = ConfigDict(frozen=True)

    session_id: int
    tx_id: int
    action: str
    created_at: datetime


class ImportCommitRequest(BaseModel):
    model_config = ConfigDict(frozen=True)

    book_id: int = 1
    account_id: int
    drafts: tuple[ImportDraft, ...]
    create_payees: bool = True
    mode: ImportMode = "match_and_enrich"
    source: str | None = None
    file_name: str | None = None
    file_hash: str | None = None
    file_size: int | None = None
    notes: str | None = None


class ImportBatchResult(BaseModel):
    model_config = ConfigDict(frozen=True)

    created_tx_ids: tuple[int, ...]
    matched_tx_ids: tuple[int, ...]
    updated_tx_ids: tuple[int, ...]
    skipped: int
    review_matches: tuple[ImportMatchResult, ...] | None = None


class ImportCommitResult(BaseModel):
    model_config = ConfigDict(frozen=True)

    session: ImportSessionSummary
    batch: ImportBatchResult
