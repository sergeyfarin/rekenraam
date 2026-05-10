from __future__ import annotations

import asyncio
from datetime import UTC, datetime

from rekenraam_api.config.settings import get_settings
from rekenraam_api.db.session import session_factory
from rekenraam_api.repositories.pricing import PricingRepository
from rekenraam_api.schemas.pricing import PricingExecutionStatusSummary, PricingRefreshRunSummary
from rekenraam_api.services.pricing_execution import PricingExecutionService


class PricingRefreshWorker:
    def __init__(self) -> None:
        settings = get_settings()
        self._enabled = settings.pricing_scheduler_enabled
        self._poll_seconds = settings.pricing_scheduler_poll_seconds
        self._task: asyncio.Task[None] | None = None
        self._stop_event = asyncio.Event()
        self._state_lock = asyncio.Lock()
        self._active_book_ids: set[int] = set()
        self._started_at: datetime | None = None

    async def start(self) -> None:
        if not self._enabled or self._task is not None:
            return
        self._started_at = datetime.now(UTC)
        self._stop_event = asyncio.Event()
        self._task = asyncio.create_task(self._scheduler_loop())

    async def stop(self) -> None:
        self._stop_event.set()
        if self._task is not None:
            self._task.cancel()
            try:
                await self._task
            except asyncio.CancelledError:
                pass
            self._task = None

    async def run_book(
        self, book_id: int, *, trigger: str, force: bool
    ) -> PricingRefreshRunSummary | None:
        async with self._state_lock:
            if book_id in self._active_book_ids:
                return None
            self._active_book_ids.add(book_id)
        try:
            async with session_factory() as session:
                service = PricingExecutionService(PricingRepository(session))
                summary = await service.run_book_refresh(book_id, trigger=trigger, force=force)
            return summary
        finally:
            async with self._state_lock:
                self._active_book_ids.discard(book_id)

    async def get_status(self, book_id: int) -> PricingExecutionStatusSummary:
        async with self._state_lock:
            active_book_ids = sorted(self._active_book_ids)
            worker_started_at = self._started_at
        async with session_factory() as session:
            service = PricingExecutionService(PricingRepository(session))
            return await service.get_execution_status(
                book_id=book_id,
                scheduler_enabled=self._enabled,
                scheduler_poll_seconds=self._poll_seconds,
                worker_started_at=worker_started_at,
                active_book_ids=active_book_ids,
                last_run=None,
            )

    async def get_run_history(
        self, book_id: int, limit: int = 10
    ) -> list[PricingRefreshRunSummary]:
        async with session_factory() as session:
            service = PricingExecutionService(PricingRepository(session))
            return await service.list_refresh_run_history(book_id, limit)

    async def _scheduler_loop(self) -> None:
        while not self._stop_event.is_set():
            try:
                async with session_factory() as session:
                    service = PricingExecutionService(PricingRepository(session))
                    book_ids = await service.list_enabled_book_ids()
                for book_id in book_ids:
                    await self.run_book(book_id, trigger="scheduled", force=False)
            except asyncio.CancelledError:
                raise
            except Exception:
                pass

            try:
                await asyncio.wait_for(self._stop_event.wait(), timeout=self._poll_seconds)
            except TimeoutError:
                continue


pricing_refresh_worker = PricingRefreshWorker()
