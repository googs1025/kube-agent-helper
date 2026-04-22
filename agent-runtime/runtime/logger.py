"""Structured JSON logger for agent runtime."""
import json
import sys
import time


def _emit(level: str, msg: str, **kwargs) -> None:
    entry = {"ts": time.strftime("%Y-%m-%dT%H:%M:%S%z"), "level": level, "msg": msg}
    entry.update(kwargs)
    print(json.dumps(entry, default=str, ensure_ascii=False), file=sys.stderr, flush=True)


def info(msg: str, **kwargs) -> None:
    _emit("info", msg, **kwargs)


def warn(msg: str, **kwargs) -> None:
    _emit("warn", msg, **kwargs)


def error(msg: str, **kwargs) -> None:
    _emit("error", msg, **kwargs)


def debug(msg: str, **kwargs) -> None:
    _emit("debug", msg, **kwargs)
