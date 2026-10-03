#ifndef CUCKOO_LUA_KERNEL_JSON_UTILS_H
#define CUCKOO_LUA_KERNEL_JSON_UTILS_H

#include <cctype>
#include <string>
#include <vector>

namespace cuckoo_json {

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
    const char c = s[pos];
    if (c == '"') {
      ++pos;
      return true;
    }
    if (c == '\\' && pos + 1 < s.size()) {
      const char esc = s[pos + 1];
      switch (esc) {
        case '"': out.push_back('"'); break;
        case '\\': out.push_back('\\'); break;
        case '/': out.push_back('/'); break;
        case 'b': out.push_back('\b'); break;
        case 'f': out.push_back('\f'); break;
        case 'n': out.push_back('\n'); break;
        case 'r': out.push_back('\r'); break;
        case 't': out.push_back('\t'); break;
        default: out.push_back(esc); break;
      }
      pos += 2;
      continue;
    }
    out.push_back(c);
    ++pos;
  }
  return false;
}

inline bool GetStringField(const std::string& json, const std::string& key,
                           std::string& out) {
  const std::string needle = "\"" + key + "\"";
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
  const std::string needle = "\"" + key + "\"";
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
    if (json[pos] == ']') {
      ++pos;
      return true;
    }
    if (json[pos] != ',') return false;
    ++pos;
    SkipWhitespace(json, pos);
  }
  return false;
}

}  // namespace cuckoo_json

#endif  // CUCKOO_LUA_KERNEL_JSON_UTILS_H
