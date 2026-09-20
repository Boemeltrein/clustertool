package initfiles

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/trueforge-org/clustertool/pkg/helper"
	"github.com/trueforge-org/clustertool/pkg/talosconfig"
)

// ConfirmInit checks the selected cluster before any repository files are changed.
func ConfirmInit(in io.Reader, out io.Writer) error {
	for _, file := range []string{
		filepath.Join(helper.TalosPath, "talconfig.yaml"),
		filepath.Join(helper.ClusterPath, "clusterenv.yaml"),
	} {
		if _, err := os.Stat(file); err == nil {
			return fmt.Errorf("Legacy Talhelper-configured cluster found.\nInitialize ClusterTool 5 in a new folder.")
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("check legacy configuration: %w", err)
		}
	}
	if _, err := os.Stat("RUNAGAIN"); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("check RUNAGAIN: %w", err)
	}
	if _, err := os.Stat(talosconfig.SecretsPath()); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return fmt.Errorf("check Talos secrets: %w", err)
	}
	fmt.Fprintln(out, "WARNING: This cluster appears to have already been initialized.\nRunning init again may add or modify files.\n\nWe recommend initializing in a new folder and comparing the files\nwith your existing configuration.")
	fmt.Fprint(out, "\nContinue with init? [y/N]: ")
	answer, err := bufio.NewReader(in).ReadString('\n')
	if err != nil || strings.ToLower(strings.TrimSpace(answer)) != "y" {
		return fmt.Errorf("init cancelled")
	}
	return nil
}
