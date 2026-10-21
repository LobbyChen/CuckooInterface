// Copyright (C) 2026 LobbyChen
// SPDX-License-Identifier: AGPL-3.0-only
#ifndef PYTHON_KERNEL_H
#define PYTHON_KERNEL_H

#include "python_runtime.h"

#include <atomic>
#include <mutex>
#include <string>
#include <thread>
#include <vector>

#include "cuckoo_kernel.h"
#include "cuckoo_plugin.h"

namespace python_kernel {

struct PythonCallback {
  PyObject* callable = nullptr;
  std::string event_name;
};

struct PythonPluginInstance {
  uint64_t instance_id = 0;
  std::string plugin_id;
  std::string language_runtime;
  std::string plugin_path;
  PyObject* module = nullptr;
  std::vector<PythonCallback*> callbacks;

  // ---- 专属线程 ----
  std::thread worker;
  std::atomic<bool> exec_failed{false};
  std::atomic<bool> exec_done{false};
  unsigned long py_thread_id = 0;
};

class PythonKernel {
 public:
  static PythonKernel& Instance();

  int InitRuntime(const CuckooHostAPI* api);
  void RunLoop(const CuckooHostAPI* api);
  int LoadPlugin(const char* plugin_path, const char* manifest_json,
                 CuckooPluginDescriptor* out_descriptor);
  void TriggerCallback(CuckooPluginHandle handle,
                       const CuckooListenerRecord* listener,
                       const char* payload);
  void UnloadPlugin(CuckooPluginHandle handle);
  void ShutdownRuntime();

  /* Runtime Control Plane */
  int GetSettingsPanel(const CuckooHostAPI* api, char** out_json);
  int GetRuntimeState(const CuckooHostAPI* api, char** out_json);
  int SetSetting(const CuckooHostAPI* api, const char* key,
                 const char* value_json, char** out_json);
  int GetOperations(const CuckooHostAPI* api, char** out_json);
  int InvokeOperation(const CuckooHostAPI* api, const char* operation_id,
                      const char* request_json, char** out_json);

  static const char* KernelID();
  static const char* KernelName();
  static const char* Version();
  static const char* Runtime();
  const CuckooHostAPI* HostAPI() const { return host_api_; }

 private:
  PythonKernel() = default;
  ~PythonKernel() = default;
  PythonKernel(const PythonKernel&) = delete;
  PythonKernel& operator=(const PythonKernel&) = delete;

  uint64_t NextInstanceID();
  std::string GetKernelDllDir();
  bool InjectHostAPIIntoSDK();
  bool FillListenersFromSDK(CuckooPluginDescriptor* out_descriptor,
                            PythonPluginInstance* inst);
  void ClearSDKListeners();
  void LogPythonException(const std::string& context);
  size_t PluginCount();
  std::string BuildRuntimeStateJson();
  std::string BuildSettingsPanelJson();
  std::string BuildOperationsJson();
  std::string BuildPluginListJson();

  const CuckooHostAPI* host_api_ = nullptr;
  std::atomic<bool> shutdown_flag_{false};
  std::atomic<bool> log_callbacks_{false};
  std::mutex plugins_mu_;
  std::vector<PythonPluginInstance*> plugins_;
  std::atomic<uint64_t> instance_counter_{1};
  bool python_initialized_ = false;
  PyObject* sdk_module_ = nullptr;
  PyThreadState* main_thread_state_ = nullptr;
};

}  // namespace python_kernel
#endif
