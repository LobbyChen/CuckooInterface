#ifndef PYTHON_KERNEL_H
#define PYTHON_KERNEL_H

#include <Python.h>

#include <atomic>
#include <mutex>
#include <string>
#include <vector>
#include <thread>

#include "cuckoo_kernel.h"
#include "cuckoo_plugin.h"

namespace python_kernel {

struct PythonCallback {
  PyObject* callable;
  std::string event_name;
};

struct PythonPluginInstance {
  uint64_t instance_id;
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

  int  InitRuntime(const CuckooHostAPI* api);
  void RunLoop(const CuckooHostAPI* api);
  int  LoadPlugin(const char* plugin_path, const char* manifest_json,
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

  const CuckooHostAPI* host_api_ = nullptr;
  std::atomic<bool> shutdown_flag_{false};
  std::mutex plugins_mu_;
  std::vector<PythonPluginInstance*> plugins_;
  std::atomic<uint64_t> instance_counter_{1};
  bool python_initialized_ = false;
  PyObject* sdk_module_ = nullptr;
  PyThreadState* main_thread_state_ = nullptr;
};

}  // namespace python_kernel
#endif