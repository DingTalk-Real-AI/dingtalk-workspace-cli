package audit

import "time"

type Event struct {
	Timestamp     time.Time `json:"ts"`
	ExecutionID   string    `json:"execution_id"`
	AgentID       string    `json:"agent_id,omitempty"`
	Actor         Actor     `json:"actor"`
	Product       string    `json:"product"`
	Command       string    `json:"command"`
	Endpoint      string    `json:"endpoint"`
	ParamsSummary string    `json:"params_summary,omitempty"`
	Result        string    `json:"result"`
	ErrCategory   string    `json:"error_category,omitempty"`
	ErrReason     string    `json:"error_reason,omitempty"`
	DurationMs    int64     `json:"duration_ms"`
	CLIVersion    string    `json:"cli_version"`
	OS            string    `json:"os"`
	Arch          string    `json:"arch"`
	PrevHash      string    `json:"prev_hash"`
	Hash          string    `json:"hash"`
}

type Actor struct {
	UserID   string `json:"user_id"`
	Name     string `json:"name,omitempty"`
	CorpID   string `json:"corp_id"`
	CorpName string `json:"corp_name,omitempty"`
}
