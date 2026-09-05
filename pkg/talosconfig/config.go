package talosconfig

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/trueforge-org/clustertool/embed"
	"github.com/trueforge-org/clustertool/pkg/helper"
	fthelper "github.com/trueforge-org/forgetool/v4/pkg/helper"
)

const (
	SecretsFilename      = "secrets.sops.yaml"
	ControlPlaneFilename = "controlplane.yaml"
	TalosconfigFilename  = "talosconfig"
)

func SecretsPath() string {
	return filepath.Join(helper.TalosPath, SecretsFilename)
}

func ControlPlanePath() string {
	return filepath.Join(helper.TalosGenerated, ControlPlaneFilename)
}

func TalosconfigPath() string {
	return filepath.Join(helper.TalosGenerated, TalosconfigFilename)
}

// EnsureSecrets creates the Talos cluster identity once. Existing secrets are
// never replaced by init or genconfig.
func EnsureSecrets() error {
	secretPath := SecretsPath()
	if _, err := os.Stat(secretPath); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("check Talos secrets: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(secretPath), 0o700); err != nil {
		return fmt.Errorf("create Talos directory: %w", err)
	}

	if err := runTalosctl("gen", "secrets", "--output-file", secretPath); err != nil {
		return fmt.Errorf("generate Talos secrets: %w", err)
	}

	if err := os.Chmod(secretPath, 0o600); err != nil {
		return fmt.Errorf("protect Talos secrets: %w", err)
	}

	return nil
}

// Generate builds a native Talos control-plane configuration using the stable
// secrets bundle and the documents in all/ and control-plane/.
func Generate() error {
	if _, err := os.Stat(SecretsPath()); err != nil {
		return fmt.Errorf("Talos secrets are missing; run clustertool init first: %w", err)
	}

	endpoint := "https://" + helper.TalEnv["VIP_IP"] + ":6443"
	if helper.TalEnv["VIP_IP"] == "" {
		endpoint = "https://" + helper.TalEnv["MASTER1IP_IP"] + ":6443"
	}

	workDir, err := os.MkdirTemp("", "clustertool-talos-")
	if err != nil {
		return fmt.Errorf("create temporary Talos directory: %w", err)
	}
	defer os.RemoveAll(workDir)

	if err := runTalosctl(
		"gen", "config", helper.ClusterName, endpoint,
		"--with-secrets", SecretsPath(),
		"--additional-sans", "127.0.0.1,"+helper.TalEnv["VIP_IP"],
		"--output-dir", workDir,
		"--output-types", "controlplane,talosconfig",
		"--force",
	); err != nil {
		return fmt.Errorf("generate Talos base configuration: %w", err)
	}

	patches, err := renderPatches(workDir)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(helper.TalosGenerated, 0o700); err != nil {
		return fmt.Errorf("create generated Talos directory: %w", err)
	}

	args := []string{"machineconfig", "patch", filepath.Join(workDir, "controlplane.yaml")}
	for _, patch := range patches {
		args = append(args, "--patch", "@"+patch)
	}
	args = append(args, "--output", ControlPlanePath())
	if err := runTalosctl(args...); err != nil {
		return fmt.Errorf("apply Talos configuration documents: %w", err)
	}

	if err := os.Chmod(ControlPlanePath(), 0o600); err != nil {
		return fmt.Errorf("protect generated control-plane configuration: %w", err)
	}

	data, err := os.ReadFile(filepath.Join(workDir, "talosconfig"))
	if err != nil {
		return fmt.Errorf("read generated talosconfig: %w", err)
	}
	if err := os.WriteFile(TalosconfigPath(), data, 0o600); err != nil {
		return fmt.Errorf("write generated talosconfig: %w", err)
	}

	if err := runTalosctl("validate", "--config", ControlPlanePath(), "--mode", "metal"); err != nil {
		return fmt.Errorf("validate generated Talos configuration: %w", err)
	}

	return nil
}

func renderPatches(workDir string) ([]string, error) {
	var sourceFiles []string
	for _, dir := range []string{
		filepath.Join(helper.TalosPath, "all"),
		filepath.Join(helper.TalosPath, "control-plane"),
	} {
		entries, err := os.ReadDir(dir)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("read Talos document directory %s: %w", dir, err)
		}
		for _, entry := range entries {
			if !entry.IsDir() && (strings.HasSuffix(entry.Name(), ".yaml") || strings.HasSuffix(entry.Name(), ".yml")) {
				sourceFiles = append(sourceFiles, filepath.Join(dir, entry.Name()))
			}
		}
	}

	sort.Strings(sourceFiles)
	rendered := make([]string, 0, len(sourceFiles))
	for index, source := range sourceFiles {
		content, err := fthelper.EnvSubst(source, helper.TalEnv)
		if err != nil {
			return nil, fmt.Errorf("render Talos document %s: %w", source, err)
		}
		if strings.Contains(content, "${") {
			return nil, fmt.Errorf("Talos document %s contains an unresolved variable", source)
		}
		target := filepath.Join(workDir, fmt.Sprintf("patch-%03d.yaml", index))
		if err := os.WriteFile(target, []byte(content), 0o600); err != nil {
			return nil, fmt.Errorf("write rendered Talos document: %w", err)
		}
		rendered = append(rendered, target)
	}

	return rendered, nil
}

func runTalosctl(args ...string) error {
	command := exec.Command(embed.GetTalosExec(), args...)
	var output bytes.Buffer
	command.Stdout = &output
	command.Stderr = &output
	if err := command.Run(); err != nil {
		return fmt.Errorf("talosctl %s failed: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(output.String()))
	}
	return nil
}
