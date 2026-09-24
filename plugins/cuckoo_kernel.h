#ifndef CUCKOO_KERNEL_H
#define CUCKOO_KERNEL_H
#ifdef __cplusplus
extern "C" {
#endif
#include <stdbool.h>
#include <stddef.h>
#include <stdint.h>

#include "cuckoo_plugin.h"
// 插件实例句柄：代表一个被加载的脚本/原生插件实例
typedef void* CuckooPluginHandle;
// CuckooHostAPI GO Core的API表
typedef struct {
  // 向核心发送事件
  void (*emit_event)(const char* event_name, const char* json_payload);
  // 打印日志
  void (*log_info)(const char* msg);
  void (*log_error)(const char* msg);
  // 获取插件配置项
  /**
   * @brief 业务插件或kernel自身获取配置项
   * @param  plug_name 插件名称
   */
  const char* (*get_plugin_config)(const char* plug_name);
  // 报告进入循环
  /**
   * @brief Kernel报告进入循环
   * @param kernel_id kernel id
   */
  void (*into_loop_report)(const char* kernel_id);
  // 触发崩溃并卸载插件
  /**
   * @brief Kernel 触发崩溃
   * @param reason 崩溃原因
   */
  void (*panic)(const char* reason);
} CuckooHostAPI;
// ============================================================================
// 3. Kernel 插件接口定义 (Kernel -> Host)
// ============================================================================
typedef struct {
  // [元数据]
  const char* kernel_id;    // 唯一标识，如 "com.cuckoo.kernel.lua"
  const char* kernel_name;  // 显示名称
  const char* version;      // 版本号
  const char* runtime;      // Kernel 提供的运行时类型
  // [生命周期管理]
  /**
   * @brief 初始化运行时环境
   * @param api Host 提供的 API 表
   */
  int (*init_runtime)(const CuckooHostAPI* api);
  /**
   * @brief 运行Kernel循环,保证Kernel进程
   */
  void (*run_loop)(const CuckooHostAPI* api);
  /**
   * @brief 加载并实例化一个插件
   * @param plugin_path 插件文件夹的绝对路径
   * @param manifest_json 插件的 manifest.json 内容
   * @param out_descriptor [out] 核心层传入的描述符结构体指针，Kernel 负责填充
   * @return 0 表示成功，非 0 表示失败
   */
  int (*load_plugin)(const char* plugin_path, const char* manifest_json,
                     CuckooPluginDescriptor* out_descriptor);
  /**
   * @brief 触发该插件实例中的某个回调函数
   * @param handle 插件实例句柄 (由 load_plugin 返回的 internal_object_ptr 或
   * instance_id)
   * @param listener 监听器记录 (包含 function_ptr)
   * @param payload 事件载荷数据（只读，Kernel 不应修改）
   */
  void (*trigger_callback)(CuckooPluginHandle handle,
                           const CuckooListenerRecord* listener,
                           const char* payload);
  /**
   * @brief 销毁插件实例
   */
  void (*unload_plugin)(CuckooPluginHandle handle);
  /**
   * @brief 关闭运行时环境，释放全局资源
   */
  void (*shutdown_runtime)(void);
} CuckooKernelInterface;
// ============================================================================
// 4. 导出入口
// ============================================================================
/**
 * @brief 获取内核接口
 */
__declspec(dllexport) CuckooKernelInterface* get_cuckoo_kernel_interface(void);
#ifdef __cplusplus
}
#endif
#endif  // CUCKOO_KERNEL_H
