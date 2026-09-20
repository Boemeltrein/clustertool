package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/trueforge-org/clustertool/pkg/initfiles"
	"github.com/trueforge-org/clustertool/pkg/sops"
	"github.com/trueforge-org/clustertool/pkg/talosconfig"
)

var initLongHelp = strings.TrimSpace(`
Clustertool requires a specific directory layout to ensure smooth operators and standardised environments.

To ensure smooth deployment, the init function can pre-generate all required files in the right places.
Afterwards, edit secrets/cluster-settings.sops.yaml and the native Talos documents to reflect your personal settings.

When done, please run clustertool genconfig to generate all configurations based on your personal settings.
`)

var initFiles = &cobra.Command{
	Use:     "init",
	Short:   "generate Basic cluster file-and-folder structure in current folder",
	Long:    initLongHelp,
	Example: "clustertool init",
	RunE: func(cmd *cobra.Command, args []string) error {
		if !initfiles.CheckRunAgainFileExists() {
			if _, err := os.Stat(talosconfig.SecretsPath()); err == nil {
				fmt.Fprintln(cmd.OutOrStdout(), "WARNING: This cluster appears to have already been initialized.\nRunning init again may add or modify files.\n\nWe recommend initializing in a new folder and comparing the files\nwith your existing configuration.")
				fmt.Fprint(cmd.OutOrStdout(), "\nContinue with init? [y/N]: ")
				answer, err := bufio.NewReader(cmd.InOrStdin()).ReadString('\n')
				if err != nil || strings.ToLower(strings.TrimSpace(answer)) != "y" {
					return fmt.Errorf("init cancelled")
				}
			} else if !os.IsNotExist(err) {
				return fmt.Errorf("check Talos secrets: %w", err)
			}
		}
		if err := sops.DecryptFiles(); err != nil {
			// A missing SOPS config is expected during the first initialization.
			if _, statErr := os.Stat(".sops.yaml"); !os.IsNotExist(statErr) {
				return err
			}
		}

		return initfiles.InitFiles()
	},
}

func init() {
	RootCmd.AddCommand(initFiles)
}
