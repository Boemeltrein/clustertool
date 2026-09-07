package talosconfig

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/trueforge-org/clustertool/embed"
	"github.com/trueforge-org/clustertool/pkg/helper"
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

	if err := checkLegacyPKI(); err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(secretPath), 0o700); err != nil {
		return fmt.Errorf("create Talos directory: %w", err)
	}

	dir, err := os.MkdirTemp(helper.TalosPath, ".secrets-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	temporary := filepath.Join(dir, "secrets.yaml")
	if err := runTalosctl("gen", "secrets", "--output-file", temporary); err != nil {
		return err
	}
	data, err := os.ReadFile(temporary)
	if err != nil {
		return err
	}
	// Exclusive creation protects a concurrent init's existing identity too.
	file, err := os.OpenFile(secretPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	if _, err := file.Write(data); err != nil {
		file.Close()
		return err
	}
	return file.Close()
}

// Generate builds a native Talos control-plane configuration using the stable
// secrets bundle and the documents in all/ and control-plane/.
func Generate() error {
	inv, err := LoadInventory()
	if err != nil {
		return err
	}
	if !inv.Legacy {
		return generateInventory(inv)
	}
	if err := ValidateNode(""); err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(helper.TalosPath, "talconfig.yaml")); err == nil {
		return fmt.Errorf("legacy talconfig.yaml exists: migrate its settings first (docs/native-talos.md)")
	} else if !os.IsNotExist(err) {
		return err
	}

	if _, err := os.Stat(SecretsPath()); err != nil {
		return fmt.Errorf("Talos secrets are missing; run clustertool init first: %w", err)
	}

	endpoint := "https://" + helper.TalEnv["VIP_IP"] + ":6443"
	if helper.TalEnv["VIP_IP"] == "" {
		endpoint = "https://" + helper.TalEnv["MASTER1IP_IP"] + ":6443"
	}

	workDir, err := os.MkdirTemp(helper.TalosPath, ".generate-")
	if err != nil {
		return fmt.Errorf("create temporary Talos directory: %w", err)
	}
	defer os.RemoveAll(workDir)

	if err := runTalosctl(
		"gen", "config", helper.ClusterName, endpoint,
		"--with-secrets", SecretsPath(),
		"--output", workDir,
		"--output-types", "controlplane,talosconfig",
		"--with-docs=false", "--with-examples=false",
	); err != nil {
		return fmt.Errorf("generate Talos base configuration: %w", err)
	}

	patches, err := renderPatches(workDir)
	if err != nil {
		return err
	}

	staged := filepath.Join(workDir, "validated")
	if err := os.MkdirAll(staged, 0o700); err != nil {
		return fmt.Errorf("create generated Talos directory: %w", err)
	}

	args := []string{"machineconfig", "patch", filepath.Join(workDir, "controlplane.yaml")}
	for _, patch := range patches {
		args = append(args, "--patch", "@"+patch)
	}
	output := filepath.Join(staged, ControlPlaneFilename)
	args = append(args, "--output", output)
	if err := runTalosctl(args...); err != nil {
		return fmt.Errorf("apply Talos configuration documents: %w", err)
	}

	if err := os.Chmod(output, 0o600); err != nil {
		return fmt.Errorf("protect generated control-plane configuration: %w", err)
	}

	if err := runTalosctl("validate", "--config", output, "--mode", "metal"); err != nil {
		return err
	}
	tc := filepath.Join(workDir, "talosconfig")
	for _, field := range []string{"endpoint", "node"} {
		if err := runTalosctl("--talosconfig", tc, "config", field, helper.TalEnv["MASTER1IP_IP"]); err != nil {
			return err
		}
	}
	data, err := os.ReadFile(tc)
	if err != nil {
		return fmt.Errorf("read generated talosconfig: %w", err)
	}
	if err := os.WriteFile(filepath.Join(staged, TalosconfigFilename), data, 0o600); err != nil {
		return fmt.Errorf("write generated talosconfig: %w", err)
	}

	return publish(staged)
}

func renderPatches(workDir string) ([]string, error) {
	return renderPatchDirs(workDir, []string{"all", "control-plane"}, true)
}

func renderPatchDirs(workDir string, dirs []string, requireNonempty bool) ([]string, error) {
	var sourceFiles []string
	for _, relative := range dirs {
		dir := filepath.Join(helper.TalosPath, relative)
		entries, err := os.ReadDir(dir)
		if os.IsNotExist(err) && !requireNonempty && (relative == "worker" || relative == "control-plane") {
			continue
		}
		if os.IsNotExist(err) && !requireNonempty && strings.HasPrefix(relative, "nodes") {
			return nil, fmt.Errorf("node patch directory %s is required", dir)
		}
		if err != nil {
			return nil, fmt.Errorf("read Talos document directory %s: %w", dir, err)
		}
		count := 0
		for _, entry := range entries {
			if !entry.IsDir() && (strings.HasSuffix(entry.Name(), ".yaml") || strings.HasSuffix(entry.Name(), ".yml")) {
				sourceFiles = append(sourceFiles, filepath.Join(dir, entry.Name()))
				count++
			}
		}
		if count == 0 && requireNonempty {
			return nil, fmt.Errorf("no Talos documents in %s", dir)
		}
	}

	user, pass := helper.TalEnv["DOCKERHUB_USER"], helper.TalEnv["DOCKERHUB_PASSWORD"]
	if (user == "") != (pass == "") {
		return nil, fmt.Errorf("set both DOCKERHUB_USER and DOCKERHUB_PASSWORD or neither")
	}
	if user != "" {
		sourceFiles = append(sourceFiles, filepath.Join(helper.TalosPath, "optional", "44-registry-auth.yaml"))
	}
	rendered := make([]string, 0, len(sourceFiles))
	for index, source := range sourceFiles {
		raw, err := os.ReadFile(source)
		if err != nil {
			return nil, err
		}
		content, err := render(raw, helper.TalEnv)
		if err != nil {
			return nil, fmt.Errorf("render Talos document %s: %w", source, err)
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

