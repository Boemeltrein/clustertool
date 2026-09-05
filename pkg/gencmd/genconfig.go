package gencmd

import (
	"os"
	"path"

	"github.com/rs/zerolog/log"

	"github.com/trueforge-org/clustertool/pkg/fluxhandler"
	"github.com/trueforge-org/clustertool/pkg/helper"
	"github.com/trueforge-org/clustertool/pkg/initfiles"
	"github.com/trueforge-org/clustertool/pkg/sops"
	"github.com/trueforge-org/clustertool/pkg/talosconfig"
)

func GenConfig(args []string) error {
	if initfiles.CheckRunAgainFileExists() {
		log.Fatal().Msg("You need to re-run Init. Exiting...")
		os.Exit(1)
	}
	if err := sops.DecryptFiles(); err != nil {
		log.Info().Msgf("Error decrypting files: %v\n", err)
	}
	initfiles.GenTalEnvConfigMap()
	initfiles.CheckEnvVariables()
	if err := talosconfig.Generate(); err != nil {
		return err
	}
	initfiles.UpdateGitRepo()

	if err := fluxhandler.ProcessDirectory(path.Join(helper.ClusterPath, "kubernetes")); err != nil {
		log.Info().Msgf("Error: %v", err)
	}
	if err := fluxhandler.ProcessDirectory(path.Join(helper.ClusterPath, "kubernetes")); err != nil {
		log.Info().Msgf("Error: %v", err)
	} else {
		log.Info().Msgf("Kustomizations processed successfully.")
	}
	helper.CreateEncrPreCommitHook()
	log.Info().Msg("GenConfig: Completed Successfully!")
	return nil
}
