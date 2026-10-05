#include "sandbox_host.h"

#include <stdio.h>
#include <stdlib.h>
#include <string.h>
// TLS 索引，用于在 VEH 中获取当前崩溃的上下文
static DWORD g_tls_index = TLS_OUT_OF_INDEXES;
static volatile LONG g_tls_inited = 0;
// 线程安全地一次性分配 TLS 索引
static void ensure_tls_index(void)
{
    if (InterlockedCompareExchange(&g_tls_inited, 1, 0) == 0)
    {
        g_tls_index = TlsAlloc();
    }
}
// VEH 回调前置声明
LONG WINAPI SandboxVehHandler(PEXCEPTION_POINTERS pExceptionInfo);
// VEH 是进程级别的，只需注册一次。
static PVOID g_veh_handle = NULL;
static volatile LONG g_veh_registered = 0;
// 确保 VEH 全局只注册一次
static void ensure_veh_registered(void)
{
    if (InterlockedCompareExchange(&g_veh_registered, 1, 0) == 0)
    {
        g_veh_handle = AddVectoredExceptionHandler(1, SandboxVehHandler);
    }
}
typedef enum
{
    TASK_INIT,
    TASK_LOAD,
    TASK_TRIGGER,
    TASK_UNLOAD,
    TASK_RUN_LOOP,
    TASK_SHUTDOWN
} TaskType;
typedef struct
{
    TaskType type;
    CuckooHostAPI *api;
    char *plugin_path;
    char *manifest;
    CuckooPluginDescriptor *out_desc;
    CuckooPluginHandle handle;
    CuckooListenerRecord listener;
    char *payload;
    HANDLE done_event; // NULL 表示异步任务
    int result;
} SandboxTask;
// 任务队列节点：每个任务独立存储，避免单槽 current_task 的 result
// 被后续任务覆盖
typedef struct TaskNode
{
    SandboxTask task;
    struct TaskNode *next;
} TaskNode;
typedef struct
{
    CuckooKernelInterface *iface;
    HMODULE hModule;
    HANDLE hThread;          // 任务处理工作线程
    HANDLE hLoopThread;      // run_loop 专用线程
    CuckooHostAPI *loop_api; // 传递给 run_loop 的 API
    // 任务队列同步原语
    HANDLE task_mutex;
    HANDLE task_event;
    TaskNode *task_head;
    TaskNode *task_tail;
    // 当前正在执行的任务
    SandboxTask *current_executing;
    // 崩溃事件
    HANDLE crash_event;
    // 崩溃原因缓存
    char crash_reason[128];
    // 状态
    BOOL is_crashed;
    BOOL is_running;
    // 缓存的元数据
    char kernel_id[64];
    char kernel_name[64];
    char version[32];
    char runtime[32];
} SandboxContext;

LONG WINAPI SandboxVehHandler(PEXCEPTION_POINTERS pExceptionInfo)
{
    DWORD code = pExceptionInfo->ExceptionRecord->ExceptionCode;
    if (code == EXCEPTION_ACCESS_VIOLATION || code == EXCEPTION_STACK_OVERFLOW ||
        code == EXCEPTION_ILLEGAL_INSTRUCTION)
    {
        SandboxContext *ctx = (SandboxContext *)TlsGetValue(g_tls_index);
        if (ctx)
        {
            ctx->is_crashed = TRUE;
            ctx->is_running = FALSE;
            // 存储崩溃原因
            snprintf(ctx->crash_reason, sizeof(ctx->crash_reason),
                     "Kernel %s crashed with exception 0x%08lX", ctx->kernel_id,
                     (unsigned long)code);
            // 唤醒可能正在等待当前任务完成的 Go 线程
            if (ctx->current_executing && ctx->current_executing->done_event)
            {
                SetEvent(ctx->current_executing->done_event);
            }
            // 触发崩溃事件，唤醒等待中的 Go 侧
            if (ctx->crash_event)
            {
                SetEvent(ctx->crash_event);
            }
            char dbg_msg[160];
            snprintf(dbg_msg, sizeof(dbg_msg),
                     "[Sandbox] Caught fatal exception 0x%08lX in Kernel %s! "
                     "Isolating...\n",
                     (unsigned long)code, ctx->kernel_id);
            OutputDebugStringA(dbg_msg);
        }
        ExitThread(1); // 仅终止当前沙箱线程
        return EXCEPTION_EXECUTE_HANDLER;
    }
    return EXCEPTION_CONTINUE_SEARCH;
}
// run_loop 专用线程入口。
static DWORD WINAPI RunLoopThread(LPVOID param)
{
    SandboxContext *ctx = (SandboxContext *)param;
    TlsSetValue(g_tls_index, ctx);
    ensure_veh_registered();
    if (ctx->iface->run_loop)
    {
        ctx->iface->run_loop(ctx->loop_api);
    }
    return 0;
}
// 沙箱工作线程入口
DWORD WINAPI SandboxWorker(LPVOID param)
{
    SandboxContext *ctx = (SandboxContext *)param;
    TlsSetValue(g_tls_index, ctx); // 绑定 TLS
    ensure_veh_registered();
    while (ctx->is_running && !ctx->is_crashed)
    {
        WaitForSingleObject(ctx->task_event, INFINITE);
        if (!ctx->is_running || ctx->is_crashed)
            break;
        // 从队列头部取出一个任务
        WaitForSingleObject(ctx->task_mutex, INFINITE);
        TaskNode *node = ctx->task_head;
        if (node)
        {
            ctx->task_head = node->next;
            if (!ctx->task_head)
                ctx->task_tail = NULL;
        }
        ReleaseMutex(ctx->task_mutex);
        if (!node)
            continue; // 虚假唤醒，队列已空
        SandboxTask task = node->task;
        ctx->current_executing = &node->task;
        switch (task.type)
        {
        case TASK_INIT:
            task.result =
                ctx->iface->init_runtime ? ctx->iface->init_runtime(task.api) : -1;
            break;
        case TASK_LOAD:
            task.result = ctx->iface->load_plugin
                              ? ctx->iface->load_plugin(
                                    task.plugin_path, task.manifest, task.out_desc)
                              : -1;
            break;
        case TASK_TRIGGER:
            if (ctx->iface->trigger_callback)
                ctx->iface->trigger_callback(task.handle, &task.listener,
                                             task.payload);
            break;
        case TASK_UNLOAD:
            if (ctx->iface->unload_plugin)
                ctx->iface->unload_plugin(task.handle);
            break;
        case TASK_SHUTDOWN:
            if (ctx->iface->shutdown_runtime)
                ctx->iface->shutdown_runtime();
            ctx->is_running = FALSE;
            break;
        default:
            break;
        }
        ctx->current_executing = NULL;
        // 将结果写回节点
        node->task.result = task.result;
        if (task.done_event != NULL)
        {
            SetEvent(task.done_event);
        }
        else
        {
            // 异步任务
            free(node);
        }
    }
    // VEH 全局共享
    return 0;
}
// 单个任务的最长执行时间。插件回调（尤其 Lua）可能死循环或阻塞，
// 若无上限会占死唯一的工作线程，导致该 Kernel 的卸载/关闭永久挂起。
#define TASK_TIMEOUT_MS 10000

// 提交任务并等待完成
static int submit_task(SandboxContext *ctx, SandboxTask *task)
{
    if (ctx->is_crashed || !ctx->is_running)
        return -1;
    TaskNode *node = (TaskNode *)malloc(sizeof(TaskNode));
    if (!node)
        return -1;
    node->task = *task;
    node->task.done_event = CreateEvent(NULL, FALSE, FALSE, NULL);
    node->next = NULL;
    // 入队
    WaitForSingleObject(ctx->task_mutex, INFINITE);
    if (ctx->task_tail)
    {
        ctx->task_tail->next = node;
    }
    else
    {
        ctx->task_head = node;
    }
    ctx->task_tail = node;
    ReleaseMutex(ctx->task_mutex);
    SetEvent(ctx->task_event); // 唤醒工作线程
    // 等待任务完成或线程崩溃，带超时兜底
    HANDLE handles[2] = {node->task.done_event, ctx->hThread};
    DWORD waitResult = WaitForMultipleObjects(2, handles, FALSE, TASK_TIMEOUT_MS);
    if (waitResult == WAIT_OBJECT_0)
    {
        int result = node->task.result;
        CloseHandle(node->task.done_event);
        free(node);
        return result;
    }
    if (waitResult == WAIT_TIMEOUT)
    {
        // 任务超时（插件回调死循环/阻塞）：标记沙箱已报废，拒绝后续任务。
        // 注意不能在此释放 node / done_event：工作线程仍可能正在执行该
        // 任务，执行完毕后会自行 SetEvent 并 free(node)。沙箱进入崩溃态，
        // 后续 submit_task 直接返回 -1，上层据此感知 Kernel 失效。
        ctx->is_crashed = TRUE;
        ctx->is_running = FALSE;
        snprintf(ctx->crash_reason, sizeof(ctx->crash_reason),
                 "Kernel %s task timed out after %dms (callback hung)", ctx->kernel_id,
                 (int)TASK_TIMEOUT_MS);
        if (ctx->crash_event)
        {
            SetEvent(ctx->crash_event);
        }
        return -1;
    }
    // 线程崩溃退出
    CloseHandle(node->task.done_event);
    free(node);
    ctx->is_crashed = TRUE;
    return -1;
}
void *create_sandbox(const char *dll_path)
{
    ensure_tls_index();
    HMODULE hMod = LoadLibraryA(dll_path);
    if (!hMod)
        return NULL;
    typedef CuckooKernelInterface *(*GetIfaceFunc)();
    GetIfaceFunc get_iface =
        (GetIfaceFunc)GetProcAddress(hMod, "get_cuckoo_kernel_interface");
    if (!get_iface)
    {
        FreeLibrary(hMod);
        return NULL;
    }
    CuckooKernelInterface *iface = get_iface();
    if (!iface)
    {
        FreeLibrary(hMod);
        return NULL;
    }
    SandboxContext *ctx = (SandboxContext *)malloc(sizeof(SandboxContext));
    memset(ctx, 0, sizeof(SandboxContext));
    ctx->iface = iface;
    ctx->hModule = hMod;
    ctx->is_running = TRUE;
    // 缓存元数据
    if (iface->kernel_id)
        strncpy(ctx->kernel_id, iface->kernel_id, 63);
    if (iface->kernel_name)
        strncpy(ctx->kernel_name, iface->kernel_name, 63);
    if (iface->version)
        strncpy(ctx->version, iface->version, 31);
    if (iface->runtime)
        strncpy(ctx->runtime, iface->runtime, 31);
    ctx->task_mutex = CreateMutex(NULL, FALSE, NULL);
    ctx->task_event = CreateEvent(NULL, FALSE, FALSE, NULL);
    ctx->crash_event =
        CreateEvent(NULL, TRUE, FALSE, NULL); // 手动重置
    ctx->hThread = CreateThread(NULL, 0, SandboxWorker, ctx, 0, NULL);
    return (void *)ctx;
}
void destroy_sandbox(void *handle)
{
    if (!handle)
        return;
    SandboxContext *ctx = (SandboxContext *)handle;
    ctx->is_running = FALSE;
    SetEvent(ctx->task_event);
    // 等待工作线程退出
    BOOL workerExited =
        (WaitForSingleObject(ctx->hThread, 3000) == WAIT_OBJECT_0);
    // 等待 run_loop 线程退出
    BOOL loopExited = TRUE;
    if (ctx->hLoopThread)
    {
        loopExited = (WaitForSingleObject(ctx->hLoopThread, 3000) == WAIT_OBJECT_0);
    }
    if (!workerExited || !loopExited)
    {
        OutputDebugStringA(
            "[Sandbox] Warning: sandbox threads did not exit cooperatively, "
            "leaking context to avoid heap corruption\n");
        // 仅关闭不被线程引用的句柄
        CloseHandle(ctx->task_mutex);
        CloseHandle(ctx->task_event);
        CloseHandle(ctx->crash_event);
        return;
    }
    // 正常清理路径
    CloseHandle(ctx->hThread);
    if (ctx->hLoopThread)
        CloseHandle(ctx->hLoopThread);
    CloseHandle(ctx->task_mutex);
    CloseHandle(ctx->task_event);
    CloseHandle(ctx->crash_event);
    // 清理队列中残留的任务节点
    TaskNode *node = ctx->task_head;
    while (node)
    {
        TaskNode *next = node->next;
        if (node->task.done_event)
            CloseHandle(node->task.done_event);
        free(node);
        node = next;
    }
    FreeLibrary(ctx->hModule);
    free(ctx);
}
void sandbox_get_meta(void *handle, char *out_id, char *out_name, char *out_ver,
                      char *out_rt)
{
    SandboxContext *ctx = (SandboxContext *)handle;
    if (!ctx)
        return;
    if (out_id)
    {
        strncpy(out_id, ctx->kernel_id, 63);
        out_id[63] = '\0';
    }
    if (out_name)
    {
        strncpy(out_name, ctx->kernel_name, 63);
        out_name[63] = '\0';
    }
    if (out_ver)
    {
        strncpy(out_ver, ctx->version, 31);
        out_ver[31] = '\0';
    }
    if (out_rt)
    {
        strncpy(out_rt, ctx->runtime, 31);
        out_rt[31] = '\0';
    }
}
int sandbox_init_runtime(void *handle, CuckooHostAPI *api)
{
    SandboxContext *ctx = (SandboxContext *)handle;
    SandboxTask t = {.type = TASK_INIT, .api = api};
    return submit_task(ctx, &t);
}
int sandbox_load_plugin(void *handle, const char *path, const char *manifest,
                        CuckooPluginDescriptor *out_desc)
{
    SandboxContext *ctx = (SandboxContext *)handle;
    SandboxTask t = {.type = TASK_LOAD,
                     .plugin_path = _strdup(path),
                     .manifest = _strdup(manifest),
                     .out_desc = out_desc};
    int res = submit_task(ctx, &t);
    free(t.plugin_path);
    free(t.manifest);
    return res;
}
void sandbox_trigger_callback(void *handle, CuckooPluginHandle h,
                              const CuckooListenerRecord *l, const char *p)
{
    SandboxContext *ctx = (SandboxContext *)handle;
    SandboxTask t = {.type = TASK_TRIGGER,
                     .handle = h,
                     .listener = *l,
                     .payload = p ? _strdup(p) : NULL};
    submit_task(ctx, &t);
    free(t.payload);
}
void sandbox_unload_plugin(void *handle, CuckooPluginHandle h)
{
    SandboxContext *ctx = (SandboxContext *)handle;
    SandboxTask t = {.type = TASK_UNLOAD, .handle = h};
    submit_task(ctx, &t);
}
// 启动 run_loop：在专用线程中运行
void sandbox_start_loop_async(void *handle, CuckooHostAPI *api)
{
    SandboxContext *ctx = (SandboxContext *)handle;
    if (ctx->is_crashed || !ctx->is_running)
        return;
    ctx->loop_api = api;
    if (ctx->hLoopThread == NULL)
    {
        ctx->hLoopThread = CreateThread(NULL, 0, RunLoopThread, ctx, 0, NULL);
    }
}
void sandbox_shutdown(void *handle)
{
    SandboxContext *ctx = (SandboxContext *)handle;
    SandboxTask t = {.type = TASK_SHUTDOWN};
    submit_task(ctx, &t);
}
int is_sandbox_alive(void *handle)
{
    SandboxContext *ctx = (SandboxContext *)handle;
    return (ctx && !ctx->is_crashed && ctx->is_running) ? 1 : 0;
}
// 获取最近一次崩溃的原因
void sandbox_get_crash_reason(void *handle, char *buf, int bufsize)
{
    SandboxContext *ctx = (SandboxContext *)handle;
    if (!ctx || !buf || bufsize <= 0)
        return;
    strncpy(buf, ctx->crash_reason, bufsize - 1);
    buf[bufsize - 1] = '\0';
}
// 阻塞等待沙箱崩溃事件触发
void sandbox_wait_for_crash(void *handle)
{
    SandboxContext *ctx = (SandboxContext *)handle;
    if (!ctx || !ctx->crash_event)
        return;
    WaitForSingleObject(ctx->crash_event, INFINITE);
}


