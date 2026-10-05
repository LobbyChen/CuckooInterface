#ifndef SANDBOX_HOST_H
#define SANDBOX_HOST_H

#include <windows.h>

#include "cuckoo_kernel.h"
#include "cuckoo_plugin.h"

#ifdef __cplusplus
extern "C" {
#endif

/* 创建/销毁 Sandbox。 */
void* create_sandbox(const char* dll_path);
void destroy_sandbox(void* ctx);

/* 元数据。 */
void sandbox_get_meta(void* ctx,
                      char* out_id,
                      char* out_name,
                      char* out_ver,
                      char* out_rt);

/* Execution Plane。 */
int sandbox_init_runtime(void* ctx, CuckooHostAPI* api);

int sandbox_load_plugin(void* ctx,
                        const char* path,
                        const char* manifest,
                        CuckooPluginDescriptor* out_desc);

void sandbox_trigger_callback(void* ctx,
                              CuckooPluginHandle handle,
                              const CuckooListenerRecord* listener,
                              const char* payload);

void sandbox_unload_plugin(void* ctx, CuckooPluginHandle handle);

/* Runtime Control Plane。 */
int sandbox_get_settings_panel(void* ctx,
                               CuckooHostAPI* api,
                               char** out_json);

int sandbox_get_runtime_state(void* ctx,
                              CuckooHostAPI* api,
                              char** out_json);

int sandbox_set_setting(void* ctx,
                        CuckooHostAPI* api,
                        const char* key,
                        const char* value_json,
                        char** out_json);

int sandbox_get_operations(void* ctx,
                           CuckooHostAPI* api,
                           char** out_json);

int sandbox_invoke_operation(void* ctx,
                             CuckooHostAPI* api,
                             const char* operation_id,
                             const char* request_json,
                             char** out_json);

/* 通过 Kernel 的 free_response 释放返回值。 */
void sandbox_free_kernel_response(void* ctx, char* response);

/* Loop / shutdown / crash。 */
void sandbox_start_loop_async(void* ctx, CuckooHostAPI* api);
void sandbox_shutdown(void* ctx);

int is_sandbox_alive(void* ctx);
void sandbox_get_crash_reason(void* ctx, char* buf, int bufsize);
void sandbox_wait_for_crash(void* ctx);

#ifdef __cplusplus
}
#endif

#endif  /* SANDBOX_HOST_H */
