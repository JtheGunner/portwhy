package launchd

import (
	"os"
	"path/filepath"
	"testing"
)

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestParseLaunchctlList_MapsPIDToLabel(t *testing.T) {
	got := parseLaunchctlList(fixture(t, "launchctl_list.txt"))

	want := map[int]string{
		1187:  "com.apple.progressd",
		1113:  "com.apple.rapportd",
		89875: "com.apple.Finder",
		3884:  "homebrew.mxcl.php@8.4",
		3863:  "ai.openclaw.gateway",
		13250: "application.com.example.player.115990561.115991375",
		4100:  "com.example.toolbox",
	}
	if len(got) != len(want) {
		t.Errorf("parsed %d entries, want %d: %v", len(got), len(want), got)
	}
	for pid, label := range want {
		if got[pid] != label {
			t.Errorf("label of PID %d = %q, want %q", pid, got[pid], label)
		}
	}
}

func TestParseLaunchctlList_SkipsUnusableLines(t *testing.T) {
	tests := []struct {
		name string
		in   string
	}{
		{"empty", ""},
		{"header only", "PID\tStatus\tLabel\n"},
		{"dash pid", "-\t0\tcom.example.idle\n"},
		{"zero pid", "0\t0\tcom.example.zero\n"},
		{"negative pid", "-5\t0\tcom.example.neg\n"},
		{"missing label", "123\t0\n"},
		{"empty label", "123\t0\t\n"},
		{"not tab separated", "123 0 com.example.spaces\n"},
		{"garbage", "this is not launchctl output\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parseLaunchctlList([]byte(tt.in)); len(got) != 0 {
				t.Errorf("parseLaunchctlList(%q) = %v, want no entries", tt.in, got)
			}
		})
	}
}

func TestParseLaunchctlList_HandlesWindowsLineEndings(t *testing.T) {
	got := parseLaunchctlList([]byte("PID\tStatus\tLabel\r\n42\t0\tcom.example.a\r\n"))

	if got[42] != "com.example.a" {
		t.Errorf("label of PID 42 = %q, want com.example.a", got[42])
	}
}
