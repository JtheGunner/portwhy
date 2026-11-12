package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/JtheGunner/portwhy/internal/portspec"
	"github.com/JtheGunner/portwhy/internal/render"
	"github.com/JtheGunner/portwhy/internal/report"
)

// Exit codes. When several apply, the highest precedence wins: 2 > 3 > 0 > 1.
const (
	exitBusy        = 0
	exitAllFree     = 1
	exitError       = 2
	exitKillFailure = 3
)

// Flags are the command line options. The kill flags are parsed here and
// acted on by the kill core.
type Flags struct {
	JSON    bool
	NoColor bool
	Kill    bool
	Yes     bool
	Force   bool
	DryRun  bool
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
	reports = visibleReports(reports, args)

	if err := write(d, reports, flags); err != nil {
		return fail(d, err)
	}
	if anyBusy(reports) {
		return exitBusy
	}
	return exitAllFree
}

func write(d Deps, reports []report.PortReport, flags Flags) error {
	if flags.JSON {
		return render.JSON(d.Stdout, reports)
	}
	if len(reports) == 0 {
		_, err := fmt.Fprintln(d.Stdout, "No busy ports.")
		return err
	}
	colors := render.Colors{Enabled: d.IsTTY && !flags.NoColor && os.Getenv("NO_COLOR") == ""}
	return render.Text(d.Stdout, reports, colors)
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
