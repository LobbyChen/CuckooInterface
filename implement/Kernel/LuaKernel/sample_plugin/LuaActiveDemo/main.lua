local sdk = require("cuckoo_sdk")

sdk.on_event("lua.timer.tick", function(payload)
  sdk.log_info("[LuaActiveDemo] received: " .. payload)
  sdk.emit_event("lua.active.tick_handled", {
    status = "ok",
    source = "LuaActiveDemo"
  })
end)
