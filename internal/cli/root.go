// Package cli wires the portwhy command line interface.
package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"github.com/spf13/cobra"
)

// NewRootCmd builds the root portwhy command wired to the real system.
func NewRootCmd() *cobra.Command {
	cmd, _ := newRootCmd(DefaultDeps())
	return cmd
}

// newRootCmd builds the root command. The returned pointer holds the exit code
// once the command has run.
func newRootCmd(d Deps) (*cobra.Command, *int) {
	var flags Flags
	exit := new(int)
	cmd := &cobra.Command{
		Use:   "portwhy [port|range]...",
		Short: "Explain why a port is taken and offer to free it",
		Long: "Explain why a port is taken and offer to free it.\n\n" +
			"Ports are single numbers (3000) or ranges (3000-3010). Without arguments or\n" +
			"with ranges, the busy ports are listed grouped by service; a single port\n" +
			"is explained in detail. Exit codes: 0 busy, 1 all free, 2 error, 3 kill failed.",
		Version:       Version,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			*exit = Run(cmd.Context(), d, args, flags)
			return nil
		},
	}
	cmd.SetVersionTemplate("portwhy {{.Version}}\n")
	f := cmd.Flags()
	f.BoolVar(&flags.JSON, "json", false, "print machine-readable JSON")
	f.BoolVar(&flags.NoColor, "no-color", false, "disable coloured output")
	f.BoolVarP(&flags.Kill, "kill", "k", false, "stop the cause of a busy port after confirmation")
	f.BoolVarP(&flags.Yes, "yes", "y", false, "skip the confirmation prompt (requires --kill)")
	f.BoolVar(&flags.Force, "force", false, "send SIGKILL immediately (requires --kill)")
	f.BoolVar(&flags.Wide, "wide", false, "do not shorten long lines to the terminal width")
	f.BoolVar(&flags.DryRun, "dry-run", false, "show what --kill would do without doing it")
	return cmd, exit
}

// Execute runs portwhy and returns the process exit code.
func Execute() int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	cmd, exit := newRootCmd(DefaultDeps())
	if err := cmd.ExecuteContext(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "portwhy:", err)
		return exitError
	}
	return *exit
}
