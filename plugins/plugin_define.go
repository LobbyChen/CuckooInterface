package plugins

import "CuckooInterface/core/constant"

// 插件元数据
type PluginMetaData struct {
	Name        string            `json:"name"`                  // 插件名称
	ID          string            `json:"id"`                    // 插件ID,不可重复
	Type        constant.PlugType `json:"type"`                  // 插件类型
	Version     string            `json:"version"`               // 语义化版本
	Description string            `json:"description"`           // 描述
	RuntimeType string            `json:"runtime_type"`          // 运行时类型
	Icon        string            `json:"icon,omitempty"`        // 图标路径 icon/png
	ConfigPage  string            `json:"config_page,omitempty"` // 配置页 HTML 相对路径 可选
	Tag         []string          `json:"tag,omitempty"`         // 插件Tag 可选
}
