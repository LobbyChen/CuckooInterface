// Copyright (C) 2026 LobbyChen
// SPDX-License-Identifier: AGPL-3.0-only
using System.Collections.Generic;
using System.Text.Json.Serialization;

namespace CuckooInterfaceUI.Models
{
    /// <summary>
    /// 与 Core SaveSettings 返回结果对齐。
    /// </summary>
    public class SettingsApplyResult
    {
        [JsonPropertyName("saved")]
        public bool Saved { get; set; }

        [JsonPropertyName("applied")]
        public List<string> Applied { get; set; } = new();

        [JsonPropertyName("requiresRestart")]
        public List<string> RequiresRestart { get; set; } = new();

        [JsonPropertyName("errors")]
        public Dictionary<string, string> Errors { get; set; } = new();
    }
}
