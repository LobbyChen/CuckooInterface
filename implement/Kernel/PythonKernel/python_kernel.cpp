// Copyright (C) 2026 LobbyChen
// SPDX-License-Identifier: AGPL-3.0-only
#include "python_kernel.h"
#include "cuckoo_control_json.h"
#include "json_utils.h"

#include <windows.h>

#include <algorithm>
#include <cctype>
#include <chrono>
#include <cstring>
#include <string>
#include <thread>

namespace python_kernel {

using python_runtime::PyApi;
using python_runtime::Runtime;

static const char* kKernelID = "com.cuckoo.kernel.python";
static const char* kKernelName = "Python Kernel";
static const char* kVersion = "2.2.0";
static const char* kRuntime = "python";

static Runtime& PyRt() { return Runtime::Instance(); }
static const PyApi& Py() { return PyRt().api(); }

// ===========================================================================
// 基础工具
// ===========================================================================
class GilGuard {
 public:
  GilGuard() { state_ = Py().gil_ensure(); }
  ~GilGuard() { Py().gil_release(state_); }
  GilGuard(const GilGuard&) = delete;
  GilGuard& operator=(const GilGuard&) = delete;

 private:
  int state_;
};

// 依赖 GIL 的 PyObject 引用守卫。
class PyRef {
 public:
  explicit PyRef(PyObject* object = nullptr) : object_(object) {}
  ~PyRef() { reset(); }
  PyRef(const PyRef&) = delete;
  PyRef& operator=(const PyRef&) = delete;

  PyObject* get() const { return object_; }
  explicit operator bool() const { return object_ != nullptr; }
  PyObject* release() {
    PyObject* value = object_;
    object_ = nullptr;
    return value;
  }
  void reset(PyObject* object = nullptr) {
    if (object_ != nullptr) {
      Py().decref(object_);
    }
    object_ = object;
  }

 private:
  PyObject* object_;
};

static bool UnicodeToString(PyObject* object, std::string* out) {
  if (object == nullptr) {
    return false;
  }
  Py_ssize_t size = 0;
  const char* data = Py().unicode_as_utf8_and_size(object, &size);
  if (data == nullptr) {
    Py().err_clear();
    return false;
  }
  out->assign(data, static_cast<size_t>(size));
  return true;
}

static bool TupleString(PyObject* tuple, Py_ssize_t index, std::string* out) {
  PyObject* item = Py().tuple_get_item(tuple, index);
  return UnicodeToString(item, out);
}

// 调用单参数函数，并接管 arg 的所有权（无论成功失败）。
static PyObject* CallWithArg(PyObject* callable, PyObject* arg) {
  if (arg == nullptr) {
    return nullptr;
  }
  PyObject* args = Py().tuple_new(1);
  if (args == nullptr) {
    Py().decref(arg);
    return nullptr;
  }
  Py().tuple_set_item(args, 0, arg);
  PyObject* result = Py().object_call_object(callable, args);
  Py().decref(args);
  return result;
}

// 调用双参数函数，并接管两个参数的所有权（无论成功失败）。
static PyObject* CallWithArgs(PyObject* callable, PyObject* first,
                              PyObject* second) {
  if (first == nullptr || second == nullptr) {
    Py().decref(first);
    Py().decref(second);
    return nullptr;
  }
  PyObject* args = Py().tuple_new(2);
  if (args == nullptr) {
    Py().decref(first);
    Py().decref(second);
    return nullptr;
  }
  Py().tuple_set_item(args, 0, first);
  Py().tuple_set_item(args, 1, second);
  PyObject* result = Py().object_call_object(callable, args);
  Py().decref(args);
  return result;
}

static unsigned long LoadPyThreadID(PythonPluginInstance* inst) {
  return static_cast<unsigned long>(
      InterlockedCompareExchange(
          reinterpret_cast<volatile LONG*>(&inst->py_thread_id), 0, 0));
}

static void StorePyThreadID(PythonPluginInstance* inst, unsigned long value) {
  InterlockedExchange(reinterpret_cast<volatile LONG*>(&inst->py_thread_id),
                      static_cast<LONG>(value));
}

// ===========================================================================
// 静态 C 回调
// ===========================================================================
static PythonKernel* g_kernel = nullptr;

static PyObject* PyEmitEvent(PyObject*, PyObject* args) {
  std::string event_name;
  std::string payload;
  if (!TupleString(args, 0, &event_name) ||
      !TupleString(args, 1, &payload)) {
    Py().err_set_string(PyRt().type_error(),
                        "emit_event expects (str, str)");
    return nullptr;
  }
  if (g_kernel && g_kernel->HostAPI() && g_kernel->HostAPI()->emit_event) {
    g_kernel->HostAPI()->emit_event(event_name.c_str(), payload.c_str());
  }
  return PyRt().NewNoneRef();
}

static PyObject* PyLogInfo(PyObject*, PyObject* args) {
  std::string msg;
  if (!TupleString(args, 0, &msg)) {
    Py().err_set_string(PyRt().type_error(), "log_info expects (str)");
    return nullptr;
  }
  if (g_kernel && g_kernel->HostAPI() && g_kernel->HostAPI()->log_info) {
    g_kernel->HostAPI()->log_info(msg.c_str());
  }
  return PyRt().NewNoneRef();
}

static PyObject* PyLogError(PyObject*, PyObject* args) {
  std::string msg;
  if (!TupleString(args, 0, &msg)) {
    Py().err_set_string(PyRt().type_error(), "log_error expects (str)");
    return nullptr;
  }
  if (g_kernel && g_kernel->HostAPI() && g_kernel->HostAPI()->log_error) {
    g_kernel->HostAPI()->log_error(msg.c_str());
  }
  return PyRt().NewNoneRef();
}

static PyObject* PyGetPluginConfig(PyObject*, PyObject* args) {
  std::string plugin_name;
  if (!TupleString(args, 0, &plugin_name)) {
    Py().err_set_string(PyRt().type_error(),
                        "get_plugin_config expects (str)");
    return nullptr;
  }
  const char* raw = "";
  if (g_kernel && g_kernel->HostAPI() &&
      g_kernel->HostAPI()->get_plugin_config) {
    raw = g_kernel->HostAPI()->get_plugin_config(plugin_name.c_str());
  }
  PyObject* result = Py().unicode_from_string(raw ? raw : "");
  return result != nullptr ? result : PyRt().NewNoneRef();
}

static const PyMethodDef kEmitEventDef = {"_impl", PyEmitEvent,
                                          python_runtime::kMETHVarArgs, ""};
static const PyMethodDef kLogInfoDef = {"_impl", PyLogInfo,
                                        python_runtime::kMETHVarArgs, ""};
static const PyMethodDef kLogErrorDef = {"_impl", PyLogError,
                                         python_runtime::kMETHVarArgs, ""};
static const PyMethodDef kGetPluginConfigDef = {
    "_impl", PyGetPluginConfig, python_runtime::kMETHVarArgs, ""};

// ===========================================================================
// PythonKernel
// ===========================================================================

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
    std::string path(dll_path);
    auto pos = path.find_last_of("\\/");
    if (pos != std::string::npos) {
      return path.substr(0, pos);
    }
  }
  return ".";
}

void PythonKernel::LogPythonException(const std::string& context) {
  if (Py().err_occurred() == nullptr) {
    return;
  }
  PyObject* type = nullptr;
  PyObject* value = nullptr;
  PyObject* traceback = nullptr;
  Py().err_fetch(&type, &value, &traceback);
  Py().err_normalize_exception(&type, &value, &traceback);

  std::string msg = "[PythonKernel] " + context + ": ";
  if (value != nullptr) {
    PyObject* str = Py().object_str(value);
    if (str != nullptr) {
      std::string text;
      if (UnicodeToString(str, &text)) {
        msg += text;
      }
      Py().decref(str);
    }
  } else {
    msg += "unknown exception";
  }
  if (host_api_ && host_api_->log_error) {
    host_api_->log_error(msg.c_str());
  }

  Py().decref(type);
  Py().decref(value);
  Py().decref(traceback);
}

bool PythonKernel::InjectHostAPIIntoSDK() {
  if (sdk_module_ == nullptr) {
    return false;
  }

  PyObject* emit = Py().cfunction_new_ex(
      const_cast<PyMethodDef*>(&kEmitEventDef), nullptr, nullptr);
  PyObject* log_info = Py().cfunction_new_ex(
      const_cast<PyMethodDef*>(&kLogInfoDef), nullptr, nullptr);
  PyObject* log_error = Py().cfunction_new_ex(
      const_cast<PyMethodDef*>(&kLogErrorDef), nullptr, nullptr);
  PyObject* get_config = Py().cfunction_new_ex(
      const_cast<PyMethodDef*>(&kGetPluginConfigDef), nullptr, nullptr);

  if (emit == nullptr || log_info == nullptr || log_error == nullptr ||
      get_config == nullptr) {
    Py().decref(emit);
    Py().decref(log_info);
    Py().decref(log_error);
    Py().decref(get_config);
    LogPythonException("create host API wrappers");
    return false;
  }

  bool ok = true;
  ok &= Py().object_set_attr_string(sdk_module_, "_emit_event_impl", emit) == 0;
  ok &= Py().object_set_attr_string(sdk_module_, "_log_info_impl", log_info) == 0;
  ok &= Py().object_set_attr_string(sdk_module_, "_log_error_impl", log_error) == 0;
  ok &= Py().object_set_attr_string(sdk_module_, "_get_plugin_config_impl",
                                    get_config) == 0;

  Py().decref(emit);
  Py().decref(log_info);
  Py().decref(log_error);
  Py().decref(get_config);
  if (!ok) {
    LogPythonException("inject host API into SDK");
  }
  return ok;
}

void PythonKernel::ClearSDKListeners() {
  if (sdk_module_ == nullptr) {
    return;
  }
  PyObject* empty = Py().list_new(0);
  if (empty != nullptr) {
    Py().object_set_attr_string(sdk_module_, "_listeners", empty);
    Py().decref(empty);
  }
}

bool PythonKernel::FillListenersFromSDK(CuckooPluginDescriptor* out_descriptor,
                                        PythonPluginInstance* inst) {
  PyRef listeners(Py().object_get_attr_string(sdk_module_, "_listeners"));
  if (!listeners) {
    Py().err_clear();
    out_descriptor->listener_count = 0;
    return true;  // 纯事件源插件没有监听器，不算错误
  }

  const Py_ssize_t count = Py().list_size(listeners.get());
  if (count < 0) {
    Py().err_clear();
    out_descriptor->listener_count = 0;
    return true;
  }

  int filled = 0;
  for (Py_ssize_t i = 0; i < count && filled < MAX_LISTENERS_PER_PLUGIN; ++i) {
    PyObject* item = Py().list_get_item(listeners.get(), i);
    if (item == nullptr || Py().tuple_size(item) != 2) {
      Py().err_clear();
      continue;
    }
    PyObject* name_obj = Py().tuple_get_item(item, 0);
    PyObject* func_obj = Py().tuple_get_item(item, 1);
    if (name_obj == nullptr || func_obj == nullptr) {
      Py().err_clear();
      continue;
    }

    std::string event_name;
    if (!UnicodeToString(name_obj, &event_name)) {
      continue;
    }

    Py().incref(func_obj);  // 跨线程使用，必须持有强引用
    auto* callback = new PythonCallback{func_obj, event_name};
    inst->callbacks.push_back(callback);

    CuckooListenerRecord* record = &out_descriptor->listeners[filled];
    std::strncpy(record->event_name, event_name.c_str(), 127);
    record->event_name[127] = '\0';
    record->function = static_cast<void*>(callback);
    ++filled;
  }
  out_descriptor->listener_count = filled;
  return true;
}

// ===========================================================================
// InitRuntime
// ===========================================================================
int PythonKernel::InitRuntime(const CuckooHostAPI* api) {
  host_api_ = api;
  g_kernel = this;
  if (python_initialized_) {
    return 0;
  }

  std::string load_error;
  if (!PyRt().Load(&load_error)) {
    if (host_api_ && host_api_->log_error) {
      host_api_->log_error(("[PythonKernel] " + load_error).c_str());
    }
    return -1;
  }

  Py().initialize();
  if (!Py().is_initialized()) {
    if (host_api_ && host_api_->log_error) {
      host_api_->log_error("[PythonKernel] Py_Initialize failed");
    }
    return -1;
  }

  main_thread_state_ = Py().save_thread();
  python_initialized_ = true;

  bool sdk_ready = false;
  {
    GilGuard gil;

    std::string sdk_dir = GetKernelDllDir() + "/../sdk";
    char resolved[MAX_PATH] = {0};
    if (GetFullPathNameA(sdk_dir.c_str(), MAX_PATH, resolved, nullptr)) {
      sdk_dir = resolved;
    }
    if (host_api_ && host_api_->log_info) {
      host_api_->log_info(("[PythonKernel] sdk_dir=" + sdk_dir).c_str());
    }

    PyObject* sys_path = Py().sys_get_object("path");
    if (sys_path != nullptr) {
      PyObject* sdk_path = Py().unicode_from_string(sdk_dir.c_str());
      if (sdk_path != nullptr) {
        Py().list_insert(sys_path, 0, sdk_path);
        Py().decref(sdk_path);
      }
    }

    sdk_module_ = Py().import_import_module("cuckoo_sdk");
    if (sdk_module_ == nullptr) {
      LogPythonException("import cuckoo_sdk");
    } else if (!InjectHostAPIIntoSDK()) {
      Py().decref(sdk_module_);
      sdk_module_ = nullptr;
    } else {
      sdk_ready = true;
    }

    // 让 Python 扩展模块能够加载运行时目录中的依赖 DLL。
    const std::string home = PyRt().home();
    if (!home.empty()) {
      PyRef os_module(Py().import_import_module("os"));
      if (os_module) {
        PyRef add_dir(
            Py().object_get_attr_string(os_module.get(), "add_dll_directory"));
        if (add_dir) {
          PyObject* result =
              CallWithArg(add_dir.get(), Py().unicode_from_string(home.c_str()));
          if (result == nullptr) {
            Py().err_clear();
          } else {
            Py().decref(result);
          }
        } else {
          Py().err_clear();
        }
      }
    }
  }

  if (!sdk_ready) {
    Py().restore_thread(main_thread_state_);
    main_thread_state_ = nullptr;
    Py().finalize();
    python_initialized_ = false;
    return -1;
  }

  if (host_api_ && host_api_->log_info) {
    std::string msg = "[PythonKernel] Python ready: " + PyRt().version() +
                      " (source=" + PyRt().source() +
                      ", home=" + PyRt().home() + ")";
    host_api_->log_info(msg.c_str());
  }
  return 0;
}

// ===========================================================================
// RunLoop
// ===========================================================================
void PythonKernel::RunLoop(const CuckooHostAPI* api) {
  if (api && api->log_info) {
    api->log_info("[PythonKernel] run_loop starting");
  }
  if (api && api->into_loop_report) {
    api->into_loop_report(kKernelID);
  }
  while (!shutdown_flag_.load()) {
    std::this_thread::sleep_for(std::chrono::milliseconds(100));
  }
  if (api && api->log_info) {
    api->log_info("[PythonKernel] run_loop exiting");
  }
}

// ===========================================================================
// LoadPlugin
// ===========================================================================
int PythonKernel::LoadPlugin(const char* plugin_path, const char* manifest_json,
                             CuckooPluginDescriptor* out_descriptor) {
  if (!plugin_path || !manifest_json || !out_descriptor) {
    return -1;
  }

  std::string manifest(manifest_json);
  std::string plugin_id;
  std::string runtime;
  std::string name;
  if (!mock_json::GetStringField(manifest, "id", plugin_id) ||
      !mock_json::GetStringField(manifest, "runtime_type", runtime)) {
    if (host_api_ && host_api_->log_error) {
      host_api_->log_error("[PythonKernel] manifest missing id/runtime_type");
    }
    return -1;
  }
  mock_json::GetStringField(manifest, "name", name);

  std::string module_name = "cuckoo_plugin_" + plugin_id;
  for (auto& c : module_name) {
    if (c == '.' || c == '-') {
      c = '_';
    }
  }

  auto* inst = new PythonPluginInstance();
  inst->instance_id = NextInstanceID();
  inst->plugin_id = plugin_id;
  inst->language_runtime = runtime;
  inst->plugin_path = plugin_path;

  {
    GilGuard gil;
    ClearSDKListeners();
  }

  std::string pp(plugin_path);
  std::string mn(module_name);
  inst->worker = std::thread([inst, pp, mn, this]() {
    GilGuard gil;
    StorePyThreadID(inst, Py().thread_get_ident());

    auto fail = [&](const std::string& context) {
      LogPythonException(context);
      if (inst->module != nullptr) {
        Py().decref(inst->module);
        inst->module = nullptr;
      }
      inst->exec_failed.store(true);
    };

    const std::string file_path = pp + "/main.py";

    PyRef importlib(Py().import_import_module("importlib.util"));
    if (!importlib) {
      fail("import importlib.util");
      return;
    }

    PyRef spec_fn(
        Py().object_get_attr_string(importlib.get(), "spec_from_file_location"));
    PyRef module_from_spec(
        Py().object_get_attr_string(importlib.get(), "module_from_spec"));
    if (!spec_fn || !module_from_spec) {
      fail("resolve importlib.util functions");
      return;
    }

    PyObject* spec = CallWithArgs(spec_fn.get(),
                                  Py().unicode_from_string(mn.c_str()),
                                  Py().unicode_from_string(file_path.c_str()));
    if (spec == nullptr) {
      fail("spec_from_file_location " + file_path);
      return;
    }

    PyRef spec_ref(spec);
    PyRef loader(Py().object_get_attr_string(spec_ref.get(), "loader"));
    if (!loader) {
      fail("get spec.loader");
      return;
    }

    PyObject* module = CallWithArg(module_from_spec.get(), spec_ref.release());
    if (module == nullptr) {
      fail("module_from_spec");
      return;
    }

    // 先保存 module，即使 exec_module 阻塞也能被 UnloadPlugin 清理。
    Py().incref(module);
    inst->module = module;

    PyRef exec_module(Py().object_get_attr_string(loader.get(), "exec_module"));
    if (!exec_module) {
      fail("get loader.exec_module");
      return;
    }

    PyObject* result = CallWithArg(exec_module.get(), module);
    if (result == nullptr) {
      if (Py().err_exception_matches(PyRt().system_exit())) {
        // UnloadPlugin/ShutdownRuntime 注入的 SystemExit 属于正常退出路径。
        Py().err_clear();
      } else {
        fail("exec_module " + file_path);
      }
      return;
    }
    Py().decref(result);
    inst->exec_done.store(true);
  });

  // 轮询等待 @on_event 注册完成（最多 3 秒）。
  for (int i = 0; i < 300; ++i) {
    std::this_thread::sleep_for(std::chrono::milliseconds(10));

    if (inst->exec_failed.load()) {
      if (inst->worker.joinable()) {
        inst->worker.join();
      }
      delete inst;
      return -1;
    }
    if (inst->exec_done.load()) {
      break;
    }

    {
      GilGuard gil;
      PyObject* listeners =
          Py().object_get_attr_string(sdk_module_, "_listeners");
      if (listeners != nullptr) {
        const Py_ssize_t count = Py().list_size(listeners);
        Py().decref(listeners);
        if (count > 0) {
          break;
        }
        if (count < 0) {
          Py().err_clear();
        }
      } else {
        Py().err_clear();
      }
    }
  }

  {
    GilGuard gil;
    std::memset(out_descriptor, 0, sizeof(CuckooPluginDescriptor));
    out_descriptor->instance_id = inst->instance_id;
    std::strncpy(out_descriptor->plugin_id, plugin_id.c_str(), 63);
    std::strncpy(out_descriptor->language_runtime, runtime.c_str(), 31);
    out_descriptor->plugin_id[63] = '\0';
    out_descriptor->language_runtime[31] = '\0';

    FillListenersFromSDK(out_descriptor, inst);

    std::vector<std::string> events;
    mock_json::GetStringArrayField(manifest, "events", events);
    int event_count = 0;
    for (const auto& event : events) {
      if (event_count >= MAX_EVENTS_PER_PLUGIN) {
        break;
      }
      std::strncpy(out_descriptor->provided_events[event_count].event_name,
                   event.c_str(), 127);
      out_descriptor->provided_events[event_count].event_name[127] = '\0';
      ++event_count;
    }
    out_descriptor->event_count = event_count;
  }

  out_descriptor->internal_object_ptr = static_cast<void*>(inst);
  {
    std::lock_guard<std::mutex> lock(plugins_mu_);
    plugins_.push_back(inst);
  }

  if (host_api_ && host_api_->log_info) {
    std::string msg = "[PythonKernel] loaded: " + plugin_id +
                      " listeners=" +
                      std::to_string(out_descriptor->listener_count) +
                      " events=" + std::to_string(out_descriptor->event_count);
    host_api_->log_info(msg.c_str());
  }
  return 0;
}

// ===========================================================================
// TriggerCallback
// ===========================================================================
void PythonKernel::TriggerCallback(CuckooPluginHandle handle,
                                   const CuckooListenerRecord* listener,
                                   const char* payload) {
  if (!handle || !listener) {
    return;
  }
  auto* callback = static_cast<PythonCallback*>(listener->function);
  if (!callback || callback->callable == nullptr) {
    return;
  }

  GilGuard gil;
  if (log_callbacks_.load() && host_api_ && host_api_->log_info) {
    std::string msg = "[PythonKernel] callback " + callback->event_name;
    host_api_->log_info(msg.c_str());
  }

  PyObject* args = Py().tuple_new(1);
  if (args == nullptr) {
    LogPythonException("build callback args");
    return;
  }
  PyObject* payload_obj = Py().unicode_from_string(payload ? payload : "");
  if (payload_obj == nullptr) {
    Py().decref(args);
    LogPythonException("build callback payload");
    return;
  }
  Py().tuple_set_item(args, 0, payload_obj);
  PyObject* result = Py().object_call_object(callback->callable, args);
  Py().decref(args);

  if (result == nullptr) {
    LogPythonException("callback " + callback->event_name);
    return;
  }
  Py().decref(result);
}

// ===========================================================================
// UnloadPlugin / ShutdownRuntime
// ===========================================================================
static int StopPythonWorker(PythonPluginInstance* inst,
                            const CuckooHostAPI* host_api,
                            const char* reason) {
  if (!inst || !inst->worker.joinable()) {
    return 1;
  }

  int result = 0;
  const unsigned long thread_id = LoadPyThreadID(inst);

  if (thread_id != 0) {
    {
      GilGuard gil;
      result = Py().thread_state_set_async_exc(thread_id,
                                               PyRt().system_exit());
      if (result > 1) {
        Py().thread_state_set_async_exc(thread_id, nullptr);
        result = -1;
      }
    }
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

  // 不要持有 GIL join：目标线程需要重新取得 GIL 才能处理异步异常。
  inst->worker.join();
  StorePyThreadID(inst, 0);
  return result;
}

void PythonKernel::UnloadPlugin(CuckooPluginHandle handle) {
  if (!handle) {
    return;
  }
  auto* inst = static_cast<PythonPluginInstance*>(handle);

  StopPythonWorker(inst, host_api_, "plugin unload");

  {
    GilGuard gil;
    for (auto* callback : inst->callbacks) {
      Py().decref(callback->callable);
      delete callback;
    }
    inst->callbacks.clear();

    if (inst->module != nullptr) {
      PyObject* sys_modules = Py().import_get_module_dict();
      if (sys_modules != nullptr) {
        std::string module_name = "cuckoo_plugin_" + inst->plugin_id;
        for (auto& c : module_name) {
          if (c == '.' || c == '-') {
            c = '_';
          }
        }
        Py().dict_del_item_string(sys_modules, module_name.c_str());
      }
      Py().decref(inst->module);
      inst->module = nullptr;
    }
  }

  {
    std::lock_guard<std::mutex> lock(plugins_mu_);
    for (auto it = plugins_.begin(); it != plugins_.end(); ++it) {
      if (*it == inst) {
        plugins_.erase(it);
        break;
      }
    }
  }

  if (host_api_ && host_api_->log_info) {
    host_api_->log_info(
        ("[PythonKernel] unloaded: " + inst->plugin_id).c_str());
  }
  delete inst;
}

void PythonKernel::ShutdownRuntime() {
  shutdown_flag_.store(true);

  std::vector<PythonPluginInstance*> to_delete;
  {
    std::lock_guard<std::mutex> lock(plugins_mu_);
    to_delete.swap(plugins_);
  }
  for (auto* inst : to_delete) {
    StopPythonWorker(inst, host_api_, "runtime shutdown");
    {
      GilGuard gil;
      for (auto* callback : inst->callbacks) {
        Py().decref(callback->callable);
        delete callback;
      }
      inst->callbacks.clear();
      if (inst->module != nullptr) {
        Py().decref(inst->module);
        inst->module = nullptr;
      }
    }
    delete inst;
  }

  if (python_initialized_) {
    Py().restore_thread(main_thread_state_);
    main_thread_state_ = nullptr;
    if (sdk_module_ != nullptr) {
      Py().decref(sdk_module_);
      sdk_module_ = nullptr;
    }
    Py().finalize();
    python_initialized_ = false;
  }

  if (host_api_ && host_api_->log_info) {
    host_api_->log_info("[PythonKernel] shutdown_runtime done");
  }
  host_api_ = nullptr;
  g_kernel = nullptr;
}

// ===========================================================================
// Runtime Control Plane
// ===========================================================================
size_t PythonKernel::PluginCount() {
  std::lock_guard<std::mutex> lock(plugins_mu_);
  return plugins_.size();
}

std::string PythonKernel::BuildRuntimeStateJson() {
  std::string state = python_initialized_ ? "ready" : "stopped";
  const std::string last_error = PyRt().load_error();

  std::string json = "{\"state\":" + cuckoo_control::Quote(state);
  json += ",\"runtime\":\"python\"";
  json += ",\"version\":" + cuckoo_control::Quote(PyRt().version());
  json += ",\"home\":" + cuckoo_control::Quote(PyRt().home());
  json += ",\"dll\":" + cuckoo_control::Quote(PyRt().dll_path());
  json += ",\"source\":" + cuckoo_control::Quote(PyRt().source());
  json += ",\"embedded\":";
  json += cuckoo_control::Bool(PyRt().embedded());
  json += ",\"pluginCount\":" + std::to_string(PluginCount());
  json += ",\"logCallbacks\":";
  json += cuckoo_control::Bool(log_callbacks_.load());
  if (!last_error.empty()) {
    json += ",\"lastError\":" + cuckoo_control::Quote(last_error);
  }
  json += "}";
  return json;
}

std::string PythonKernel::BuildSettingsPanelJson() {
  const std::string version =
      PyRt().version().empty() ? "未加载" : PyRt().version();
  const std::string home =
      PyRt().home().empty() ? "尚未确定" : PyRt().home();
  std::string source = "未加载";
  if (PyRt().source() == "system") {
    source = "系统安装";
  } else if (PyRt().source() == "config") {
    source = "手动选择";
  }

  std::string settings;
  settings += cuckoo_control::TextSetting(
      "runtime.version", "Python 版本",
      "Kernel 实际加载的 CPython 版本，随内嵌运行时或系统安装变化。", version);
  settings += ",";
  settings += cuckoo_control::TextSetting(
      "runtime.home", "运行时目录", "CPython 标准库根目录。", home);
  settings += ",";
  settings += cuckoo_control::TextSetting(
      "runtime.source", "运行时来源",
      "system = 自动探测的系统安装，config = 用户在运行时页面手动选择。", source);
  settings += ",";
  settings += cuckoo_control::SwitchSetting(
      "log.callbacks", "记录事件回调日志",
      "每次触发插件事件回调时写入一条 INFO 日志，便于排查问题。",
      log_callbacks_.load());

  std::string json = "{\"pages\":[{\"key\":\"python-runtime\",";
  json += "\"name\":\"Python 运行时\",\"sections\":[";
  json += "{\"key\":\"runtime-info\",\"name\":\"运行时信息\",\"settings\":[";
  json += settings;
  json += "]}]}]}";
  return json;
}

std::string PythonKernel::BuildOperationsJson() {
  return "[{\"id\":\"runtime.probe\",\"name\":\"重新探测运行时\","
         "\"description\":\"返回当前 Python 版本、运行时目录与插件数量。\"},"
         "{\"id\":\"plugins.list\",\"name\":\"列出插件\","
         "\"description\":\"返回当前 Python Kernel 托管的插件 ID。\"}]";
}

std::string PythonKernel::BuildPluginListJson() {
  std::vector<std::string> ids;
  {
    std::lock_guard<std::mutex> lock(plugins_mu_);
    ids.reserve(plugins_.size());
    for (const auto* plugin : plugins_) {
      ids.push_back(plugin->plugin_id);
    }
  }
  return cuckoo_control::StringArray(ids);
}

int PythonKernel::GetSettingsPanel(const CuckooHostAPI*, char** out_json) {
  if (out_json == nullptr) {
    return -1;
  }
  *out_json = cuckoo_control::Alloc(BuildSettingsPanelJson());
  return *out_json != nullptr ? 0 : -1;
}

int PythonKernel::GetRuntimeState(const CuckooHostAPI*, char** out_json) {
  if (out_json == nullptr) {
    return -1;
  }
  *out_json = cuckoo_control::Alloc(BuildRuntimeStateJson());
  return *out_json != nullptr ? 0 : -1;
}

int PythonKernel::SetSetting(const CuckooHostAPI*, const char* key,
                             const char* value_json, char** out_json) {
  if (out_json == nullptr) {
    return -1;
  }
  *out_json = nullptr;
  if (key == nullptr || value_json == nullptr) {
    *out_json = cuckoo_control::AllocError("key and value are required");
    return 0;
  }

  std::string name(key);
  std::string value(value_json);
  value.erase(std::remove_if(value.begin(), value.end(), [](unsigned char c) {
                return std::isspace(c) != 0;
              }),
              value.end());

  if (name == "log.callbacks") {
    if (value != "true" && value != "false") {
      *out_json =
          cuckoo_control::AllocError("log.callbacks expects a boolean value");
      return 0;
    }
    log_callbacks_.store(value == "true");
    std::string extra = "\"key\":" + cuckoo_control::Quote(name) +
                        ",\"value\":" + cuckoo_control::Bool(log_callbacks_.load());
    *out_json = cuckoo_control::AllocOk(extra);
    return 0;
  }

  *out_json =
      cuckoo_control::AllocError("unsupported setting: " + name);
  return 0;
}

int PythonKernel::GetOperations(const CuckooHostAPI*, char** out_json) {
  if (out_json == nullptr) {
    return -1;
  }
  *out_json = cuckoo_control::Alloc(BuildOperationsJson());
  return *out_json != nullptr ? 0 : -1;
}

int PythonKernel::InvokeOperation(const CuckooHostAPI*,
                                  const char* operation_id,
                                  const char* request_json,
                                  char** out_json) {
  (void)request_json;
  if (out_json == nullptr) {
    return -1;
  }
  *out_json = nullptr;
  if (operation_id == nullptr) {
    *out_json = cuckoo_control::AllocError("operation id is required");
    return 0;
  }

  const std::string operation(operation_id);
  if (operation == "runtime.probe") {
    std::string extra = "\"state\":" + BuildRuntimeStateJson();
    *out_json = cuckoo_control::AllocOk(extra);
    return 0;
  }
  if (operation == "plugins.list") {
    std::string extra = "\"plugins\":" + BuildPluginListJson();
    extra += ",\"pluginCount\":" + std::to_string(PluginCount());
    *out_json = cuckoo_control::AllocOk(extra);
    return 0;
  }

  *out_json =
      cuckoo_control::AllocError("unsupported operation: " + operation);
  return 0;
}

}  // namespace python_kernel

// ============================================================================
// C ABI 导出层
// ============================================================================
static CuckooKernelInterface g_iface = {};
static bool g_iface_init = false;

static int c_init(const CuckooHostAPI* api) {
  return python_kernel::PythonKernel::Instance().InitRuntime(api);
}
static void c_loop(const CuckooHostAPI* api) {
  python_kernel::PythonKernel::Instance().RunLoop(api);
}
static int c_load(const char* path, const char* manifest,
                  CuckooPluginDescriptor* descriptor) {
  return python_kernel::PythonKernel::Instance().LoadPlugin(path, manifest,
                                                            descriptor);
}
static void c_trigger(CuckooPluginHandle handle,
                      const CuckooListenerRecord* listener,
                      const char* payload) {
  python_kernel::PythonKernel::Instance().TriggerCallback(handle, listener,
                                                          payload);
}
static void c_unload(CuckooPluginHandle handle) {
  python_kernel::PythonKernel::Instance().UnloadPlugin(handle);
}
static void c_shutdown(void) {
  python_kernel::PythonKernel::Instance().ShutdownRuntime();
}

static int c_get_settings_panel(const CuckooHostAPI* api, char** out_json) {
  return python_kernel::PythonKernel::Instance().GetSettingsPanel(api, out_json);
}
static int c_get_runtime_state(const CuckooHostAPI* api, char** out_json) {
  return python_kernel::PythonKernel::Instance().GetRuntimeState(api, out_json);
}
static int c_set_setting(const CuckooHostAPI* api, const char* key,
                         const char* value_json, char** out_json) {
  return python_kernel::PythonKernel::Instance().SetSetting(api, key, value_json,
                                                            out_json);
}
static int c_get_operations(const CuckooHostAPI* api, char** out_json) {
  return python_kernel::PythonKernel::Instance().GetOperations(api, out_json);
}
static int c_invoke_operation(const CuckooHostAPI* api, const char* operation_id,
                              const char* request_json, char** out_json) {
  return python_kernel::PythonKernel::Instance().InvokeOperation(
      api, operation_id, request_json, out_json);
}
static void c_free_response(char* response) {
  if (response != nullptr) {
    std::free(response);
  }
}

#ifdef __cplusplus
extern "C" {
#endif

__declspec(dllexport) CuckooKernelInterface* get_cuckoo_kernel_interface(void) {
  if (!g_iface_init) {
    g_iface.kernel_id = python_kernel::PythonKernel::KernelID();
    g_iface.kernel_name = python_kernel::PythonKernel::KernelName();
    g_iface.version = python_kernel::PythonKernel::Version();
    g_iface.runtime = python_kernel::PythonKernel::Runtime();
    g_iface.init_runtime = c_init;
    g_iface.run_loop = c_loop;
    g_iface.load_plugin = c_load;
    g_iface.trigger_callback = c_trigger;
    g_iface.unload_plugin = c_unload;
    g_iface.shutdown_runtime = c_shutdown;
    g_iface.get_settings_panel = c_get_settings_panel;
    g_iface.get_runtime_state = c_get_runtime_state;
    g_iface.set_setting = c_set_setting;
    g_iface.get_operations = c_get_operations;
    g_iface.invoke_operation = c_invoke_operation;
    g_iface.free_response = c_free_response;
    g_iface_init = true;
  }
  return &g_iface;
}

#ifdef __cplusplus
}
#endif
