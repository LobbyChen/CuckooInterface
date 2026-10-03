# CuckooInterface Lua Kernel

这是一个与现有 `CuckooKernelInterface` 兼容的 Lua Runtime Kernel。

## 目标

- `runtime_type = "lua"`
- Lua VM 直接编译进 `export.dll`
- 每个业务插件拥有独立 `lua_State`
- Base / Active 插件模型与现有 Python Kernel 一致
- 通过 `CuckooHostAPI` 访问 EventBus、日志和配置
- 插件事件回调完全由现有 SandboxWorker 串行调用

## Lua SDK

```lua
local sdk = require("cuckoo_sdk")

sdk.log_info("hello")
sdk.log_error("oops")

sdk.on_event("demo.event", function(payload)
  sdk.log_info(payload)
end)

sdk.emit_event("demo.event", { ok = true, count = 1 })

local raw = sdk.get_plugin_config("com.example.plugin")
```

`emit_event` 支持：字符串、nil、boolean、number、table。Lua table 会被编码为 JSON；循环引用、非字符串对象键和不支持的值类型会返回 Lua error。

`get_plugin_config` 返回 Core 提供的原始 JSON 字符串，不额外绑定 JSON 解析器。

插件默认入口为 `main.lua`，也可以在 `META-INF.json` 中用 `entry` 指定脚本文件名。

## 构建

当前环境无法联网下载 Lua 源码，因此没有把第三方 Lua 源码字节直接放进本交付目录。运行：

```bat
vendor_lua.bat
build_lua_kernel.bat
```

`vendor_lua.bat` 会从 Lua 官方站点获取 Lua 5.5.1，并校验 SHA-256；CMake 随后直接把 Lua `src/*.c` 编进 `export.dll`，不依赖外部 `lua.dll`。

也可以预先把 Lua 5.5.1 解压到：

```text
vendor/lua-5.5.1/
```

然后直接运行 `build_lua_kernel.bat`。

## 集成到现有项目

生成：

```text
build/Release/binary/export.dll
```

把下面内容打包到：

```text
user/Kernel/LuaKernel/
├── META-INF.json
└── binary/
    └── export.dll
```

或者按项目现有 Kernel ZIP 方式打成 `LuaKernel.zip` 放入 `user/Kernel/`。

现有 Core 会依据 `runtime_type == "lua"` 自动匹配此 Kernel。`KernelManager` 的匹配逻辑无需修改。
