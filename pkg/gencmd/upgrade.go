package gencmd

import (
	"github.com/trueforge-org/clustertool/embed"
	"github.com/trueforge-org/clustertool/pkg/helper"
	"github.com/trueforge-org/clustertool/pkg/talosconfig"
)

func GenUpgrade(node string, extraFlags []string) []string {
	if node == "" {
		node = helper.TalEnv["MASTER1IP_IP"]
	}
	command := embed.GetTalosExec() + " upgrade --talosconfig " + talosconfig.TalosconfigPath() +
		" -n " + node + " --image ghcr.io/siderolabs/installer:" + helper.TalEnv["TALOS_VERSION"] + " --preserve"
	for _, flag := range extraFlags {
		command += " " + flag
	}
	return []string{command}
}

func GenKubeUpgrade(node string) string {
	talosPath := embed.GetTalosExec()
	strout := talosPath + " upgrade-k8s --talosconfig " + talosconfig.TalosconfigPath() + " -n " + node
	return strout
}
