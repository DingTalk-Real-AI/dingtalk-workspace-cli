package mcptypes

import "encoding/json"

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
	Aliases       []string                   `json:"aliases"`
	Prefixes      []string                   `json:"prefixes"`
	Skip          bool                       `json:"skip"`
	Tools         []CLITool                  `json:"tools"`
	ToolOverrides map[string]CLIToolOverride `json:"toolOverrides,omitempty"`
	Groups        map[string]GroupMeta       `json:"groups,omitempty"`
}

type CLITool struct {
	Name string `json:"name"`
}

type CLIToolOverride struct {
	ServerOverride string         `json:"serverOverride,omitempty"`
	CLIName        string         `json:"cliName,omitempty"`
	Group          string         `json:"group,omitempty"`
	Description    string         `json:"description,omitempty"`
	Hidden         bool           `json:"hidden,omitempty"`
	IsSensitive    bool           `json:"isSensitive,omitempty"`
	Flags          map[string]any `json:"flags,omitempty"`
}

type GroupMeta struct {
	Description string `json:"description,omitempty"`
}

func OverlayFromJSON(data json.RawMessage) CLIOverlay {
	var overlay CLIOverlay
	if len(data) > 0 {
		_ = json.Unmarshal(data, &overlay)
	}
	return overlay
}
