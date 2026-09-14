package cmd

import (
	"strings"

	"github.com/spf13/cobra"
)

var fluxLongHelp = strings.TrimSpace(`
These are all commands that can be used to maintain Flux

`)

var fluxCmd = &cobra.Command{
	Use:           "flux",
	Short:         "Commands for handling Flux",
	Example:       "clustertool flux bootstrap",
	Long:          fluxLongHelp,
	SilenceUsage:  true,
	SilenceErrors: true,
}

func init() {
	RootCmd.AddCommand(fluxCmd)
}
