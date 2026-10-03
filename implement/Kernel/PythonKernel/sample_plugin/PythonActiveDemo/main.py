"""
PythonActiveDemo —— Active 插件示例

Active 插件负责"消费"事件。本示例通过 @on_event 装饰器
订阅 python.timer.tick 事件，并在事件触发时记录日志、
发布一个派生事件，演示 Active 插件的用法。
"""

import json

from cuckoo_sdk import on_event, emit_event, log_info, log_error


@on_event("python.timer.tick")
def on_timer_tick(payload: str) -> None:
    """处理定时器 tick 事件。

    Args:
        payload: 事件载荷（JSON 字符串），如 {"source": "PythonBaseDemo", "count": 1}
    """
    try:
        data = json.loads(payload)
        source = data.get("source")
        count = data.get("count")
        log_info(f"[PythonActiveDemo] timer tick from {source}, count={count}")

        # 发布一个派生事件
        emit_event("python.active.tick_handled", {
            "handled_source": source,
            "status": "ok"
        })
    except (json.JSONDecodeError, TypeError) as e:
        log_error(f"[PythonActiveDemo] failed to parse payload: {e}")