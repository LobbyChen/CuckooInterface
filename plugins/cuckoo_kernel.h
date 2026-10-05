#ifndef CUCKOO_KERNEL_H
#define CUCKOO_KERNEL_H

#ifdef __cplusplus
extern "C" {
#endif

#include <stddef.h>
#include <stdint.h>

#include "cuckoo_plugin.h"

typedef void* CuckooPluginHandle;

/*
 * Kernel -> Host API.
 *
 * operation 是通用宿主能力入口。具体 operation_id 由 Kernel 定义，
 * Core 不需要理解 Python/Lua 等 Runtime 的业务语义。
 */
typedef struct {
  void (*emit_event)(const char* event_name, const char* json_payload);
  void (*log_info)(const char* msg);
  void (*log_error)(const char* msg);

  /* 返回值生命周期仅保证到本次调用结束。 */
  const char* (*get_plugin_config)(const char* plugin_name);

  void (*into_loop_report)(const char* kernel_id);

  /* 例如 ui.confirm / ui.notify / ui.browse_file / download。 */
  const char* (*operation)(const char* operation_id,
                           const char* request_json);

  void (*panic)(const char* reason);
} CuckooHostAPI;

/*
 * Kernel Interface。
 *
 * 当前 ABI 尚未冻结，因此 Runtime Control API 直接扩展原始 Interface。
 * Control Plane 可以在 init_runtime() 之前调用。
 */
typedef struct {
  /* Metadata */
  const char* kernel_id;
  const char* kernel_name;
  const char* version;
  const char* runtime;

  /* Execution Plane */
  int (*init_runtime)(const CuckooHostAPI* api);
  void (*run_loop)(const CuckooHostAPI* api);

  int (*load_plugin)(const char* plugin_path,
                     const char* manifest_json,
                     CuckooPluginDescriptor* out_descriptor);

  void (*trigger_callback)(CuckooPluginHandle handle,
                           const CuckooListenerRecord* listener,
                           const char* payload);

  void (*unload_plugin)(CuckooPluginHandle handle);
  void (*shutdown_runtime)(void);

  /* Runtime Control Plane */

  /* 获取动态 Settings Panel JSON。 */
  int (*get_settings_panel)(const CuckooHostAPI* api,
                            char** out_json);

  /* 获取当前 Runtime State JSON。 */
  int (*get_runtime_state)(const CuckooHostAPI* api,
                           char** out_json);

  /* 修改 Kernel-defined setting，value_json 是 JSON value。 */
  int (*set_setting)(const CuckooHostAPI* api,
                     const char* key,
                     const char* value_json,
                     char** out_json);

  /* 执行 Kernel-defined operation，例如 runtime.discover。 */
  int (*invoke_operation)(const CuckooHostAPI* api,
                          const char* operation_id,
                          const char* request_json,
                          char** out_json);

  /*
   * 释放 Control Plane 返回的 JSON 字符串。
   * Host/Core 不得直接 free Kernel-owned memory。
   */
  void (*free_response)(char* response);

} CuckooKernelInterface;

__declspec(dllexport)
CuckooKernelInterface* get_cuckoo_kernel_interface(void);

#ifdef __cplusplus
}
#endif

#endif  /* CUCKOO_KERNEL_H */
