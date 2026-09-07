package gencmd

import (
	"strings"

	"github.com/trueforge-org/clustertool/embed"
	"github.com/trueforge-org/clustertool/pkg/talosconfig"
)

func GenApply(node string, extraArgs []string) []string {

	commands := []string{}

	talosPath := embed.GetTalosExec()
	inv, err := talosconfig.LoadInventory()
	if err != nil {
		return nil
	}
	nodes, err := inv.Select(node)
	if err != nil {
		return nil
	}
	for _, n := range nodes {
		cmd := talosPath + " apply-config -f " + talosconfig.NodeConfigPath(n) + " --talosconfig " + talosconfig.TalosconfigPath() + " -n " + n.Address
		if len(extraArgs) > 0 {
			cmd += " " + strings.Join(extraArgs, " ")
		}
		commands = append(commands, cmd)
	}
	return commands
}

