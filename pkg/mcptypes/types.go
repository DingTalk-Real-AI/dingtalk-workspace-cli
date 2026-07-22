package mcptypes

import (
	"bytes"
	"encoding/json"
	"strings"
)

type ServerDescriptor struct {
	Key         string
	DisplayName string
	Description string
	Endpoint    string
	Source      string
	CLI         CLIOverlay
	HasCLIMeta  bool
	AuthHeaders map[string]string
}

type CLIOverlay struct {
	ID            string                     `json:"id"`
	Command       string                     `json:"command"`
	Parent        string                     `json:"parent,omitempty"`
	Description   string                     `json:"description,omitempty"`
	Aliases       []string                   `json:"aliases"`
	Prefixes      []string                   `json:"prefixes"`
	Group         string                     `json:"group,omitempty"`
	Skip          bool                       `json:"skip"`
	Hidden        bool                       `json:"hidden,omitempty"`
	Tools         []CLITool                  `json:"tools"`
	Groups        map[string]CLIGroupDef     `json:"groups,omitempty"`
	ToolOverrides map[string]CLIToolOverride `json:"toolOverrides,omitempty"`
	ServerDeps    []string                   `json:"serverDeps,omitempty"`
	Hints         map[string]json.RawMessage `json:"hintCommands,omitempty"`
	RedirectTo    string                     `json:"redirectTo,omitempty"`
}

type CLIGroupDef struct {
	Description string `json:"description,omitempty"`
}

type CLITool struct {
	Name        string                 `json:"name"`
	CLIName     string                 `json:"cliName,omitempty"`
	Title       string                 `json:"title,omitempty"`
	Description string                 `json:"description,omitempty"`
	IsSensitive bool                   `json:"isSensitive,omitempty"`
	Category    string                 `json:"category,omitempty"`
	Hidden      bool                   `json:"hidden,omitempty"`
	Flags       map[string]CLIFlagHint `json:"flags,omitempty"`
}

type CLIFlagHint struct {
	Shorthand string `json:"shorthand,omitempty"`
	Alias     string `json:"alias,omitempty"`
}

type CLIToolOverride struct {
	CLIName           string                     `json:"cliName,omitempty"`
	CLIAliases        []string                   `json:"cliAliases,omitempty"`
	Description       string                     `json:"description,omitempty"`
	Example           string                     `json:"example,omitempty"`
	Group             string                     `json:"group,omitempty"`
	IsSensitive       bool                       `json:"isSensitive,omitempty"`
	Hidden            bool                       `json:"hidden,omitempty"`
	Flags             map[string]CLIFlagOverride `json:"flags,omitempty"`
	OutputFormat      map[string]any             `json:"outputFormat,omitempty"`
	ServerOverride    string                     `json:"serverOverride,omitempty"`
	BodyWrapper       string                     `json:"bodyWrapper,omitempty"`
	MutuallyExclusive [][]string                 `json:"mutuallyExclusive,omitempty"`
	RequireOneOf      [][]string                 `json:"requireOneOf,omitempty"`
	RequireTogether   [][]string                 `json:"requireTogether,omitempty"`
	RejectPositional  bool                       `json:"rejectPositional,omitempty"`
	RedirectTo        string                     `json:"redirectTo,omitempty"`
	Pipeline          []json.RawMessage          `json:"pipeline,omitempty"`
}

type CLIFlagOverride struct {
	Alias           string         `json:"alias,omitempty"`
	Aliases         []string       `json:"aliases,omitempty"`
	MapsTo          string         `json:"mapsTo,omitempty"`
	Transform       string         `json:"transform,omitempty"`
	TransformArgs   map[string]any `json:"transformArgs,omitempty"`
	EnvDefault      string         `json:"envDefault,omitempty"`
	Hidden          bool           `json:"hidden,omitempty"`
	Default         string         `json:"default,omitempty"`
	Shorthand       string         `json:"shorthand,omitempty"`
	Required        bool           `json:"required,omitempty"`
	Description     string         `json:"description,omitempty"`
	Positional      bool           `json:"positional,omitempty"`
	PositionalIndex int            `json:"positionalIndex,omitempty"`
	Type            string         `json:"type,omitempty"`
	OmitWhen        string         `json:"omitWhen,omitempty"`
	RuntimeDefault  string         `json:"runtimeDefault,omitempty"`
	PipelineLocal   bool           `json:"pipelineLocal,omitempty"`
}

func (override *CLIFlagOverride) UnmarshalJSON(data []byte) error {
	type alias CLIFlagOverride
	aux := struct {
		Default json.RawMessage `json:"default,omitempty"`
		*alias
	}{alias: (*alias)(override)}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&aux); err != nil {
		return err
	}
	override.Default = coercePluginScalar(aux.Default)
	return nil
}

func coercePluginScalar(raw json.RawMessage) string {
	value := strings.TrimSpace(string(raw))
	if value == "" || value == "null" ||
		strings.HasPrefix(value, "{") || strings.HasPrefix(value, "[") {
		return ""
	}
	if strings.HasPrefix(value, `"`) {
		var decoded string
		if json.Unmarshal(raw, &decoded) == nil {
			return decoded
		}
		return ""
	}
	return value
}

func OverlayFromJSON(data json.RawMessage) CLIOverlay {
	var overlay CLIOverlay
	if len(data) > 0 {
		_ = json.Unmarshal(data, &overlay)
	}
	return overlay
}
