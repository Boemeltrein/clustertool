package fluxhandler

import (
	"time"

	"helm.sh/helm/v4/pkg/kube"
)

// Helm 4.3 uses the install's wait strategy for CRD registration as well as
// workloads. HookOnlyStrategy.Wait is a no-op, allowing discovery to run before
// new CRDs are established. Keep Helm's CRD installation and discovery reset,
// but restore the registration barrier without enabling workload waiting.
type crdRegistrationKubeClient struct {
	kube.Interface
}

func (c *crdRegistrationKubeClient) GetWaiter(strategy kube.WaitStrategy) (kube.Waiter, error) {
	return c.GetWaiterWithOptions(strategy)
}

func (c *crdRegistrationKubeClient) GetWaiterWithOptions(strategy kube.WaitStrategy, opts ...kube.WaitOption) (kube.Waiter, error) {
	getWaiter := func(s kube.WaitStrategy) (kube.Waiter, error) {
		if client, ok := c.Interface.(kube.InterfaceWaitOptions); ok {
			return client.GetWaiterWithOptions(s, opts...)
		}
		return c.Interface.GetWaiter(s)
	}
	waiter, err := getWaiter(strategy)
	if err != nil || strategy != kube.HookOnlyStrategy {
		return waiter, err
	}
	return &crdRegistrationWaiter{Waiter: waiter, getWaiter: getWaiter}, nil
}

type crdRegistrationWaiter struct {
	kube.Waiter
	getWaiter func(kube.WaitStrategy) (kube.Waiter, error)
}

func (w *crdRegistrationWaiter) Wait(resources kube.ResourceList, timeout time.Duration) error {
	var crds kube.ResourceList
	for _, info := range resources {
		if info == nil || info.Object == nil {
			continue
		}
		gvk := info.Object.GetObjectKind().GroupVersionKind()
		if gvk.Group == "apiextensions.k8s.io" && gvk.Kind == "CustomResourceDefinition" {
			crds = append(crds, info)
		}
	}
	if len(crds) > 0 {
		waiter, err := w.getWaiter(kube.StatusWatcherStrategy)
		if err != nil {
			return err
		}
		if err := waiter.Wait(crds, timeout); err != nil {
			return err
		}
	}
	return w.Waiter.Wait(resources, timeout)
}
