"""
PythonBaseDemo —— Base 插件示例

Base 插件负责"产生"事件。本示例不注册任何事件监听器，
仅在被加载时通过 emit_event 发布一个事件，演示 Base 插件的用法。
"""

from cuckoo_sdk import emit_event, log_info


def main():
    log_info("PythonBaseDemo loaded")
    # Base 插件可以在任意时机发布事件
    emit_event("python.timer.tick", {"source": "PythonBaseDemo", "count": 1})


# 插件入口：内核加载模块后会执行模块顶层代码
# 如需在加载时执行初始化逻辑，可直接写在模块顶层或调用 main()
main()
