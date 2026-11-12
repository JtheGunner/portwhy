package cli

import (
	"context"
	"errors"
	"fmt"
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
	Wide    bool
	// Expanded switches the overview of all ports to the airy layout.
	Expanded bool
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
	if flags.Force && !flags.Kill {
		return fail(d, errors.New("--force requires --kill"))
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

	if err := write(d, reports, args, flags, sources, available); err != nil {
		return fail(d, err)
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
	ropts := textOptions(d, nil, flags, nil)
	ropts.Colors = render.NewColors(d.StderrIsTTY, flags.NoColor)
	counts := map[render.KillOutcome]int{}
	line := func(o render.KillOutcome, port uint16, detail string) {
		counts[o]++
		_, _ = fmt.Fprintln(d.Stderr, render.KillLine(o, port, detail, ropts))
	}
	defer func() {
		if !flags.DryRun && len(counts) > 0 {
			_, _ = fmt.Fprintln(d.Stderr, render.KillSummary(counts, ropts))
		}
	}()

	for _, r := range reports {
		for _, f := range r.Findings {
			if f.Explanation.Terminator == nil {
				line(render.KillFailed, r.Port, "cannot stop it: the owning process is not visible; try again with sudo")
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
					line(render.KillStopped, r.Port, "")
				}
			case errors.Is(err, kill.ErrDeclined):
				line(render.KillDeclined, r.Port, "")
			default:
				line(render.KillFailed, r.Port, err.Error())
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

func write(d Deps, reports []report.PortReport, args []string, flags Flags, sources []detect.Source, available []report.SourceCount) error {
	if flags.JSON {
		return render.JSON(d.Stdout, reports)
	}
	if len(reports) == 0 {
		_, err := fmt.Fprintln(d.Stdout, noBusyPortsMessage(sources))
		return err
	}
	return render.Text(d.Stdout, reports, textOptions(d, args, flags, available))
}

// textOptions are the text layout settings for this run. available are the
// sources found before the --source filter; only the overview of all ports
// names them, as the hint how to filter.
func textOptions(d Deps, args []string, flags Flags, available []report.SourceCount) render.Options {
	opts := render.Options{
		Colors:   render.NewColors(d.IsTTY, flags.NoColor),
		Width:    d.Width,
		Wide:     flags.Wide,
		Overview: overviewMode(args, flags.Kill),
		ASCII:    d.ASCII,
		Expanded: flags.Expanded,
	}
	if len(args) == 0 {
		opts.Sources = available
	}
	return opts
}

// overviewMode reports whether the text output is the grouped overview: for
// no arguments or only ranges. A named port, and every --kill run, gets the
// detailed blocks so the user sees exactly what would be stopped.
func overviewMode(args []string, kill bool) bool {
	if kill {
		return false
	}
	for _, arg := range args {
		if !strings.Contains(arg, "-") {
			return false
		}
	}
	return true
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
