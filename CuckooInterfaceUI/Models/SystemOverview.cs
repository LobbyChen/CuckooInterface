namespace CuckooInterfaceUI.Models
{
    /// <summary>
    /// 系统概览数据
    /// </summary>
    public class SystemOverview
    {
        public int TotalPlugins { get; set; } = 0;
        public int ActivePlugins { get; set; } = 0;
        public int TotalKernels { get; set; } = 0;
        public int ActiveKernels { get; set; } = 0;
        public int EventCountToday { get; set; } = 0;
        public int ErrorCountToday { get; set; } = 0;
        public double CpuUsage { get; set; } = 0;
        public double MemoryUsageMB { get; set; } = 0;
        public string Uptime { get; set; } = "0h 0m";
    }
}


