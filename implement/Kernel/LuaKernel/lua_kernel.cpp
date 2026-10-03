#include "lua_kernel.h"
#include "json_utils.h"

#include <windows.h>

#include <cmath>
#include <iomanip>
#include <limits>
#include <sstream>
#include <string>
#include <unordered_set>

namespace lua_kernel {

namespace {

constexpr const char* kKernelID = "com.cuckoo.kernel.lua";
constexpr const char* kKernelName = "Lua Kernel";
constexpr const char* kVersion = "1.0.0";
constexpr const char* kRuntime = "lua";
constexpr int kMaxJsonDepth = 32;

LuaKernel* g_kernel = nullptr;

std::string LuaString(lua_State* L, int index) {
  size_t len = 0;
  const char* p = lua_tolstring(L, index, &len);
  return p ? std::string(p, len) : std::string();
}

void AppendJsonEscaped(std::string& out, const char* p, size_t len) {
  static constexpr char hex[] = "0123456789abcdef";
  out.push_back('"');
  for (size_t i = 0; i < len; ++i) {
    const unsigned char c = static_cast<unsigned char>(p[i]);
    switch (c) {
      case '"': out += "\\\""; break;
      case '\\': out += "\\\\"; break;
      case '\b': out += "\\b"; break;
      case '\f': out += "\\f"; break;
      case '\n': out += "\\n"; break;
      case '\r': out += "\\r"; break;
      case '\t': out += "\\t"; break;
      default:
        if (c < 0x20) {
          out += "\\u00";
          out.push_back(hex[(c >> 4) & 0x0F]);
          out.push_back(hex[c & 0x0F]);
        } else {
          out.push_back(static_cast<char>(c));
        }
        break;
    }
  }
  out.push_back('"');
}

bool IsArrayTable(lua_State* L, int index, lua_Unsigned& n) {
  index = lua_absindex(L, index);
  n = lua_rawlen(L, index);
  lua_Unsigned count = 0;
  lua_pushnil(L);
  while (lua_next(L, index) != 0) {
    const int key_index = -2;
    if (!lua_isinteger(L, key_index)) {
      lua_pop(L, 2);
      return false;
    }
    const lua_Integer key = lua_tointeger(L, key_index);
    if (key < 1 || static_cast<lua_Unsigned>(key) > n) {
      lua_pop(L, 2);
      return false;
    }
    ++count;
    lua_pop(L, 1);
  }
  return count == n;
}

bool EncodeJsonValue(lua_State* L, int index, std::string& out,
                     std::unordered_set<const void*>& active_tables,
                     int depth, std::string& error) {
  index = lua_absindex(L, index);
  if (depth > kMaxJsonDepth) {
    error = "payload nesting exceeds maximum depth";
    return false;
  }

  switch (lua_type(L, index)) {
    case LUA_TNIL:
      out += "null";
      return true;
    case LUA_TBOOLEAN:
      out += lua_toboolean(L, index) ? "true" : "false";
      return true;
    case LUA_TNUMBER:
      if (lua_isinteger(L, index)) {
        out += std::to_string(static_cast<long long>(lua_tointeger(L, index)));
      } else {
        const lua_Number value = lua_tonumber(L, index);
        if (!std::isfinite(static_cast<double>(value))) {
          error = "NaN/Inf cannot be encoded as JSON";
          return false;
        }
        std::ostringstream oss;
        oss << std::setprecision(std::numeric_limits<lua_Number>::max_digits10)
            << static_cast<double>(value);
        out += oss.str();
      }
      return true;
    case LUA_TSTRING: {
      size_t len = 0;
      const char* p = lua_tolstring(L, index, &len);
      AppendJsonEscaped(out, p ? p : "", len);
      return true;
    }
    case LUA_TTABLE: {
      const void* identity = lua_topointer(L, index);
      if (!active_tables.insert(identity).second) {
        error = "cyclic table cannot be encoded as JSON";
        return false;
      }

      lua_Unsigned n = 0;
      const bool array = IsArrayTable(L, index, n);
      if (array) {
        out.push_back('[');
        for (lua_Unsigned i = 1; i <= n; ++i) {
          if (i > 1) out.push_back(',');
          lua_rawgeti(L, index, static_cast<lua_Integer>(i));
          const bool ok = EncodeJsonValue(L, -1, out, active_tables, depth + 1,
                                          error);
          lua_pop(L, 1);
          if (!ok) {
            active_tables.erase(identity);
            return false;
          }
        }
        out.push_back(']');
      } else {
        out.push_back('{');
        bool first = true;
        lua_pushnil(L);
        while (lua_next(L, index) != 0) {
          if (!lua_isstring(L, -2)) {
            lua_pop(L, 2);
            active_tables.erase(identity);
            error = "JSON object keys must be strings";
            return false;
          }
          if (!first) out.push_back(',');
          first = false;
          size_t key_len = 0;
          const char* key = lua_tolstring(L, -2, &key_len);
          AppendJsonEscaped(out, key ? key : "", key_len);
          out.push_back(':');
          if (!EncodeJsonValue(L, -1, out, active_tables, depth + 1, error)) {
            lua_pop(L, 1);
            active_tables.erase(identity);
            return false;
          }
          lua_pop(L, 1);
        }
        out.push_back('}');
      }
      active_tables.erase(identity);
      return true;
    }
    default:
      error = "payload type is not JSON serializable";
      return false;
  }
}

bool EncodePayload(lua_State* L, int index, std::string& out, std::string& error) {
  if (lua_isnoneornil(L, index)) {
    out = "{}";
    return true;
  }
  if (lua_isstring(L, index)) {
    out = LuaString(L, index);
    return true;
  }
  std::unordered_set<const void*> active_tables;
  out.clear();
  return EncodeJsonValue(L, index, out, active_tables, 0, error);
}

LuaPluginInstance* PluginFromUpvalue(lua_State* L) {
  auto* inst = static_cast<LuaPluginInstance*>(lua_touserdata(L, lua_upvalueindex(1)));
  if (!inst || !inst->state) return nullptr;
  return inst;
}

int LuaEmitEvent(lua_State* L) {
  auto* inst = PluginFromUpvalue(L);
  if (!inst || !g_kernel || !g_kernel->HostAPI() ||
      !g_kernel->HostAPI()->emit_event) {
    return 0;
  }
  const char* event_name = luaL_checkstring(L, 1);
  std::string payload;
  std::string error;
  if (!EncodePayload(L, 2, payload, error)) {
    return luaL_error(L, "emit_event payload: %s", error.c_str());
  }
  g_kernel->HostAPI()->emit_event(event_name, payload.c_str());
  return 0;
}

int LuaLogInfo(lua_State* L) {
  if (!g_kernel || !g_kernel->HostAPI() || !g_kernel->HostAPI()->log_info) return 0;
  const char* msg = luaL_tolstring(L, 1, nullptr);
  g_kernel->HostAPI()->log_info(msg ? msg : "");
  lua_pop(L, 1);
  return 0;
}

int LuaLogError(lua_State* L) {
  if (!g_kernel || !g_kernel->HostAPI() || !g_kernel->HostAPI()->log_error) return 0;
  const char* msg = luaL_tolstring(L, 1, nullptr);
  g_kernel->HostAPI()->log_error(msg ? msg : "");
  lua_pop(L, 1);
  return 0;
}

int LuaGetPluginConfig(lua_State* L) {
  const char* plugin_name = luaL_optstring(L, 1, "");
  const char* raw = "";
  if (g_kernel && g_kernel->HostAPI() && g_kernel->HostAPI()->get_plugin_config) {
    raw = g_kernel->HostAPI()->get_plugin_config(plugin_name);
  }
  lua_pushstring(L, raw ? raw : "");
  return 1;
}

int LuaOnEvent(lua_State* L) {
  auto* inst = PluginFromUpvalue(L);
  if (!inst) return luaL_error(L, "plugin instance is unavailable");
  const char* event_name = luaL_checkstring(L, 1);
  luaL_checktype(L, 2, LUA_TFUNCTION);
  if (std::strlen(event_name) == 0) return luaL_error(L, "event name is empty");
  if (inst->callbacks.size() >= MAX_LISTENERS_PER_PLUGIN) {
    return luaL_error(L, "too many listeners (max %d)", MAX_LISTENERS_PER_PLUGIN);
  }

  lua_pushvalue(L, 2);
  const int ref = luaL_ref(L, LUA_REGISTRYINDEX);
  auto* callback = new LuaCallback();
  callback->event_name = event_name;
  callback->registry_ref = ref;
  inst->callbacks.push_back(callback);

  // 允许链式写法：local f = on_event("x", function(...) ... end)
  lua_pushvalue(L, 2);
  return 1;
}

int LuaOpenCuckooSDK(lua_State* L) {
  auto* inst = static_cast<LuaPluginInstance*>(lua_touserdata(L, lua_upvalueindex(1)));
  if (!inst) return luaL_error(L, "plugin instance is unavailable");

  lua_newtable(L);

  lua_pushlightuserdata(L, inst);
  lua_pushcclosure(L, LuaOnEvent, 1);
  lua_setfield(L, -2, "on_event");

  lua_pushlightuserdata(L, inst);
  lua_pushcclosure(L, LuaEmitEvent, 1);
  lua_setfield(L, -2, "emit_event");

  lua_pushcfunction(L, LuaLogInfo);
  lua_setfield(L, -2, "log_info");

  lua_pushcfunction(L, LuaLogError);
  lua_setfield(L, -2, "log_error");

  lua_pushcfunction(L, LuaGetPluginConfig);
  lua_setfield(L, -2, "get_plugin_config");

  // 便捷别名，保持与 Python SDK 的心智模型一致。
  lua_getfield(L, -1, "emit_event");
  lua_setfield(L, -2, "emit");
  lua_getfield(L, -1, "log_info");
  lua_setfield(L, -2, "info");
  lua_getfield(L, -1, "log_error");
  lua_setfield(L, -2, "error");
  lua_getfield(L, -1, "get_plugin_config");
  lua_setfield(L, -2, "config");

  return 1;
}

void InstallSDK(lua_State* L, LuaPluginInstance* inst) {
  lua_getglobal(L, "package");
  lua_getfield(L, -1, "preload");
  lua_pushlightuserdata(L, inst);
  lua_pushcclosure(L, LuaOpenCuckooSDK, 1);
  lua_setfield(L, -2, "cuckoo_sdk");
  lua_pop(L, 2);
}

void PushPluginEnvironment(lua_State* L) {
  lua_newtable(L);             // env
  lua_newtable(L);             // metatable
  lua_pushglobaltable(L);      // global table as fallback
  lua_setfield(L, -2, "__index");
  lua_setmetatable(L, -2);
}

}  // namespace

LuaKernel& LuaKernel::Instance() {
  static LuaKernel inst;
  return inst;
}

const char* LuaKernel::KernelID() { return kKernelID; }
const char* LuaKernel::KernelName() { return kKernelName; }
const char* LuaKernel::Version() { return kVersion; }
const char* LuaKernel::Runtime() { return kRuntime; }

uint64_t LuaKernel::NextInstanceID() {
  return instance_counter_.fetch_add(1);
}

lua_State* LuaKernel::CreatePluginState(LuaPluginInstance* inst) {
  lua_State* L = luaL_newstate();
  if (!L) return nullptr;
  luaL_openlibs(L);
  InstallSDK(L, inst);
  return L;
}

void LuaKernel::LogLuaError(const std::string& context, lua_State* state) {
  const char* msg = state ? lua_tostring(state, -1) : nullptr;
  std::string full = "[LuaKernel] " + context + ": " + (msg ? msg : "unknown Lua error");
  if (host_api_ && host_api_->log_error) {
    host_api_->log_error(full.c_str());
  }
}

bool LuaKernel::ExecutePluginFile(LuaPluginInstance* inst,
                                  const std::string& file_path) {
  lua_State* L = inst->state;
  if (luaL_loadfile(L, file_path.c_str()) != LUA_OK) {
    LogLuaError("load " + file_path, L);
    lua_pop(L, 1);
    return false;
  }

  // 每个插件拥有独立 _ENV，但仍可读取 Lua 标准库和全局 API。
  PushPluginEnvironment(L);
  if (lua_setupvalue(L, -2, 1) == nullptr) {
    lua_pop(L, 1);
    if (host_api_ && host_api_->log_error) {
      host_api_->log_error("[LuaKernel] plugin chunk does not expose _ENV");
    }
    return false;
  }

  const int rc = lua_pcall(L, 0, 0, 0);
  if (rc != LUA_OK) {
    LogLuaError("execute " + file_path, L);
    lua_pop(L, 1);
    return false;
  }
  return true;
}

bool LuaKernel::FillListenersFromSDK(CuckooPluginDescriptor* out_descriptor,
                                     LuaPluginInstance* inst) {
  const size_t count = inst->callbacks.size();
  if (count > MAX_LISTENERS_PER_PLUGIN) {
    if (host_api_ && host_api_->log_error) {
      host_api_->log_error("[LuaKernel] plugin declared too many listeners");
    }
    return false;
  }
  out_descriptor->listener_count = static_cast<int>(count);
  for (size_t i = 0; i < count; ++i) {
    const auto* cb = inst->callbacks[i];
    std::strncpy(out_descriptor->listeners[i].event_name,
                 cb->event_name.c_str(), 127);
    out_descriptor->listeners[i].event_name[127] = '\0';
    out_descriptor->listeners[i].function = const_cast<LuaCallback*>(cb);
  }
  return true;
}

int LuaKernel::InitRuntime(const CuckooHostAPI* api) {
  host_api_ = api;
  g_kernel = this;
  shutdown_flag_.store(false);
  if (host_api_ && host_api_->log_info) {
    host_api_->log_info("[LuaKernel] runtime ready (Lua 5.5.1 statically linked)");
  }
  return 0;
}

void LuaKernel::RunLoop(const CuckooHostAPI* api) {
  if (api && api->log_info) {
    api->log_info("[LuaKernel] run_loop starting");
  }
  if (api && api->into_loop_report) {
    api->into_loop_report(kKernelID);
  }
  while (!shutdown_flag_.load()) {
    Sleep(100);
  }
  if (api && api->log_info) {
    api->log_info("[LuaKernel] run_loop exiting");
  }
}

int LuaKernel::LoadPlugin(const char* plugin_path, const char* manifest_json,
                          CuckooPluginDescriptor* out_descriptor) {
  if (!plugin_path || !manifest_json || !out_descriptor) return -1;

  std::string manifest(manifest_json);
  std::string plugin_id;
  std::string runtime;
  std::string script;
  if (!cuckoo_json::GetStringField(manifest, "id", plugin_id) ||
      !cuckoo_json::GetStringField(manifest, "runtime_type", runtime)) {
    if (host_api_ && host_api_->log_error) {
      host_api_->log_error("[LuaKernel] plugin manifest missing id/runtime_type");
    }
    return -1;
  }
  cuckoo_json::GetStringField(manifest, "entry", script);
  if (script.empty()) script = "main.lua";
  if (runtime != kRuntime) {
    if (host_api_ && host_api_->log_error) {
      std::string msg = "[LuaKernel] unsupported runtime_type: " + runtime;
      host_api_->log_error(msg.c_str());
    }
    return -1;
  }

  auto* inst = new LuaPluginInstance();
  inst->instance_id = NextInstanceID();
  inst->plugin_id = plugin_id;
  inst->language_runtime = runtime;
  inst->plugin_path = plugin_path;

  inst->state = CreatePluginState(inst);
  if (!inst->state) {
    delete inst;
    if (host_api_ && host_api_->log_error) {
      host_api_->log_error("[LuaKernel] failed to create lua_State");
    }
    return -1;
  }

  std::string file_path = inst->plugin_path + "/" + script;
  if (!ExecutePluginFile(inst, file_path)) {
    DestroyPluginInstance(inst);
    return -1;
  }

  std::memset(out_descriptor, 0, sizeof(CuckooPluginDescriptor));
  out_descriptor->instance_id = inst->instance_id;
  std::strncpy(out_descriptor->plugin_id, plugin_id.c_str(), 63);
  out_descriptor->plugin_id[63] = '\0';
  std::strncpy(out_descriptor->language_runtime, runtime.c_str(), 31);
  out_descriptor->language_runtime[31] = '\0';

  if (!FillListenersFromSDK(out_descriptor, inst)) {
    DestroyPluginInstance(inst);
    return -1;
  }

  std::vector<std::string> events;
  cuckoo_json::GetStringArrayField(manifest, "events", events);
  const int event_count = static_cast<int>(events.size() > MAX_EVENTS_PER_PLUGIN
                                               ? MAX_EVENTS_PER_PLUGIN
                                               : events.size());
  for (int i = 0; i < event_count; ++i) {
    std::strncpy(out_descriptor->provided_events[i].event_name,
                 events[static_cast<size_t>(i)].c_str(), 127);
    out_descriptor->provided_events[i].event_name[127] = '\0';
  }
  out_descriptor->event_count = event_count;
  out_descriptor->internal_object_ptr = static_cast<void*>(inst);

  {
    std::lock_guard<std::mutex> lk(plugins_mu_);
    plugins_.push_back(inst);
  }

  if (host_api_ && host_api_->log_info) {
    std::string msg = "[LuaKernel] loaded plugin: " + plugin_id +
                      " (runtime=" + runtime +
                      ", listeners=" + std::to_string(out_descriptor->listener_count) +
                      ", events=" + std::to_string(event_count) + ")";
    host_api_->log_info(msg.c_str());
  }
  return 0;
}

void LuaKernel::TriggerCallback(CuckooPluginHandle handle,
                                const CuckooListenerRecord* listener,
                                const char* payload) {
  if (!handle || !listener) return;
  auto* inst = static_cast<LuaPluginInstance*>(handle);
  auto* cb = static_cast<LuaCallback*>(listener->function);
  if (!inst || !cb || !inst->state) return;

  lua_State* L = inst->state;
  lua_rawgeti(L, LUA_REGISTRYINDEX, cb->registry_ref);
  if (!lua_isfunction(L, -1)) {
    lua_pop(L, 1);
    return;
  }
  lua_pushstring(L, payload ? payload : "");
  if (lua_pcall(L, 1, 0, 0) != LUA_OK) {
    LogLuaError("callback " + cb->event_name, L);
    lua_pop(L, 1);
  }
}

void LuaKernel::DestroyPluginInstance(LuaPluginInstance* inst) {
  if (!inst) return;
  if (inst->state) {
    for (auto* cb : inst->callbacks) {
      if (cb->registry_ref != LUA_NOREF && cb->registry_ref != LUA_REFNIL) {
        luaL_unref(inst->state, LUA_REGISTRYINDEX, cb->registry_ref);
      }
      delete cb;
    }
    inst->callbacks.clear();
    lua_close(inst->state);
    inst->state = nullptr;
  } else {
    for (auto* cb : inst->callbacks) delete cb;
    inst->callbacks.clear();
  }
  delete inst;
}

void LuaKernel::UnloadPlugin(CuckooPluginHandle handle) {
  if (!handle) return;
  auto* inst = static_cast<LuaPluginInstance*>(handle);
  {
    std::lock_guard<std::mutex> lk(plugins_mu_);
    for (auto it = plugins_.begin(); it != plugins_.end(); ++it) {
      if (*it == inst) {
        plugins_.erase(it);
        break;
      }
    }
  }
  const std::string plugin_id = inst->plugin_id;
  DestroyPluginInstance(inst);
  if (host_api_ && host_api_->log_info) {
    std::string msg = "[LuaKernel] unloaded plugin: " + plugin_id;
    host_api_->log_info(msg.c_str());
  }
}

void LuaKernel::ShutdownRuntime() {
  shutdown_flag_.store(true);
  std::vector<LuaPluginInstance*> to_delete;
  {
    std::lock_guard<std::mutex> lk(plugins_mu_);
    to_delete.swap(plugins_);
  }
  for (auto* inst : to_delete) {
    DestroyPluginInstance(inst);
  }
  if (host_api_ && host_api_->log_info) {
    host_api_->log_info("[LuaKernel] shutdown_runtime called");
  }
  host_api_ = nullptr;
  g_kernel = nullptr;
}

}  // namespace lua_kernel

extern "C" {

static CuckooKernelInterface g_lua_kernel_interface = {};
static bool g_interface_initialized = false;

static int lua_init_runtime(const CuckooHostAPI* api) {
  return lua_kernel::LuaKernel::Instance().InitRuntime(api);
}

static void lua_run_loop(const CuckooHostAPI* api) {
  lua_kernel::LuaKernel::Instance().RunLoop(api);
}

static int lua_load_plugin(const char* plugin_path, const char* manifest_json,
                           CuckooPluginDescriptor* out_descriptor) {
  return lua_kernel::LuaKernel::Instance().LoadPlugin(plugin_path, manifest_json,
                                                      out_descriptor);
}

static void lua_trigger_callback(CuckooPluginHandle handle,
                                 const CuckooListenerRecord* listener,
                                 const char* payload) {
  lua_kernel::LuaKernel::Instance().TriggerCallback(handle, listener, payload);
}

static void lua_unload_plugin(CuckooPluginHandle handle) {
  lua_kernel::LuaKernel::Instance().UnloadPlugin(handle);
}

static void lua_shutdown_runtime(void) {
  lua_kernel::LuaKernel::Instance().ShutdownRuntime();
}

__declspec(dllexport) CuckooKernelInterface* get_cuckoo_kernel_interface(void) {
  if (!g_interface_initialized) {
    g_lua_kernel_interface.kernel_id = lua_kernel::LuaKernel::KernelID();
    g_lua_kernel_interface.kernel_name = lua_kernel::LuaKernel::KernelName();
    g_lua_kernel_interface.version = lua_kernel::LuaKernel::Version();
    g_lua_kernel_interface.runtime = lua_kernel::LuaKernel::Runtime();
    g_lua_kernel_interface.init_runtime = lua_init_runtime;
    g_lua_kernel_interface.run_loop = lua_run_loop;
    g_lua_kernel_interface.load_plugin = lua_load_plugin;
    g_lua_kernel_interface.trigger_callback = lua_trigger_callback;
    g_lua_kernel_interface.unload_plugin = lua_unload_plugin;
    g_lua_kernel_interface.shutdown_runtime = lua_shutdown_runtime;
    g_interface_initialized = true;
  }
  return &g_lua_kernel_interface;
}

}  // extern "C"
