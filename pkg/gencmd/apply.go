package gencmd

import (
	"strings"

	"github.com/trueforge-org/clustertool/embed"
	"github.com/trueforge-org/clustertool/pkg/helper"
	"github.com/trueforge-org/clustertool/pkg/talosconfig"
)

func GenApply(node string, extraArgs []string) []string {

	commands := []string{}

	talosPath := embed.GetTalosExec()
	if node == "" {
		node = helper.TalEnv["MASTER1IP_IP"]
	}
	cmd := talosPath + " apply-config -f " + talosconfig.ControlPlanePath() + " --talosconfig " + talosconfig.TalosconfigPath() + " -n " + node
	if len(extraArgs) > 0 {
		cmd += " " + strings.Join(extraArgs, " ")
	}
	commands = append(commands, cmd)
	return commands
}
