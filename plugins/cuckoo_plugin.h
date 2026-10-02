#ifndef CUCKOO_PLUGIN_DESCRIPTOR_H
#define CUCKOO_PLUGIN_DESCRIPTOR_H
#include <stdint.h>
// 最大监听事件数限制
#define MAX_LISTENERS_PER_PLUGIN 64
// 插件最大提供的事件记录数量
#define MAX_EVENTS_PER_PLUGIN 64
// 插件实例的唯一标识
typedef uint64_t CuckooInstanceID;
// 事件监听记录
typedef struct {
  char event_name[128];  // 事件名，如 "foundation.process.exited"
  void* function;        // 指向函数对象封装的指针
} CuckooListenerRecord;
// 事件提供记录
typedef struct {
  char event_name[128];  // 事件名，如 "foundation.process.exited"
} CuckooEventRecord;
typedef struct {
  CuckooInstanceID instance_id;
  char plugin_id[64];
  char language_runtime[32];
  // 订阅的监听器
  CuckooListenerRecord listeners[MAX_LISTENERS_PER_PLUGIN];
  int listener_count;
  // 该插件提供的事件
  CuckooEventRecord provided_events[MAX_EVENTS_PER_PLUGIN];
  int event_count;
  void* internal_object_ptr;
} CuckooPluginDescriptor;
#endif


