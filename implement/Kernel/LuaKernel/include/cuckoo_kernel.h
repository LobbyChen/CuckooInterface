#ifndef CUCKOO_KERNEL_H
#define CUCKOO_KERNEL_H
#ifdef __cplusplus
extern "C" {
#endif
#include <stdbool.h>
#include <stddef.h>
#include <stdint.h>
#include "cuckoo_plugin.h"
typedef void* CuckooPluginHandle;
typedef struct {
  void (*emit_event)(const char* event_name, const char* json_payload);
  void (*log_info)(const char* msg);
  void (*log_error)(const char* msg);
  const char* (*get_plugin_config)(const char* plug_name);
  void (*into_loop_report)(const char* kernel_id);
  void (*panic)(const char* reason);
} CuckooHostAPI;
typedef struct {
  const char* kernel_id;
  const char* kernel_name;
  const char* version;
  const char* runtime;
  int (*init_runtime)(const CuckooHostAPI* api);
  void (*run_loop)(const CuckooHostAPI* api);
  int (*load_plugin)(const char* plugin_path, const char* manifest_json,
                     CuckooPluginDescriptor* out_descriptor);
  void (*trigger_callback)(CuckooPluginHandle handle,
                           const CuckooListenerRecord* listener,
                           const char* payload);
  void (*unload_plugin)(CuckooPluginHandle handle);
  void (*shutdown_runtime)(void);
} CuckooKernelInterface;
__declspec(dllexport) CuckooKernelInterface* get_cuckoo_kernel_interface(void);
#ifdef __cplusplus
}
#endif
#endif
