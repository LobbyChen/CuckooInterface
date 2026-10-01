using System.Collections.Generic;
using System.ComponentModel;
using System.Runtime.CompilerServices;
using System.Windows.Media;
using Brush = System.Windows.Media.Brush;
using Color = System.Windows.Media.Color;

namespace CuckooInterfaceUI.Models
{
    /// <summary>
    /// 插件类型枚举
    /// </summary>
    public enum PluginType
    {
        Kernel,
        Base,
        Active
    }

    /// <summary>
    /// 插件信息
    /// </summary>
    public class PluginInfo : INotifyPropertyChanged
    {
        private bool _isEnabled;
        private bool _isLoaded;

        public string Id { get; set; } = string.Empty;
        public string Name { get; set; } = string.Empty;
        public string Version { get; set; } = "1.0.0";
        public string Description { get; set; } = string.Empty;
        public PluginType Type { get; set; } = PluginType.Base;
        public string Author { get; set; } = string.Empty;
        public List<string> ProvidedEvents { get; set; } = new();
        public List<string> Listeners { get; set; } = new();

        public bool IsEnabled
        {
            get => _isEnabled;
            set { _isEnabled = value; OnPropertyChanged(); OnPropertyChanged(nameof(StatusText)); OnPropertyChanged(nameof(StatusColor)); }
        }

        public bool IsLoaded
        {
            get => _isLoaded;
            set { _isLoaded = value; OnPropertyChanged(); OnPropertyChanged(nameof(StatusText)); OnPropertyChanged(nameof(StatusColor)); }
        }

        public string TypeText => Type switch
        {
            PluginType.Kernel => "内核",
            PluginType.Base => "基础",
            PluginType.Active => "应用",
            _ => "未知"
        };

        public string StatusText
        {
            get
            {
                if (!IsEnabled) return "已禁用";
                if (IsLoaded) return "运行中";
                return "加载失败";
            }
        }

        public Brush StatusColor
        {
            get
            {
                if (!IsEnabled) return new SolidColorBrush(Color.FromRgb(0x8A, 0x8A, 0x8A));
                if (IsLoaded) return new SolidColorBrush(Color.FromRgb(0x43, 0xB5, 0x81));
                return new SolidColorBrush(Color.FromRgb(0xE5, 0x48, 0x4D));
            }
        }

        public event PropertyChangedEventHandler? PropertyChanged;

        protected void OnPropertyChanged([CallerMemberName] string? name = null)
        {
            PropertyChanged?.Invoke(this, new PropertyChangedEventArgs(name));
        }
    }
}
