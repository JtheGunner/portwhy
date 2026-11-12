// Package cli wires the portwhy command line interface.
package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

const (
	exitOK    = 0
	exitError = 2
)

// NewRootCmd builds the root portwhy command.
func NewRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "portwhy",
		Short:         "Explain why a port is taken and offer to free it",
		Version:       Version,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	cmd.SetVersionTemplate("portwhy {{.Version}}\n")
	return cmd
}

// Execute runs portwhy and returns the process exit code.
func Execute() int {
	if err := NewRootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "portwhy:", err)
		return exitError
	}
	return exitOK
}
