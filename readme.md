# CuckooInterface

<div align="center">
  <img src=".\icon.png" alt="icon" height="100px" width="100px">
</div>

> 面向电教场景的**下一代插件平台**


CuckooInterface 是一个主要为电教场景打造的，面向 Windows 的插件运行时与宿主管理平台。

---

## 它为了解决什么?
我们观察到，当前电教场景经常存在小工具泛滥、难以管理的现状。即使ClassIsland提供了完善的自动化，但是它的能力受限于CI自动化框架的能力。

如果希望系统级集成，那么它也无法运行。因为ClassIsland的插件开发依赖C#，而C#具备一定的上手难度和门槛。

我们希望能解决这项问题，因此编写了CuckooInterface。

官方提供Python Kernel和Lua SDK，只需要安装python就可以使用Python进行插件开发，也可以采用Lua进行开发。

请阅读
- [Python SDK](./implement/Kernel/PythonKernel/sdk/cuckoo_sdk/sdk.md)
- [Lua SDK](./implement/Kernel/LuaKernel/README.md)

我们也鼓励编写第三方的其他编程语言SDK。
我们预计会在下一个版本支持JVM语言。

---

## ✨ 特性

- **多层级插件模型**：Kernel / Base / Active
- **Kernel Runtime 抽象**：通过统一 C ABI 托管不同脚本/运行时
- **CGo + C ABI 桥接**：Go Core 与 Kernel / 插件运行时之间通过 Host API 交互
- **事件驱动架构**：内置 EventBus，支持事件提供、监听、异步分发与最近事件缓冲
- **插件热管理**：扫描、解压、加载、卸载、删除、外部安装
- **RuntimeManager**：统一管理 Kernel 运行时状态、动态设置面板、运行时操作与环境偏好
- **可管理的运行时**：RuntimeManager 发现目标机器上的 CPython，用户可在运行时页面选择 Python Kernel 使用哪一份安装；Lua Kernel 静态链接 Lua VM
- **配置系统**：支持插件独立配置，并提供类型化 Setting / 动态 UI 描述模型
- **双 IPC 通道**：Core 与 Daemon 分别通过 Windows Named Pipe 对外提供控制接口
- **Daemon 守护**：负责 Core 生命周期、保活与系统托盘
- **WPF UI**：提供概览、插件、事件、日志、配置和设置等管理页面
- **可观测性**：日志、事件记录、插件状态、Kernel 状态与运行时信息均可通过 UI 查询
- **运行时隔离准备**：Kernel 通过 sandbox host 管理 DLL、线程、任务以及崩溃状态

---
## 🧩 插件分层

CuckooInterface 当前定义了三种插件类型：

| 类型 | 职责 | 典型能力 |
|---|---|---|
| `Kernel` | 提供底层运行时 | Lua / Python / 其他可被宿主统一托管的 runtime |
| `Base` | 基础能力插件、事件提供者 | 设备监控、进程监控、平台基础服务 |
| `Active` | 应用层插件、事件消费者 | 业务逻辑、自动化动作、事件响应 |

### Kernel

Kernel 是整个插件体系的运行时基础。

Kernel 初始化时，宿主注入 `CuckooHostAPI`。
### Base

Base 插件主要承担“提供平台能力”和“发布事件”的职责。

加载后，其 `provided_events` 会被注册到 EventBus。事件名需要全局唯一。

### Active

Active 插件主要承担“消费事件”和“执行业务逻辑”的职责。

其 `listeners` 会注册到 EventBus。当对应事件触发时，宿主将通过关联 Kernel 调用插件回调。

## 📦 插件目录与文件格式

项目定义了以下主要目录：

```text
logs/                  # Core 日志
user/                  # 用户插件根目录
user/Kernel/           # Kernel 插件
user/Base/             # Base 插件
user/Active/           # Active 插件
plugins/               # 运行时解压后的插件目录
configs/               # 全局配置与插件配置
```

运行时插件目录会把用户目录中的插件包解压到程序目录下的 `plugins/`。

## 📁 项目结构

```text
CuckooInterface/
├── main.go                      # 程序入口，根据 launch_type 分发到 Core 或 Daemon
├── main_loop.go                 # Core 主循环（Normal 模式）
├── daemon_loop.go               # Daemon 主循环（托盘 / Core 保活）
├── constant.go                  # 进程间通信端口常量
├── build.bat                    # 一键构建脚本（Go Core + WPF UI + Kernel）
├── go.mod / go.sum              # Go 模块依赖
├── icon.ico / icon.png / icon.svg
├── LICENSE.txt
├── readme.md
│
├── core/                        # Go Core 运行时
│   ├── config/                  # 全局与插件配置
│   │   ├── config.go            # 配置加载与持久化
│   │   ├── globalPage.go        # 全局配置页模型
│   │   └── settings_models.go   # 设置面板数据模型
│   ├── constant/                # 常量、插件类型、目录、IPC 名称
│   │   └── constant.go
│   ├── event/                   # EventBus 事件总线
│   │   └── event_framework.go
│   ├── ipc/                     # Core 与前端的 IPC 桥接
│   │   ├── core_fronted_bridge.go
│   │   ├── pkgs.go              # IPC 数据包定义
│   │   └── protocol.go          # 协议帧格式
│   ├── logger/                  # 日志系统
│   │   └── logger.go
│   ├── plugin_manager/          # 插件与 Kernel 管理
│   │   ├── kernel_manager.go    # Kernel 加载 / 初始化 / 生命周期
│   │   ├── plugin_file_manager.go  # 插件扫描 / 解压 / 文件管理
│   │   └── plugin_manager.go    # Base / Active 插件加载与事件注册
│   ├── runtime_manager/         # 运行时统一管理
│   │   ├── runtime_manager.go   # Kernel 状态聚合 / 控制面调用 / Python 环境探测
│   │   ├── selection_panel.go   # 动态运行时选择面板
│   │   └── config.go            # configs/runtimes.json 运行时偏好
│   ├── service/                 # Windows 服务注册 / 卸载 / 状态查询
│   └── utils/                   # 通用工具
│       ├── buffer.go            # 字节缓冲
│       ├── json.go              # JSON 辅助
│       ├── msgbox.go            # 系统消息框
│       ├── path.go              # 路径处理
│       ├── process.go           # 进程管理
│       └── singleinstance.go    # 单实例锁
│
├── dameon/                      # Daemon 进程（托盘 / Core 保活）
│   ├── daemon_fronted_bridge.go # Daemon 与前端 IPC
│   ├── dameon_utils.go
│   ├── get_args.go              # 启动参数解析
│   ├── system.go                # 系统调用
│   ├── tray.go                  # 系统托盘
│   └── icon.ico
│
├── plugins/                     # Kernel / Plugin C ABI 与 Go 桥接
│   ├── cuckoo_kernel.h          # Kernel 接口 ABI
│   ├── cuckoo_plugin.h          # Plugin Descriptor ABI
│   ├── kernel_warpper.go        # Go ↔ C Kernel 桥接（cgo）
│   ├── sandbox_host.c           # Kernel 沙箱宿主（DLL / 线程 / 崩溃隔离）
│   └── sandbox_host.h
│
├── implement/                   # Kernel 参考实现
│   └── Kernel/
│       ├── MockKernel/          # 最小化 Mock 内核（验证插件加载与事件链路）
│       │   ├── mock_kernel.h / mock_kernel.cpp
│       │   ├── json_utils.h
│       │   ├── CMakeLists.txt
│       │   ├── build.bat
│       │   ├── META-INF.json
│       │   └── sample_plugin/   # Mock 示例插件
│       │       ├── MockBaseDemo/
│       │       └── MockActiveDemo/
│       │
│       └── PythonKernel/        # CPython 运行时内核
│           ├── python_kernel.h / python_kernel.cpp
│           ├── python_runtime.h / python_runtime.cpp  # 动态加载 CPython 稳定 ABI
│           ├── json_utils.h
│           ├── CMakeLists.txt
│           ├── build.bat
│           ├── META-INF.json
│           ├── sdk/             # Python 插件开发 SDK
│           │   ├── install.bat  # SDK 安装脚本（一键装到 site-packages）
│           │   └── cuckoo_sdk/
│           │       ├── __init__.py
│           │       └── sdk.md   # SDK 文档
│           └── sample_plugin/   # Python 示例插件
│               ├── PythonBaseDemo/
│               └── PythonActiveDemo/
│
└── CuckooInterfaceUI/           # WPF 桌面前端
    ├── App.xaml / App.xaml.cs
    ├── MainWindow.xaml / MainWindow.xaml.cs
    ├── CuckooInterfaceUI.csproj
    ├── CuckooInterfaceUI.slnx
    ├── AssemblyInfo.cs
    ├── Models/                  # 前端数据模型
    │   ├── EventRecord.cs
    │   ├── LogEntry.cs
    │   ├── PluginInfo.cs
    │   ├── SettingsPanel.cs
    │   └── SystemOverview.cs
    ├── Pages/                   # 功能页面
    │   ├── HomePage.xaml        # 概览
    │   ├── PluginsPage.xaml     # 插件管理
    │   ├── RuntimesPage.xaml    # 运行时管理
    │   ├── ConfigPage.xaml      # 配置
    │   ├── EventsPage.xaml      # 事件记录
    │   ├── LogsPage.xaml        # 日志
    │   └── SettingsPage.xaml    # 设置
    ├── Services/
    │   └── CoreBackend.cs       # 与 Core / Daemon IPC 通信
    ├── Behaviors/
    │   └── SmoothScrollBehavior.cs
    └── Converters/
        └── VerticalTextConverter.cs
```

---

## 🔨 构建

当前项目包含 Go Core、C/CGO 桥接代码以及 .NET WPF UI，因此构建环境应为 Windows 开发环境。

### 一键构建（推荐）

根目录提供 `build.bat`，自动完成 Go Core、WPF UI 与 PythonKernel 的构建与打包：

```bat
build.bat
```

前置依赖：`go`、`dotnet`、`cmake`、`powershell` 以及 MSVC 或 MinGW 工具链。

PythonKernel 不再需要构建机的 Python 开发包，Kernel 包也不包含 Python 运行时。运行 Python 插件的目标机器需要安装 **64 位 CPython 3.8+**，安装位置可在运行时页面中选择。

产物输出到 `bin/` 目录，包含 `CuckooInterface.exe`、WPF UI 文件以及 `user/Kernel/PythonKernel.zip`、`user/Kernel/LuaKernel.zip`。

### 构建 Go Core

```bash
go build -ldflags="-H windowsgui" .
```

由于项目使用 CGO 与 C 桥接，构建环境需要具备可用的 CGO/C 编译链。

### 构建 UI

项目 UI 使用：

```text
.NET 10
WPF
WPF-UI 3.0.5
```

构建：

```bash
dotnet build CuckooInterfaceUI/CuckooInterfaceUI.csproj
```
---

## ▶️ 启动方式

程序入口根据 `launch_type` 决定启动模式。

### Normal

```bash
CuckooInterface.exe -launch_type normal
```

Normal 模式直接运行 Core。

### Daemon

```bash
CuckooInterface.exe -launch_type dameon
```
Daemon 模式负责：

1. 启动 Daemon
2. 建立 Daemon IPC
3. 启动 Core
4. 启动 UI
5. 维持 Core
6. 退出时统一清理


## ⚙️ 运行时管理

Core 中的 `RuntimeManager` 统一聚合每个 Kernel 的运行状态，并通过 Runtime Control Plane 调用 Kernel 的：

- `get_runtime_state`：运行时版本、目录、插件数量
- `get_settings_panel` / `set_setting`：Kernel 自定义设置
- `get_operations` / `invoke_operation`：Kernel 自定义操作（如 `runtime.probe`）

WPF 的“运行时”页面会展示以上信息，并提供 Python 运行时环境选择。

### Python 运行时

PythonKernel 通过 CPython 稳定 ABI 动态加载 `python3.dll`，因此同一个 `export.dll` 可以运行在任意 CPython 3.x 上，不再与构建机 Python 版本绑定,也不随包分发 Python。

运行时页面由 RuntimeManager 动态生成（与设置页相同的 `SettingPanel` 模型），会从以下位置发现 Python：

1. 用户在运行时页面手动选择的安装（`configs/runtimes.json`）
2. 系统 PATH
3. Windows `py` 启动器
4. 注册表中的 Python 安装

页面会显示每个安装的版本、架构、路径与兼容状态；仅 64 位 CPython 3.8+ 可被选择。选择后重启 Core 生效。Python Kernel 在运行时按“用户选择 → PATH → 注册表”的顺序定位 CPython；若机器上没有可用 Python，Kernel 会返回明确的安装提示。

### Lua 运行时

Lua 5.5.1 VM 直接静态链接进 `export.dll`，目标机器无需安装 Lua 或额外 VC 运行库。

---

## ⚙️ 设置项行为

- **开机自启动**：写入当前用户 `Run` 注册表项，使用 `-no_ui` 静默启动守护进程和托盘，不自动打开 UI。
- **将 Core 注册为系统服务并开机自启**：需要管理员权限。注册后服务以系统账户在后台运行 Core，开机自动启动且不显示界面；命名管道仅授权安装服务时的用户 SID、SYSTEM 与 Administrators。
- **日志级别 / 单文件大小 / 压缩**：修改后立即生效。
- **单实例 / 启动时自动加载插件 / 加载失败继续 / 插件目录**：保存后需重启 Core 生效。
- **主题 / 强调色 / 字号缩放**：UI 立即应用。

---

## 🧪 一个插件应该如何工作

一个典型的 Base / Active 插件可以抽象成：

```text
1. 被添加，并放入 user/Base 或 user/Active
2. Core 扫描并解压
3. Core 读取 META-INF.json
4. 根据 runtime_type 找到匹配 Kernel
5. Kernel 创建插件实例
6. Base -> 注册 provided events
7. Active -> 注册 listeners
8. 运行时通过 EventBus 传递事件
9. 插件通过 HostAPI 获取配置、记录日志或发送事件
10. 卸载时反注册事件并销毁插件实例
```

---

## 许可证
本项目采用AGPL-3.0开源
[许可证](LICENSE.txt)

## 鸣谢
感谢在本项目开发时为本项目提供帮助的开发者
也感谢每一个愿意编写插件和使用的你
我们一起进步
