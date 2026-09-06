package cmd

import (
	"fmt"
	"strings"

	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"github.com/trueforge-org/clustertool/pkg/gencmd"
	"github.com/trueforge-org/clustertool/pkg/helper"
	"github.com/trueforge-org/clustertool/pkg/initfiles"
	"github.com/trueforge-org/clustertool/pkg/nodestatus"
	"github.com/trueforge-org/clustertool/pkg/sops"
	"github.com/trueforge-org/clustertool/pkg/talosconfig"
	fthelper "github.com/trueforge-org/forgetool/v4/pkg/helper"
)

var applyLongHelp = strings.TrimSpace(`
The "apply" command validates and applies your Talos configuration to the configured single control-plane node, existing or new.

This is the recommended command for both initial cluster bootstrap and day-2 Talos config maintenance.

## Bootstrapping
If the cluster has not been bootstrapped yet, Apply will automatically detect this and ask if you want to bootstrap the cluster

Bootstrapping applies the generated native Talos configuration to the single control-plane node and then bootstraps the cluster.

After this is done, we apply a number of helm-charts and manifests by default such as:

- Metallb
- Metallb-Config
- Cilium (CNI)
- Certificate-Approver
- Headlamp

### Bootstrapping FluxCD

During Bootstrapping, if a "GITHUB_REPOSITORY" is set in "clusterenv.yaml", you will be asked if you also want to bootstrap FluxCD, checkout the getting-started guide for more info

## About Bootstrapping

While we load a lot of helm-charts during bootstrap, we will *never* manage them for you.
You're responsible for maintaining and configuring your cluster after bootstrapping.

Apply and *all other* commands, are just for maintaining Talos itself.
Not any contained helm-charts

`)

var apply = &cobra.Command{
	Use:     "apply",
	Short:   "apply",
	Aliases: []string{"apply-config"},
	Example: "clustertool talos apply <NodeIP>",
	Long:    applyLongHelp,
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
		if err := talosconfig.ValidateNode(node); err != nil {
			return err
		}
		if err := gencmd.ValidateExtraArgs(extraArgs); err != nil {
			return err
		}
		if err := gencmd.GenConfig(nil); err != nil {
			return err
		}
		bootstrapNode := helper.TalEnv["MASTER1IP_IP"]

		log.Info().Msgf("Checking if first node   is ready to recieve anything... %s", bootstrapNode)
		status, err := nodestatus.WaitForHealth(bootstrapNode, []string{"running", "maintenance"})
		if err != nil {
			return err
		}
		if status == "maintenance" {
			needed, err := nodestatus.CheckNeedBootstrap(bootstrapNode)
			if err != nil {
				return err
			}
			if needed {
				if !fthelper.GetYesOrNo("Bootstrap this new single-node cluster? [y/n]: ", false) {
					return fmt.Errorf("bootstrap cancelled")
				}
				return gencmd.RunBootstrap(extraArgs)
			}
		}
		return RunApply(true, node, extraArgs)
	},
}

func RunApply(kubeconfig bool, node string, extraArgs []string) error {
	taloscmds := gencmd.GenApply(node, extraArgs)
	if err := gencmd.ExecCmds(taloscmds, true); err != nil {
		return err
	}

	if kubeconfig {
		kubeconfigcmds := gencmd.GenPlain("kubeconfig", helper.TalEnv["VIP_IP"], []string{"-f"})
		return gencmd.ExecCmd(kubeconfigcmds[0])
	}

	return nil
}

func init() {
	talosCmd.AddCommand(apply)
}
