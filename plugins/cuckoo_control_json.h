// Copyright (C) 2026 LobbyChen
// SPDX-License-Identifier: AGPL-3.0-only
#ifndef CUCKOO_CONTROL_JSON_H
#define CUCKOO_CONTROL_JSON_H

#include <cstdlib>
#include <cstring>
#include <string>
#include <vector>

/*
 * Kernel Runtime Control Plane 的最小 JSON 构造工具。
 * 仅用于 Kernel 内部构造控制面响应，不引入额外依赖。
 */
namespace cuckoo_control {

inline std::string Escape(const std::string& value) {
  static const char hex[] = "0123456789abcdef";
  std::string out;
  out.reserve(value.size() + 8);
  for (const unsigned char c : value) {
    switch (c) {
      case '"': out += "\\\""; break;
      case '\\': out += "\\\\"; break;
      case '\b': out += "\\b"; break;
      case '\f': out += "\\f"; break;
      case '\n': out += "\\n"; break;
      case '\r': out += "\\r"; break;
      case '\t': out += "\\t"; break;
      default:
        if (c < 0x20) {
          out += "\\u00";
          out.push_back(hex[(c >> 4) & 0x0F]);
          out.push_back(hex[c & 0x0F]);
        } else {
          out.push_back(static_cast<char>(c));
        }
    }
  }
  return out;
}

inline std::string Quote(const std::string& value) {
  return "\"" + Escape(value) + "\"";
}

inline const char* Bool(bool value) { return value ? "true" : "false"; }

inline char* Alloc(const std::string& value) {
  char* buffer = static_cast<char*>(std::malloc(value.size() + 1));
  if (buffer != nullptr) {
    std::memcpy(buffer, value.c_str(), value.size() + 1);
  }
  return buffer;
}

inline char* AllocError(const std::string& message) {
  return Alloc(std::string("{\"ok\":false,\"error\":") + Quote(message) + "}");
}

inline char* AllocOk(const std::string& extra_fields = std::string()) {
  std::string out = "{\"ok\":true";
  if (!extra_fields.empty()) {
    out += ",";
    out += extra_fields;
  }
  out += "}";
  return Alloc(out);
}

inline std::string TextSetting(const std::string& key, const std::string& name,
                               const std::string& description,
                               const std::string& value,
                               bool read_only = true) {
  std::string out = "{\"key\":" + Quote(key);
  out += ",\"name\":" + Quote(name);
  out += ",\"description\":" + Quote(description);
  out += ",\"displayName\":true,\"displayDescription\":true";
  out += ",\"value\":" + Quote(value);
  out += ",\"defaultValue\":" + Quote(value);
  out += ",\"editor\":{\"type\":\"text\"";
  if (read_only) {
    out += ",\"readOnly\":true";
  }
  out += "}}";
  return out;
}

inline std::string SwitchSetting(const std::string& key,
                                 const std::string& name,
                                 const std::string& description, bool value) {
  std::string out = "{\"key\":" + Quote(key);
  out += ",\"name\":" + Quote(name);
  out += ",\"description\":" + Quote(description);
  out += ",\"displayName\":true,\"displayDescription\":true";
  out += ",\"value\":";
  out += Bool(value);
  out += ",\"defaultValue\":false";
  out += ",\"editor\":{\"type\":\"switch\"}}";
  return out;
}

inline std::string StringArray(const std::vector<std::string>& values) {
  std::string out = "[";
  for (size_t i = 0; i < values.size(); ++i) {
    if (i != 0) {
      out += ",";
    }
    out += Quote(values[i]);
  }
  out += "]";
  return out;
}

}  // namespace cuckoo_control

#endif  // CUCKOO_CONTROL_JSON_H
