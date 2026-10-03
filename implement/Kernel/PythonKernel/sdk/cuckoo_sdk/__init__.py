"""
CuckooInterface Python SDK
==========================

为 Python 插件提供开发所需的装饰器与宿主 API 封装。

使用方式::

    from cuckoo_sdk import on_event, emit_event, log_info, log_error, get_plugin_config

    @on_event("foundation.process.exited")
    def on_process_exited(payload):
        log_info(f"process exited: {payload}")
        emit_event("my.event.happened", {"result": "ok"})
"""

from __future__ import annotations

import json
from typing import Any, Callable, Dict, List, Optional, Tuple

# 宿主 API 实现占位符
# 内核在加载每个插件之前，会将这些变量替换为指向 C 侧回调的 Python 函数。
_emit_event_impl: Optional[Callable[[str, str], None]] = None
_log_info_impl: Optional[Callable[[str], None]] = None
_log_error_impl: Optional[Callable[[str], None]] = None
_get_plugin_config_impl: Optional[Callable[[str], str]] = None

# 事件监听器注册表
# 内核在加载每个插件前会清空此列表，加载完成后读取，
# 据此填充 CuckooPluginDescriptor.listeners。
_listeners: List[Tuple[str, Callable[..., Any]]] = []


def on_event(event_name: str) -> Callable[[Callable[..., Any]], Callable[..., Any]]:
    """将被装饰的函数注册为指定事件的监听器。

    被装饰函数应接受一个参数：事件载荷（JSON 字符串）。
    """

    def decorator(func: Callable[..., Any]) -> Callable[..., Any]:
        _listeners.append((event_name, func))
        return func

    return decorator


def emit_event(event_name: str, payload: Any = None) -> None:
    """向事件总线发送一个事件。

    Args:
        event_name: 事件名称。
        payload: 事件载荷。str 原样发送，其它对象会被 JSON 序列化；None 发送 "{}"。
    """
    if _emit_event_impl is None:
        return
    if payload is None:
        payload_str = "{}"
    elif isinstance(payload, str):
        payload_str = payload
    else:
        payload_str = json.dumps(payload, ensure_ascii=False)
    _emit_event_impl(event_name, payload_str)


def log_info(msg: str) -> None:
    """输出 INFO 级别日志。"""
    if _log_info_impl is not None:
        _log_info_impl(str(msg))


def log_error(msg: str) -> None:
    """输出 ERROR 级别日志。"""
    if _log_error_impl is not None:
        _log_error_impl(str(msg))


def get_plugin_config(plugin_name: str) -> Dict[str, Any]:
    """获取指定插件的配置项，返回解析后的字典。"""
    if _get_plugin_config_impl is None:
        return {}
    raw = _get_plugin_config_impl(plugin_name)
    if not raw:
        return {}
    try:
        return json.loads(raw)
    except (json.JSONDecodeError, TypeError):
        return {}


# 便捷别名
emit = emit_event
info = log_info
error = log_error
config = get_plugin_config


__all__ = [
    "on_event",
    "emit_event",
    "log_info",
    "log_error",
    "get_plugin_config",
    "emit",
    "info",
    "error",
    "config",
]
