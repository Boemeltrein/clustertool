package cmd

import (
	"strings"

	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"github.com/trueforge-org/clustertool/pkg/gencmd"
	"github.com/trueforge-org/clustertool/pkg/helper"
	"github.com/trueforge-org/clustertool/pkg/initfiles"
	"github.com/trueforge-org/clustertool/pkg/sops"
)

var advHealthLongHelp = strings.TrimSpace(`

`)

var health = &cobra.Command{
	Use:     "health",
	Short:   "Check Talos Cluster Health",
	Example: "clustertool talos health",
	Long:    advHealthLongHelp,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := sops.DecryptFiles(); err != nil {
			return err
		}
		initfiles.LoadTalEnv(false)
		log.Info().Msg("Running Cluster HealthCheck")
		healthcmd := gencmd.GenPlain("health", helper.TalEnv["VIP_IP"], []string{})
		if err := gencmd.ExecCmd(healthcmd[0]); err != nil {
			return err
		}
		return nil
	},
}

func init() {
	talosCmd.AddCommand(health)
}
