"""Agent runtime 的结构化 JSON 日志输出。

每行一个 JSON 对象写到 stderr，controller 侧的 collectPodLogs / streamLogsFromPod
逐行 json.Unmarshal 后写入 SQLite run_logs 表 / SSE 推送到 Dashboard。

字段约定：
    ts    ISO 8601 时间戳
    level info / warn / error / debug
    msg   人类可读消息
    其它任意 kwargs 平铺到顶层（tool=..., turn=..., error=... 等）

故意避开 Python 标准 logging 模块：
    - 那个模块输出文本格式，要解析成结构化数据需要正则
    - 这里直接 print(json.dumps(...)) 简单且无依赖
    - flush=True 确保 K8s 实时拿到日志（不被 Python stdout buffer 挡住）
"""
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
