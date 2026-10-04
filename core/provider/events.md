## 内部事件命名规范

内部事件由宿主 Go 代码实现（见 `provider.InternalEvent`），与插件发布的事件共用同一张
EventBus 事件表，因此对订阅方而言二者完全等价。

基础类:    <br>
foundation.xxx.xxx<br>
定义:与插件框架有关的事件

例如:<br>
foundation.framework.loaded

系统类:<br>
system.xxx.xxx<br>
定义：与系统操作有关的，包括硬件设备

例如:<br>
system.device.plugged

## 当前已注册的内部事件

| 事件名 | 提供者 id | 触发时机 | 载荷 |
|---|---|---|---|
| `foundation.framework.started` | `foundation.framework` | Core 启动、内部事件源开始运行 | `{started_at, uptime_seconds, pid}` |
| `foundation.framework.stopping` | `foundation.framework` | Core 收到退出信号、开始收尾 | `{started_at, uptime_seconds, pid}` |
| `foundation.plugin.loaded` | `foundation.framework` | 某个 Base / Active 插件装载成功 | `{plugin_id, plugin_name, plugin_type}` |
| `foundation.plugin.unloaded` | `foundation.framework` | 某个 Base / Active 插件卸载成功 | `{plugin_id, plugin_name, plugin_type}` |
| `system.disk.plugged` | `system.diskChecker` | 检测到新磁盘出现（3s 轮询） | `{all_disks:[{total,symbol}], changed_disks:[...]}` |
| `system.disk.unplug` | `system.diskChecker` | 检测到磁盘消失（3s 轮询） | 同上 |

`plugin_type` 取值为 `kernel` / `base` / `active`。

## 新增一个内部事件源

1. 在 `core/provider/` 下建包，实现 `provider.InternalEvent` 四个方法。
2. `GetMeta()` 返回的事件名必须全局唯一，不能与已有插件事件冲突。
3. `StartService()` 会运行在独立 goroutine 中，应当阻塞直到 `StopService()` 被调用。
4. **不要用忙循环**——轮询请用 `time.Ticker`，否则会打满一个 CPU 核。
5. 在 `main_loop.go` 中调用 `registerInternalEvent(...)` 完成注册。
