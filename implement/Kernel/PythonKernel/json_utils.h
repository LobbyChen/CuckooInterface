// Copyright (C) 2026 LobbyChen
// SPDX-License-Identifier: AGPL-3.0-only
#ifndef MOCK_KERNEL_JSON_UTILS_H
#define MOCK_KERNEL_JSON_UTILS_H

#include <string>
#include <vector>
#include <cctype>

namespace mock_json {

inline void SkipWhitespace(const std::string& s, size_t& pos) {
  while (pos < s.size() && std::isspace(static_cast<unsigned char>(s[pos]))) {
    ++pos;
  }
}

inline bool ParseString(const std::string& s, size_t& pos, std::string& out) {
  if (pos >= s.size() || s[pos] != '"') return false;
  ++pos;
  out.clear();
  while (pos < s.size()) {
    char c = s[pos];
    if (c == '"') { ++pos; return true; }
    if (c == '\\' && pos + 1 < s.size()) {
      char esc = s[pos + 1];
      switch (esc) {
        case '"':  out.push_back('"');  break;
        case '\\': out.push_back('\\'); break;
        case '/':  out.push_back('/');  break;
        case 'n':  out.push_back('\n'); break;
        case 't':  out.push_back('\t'); break;
        case 'r':  out.push_back('\r'); break;
        default:   out.push_back(esc);  break;
      }
      pos += 2;
    } else {
      out.push_back(c);
      ++pos;
    }
  }
  return false;
}

inline bool GetStringField(const std::string& json, const std::string& key,
                           std::string& out) {
  std::string needle = "\"" + key + "\"";
  size_t pos = json.find(needle);
  if (pos == std::string::npos) return false;
  pos += needle.size();
  SkipWhitespace(json, pos);
  if (pos >= json.size() || json[pos] != ':') return false;
  ++pos;
  SkipWhitespace(json, pos);
  return ParseString(json, pos, out);
}

inline bool GetStringArrayField(const std::string& json, const std::string& key,
                                std::vector<std::string>& out) {
  out.clear();
  std::string needle = "\"" + key + "\"";
  size_t pos = json.find(needle);
  if (pos == std::string::npos) return false;
  pos += needle.size();
  SkipWhitespace(json, pos);
  if (pos >= json.size() || json[pos] != ':') return false;
  ++pos;
  SkipWhitespace(json, pos);
  if (pos >= json.size() || json[pos] != '[') return false;
  ++pos;
  SkipWhitespace(json, pos);
  if (pos < json.size() && json[pos] == ']') return true;
  while (pos < json.size()) {
    std::string item;
    if (!ParseString(json, pos, item)) return false;
    out.push_back(item);
    SkipWhitespace(json, pos);
    if (pos >= json.size()) return false;
    if (json[pos] == ']') { ++pos; return true; }
    if (json[pos] == ',') { ++pos; SkipWhitespace(json, pos); }
    else return false;
  }
  return false;
}

}  // namespace mock_json
#endif
