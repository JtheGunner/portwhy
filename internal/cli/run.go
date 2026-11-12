package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/JtheGunner/portwhy/internal/detect"
	"github.com/JtheGunner/portwhy/internal/kill"
	"github.com/JtheGunner/portwhy/internal/portspec"
	"github.com/JtheGunner/portwhy/internal/render"
	"github.com/JtheGunner/portwhy/internal/report"
	"github.com/JtheGunner/portwhy/internal/socket"
)

// Exit codes. When several apply, the highest precedence wins: 2 > 3 > 0 > 1.
const (
	exitBusy        = 0
	exitAllFree     = 1
	exitError       = 2
	exitKillFailure = 3
)

// Flags are the command line options.
type Flags struct {
	JSON    bool
	NoColor bool
	Kill    bool
	Yes     bool
	Force   bool
	DryRun  bool
	// Source limits the listing, and with Kill the stop, to ports whose owner
	// is one of these sources (docker, systemd, launchd, process).
	Source []string
}

// Run explains the given port arguments and returns the process exit code.
func Run(ctx context.Context, d Deps, args []string, flags Flags) int {
	ports, err := portspec.Parse(args)
	if err != nil {
		return fail(d, err)
	}
	if flags.Yes && !flags.Kill {
		return fail(d, errors.New("--yes requires --kill"))
	}

	svc := report.Service{Sockets: d.Sockets, Procs: d.Procs, Detector: d.Detector}
	reports, err := svc.Explain(ctx, ports)
	if err != nil {
		return fail(d, err)
	}
	available := report.CountSources(reports)
	sources, err := parseSources(flags.Source, available)
	if err != nil {
		return fail(d, err)
	}
	reports = report.FilterBySource(reports, sources)
	reports = visibleReports(reports, args)

	if err := write(d, reports, flags, sources); err != nil {
		return fail(d, err)
	}
	if len(args) == 0 && !flags.JSON && len(reports) > 0 {
		if err := render.Sources(d.Stdout, available); err != nil {
			return fail(d, err)
		}
	}
	if flags.Kill && stopAll(ctx, d, reports, flags) {
		return exitKillFailure
	}
	if anyBusy(reports) {
		return exitBusy
	}
	return exitAllFree
}

// stopAll stops the cause of every busy port and reports whether any attempt
// failed. Prompts and messages go to stderr so JSON on stdout stays intact.
// A declined prompt is the user's choice and not a failure.
func stopAll(ctx context.Context, d Deps, reports []report.PortReport, flags Flags) (failed bool) {
	opts := kill.Options{
		Yes:         flags.Yes,
		Force:       flags.Force,
		DryRun:      flags.DryRun,
		Interactive: d.IsTTY && d.StdinIsTTY,
		In:          d.Stdin,
		Out:         d.Stderr,
	}
	check := portFree(d.Sockets)
	done := make(map[string]bool)

	for _, r := range reports {
		for _, f := range r.Findings {
			if f.Explanation.Terminator == nil {
				_, _ = fmt.Fprintf(d.Stderr, "portwhy: cannot stop port %d: the owning process is not visible; try again with sudo\n", r.Port)
				failed = true
				continue
			}
			t := f.Explanation.Terminator
			key := fmt.Sprintf("%d/%s", r.Port, t.Describe())
			if done[key] {
				continue
			}
			done[key] = true

			err := kill.Run(ctx, t, r.Port, opts, check)
			switch {
			case err == nil:
				if !flags.DryRun {
					_, _ = fmt.Fprintf(d.Stderr, "Port %d is free.\n", r.Port)
				}
			case errors.Is(err, kill.ErrDeclined):
				_, _ = fmt.Fprintf(d.Stderr, "Port %d left untouched.\n", r.Port)
			default:
				_, _ = fmt.Fprintf(d.Stderr, "portwhy: port %d: %v\n", r.Port, err)
				failed = true
			}
		}
	}
	return failed
}

// portFree returns a kill.PortChecker that treats any listening socket on the
// port, including one whose owner is not visible, as busy.
func portFree(c socket.Collector) kill.PortChecker {
	return func(ctx context.Context, port uint16) (bool, error) {
		sockets, err := c.Sockets(ctx)
		if err != nil {
			return false, err
		}
		for _, s := range sockets {
			if s.Port == port && s.State == socket.Listen {
				return false, nil
			}
		}
		return true, nil
	}
}

func write(d Deps, reports []report.PortReport, flags Flags, sources []detect.Source) error {
	if flags.JSON {
		return render.JSON(d.Stdout, reports)
	}
	if len(reports) == 0 {
		_, err := fmt.Fprintln(d.Stdout, noBusyPortsMessage(sources))
		return err
	}
	colors := render.Colors{Enabled: d.IsTTY && !flags.NoColor && os.Getenv("NO_COLOR") == ""}
	return render.Text(d.Stdout, reports, colors)
}

func noBusyPortsMessage(sources []detect.Source) string {
	if len(sources) == 0 {
		return "No busy ports."
	}
	names := make([]string, len(sources))
	for i, s := range sources {
		names[i] = string(s)
	}
	return "No busy ports from " + strings.Join(names, ", ") + "."
}

// knownSources are the values --source accepts. A known source that is not
// currently listening is valid and simply matches nothing.
var knownSources = []detect.Source{
	detect.SourceDocker, detect.SourceSystemd, detect.SourceLaunchd, detect.SourceProcess,
}

// parseSources validates the --source values, case-insensitively and without
// duplicates. An unknown value is an error that names the sources found now.
func parseSources(values []string, available []report.SourceCount) ([]detect.Source, error) {
	var out []detect.Source
	for _, v := range values {
		s := detect.Source(strings.ToLower(strings.TrimSpace(v)))
		if !slices.Contains(knownSources, s) {
			return nil, fmt.Errorf("unknown source %q; available now: %s", v, availableNames(available))
		}
		if !slices.Contains(out, s) {
			out = append(out, s)
		}
	}
	return out, nil
}

func availableNames(available []report.SourceCount) string {
	if len(available) == 0 {
		return "none"
	}
	names := make([]string, len(available))
	for i, c := range available {
		names[i] = string(c.Source)
	}
	return strings.Join(names, ", ")
}

// visibleReports hides free ports unless they were requested as a single port.
// Ranges and list mode (no arguments) only show busy ports.
func visibleReports(reports []report.PortReport, args []string) []report.PortReport {
	explicit := explicitPorts(args)
	out := make([]report.PortReport, 0, len(reports))
	for _, r := range reports {
		if len(r.Findings) > 0 || explicit[r.Port] {
			out = append(out, r)
		}
	}
	return out
}

// explicitPorts returns the ports named as single ports (not part of a range).
// The arguments are already validated by portspec.Parse.
func explicitPorts(args []string) map[uint16]bool {
	explicit := make(map[uint16]bool)
	for _, arg := range args {
		if strings.Contains(arg, "-") {
			continue
		}
		ports, err := portspec.Parse([]string{arg})
		if err != nil || len(ports) != 1 {
			continue
		}
		explicit[ports[0]] = true
	}
	return explicit
}

func anyBusy(reports []report.PortReport) bool {
	for _, r := range reports {
		if len(r.Findings) > 0 {
			return true
		}
	}
	return false
}

func fail(d Deps, err error) int {
	_, _ = fmt.Fprintln(d.Stderr, "portwhy:", err)
	return exitError
}
