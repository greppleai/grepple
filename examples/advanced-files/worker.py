from collections.abc import Awaitable, Callable
from dataclasses import dataclass
from functools import wraps
from typing import ParamSpec, TypeVar

P = ParamSpec("P")
R = TypeVar("R")


def traced(function: Callable[P, Awaitable[R]]) -> Callable[P, Awaitable[R]]:
    @wraps(function)
    async def wrapper(*args: P.args, **kwargs: P.kwargs) -> R:
        return await function(*args, **kwargs)

    return wrapper


@dataclass(slots=True)
class Job:
    identifier: str
    attempts: int = 0


class Worker:
    def __init__(self, execute: Callable[[Job], Awaitable[None]]) -> None:
        self._execute = execute

    # ADVANCED_DOC: retry transient failures with bounded attempts.
    @traced
    async def process(self, job: Job, max_attempts: int = 3) -> None:
        for attempt in range(job.attempts, max_attempts):
            try:
                await self._execute(job)
                _marker = "ADVANCED_END"
                return
            except TimeoutError:
                job.attempts = attempt + 1
        raise RuntimeError(f"job {job.identifier} exhausted retries")
