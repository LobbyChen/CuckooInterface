# 插件配置页面协议

本文档说明如何在插件中声明配置页面、提供默认配置，以及配置页 HTML 与 CuckooInterface 宿主之间的通信协议。

> 适用范围：`Base` / `Active` 插件。`Kernel` 插件的运行时配置由“运行时”页面统一管理，不在插件配置页中展示。

---

## 1. 插件目录结构

支持配置页的插件需要在解压目录中包含一个 HTML 文件，并在 `META-INF.json` 中声明：

```text
MyPlugin/
├── META-INF.json
├── main.py            # 插件入口（按实际运行时语言）
└── config.html        # 配置页 HTML
```

`config.html` 可以引用同目录下的 CSS、JS、图片等相对路径资源，加载时会按插件目录解析。

---

## 2. META-INF.json 声明

填写两个字段

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `config_page` | string | 否 | 配置页 HTML 在插件目录内的相对路径。必须指向真实存在的文件，不允许绝对路径或 `..` 路径穿越。 |
| `default_config` | string | 否 | 默认配置字符串，通常是一段 JSON 文本。仅在对应 `.cfg` 文件不存在时写入。 |

示例：

```json
{
  "name": "MyPlugin",
  "id": "com.example.my_plugin",
  "type": 30,
  "version": "1.0.0",
  "description": "示例插件",
  "runtime_type": "python",
  "listeners": ["some.event.name"],
  "config_page": "config.html",
  "default_config": "{\"interval\": 60, \"enabled\": true}"
}
```

注意：`default_config` 本身是**字符串**，因此其中的 JSON 需要转义。

---

## 3. 配置数据的生命周期

1. Core 在加载插件前确保配置文件存在。
2. 配置文件位于 `configs/<插件ID>.cfg`：
   - 已存在：直接加载已有配置，忽略 `default_config`；
   - 不存在：写入 `default_config`。
3. 用户在配置页保存后，Core 立即更新内存并写回 `.cfg`。
4. 插件通过 SDK 的 `get_plugin_config` 读取配置：
   - 每次调用都会读取 Core 当前内存中的配置，保存后无需重启 Core；
   - 如果插件在加载时把配置缓存到了自己的变量里，需要重新加载插件才能使用新值。

配置内容按字符串原样存储。约定使用 JSON 文本，便于插件侧解析。

---

## 4. 宿主与配置页通信协议

配置页运行在 WebView2 中，通过 `window.chrome.webview` 与宿主通信。

### 4.1 宿主下发配置

页面加载完成后，宿主会通过两种方式下发当前配置：

1. 发送消息：

```json
{ "type": "cuckoo:init", "config": "{\"interval\": 60}" }
```

2. 注入全局变量并派发事件：

```js
window.__cuckooConfig = "{\"interval\": 60}";
window.dispatchEvent(new Event("cuckoo-config-ready"));
```

页面可以选择监听消息，或监听事件/读取全局变量。

### 4.2 配置页保存配置

配置页调用：

```js
window.chrome.webview.postMessage({
  type: "cuckoo:saveConfig",
  config: JSON.stringify({ interval: 60, enabled: true })
});
```

要求：

- `type` 必须是 `cuckoo:saveConfig`；
- `config` 必须是字符串；
- 字符串内容会原样写入配置文件。

### 4.3 宿主返回保存结果

```json
{ "type": "cuckoo:saved", "ok": true, "error": null }
```

失败时：

```json
{ "type": "cuckoo:saved", "ok": false, "error": "失败原因" }
```

---

## 5. 配置页最小示例

```html
<!DOCTYPE html>
<html lang="zh-CN">
<head>
  <meta charset="utf-8">
  <title>MyPlugin 配置</title>
</head>
<body>
  <label>事件间隔（秒）<input id="interval" type="number" value="60"></label>
  <label>启用<input id="enabled" type="checkbox" checked></label>
  <button id="save">保存</button>
  <span id="status"></span>

  <script>
    function applyConfig(raw) {
      let cfg = {};
      try { cfg = raw ? JSON.parse(raw) : {}; } catch { cfg = {}; }
      document.getElementById("interval").value = Number(cfg.interval) || 60;
      document.getElementById("enabled").checked = cfg.enabled !== false;
    }

    function save() {
      const config = JSON.stringify({
        interval: Number(document.getElementById("interval").value) || 60,
        enabled: document.getElementById("enabled").checked
      });
      window.chrome.webview.postMessage({
        type: "cuckoo:saveConfig",
        config
      });
    }

    // 方式一：监听宿主事件
    window.addEventListener("cuckoo-config-ready", () => {
      applyConfig(window.__cuckooConfig);
    });

    // 方式二：监听宿主消息
    if (window.chrome && window.chrome.webview) {
      window.chrome.webview.addEventListener("message", (event) => {
        const msg = event.data || {};
        if (msg.type === "cuckoo:init") {
          applyConfig(msg.config);
        } else if (msg.type === "cuckoo:saved") {
          const status = document.getElementById("status");
          status.textContent = msg.ok ? "已保存" : ("保存失败：" + (msg.error || ""));
        }
      });
    }

    document.getElementById("save").addEventListener("click", save);
  </script>
</body>
</html>
```

---

## 6. 插件侧读取配置

### Python

```python
from cuckoo_sdk import get_plugin_config

cfg = get_plugin_config("com.example.my_plugin")  # 返回 dict
interval = cfg.get("interval", 60)
enabled = cfg.get("enabled", True)
```

`cuckoo_sdk.get_plugin_config` 会把配置字符串按 JSON 解析为字典；解析失败或没有配置时返回 `{}`。

### Lua

```lua
local sdk = require("cuckoo_sdk")

local raw = sdk.get_plugin_config("com.example.my_plugin")  -- 返回原始字符串
local raw2 = sdk.config("com.example.my_plugin")            -- 同上，便捷别名
```

Lua SDK 返回原始字符串，需要自行使用 JSON 库解析。
---

## 8. 完整示例

仓库内的可运行示例：

- [PythonBaseDemo META-INF.json](./implement/Kernel/PythonKernel/sample_plugin/PythonBaseDemo/META-INF.json)
- [PythonBaseDemo config.html](./implement/Kernel/PythonKernel/sample_plugin/PythonBaseDemo/config.html)
- [PythonBaseDemo main.py](./implement/Kernel/PythonKernel/sample_plugin/PythonBaseDemo/main.py)
