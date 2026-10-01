package IPC

type SystemOverview struct {
	TotalPlugins  int     `json:"totalPlugins"`  // 总插件数
	ActivePlugins int     `json:"activePlugins"` // 激活插件数
	KernelCount   int     `json:"kernelCount"`   // 内核数
	TodayEvents   int     `json:"todayEvents"`   // 今日事件数
	ErrorCount    int     `json:"errorCount"`    // 错误数
	CPUUsage      float64 `json:"cpuUsage"`      // CPU使用率(%)
	MemoryUsage   float64 `json:"memoryUsage"`   // 内存使用率(%)
	Uptime        string  `json:"uptime"`        // 运行时长(格式: "2d 3h 15m")
}
