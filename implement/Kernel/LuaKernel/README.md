# CuckooInterface Lua Kernel


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
