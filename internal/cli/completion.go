package cli

import (
	"slices"
	"strings"

	"github.com/spf13/cobra"
)

// completeSources suggests the --source values. The flag is comma-separated, so
// the part after the last comma is completed and sources already chosen before
// it are not offered again. The list is static: completing it dynamically would
// run socket detection on every Tab press.
func completeSources(_ *cobra.Command, _ []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	chosenPart, current := "", toComplete
	if i := strings.LastIndex(toComplete, ","); i >= 0 {
		chosenPart, current = toComplete[:i+1], toComplete[i+1:]
	}
	chosen := strings.Split(strings.ToLower(chosenPart), ",")

	var suggestions []string
	for _, s := range knownSources {
		name := string(s)
		if slices.Contains(chosen, name) || !strings.HasPrefix(name, strings.ToLower(current)) {
			continue
		}
		suggestions = append(suggestions, chosenPart+name)
	}
	return suggestions, cobra.ShellCompDirectiveNoFileComp
}

// completeNoFiles keeps the shell from suggesting file names for the port
// arguments; portwhy takes port numbers and ranges only.
func completeNoFiles(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
	return nil, cobra.ShellCompDirectiveNoFileComp
}
