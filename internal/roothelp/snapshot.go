package roothelp

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"unicode/utf8"
)

const MaxSnapshotBytes = 256 << 10

// ErrSnapshotMismatch is the stable classification for a structurally valid
// snapshot that is bound to a different finalized package identity.
var ErrSnapshotMismatch = errors.New("help snapshot does not match finalized core")

// Snapshot is a disposable build derivative, sealed into the launcher only
// after comparing both locale projections with the exact finalized core.
// It is never read from user configuration or a runtime cache.
type Snapshot struct {
	Version    int
	Edition    string
	Commit     string
	CoreSHA256 string
	English    Model
	Chinese    Model
}

func (s Snapshot) validate() error {
	if s.Version != 1 || s.Edition != "open" || !lowerHex(s.Commit, 40) || !lowerHex(s.CoreSHA256, 64) {
		return errors.New("invalid help snapshot identity")
	}
	for _, model := range []Model{s.English, s.Chinese} {
		if model.Long == "" || len(model.Services) == 0 || len(model.Utilities) == 0 || len(model.Flags) == 0 || len(model.Services) > 128 || len(model.Utilities) > 128 || len(model.Flags) > 128 {
			return errors.New("incomplete or oversized help projection")
		}
		for _, commands := range [][]Command{model.Services, model.Utilities} {
			seen := map[string]bool{}
			for _, cmd := range commands {
				if cmd.Name == "" || strings.TrimSpace(cmd.Name) != cmd.Name || seen[cmd.Name] {
					return errors.New("invalid help command projection")
				}
				seen[cmd.Name] = true
			}
		}
	}
	return nil
}
func lowerHex(value string, size int) bool {
	if len(value) != size || strings.ToLower(value) != value {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func EncodeSnapshot(snapshot Snapshot) (string, error) {
	if err := snapshot.validate(); err != nil {
		return "", err
	}
	data, err := json.Marshal(snapshot)
	if err != nil {
		return "", err
	}
	if len(data) > MaxSnapshotBytes {
		return "", errors.New("help snapshot exceeds size limit")
	}
	return base64.RawStdEncoding.EncodeToString(data), nil
}

func DecodeSnapshot(encoded, coreSHA256, commit, edition, locale string) (Model, error) {
	if len(encoded) > base64.RawStdEncoding.EncodedLen(MaxSnapshotBytes) {
		return Model{}, errors.New("help snapshot exceeds size limit")
	}
	data, err := base64.RawStdEncoding.Strict().DecodeString(encoded)
	if err != nil || !utf8.Valid(data) || base64.RawStdEncoding.EncodeToString(data) != encoded {
		return Model{}, errors.New("invalid help snapshot encoding")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var snapshot Snapshot
	if err := decoder.Decode(&snapshot); err != nil {
		return Model{}, err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return Model{}, errors.New("trailing help snapshot data")
	}
	if err := snapshot.validate(); err != nil {
		return Model{}, err
	}
	canonical, err := json.Marshal(snapshot)
	if err != nil || !bytes.Equal(canonical, data) {
		return Model{}, errors.New("non-canonical help snapshot")
	}
	if snapshot.CoreSHA256 != coreSHA256 || snapshot.Commit != commit || snapshot.Edition != edition {
		return Model{}, ErrSnapshotMismatch
	}
	switch locale {
	case "en":
		return snapshot.English, nil
	case "zh":
		return snapshot.Chinese, nil
	default:
		return Model{}, errors.New("unsupported help locale")
	}
}
