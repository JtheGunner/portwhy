package cli

import (
	"bytes"
	"testing"
)

func TestRootCmd_VersionFlagPrintsVersion(t *testing.T) {
	var out bytes.Buffer
	cmd := NewRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"--version"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() returned error: %v", err)
	}

	if got, want := out.String(), "portwhy dev\n"; got != want {
		t.Errorf("version output = %q, want %q", got, want)
	}
}
