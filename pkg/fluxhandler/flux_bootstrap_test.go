package fluxhandler

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/trueforge-org/clustertool/pkg/helper"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestRepeatedSecretGenerationAndKustomization(t *testing.T) {
	t.Chdir(t.TempDir())
	oldPath := helper.ClusterPath
	helper.ClusterPath = "clusters/lab"
	t.Cleanup(func() { helper.ClusterPath = oldPath })
	if err := CreateGitSecret("https://github.com/example/cluster"); err != nil {
		t.Fatal(err)
	}
	app := filepath.Join(helper.ClusterPath, "kubernetes", "flux-system", "flux-instance", "app")
	secret := filepath.Join(app, "deploykey.secret.yaml")
	first, err := os.ReadFile(secret)
	if err != nil {
		t.Fatal(err)
	}
	if err := CreateGitSecret("https://github.com/example/cluster"); err != nil {
		t.Fatal(err)
	}
	second, err := os.ReadFile(secret)
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Fatal("deploy key changed during repeated init")
	}
	// No comment is required to exclude the local age key.
	if err := os.WriteFile(filepath.Join(app, "sopssecret.secret.yaml"), []byte("kind: Secret\n"), 0600); err != nil {
		t.Fatal(err)
	}
	parent := filepath.Dir(app)
	ks := filepath.Join(parent, "ks.yaml")
	custom := []byte("spec:\n  dependsOn:\n    - name: flux-operator\n  wait: true\n")
	if err := os.WriteFile(ks, custom, 0644); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := ProcessDirectory(parent); err != nil {
			t.Fatal(err)
		}
	}
	content, err := os.ReadFile(filepath.Join(app, "kustomization.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(content), "sopssecret") || !strings.Contains(string(content), "deploykey.secret.yaml") {
		t.Fatalf("incorrect resources: %s", content)
	}
	content, err = os.ReadFile(ks)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != string(custom) {
		t.Fatal("replaced explicit dependency configuration")
	}
}

func TestInstallFluxStopsBeforeInstanceOnOperatorFailure(t *testing.T) {
	for _, failure := range []string{"", "deploykey.secret.yaml", "install:flux-operator", "ready:flux-operator"} {
		t.Run(failure, func(t *testing.T) {
			root := t.TempDir()
			for _, name := range []string{"flux-operator", "flux-instance"} {
				dir := filepath.Join(root, name, "app")
				if err := os.MkdirAll(dir, 0755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(dir, "helm-release.yaml"), []byte("metadata:\n  name: "+name+"\n  namespace: flux-system\n"), 0644); err != nil {
					t.Fatal(err)
				}
			}
			var calls []string
			operation := func(name string) error {
				calls = append(calls, name)
				if name == failure {
					return errors.New("test failure")
				}
				return nil
			}
			err := installFlux(context.Background(), root, nil,
				func(_ context.Context, file string) error { return operation(filepath.Base(file)) },
				func(charts []HelmChart, _ map[string]*HelmRepo, async bool) error {
					if async || !charts[0].Wait {
						t.Fatal("bootstrap installation must wait")
					}
					return operation("install:" + filepath.Base(filepath.Dir(charts[0].ChartPath)))
				},
				func(_ context.Context, resource, release string) error { return operation("ready:" + release) },
			)
			expected := []string{"namespace.yaml", "deploykey.secret.yaml", "sopssecret.secret.yaml", "clustersettings.secret.yaml", "install:flux-operator", "ready:flux-operator", "install:flux-instance", "ready:flux-instance"}
			if failure != "" {
				for i, name := range expected {
					if name == failure {
						expected = expected[:i+1]
						break
					}
				}
				if err == nil {
					t.Fatal("expected failure")
				}
			} else if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(calls, expected) {
				t.Fatalf("calls %v, want %v", calls, expected)
			}
		})
	}
}

func TestFluxReadyRequiresCurrentGeneration(t *testing.T) {
	obj := &unstructured.Unstructured{Object: map[string]interface{}{
		"kind":     "FluxInstance",
		"metadata": map[string]interface{}{"generation": int64(2)},
		"status":   map[string]interface{}{"conditions": []interface{}{map[string]interface{}{"type": "Ready", "status": "True", "observedGeneration": int64(1)}}},
	}}
	if fluxResourceReady(obj) {
		t.Fatal("accepted stale Ready condition")
	}
	_ = unstructured.SetNestedSlice(obj.Object, []interface{}{map[string]interface{}{"type": "Ready", "status": "True", "observedGeneration": int64(2)}}, "status", "conditions")
	if !fluxResourceReady(obj) {
		t.Fatal("rejected current Ready condition")
	}
}

func TestResolveChartSources(t *testing.T) {
	hr := &HelmRelease{Metadata: Metadata{Namespace: "flux-system"}}
	hr.Spec.Chart.Spec = ChartSpec{Chart: "example", Version: "1.2.3", SourceRef: SourceRef{Name: "example", Kind: "HelmRepository"}}
	repos := map[string]*HelmRepo{"example": {Metadata: HelmRepoMetadata{Namespace: "flux-system"}, Spec: HelmRepoSpec{URL: "https://charts.example.org"}}}
	source, err := ResolveChart(hr, repos, t.TempDir())
	if err != nil || source.Chart != "example" || source.Version != "1.2.3" {
		t.Fatalf("%+v %v", source, err)
	}
	dir := t.TempDir()
	file := filepath.Join(dir, "source.yaml")
	data := "kind: OCIRepository\nmetadata:\n  name: example\n  namespace: flux-system\nspec:\n  url: oci://ghcr.io/example/charts/operator\n  ref:\n    tag: 0.59.0\n"
	if err := os.WriteFile(file, []byte(data), 0644); err != nil {
		t.Fatal(err)
	}
	hr.Spec.Chart = Chart{}
	hr.Spec.ChartRef = &SourceRef{Kind: "OCIRepository", Name: "example"}
	source, err = ResolveChart(hr, nil, dir)
	if err != nil || source != (ChartSource{"oci://ghcr.io/example/charts", "operator", "0.59.0"}) {
		t.Fatalf("%+v %v", source, err)
	}
	hr.Spec.ChartRef.Namespace = "wrong"
	if _, err = ResolveChart(hr, nil, dir); err == nil {
		t.Fatal("matched wrong namespace")
	}
	hr.Spec.ChartRef.Namespace = ""
	if err := os.WriteFile(file, []byte(strings.ReplaceAll(data, "tag: 0.59.0", "semver: '*'")), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err = ResolveChart(hr, nil, dir); err == nil {
		t.Fatal("accepted unsupported version selector")
	}
}
