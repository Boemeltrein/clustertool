package cmd

import (
	"context"
	"strings"

	"github.com/spf13/cobra"
	"github.com/trueforge-org/clustertool/pkg/fluxhandler"
	"github.com/trueforge-org/clustertool/pkg/initfiles"
	"github.com/trueforge-org/clustertool/pkg/sops"
)

var fluxBootstrapLongHelp = strings.TrimSpace(`
Install Flux Operator, wait for it to be ready, then install the FluxInstance.
The deploy key and SOPS age key are applied before the instance is installed.
Run genconfig and push your configuration to Git before bootstrapping.
`)

var fluxbootstrap = &cobra.Command{
	Use:     "bootstrap",
	Short:   "Manually bootstrap Flux on existing cluster",
	Example: "clustertool flux bootstrap",
	Long:    fluxBootstrapLongHelp,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()

		if err := sops.DecryptFiles(); err != nil {
			return err
		}
		if err := initfiles.LoadTalEnv(false); err != nil {
			return err
		}
		return fluxhandler.FluxBootstrap(ctx)
	},
}

func init() {
	fluxCmd.AddCommand(fluxbootstrap)
}
