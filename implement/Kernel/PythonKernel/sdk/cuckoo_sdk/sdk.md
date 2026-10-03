# CuckooInterface Python SDK

Python 插件开发工具包，为 Python 运行时插件提供事件订阅、事件发布、日志输出和配置读取等能力。

## 目录

- [安装 SDK](#安装-sdk)
- [快速开始](#快速开始)
- [插件类型](#插件类型)
- [META-INF.json 规范](#meta-infjjson-规范)
- [API 参考](#api-参考)
- [完整示例](#完整示例)
- [注意事项](#注意事项)

---

## 安装 SDK

在本地开发 Python 插件前，需要将 `cuckoo_sdk` 安装到 Python 环境中，或拷贝到 CuckooInterface 的安装目录下。这样编辑器才能正确识别和补全 SDK API，插件也能在本地独立运行测试。

> **Python 版本兼容性**：`cuckoo_sdk` 是纯 Python 代码，不依赖特定 CPython 版本，本地开发可使用任意 Python 3.x。但 CuckooInterface 运行时由 **PythonKernel 内嵌的 CPython** 执行插件，该内嵌版本取决于构建 PythonKernel 时所用的 Python 版本（例如构建时用 Python 3.11，则运行时为 3.11）。若本地开发版本与内嵌版本不一致，标准库和第三方包的可用性可能存在差异，建议尽量保持一致。

### 方式一：使用安装脚本（推荐）

SDK 目录下提供了 `install.bat`，一键将 `cuckoo_sdk` 安装到 Python 的 `site-packages`：

```bat
:: 安装到当前用户的 site-packages（无需管理员权限，默认）
install.bat

:: 安装到系统级 site-packages（可能需要管理员权限）
install.bat system
```

安装完成后，可在任意位置通过 `import cuckoo_sdk` 使用：

```python
from cuckoo_sdk import on_event, emit_event, log_info
```

### 方式二：手动拷贝

将 `cuckoo_sdk` 目录拷贝到以下任一位置：

| 目标位置 | 说明 |
|----------|------|
| Python 的 `site-packages` 目录 | 全局可用，`python -c "import site; print(site.getsitepackages())"` 查看路径 |
| CuckooInterface 安装目录的 `plugins/PythonKernel/sdk/` 下 | 随内核分发，运行时由内核自动加入 `sys.path` |
| 插件自身目录内 | 仅当前插件可用，需确保 `cuckoo_sdk` 与 `main.py` 同级 |

> **注意**：CuckooInterface 运行时，PythonKernel 会自动将内核自带的 SDK 加入 `sys.path`，因此插件在 CuckooInterface 中运行无需额外安装。安装 SDK 仅用于本地开发和独立测试。

### 卸载

删除 `site-packages` 下的 `cuckoo_sdk` 目录即可。由于不同用户的 Python 版本可能不同（3.10、3.11、3.12 等），请使用以下命令动态获取路径后删除：

```bat
:: 用户级安装（默认）
for /f "delims=" %i in ('python -c "import site; print(site.getusersitepackages())"') do rmdir /s /q "%i\cuckoo_sdk"

:: 系统级安装
for /f "delims=" %i in ('python -c "import site; print(site.getsitepackages()[0])"') do rmdir /s /q "%i\cuckoo_sdk"
```

> 在 `.bat` 脚本中使用时，需将 `%i` 改为 `%%i`。

---

## 快速开始

### 1. 创建插件目录结构

```
MyPlugin/
├── META-INF.json      # 插件元数据（必需）
└── main.py            # 插件入口（必需，文件名任意但需在 META-INF 中声明）
```

### 2. 编写 META-INF.json

```json
{
  "name": "MyPlugin",
  "id": "com.example.my_plugin",
  "type": 30,
  "version": "1.0.0",
  "description": "我的第一个 Python 插件",
  "runtime_type": "python",
  "tag": ["demo"],
  "listeners": ["some.event.name"]
}
```

### 3. 编写 main.py

```python
from cuckoo_sdk import on_event, log_info

@on_event("some.event.name")
def handle_event(payload: str) -> None:
    log_info(f"received event: {payload}")
```

### 4. 打包部署

将插件目录压缩为 `MyPlugin.zip`，放入 CuckooInterface 的 `user/Active/`（或 `user/Base/`）目录，重启程序即可自动加载。

---

## 插件类型

CuckooInterface 中插件分为三类，由 `META-INF.json` 的 `type` 字段决定：

| type 值 | 名称   | 职责                     | 关键能力                     |
|---------|--------|--------------------------|------------------------------|
| `10`    | Kernel | 提供运行时内核           | 实现 `CuckooKernelInterface` |
| `42`    | Base   | 提供（产生）事件         | `emit_event` 发布事件        |
| `30`    | Active | 消费（订阅）事件         | `@on_event` 订阅事件         |

> Python SDK 用于开发 **Base** 和 **Active** 插件。Kernel 由 C++ 实现（即 PythonKernel 本身）。

### 事件流

```
Base 插件 --emit_event()--> 事件总线 --分发--> Active 插件(@on_event)
```

- **Base 插件**：在 `events` 字段声明其能产生的事件，运行时通过 `emit_event()` 发布。
- **Active 插件**：在 `listeners` 字段声明其订阅的事件，通过 `@on_event` 装饰器注册回调。

> 只有在 `META-INF.json` 中声明的事件才能被发布或订阅。Active 插件订阅的事件必须存在某个 Base 插件提供。

---

## META-INF.json 规范

| 字段           | 类型     | 必填 | 说明                                                         |
|----------------|----------|------|--------------------------------------------------------------|
| `name`         | string   | 是   | 插件显示名称                                                 |
| `id`           | string   | 是   | 插件唯一标识，建议反向域名格式（如 `com.example.xxx`）       |
| `type`         | int      | 是   | 插件类型：`42`=Base, `30`=Active                             |
| `version`      | string   | 是   | 语义化版本号                                                 |
| `description`  | string   | 否   | 插件描述                                                     |
| `runtime_type` | string   | 是   | 运行时类型，Python 插件固定为 `"python"`                     |
| `tag`          | string[] | 否   | 标签数组，用于分类检索                                       |
| `events`       | string[] | Base | Base 插件提供的事件名称列表                                  |
| `listeners`    | string[] | Active | Active 插件订阅的事件名称列表                              |

### Base 插件示例

```json
{
  "name": "MyBasePlugin",
  "id": "com.example.my_base",
  "type": 42,
  "version": "1.0.0",
  "description": "提供定时事件",
  "runtime_type": "python",
  "events": ["my.timer.tick"]
}
```

### Active 插件示例

```json
{
  "name": "MyActivePlugin",
  "id": "com.example.my_active",
  "type": 30,
  "version": "1.0.0",
  "description": "处理定时事件",
  "runtime_type": "python",
  "listeners": ["my.timer.tick"]
}
```

---

## API 参考

### `on_event(event_name)`

事件监听装饰器。将被装饰的函数注册为指定事件的回调。

```python
from cuckoo_sdk import on_event

@on_event("my.event.name")
def my_handler(payload: str) -> None:
    ...
```

**参数**

| 参数         | 类型   | 说明           |
|--------------|--------|----------------|
| `event_name` | string | 要订阅的事件名 |

**回调签名**

```python
def handler(payload: str) -> None:
```

| 参数      | 类型   | 说明                                   |
|-----------|--------|----------------------------------------|
| `payload` | string | 事件载荷，JSON 字符串，需自行 `json.loads` 解析 |

> 事件名必须在 `META-INF.json` 的 `listeners` 中声明，否则内核加载时会报错。

---

### `emit_event(event_name, payload=None)`

向事件总线发布一个事件。

```python
from cuckoo_sdk import emit_event

emit_event("my.event.name", {"key": "value"})
```

**参数**

| 参数         | 类型          | 默认值 | 说明                                                         |
|--------------|---------------|--------|--------------------------------------------------------------|
| `event_name` | string        | —      | 事件名称                                                     |
| `payload`    | str \| dict \| None | `None` | 事件载荷。`str` 原样发送；其它对象自动 `json.dumps` 序列化；`None` 发送 `"{}"` |

> 事件名必须在 `META-INF.json` 的 `events` 中声明（Base 插件），否则事件不会被分发。

**别名**：`emit`

---

### `log_info(msg)`

输出 INFO 级别日志，日志会写入 CuckooInterface 的日志系统。

```python
from cuckoo_sdk import log_info

log_info("plugin started")
```

**参数**

| 参数  | 类型   | 说明     |
|-------|--------|----------|
| `msg` | string | 日志内容 |

**别名**：`info`

---

### `log_error(msg)`

输出 ERROR 级别日志。

```python
from cuckoo_sdk import log_error

try:
    ...
except Exception as e:
    log_error(f"something went wrong: {e}")
```

**参数**

| 参数  | 类型   | 说明     |
|-------|--------|----------|
| `msg` | string | 日志内容 |

**别名**：`error`

---

### `get_plugin_config(plugin_name)`

获取指定插件的配置项，返回解析后的字典。

```python
from cuckoo_sdk import get_plugin_config

config = get_plugin_config("com.example.my_base")
value = config.get("some_key")
```

**参数**

| 参数          | 类型   | 说明                 |
|---------------|--------|----------------------|
| `plugin_name` | string | 插件 ID（如 `com.example.xxx`） |

**返回值**：`dict` — 解析后的配置字典；若插件不存在或无配置，返回空字典 `{}`。

**别名**：`config`

---

## 完整示例

### Base 插件（产生事件）

**META-INF.json**

```json
{
  "name": "TimerBase",
  "id": "com.example.timer_base",
  "type": 42,
  "version": "1.0.0",
  "description": "每秒发布一个 tick 事件",
  "runtime_type": "python",
  "events": ["timer.tick"]
}
```

**main.py**

```python
import time
import threading

from cuckoo_sdk import emit_event, log_info


def tick_loop():
    count = 0
    while True:
        emit_event("timer.tick", {"count": count})
        count += 1
        time.sleep(1)


# 插件加载时启动后台线程发布事件
threading.Thread(target=tick_loop, daemon=True).start()
log_info("TimerBase started")
```

### Active 插件（消费事件）

**META-INF.json**

```json
{
  "name": "TickCounter",
  "id": "com.example.tick_counter",
  "type": 30,
  "version": "1.0.0",
  "description": "统计 tick 事件次数",
  "runtime_type": "python",
  "listeners": ["timer.tick"]
}
```

**main.py**

```python
import json

from cuckoo_sdk import on_event, log_info, log_error

counter = 0


@on_event("timer.tick")
def on_tick(payload: str) -> None:
    global counter
    try:
        data = json.loads(payload)
        counter += 1
        log_info(f"received tick #{data['count']} (total={counter})")
    except (json.JSONDecodeError, KeyError) as e:
        log_error(f"failed to parse tick payload: {e}")
```

---

## 注意事项

### 1. 载荷格式

事件载荷在传输过程中始终是 **JSON 字符串**。发布时 `emit_event` 会自动序列化 `dict`/`list` 等对象；接收时需手动 `json.loads(payload)` 解析。

```python
# 发布端
emit_event("my.event", {"a": 1, "b": [2, 3]})

# 接收端
@on_event("my.event")
def handler(payload: str):
    data = json.loads(payload)  # {"a": 1, "b": [2, 3]}
```

### 2. 事件声明一致性

- Base 插件：`emit_event` 发布的事件必须出现在 `META-INF.json` 的 `events` 字段中。
- Active 插件：`@on_event` 订阅的事件必须出现在 `META-INF.json` 的 `listeners` 字段中。

未声明的事件会被内核拒绝加载。

### 3. 模块顶层执行

内核加载插件时会执行 `main.py`（模块顶层代码）。`@on_event` 装饰器在模块导入时即完成监听器注册，因此：

- 装饰器必须写在模块顶层（不能写在函数内部）。
- 如需在加载时执行初始化逻辑，可直接写在模块顶层。

### 4. 线程安全

事件回调由内核的工作线程调用，**非主线程**。若插件涉及共享状态，需自行加锁。

### 5. 宿主 API 可用性

`emit_event`、`log_info` 等函数在 SDK 内部通过宿主注入的实现工作。在插件被内核加载前，这些函数调用是空操作（no-op），不会报错。因此可以安全地在模块顶层调用。

### 6. 编码

- 源文件使用 **UTF-8（无 BOM）** 编码。
- JSON 序列化使用 `ensure_ascii=False`，中文可正常传输。