using System;
using System.Threading.Tasks;
using System.Windows.Controls;
using System.Windows.Threading;

namespace CuckooInterfaceUI.Pages
{
    /// <summary>
    /// 支持数据自动刷新的页面基类：页面可见时每 2 秒触发一次 <see cref="OnAutoRefreshAsync"/>。
    /// <para>
    /// 刷新在 UI 线程触发，带防重入保护（上一次刷新未完成时跳过本次）；
    /// 页面 Unloaded 后自动停止，重新 Loaded 时自动恢复。
    /// </para>
    /// </summary>
    public abstract class AutoRefreshPage : Page
    {
        /// <summary>自动刷新间隔。</summary>
        public static readonly TimeSpan RefreshInterval = TimeSpan.FromSeconds(2);

        private readonly DispatcherTimer _refreshTimer;
        private bool _isRefreshing;

        protected AutoRefreshPage()
        {
            _refreshTimer = new DispatcherTimer { Interval = RefreshInterval };
            _refreshTimer.Tick += async (_, _) => await TickAsync();

            Loaded += (_, _) => _refreshTimer.Start();
            Unloaded += (_, _) => _refreshTimer.Stop();
        }

        private async Task TickAsync()
        {
            if (_isRefreshing || !IsVisible)
                return;

            _isRefreshing = true;
            try
            {
                await OnAutoRefreshAsync();
            }
            catch
            {
                // Tick 是 async void，异常逃逸会拖垮进程；
                // 常规错误应由各页面的加载方法内部处理，这里只做兜底。
            }
            finally
            {
                _isRefreshing = false;
            }
        }

        /// <summary>
        /// 自动刷新回调：页面可见时每 2 秒调用一次。
        /// 页面实现中应检查后端连接状态并自行处理断连展示。
        /// </summary>
        protected abstract Task OnAutoRefreshAsync();
    }
}
