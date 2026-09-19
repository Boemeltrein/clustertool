package gencmd

import (
	"fmt"

	"github.com/rs/zerolog/log"

	"github.com/trueforge-org/clustertool/pkg/helper"
	"github.com/trueforge-org/clustertool/pkg/initfiles"
	"github.com/trueforge-org/clustertool/pkg/sops"
	"github.com/trueforge-org/clustertool/pkg/talosconfig"
	fthelper "github.com/trueforge-org/forgetool/v4/pkg/helper"
)

func GenConfig(args []string) error {
	if initfiles.CheckRunAgainFileExists() {
		return fmt.Errorf("run init again after completing secrets/cluster-settings.sops.yaml")
	}
	if err := sops.DecryptFiles(); err != nil {
		return err
	}
	if err := confirmTalosVersion(fthelper.GetYesOrNo); err != nil {
		return err
	}
	if err := initfiles.CheckEnvVariables(); err != nil {
		return err
	}
	if err := talosconfig.Generate(); err != nil {
		return err
	}
	helper.CreateEncrPreCommitHook()
	log.Info().Msg("GenConfig: Completed Successfully!")
	return nil
}
