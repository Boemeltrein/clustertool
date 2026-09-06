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

var upgradeLongHelp = strings.TrimSpace(`
The "upgrade" command updates the single Talos node using the installer image from the validated configuration, including its schematic ID.

After upgrading Talos, it upgrades Kubernetes to the configured kubelet version.

`)

var upgrade = &cobra.Command{
	Use:     "upgrade",
	Short:   "Upgrade Talos Nodes and Kubernetes",
	Example: "clustertool talos upgrade <NodeIP>",
	Long:    upgradeLongHelp,
	RunE: func(cmd *cobra.Command, args []string) error {
		var extraArgs []string
		node := ""

		if len(args) > 1 {
			extraArgs = args[1:]
		}
		if len(args) >= 1 {
			node = args[0]
			if args[0] == "all" {
				node = ""
			}
		}

		if err := sops.DecryptFiles(); err != nil {
			return err
		}
		initfiles.LoadTalEnv(false)

		log.Info().Msg("Running Cluster Upgrade")

		if err := gencmd.GenConfig(nil); err != nil {
			return err
		}
		taloscmds, err := gencmd.GenUpgrade(node, extraArgs)
		if err != nil {
			return err
		}
		if err := gencmd.ExecCmds(taloscmds, true); err != nil {
			return err
		}

		log.Info().Msg("Running Kubernetes Upgrade")
		kubeUpgradeCmd, err := gencmd.GenKubeUpgrade(helper.TalEnv["MASTER1IP_IP"])
		if err != nil {
			return err
		}
		if err := gencmd.ExecCmd(kubeUpgradeCmd); err != nil {
			return err
		}

		log.Info().Msg("(re)Loading KubeConfig)")
		kubeconfigcmds := gencmd.GenPlain("kubeconfig", helper.TalEnv["VIP_IP"], []string{"-f"})
		if err := gencmd.ExecCmd(kubeconfigcmds[0]); err != nil {
			return err
		}

		return nil
	},
}

func init() {
	talosCmd.AddCommand(upgrade)
}
