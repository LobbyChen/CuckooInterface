local sdk = require("cuckoo_sdk")

sdk.log_info("[LuaBaseDemo] loaded")
sdk.emit_event("lua.timer.tick", {
  source = "LuaBaseDemo",
  count = 1,
  ok = true
})
