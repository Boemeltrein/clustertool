package gencmd

import (
	"fmt"
	"strings"

	"github.com/trueforge-org/clustertool/embed"
	"github.com/trueforge-org/clustertool/pkg/helper"
	"github.com/trueforge-org/clustertool/pkg/talosconfig"
)

func GenUpgrade(node string, extraFlags []string) ([]string, error) {
	if err := talosconfig.ValidateNode(node); err != nil {
		return nil, err
	}
	image, err := talosconfig.GeneratedValue("UnattendedInstallConfig", "installer", "image")
	if err != nil {
		return nil, err
	}
	if err := ValidateExtraArgs(extraFlags); err != nil {
		return nil, err
	}
	if node == "" {
		node = helper.TalEnv["MASTER1IP_IP"]
	}
	command := embed.GetTalosExec() + " upgrade --talosconfig " + talosconfig.TalosconfigPath() +
		" -n " + node + " --preserve --wait --image " + image
	for _, flag := range extraFlags {
		command += " " + flag
	}
	return []string{command}, nil
}

func GenKubeUpgrade(node string) (string, error) {
	image, err := talosconfig.GeneratedValue("", "machine", "kubelet", "image")
	if err != nil {
		image, err = talosconfig.GeneratedValue("KubeletConfig", "image")
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
