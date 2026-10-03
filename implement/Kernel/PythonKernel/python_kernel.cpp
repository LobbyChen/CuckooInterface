#include "python_kernel.h"
#include "json_utils.h"

#include <windows.h>
#include <cstring>
#include <thread>
#include <chrono>

namespace python_kernel {

static const char* kKernelID   = "com.cuckoo.kernel.python";
static const char* kKernelName = "Python Kernel";
static const char* kVersion    = "2.0.0";
static const char* kRuntime    = "python";

// ===========================================================================
// RAII GIL 守卫
// ===========================================================================
class GilGuard {
 public:
  GilGuard()  { state_ = PyGILState_Ensure(); }
  ~GilGuard() { PyGILState_Release(state_); }
  GilGuard(const GilGuard&) = delete;
  GilGuard& operator=(const GilGuard&) = delete;
 private:
  PyGILState_STATE state_;
};

// py_thread_id 在 worker 线程写入、卸载线程读取。由于头文件中的字段
// 目前保持为 unsigned long，这里用 Win32 原子操作避免裸读写的数据竞争。
static unsigned long LoadPyThreadID(PythonPluginInstance* inst) {
  return static_cast<unsigned long>(
      InterlockedCompareExchange(
          reinterpret_cast<volatile LONG*>(&inst->py_thread_id), 0, 0));
}

static void StorePyThreadID(PythonPluginInstance* inst, unsigned long value) {
  InterlockedExchange(
      reinterpret_cast<volatile LONG*>(&inst->py_thread_id),
      static_cast<LONG>(value));
}

// 请求插件工作线程退出。PyThreadState_SetAsyncExc 是 Python C API，
// 调用时必须持有 GIL；但 join() 绝不能在持有 GIL 时执行。
static int StopPythonWorker(PythonPluginInstance* inst,
                            const CuckooHostAPI* host_api,
                            const char* reason) {
  if (!inst || !inst->worker.joinable()) return 1;

  int result = 0;
  const unsigned long thread_id = LoadPyThreadID(inst);

  if (thread_id != 0) {
    {
      GilGuard gil;

      result = PyThreadState_SetAsyncExc(thread_id, PyExc_SystemExit);

      // 官方 API 约定：0 表示未找到线程，1 表示成功，>1 表示
      // 错误地影响了多个线程，此时必须回滚。
      if (result > 1) {
        PyThreadState_SetAsyncExc(thread_id, nullptr);
        result = -1;
      }
    }  // 先释放 GIL，让目标线程有机会处理 SystemExit
  } else {
    // 正常情况下 worker 启动后会很快设置 py_thread_id。
    // 如果线程已经结束，则直接 join；否则 join 等待其自然结束。
    result = 0;
  }

  if (host_api && host_api->log_info) {
    std::string msg = "[PythonKernel] stopping worker";
    if (reason && *reason) {
      msg += " (";
      msg += reason;
      msg += ")";
    }
    msg += " plugin=" + inst->plugin_id;
    msg += " tid=" + std::to_string(thread_id);
    msg += " SetAsyncExc=" + std::to_string(result);
    host_api->log_info(msg.c_str());
  }

  // 绝对不要持有 GIL join。目标 Python 线程需要重新取得 GIL 才能
  // 处理异步异常并从 exec_module 返回。
  inst->worker.join();
  StorePyThreadID(inst, 0);
  return result;
}

// ===========================================================================
// 静态 C 回调
// ===========================================================================
static PythonKernel* g_kernel = nullptr;

static PyObject* PyEmitEvent(PyObject*, PyObject* args) {
  const char* event_name = nullptr;
  const char* payload    = nullptr;
  if (!PyArg_ParseTuple(args, "ss", &event_name, &payload)) return nullptr;
  if (g_kernel && g_kernel->HostAPI() && g_kernel->HostAPI()->emit_event)
    g_kernel->HostAPI()->emit_event(event_name, payload);
  Py_RETURN_NONE;
}

static PyObject* PyLogInfo(PyObject*, PyObject* args) {
  const char* msg = nullptr;
  if (!PyArg_ParseTuple(args, "s", &msg)) return nullptr;
  if (g_kernel && g_kernel->HostAPI() && g_kernel->HostAPI()->log_info)
    g_kernel->HostAPI()->log_info(msg);
  Py_RETURN_NONE;
}

static PyObject* PyLogError(PyObject*, PyObject* args) {
  const char* msg = nullptr;
  if (!PyArg_ParseTuple(args, "s", &msg)) return nullptr;
  if (g_kernel && g_kernel->HostAPI() && g_kernel->HostAPI()->log_error)
    g_kernel->HostAPI()->log_error(msg);
  Py_RETURN_NONE;
}

static PyObject* PyGetPluginConfig(PyObject*, PyObject* args) {
  const char* plugin_name = nullptr;
  if (!PyArg_ParseTuple(args, "s", &plugin_name)) return nullptr;
  const char* raw = "";
  if (g_kernel && g_kernel->HostAPI() && g_kernel->HostAPI()->get_plugin_config)
    raw = g_kernel->HostAPI()->get_plugin_config(plugin_name);
  return PyUnicode_FromString(raw ? raw : "");
}

static PyMethodDef kEmitEventDef      = {"_impl", PyEmitEvent,      METH_VARARGS, ""};
static PyMethodDef kLogInfoDef        = {"_impl", PyLogInfo,        METH_VARARGS, ""};
static PyMethodDef kLogErrorDef       = {"_impl", PyLogError,       METH_VARARGS, ""};
static PyMethodDef kGetPluginConfigDef= {"_impl", PyGetPluginConfig,METH_VARARGS, ""};

// ===========================================================================
// PythonKernel
// ===========================================================================

PythonKernel& PythonKernel::Instance() {
  static PythonKernel inst;
  return inst;
}

const char* PythonKernel::KernelID()   { return kKernelID; }
const char* PythonKernel::KernelName() { return kKernelName; }
const char* PythonKernel::Version()    { return kVersion; }
const char* PythonKernel::Runtime()    { return kRuntime; }

uint64_t PythonKernel::NextInstanceID() {
  return instance_counter_.fetch_add(1);
}

std::string PythonKernel::GetKernelDllDir() {
  HMODULE hModule = nullptr;
  GetModuleHandleExA(GET_MODULE_HANDLE_EX_FLAG_FROM_ADDRESS |
                         GET_MODULE_HANDLE_EX_FLAG_UNCHANGED_REFCOUNT,
                     reinterpret_cast<LPCSTR>(&get_cuckoo_kernel_interface),
                     &hModule);
  char dll_path[MAX_PATH] = {0};
  if (hModule && GetModuleFileNameA(hModule, dll_path, MAX_PATH)) {
    std::string p(dll_path);
    auto pos = p.find_last_of("\\/");
    if (pos != std::string::npos) return p.substr(0, pos);
  }
  return ".";
}

void PythonKernel::LogPythonException(const std::string& context) {
  if (!PyErr_Occurred()) return;
  PyObject *type, *value, *traceback;
  PyErr_Fetch(&type, &value, &traceback);
  PyErr_NormalizeException(&type, &value, &traceback);

  std::string msg = "[PythonKernel] " + context + ": ";
  if (value) {
    PyObject* str = PyObject_Str(value);
    if (str) {
      const char* s = PyUnicode_AsUTF8(str);
      if (s) msg += s;
      Py_DECREF(str);
    }
  } else {
    msg += "unknown exception";
  }
  if (host_api_ && host_api_->log_error)
    host_api_->log_error(msg.c_str());

  Py_XDECREF(type);
  Py_XDECREF(value);
  Py_XDECREF(traceback);
}

bool PythonKernel::InjectHostAPIIntoSDK() {
  if (!sdk_module_) return false;

  PyObject* emit       = PyCFunction_New(&kEmitEventDef, nullptr);
  PyObject* log_info   = PyCFunction_New(&kLogInfoDef, nullptr);
  PyObject* log_error  = PyCFunction_New(&kLogErrorDef, nullptr);
  PyObject* get_config = PyCFunction_New(&kGetPluginConfigDef, nullptr);

  if (!emit || !log_info || !log_error || !get_config) {
    Py_XDECREF(emit); Py_XDECREF(log_info);
    Py_XDECREF(log_error); Py_XDECREF(get_config);
    LogPythonException("create host API wrappers");
    return false;
  }

  PyObject_SetAttrString(sdk_module_, "_emit_event_impl", emit);
  PyObject_SetAttrString(sdk_module_, "_log_info_impl", log_info);
  PyObject_SetAttrString(sdk_module_, "_log_error_impl", log_error);
  PyObject_SetAttrString(sdk_module_, "_get_plugin_config_impl", get_config);

  Py_DECREF(emit); Py_DECREF(log_info);
  Py_DECREF(log_error); Py_DECREF(get_config);
  return true;
}

void PythonKernel::ClearSDKListeners() {
  if (!sdk_module_) return;
  PyObject* empty = PyList_New(0);
  if (empty) {
    PyObject_SetAttrString(sdk_module_, "_listeners", empty);
    Py_DECREF(empty);
  }
}

bool PythonKernel::FillListenersFromSDK(CuckooPluginDescriptor* out_descriptor,
                                         PythonPluginInstance* inst) {
  PyObject* listeners = PyObject_GetAttrString(sdk_module_, "_listeners");
  if (!listeners || !PyList_Check(listeners)) {
    Py_XDECREF(listeners);
    PyErr_Clear();
    out_descriptor->listener_count = 0;
    return true;  // 纯事件源插件没有监听器，不算错误
  }

  Py_ssize_t count = PyList_Size(listeners);
  int filled = 0;
  for (Py_ssize_t i = 0; i < count && filled < MAX_LISTENERS_PER_PLUGIN; ++i) {
    PyObject* item = PyList_GetItem(listeners, i);
    if (!item || !PyTuple_Check(item) || PyTuple_Size(item) != 2) continue;

    PyObject* name_obj = PyTuple_GetItem(item, 0);
    PyObject* func_obj = PyTuple_GetItem(item, 1);
    if (!name_obj || !func_obj) continue;

    const char* event_name = PyUnicode_AsUTF8(name_obj);
    if (!event_name) continue;

    Py_INCREF(func_obj);  //  跨线程使用，必须持有强引用
    auto* cb = new PythonCallback{func_obj, event_name};
    inst->callbacks.push_back(cb);

    CuckooListenerRecord* rec = &out_descriptor->listeners[filled];
    std::strncpy(rec->event_name, event_name, 127);
    rec->event_name[127] = '\0';
    rec->function = static_cast<void*>(cb);
    ++filled;
  }
  out_descriptor->listener_count = filled;
  Py_DECREF(listeners);
  return true;
}

// ===========================================================================
// InitRuntime —— 释放主线程 GIL
// ===========================================================================
int PythonKernel::InitRuntime(const CuckooHostAPI* api) {
  host_api_ = api;
  g_kernel  = this;
  if (python_initialized_) return 0;

  Py_Initialize();
  if (!Py_IsInitialized()) {
    if (host_api_ && host_api_->log_error)
      host_api_->log_error("[PythonKernel] Py_Initialize failed");
    return -1;
  }

  //  释放主线程 GIL，让工作线程 / Python threading 都能并发
  main_thread_state_ = PyEval_SaveThread();
  python_initialized_ = true;

  {
    GilGuard gil;  // 临时获取 GIL 完成初始化

    std::string binary_dir = GetKernelDllDir();
    std::string sdk_dir = binary_dir + "/../sdk";
    char resolved[MAX_PATH];
    if (GetFullPathNameA(sdk_dir.c_str(), MAX_PATH, resolved, nullptr))
      sdk_dir = resolved;

    if (host_api_ && host_api_->log_info) {
      std::string dbg = "[PythonKernel] sdk_dir=" + sdk_dir;
      host_api_->log_info(dbg.c_str());
    }

    PyObject* sys_path = PySys_GetObject("path");
    if (sys_path) {
      PyObject* sdk_path = PyUnicode_FromString(sdk_dir.c_str());
      if (sdk_path) { PyList_Insert(sys_path, 0, sdk_path); Py_DECREF(sdk_path); }
    }

    sdk_module_ = PyImport_ImportModule("cuckoo_sdk");
    if (!sdk_module_) { LogPythonException("import cuckoo_sdk"); return -1; }
    if (!InjectHostAPIIntoSDK()) return -1;
  }

  if (host_api_ && host_api_->log_info)
    host_api_->log_info("[PythonKernel] Python ready (top-level blocking supported)");
  return 0;
}

// ===========================================================================
// RunLoop
// ===========================================================================
void PythonKernel::RunLoop(const CuckooHostAPI* api) {
  if (api && api->log_info) api->log_info("[PythonKernel] run_loop starting");
  if (api && api->into_loop_report) api->into_loop_report(kKernelID);
  while (!shutdown_flag_.load())
    std::this_thread::sleep_for(std::chrono::milliseconds(100));
  if (api && api->log_info) api->log_info("[PythonKernel] run_loop exiting");
}

// ===========================================================================
// LoadPlugin —— exec_module 放入工作线程，轮询等待 _listeners
// ===========================================================================
int PythonKernel::LoadPlugin(const char* plugin_path,
                              const char* manifest_json,
                              CuckooPluginDescriptor* out_descriptor) {
  if (!plugin_path || !manifest_json || !out_descriptor) return -1;

  std::string manifest(manifest_json);
  std::string plugin_id, runtime, name;
  if (!mock_json::GetStringField(manifest, "id", plugin_id) ||
      !mock_json::GetStringField(manifest, "runtime_type", runtime)) {
    if (host_api_ && host_api_->log_error)
      host_api_->log_error("[PythonKernel] manifest missing id/runtime_type");
    return -1;
  }
  mock_json::GetStringField(manifest, "name", name);

  std::string module_name = "cuckoo_plugin_" + plugin_id;
  for (auto& c : module_name)
    if (c == '.' || c == '-') c = '_';

  auto* inst = new PythonPluginInstance();
  inst->instance_id      = NextInstanceID();
  inst->plugin_id        = plugin_id;
  inst->language_runtime = runtime;
  inst->plugin_path      = plugin_path;

  // 1) 清空上一轮 _listeners
  { GilGuard gil; ClearSDKListeners(); }

  // 2)  启动工作线程执行 exec_module（允许顶层阻塞）
  std::string pp(plugin_path), mn(module_name);
  inst->worker = std::thread([inst, pp, mn, this]() {
    GilGuard gil;
    StorePyThreadID(inst, PyThreadState_Get()->thread_id);

    std::string file_path = pp + "/main.py";

    PyObject* importlib = PyImport_ImportModule("importlib.util");
    if (!importlib) { LogPythonException("import importlib.util"); inst->exec_failed.store(true); return; }

    PyObject* spec = PyObject_CallMethod(importlib, "spec_from_file_location",
                                         "ss", mn.c_str(), file_path.c_str());
    if (!spec) { Py_DECREF(importlib); LogPythonException("spec"); inst->exec_failed.store(true); return; }

    PyObject* module = PyObject_CallMethod(importlib, "module_from_spec", "O", spec);
    Py_DECREF(importlib);
    if (!module) { Py_DECREF(spec); LogPythonException("module_from_spec"); inst->exec_failed.store(true); return; }

    //  先设置 inst->module，即使 exec_module 阻塞也能被 UnloadPlugin 清理
    inst->module = module;

    PyObject* loader = PyObject_GetAttrString(spec, "loader");
    Py_DECREF(spec);
    if (!loader) { LogPythonException("get loader"); inst->exec_failed.store(true); return; }

    //  执行模块 —— 如果顶层有 while True，这里永远不返回
    PyObject* result = PyObject_CallMethod(loader, "exec_module", "O", module);
    Py_DECREF(loader);

    if (!result) {
      // UnloadPlugin/ShutdownRuntime 通过 SetAsyncExc 注入 SystemExit 时，
      // 这里是正常退出路径，不应该被记录成 exec_failed。
      if (PyErr_ExceptionMatches(PyExc_SystemExit)) {
        PyErr_Clear();
      } else {
        LogPythonException("exec_module " + file_path);
        inst->exec_failed.store(true);
      }
      return;
    }
    Py_DECREF(result);
    inst->exec_done.store(true);
  });

  // 3)  轮询等待 @on_event 注册完成（最多 3 秒）
  bool ready = false;
  for (int i = 0; i < 300; ++i) {
    std::this_thread::sleep_for(std::chrono::milliseconds(10));

    if (inst->exec_failed.load()) {
      if (inst->worker.joinable()) inst->worker.join();
      delete inst;
      return -1;
    }
    if (inst->exec_done.load()) { ready = true; break; }

    {
      GilGuard gil;
      PyObject* ls = PyObject_GetAttrString(sdk_module_, "_listeners");
      if (ls && PyList_Check(ls) && PyList_Size(ls) > 0) {
        Py_DECREF(ls);
        ready = true;
        break;
      }
      Py_XDECREF(ls);
    }
  }

  // 4) 填充 descriptor
  {
    GilGuard gil;
    std::memset(out_descriptor, 0, sizeof(CuckooPluginDescriptor));
    out_descriptor->instance_id = inst->instance_id;
    std::strncpy(out_descriptor->plugin_id, plugin_id.c_str(), 63);
    std::strncpy(out_descriptor->language_runtime, runtime.c_str(), 31);

    FillListenersFromSDK(out_descriptor, inst);

    std::vector<std::string> events;
    mock_json::GetStringArrayField(manifest, "events", events);
    int ec = 0;
    for (const auto& ev : events) {
      if (ec >= MAX_EVENTS_PER_PLUGIN) break;
      std::strncpy(out_descriptor->provided_events[ec].event_name, ev.c_str(), 127);
      ++ec;
    }
    out_descriptor->event_count = ec;
  }

  out_descriptor->internal_object_ptr = static_cast<void*>(inst);
  {
    std::lock_guard<std::mutex> lk(plugins_mu_);
    plugins_.push_back(inst);
  }

  if (host_api_ && host_api_->log_info) {
    std::string msg = "[PythonKernel] loaded: " + plugin_id +
                      " listeners=" + std::to_string(out_descriptor->listener_count) +
                      " events=" + std::to_string(out_descriptor->event_count);
    host_api_->log_info(msg.c_str());
  }
  return 0;
}

// ===========================================================================
// TriggerCallback —— 直接在调用线程获取 GIL 执行
// （不投递到工作线程，因为工作线程可能被顶层阻塞循环占用）
// ===========================================================================
void PythonKernel::TriggerCallback(CuckooPluginHandle handle,
                                    const CuckooListenerRecord* listener,
                                    const char* payload) {
  if (!handle || !listener) return;
  auto* cb = static_cast<PythonCallback*>(listener->function);
  if (!cb || !cb->callable) return;

  GilGuard gil;  //  在调用线程中获取 GIL

  PyObject* args = PyTuple_Pack(1, PyUnicode_FromString(payload ? payload : ""));
  if (!args) { LogPythonException("build args"); return; }

  PyObject* result = PyObject_CallObject(cb->callable, args);
  Py_DECREF(args);

  if (!result) { LogPythonException("callback " + cb->event_name); return; }
  Py_DECREF(result);
}

// ===========================================================================
// UnloadPlugin —— 异步中断 + join
// ===========================================================================
void PythonKernel::UnloadPlugin(CuckooPluginHandle handle) {
  if (!handle) return;
  auto* inst = static_cast<PythonPluginInstance*>(handle);

  // 先请求 Python 工作线程退出；SetAsyncExc 必须在持有 GIL 时调用，
  // 但 join 必须在释放 GIL 后执行。
  StopPythonWorker(inst, host_api_, "plugin unload");

  // 清理 Python 对象（此时线程已退出，安全操作）
  {
    GilGuard gil;

    for (auto* cb : inst->callbacks) {
      Py_XDECREF(cb->callable);
      delete cb;
    }
    inst->callbacks.clear();

    if (inst->module) {
      PyObject* sys_modules = PyImport_GetModuleDict();
      if (sys_modules) {
        std::string mod_name = "cuckoo_plugin_" + inst->plugin_id;
        for (auto& c : mod_name)
          if (c == '.' || c == '-') c = '_';
        PyDict_DelItemString(sys_modules, mod_name.c_str());
      }
      Py_DECREF(inst->module);
      inst->module = nullptr;
    }
  }

  {
    std::lock_guard<std::mutex> lk(plugins_mu_);
    for (auto it = plugins_.begin(); it != plugins_.end(); ++it) {
      if (*it == inst) { plugins_.erase(it); break; }
    }
  }

  if (host_api_ && host_api_->log_info) {
    std::string msg = "[PythonKernel] unloaded: " + inst->plugin_id;
    host_api_->log_info(msg.c_str());
  }
  delete inst;
}

// ===========================================================================
// ShutdownRuntime
// ===========================================================================
void PythonKernel::ShutdownRuntime() {
  shutdown_flag_.store(true);

  std::vector<PythonPluginInstance*> to_delete;
  {
    std::lock_guard<std::mutex> lk(plugins_mu_);
    to_delete.swap(plugins_);
  }
  for (auto* inst : to_delete) {
    StopPythonWorker(inst, host_api_, "runtime shutdown");
    {
      GilGuard gil;
      for (auto* cb : inst->callbacks) { Py_XDECREF(cb->callable); delete cb; }
      if (inst->module) Py_DECREF(inst->module);
    }
    delete inst;
  }

  if (python_initialized_) {
    PyEval_RestoreThread(main_thread_state_);  //  恢复主线程 GIL
    Py_XDECREF(sdk_module_);
    sdk_module_ = nullptr;
    Py_Finalize();
    python_initialized_ = false;
    main_thread_state_ = nullptr;
  }

  if (host_api_ && host_api_->log_info)
    host_api_->log_info("[PythonKernel] shutdown_runtime done");
  host_api_ = nullptr;
  g_kernel  = nullptr;
}

}  // namespace python_kernel

// ============================================================================
//  C ABI 导出层
// ============================================================================
static CuckooKernelInterface g_iface = {};
static bool g_iface_init = false;

static int  c_init(const CuckooHostAPI* api) { return python_kernel::PythonKernel::Instance().InitRuntime(api); }
static void c_loop(const CuckooHostAPI* api) { python_kernel::PythonKernel::Instance().RunLoop(api); }
static int  c_load(const char* p, const char* m, CuckooPluginDescriptor* d) {
  return python_kernel::PythonKernel::Instance().LoadPlugin(p, m, d);
}
static void c_trigger(CuckooPluginHandle h, const CuckooListenerRecord* l, const char* p) {
  python_kernel::PythonKernel::Instance().TriggerCallback(h, l, p);
}
static void c_unload(CuckooPluginHandle h) { python_kernel::PythonKernel::Instance().UnloadPlugin(h); }
static void c_shutdown(void) { python_kernel::PythonKernel::Instance().ShutdownRuntime(); }

#ifdef __cplusplus
extern "C" {
#endif

__declspec(dllexport) CuckooKernelInterface* get_cuckoo_kernel_interface(void) {
  if (!g_iface_init) {
    g_iface.kernel_id        = python_kernel::PythonKernel::KernelID();
    g_iface.kernel_name      = python_kernel::PythonKernel::KernelName();
    g_iface.version          = python_kernel::PythonKernel::Version();
    g_iface.runtime          = python_kernel::PythonKernel::Runtime();
    g_iface.init_runtime     = c_init;
    g_iface.run_loop         = c_loop;
    g_iface.load_plugin      = c_load;
    g_iface.trigger_callback = c_trigger;
    g_iface.unload_plugin    = c_unload;
    g_iface.shutdown_runtime = c_shutdown;
    g_iface_init = true;
  }
  return &g_iface;
}

#ifdef __cplusplus
}
#endif