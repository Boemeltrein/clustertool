package fluxhandler

import (
	"context"
	"testing"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic/fake"
	ktesting "k8s.io/client-go/testing"
)

func TestFluxHealthWaitIncludesInitialSync(t *testing.T) {
	instance := readyObject("fluxcd.controlplane.io/v1", "FluxInstance", "flux")
	instance.SetLabels(map[string]string{"app.kubernetes.io/instance": "flux-instance"})
	instance.Object["spec"] = map[string]interface{}{"sync": map[string]interface{}{"kind": "GitRepository", "name": "cluster"}}
	source := readyObject("source.toolkit.fluxcd.io/v1", "GitRepository", "cluster")
	sync := readyObject("kustomize.toolkit.fluxcd.io/v1", "Kustomization", "cluster")
	for _, test := range []struct {
		name      string
		objects   []runtime.Object
		wantReady bool
	}{
		{"missing sync", []runtime.Object{instance, source}, false},
		{"ready", []runtime.Object{instance, source, sync}, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			client := fake.NewSimpleDynamicClient(runtime.NewScheme(), test.objects...)
			h := &fluxHealthClient{client: client}
			ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
			defer cancel()
			err := h.wait(ctx, "fluxinstances", "flux-instance")
			if (err == nil) != test.wantReady {
				t.Fatalf("ready=%v, err=%v", test.wantReady, err)
			}
		})
	}
}

func TestFluxHealthForbiddenIsNotRetried(t *testing.T) {
	gvr := schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}
	client := fake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{gvr: "DeploymentList"})
	calls := 0
	client.PrependReactor("list", "deployments", func(ktesting.Action) (bool, runtime.Object, error) {
		calls++
		return true, nil, apierrors.NewForbidden(gvr.GroupResource(), "flux-operator", nil)
	})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	err := (&fluxHealthClient{client: client}).wait(ctx, "deployments", "flux-operator")
	if !apierrors.IsForbidden(err) || calls != 1 {
		t.Fatalf("calls=%d err=%v", calls, err)
	}
}

func readyObject(apiVersion, kind, name string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": apiVersion, "kind": kind,
		"metadata": map[string]interface{}{"name": name, "namespace": "flux-system", "generation": int64(1)},
		"status":   map[string]interface{}{"conditions": []interface{}{map[string]interface{}{"type": "Ready", "status": "True", "observedGeneration": int64(1)}}},
	}}
}
