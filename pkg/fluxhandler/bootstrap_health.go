package fluxhandler

import (
	"context"
	"fmt"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/clientcmd"
)

type fluxHealthClient struct{ client dynamic.Interface }

func newFluxHealthClient() (*fluxHealthClient, error) {
	config, err := clientcmd.BuildConfigFromFlags("", clientcmd.RecommendedHomeFile)
	if err != nil {
		return nil, fmt.Errorf("load kubeconfig: %w", err)
	}
	config.Timeout = 30 * time.Second
	client, err := dynamic.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("create Flux health client: %w", err)
	}
	return &fluxHealthClient{client: client}, nil
}

func (h *fluxHealthClient) wait(ctx context.Context, resource, release string) error {
	gvr := schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: resource}
	if resource == "fluxinstances" {
		gvr.Group = "fluxcd.controlplane.io"
	}
	last := "resource not found"
	err := wait.PollUntilContextCancel(ctx, 5*time.Second, true, func(ctx context.Context) (bool, error) {
		objects, err := h.client.Resource(gvr).Namespace("flux-system").List(ctx, metav1.ListOptions{LabelSelector: "app.kubernetes.io/instance=" + release})
		if err != nil {
			if apierrors.IsForbidden(err) || apierrors.IsUnauthorized(err) {
				return false, err
			}
			last = err.Error()
			return false, nil
		}
		if len(objects.Items) == 0 {
			last = "resource not found"
			return false, nil
		}
		for _, obj := range objects.Items {
			if !fluxResourceReady(&obj) {
				last = obj.GetName() + " is not ready"
				conditions, _, _ := unstructured.NestedSlice(obj.Object, "status", "conditions")
				for _, raw := range conditions {
					condition, _ := raw.(map[string]interface{})
					if condition["type"] == "Ready" && condition["message"] != nil {
						last = fmt.Sprint(condition["message"])
					}
				}
				return false, nil
			}
			if resource == "fluxinstances" {
				ok, message, err := h.syncReady(ctx, &obj)
				if err != nil {
					return false, err
				}
				if !ok {
					last = message
					return false, nil
				}
			}
		}
		return true, nil
	})
	if err != nil {
		return fmt.Errorf("%s for release %s: %s: %w", resource, release, last, err)
	}
	return nil
}

// Read sync identity from the installed instance, including chart value overrides.
func (h *fluxHealthClient) syncReady(ctx context.Context, instance *unstructured.Unstructured) (bool, string, error) {
	name, _, _ := unstructured.NestedString(instance.Object, "spec", "sync", "name")
	if name == "" {
		name = instance.GetNamespace()
	}
	kind, _, _ := unstructured.NestedString(instance.Object, "spec", "sync", "kind")
	sources := map[string]string{"GitRepository": "gitrepositories", "OCIRepository": "ocirepositories", "Bucket": "buckets"}
	resource, ok := sources[kind]
	if !ok {
		return false, "", fmt.Errorf("FluxInstance %s has no supported sync source", instance.GetName())
	}
	for _, gvr := range []schema.GroupVersionResource{
		{Group: "source.toolkit.fluxcd.io", Version: "v1", Resource: resource},
		{Group: "kustomize.toolkit.fluxcd.io", Version: "v1", Resource: "kustomizations"},
	} {
		obj, err := h.client.Resource(gvr).Namespace(instance.GetNamespace()).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			if apierrors.IsForbidden(err) || apierrors.IsUnauthorized(err) {
				return false, "", err
			}
			return false, err.Error(), nil
		}
		if !fluxResourceReady(obj) {
			return false, fmt.Sprintf("%s/%s is not ready; inspect its status with kubectl", gvr.Resource, name), nil
		}
	}
	return true, "", nil
}

func fluxResourceReady(obj *unstructured.Unstructured) bool {
	if obj.GetDeletionTimestamp() != nil {
		return false
	}
	if obj.GetKind() == "Deployment" {
		observed, _, _ := unstructured.NestedInt64(obj.Object, "status", "observedGeneration")
		desired, found, _ := unstructured.NestedInt64(obj.Object, "spec", "replicas")
		if !found {
			desired = 1
		}
		updated, _, _ := unstructured.NestedInt64(obj.Object, "status", "updatedReplicas")
		available, _, _ := unstructured.NestedInt64(obj.Object, "status", "availableReplicas")
		return observed >= obj.GetGeneration() && desired > 0 && updated >= desired && available >= desired
	}
	conditions, _, _ := unstructured.NestedSlice(obj.Object, "status", "conditions")
	for _, raw := range conditions {
		condition, ok := raw.(map[string]interface{})
		if !ok || condition["type"] != "Ready" {
			continue
		}
		observed, _, _ := unstructured.NestedInt64(condition, "observedGeneration")
		return condition["status"] == "True" && observed >= obj.GetGeneration()
	}
	return false
}
