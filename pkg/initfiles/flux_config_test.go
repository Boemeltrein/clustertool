package initfiles

import (
	"github.com/trueforge-org/clustertool/pkg/helper"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUpdateFluxConfigPreservesUserSettings(t *testing.T) {
	oldPath, oldName, oldEnv := helper.ClusterPath, helper.ClusterName, helper.TalEnv
	t.Cleanup(func() { helper.ClusterPath, helper.ClusterName, helper.TalEnv = oldPath, oldName, oldEnv })
	helper.ClusterPath = t.TempDir()
	helper.ClusterName = "lab"
	helper.TalEnv = map[string]string{"GITHUB_REPOSITORY": "https://github.com/example/cluster.git"}
	file := filepath.Join(helper.ClusterPath, "kubernetes", "flux-system", "flux-instance", "app", "helm-release.yaml")
	if err := os.MkdirAll(filepath.Dir(file), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte("url: ssh://REPLACEWITHGITREPO\npath: clusters/REPLACEWITHCLUSTERNAME/flux-entry\nref: refs/heads/custom\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := UpdateFluxConfig(); err != nil {
		t.Fatal(err)
	}
	first, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(first), "ssh://git@github.com/example/cluster.git") || !strings.Contains(string(first), "clusters/lab/flux-entry") || !strings.Contains(string(first), "refs/heads/custom") {
		t.Fatalf("unexpected rendering: %s", first)
	}
	helper.TalEnv["GITHUB_REPOSITORY"] = "https://github.com/another/repo.git"
	if err := UpdateFluxConfig(); err != nil {
		t.Fatal(err)
	}
	second, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Fatal("overwrote existing user configuration")
	}
}
