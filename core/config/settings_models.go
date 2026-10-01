package config

import (
	"encoding/json"
	"fmt"
	"strconv"
)

/*
 * Editor
 */

type EditorType string

const (
	EditorText   EditorType = "text"
	EditorSlider EditorType = "slider"
	EditorSwitch EditorType = "switch"
	EditorSelect EditorType = "select"
	EditorColor  EditorType = "color"
)

// Editor 是“这个 Setting 应该如何编辑”的描述。
// 不负责保存配置，也不负责真正创建 WPF Control。
type Editor interface {
	Type() EditorType
}

/*
 * Text
 */

type TextEditor struct {
	ReadOnly    bool   `json:"readOnly,omitempty"`
	Placeholder string `json:"placeholder,omitempty"`
	MultiLine   bool   `json:"multiLine,omitempty"`
}

func (TextEditor) Type() EditorType {
	return EditorText
}

/*
 * Slider
 */

type SliderEditor struct {
	Min  float64 `json:"min"`
	Max  float64 `json:"max"`
	Step float64 `json:"step"`

	// 可选，例如 "MB"、"秒"
	Unit string `json:"unit,omitempty"`
}

func (SliderEditor) Type() EditorType {
	return EditorSlider
}

/*
 * Switch
 */

// 用于描述某个值在 UI 中对应的文字。
type TextOption struct {
	Value any    `json:"value"`
	Text  string `json:"text"`
}

type SwitchEditor struct {
	States []TextOption `json:"states,omitempty"`
}

func (SwitchEditor) Type() EditorType {
	return EditorSwitch
}

/*
 * Select
 */

type SelectEditor struct {
	Options []TextOption `json:"options"`
}

func (SelectEditor) Type() EditorType {
	return EditorSelect
}

/*
 * Color
 */

type ColorEditor struct {
	Colors  []string `json:"colors"`
	Columns int      `json:"columns,omitempty"`
}

func (ColorEditor) Type() EditorType {
	return EditorColor
}

/*
 * Setting Action
 */

type ActionType string

const (
	ActionBrowseFile      ActionType = "browse-file"
	ActionBrowseDirectory ActionType = "browse-directory"
	ActionReset           ActionType = "reset"
)

type SettingAction struct {
	Key  string     `json:"key"`
	Type ActionType `json:"type"`
	Text string     `json:"text"`
}

/*

 * Setting Definition

 */

type SettingDefinition struct {
	Key         string
	Name        string
	Description string

	DisplayName        bool
	DisplayDescription bool

	Editor  Editor
	Actions []SettingAction
}

/*

 * Setting

 */

// Setting 是供 Page / Section 使用的统一接口。
type Setting interface {
	Key() string

	Definition() SettingDefinition

	Value() any
	DefaultValue() any

	SetValue(any) error
	Reset()

	DecodeAndSet(json.RawMessage) error
}

/*

 * Generic Setting

 */

type TypedSetting[T any] struct {
	definition   SettingDefinition
	value        T
	defaultValue T
}

func NewSetting[T any](
	definition SettingDefinition,
	value T,
	defaultValue T,
) *TypedSetting[T] {
	return &TypedSetting[T]{
		definition:   definition,
		value:        value,
		defaultValue: defaultValue,
	}
}

func (s *TypedSetting[T]) Key() string {
	return s.definition.Key
}

func (s *TypedSetting[T]) Definition() SettingDefinition {
	return s.definition
}

func (s *TypedSetting[T]) Value() any {
	return s.value
}

func (s *TypedSetting[T]) DefaultValue() any {
	return s.defaultValue
}

func (s *TypedSetting[T]) SetValue(v any) error {
	value, ok := v.(T)
	if !ok {
		return fmt.Errorf(
			"setting %q: expected value of type %T, got %T",
			s.definition.Key,
			s.value,
			v,
		)
	}

	s.value = value
	return nil
}

func (s *TypedSetting[T]) Reset() {
	s.value = s.defaultValue
}

func (s *TypedSetting[T]) DecodeAndSet(raw json.RawMessage) error {
	var value T

	if err := json.Unmarshal(raw, &value); err != nil {
		return fmt.Errorf(
			"setting %q: decode value: %w",
			s.definition.Key,
			err,
		)
	}

	s.value = value
	return nil
}

/*

 * Editor JSON

 */

func MarshalEditor(editor Editor) (json.RawMessage, error) {
	if editor == nil {
		return nil, nil
	}

	raw, err := json.Marshal(editor)
	if err != nil {
		return nil, err
	}

	var object map[string]json.RawMessage

	if err := json.Unmarshal(raw, &object); err != nil {
		return nil, fmt.Errorf(
			"editor %q must serialize to a JSON object",
			editor.Type(),
		)
	}

	object["type"] = json.RawMessage(
		strconv.Quote(string(editor.Type())),
	)

	return json.Marshal(object)
}

func UnmarshalEditor(raw json.RawMessage) (Editor, error) {
	var meta struct {
		Type EditorType `json:"type"`
	}

	if err := json.Unmarshal(raw, &meta); err != nil {
		return nil, err
	}

	switch meta.Type {
	case EditorText:
		var editor TextEditor
		if err := json.Unmarshal(raw, &editor); err != nil {
			return nil, err
		}
		return editor, nil

	case EditorSlider:
		var editor SliderEditor
		if err := json.Unmarshal(raw, &editor); err != nil {
			return nil, err
		}
		return editor, nil

	case EditorSwitch:
		var editor SwitchEditor
		if err := json.Unmarshal(raw, &editor); err != nil {
			return nil, err
		}
		return editor, nil

	case EditorSelect:
		var editor SelectEditor
		if err := json.Unmarshal(raw, &editor); err != nil {
			return nil, err
		}
		return editor, nil

	case EditorColor:
		var editor ColorEditor
		if err := json.Unmarshal(raw, &editor); err != nil {
			return nil, err
		}
		return editor, nil

	default:
		return nil, fmt.Errorf(
			"unknown editor type %q",
			meta.Type,
		)
	}
}
func (s *TypedSetting[T]) MarshalJSON() ([]byte, error) {
	editor, err := MarshalEditor(s.definition.Editor)
	if err != nil {
		return nil, err
	}

	type WireSetting struct {
		Key         string `json:"key"`
		Name        string `json:"name"`
		Description string `json:"description,omitempty"`

		DisplayName        bool `json:"displayName"`
		DisplayDescription bool `json:"displayDescription"`

		Value        T `json:"value"`
		DefaultValue T `json:"defaultValue"`

		Editor  json.RawMessage `json:"editor"`
		Actions []SettingAction `json:"actions,omitempty"`
	}

	return json.Marshal(WireSetting{
		Key:                s.definition.Key,
		Name:               s.definition.Name,
		Description:        s.definition.Description,
		DisplayName:        s.definition.DisplayName,
		DisplayDescription: s.definition.DisplayDescription,
		Value:              s.value,
		DefaultValue:       s.defaultValue,
		Editor:             editor,
		Actions:            s.definition.Actions,
	})
}

/*

 * Page / Section / Panel

 */

type SettingSection struct {
	Key      string    `json:"key"`
	Name     string    `json:"name"`
	Settings []Setting `json:"settings"`
}

type SettingPage struct {
	Key      string           `json:"key"`
	Name     string           `json:"name"`
	Sections []SettingSection `json:"sections"`
}

type SettingPanel struct {
	Pages []SettingPage `json:"pages"`
}
