package gencmd

import (
	"fmt"
	"strings"

	"github.com/trueforge-org/clustertool/embed"
	"github.com/trueforge-org/clustertool/pkg/talosconfig"
)

func GenUpgrade(node string, extraFlags []string) ([]string, error) {
	if err := talosconfig.ValidateNode(node); err != nil {
		return nil, err
	}
	inv, err := talosconfig.LoadInventory()
	if err != nil {
		return nil, err
	}
	nodes, err := inv.Select(node)
	if err != nil {
		return nil, err
	}
	if err := ValidateExtraArgs(extraFlags); err != nil {
		return nil, err
	}
	var commands []string
	for _, n := range nodes {
		image, err := talosconfig.GeneratedNodeValue(talosconfig.NodeConfigPath(n), "UnattendedInstallConfig", "installer", "image")
		if err != nil {
			return nil, fmt.Errorf("node %s: %w", n.Name, err)
		}
		command := embed.GetTalosExec() + " upgrade --talosconfig " + talosconfig.TalosconfigPath() + " -n " + n.Address + " --preserve --wait --image " + image
		for _, flag := range extraFlags {
			command += " " + flag
		}
		commands = append(commands, command)
	}
	return commands, nil
}

func GenKubeUpgrade(node string) (string, error) {
	inv, err := talosconfig.LoadInventory()
	if err != nil {
		return "", err
	}
	path := talosconfig.NodeConfigPath(inv.Bootstrap())
	image, err := talosconfig.GeneratedNodeValue(path, "", "machine", "kubelet", "image")
	if err != nil {
		image, err = talosconfig.GeneratedNodeValue(path, "KubeletConfig", "image")
	}
	if err != nil {
		return "", err
	}
	_, tag, ok := strings.Cut(image, ":")
	if !ok {
		return "", fmt.Errorf("kubelet image must contain a Kubernetes version tag")
	}
	tag, _, _ = strings.Cut(tag, "@")
	talosPath := embed.GetTalosExec()
	strout := talosPath + " upgrade-k8s --talosconfig " + talosconfig.TalosconfigPath() + " -n " + node + " --to " + strings.TrimPrefix(tag, "v")
	return strout, nil
}

