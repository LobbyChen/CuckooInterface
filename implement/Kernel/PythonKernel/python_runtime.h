// Copyright (C) 2026 LobbyChen
// SPDX-License-Identifier: AGPL-3.0-only
#ifndef CUCKOO_PYTHON_RUNTIME_H
#define CUCKOO_PYTHON_RUNTIME_H

#include <windows.h>

#include <cstdint>
#include <string>

/*
 * 动态加载 CPython 的稳定 ABI（python3.dll / python3XX.dll）。
 *
 * PythonKernel 不链接任何 Python 导入库，因此构建阶段不需要安装对应版本的
 * Python 开发包；运行时可以从内嵌 runtime、环境变量、系统安装或注册表中
 * 选择任意 CPython 3.x。
 */

struct PyObject;
struct PyThreadState;

using Py_ssize_t = intptr_t;

struct PyMethodDef {
  const char* ml_name;
  PyObject* (*ml_meth)(PyObject*, PyObject*);
  int ml_flags;
  const char* ml_doc;
};

namespace python_runtime {

constexpr int kMETHVarArgs = 0x0001;

using PyCFunction = PyObject* (*)(PyObject*, PyObject*);

struct PyApi {
  void (*initialize)();
  int (*is_initialized)();
  void (*finalize)();
  void (*set_python_home)(const wchar_t*);
  const char* (*get_version)();

  int (*gil_ensure)();
  void (*gil_release)(int);
  PyThreadState* (*save_thread)();
  void (*restore_thread)(PyThreadState*);
  unsigned long (*thread_get_ident)();
  int (*thread_state_set_async_exc)(unsigned long, PyObject*);

  PyObject* (*err_occurred)();
  void (*err_fetch)(PyObject**, PyObject**, PyObject**);
  void (*err_normalize_exception)(PyObject**, PyObject**, PyObject**);
  void (*err_clear)();
  int (*err_exception_matches)(PyObject*);
  void (*err_set_string)(PyObject*, const char*);

  PyObject* (*object_str)(PyObject*);
  const char* (*unicode_as_utf8_and_size)(PyObject*, Py_ssize_t*);
  PyObject* (*cfunction_new_ex)(PyMethodDef*, PyObject*, PyObject*);
  int (*object_set_attr_string)(PyObject*, const char*, PyObject*);
  PyObject* (*object_get_attr_string)(PyObject*, const char*);
  PyObject* (*object_call_object)(PyObject*, PyObject*);

  PyObject* (*list_new)(Py_ssize_t);
  Py_ssize_t (*list_size)(PyObject*);
  PyObject* (*list_get_item)(PyObject*, Py_ssize_t);
  int (*list_insert)(PyObject*, Py_ssize_t, PyObject*);

  Py_ssize_t (*tuple_size)(PyObject*);
  PyObject* (*tuple_get_item)(PyObject*, Py_ssize_t);
  PyObject* (*tuple_new)(Py_ssize_t);
  int (*tuple_set_item)(PyObject*, Py_ssize_t, PyObject*);

  PyObject* (*import_import_module)(const char*);
  PyObject* (*import_get_module_dict)();
  int (*dict_del_item_string)(PyObject*, const char*);
  PyObject* (*sys_get_object)(const char*);
  PyObject* (*unicode_from_string)(const char*);

  void (*incref)(PyObject*);
  void (*decref)(PyObject*);
};

class Runtime {
 public:
  static Runtime& Instance();

  // 查找并加载 Python DLL，解析稳定 ABI 符号。失败时 error 包含原因。
  bool Load(std::string* error);
  bool IsLoaded() const { return module_ != nullptr; }

  const PyApi& api() const { return api_; }

  PyObject* none() const { return none_; }
  PyObject* system_exit() const { return system_exit_; }
  PyObject* type_error() const { return type_error_; }

  void IncRef(PyObject* object) const {
    if (object && api_.incref) {
      api_.incref(object);
    }
  }
  void DecRef(PyObject* object) const {
    if (object && api_.decref) {
      api_.decref(object);
    }
  }
  // 返回一个新的 None 引用，用于 C 函数返回值。
  PyObject* NewNoneRef() const {
    IncRef(none_);
    return none_;
  }

  const std::string& version() const { return version_; }
  const std::string& home() const { return home_; }
  const std::string& dll_path() const { return dll_path_; }
  const std::string& source() const { return source_; }
  bool embedded() const { return source_ == "embedded"; }
  const std::string& load_error() const { return load_error_; }

 private:
  Runtime() = default;
  Runtime(const Runtime&) = delete;
  Runtime& operator=(const Runtime&) = delete;

  bool TryLoadCandidate(const std::wstring& dll_path,
                        const std::wstring& home,
                        const std::string& source,
                        std::string* missing);
  bool ResolveSymbols(std::string* missing);
  static std::string ModuleDirectory();

  HMODULE module_ = nullptr;
  PyApi api_{};
  PyObject* none_ = nullptr;
  PyObject* system_exit_ = nullptr;
  PyObject* type_error_ = nullptr;
  std::wstring python_home_;
  std::string version_;
  std::string home_;
  std::string dll_path_;
  std::string source_;
  std::string load_error_;
};

}  // namespace python_runtime

#endif  // CUCKOO_PYTHON_RUNTIME_H
