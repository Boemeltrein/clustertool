package fluxhandler

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type ChartSource struct{ URL, Chart, Version string }

// ResolveChart reads local sources: no running Flux controllers are required.
func ResolveChart(hr *HelmRelease, repos map[string]*HelmRepo, ociDir string) (ChartSource, error) {
	if hr.Spec.ChartRef == nil {
		ref := hr.Spec.Chart.Spec.SourceRef
		if ref.Kind != "" && ref.Kind != "HelmRepository" {
			return ChartSource{}, fmt.Errorf("unsupported chart source kind %q", ref.Kind)
		}
		repo := repos[ref.Name]
		if repo == nil || repo.Spec.URL == "" {
			return ChartSource{}, fmt.Errorf("HelmRepository %s not found", ref.Name)
		}
		ns := ref.Namespace
		if ns == "" {
			ns = hr.Metadata.Namespace
		}
		if ns != "" && repo.Metadata.Namespace != ns {
			return ChartSource{}, fmt.Errorf("HelmRepository %s not found in namespace %s", ref.Name, ns)
		}
		if hr.Spec.Chart.Spec.Chart == "" || hr.Spec.Chart.Spec.Version == "" {
			return ChartSource{}, fmt.Errorf("chart name and version are required")
		}
		return ChartSource{repo.Spec.URL, hr.Spec.Chart.Spec.Chart, hr.Spec.Chart.Spec.Version}, nil
	}
	ref := hr.Spec.ChartRef
	if hr.Spec.Chart.Spec.Chart != "" {
		return ChartSource{}, fmt.Errorf("use either chart or chartRef")
	}
	if ref.Kind != "OCIRepository" {
		return ChartSource{}, fmt.Errorf("unsupported chartRef kind %q", ref.Kind)
	}
	ns := ref.Namespace
	if ns == "" {
		ns = hr.Metadata.Namespace
	}
	files, err := os.ReadDir(ociDir)
	if err != nil {
		return ChartSource{}, fmt.Errorf("read OCI repositories: %w", err)
	}
	var found *ChartSource
	for _, file := range files {
		if file.IsDir() || !(strings.HasSuffix(file.Name(), ".yaml") || strings.HasSuffix(file.Name(), ".yml")) {
			continue
		}
		data, err := os.ReadFile(filepath.Join(ociDir, file.Name()))
		if err != nil {
			return ChartSource{}, err
		}
		var source struct {
			Kind     string   `yaml:"kind"`
			Metadata Metadata `yaml:"metadata"`
			Spec     struct {
				URL string `yaml:"url"`
				Ref struct {
					Tag    string `yaml:"tag"`
					Digest string `yaml:"digest"`
					Semver string `yaml:"semver"`
				} `yaml:"ref"`
			} `yaml:"spec"`
		}
		if err := yaml.Unmarshal(data, &source); err != nil {
			return ChartSource{}, fmt.Errorf("parse OCI source %s: %w", file.Name(), err)
		}
		if source.Kind != "OCIRepository" || source.Metadata.Name != ref.Name || source.Metadata.Namespace != ns {
			continue
		}
		if found != nil {
			return ChartSource{}, fmt.Errorf("duplicate OCIRepository %s/%s", ns, ref.Name)
		}
		if source.Spec.Ref.Tag == "" || source.Spec.Ref.Digest != "" || source.Spec.Ref.Semver != "" {
			return ChartSource{}, fmt.Errorf("OCIRepository %s/%s: bootstrap requires an explicit ref.tag", ns, ref.Name)
		}
		url := strings.TrimSuffix(source.Spec.URL, "/")
		if !strings.HasPrefix(url, "oci://") || !strings.Contains(strings.TrimPrefix(url, "oci://"), "/") {
			return ChartSource{}, fmt.Errorf("OCIRepository %s/%s: invalid chart URL", ns, ref.Name)
		}
		chart := path.Base(url)
		found = &ChartSource{strings.TrimSuffix(url, "/"+chart), chart, source.Spec.Ref.Tag}
	}
	if found == nil {
		return ChartSource{}, fmt.Errorf("OCIRepository %s/%s not found", ns, ref.Name)
	}
	return *found, nil
}
