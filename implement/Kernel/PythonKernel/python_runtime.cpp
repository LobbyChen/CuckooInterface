// Copyright (C) 2026 LobbyChen
// SPDX-License-Identifier: AGPL-3.0-only
#include "python_runtime.h"

#include <algorithm>
#include <cstring>
#include <iterator>
#include <type_traits>
#include <vector>

namespace python_runtime {

namespace {

struct Candidate {
  std::wstring dll_path;
  std::wstring home;
  std::string source;
};

std::string WideToUtf8(const std::wstring& value) {
  if (value.empty()) {
    return std::string();
  }
  const int size = WideCharToMultiByte(CP_UTF8, 0, value.c_str(),
                                       static_cast<int>(value.size()), nullptr,
                                       0, nullptr, nullptr);
  if (size <= 0) {
    return std::string();
  }
  std::string out(static_cast<size_t>(size), '\0');
  WideCharToMultiByte(CP_UTF8, 0, value.c_str(),
                      static_cast<int>(value.size()), out.data(), size,
                      nullptr, nullptr);
  return out;
}

std::wstring JoinPath(const std::wstring& base, const wchar_t* child) {
  if (base.empty()) {
    return std::wstring(child);
  }
  std::wstring out = base;
  if (out.back() != L'\\' && out.back() != L'/') {
    out.push_back(L'\\');
  }
  out += child;
  return out;
}

bool FileExists(const std::wstring& path) {
  const DWORD attrs = GetFileAttributesW(path.c_str());
  return attrs != INVALID_FILE_ATTRIBUTES &&
         (attrs & FILE_ATTRIBUTE_DIRECTORY) == 0;
}

bool DirExists(const std::wstring& path) {
  const DWORD attrs = GetFileAttributesW(path.c_str());
  return attrs != INVALID_FILE_ATTRIBUTES &&
         (attrs & FILE_ATTRIBUTE_DIRECTORY) != 0;
}

// 在目录中查找 Python DLL：优先 python3.dll（稳定 ABI 转发器）。
std::wstring FindPythonDll(const std::wstring& home) {
  if (home.empty()) {
    return std::wstring();
  }
  const std::wstring stable = JoinPath(home, L"python3.dll");
  if (FileExists(stable)) {
    return stable;
  }

  const std::wstring pattern = JoinPath(home, L"python3*.dll");
  WIN32_FIND_DATAW data{};
  HANDLE find = FindFirstFileW(pattern.c_str(), &data);
  if (find == INVALID_HANDLE_VALUE) {
    return std::wstring();
  }
  std::wstring best;
  do {
    std::wstring name(data.cFileName);
    if (name.size() <= 11) {  // python3XX.dll
      continue;
    }
    best = JoinPath(home, name.c_str());
  } while (FindNextFileW(find, &data));
  FindClose(find);
  return best;
}

bool HasPythonStdlib(const std::wstring& home) {
  if (home.empty()) {
    return false;
  }
  if (DirExists(JoinPath(home, L"Lib"))) {
    return true;
  }
  const std::wstring zip = JoinPath(home, L"python3*.zip");
  WIN32_FIND_DATAW data{};
  HANDLE find = FindFirstFileW(zip.c_str(), &data);
  if (find != INVALID_HANDLE_VALUE) {
    FindClose(find);
    return true;
  }
  const std::wstring pth = JoinPath(home, L"python3*._pth");
  find = FindFirstFileW(pth.c_str(), &data);
  if (find != INVALID_HANDLE_VALUE) {
    FindClose(find);
    return true;
  }
  return false;
}

std::wstring NormalizeDir(const std::wstring& value) {
  if (value.empty()) {
    return std::wstring();
  }
  wchar_t buffer[32768] = {0};
  if (GetFullPathNameW(value.c_str(), 32768, buffer, nullptr) == 0) {
    return value;
  }
  std::wstring out(buffer);
  while (out.size() > 3 && (out.back() == L'\\' || out.back() == L'/')) {
    out.pop_back();
  }
  return out;
}

void AddCandidate(std::vector<Candidate>* out, const std::wstring& dll,
                  const std::wstring& home, const std::string& source) {
  if (dll.empty()) {
    return;
  }
  for (const auto& existing : *out) {
    if (_wcsicmp(existing.dll_path.c_str(), dll.c_str()) == 0) {
      return;
    }
  }
  out->push_back(Candidate{dll, NormalizeDir(home), source});
}

void CollectRegistryHomes(HKEY root, const wchar_t* subkey,
                          std::vector<std::wstring>* homes) {
  HKEY versions = nullptr;
  if (RegOpenKeyExW(root, subkey, 0, KEY_READ | KEY_ENUMERATE_SUB_KEYS,
                    &versions) != ERROR_SUCCESS) {
    return;
  }

  std::vector<std::wstring> names;
  wchar_t name[256];
  for (DWORD index = 0;; ++index) {
    DWORD size = 256;
    const LONG rc = RegEnumKeyExW(versions, index, name, &size, nullptr, nullptr,
                                  nullptr, nullptr);
    if (rc == ERROR_NO_MORE_ITEMS) {
      break;
    }
    if (rc == ERROR_SUCCESS) {
      names.emplace_back(name, size);
    }
  }
  RegCloseKey(versions);

  std::sort(names.rbegin(), names.rend());
  for (const auto& version : names) {
    std::wstring path(subkey);
    path += L"\\";
    path += version;
    path += L"\\InstallPath";

    HKEY install = nullptr;
    if (RegOpenKeyExW(root, path.c_str(), 0, KEY_READ, &install) !=
        ERROR_SUCCESS) {
      continue;
    }
    wchar_t value[32768] = {0};
    DWORD value_size = sizeof(value) - sizeof(wchar_t);
    DWORD type = 0;
    const LONG rc = RegQueryValueExW(install, nullptr, nullptr, &type,
                                     reinterpret_cast<LPBYTE>(value),
                                     &value_size);
    RegCloseKey(install);
    if (rc == ERROR_SUCCESS && (type == REG_SZ || type == REG_EXPAND_SZ) &&
        value[0] != L'\0') {
      homes->push_back(NormalizeDir(value));
    }
  }
}

std::vector<std::wstring> SystemRegistryHomes() {
  std::vector<std::wstring> homes;
  CollectRegistryHomes(HKEY_CURRENT_USER, L"SOFTWARE\\Python\\PythonCore",
                       &homes);
  CollectRegistryHomes(HKEY_LOCAL_MACHINE, L"SOFTWARE\\Python\\PythonCore",
                       &homes);
  CollectRegistryHomes(HKEY_LOCAL_MACHINE,
                       L"SOFTWARE\\WOW6432Node\\Python\\PythonCore", &homes);
  return homes;
}

std::vector<Candidate> SystemCandidates() {
  std::vector<Candidate> out;

  wchar_t found[MAX_PATH] = {0};
  if (SearchPathW(nullptr, L"python3.dll", nullptr, MAX_PATH, found, nullptr) >
      0) {
    std::wstring dll(found);
    std::wstring home = dll.substr(0, dll.find_last_of(L"\\/"));
    AddCandidate(&out, dll, home, "system");
  }

  for (const auto& home : SystemRegistryHomes()) {
    AddCandidate(&out, FindPythonDll(home), home, "system");
  }
  return out;
}

std::string FirstVersionToken(const char* version) {
  if (!version) {
    return std::string();
  }
  std::string value(version);
  const size_t pos = value.find_first_of(" \t\r\n");
  if (pos != std::string::npos) {
    value.resize(pos);
  }
  return value;
}

}  // namespace

Runtime& Runtime::Instance() {
  static Runtime instance;
  return instance;
}

bool Runtime::Load(std::string* error) {
  if (module_ != nullptr) {
    return true;
  }

  std::vector<Candidate> candidates;
  const std::vector<Candidate> system = SystemCandidates();

  // 用户选择的 Python 优先，其次是 PATH / 注册表中的系统安装。
  wchar_t env_home[32768] = {0};
  const DWORD env_len = GetEnvironmentVariableW(
      L"CUCKOO_PYTHON_HOME", env_home, static_cast<DWORD>(std::size(env_home)));
  if (env_len > 0 && env_len < std::size(env_home)) {
    const std::wstring home = NormalizeDir(env_home);
    std::string source = "config";
    char source_buffer[32] = {0};
    const DWORD source_len = GetEnvironmentVariableA(
        "CUCKOO_PYTHON_HOME_SOURCE", source_buffer,
        static_cast<DWORD>(std::size(source_buffer)));
    if (source_len > 0 && source_len < std::size(source_buffer)) {
      const std::string configured(source_buffer, source_len);
      if (configured == "system" || configured == "config") {
        source = configured;
      }
    }
    AddCandidate(&candidates, FindPythonDll(home), home, source);
  }
  candidates.insert(candidates.end(), system.begin(), system.end());

  if (candidates.empty()) {
    load_error_ =
        "no CPython runtime found. Install 64-bit CPython 3.8+ on this "
        "machine, or choose an installation in the Runtimes page.";
    if (error) {
      *error = load_error_;
    }
    return false;
  }

  std::string attempts;
  for (const auto& candidate : candidates) {
    std::string missing;
    if (!TryLoadCandidate(candidate.dll_path, candidate.home, candidate.source,
                          &missing)) {
      if (!attempts.empty()) {
        attempts += "; ";
      }
      attempts += WideToUtf8(candidate.dll_path) + ": " + missing;
      continue;
    }

    version_ = FirstVersionToken(api_.get_version());
    home_ = WideToUtf8(python_home_);
    dll_path_ = WideToUtf8(candidate.dll_path);
    source_ = candidate.source;
    load_error_.clear();
    return true;
  }

  load_error_ =
      "unable to load a CPython runtime. Install 64-bit CPython 3.8+ or "
      "choose a valid installation in the Runtimes page: " +
      attempts;
  if (error) {
    *error = load_error_;
  }
  return false;
}

bool Runtime::TryLoadCandidate(const std::wstring& dll_path,
                               const std::wstring& home,
                               const std::string& source,
                               std::string* missing) {
  HMODULE module =
      LoadLibraryExW(dll_path.c_str(), nullptr, LOAD_WITH_ALTERED_SEARCH_PATH);
  if (module == nullptr) {
    *missing = "LoadLibraryEx failed with error " +
               std::to_string(static_cast<unsigned long>(GetLastError()));
    return false;
  }

  module_ = module;
  api_ = PyApi{};
  none_ = nullptr;
  system_exit_ = nullptr;
  type_error_ = nullptr;
  if (!ResolveSymbols(missing)) {
    FreeLibrary(module);
    module_ = nullptr;
    return false;
  }

  std::wstring effective_home = home;
  if (effective_home.empty()) {
    wchar_t loaded_path[MAX_PATH] = {0};
    const DWORD len = GetModuleFileNameW(module, loaded_path, MAX_PATH);
    if (len > 0 && len < MAX_PATH) {
      const std::wstring dll(loaded_path, len);
      const size_t pos = dll.find_last_of(L"\\/");
      if (pos != std::wstring::npos) {
        const std::wstring dir = dll.substr(0, pos);
        if (HasPythonStdlib(dir)) {
          effective_home = dir;
        }
      }
    }
  }

  if (!effective_home.empty()) {
    python_home_ = effective_home;
    api_.set_python_home(python_home_.c_str());
  } else {
    python_home_.clear();
  }

  return true;
}

bool Runtime::ResolveSymbols(std::string* missing) {
  auto resolve = [&](auto& field, const char* name) -> bool {
    FARPROC proc = GetProcAddress(module_, name);
    if (proc == nullptr) {
      if (!missing->empty()) {
        *missing += ", ";
      }
      *missing += name;
      return false;
    }
    field = reinterpret_cast<std::decay_t<decltype(field)>>(proc);
    return true;
  };

  bool ok = true;
  ok &= resolve(api_.initialize, "Py_Initialize");
  ok &= resolve(api_.is_initialized, "Py_IsInitialized");
  ok &= resolve(api_.finalize, "Py_Finalize");
  ok &= resolve(api_.set_python_home, "Py_SetPythonHome");
  ok &= resolve(api_.get_version, "Py_GetVersion");

  ok &= resolve(api_.gil_ensure, "PyGILState_Ensure");
  ok &= resolve(api_.gil_release, "PyGILState_Release");
  ok &= resolve(api_.save_thread, "PyEval_SaveThread");
  ok &= resolve(api_.restore_thread, "PyEval_RestoreThread");
  ok &= resolve(api_.thread_get_ident, "PyThread_get_thread_ident");
  ok &= resolve(api_.thread_state_set_async_exc,
                "PyThreadState_SetAsyncExc");

  ok &= resolve(api_.err_occurred, "PyErr_Occurred");
  ok &= resolve(api_.err_fetch, "PyErr_Fetch");
  ok &= resolve(api_.err_normalize_exception, "PyErr_NormalizeException");
  ok &= resolve(api_.err_clear, "PyErr_Clear");
  ok &= resolve(api_.err_exception_matches, "PyErr_ExceptionMatches");
  ok &= resolve(api_.err_set_string, "PyErr_SetString");

  ok &= resolve(api_.object_str, "PyObject_Str");
  ok &= resolve(api_.unicode_as_utf8_and_size, "PyUnicode_AsUTF8AndSize");
  ok &= resolve(api_.cfunction_new_ex, "PyCFunction_NewEx");
  ok &= resolve(api_.object_set_attr_string, "PyObject_SetAttrString");
  ok &= resolve(api_.object_get_attr_string, "PyObject_GetAttrString");
  ok &= resolve(api_.object_call_object, "PyObject_CallObject");

  ok &= resolve(api_.list_new, "PyList_New");
  ok &= resolve(api_.list_size, "PyList_Size");
  ok &= resolve(api_.list_get_item, "PyList_GetItem");
  ok &= resolve(api_.list_insert, "PyList_Insert");

  ok &= resolve(api_.tuple_size, "PyTuple_Size");
  ok &= resolve(api_.tuple_get_item, "PyTuple_GetItem");
  ok &= resolve(api_.tuple_new, "PyTuple_New");
  ok &= resolve(api_.tuple_set_item, "PyTuple_SetItem");

  ok &= resolve(api_.import_import_module, "PyImport_ImportModule");
  ok &= resolve(api_.import_get_module_dict, "PyImport_GetModuleDict");
  ok &= resolve(api_.dict_del_item_string, "PyDict_DelItemString");
  ok &= resolve(api_.sys_get_object, "PySys_GetObject");
  ok &= resolve(api_.unicode_from_string, "PyUnicode_FromString");

  ok &= resolve(api_.incref, "Py_IncRef");
  ok &= resolve(api_.decref, "Py_DecRef");

  if (!ok) {
    return false;
  }

  none_ = reinterpret_cast<PyObject*>(GetProcAddress(module_, "_Py_NoneStruct"));
  auto* system_exit = reinterpret_cast<PyObject**>(
      GetProcAddress(module_, "PyExc_SystemExit"));
  auto* type_error = reinterpret_cast<PyObject**>(
      GetProcAddress(module_, "PyExc_TypeError"));
  if (none_ == nullptr || system_exit == nullptr || type_error == nullptr) {
    if (!missing->empty()) {
      *missing += ", ";
    }
    *missing += "_Py_NoneStruct/PyExc_SystemExit/PyExc_TypeError";
    return false;
  }
  system_exit_ = *system_exit;
  type_error_ = *type_error;

  return true;
}

}  // namespace python_runtime
