// mock_kernel.h —— MockKernel 内部状态与接口声明
//
// MockKernel 是 CuckooInterface 插件体系的一个最小化内核实现，
// 用于在无真实脚本运行时（如 Lua/Python）的情况下验证插件加载、
// 事件分发与生命周期管理链路。
//
// 它实现了 cuckoo_kernel.h 中定义的 CuckooKernelInterface，
// 通过 get_cuckoo_kernel_interface() 导出给宿主（sandbox_host.c）加载。
#ifndef MOCK_KERNEL_H
#define MOCK_KERNEL_H

#include <atomic>
#include <mutex>
#include <string>
#include <vector>

#include "cuckoo_kernel.h"
#include "cuckoo_plugin.h"

namespace mock_kernel {

// MockPluginInstance —— 由 MockKernel 加载的插件实例
// 对应 CuckooPluginDescriptor.internal_object_ptr，
// 在 trigger_callback / unload_plugin 时通过 handle 取回。
struct MockPluginInstance {
  uint64_t instance_id;
  std::string plugin_id;
  std::string language_runtime;
  std::vector<std::string> listeners;
  std::vector<std::string> provided_events;
  std::string plugin_path;
};

// MockKernel —— 单例内核，持有 HostAPI 与所有已加载插件实例
class MockKernel {
 public:
  static MockKernel& Instance();

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

  // 元数据（供 get_cuckoo_kernel_interface 填充）
  static const char* KernelID();
  static const char* KernelName();
  static const char* Version();
  static const char* Runtime();

 private:
  MockKernel() = default;
  ~MockKernel() = default;
  MockKernel(const MockKernel&) = delete;
  MockKernel& operator=(const MockKernel&) = delete;

  // 生成自增的 instance_id
  uint64_t NextInstanceID();

  const CuckooHostAPI* host_api_ = nullptr;
  std::atomic<bool> shutdown_flag_{false};
  std::mutex plugins_mu_;
  std::vector<MockPluginInstance*> plugins_;
  std::atomic<uint64_t> instance_counter_{1};
};

}  // namespace mock_kernel

#endif  // MOCK_KERNEL_H
