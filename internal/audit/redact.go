package audit

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

type RedactLevel string

const (
	RedactNone    RedactLevel = "none"
	RedactHashed  RedactLevel = "hashed"
	RedactMinimal RedactLevel = "minimal"
)

func RedactEvent(evt Event, level RedactLevel) Event {
	switch level {
	case RedactHashed:
		if evt.Actor.Name != "" {
			evt.Actor.Name = hashString(evt.Actor.Name)
		}
		if evt.Actor.CorpName != "" {
			evt.Actor.CorpName = hashString(evt.Actor.CorpName)
		}
		evt.ParamsSummary = ""
	case RedactMinimal:
		evt.Actor = Actor{UserID: hashString(evt.Actor.UserID), CorpID: hashString(evt.Actor.CorpID)}
		evt.ParamsSummary = ""
		evt.Endpoint = ""
		evt.ErrReason = ""
		evt.AgentID = ""
		evt.PrevHash = ""
		evt.Hash = ""
	}
	return evt
}

func RedactEventJSON(evt Event, level RedactLevel) ([]byte, error) {
	redacted := RedactEvent(evt, level)
	return json.Marshal(redacted)
}

func hashString(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:8])
}
