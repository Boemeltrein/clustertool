package gencmd

import (
	"strings"

	"github.com/rs/zerolog/log"
	"github.com/trueforge-org/clustertool/embed"
	"github.com/trueforge-org/clustertool/pkg/talosconfig"
)

func GenPlain(command string, node string, extraArgs []string) []string {

	commands := []string{}

	talosPath := embed.GetTalosExec()
	log.Debug().Msg("Generating plain CMDs...")
	if node == "" {
		log.Debug().Msg("Cmd Nodes is empty, rendering cmds for all nodes...")

		inv, err := talosconfig.LoadInventory()
		if err != nil {
			return nil
		}
		for _, n := range inv.Nodes {
			commands = append(commands, GenPlain(command, n.Address, extraArgs)...)
		}
	} else {
		log.Debug().Msgf("Rendering for single node: %s", node)
		cmd := talosPath + " " + command + " --talosconfig " + talosconfig.TalosconfigPath() + " -n " + node
		if len(extraArgs) == 0 {
			log.Debug().Msg("extraArgs is empty, not adding extra args to cmd")
		} else {
			log.Debug().Msgf("extraArgs not empty, adding extra args to cmd: %s", extraArgs)
			cmd = cmd + " " + strings.Join(extraArgs, " ")
		}

		commands = append(commands, cmd)
	}
	log.Debug().Msgf("%s Commands rendered: %s", command, commands)
	return commands
}

