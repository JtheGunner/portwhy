package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/JtheGunner/portwhy/internal/detect"
	"github.com/JtheGunner/portwhy/internal/kill"
	"github.com/JtheGunner/portwhy/internal/proc"
	"github.com/JtheGunner/portwhy/internal/socket"
)

// portDetector explains each socket by its port: the source and optional
// terminator are looked up per port, so one test can mix several sources.
type portDetector struct {
	sources     map[uint16]detect.Source
	terminators map[uint16]kill.Terminator
}

func (p portDetector) Detect(_ context.Context, s socket.Socket, _ *proc.Process) (*detect.Explanation, bool) {
	return &detect.Explanation{
		Source:     p.sources[s.Port],
		Title:      string(p.sources[s.Port]) + " owner",
		Terminator: p.terminators[s.Port],
	}, true
}

// mixedDeps listens on 3000 (process), 5432 (docker) and 8080 (docker).
func mixedDeps() (Deps, *bytes.Buffer, *bytes.Buffer) {
	d, stdout, stderr := testDeps(fakeSockets{sockets: []socket.Socket{
		listener(3000, 42), listener(5432, 43), listener(8080, 44),
	}})
	d.Detector = portDetector{sources: map[uint16]detect.Source{
		3000: detect.SourceProcess,
		5432: detect.SourceDocker,
		8080: detect.SourceDocker,
	}}
	return d, stdout, stderr
}

func TestRun_SourceFilterKeepsOnlyMatchingPorts(t *testing.T) {
	d, stdout, _ := mixedDeps()

	got := Run(context.Background(), d, nil, Flags{Source: []string{"docker"}})

	if got != 0 {
		t.Errorf("exit = %d, want 0", got)
	}
	out := stdout.String()
	if !strings.Contains(out, "5432") || !strings.Contains(out, "8080") {
		t.Errorf("stdout = %q, want both docker ports", out)
	}
	if strings.Contains(out, "3000") {
		t.Errorf("stdout = %q, want the process port filtered out", out)
	}
}

func TestRun_SourceFilterIsCaseInsensitive(t *testing.T) {
	d, stdout, _ := mixedDeps()

	got := Run(context.Background(), d, nil, Flags{Source: []string{"Docker"}})

	if got != 0 || !strings.Contains(stdout.String(), "5432") {
		t.Errorf("exit = %d, stdout = %q, want docker ports", got, stdout.String())
	}
}

func TestRun_SourceFilterAppliesToJSON(t *testing.T) {
	d, stdout, _ := mixedDeps()

	got := Run(context.Background(), d, nil, Flags{JSON: true, Source: []string{"process"}})

	if got != 0 {
		t.Errorf("exit = %d, want 0", got)
	}
	out := stdout.String()
	if !strings.Contains(out, `"port": 3000`) || strings.Contains(out, `"port": 5432`) {
		t.Errorf("stdout = %q, want only port 3000", out)
	}
	if !strings.Contains(out, `"version": 1`) {
		t.Errorf("stdout = %q, want schema version 1 unchanged", out)
	}
}

func TestRun_KnownSourceWithoutMatchExit1(t *testing.T) {
	d, stdout, _ := mixedDeps()

	got := Run(context.Background(), d, nil, Flags{Source: []string{"launchd"}})

	if got != 1 {
		t.Errorf("exit = %d, want 1", got)
	}
	if want := "No busy ports from launchd.\n"; stdout.String() != want {
		t.Errorf("stdout = %q, want %q", stdout.String(), want)
	}
}

func TestRun_MultipleMissingSourcesAreNamed(t *testing.T) {
	d, stdout, _ := mixedDeps()

	got := Run(context.Background(), d, nil, Flags{Source: []string{"launchd", "systemd"}})

	if got != 1 {
		t.Errorf("exit = %d, want 1", got)
	}
	if want := "No busy ports from launchd, systemd.\n"; stdout.String() != want {
		t.Errorf("stdout = %q, want %q", stdout.String(), want)
	}
}

func TestRun_UnknownSourceExit2ListsAvailable(t *testing.T) {
	d, stdout, stderr := mixedDeps()

	got := Run(context.Background(), d, nil, Flags{Source: []string{"foo"}})

	if got != 2 {
		t.Errorf("exit = %d, want 2", got)
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want empty", stdout.String())
	}
	for _, want := range []string{`unknown source "foo"`, "available now: docker, process"} {
		if !strings.Contains(stderr.String(), want) {
			t.Errorf("stderr = %q, want it to contain %q", stderr.String(), want)
		}
	}
}

func TestRun_UnknownSourceWithNothingListeningSaysNone(t *testing.T) {
	d, _, stderr := testDeps(fakeSockets{})

	got := Run(context.Background(), d, nil, Flags{Source: []string{"foo"}})

	if got != 2 {
		t.Errorf("exit = %d, want 2", got)
	}
	if !strings.Contains(stderr.String(), "available now: none") {
		t.Errorf("stderr = %q, want %q", stderr.String(), "available now: none")
	}
}

func TestRun_ExplicitPortNotMatchingFilterIsNotReportedFree(t *testing.T) {
	// Port 3000 is busy, just not by docker. Showing ":3000  free" would be a lie.
	d, stdout, _ := mixedDeps()

	got := Run(context.Background(), d, []string{"3000"}, Flags{Source: []string{"docker"}})

	if got != 1 {
		t.Errorf("exit = %d, want 1", got)
	}
	if strings.Contains(stdout.String(), "free") {
		t.Errorf("stdout = %q, want the port not reported as free", stdout.String())
	}
}

func TestRun_ListModeEndsWithSourcesFooter(t *testing.T) {
	d, stdout, _ := mixedDeps()

	Run(context.Background(), d, nil, Flags{})

	want := "Sources: docker (2) · process (1) — filter with --source <name>\n"
	if !strings.HasSuffix(stdout.String(), want) {
		t.Errorf("stdout = %q, want it to end with %q", stdout.String(), want)
	}
}

func TestRun_FooterListsAllSourcesEvenWhenFiltered(t *testing.T) {
	d, stdout, _ := mixedDeps()

	Run(context.Background(), d, nil, Flags{Source: []string{"docker"}})

	want := "Sources: docker (2) · process (1) — filter with --source <name>\n"
	if !strings.HasSuffix(stdout.String(), want) {
		t.Errorf("stdout = %q, want the unfiltered footer %q", stdout.String(), want)
	}
}

func TestRun_NoFooterWithPortArguments(t *testing.T) {
	d, stdout, _ := mixedDeps()

	Run(context.Background(), d, []string{"3000-9000"}, Flags{})

	if strings.Contains(stdout.String(), "Sources:") {
		t.Errorf("stdout = %q, want no footer outside list mode", stdout.String())
	}
}

func TestRun_NoFooterInJSON(t *testing.T) {
	d, stdout, _ := mixedDeps()

	Run(context.Background(), d, nil, Flags{JSON: true})

	if strings.Contains(stdout.String(), "Sources:") {
		t.Errorf("stdout = %q, want JSON without the text footer", stdout.String())
	}
}

func TestRun_NoFooterWhenNothingIsBusy(t *testing.T) {
	d, stdout, _ := testDeps(fakeSockets{})

	Run(context.Background(), d, nil, Flags{})

	if want := "No busy ports.\n"; stdout.String() != want {
		t.Errorf("stdout = %q, want %q", stdout.String(), want)
	}
}

func TestRun_KillOnlyStopsFilteredPorts(t *testing.T) {
	dockerTerm := &fakeTerminator{describe: "docker stop db"}
	processTerm := &fakeTerminator{describe: "kill -TERM 42"}
	d, _, _ := mixedDeps()
	d.Detector = portDetector{
		sources: map[uint16]detect.Source{
			3000: detect.SourceProcess, 5432: detect.SourceDocker, 8080: detect.SourceDocker,
		},
		terminators: map[uint16]kill.Terminator{3000: processTerm, 5432: dockerTerm, 8080: dockerTerm},
	}

	Run(context.Background(), d, nil, Flags{Kill: true, Yes: true, Source: []string{"docker"}})

	if processTerm.calls != 0 {
		t.Errorf("process terminator calls = %d, want 0: --source must limit --kill", processTerm.calls)
	}
	if dockerTerm.calls == 0 {
		t.Errorf("docker terminator calls = 0, want the docker ports stopped")
	}
}

func TestRootCmd_SourceFlagAcceptsCommaSeparatedAndRepeated(t *testing.T) {
	for _, args := range [][]string{
		{"--source", "docker,process"},
		{"--source", "docker", "--source", "process"},
	} {
		d, stdout, _ := mixedDeps()
		cmd, exit := newRootCmd(d)
		cmd.SetArgs(args)

		if err := cmd.ExecuteContext(context.Background()); err != nil {
			t.Fatal(err)
		}

		out := stdout.String()
		if *exit != 0 || !strings.Contains(out, "3000") || !strings.Contains(out, "5432") {
			t.Errorf("args %v: exit = %d, stdout = %q, want docker and process ports", args, *exit, out)
		}
	}
}
