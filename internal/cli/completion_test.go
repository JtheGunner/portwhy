package cli

import (
	"bytes"
	"slices"
	"strings"
	"testing"
)

// noFileCompDirective is cobra's ShellCompDirectiveNoFileComp as printed by the
// hidden __complete command.
const noFileCompDirective = ":4"

// complete runs the hidden __complete command and returns the suggested
// candidates plus the trailing directive line.
func complete(t *testing.T, args ...string) (candidates []string, directive string) {
	t.Helper()
	var out bytes.Buffer
	cmd := NewRootCmd()
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs(append([]string{"__complete"}, args...))

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() returned error: %v", err)
	}

	var lines []string
	for _, line := range strings.Split(strings.TrimRight(out.String(), "\n"), "\n") {
		// Skip cobra's "Completion ended with directive: ..." debug line.
		if strings.HasPrefix(line, "Completion ended") {
			continue
		}
		lines = append(lines, line)
	}
	if len(lines) == 0 {
		t.Fatalf("no output from __complete %q", args)
	}
	return lines[:len(lines)-1], lines[len(lines)-1]
}

func TestCompletion_SourceSuggestsAllKnownSources(t *testing.T) {
	got, directive := complete(t, "--source", "")

	want := []string{"docker", "launchd", "process", "systemd"}
	slices.Sort(got)
	if !slices.Equal(got, want) {
		t.Errorf("candidates = %v, want %v", got, want)
	}
	if directive != noFileCompDirective {
		t.Errorf("directive = %q, want %q", directive, noFileCompDirective)
	}
}

func TestCompletion_SourceFiltersByPrefix(t *testing.T) {
	got, _ := complete(t, "--source", "d")

	if want := []string{"docker"}; !slices.Equal(got, want) {
		t.Errorf("candidates = %v, want %v", got, want)
	}
}

func TestCompletion_SourceCompletesAfterComma(t *testing.T) {
	got, _ := complete(t, "--source", "docker,s")

	if want := []string{"docker,systemd"}; !slices.Equal(got, want) {
		t.Errorf("candidates = %v, want %v", got, want)
	}
}

func TestCompletion_SourceSkipsAlreadyChosenValues(t *testing.T) {
	got, _ := complete(t, "--source", "docker,")

	if slices.Contains(got, "docker,docker") {
		t.Errorf("candidates %v repeat an already chosen source", got)
	}
	if want := 3; len(got) != want {
		t.Errorf("got %d candidates %v, want %d", len(got), got, want)
	}
}

func TestCompletion_SourceUnknownPrefixSuggestsNothing(t *testing.T) {
	got, directive := complete(t, "--source", "x")

	if len(got) != 0 {
		t.Errorf("candidates = %v, want none", got)
	}
	if directive != noFileCompDirective {
		t.Errorf("directive = %q, want %q", directive, noFileCompDirective)
	}
}

func TestCompletion_PortArgumentsDoNotSuggestFiles(t *testing.T) {
	got, directive := complete(t, "")

	if len(got) != 0 {
		t.Errorf("candidates = %v, want none", got)
	}
	if directive != noFileCompDirective {
		t.Errorf("directive = %q, want %q", directive, noFileCompDirective)
	}
}

func TestCompletion_ScriptIsGeneratedForEachShell(t *testing.T) {
	for _, shell := range []string{"bash", "zsh", "fish"} {
		t.Run(shell, func(t *testing.T) {
			var out bytes.Buffer
			cmd := NewRootCmd()
			cmd.SetOut(&out)
			cmd.SetArgs([]string{"completion", shell})

			if err := cmd.Execute(); err != nil {
				t.Fatalf("Execute() returned error: %v", err)
			}
			if !strings.Contains(out.String(), "portwhy") {
				t.Errorf("%s script does not mention portwhy: %.80q", shell, out.String())
			}
		})
	}
}
