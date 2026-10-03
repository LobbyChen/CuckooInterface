#ifndef CUCKOO_LUA_KERNEL_H
#define CUCKOO_LUA_KERNEL_H

#include <atomic>
#include <cstdint>
#include <mutex>
#include <string>
#include <vector>

extern "C" {
#include "lua.h"
#include "lauxlib.h"
#include "lualib.h"
#include "cuckoo_kernel.h"
#include "cuckoo_plugin.h"
}

namespace lua_kernel {

struct LuaCallback {
  std::string event_name;
  int registry_ref = LUA_NOREF;
};

struct LuaPluginInstance {
  uint64_t instance_id = 0;
  std::string plugin_id;
  std::string language_runtime;
  std::string plugin_path;
  lua_State* state = nullptr;
  std::vector<LuaCallback*> callbacks;
};

class LuaKernel {
 public:
  static LuaKernel& Instance();

  int InitRuntime(const CuckooHostAPI* api);
  void RunLoop(const CuckooHostAPI* api);
  int LoadPlugin(const char* plugin_path, const char* manifest_json,
                 CuckooPluginDescriptor* out_descriptor);
  void TriggerCallback(CuckooPluginHandle handle,
                       const CuckooListenerRecord* listener,
                       const char* payload);
  void UnloadPlugin(CuckooPluginHandle handle);
  void ShutdownRuntime();

  static const char* KernelID();
  static const char* KernelName();
  static const char* Version();
  static const char* Runtime();

  const CuckooHostAPI* HostAPI() const { return host_api_; }

 private:
  LuaKernel() = default;
  ~LuaKernel() = default;
  LuaKernel(const LuaKernel&) = delete;
  LuaKernel& operator=(const LuaKernel&) = delete;

  uint64_t NextInstanceID();
  lua_State* CreatePluginState(LuaPluginInstance* inst);
  bool ExecutePluginFile(LuaPluginInstance* inst, const std::string& file_path);
  bool FillListenersFromSDK(CuckooPluginDescriptor* out_descriptor,
                            LuaPluginInstance* inst);
  void LogLuaError(const std::string& context, lua_State* state);
  void DestroyPluginInstance(LuaPluginInstance* inst);

  const CuckooHostAPI* host_api_ = nullptr;
  std::atomic<bool> shutdown_flag_{false};
  std::mutex plugins_mu_;
  std::vector<LuaPluginInstance*> plugins_;
  std::atomic<uint64_t> instance_counter_{1};

};

}  // namespace lua_kernel

#endif  // CUCKOO_LUA_KERNEL_H
