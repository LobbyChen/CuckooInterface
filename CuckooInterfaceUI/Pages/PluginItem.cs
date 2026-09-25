using System.ComponentModel;
using System.Runtime.CompilerServices;
using System.Windows.Media;

namespace CuckooInterfaceUI.Pages
{
    /// <summary>
    /// 插件数据项（硬编码示例，后续接入后端 IPC 替换）
    /// </summary>
    public class PluginItem : INotifyPropertyChanged
    {
        private bool _enabled;

        public string Name { get; set; } = string.Empty;
        public string Version { get; set; } = string.Empty;
        public string TypeText { get; set; } = string.Empty;
        public string Description { get; set; } = string.Empty;
        public string StatusText { get; set; } = string.Empty;
        public Brush StatusColor { get; set; } = Brushes.Gray;

        public bool Enabled
        {
            get => _enabled;
            set
            {
                _enabled = value;
                OnPropertyChanged();
            }
        }

        public event PropertyChangedEventHandler? PropertyChanged;

        protected void OnPropertyChanged([CallerMemberName] string? name = null)
        {
            PropertyChanged?.Invoke(this, new PropertyChangedEventArgs(name));
        }
    }
}
