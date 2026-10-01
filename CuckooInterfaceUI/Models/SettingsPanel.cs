using System.Collections.Generic;
using System.Text.Json.Serialization;

namespace CuckooInterfaceUI.Models
{
    /*
     * 与 Go 端 core/config/global.go 对齐的数据模型。
     * 后端通过 IPC 下发 SettingPanel JSON，前端据此动态渲染设置界面。
     */

    // ===== Editor 类型 =====

    public enum EditorType
    {
        Text,
        Slider,
        Switch,
        Select,
        Color
    }

    /// <summary>
    /// 编辑器描述基类。由 Type 字段区分具体类型（对应 Go 的多态 Editor）。
    /// </summary>
    public abstract class Editor
    {
        [JsonPropertyName("type")]
        public abstract EditorType Type { get; }
    }

    public class TextEditor : Editor
    {
        public override EditorType Type => EditorType.Text;

        [JsonPropertyName("readOnly")]
        public bool ReadOnly { get; set; }

        [JsonPropertyName("placeholder")]
        public string Placeholder { get; set; } = string.Empty;

        [JsonPropertyName("multiLine")]
        public bool MultiLine { get; set; }
    }

    public class SliderEditor : Editor
    {
        public override EditorType Type => EditorType.Slider;

        [JsonPropertyName("min")]
        public double Min { get; set; }

        [JsonPropertyName("max")]
        public double Max { get; set; }

        [JsonPropertyName("step")]
        public double Step { get; set; }

        [JsonPropertyName("unit")]
        public string Unit { get; set; } = string.Empty;
    }

    public class TextOption
    {
        [JsonPropertyName("value")]
        public object Value { get; set; } = new();

        [JsonPropertyName("text")]
        public string Text { get; set; } = string.Empty;
    }

    public class SwitchEditor : Editor
    {
        public override EditorType Type => EditorType.Switch;

        [JsonPropertyName("states")]
        public List<TextOption> States { get; set; } = new();
    }

    public class SelectEditor : Editor
    {
        public override EditorType Type => EditorType.Select;

        [JsonPropertyName("options")]
        public List<TextOption> Options { get; set; } = new();
    }

    public class ColorEditor : Editor
    {
        public override EditorType Type => EditorType.Color;

        [JsonPropertyName("colors")]
        public List<string> Colors { get; set; } = new();

        [JsonPropertyName("columns")]
        public int Columns { get; set; }
    }

    // ===== Setting Action =====

    public enum ActionType
    {
        BrowseFile,
        BrowseDirectory,
        Reset
    }

    public class SettingAction
    {
        [JsonPropertyName("key")]
        public string Key { get; set; } = string.Empty;

        [JsonPropertyName("type")]
        public ActionType Type { get; set; }

        [JsonPropertyName("text")]
        public string Text { get; set; } = string.Empty;
    }

    // ===== Setting 定义 =====

    public class SettingDefinition
    {
        [JsonPropertyName("key")]
        public string Key { get; set; } = string.Empty;

        [JsonPropertyName("name")]
        public string Name { get; set; } = string.Empty;

        [JsonPropertyName("description")]
        public string Description { get; set; } = string.Empty;

        [JsonPropertyName("displayName")]
        public bool DisplayName { get; set; }

        [JsonPropertyName("displayDescription")]
        public bool DisplayDescription { get; set; }

        [JsonPropertyName("editor")]
        public Editor Editor { get; set; } = new TextEditor();

        [JsonPropertyName("actions")]
        public List<SettingAction> Actions { get; set; } = new();
    }

    /// <summary>
    /// 单个设置项（值与元数据合一，对应 Go 端 TypedSetting 的序列化结果）。
    /// </summary>
    public class Setting
    {
        [JsonPropertyName("key")]
        public string Key { get; set; } = string.Empty;

        [JsonPropertyName("name")]
        public string Name { get; set; } = string.Empty;

        [JsonPropertyName("description")]
        public string Description { get; set; } = string.Empty;

        [JsonPropertyName("displayName")]
        public bool DisplayName { get; set; }

        [JsonPropertyName("displayDescription")]
        public bool DisplayDescription { get; set; }

        [JsonPropertyName("editor")]
        public Editor Editor { get; set; } = new TextEditor();

        [JsonPropertyName("actions")]
        public List<SettingAction> Actions { get; set; } = new();

        [JsonPropertyName("value")]
        public object Value { get; set; } = new();

        [JsonPropertyName("defaultValue")]
        public object DefaultValue { get; set; } = new();
    }

    // ===== 层级结构 =====

    public class SettingSection
    {
        [JsonPropertyName("key")]
        public string Key { get; set; } = string.Empty;

        [JsonPropertyName("name")]
        public string Name { get; set; } = string.Empty;

        [JsonPropertyName("settings")]
        public List<Setting> Settings { get; set; } = new();
    }

    public class SettingPage
    {
        [JsonPropertyName("key")]
        public string Key { get; set; } = string.Empty;

        [JsonPropertyName("name")]
        public string Name { get; set; } = string.Empty;

        [JsonPropertyName("sections")]
        public List<SettingSection> Sections { get; set; } = new();
    }

    public class SettingPanel
    {
        [JsonPropertyName("pages")]
        public List<SettingPage> Pages { get; set; } = new();
    }
}
