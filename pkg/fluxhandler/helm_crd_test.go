package fluxhandler

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"helm.sh/helm/v4/pkg/action"
	"helm.sh/helm/v4/pkg/chart/common"
	chart "helm.sh/helm/v4/pkg/chart/v2"
	"helm.sh/helm/v4/pkg/kube"
	"helm.sh/helm/v4/pkg/kube/fake"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/cli-runtime/pkg/resource"
)

// The CRD remains unavailable until a readiness waiter observes registration.
// There are no wall-clock sleeps, so the race is deterministic on every OS.
type crdRegistrationClient struct {
	fake.PrintingKubeClient
	hookOnly      kube.Waiter
	registered    bool
	crdWaits      int
	workloadWaits int
	manifestBuilt bool
	waitErr       error
}

func (c *crdRegistrationClient) Build(r io.Reader, _ bool) (kube.ResourceList, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	if strings.Contains(string(data), "kind: CustomResourceDefinition") {
		return kube.ResourceList{&resource.Info{Object: &unstructured.Unstructured{Object: map[string]interface{}{
			"apiVersion": "apiextensions.k8s.io/v1", "kind": "CustomResourceDefinition",
			"metadata": map[string]interface{}{"name": "servicemonitors.monitoring.coreos.com"},
		}}}}, nil
	}
	if strings.Contains(string(data), "kind: ServiceMonitor") {
		c.manifestBuilt = true
		if !c.registered {
			return nil, errors.New("no matches for kind ServiceMonitor: CRD registration has not completed")
		}
	}
	// Resource application is outside this fixture's scope. Only discovery order
	// and the action's selection of readiness waiters are under test.
	return nil, nil
}

func (c *crdRegistrationClient) Create(r kube.ResourceList, _ ...kube.ClientCreateOption) (*kube.Result, error) {
	return &kube.Result{Created: r}, nil
}

func (c *crdRegistrationClient) GetWaiter(s kube.WaitStrategy) (kube.Waiter, error) {
	return c.GetWaiterWithOptions(s)
}

func (c *crdRegistrationClient) GetWaiterWithOptions(s kube.WaitStrategy, _ ...kube.WaitOption) (kube.Waiter, error) {
	if s == kube.HookOnlyStrategy {
		return c.hookOnly, nil
	}
	return &registrationWaiter{PrintingKubeWaiter: fake.PrintingKubeWaiter{Out: io.Discard}, client: c}, nil
}

type registrationWaiter struct {
	fake.PrintingKubeWaiter
	client *crdRegistrationClient
}

func (w *registrationWaiter) Wait(r kube.ResourceList, timeout time.Duration) error {
	if len(r) > 0 && r[0].Object.GetObjectKind().GroupVersionKind().Kind == "CustomResourceDefinition" {
		w.client.crdWaits++
		if timeout != 60*time.Second {
			return errors.New("CRD registration timeout changed")
		}
		if w.client.waitErr != nil {
			return w.client.waitErr
		}
		w.client.registered = true
	} else {
		w.client.workloadWaits++
	}
	return nil
}

func realHookOnlyWaiter(t *testing.T) kube.Waiter {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("hookOnly unexpectedly contacted Kubernetes: %s", r.URL)
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	t.Cleanup(server.Close)
	configPath := filepath.Join(t.TempDir(), "kubeconfig")
	if err := os.WriteFile(configPath, []byte("apiVersion: v1\nkind: Config\nclusters: []\ncontexts: []\nusers: []\n"), 0600); err != nil {
		t.Fatal(err)
	}
	flags := genericclioptions.NewConfigFlags(false)
	flags.KubeConfig = &configPath
	flags.APIServer = &server.URL
	waiter, err := kube.New(flags).GetWaiter(kube.HookOnlyStrategy)
	if err != nil {
		t.Fatal(err)
	}
	return waiter
}

func TestInstallWaitsForCRDRegistration(t *testing.T) {
	registrationErr := errors.New("CRD registration timed out")
	for _, tc := range []struct {
		name     string
		strategy kube.WaitStrategy
		waitErr  error
	}{
		{"without workload waiting", kube.HookOnlyStrategy, nil},
		{"with workload waiting", kube.StatusWatcherStrategy, nil},
		{"registration fails", kube.HookOnlyStrategy, registrationErr},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg, _ := helmTestConfig()
			client := &crdRegistrationClient{PrintingKubeClient: fake.PrintingKubeClient{Out: io.Discard}, hookOnly: realHookOnlyWaiter(t), waitErr: tc.waitErr}
			cfg.KubeClient = client
			install := action.NewInstall(cfg)
			install.ReleaseName, install.Namespace = "monitoring", "test"
			install.WaitStrategy = tc.strategy
			install.Timeout = time.Second
			ch := &chart.Chart{
				Metadata:  &chart.Metadata{Name: "monitoring", Version: "1.0.0", APIVersion: "v2"},
				Files:     []*common.File{{Name: "crds/monitor.yaml", Data: []byte("apiVersion: apiextensions.k8s.io/v1\nkind: CustomResourceDefinition\nmetadata:\n  name: servicemonitors.monitoring.coreos.com\n")}},
				Templates: []*common.File{{Name: "templates/monitor.yaml", Data: []byte("apiVersion: monitoring.coreos.com/v1\nkind: ServiceMonitor\nmetadata:\n  name: example\n")}},
			}
			_, err := installRelease(install, ch, nil)
			if !errors.Is(err, tc.waitErr) {
				t.Fatalf("install error = %v; want %v", err, tc.waitErr)
			}
			if client.crdWaits != 1 {
				t.Fatalf("CRD readiness checks = %d; want 1", client.crdWaits)
			}
			if tc.waitErr != nil && client.manifestBuilt {
				t.Fatal("processed custom resources despite failed CRD registration")
			}
			wantWorkloadWaits := 0
			if tc.strategy == kube.StatusWatcherStrategy && tc.waitErr == nil {
				wantWorkloadWaits = 1
			}
			if client.workloadWaits != wantWorkloadWaits {
				t.Fatalf("workload readiness checks = %d; want %d", client.workloadWaits, wantWorkloadWaits)
			}
		})
	}
}
