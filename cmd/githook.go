package cmd

import (
	"github.com/spf13/cobra"
	"github.com/trueforge-org/clustertool/pkg/helper"
)

var githook = &cobra.Command{
	Use:     "githook",
	Short:   "Install the Git pre-commit encryption hook",
	Long:    "Install or replace the Git pre-commit encryption hook for the current environment. Run from your cluster repository root after switching environments or rebuilding a devcontainer.",
	Example: "clustertool githook",
	Args:    cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return helper.CreateEncrPreCommitHook()
	},
}

func init() {
	RootCmd.AddCommand(githook)
}
