package skillstate

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/skillprovenance"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

func TestCrossPlatformCoverageSkillStateReadWriteRemoveAndErrors(t *testing.T) {
	t.Setenv("DWS_CONFIG_DIR", "")
	home := t.TempDir()
	if state, readable, err := Read(home); err != nil || readable || state != nil {
		t.Fatalf("missing = %#v, %v, %v", state, readable, err)
	}
	want := State{
		Version:        "1.2.3",
		OfficialSkills: []string{"dingtalk-b", "dingtalk-a", "dingtalk-a"},
		UpdatedSkills:  []string{"dingtalk-a"},
		ManagedSkills: []skillprovenance.Record{
			{Name: "dingtalk-b", Version: "old"},
			{Name: "dingtalk-a", Version: "1"},
			{Name: "dingtalk-b", Version: "2"},
		},
	}
	if err := Write(home, want); err != nil {
		t.Fatal(err)
	}
	got, readable, err := Read(home)
	if err != nil || !readable || !reflect.DeepEqual(got.OfficialSkills, []string{"dingtalk-a", "dingtalk-b"}) {
		t.Fatalf("round trip = %#v, %v, %v", got, readable, err)
	}
	if !reflect.DeepEqual(got.ManagedSkills, []skillprovenance.Record{{Name: "dingtalk-a", Version: "1"}, {Name: "dingtalk-b", Version: "2"}}) {
		t.Fatalf("managed skills = %#v", got.ManagedSkills)
	}
	if names := ManagedSkillNames(got); !reflect.DeepEqual(names, map[string]bool{"dingtalk-a": true, "dingtalk-b": true}) {
		t.Fatalf("managed names = %#v", names)
	}
	if names := ManagedSkillNames(nil); len(names) != 0 {
		t.Fatalf("nil managed names = %#v", names)
	}
	if err := Remove(home); err != nil {
		t.Fatal(err)
	}
	if err := Remove(home); err != nil {
		t.Fatal(err)
	}
	badHome := t.TempDir()
	if err := os.MkdirAll(filepath.Dir(Path(badHome)), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(Path(badHome), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Read(badHome); err == nil || !strings.Contains(err.Error(), "不可读") {
		t.Fatalf("malformed = %v", err)
	}
	blocked := filepath.Join(t.TempDir(), "home-file")
	if err := os.WriteFile(blocked, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Write(blocked, State{}); err == nil {
		t.Fatal("blocked write succeeded")
	}
	failure := errors.New("denied")
	testseam.Swap(t, &skillStateReadFile, func(string) ([]byte, error) { return nil, failure })
	if _, _, err := Read(blocked); !errors.Is(err, failure) {
		t.Fatal("blocked read succeeded")
	}
	testseam.Swap(t, &skillStateRemove, func(string) error { return failure })
	if err := Remove(blocked); !errors.Is(err, failure) {
		t.Fatal("blocked remove succeeded")
	}
	configured := t.TempDir()
	t.Setenv("DWS_CONFIG_DIR", "  "+configured+"  ")
	if Path("ignored") != filepath.Join(configured, stateFile) {
		t.Fatal("configured path ignored")
	}
}

func TestCrossPlatformCoverageIsLegacyOfficialSkillName(t *testing.T) {
	for _, name := range []string{"dingtalk-aitable", "dingtalk-devdoc", "dws-shared"} {
		if !IsLegacyOfficialSkillName(name) {
			t.Fatalf("historical official Skill %q not recognized", name)
		}
	}
	for _, name := range []string{"dingtalk-custom", "other-skill", ""} {
		if IsLegacyOfficialSkillName(name) {
			t.Fatalf("user Skill %q must not be treated as historical official", name)
		}
	}
}
