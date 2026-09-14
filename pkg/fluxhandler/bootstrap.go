package fluxhandler

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/trueforge-org/clustertool/pkg/helper"
	"github.com/trueforge-org/clustertool/pkg/kubectlcmds"
	fthelper "github.com/trueforge-org/forgetool/v4/pkg/helper"
)

func init() {
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stdout}).With().Timestamp().Logger()
}

// FluxBootstrap is shared by Talos bootstrap and the standalone Flux command.
func FluxBootstrap(ctx context.Context) error {
	if helper.TalEnv["GITHUB_REPOSITORY"] == "" {
		return nil
	}
	if !fthelper.GetYesOrNo("Do you want to (re)bootstrap FluxCD as well? (yes/no) [y/n]: ", false) {
		return nil
	}
	if err := bootstrapFluxCD(ctx); err != nil {
		return fmt.Errorf("bootstrap Flux: %w", err)
	}
	log.Info().Msg("Flux bootstrapped successfully")
	return nil
}

func bootstrapFluxCD(ctx context.Context) error {
	isRepo, err := helper.IsCurrentDirGitRepo()
	if err != nil {
		return fmt.Errorf("check Git repository: %w", err)
	}
	if !isRepo {
		return fmt.Errorf("current directory is not a Git repository")
	}
	root := filepath.Join(helper.ClusterPath, "kubernetes", "flux-system")
	repos, err := LoadAllHelmRepos(filepath.Join("repositories", "helm"))
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("load Helm repositories: %w", err)
	}
	// Check local inputs before installing anything. Genconfig resolves placeholders.
	for _, file := range []string{
		filepath.Join(root, "namespace.yaml"),
		filepath.Join(root, "flux", "clustersettings.secret.yaml"),
		filepath.Join(root, "flux-instance", "app", "deploykey.secret.yaml"),
		filepath.Join(root, "flux-instance", "app", "sopssecret.secret.yaml"),
		filepath.Join(helper.ClusterPath, "flux-entry", "ks.yaml"),
		filepath.Join(root, "flux-operator", "app", "helm-release.yaml"),
		filepath.Join(root, "flux-instance", "app", "helm-release.yaml"),
	} {
		data, err := os.ReadFile(file)
		if err != nil {
			return fmt.Errorf("read Flux bootstrap file %s: %w", file, err)
		}
		if strings.Contains(string(data), "REPLACEWITH") {
			return fmt.Errorf("unresolved placeholder in %s; run genconfig first", file)
		}
	}
	for _, chart := range []string{"flux-operator", "flux-instance"} {
		dir := filepath.Join(root, chart, "app")
		hr, err := LoadHelmRelease(filepath.Join(dir, "helm-release.yaml"))
		if err != nil {
			return err
		}
		if hr.Metadata.Namespace != "flux-system" {
			return fmt.Errorf("%s: bootstrap requires namespace flux-system", dir)
		}
		if _, err := ResolveChart(hr, repos, filepath.Join("repositories", "oci")); err != nil {
			return fmt.Errorf("%s: %w", dir, err)
		}
	}
	health, err := newFluxHealthClient()
	if err != nil {
		return err
	}
	return installFlux(ctx, root, repos, kubectlcmds.KubectlApply, InstallCharts, health.wait)
}

// installFlux keeps credentials and both chart installations in one sequence.
func installFlux(ctx context.Context, root string, repos map[string]*HelmRepo,
	apply func(context.Context, string) error,
	install func([]HelmChart, map[string]*HelmRepo, bool) error,
	ready func(context.Context, string, string) error,
) error {
	instance := filepath.Join(root, "flux-instance", "app")
	for _, file := range []string{filepath.Join(root, "namespace.yaml"), filepath.Join(instance, "deploykey.secret.yaml"), filepath.Join(instance, "sopssecret.secret.yaml"), filepath.Join(root, "flux", "clustersettings.secret.yaml")} {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := apply(ctx, file); err != nil {
			return fmt.Errorf("apply %s: %w", file, err)
		}
	}
	for _, chart := range []string{"flux-operator", "flux-instance"} {
		if err := ctx.Err(); err != nil {
			return err
		}
		dir := filepath.Join(root, chart, "app")
		if err := install([]HelmChart{{ChartPath: dir, Wait: true}}, repos, false); err != nil {
			return err
		}
		resource := "deployments"
		if chart == "flux-instance" {
			resource = "fluxinstances"
		}
		// Helm release labels allow chart fullname overrides.
		hr, err := LoadHelmRelease(filepath.Join(dir, "helm-release.yaml"))
		if err != nil {
			return err
		}
		release := hr.Metadata.Name
		if hr.Spec.ReleaseName != "" {
			release = hr.Spec.ReleaseName
		}
		log.Info().Msgf("Bootstrap: Waiting for %s to be ready", chart)
		waitCtx, cancel := context.WithTimeout(ctx, 20*time.Minute)
		err = ready(waitCtx, resource, release)
		cancel()
		if err != nil {
			return fmt.Errorf("wait for %s: %w", chart, err)
		}
	}
	return nil
}
