// python_kernel.h —— PythonKernel 内部状态与接口声明
//
// PythonKernel 通过嵌入 CPython 解释器，为 CuckooInterface 提供
// Python 运行时。它实现了 cuckoo_kernel.h 中定义的 CuckooKernelInterface，
// 由宿主（sandbox_host.c）通过 get_cuckoo_kernel_interface() 加载。
//
// 插件约定：
//   - 插件目录下包含 META-INF.json 与 main.py
//   - main.py 通过 from cuckoo_sdk import on_event, ... 引入 SDK
//   - 使用 @on_event("event.name") 装饰器注册事件回调
//   - 回调签名：def callback(payload: str)
#ifndef PYTHON_KERNEL_H
#define PYTHON_KERNEL_H

#include <Python.h>

#include <atomic>
#include <mutex>
#include <string>
#include <vector>

#include "cuckoo_kernel.h"
#include "cuckoo_plugin.h"

namespace python_kernel {

// PythonCallback —— 持有一个 Python 可调用对象（监听器回调）
// 对应 CuckooListenerRecord.function 字段，在 trigger_callback 时取回并调用。
struct PythonCallback {
  PyObject* callable;   // 插件模块拥有此引用；内核不持有
  std::string event_name;
};

// PythonPluginInstance —— 由 PythonKernel 加载的插件实例
// 对应 CuckooPluginDescriptor.internal_object_ptr。
struct PythonPluginInstance {
  uint64_t instance_id;
  std::string plugin_id;
  std::string language_runtime;
  std::string plugin_path;
  PyObject* module;  // 已导入的插件模块（强引用）
  std::vector<PythonCallback*> callbacks;
};

// PythonKernel —— 单例内核，持有 HostAPI、Python 解释器与所有已加载插件实例
class PythonKernel {
 public:
  static PythonKernel& Instance();

  // ---- CuckooKernelInterface 对应实现 ----
  int InitRuntime(const CuckooHostAPI* api);
  void RunLoop(const CuckooHostAPI* api);
  int LoadPlugin(const char* plugin_path, const char* manifest_json,
                 CuckooPluginDescriptor* out_descriptor);
  void TriggerCallback(CuckooPluginHandle handle,
                       const CuckooListenerRecord* listener,
                       const char* payload);
  void UnloadPlugin(CuckooPluginHandle handle);
  void ShutdownRuntime();

  // 元数据
  static const char* KernelID();
  static const char* KernelName();
  static const char* Version();
  static const char* Runtime();

  // 供 C 回调函数访问的宿主 API
  const CuckooHostAPI* HostAPI() const { return host_api_; }

 private:
  PythonKernel() = default;
  ~PythonKernel() = default;
  PythonKernel(const PythonKernel&) = delete;
  PythonKernel& operator=(const PythonKernel&) = delete;

  uint64_t NextInstanceID();

  // 获取内核 DLL 所在目录，用于定位内置 SDK
  std::string GetKernelDllDir();

  // 将宿主 API 注入到 cuckoo_sdk 模块
  bool InjectHostAPIIntoSDK();

  // 从插件目录导入 main.py，返回新模块（强引用），失败返回 nullptr
  PyObject* ImportPluginModule(const std::string& plugin_path,
                               const std::string& module_name);

  // 读取 cuckoo_sdk._listeners 列表，填充 descriptor 与 callbacks
  bool FillListenersFromSDK(CuckooPluginDescriptor* out_descriptor,
                            PythonPluginInstance* inst);

  // 读取 cuckoo_sdk._listeners 后清空（供下个插件加载使用）
  void ClearSDKListeners();

  // 捕获并记录 Python 异常到宿主日志
  void LogPythonException(const std::string& context);

  const CuckooHostAPI* host_api_ = nullptr;
  std::atomic<bool> shutdown_flag_{false};
  std::mutex plugins_mu_;
  std::vector<PythonPluginInstance*> plugins_;
  std::atomic<uint64_t> instance_counter_{1};
  bool python_initialized_ = false;
  PyObject* sdk_module_ = nullptr;  // cuckoo_sdk 模块（强引用）
};

}  // namespace python_kernel

#endif  // PYTHON_KERNEL_H
