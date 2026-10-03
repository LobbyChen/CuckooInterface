// mock_kernel.cpp —— MockKernel 实现
//
// 导出 get_cuckoo_kernel_interface()，返回填充好的 CuckooKernelInterface。
// 所有函数转发到 mock_kernel::MockKernel 单例。

#include "mock_kernel.h"
#include "json_utils.h"

#include <cstring>
#include <thread>
#include <chrono>

namespace mock_kernel {

// ---- 元数据常量 ----
static const char* kKernelID = "com.cuckoo.kernel.mock";
static const char* kKernelName = "Mock Kernel";
static const char* kVersion = "1.0.0";
static const char* kRuntime = "mock";

MockKernel& MockKernel::Instance() {
  static MockKernel inst;
  return inst;
}

const char* MockKernel::KernelID() { return kKernelID; }
const char* MockKernel::KernelName() { return kKernelName; }
const char* MockKernel::Version() { return kVersion; }
const char* MockKernel::Runtime() { return kRuntime; }

uint64_t MockKernel::NextInstanceID() {
  return instance_counter_.fetch_add(1);
}

int MockKernel::InitRuntime(const CuckooHostAPI* api) {
  host_api_ = api;
  if (api && api->log_info) {
    api->log_info("[MockKernel] init_runtime called");
  }
  return 0;  // 0 表示成功
}

void MockKernel::RunLoop(const CuckooHostAPI* api) {
  if (api && api->log_info) {
    api->log_info("[MockKernel] run_loop starting");
  }
  // 通知宿主：已进入循环。宿主据此解除 StartLoop 的阻塞等待。
  if (api && api->into_loop_report) {
    api->into_loop_report(kKernelID);
  }
  // 事件循环：Mock 不处理真实事件，仅自旋等待关闭信号。
  while (!shutdown_flag_.load()) {
    std::this_thread::sleep_for(std::chrono::milliseconds(100));
  }
  if (api && api->log_info) {
    api->log_info("[MockKernel] run_loop exiting");
  }
}

int MockKernel::LoadPlugin(const char* plugin_path, const char* manifest_json,
                           CuckooPluginDescriptor* out_descriptor) {
  if (!plugin_path || !manifest_json || !out_descriptor) return -1;

  std::string manifest(manifest_json);
  std::string plugin_id, runtime, name;
  mock_json::GetStringField(manifest, "id", plugin_id);
  mock_json::GetStringField(manifest, "runtime_type", runtime);
  mock_json::GetStringField(manifest, "name", name);

  // 创建内部插件实例
  auto* inst = new MockPluginInstance();
  inst->instance_id = NextInstanceID();
  inst->plugin_id = plugin_id;
  inst->language_runtime = runtime;
  inst->plugin_path = plugin_path;
  mock_json::GetStringArrayField(manifest, "listeners", inst->listeners);
  mock_json::GetStringArrayField(manifest, "events", inst->provided_events);

  {
    std::lock_guard<std::mutex> lk(plugins_mu_);
    plugins_.push_back(inst);
  }

  // 填充描述符
  std::memset(out_descriptor, 0, sizeof(CuckooPluginDescriptor));
  out_descriptor->instance_id = inst->instance_id;

  // plugin_id
  std::strncpy(out_descriptor->plugin_id, plugin_id.c_str(), 63);
  out_descriptor->plugin_id[63] = '\0';

  // language_runtime
  std::strncpy(out_descriptor->language_runtime, runtime.c_str(), 31);
  out_descriptor->language_runtime[31] = '\0';

  // listeners
  int lc = 0;
  for (const auto& ev : inst->listeners) {
    if (lc >= MAX_LISTENERS_PER_PLUGIN) break;
    std::strncpy(out_descriptor->listeners[lc].event_name, ev.c_str(), 127);
    out_descriptor->listeners[lc].event_name[127] = '\0';
    out_descriptor->listeners[lc].function = nullptr;  // Mock 无实际回调函数
    ++lc;
  }
  out_descriptor->listener_count = lc;

  // provided_events
  int ec = 0;
  for (const auto& ev : inst->provided_events) {
    if (ec >= MAX_EVENTS_PER_PLUGIN) break;
    std::strncpy(out_descriptor->provided_events[ec].event_name, ev.c_str(), 127);
    out_descriptor->provided_events[ec].event_name[127] = '\0';
    ++ec;
  }
  out_descriptor->event_count = ec;

  // internal_object_ptr 指向实例，作为 handle 回传
  out_descriptor->internal_object_ptr = static_cast<void*>(inst);

  if (host_api_ && host_api_->log_info) {
    std::string msg = "[MockKernel] loaded plugin: " + plugin_id +
                      " (runtime=" + runtime +
                      ", listeners=" + std::to_string(lc) +
                      ", events=" + std::to_string(ec) + ")";
    host_api_->log_info(msg.c_str());
  }

  return 0;  // 0 表示成功
}

void MockKernel::TriggerCallback(CuckooPluginHandle handle,
                                 const CuckooListenerRecord* listener,
                                 const char* payload) {
  if (!handle || !listener) return;
  auto* inst = static_cast<MockPluginInstance*>(handle);

  std::string ev_name(listener->event_name ? listener->event_name : "");
  std::string pl(payload ? payload : "");

  if (host_api_ && host_api_->log_info) {
    std::string msg = "[MockKernel] trigger_callback plugin=" + inst->plugin_id +
                      " event=" + ev_name + " payload=" + pl;
    host_api_->log_info(msg.c_str());
  }
  // Mock 不执行真实回调；如需模拟事件回传，可在此调用 host_api_->emit_event。
}

void MockKernel::UnloadPlugin(CuckooPluginHandle handle) {
  if (!handle) return;
  auto* inst = static_cast<MockPluginInstance*>(handle);

  {
    std::lock_guard<std::mutex> lk(plugins_mu_);
    for (auto it = plugins_.begin(); it != plugins_.end(); ++it) {
      if (*it == inst) {
        plugins_.erase(it);
        break;
      }
    }
  }

  if (host_api_ && host_api_->log_info) {
    std::string msg = "[MockKernel] unloaded plugin: " + inst->plugin_id;
    host_api_->log_info(msg.c_str());
  }
  delete inst;
}

void MockKernel::ShutdownRuntime() {
  shutdown_flag_.store(true);
  if (host_api_ && host_api_->log_info) {
    host_api_->log_info("[MockKernel] shutdown_runtime called");
  }
  // 清理所有残留插件实例
  std::lock_guard<std::mutex> lk(plugins_mu_);
  for (auto* inst : plugins_) {
    delete inst;
  }
  plugins_.clear();
}

}  // namespace mock_kernel

// ============================================================================
//  C ABI 导出层
// ============================================================================

// 静态接口表：首次调用 get_cuckoo_kernel_interface 时填充一次。
static CuckooKernelInterface g_mock_kernel_interface = {};
static bool g_interface_initialized = false;

static int mock_init_runtime(const CuckooHostAPI* api) {
  return mock_kernel::MockKernel::Instance().InitRuntime(api);
}

static void mock_run_loop(const CuckooHostAPI* api) {
  mock_kernel::MockKernel::Instance().RunLoop(api);
}

static int mock_load_plugin(const char* plugin_path, const char* manifest_json,
                            CuckooPluginDescriptor* out_descriptor) {
  return mock_kernel::MockKernel::Instance().LoadPlugin(plugin_path,
                                                         manifest_json,
                                                         out_descriptor);
}

static void mock_trigger_callback(CuckooPluginHandle handle,
                                  const CuckooListenerRecord* listener,
                                  const char* payload) {
  mock_kernel::MockKernel::Instance().TriggerCallback(handle, listener, payload);
}

static void mock_unload_plugin(CuckooPluginHandle handle) {
  mock_kernel::MockKernel::Instance().UnloadPlugin(handle);
}

static void mock_shutdown_runtime(void) {
  mock_kernel::MockKernel::Instance().ShutdownRuntime();
}

#ifdef __cplusplus
extern "C" {
#endif

// 导出入口：sandbox_host.c 通过 GetProcAddress 加载此函数
__declspec(dllexport) CuckooKernelInterface* get_cuckoo_kernel_interface(void) {
  if (!g_interface_initialized) {
    g_mock_kernel_interface.kernel_id = mock_kernel::MockKernel::KernelID();
    g_mock_kernel_interface.kernel_name = mock_kernel::MockKernel::KernelName();
    g_mock_kernel_interface.version = mock_kernel::MockKernel::Version();
    g_mock_kernel_interface.runtime = mock_kernel::MockKernel::Runtime();
    g_mock_kernel_interface.init_runtime = mock_init_runtime;
    g_mock_kernel_interface.run_loop = mock_run_loop;
    g_mock_kernel_interface.load_plugin = mock_load_plugin;
    g_mock_kernel_interface.trigger_callback = mock_trigger_callback;
    g_mock_kernel_interface.unload_plugin = mock_unload_plugin;
    g_mock_kernel_interface.shutdown_runtime = mock_shutdown_runtime;
    g_interface_initialized = true;
  }
  return &g_mock_kernel_interface;
}

#ifdef __cplusplus
}
#endif
