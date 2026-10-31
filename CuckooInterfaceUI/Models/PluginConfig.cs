// Copyright (C) 2026 LobbyChen
// SPDX-License-Identifier: AGPL-3.0-only
namespace CuckooInterfaceUI.Models
{
    /// <summary>
    /// 单个插件的配置快照，对应 Core 的 getPluginConfig 响应。
    /// Config 为插件配置字符串（通常为 JSON）；ConfigPage 为配置页 HTML 的绝对路径。
    /// </summary>
    public class PluginConfig
    {
        public string PluginId { get; set; } = string.Empty;

        public string Config { get; set; } = string.Empty;

        public string ConfigPage { get; set; } = string.Empty;

        /// <summary>插件是否在 META-INF.json 中声明了 config_page。</summary>
        public bool HasConfigPage => !string.IsNullOrWhiteSpace(ConfigPage);
    }
}
