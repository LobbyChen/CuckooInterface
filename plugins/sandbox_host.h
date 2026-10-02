#ifndef SANDBOX_HOST_H
#define SANDBOX_HOST_H
#include <windows.h>

#include "cuckoo_kernel.h"
#include "cuckoo_plugin.h"
#ifdef __cplusplus
extern "C" {
#endif
// 创建沙箱上下文
void* create_sandbox(const char* dll_path);
// 销毁沙箱，停止线程并卸载 DLL
void destroy_sandbox(void* ctx);
// 获取沙箱元数据
void sandbox_get_meta(void* ctx, char* out_id, char* out_name, char* out_ver,
                      char* out_rt);
// 提交任务到沙箱线程执行
int sandbox_init_runtime(void* ctx, CuckooHostAPI* api);
int sandbox_load_plugin(void* ctx, const char* path, const char* manifest,
                        CuckooPluginDescriptor* out_desc);
void sandbox_trigger_callback(void* ctx, CuckooPluginHandle handle,
                              const CuckooListenerRecord* listener,
                              const char* payload);
void sandbox_unload_plugin(void* ctx, CuckooPluginHandle handle);
// 异步启动循环
void sandbox_start_loop_async(void* ctx, CuckooHostAPI* api);
// 关闭沙箱
void sandbox_shutdown(void* ctx);
// 检查沙箱是否存活
int is_sandbox_alive(void* ctx);
// 获取最近一次崩溃的原因
void sandbox_get_crash_reason(void* ctx, char* buf, int bufsize);
// 阻塞等待沙箱崩溃事件触发
void sandbox_wait_for_crash(void* ctx);
#ifdef __cplusplus
}
#endif
#endif  // SANDBOX_HOST_H


