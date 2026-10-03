#ifndef CUCKOO_PLUGIN_DESCRIPTOR_H
#define CUCKOO_PLUGIN_DESCRIPTOR_H
#include <stdint.h>
#define MAX_LISTENERS_PER_PLUGIN 64
#define MAX_EVENTS_PER_PLUGIN 64
typedef uint64_t CuckooInstanceID;
typedef struct {
  char event_name[128];
  void* function;
} CuckooListenerRecord;
typedef struct {
  char event_name[128];
} CuckooEventRecord;
typedef struct {
  CuckooInstanceID instance_id;
  char plugin_id[64];
  char language_runtime[32];
  CuckooListenerRecord listeners[MAX_LISTENERS_PER_PLUGIN];
  int listener_count;
  CuckooEventRecord provided_events[MAX_EVENTS_PER_PLUGIN];
  int event_count;
  void* internal_object_ptr;
} CuckooPluginDescriptor;
#endif
