// python_kernel.cpp —— PythonKernel 实现
//
// 通过嵌入 CPython 解释器，加载并执行 Python 插件。
// 所有 Python API 调用都在沙箱工作线程上同步执行（GIL 由该线程持有）。

#include "python_kernel.h"
#include "json_utils.h"

#include <windows.h>

#include <cstring>
#include <thread>

namespace python_kernel {

// 内核元数据常量
static const char* kKernelID = "com.cuckoo.kernel.python";
static const char* kKernelName = "Python Kernel";
static const char* kVersion = "1.0.0";
static const char* kRuntime = "python";

// ---------------------------------------------------------------------------
// 静态 C 回调：将宿主 API 暴露给 Python 层
// 这些函数通过 cuckoo_sdk._xxx_impl 变量注入到 SDK 模块中。
// ---------------------------------------------------------------------------
static PythonKernel* g_kernel = nullptr;

static PyObject* PyEmitEvent(PyObject* /*self*/, PyObject* args) {
  const char* event_name = nullptr;
  const char* payload = nullptr;
  if (!PyArg_ParseTuple(args, "ss", &event_name, &payload)) return nullptr;
  if (g_kernel && g_kernel->HostAPI() && g_kernel->HostAPI()->emit_event) {
    g_kernel->HostAPI()->emit_event(event_name, payload);
  }
  Py_RETURN_NONE;
}

static PyObject* PyLogInfo(PyObject* /*self*/, PyObject* args) {
  const char* msg = nullptr;
  if (!PyArg_ParseTuple(args, "s", &msg)) return nullptr;
  if (g_kernel && g_kernel->HostAPI() && g_kernel->HostAPI()->log_info) {
    g_kernel->HostAPI()->log_info(msg);
  }
  Py_RETURN_NONE;
}

static PyObject* PyLogError(PyObject* /*self*/, PyObject* args) {
  const char* msg = nullptr;
  if (!PyArg_ParseTuple(args, "s", &msg)) return nullptr;
  if (g_kernel && g_kernel->HostAPI() && g_kernel->HostAPI()->log_error) {
    g_kernel->HostAPI()->log_error(msg);
  }
  Py_RETURN_NONE;
}

static PyObject* PyGetPluginConfig(PyObject* /*self*/, PyObject* args) {
  const char* plugin_name = nullptr;
  if (!PyArg_ParseTuple(args, "s", &plugin_name)) return nullptr;
  const char* raw = "";
  if (g_kernel && g_kernel->HostAPI() &&
      g_kernel->HostAPI()->get_plugin_config) {
    raw = g_kernel->HostAPI()->get_plugin_config(plugin_name);
  }
  return PyUnicode_FromString(raw ? raw : "");
}

static PyMethodDef kEmitEventDef = {"_impl", PyEmitEvent, METH_VARARGS,
                                    "emit_event host callback"};
static PyMethodDef kLogInfoDef = {"_impl", PyLogInfo, METH_VARARGS,
                                  "log_info host callback"};
static PyMethodDef kLogErrorDef = {"_impl", PyLogError, METH_VARARGS,
                                   "log_error host callback"};
static PyMethodDef kGetPluginConfigDef = {
    "_impl", PyGetPluginConfig, METH_VARARGS, "get_plugin_config host callback"};

// ---------------------------------------------------------------------------
// PythonKernel 实现
// ---------------------------------------------------------------------------

PythonKernel& PythonKernel::Instance() {
  static PythonKernel inst;
  return inst;
}

const char* PythonKernel::KernelID() { return kKernelID; }
const char* PythonKernel::KernelName() { return kKernelName; }
const char* PythonKernel::Version() { return kVersion; }
const char* PythonKernel::Runtime() { return kRuntime; }

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
    if (pos != std::string::npos) {
      return p.substr(0, pos);
    }
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

  if (host_api_ && host_api_->log_error) {
    host_api_->log_error(msg.c_str());
  }

  Py_XDECREF(type);
  Py_XDECREF(value);
  Py_XDECREF(traceback);
}

bool PythonKernel::InjectHostAPIIntoSDK() {
  if (!sdk_module_) return false;

  // 将 C 函数包装为 Python 可调用对象，注入到 cuckoo_sdk._xxx_impl
  PyObject* emit = PyCFunction_New(&kEmitEventDef, nullptr);
  PyObject* log_info = PyCFunction_New(&kLogInfoDef, nullptr);
  PyObject* log_error = PyCFunction_New(&kLogErrorDef, nullptr);
  PyObject* get_config = PyCFunction_New(&kGetPluginConfigDef, nullptr);

  if (!emit || !log_info || !log_error || !get_config) {
    Py_XDECREF(emit);
    Py_XDECREF(log_info);
    Py_XDECREF(log_error);
    Py_XDECREF(get_config);
    LogPythonException("create host API wrappers");
    return false;
  }

  PyObject_SetAttrString(sdk_module_, "_emit_event_impl", emit);
  PyObject_SetAttrString(sdk_module_, "_log_info_impl", log_info);
  PyObject_SetAttrString(sdk_module_, "_log_error_impl", log_error);
  PyObject_SetAttrString(sdk_module_, "_get_plugin_config_impl", get_config);

  Py_DECREF(emit);
  Py_DECREF(log_info);
  Py_DECREF(log_error);
  Py_DECREF(get_config);
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

PyObject* PythonKernel::ImportPluginModule(const std::string& plugin_path,
                                           const std::string& module_name) {
  // 使用 importlib.util.spec_from_file_location 从指定路径加载模块
  PyObject* importlib = PyImport_ImportModule("importlib.util");
  if (!importlib) {
    LogPythonException("import importlib.util");
    return nullptr;
  }

  std::string file_path = plugin_path + "/main.py";

  PyObject* spec = PyObject_CallMethod(importlib, "spec_from_file_location",
                                       "ss", module_name.c_str(),
                                       file_path.c_str());
  Py_DECREF(importlib);
  if (!spec) {
    LogPythonException("spec_from_file_location");
    return nullptr;
  }

  PyObject* module =
      PyObject_CallMethod(importlib, "module_from_spec", "O", spec);
  if (!module) {
    LogPythonException("module_from_spec");
    Py_DECREF(spec);
    return nullptr;
  }

  PyObject* loader = PyObject_GetAttrString(spec, "loader");
  Py_DECREF(spec);
  if (!loader) {
    LogPythonException("get loader");
    Py_DECREF(module);
    return nullptr;
  }

  PyObject* result =
      PyObject_CallMethod(loader, "exec_module", "O", module);
  Py_DECREF(loader);
  if (!result) {
    LogPythonException("exec_module for " + file_path);
    Py_DECREF(module);
    return nullptr;
  }
  Py_DECREF(result);

  return module;  // 强引用，调用方负责释放
}

bool PythonKernel::FillListenersFromSDK(CuckooPluginDescriptor* out_descriptor,
                                         PythonPluginInstance* inst) {
  PyObject* listeners = PyObject_GetAttrString(sdk_module_, "_listeners");
  if (!listeners || !PyList_Check(listeners)) {
    Py_XDECREF(listeners);
    if (host_api_ && host_api_->log_error) {
      host_api_->log_error("[PythonKernel] cuckoo_sdk._listeners not found");
    }
    return false;
  }

  Py_ssize_t count = PyList_Size(listeners);
  int filled = 0;
  for (Py_ssize_t i = 0; i < count && filled < MAX_LISTENERS_PER_PLUGIN; ++i) {
    PyObject* item = PyList_GetItem(listeners, i);  // 借引用
    if (!item || !PyTuple_Check(item) || PyTuple_Size(item) != 2) continue;

    PyObject* name_obj = PyTuple_GetItem(item, 0);  // 借引用
    PyObject* func_obj = PyTuple_GetItem(item, 1);  // 借引用
    if (!name_obj || !func_obj) continue;

    const char* event_name = PyUnicode_AsUTF8(name_obj);
    if (!event_name) continue;

    // 创建 PythonCallback，存储在实例中
    auto* cb = new PythonCallback{func_obj, event_name};
    inst->callbacks.push_back(cb);

    // 填充 CuckooListenerRecord
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

int PythonKernel::InitRuntime(const CuckooHostAPI* api) {
  host_api_ = api;
  g_kernel = this;

  if (python_initialized_) return 0;

  // 初始化 Python 解释器
  Py_Initialize();
  if (!Py_IsInitialized()) {
    if (host_api_ && host_api_->log_error) {
      host_api_->log_error("[PythonKernel] Py_Initialize failed");
    }
    return -1;
  }
  python_initialized_ = true;

  // 将 SDK 目录加入 sys.path
  // DLL 位于 <kernel_dir>/binary/，SDK 位于 <kernel_dir>/sdk/
  std::string binary_dir = GetKernelDllDir();
  std::string sdk_dir = binary_dir + "/../sdk";
  char resolved[MAX_PATH];
  if (GetFullPathNameA(sdk_dir.c_str(), MAX_PATH, resolved, nullptr)) {
    sdk_dir = resolved;
  }
  if (host_api_ && host_api_->log_info) {
    std::string dbg = "[PythonKernel] binary_dir=" + binary_dir +
                      " sdk_dir=" + sdk_dir;
    host_api_->log_info(dbg.c_str());
  }
  PyObject* sys_path = PySys_GetObject("path");  // 借引用
  if (sys_path) {
    PyObject* sdk_path = PyUnicode_FromString(sdk_dir.c_str());
    if (sdk_path) {
      PyList_Insert(sys_path, 0, sdk_path);
      Py_DECREF(sdk_path);
    }
  }

  // 导入 cuckoo_sdk
  sdk_module_ = PyImport_ImportModule("cuckoo_sdk");
  if (!sdk_module_) {
    LogPythonException("import cuckoo_sdk");
    return -1;
  }

  // 注入宿主 API
  if (!InjectHostAPIIntoSDK()) {
    return -1;
  }

  if (host_api_ && host_api_->log_info) {
    host_api_->log_info("[PythonKernel] init_runtime called, Python ready");
  }
  return 0;
}

void PythonKernel::RunLoop(const CuckooHostAPI* api) {
  if (api && api->log_info) {
    api->log_info("[PythonKernel] run_loop starting");
  }
  // 通知宿主已进入循环
  if (api && api->into_loop_report) {
    api->into_loop_report(kKernelID);
  }
  // Python 回调由 trigger_callback 同步调用，无需后台事件循环
  while (!shutdown_flag_.load()) {
    std::this_thread::sleep_for(std::chrono::milliseconds(100));
  }
  if (api && api->log_info) {
    api->log_info("[PythonKernel] run_loop exiting");
  }
}

int PythonKernel::LoadPlugin(const char* plugin_path,
                              const char* manifest_json,
                              CuckooPluginDescriptor* out_descriptor) {
  if (!plugin_path || !manifest_json || !out_descriptor) return -1;

  std::string manifest(manifest_json);
  std::string plugin_id, runtime, name;
  if (!mock_json::GetStringField(manifest, "id", plugin_id) ||
      !mock_json::GetStringField(manifest, "runtime_type", runtime)) {
    if (host_api_ && host_api_->log_error) {
      host_api_->log_error("[PythonKernel] plugin manifest missing id/runtime_type");
    }
    return -1;
  }
  mock_json::GetStringField(manifest, "name", name);

  // 生成唯一模块名，避免多插件 main.py 命名冲突
  std::string module_name = "cuckoo_plugin_" + plugin_id;
  // 替换非法字符
  for (auto& c : module_name) {
    if (c == '.' || c == '-') c = '_';
  }

  // 清空 SDK 监听器注册表
  ClearSDKListeners();

  // 导入插件模块
  PyObject* module = ImportPluginModule(plugin_path, module_name);
  if (!module) {
    return -1;
  }

  // 创建插件实例
  auto* inst = new PythonPluginInstance();
  inst->instance_id = NextInstanceID();
  inst->plugin_id = plugin_id;
  inst->language_runtime = runtime;
  inst->plugin_path = plugin_path;
  inst->module = module;

  // 从 SDK 读取监听器并填充描述符
  std::memset(out_descriptor, 0, sizeof(CuckooPluginDescriptor));
  out_descriptor->instance_id = inst->instance_id;
  std::strncpy(out_descriptor->plugin_id, plugin_id.c_str(), 63);
  out_descriptor->plugin_id[63] = '\0';
  std::strncpy(out_descriptor->language_runtime, runtime.c_str(), 31);
  out_descriptor->language_runtime[31] = '\0';

  if (!FillListenersFromSDK(out_descriptor, inst)) {
    delete inst;
    Py_DECREF(module);
    return -1;
  }

  // 从 manifest 解析提供的事件列表
  std::vector<std::string> events;
  mock_json::GetStringArrayField(manifest, "events", events);
  int ec = 0;
  for (const auto& ev : events) {
    if (ec >= MAX_EVENTS_PER_PLUGIN) break;
    std::strncpy(out_descriptor->provided_events[ec].event_name, ev.c_str(), 127);
    out_descriptor->provided_events[ec].event_name[127] = (char)0;
    ++ec;
  }
  out_descriptor->event_count = ec;

  out_descriptor->internal_object_ptr = static_cast<void*>(inst);

  {
    std::lock_guard<std::mutex> lk(plugins_mu_);
    plugins_.push_back(inst);
  }

  if (host_api_ && host_api_->log_info) {
    std::string msg = "[PythonKernel] loaded plugin: " + plugin_id +
                      " (runtime=" + runtime +
                      ", listeners=" +
                      std::to_string(out_descriptor->listener_count) +
                      ", events=" + std::to_string(ec) + ")";
    host_api_->log_info(msg.c_str());
  }

  return 0;
}

void PythonKernel::TriggerCallback(CuckooPluginHandle handle,
                                    const CuckooListenerRecord* listener,
                                    const char* payload) {
  if (!handle || !listener) return;

  auto* cb = static_cast<PythonCallback*>(listener->function);
  if (!cb || !cb->callable) return;

  // 构造参数元组 (payload,)
  PyObject* args = PyTuple_Pack(1, PyUnicode_FromString(payload ? payload : ""));
  if (!args) {
    LogPythonException("build callback args");
    return;
  }

  PyObject* result = PyObject_CallObject(cb->callable, args);
  Py_DECREF(args);

  if (!result) {
    LogPythonException("callback for " + cb->event_name);
    return;
  }
  Py_DECREF(result);
}

void PythonKernel::UnloadPlugin(CuckooPluginHandle handle) {
  if (!handle) return;
  auto* inst = static_cast<PythonPluginInstance*>(handle);

  // 清理回调对象
  for (auto* cb : inst->callbacks) {
    delete cb;
  }
  inst->callbacks.clear();

  // 释放模块
  if (inst->module) {
    // 从 sys.modules 中移除，避免下次导入时复用旧模块
    PyObject* sys_modules = PyImport_GetModuleDict();
    if (sys_modules) {
      std::string mod_name = "cuckoo_plugin_" + inst->plugin_id;
      for (auto& c : mod_name) {
        if (c == '.' || c == '-') c = '_';
      }
      PyDict_DelItemString(sys_modules, mod_name.c_str());
    }
    Py_DECREF(inst->module);
    inst->module = nullptr;
  }

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
    std::string msg = "[PythonKernel] unloaded plugin: " + inst->plugin_id;
    host_api_->log_info(msg.c_str());
  }

  delete inst;
}

void PythonKernel::ShutdownRuntime() {
  shutdown_flag_.store(true);

  // 卸载所有插件
  std::vector<PythonPluginInstance*> to_delete;
  {
    std::lock_guard<std::mutex> lk(plugins_mu_);
    to_delete.swap(plugins_);
  }
  for (auto* inst : to_delete) {
    for (auto* cb : inst->callbacks) delete cb;
    if (inst->module) Py_DECREF(inst->module);
    delete inst;
  }

  // 释放 SDK 模块
  Py_XDECREF(sdk_module_);
  sdk_module_ = nullptr;

  if (python_initialized_) {
    Py_Finalize();
    python_initialized_ = false;
  }

  if (host_api_ && host_api_->log_info) {
    host_api_->log_info("[PythonKernel] shutdown_runtime called");
  }
  host_api_ = nullptr;
  g_kernel = nullptr;
}

}  // namespace python_kernel

// ============================================================================
//  C ABI 导出层
// ============================================================================

static CuckooKernelInterface g_python_kernel_interface = {};
static bool g_interface_initialized = false;

static int py_init_runtime(const CuckooHostAPI* api) {
  return python_kernel::PythonKernel::Instance().InitRuntime(api);
}

static void py_run_loop(const CuckooHostAPI* api) {
  python_kernel::PythonKernel::Instance().RunLoop(api);
}

static int py_load_plugin(const char* plugin_path, const char* manifest_json,
                          CuckooPluginDescriptor* out_descriptor) {
  return python_kernel::PythonKernel::Instance().LoadPlugin(
      plugin_path, manifest_json, out_descriptor);
}

static void py_trigger_callback(CuckooPluginHandle handle,
                                const CuckooListenerRecord* listener,
                                const char* payload) {
  python_kernel::PythonKernel::Instance().TriggerCallback(handle, listener,
                                                           payload);
}

static void py_unload_plugin(CuckooPluginHandle handle) {
  python_kernel::PythonKernel::Instance().UnloadPlugin(handle);
}

static void py_shutdown_runtime(void) {
  python_kernel::PythonKernel::Instance().ShutdownRuntime();
}

#ifdef __cplusplus
extern "C" {
#endif

__declspec(dllexport) CuckooKernelInterface* get_cuckoo_kernel_interface(void) {
  if (!g_interface_initialized) {
    g_python_kernel_interface.kernel_id =
        python_kernel::PythonKernel::KernelID();
    g_python_kernel_interface.kernel_name =
        python_kernel::PythonKernel::KernelName();
    g_python_kernel_interface.version = python_kernel::PythonKernel::Version();
    g_python_kernel_interface.runtime = python_kernel::PythonKernel::Runtime();
    g_python_kernel_interface.init_runtime = py_init_runtime;
    g_python_kernel_interface.run_loop = py_run_loop;
    g_python_kernel_interface.load_plugin = py_load_plugin;
    g_python_kernel_interface.trigger_callback = py_trigger_callback;
    g_python_kernel_interface.unload_plugin = py_unload_plugin;
    g_python_kernel_interface.shutdown_runtime = py_shutdown_runtime;
    g_interface_initialized = true;
  }
  return &g_python_kernel_interface;
}

#ifdef __cplusplus
}
#endif
