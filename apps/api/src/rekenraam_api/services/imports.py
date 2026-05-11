from __future__ import annotations

import base64
import csv
import io
import re
from collections.abc import Callable, Iterable, Sequence
from datetime import date, datetime
from decimal import Decimal, InvalidOperation
from typing import Any, Literal, cast

from python_calamine import (
    load_workbook as _load_workbook,  # pyright: ignore[reportUnknownVariableType]
)

from rekenraam_api.db.models.ergonomics import AuditEvent
from rekenraam_api.db.models.imports import ImportRule, ImportSession
from rekenraam_api.repositories.imports import ImportRepository
from rekenraam_api.schemas.imports import (
    ImportBatchResult,
    ImportCommitRequest,
    ImportCommitResult,
    ImportDraft,
    ImportLocaleOptions,
    ImportMatchResult,
    ImportPreviewRequest,
    ImportPreviewResult,
    ImportPreviewRow,
    ImportRuleCreate,
    ImportRulesApplyRequest,
    ImportRuleSummary,
    ImportSessionSummary,
    ImportSessionTransactionSummary,
)
from rekenraam_api.services.access import AccessPolicy
from rekenraam_api.services.report_invalidation import bump_report_state

load_workbook = cast(Callable[[io.BytesIO], Any], _load_workbook)
ImportRuleKind = Literal["payee", "memo", "amount", "date", "account"]
ImportMatchType = Literal["contains", "equals"]


class ImportService:
    def __init__(
        self, repository: ImportRepository, access_policy: AccessPolicy | None = None
    ) -> None:
        self._repository = repository
        self._access_policy = access_policy

    async def preview(self, input: ImportPreviewRequest) -> ImportPreviewResult:
        await self._require_book_read(input.book_id)
        preview = self._parse_preview(input)
        return await self._attach_matches(input.account_id, preview)

    async def list_rules(self, book_id: int) -> list[ImportRuleSummary]:
        await self._require_book_read(book_id)
        return [self._map_rule(rule) for rule in await self._repository.list_rules(book_id)]

    async def create_rule(self, input: ImportRuleCreate) -> ImportRuleSummary:
        await self._require_book_write(input.book_id)
        audit = self._audit()
        rule = await self._repository.create_rule(**input.model_dump(), **audit)
        await bump_report_state(getattr(self._repository, "_session", None), input.book_id)
        db_session = getattr(self._repository, "_session", None)
        if db_session is not None:
            await db_session.commit()
        return self._map_rule(rule)

    async def delete_rule(self, rule_id: int) -> bool:
        rule = await self._repository.get_rule(rule_id)
        if rule is None:
            return False
        await self._require_book_write(rule.book_id)
        deleted = await self._repository.delete_rule(rule_id)
        if deleted:
            await bump_report_state(getattr(self._repository, "_session", None), rule.book_id)
            db_session = getattr(self._repository, "_session", None)
            if db_session is not None:
                await db_session.commit()
        return deleted

    async def apply_rules(self, input: ImportRulesApplyRequest) -> tuple[ImportDraft, ...]:
        await self._require_book_read(input.book_id)
        return tuple(await self._apply_rules(input.book_id, list(input.drafts)))

    async def list_sessions(self, book_id: int) -> list[ImportSessionSummary]:
        await self._require_book_read(book_id)
        return [
            self._map_session(session) for session in await self._repository.list_sessions(book_id)
        ]

    async def get_session(self, session_id: int) -> ImportSessionSummary | None:
        session = await self._repository.get_session(session_id)
        if session is None:
            return None
        await self._require_book_read(session.book_id)
        return self._map_session(session)

    async def list_session_transactions(
        self, session_id: int
    ) -> list[ImportSessionTransactionSummary] | None:
        session = await self._repository.get_session(session_id)
        if session is None:
            return None
        await self._require_book_read(session.book_id)
        rows = await self._repository.list_session_transactions(session_id)
        return [
            ImportSessionTransactionSummary(
                session_id=row.session_id,
                tx_id=row.tx_id,
                action=row.action,
                created_at=row.created_at,
            )
            for row in rows
        ]

    async def commit(self, input: ImportCommitRequest) -> ImportCommitResult:
        await self._require_book_write(input.book_id)
        audit = self._audit()
        session = await self._repository.create_session(
            book_id=input.book_id,
            source=input.source,
            file_name=input.file_name,
            file_hash=input.file_hash,
            file_size=input.file_size,
            notes=input.notes,
            **audit,
        )
        db_session = getattr(self._repository, "_session", None)
        if db_session is not None:
            await db_session.commit()
        try:
            batch = await self._commit_with_session(input, session.id, audit)
            committed = await self._repository.mark_session_committed(session.id)
            await bump_report_state(getattr(self._repository, "_session", None), input.book_id)
            session_obj = committed or session
            if db_session is not None:
                db_session.add(
                    AuditEvent(
                        book_id=input.book_id,
                        actor_user_id=audit["created_by_user_id"],
                        actor_session_id=audit["created_session_id"],
                        actor_device_id=audit["created_device_id"],
                        actor_request_id=str(audit["created_request_id"])
                        if audit["created_request_id"] is not None
                        else None,
                        event_type="import.committed",
                        target_type="import_session",
                        target_id=session.id,
                        summary=f"Committed import session {session.id}",
                        metadata_json=None,
                    )
                )
                await db_session.commit()
            return ImportCommitResult(session=self._map_session(session_obj), batch=batch)
        except Exception as error:
            if db_session is not None:
                await db_session.rollback()
            await self._repository.mark_session_abandoned(session.id, str(error))
            if db_session is not None:
                await db_session.commit()
            raise

    async def _commit_with_session(
        self, input: ImportCommitRequest, session_id: int, audit: dict[str, int | str | None]
    ) -> ImportBatchResult:
        drafts = await self._apply_rules(input.book_id, list(input.drafts))
        if input.mode == "review":
            matches = [
                ImportMatchResult(
                    draft=draft, matched_tx_id=await self._match(input.account_id, draft)
                )
                for draft in drafts
            ]
            for match in matches:
                if match.matched_tx_id is not None:
                    await self._repository.record_session_transaction(
                        session_id, match.matched_tx_id, "validated"
                    )
            return ImportBatchResult(
                created_tx_ids=(),
                matched_tx_ids=tuple(
                    matched
                    for matched in (item.matched_tx_id for item in matches)
                    if matched is not None
                ),
                updated_tx_ids=(),
                skipped=0,
                review_matches=tuple(matches),
            )

        default_account = await self._repository.get_account(input.account_id)
        if default_account is None or default_account.book_id != input.book_id:
            raise ValueError("account not found")
        default_commodity = await self._repository.get_account_commodity(input.account_id)
        if default_commodity is None:
            raise ValueError("account commodity not found")
        offset = await self._repository.get_or_create_imbalance_account(
            input.book_id, default_commodity.id
        )

        created: list[int] = []
        matched: list[int] = []
        updated: list[int] = []
        skipped = 0

        for draft in drafts:
            target_account_id = draft.account_id or input.account_id
            target_account = await self._repository.get_account(target_account_id)
            if target_account is None or target_account.book_id != input.book_id:
                raise ValueError(f"account {target_account_id} not found")
            commodity = await self._repository.get_account_commodity(target_account_id)
            if commodity is None:
                raise ValueError(f"account {target_account_id} commodity not found")
            if (
                draft.currency_code
                and commodity.symbol
                and draft.currency_code.upper() != commodity.symbol.upper()
            ):
                raise ValueError(
                    f"currency '{draft.currency_code}' does not match account currency "
                    f"'{commodity.symbol}'"
                )

            persistent_match_id = await self._match_import_id(target_account_id, draft)
            if persistent_match_id is not None:
                matched.append(persistent_match_id)
                await self._repository.record_session_transaction(
                    session_id, persistent_match_id, "validated"
                )
                continue

            match_id = await self._match(target_account_id, draft)
            if match_id is not None:
                matched.append(match_id)
                if input.mode == "always_create":
                    pass
                elif input.mode == "deduplicate":
                    await self._repository.record_session_transaction(
                        session_id, match_id, "validated"
                    )
                    continue
                elif input.mode in {"match_and_enrich", "update_only"}:
                    tx_account_ids = await self._repository.list_transaction_account_ids(match_id)
                    await self._ensure_unlocked(tx_account_ids, draft.txn_date)
                    payee_id = await self._resolve_payee_id(
                        input.book_id, draft, input.create_payees
                    )
                    replacement = await self._repository.enrich_transaction(
                        match_id,
                        payee_id=payee_id,
                        memo=draft.memo,
                        reference=draft.reference,
                        import_id=draft.import_id,
                        import_session_id=session_id,
                        audit=audit,
                    )
                    await self._repository.record_session_transaction(
                        session_id, match_id, "updated" if replacement is not None else "validated"
                    )
                    if replacement is not None:
                        updated.append(match_id)
                        recorded = await self._repository.record_import_key(
                            book_id=input.book_id,
                            account_id=target_account_id,
                            tx_id=replacement.id,
                            import_id=draft.import_id,
                        )
                        if draft.import_id and not recorded:
                            raise ValueError("duplicate import identifier for account")
                    continue
            elif input.mode == "update_only":
                skipped += 1
                continue

            if draft.txn_date is None or draft.amount_minor is None:
                skipped += 1
                continue
            await self._ensure_unlocked({target_account_id, offset.id}, draft.txn_date)
            payee_id = await self._resolve_payee_id(input.book_id, draft, input.create_payees)
            tx = await self._repository.create_import_transaction(
                book_id=input.book_id,
                account_id=target_account_id,
                offset_account_id=offset.id,
                commodity_id=commodity.id,
                txn_date=draft.txn_date,
                amount_minor=draft.amount_minor,
                payee_id=payee_id,
                memo=draft.memo,
                reference=draft.reference,
                import_id=draft.import_id,
                import_session_id=session_id,
                category_id=draft.category_id,
                audit=audit,
            )
            recorded = await self._repository.record_import_key(
                book_id=input.book_id,
                account_id=target_account_id,
                tx_id=tx.id,
                import_id=draft.import_id,
            )
            if draft.import_id and not recorded:
                raise ValueError("duplicate import identifier for account")
            created.append(tx.id)
            await self._repository.record_session_transaction(session_id, tx.id, "created")

        return ImportBatchResult(
            created_tx_ids=tuple(created),
            matched_tx_ids=tuple(matched),
            updated_tx_ids=tuple(updated),
            skipped=skipped,
            review_matches=None,
        )

    def _parse_preview(self, input: ImportPreviewRequest) -> ImportPreviewResult:
        detected = self._detect_format(input.format, input.file_name, input.content)
        try:
            if detected in {"csv"}:
                return self._preview_csv(input.content or "", input.locale, detected)
            if detected in {"qif"}:
                drafts = self._parse_qif(input.content or "", input.locale)
                return self._drafts_preview(detected, drafts)
            if detected in {"ofx", "qfx"}:
                drafts = self._parse_ofx(input.content or "", input.locale)
                return self._drafts_preview(detected, drafts)
            if detected in {"xls", "xlsx"}:
                if input.content_base64 is None:
                    return ImportPreviewResult(
                        format=detected,
                        rows=(),
                        file_error="content_base64 is required for spreadsheet import",
                    )
                return self._preview_xlsx(
                    base64.b64decode(input.content_base64), input.locale, detected
                )
        except Exception as error:
            return ImportPreviewResult(format=detected, rows=(), file_error=str(error))
        return ImportPreviewResult(format=detected, rows=(), file_error="unsupported import format")

    async def _attach_matches(
        self, default_account_id: int, preview: ImportPreviewResult
    ) -> ImportPreviewResult:
        rows: list[ImportPreviewRow] = []
        for row in preview.rows:
            if row.error or row.draft is None:
                rows.append(row)
                continue
            draft = row.draft
            account_id = draft.account_id or default_account_id
            commodity = await self._repository.get_account_commodity(account_id)
            error = None
            if commodity is None:
                error = f"account {account_id} not found"
            elif (
                draft.currency_code
                and commodity.symbol
                and draft.currency_code.upper() != commodity.symbol.upper()
            ):
                error = (
                    f"currency '{draft.currency_code}' does not match account currency "
                    f"'{commodity.symbol}'"
                )
            matched_id = None if error else await self._match(account_id, draft)
            rows.append(
                ImportPreviewRow(
                    row_number=row.row_number,
                    raw_values=row.raw_values,
                    draft=draft,
                    error=error,
                    matched_tx_id=matched_id,
                    is_duplicate=matched_id is not None,
                )
            )
        return ImportPreviewResult(
            format=preview.format, rows=tuple(rows), file_error=preview.file_error
        )

    async def _match(self, account_id: int, draft: ImportDraft) -> int | None:
        return await self._repository.find_matching_transaction(
            account_id,
            txn_date=draft.txn_date,
            amount_minor=draft.amount_minor,
            import_id=draft.import_id,
            payee_name=draft.payee_name,
            memo=draft.memo,
        )

    async def _match_import_id(self, account_id: int, draft: ImportDraft) -> int | None:
        if draft.import_id is None or not draft.import_id.strip():
            return None
        return await self._repository.find_import_key_transaction(
            account_id, draft.import_id.strip()
        )

    async def _resolve_payee_id(
        self, book_id: int, draft: ImportDraft, create_payees: bool
    ) -> int | None:
        if draft.payee_id is not None:
            return draft.payee_id
        if not draft.payee_name:
            return None
        existing = await self._repository.find_payee_by_name(book_id, draft.payee_name)
        if existing is not None:
            return existing.id
        if not create_payees:
            return None
        return (await self._repository.create_payee(book_id, draft.payee_name)).id

    async def _ensure_unlocked(self, account_ids: set[int], txn_date: date | None) -> None:
        if txn_date is None:
            return
        locked = await self._repository.locked_account_dates(account_ids, txn_date)
        if locked:
            details = ", ".join(
                f"{account_id} (balanced through {locked_date})"
                for account_id, locked_date in locked
            )
            raise ValueError(
                f"cannot post transaction dated {txn_date} to locked accounts: {details}"
            )

    async def _apply_rules(self, book_id: int, drafts: list[ImportDraft]) -> list[ImportDraft]:
        rules = await self._repository.list_rules(book_id)
        results: list[ImportDraft] = []
        for draft in drafts:
            payload = draft.model_dump()
            for rule in rules:
                matched = False
                if rule.rule_kind in {"payee", "memo"}:
                    haystack = (draft.payee_name if rule.rule_kind == "payee" else draft.memo) or ""
                    matched = (
                        haystack.lower() == rule.match_text.lower()
                        if rule.match_type == "equals"
                        else rule.match_text.lower() in haystack.lower()
                    )
                elif rule.rule_kind == "amount" and draft.amount_minor is not None:
                    matched = (
                        rule.amount_min_minor is None or draft.amount_minor >= rule.amount_min_minor
                    ) and (
                        rule.amount_max_minor is None or draft.amount_minor <= rule.amount_max_minor
                    )
                elif rule.rule_kind == "date" and draft.txn_date is not None:
                    matched = (rule.date_from is None or draft.txn_date >= rule.date_from) and (
                        rule.date_to is None or draft.txn_date <= rule.date_to
                    )
                elif rule.rule_kind == "account" and draft.account_id is not None:
                    matched = rule.match_account_id == draft.account_id
                if matched:
                    if rule.target_account_id is not None:
                        payload["account_id"] = rule.target_account_id
                    if rule.target_category_id is not None:
                        payload["category_id"] = rule.target_category_id
                    if rule.target_payee_id is not None:
                        payload["payee_id"] = rule.target_payee_id
            results.append(ImportDraft(**payload))
        return results

    def _detect_format(self, requested: str, file_name: str | None, content: str | None) -> str:
        if requested != "auto":
            return requested
        if file_name and "." in file_name:
            ext = file_name.rsplit(".", 1)[-1].lower()
            if ext in {"csv", "qif", "ofx", "qfx", "xls", "xlsx"}:
                return ext
        text = content or ""
        if "<OFX" in text.upper():
            return "ofx"
        if "!Type:" in text:
            return "qif"
        if self._looks_like_csv(text):
            return "csv"
        raise ValueError("unable to detect import format")

    def _looks_like_csv(self, content: str) -> bool:
        first = next((line for line in content.splitlines() if line.strip()), "")
        lower = first.lower()
        return ("," in first or ";" in first) and "date" in lower and "amount" in lower

    def _preview_csv(
        self, content: str, locale: ImportLocaleOptions | None, fmt: str
    ) -> ImportPreviewResult:
        delimiter = (locale.csv_delimiter if locale and locale.csv_delimiter else ",")[:1]
        reader = csv.reader(io.StringIO(content), delimiter=delimiter)
        rows = list(reader)
        if not rows:
            return ImportPreviewResult(format=fmt, rows=(), file_error="CSV header row is required")
        headers = [header.strip().lower() for header in rows[0]]
        preview_rows: list[ImportPreviewRow] = []
        for index, raw in enumerate(rows[1:], start=2):
            try:
                draft = self._parse_tabular_row(headers, raw, locale)
                if draft is not None:
                    preview_rows.append(
                        ImportPreviewRow(row_number=index, raw_values=tuple(raw), draft=draft)
                    )
            except ValueError as error:
                preview_rows.append(
                    ImportPreviewRow(row_number=index, raw_values=tuple(raw), error=str(error))
                )
        return ImportPreviewResult(format=fmt, rows=tuple(preview_rows))

    def _preview_xlsx(
        self, content: bytes, locale: ImportLocaleOptions | None, fmt: str
    ) -> ImportPreviewResult:
        rows = self._read_xlsx_rows(content)
        if not rows:
            return ImportPreviewResult(
                format=fmt, rows=(), file_error="spreadsheet header row is required"
            )
        headers = [str(header).strip().lower() for header in rows[0]]
        preview_rows: list[ImportPreviewRow] = []
        for index, raw in enumerate(rows[1:], start=2):
            try:
                draft = self._parse_tabular_row(headers, [str(value) for value in raw], locale)
                if draft is not None:
                    preview_rows.append(
                        ImportPreviewRow(
                            row_number=index,
                            raw_values=tuple(str(value) for value in raw),
                            draft=draft,
                        )
                    )
            except ValueError as error:
                preview_rows.append(
                    ImportPreviewRow(
                        row_number=index,
                        raw_values=tuple(str(value) for value in raw),
                        error=str(error),
                    )
                )
        return ImportPreviewResult(format=fmt, rows=tuple(preview_rows))

    def _parse_tabular_row(
        self, headers: list[str], row: list[str], locale: ImportLocaleOptions | None
    ) -> ImportDraft | None:
        index = {header: pos for pos, header in enumerate(headers)}

        def pick(*names: str) -> str | None:
            for name in names:
                pos = index.get(name)
                if pos is not None and pos < len(row):
                    value = row[pos].strip()
                    if value:
                        return value
            return None

        raw_date = pick("date", "txn_date", "transaction_date")
        raw_amount = pick("amount", "amount_minor")
        txn_date = self._parse_date(raw_date, locale) if raw_date else None
        amount_minor = self._parse_amount(raw_amount, locale) if raw_amount else None
        if txn_date is None and amount_minor is None:
            return None
        return ImportDraft(
            payee_name=pick("payee", "payee_name"),
            memo=pick("memo", "notes"),
            txn_date=txn_date,
            amount_minor=amount_minor,
            reference=pick("reference", "ref"),
            import_id=pick("import_id", "fitid"),
            currency_code=pick("currency", "currency_code"),
        )

    def _parse_qif(self, content: str, locale: ImportLocaleOptions | None) -> list[ImportDraft]:
        drafts: list[ImportDraft] = []
        current: dict[str, str | date | int | None] = {}
        for line in content.splitlines():
            line = line.rstrip()
            if line == "^":
                if current:
                    drafts.append(ImportDraft.model_validate(current))
                current = {}
            elif line.startswith("D"):
                current["txn_date"] = self._parse_date(line[1:], locale)
            elif line.startswith("T"):
                current["amount_minor"] = self._parse_amount(line[1:], locale)
            elif line.startswith("P"):
                current["payee_name"] = line[1:].strip()
            elif line.startswith("M"):
                current["memo"] = line[1:].strip()
            elif line.startswith("N"):
                current["reference"] = line[1:].strip()
        if current:
            drafts.append(ImportDraft.model_validate(current))
        return drafts

    def _parse_ofx(self, content: str, locale: ImportLocaleOptions | None) -> list[ImportDraft]:
        currency = self._extract_ofx_tag(content, "CURDEF")
        blocks = re.findall(
            r"<STMTTRN>(.*?)(?:</STMTTRN>|(?=<STMTTRN>)|$)",
            content,
            flags=re.IGNORECASE | re.DOTALL,
        )
        drafts: list[ImportDraft] = []
        for block in blocks:
            raw_date = self._extract_ofx_tag(block, "DTPOSTED")
            raw_amount = self._extract_ofx_tag(block, "TRNAMT")
            drafts.append(
                ImportDraft(
                    payee_name=self._extract_ofx_tag(block, "NAME"),
                    memo=self._extract_ofx_tag(block, "MEMO"),
                    txn_date=self._parse_ofx_date(raw_date) if raw_date else None,
                    amount_minor=self._parse_amount(raw_amount, locale) if raw_amount else None,
                    import_id=self._extract_ofx_tag(block, "FITID"),
                    currency_code=currency,
                )
            )
        return drafts

    def _extract_ofx_tag(self, content: str, tag: str) -> str | None:
        match = re.search(rf"<{tag}>([^\r\n<]+)", content, flags=re.IGNORECASE)
        return match.group(1).strip() if match else None

    def _drafts_preview(self, fmt: str, drafts: list[ImportDraft]) -> ImportPreviewResult:
        return ImportPreviewResult(
            format=fmt,
            rows=tuple(
                ImportPreviewRow(row_number=index, draft=draft)
                for index, draft in enumerate(drafts, start=1)
            ),
        )

    def _parse_amount(self, raw: str, locale: ImportLocaleOptions | None) -> int:
        text = raw.strip().replace(" ", "")
        negative = (text.startswith("(") and text.endswith(")")) or text.startswith("-")
        text = text.strip("()+-")
        if locale and locale.thousands_separator:
            text = text.replace(locale.thousands_separator, "")
        decimal_sep = locale.decimal_separator if locale and locale.decimal_separator else None
        if decimal_sep:
            text = text.replace(decimal_sep, ".")
        elif "," in text and "." not in text:
            text = text.replace(",", ".")
        else:
            text = text.replace(",", "")
        try:
            minor = int((Decimal(text) * Decimal("100")).quantize(Decimal("1")))
        except (InvalidOperation, ValueError) as error:
            raise ValueError(f"invalid amount: {raw}") from error
        return -abs(minor) if negative else minor

    def _parse_date(self, raw: str | None, locale: ImportLocaleOptions | None) -> date | None:
        if raw is None:
            return None
        text = raw.strip().replace("\\", "/")
        formats: list[str] = []
        if locale and locale.date_format:
            formats.append(self._python_date_format(locale.date_format))
        formats.extend(["%Y-%m-%d", "%Y/%m/%d", "%m/%d/%Y", "%m/%d/%y", "%d/%m/%Y", "%d/%m/%y"])
        for fmt in formats:
            try:
                return datetime.strptime(text, fmt).date()
            except ValueError:
                continue
        return None

    def _parse_ofx_date(self, raw: str) -> date | None:
        digits = "".join(char for char in raw if char.isdigit())
        if len(digits) < 8:
            return None
        try:
            return date(int(digits[0:4]), int(digits[4:6]), int(digits[6:8]))
        except ValueError:
            return None

    def _python_date_format(self, fmt: str) -> str:
        return (
            fmt.upper()
            .replace("YYYY", "%Y")
            .replace("YY", "%y")
            .replace("MM", "%m")
            .replace("DD", "%d")
        )

    def _read_xlsx_rows(self, content: bytes) -> list[list[str]]:
        workbook = load_workbook(io.BytesIO(content))
        if not workbook.sheet_names:
            return []
        sheet = workbook.get_sheet_by_index(0)
        rows: list[list[str]] = []
        for row in cast(Iterable[Sequence[object | None]], sheet.iter_rows()):
            values = ["" if value is None else str(value) for value in row]
            if any(value.strip() for value in values):
                rows.append(values)
        workbook.close()
        return rows

    def _map_rule(self, rule: ImportRule) -> ImportRuleSummary:
        return ImportRuleSummary(
            id=rule.id,
            previous_import_rule_id=rule.previous_import_rule_id,
            book_id=rule.book_id,
            rule_kind=cast(ImportRuleKind, rule.rule_kind),
            match_type=cast(ImportMatchType, rule.match_type),
            match_text=rule.match_text,
            priority=rule.priority,
            amount_min_minor=rule.amount_min_minor,
            amount_max_minor=rule.amount_max_minor,
            date_from=rule.date_from,
            date_to=rule.date_to,
            match_account_id=rule.match_account_id,
            target_account_id=rule.target_account_id,
            target_category_id=rule.target_category_id,
            target_payee_id=rule.target_payee_id,
            deleted_at=rule.deleted_at,
            created_at=rule.created_at,
        )

    def _map_session(self, session: ImportSession) -> ImportSessionSummary:
        return ImportSessionSummary(
            id=session.id,
            book_id=session.book_id,
            source=session.source,
            file_name=session.file_name,
            file_hash=session.file_hash,
            file_size=session.file_size,
            status=session.status,
            started_at=session.started_at,
            committed_at=session.committed_at,
            notes=session.notes,
        )

    async def _require_book_read(self, book_id: int) -> None:
        if self._access_policy is not None:
            await self._access_policy.require_book_read(book_id)

    async def _require_book_write(self, book_id: int) -> None:
        if self._access_policy is not None:
            await self._access_policy.require_book_write(book_id)

    def _audit(self) -> dict[str, int | str | None]:
        audit = self._access_policy.audit_stamp() if self._access_policy is not None else None
        return {
            "created_by_user_id": audit.created_by_user_id if audit is not None else None,
            "created_session_id": audit.created_session_id if audit is not None else None,
            "created_device_id": audit.created_device_id if audit is not None else None,
            "created_request_id": audit.created_request_id if audit is not None else None,
        }
