# Storage acceptance for native kubelet configuration

Run these steps yourself on a disposable cluster using the new templates and
HelmRelease values. The coding agent has not run these live checks. Record the
ClusterTool commit, Talos/Kubernetes versions and results before merging.

Use fresh volumes. Changing a default path does not move existing Longhorn
replicas or OpenEBS PV data. Confirm that Flux/Helm has applied the updated
HelmReleases; `genconfig` alone does not reconcile those releases.

## Check configuration and storage readiness

1. Run `clustertool genconfig` and inspect every generated node configuration:
   native `KubeletConfig`, no `machine.kubelet`, two directory `UserVolumeConfig`
   documents, the expected component versions and installer schematic.
2. Apply the configurations to the test cluster. Check the user volume status
   on each storage node (replace the address with that node's management IP):

   ```sh
   talosctl --talosconfig clusters/main/talos/generated/talosconfig \
     -n 192.0.2.11 -e 192.0.2.11 get volumestatus u-longhorn -o yaml
   talosctl --talosconfig clusters/main/talos/generated/talosconfig \
     -n 192.0.2.11 -e 192.0.2.11 get volumestatus u-openebs -o yaml
   ```

   Use ClusterTool's cached `talosctl` executable if it is not on PATH. Both
   volumes should be ready with the intended `/var/mnt/` paths. Test with the
   actual `SecurityProfileConfig.workloadIsolation` setting you intend to use.
3. Check Longhorn's default data path and each configured disk in its UI. New
   disks should use `/var/mnt/longhorn`. Check the generated OpenEBS class:

   ```sh
   kubectl get storageclass openebs-hostpath -o yaml
   kubectl get pods -n longhorn-system
   kubectl get pods -n openebs
   ```

   The StorageClass BasePath must be `/var/mnt/openebs`. Both storage systems
   must be healthy before continuing. A ready Pod alone is not a persistence test.

## Create a workload using both storage systems

Save this as `storage-smoke.yaml` and apply it with `kubectl apply -f storage-smoke.yaml`.
Use the actual StorageClass names if yours differ from these template defaults.

```yaml
apiVersion: v1
kind: Namespace
metadata:
  name: storage-smoke
---
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: longhorn
  namespace: storage-smoke
spec:
  accessModes: [ReadWriteOnce]
  storageClassName: longhorn
  resources:
    requests:
      storage: 1Gi
---
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: openebs
  namespace: storage-smoke
spec:
  accessModes: [ReadWriteOnce]
  storageClassName: openebs-hostpath
  resources:
    requests:
      storage: 1Gi
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: storage-smoke
  namespace: storage-smoke
spec:
  replicas: 1
  strategy:
    type: Recreate
  selector:
    matchLabels:
      app: storage-smoke
  template:
    metadata:
      labels:
        app: storage-smoke
    spec:
      containers:
        - name: check
          image: busybox:1.37.0
          command: [sh, -c, "sleep 86400"]
          volumeMounts:
            - name: longhorn
              mountPath: /longhorn
            - name: openebs
              mountPath: /openebs
      volumes:
        - name: longhorn
          persistentVolumeClaim:
            claimName: longhorn
        - name: openebs
          persistentVolumeClaim:
            claimName: openebs
```

Wait for readiness and write a unique marker once:

```sh
kubectl -n storage-smoke rollout status deployment/storage-smoke --timeout=5m
kubectl -n storage-smoke exec deployment/storage-smoke -- sh -c \
  'date -u > /longhorn/marker; cp /longhorn/marker /openebs/marker; sync; cat /longhorn/marker'
```

Record the marker outside the Pod. Confirm both PVCs are Bound and inspect the
OpenEBS PV's local path: it must be beneath `/var/mnt/openebs`.

```sh
kubectl -n storage-smoke get pvc
kubectl -n storage-smoke rollout restart deployment/storage-smoke
kubectl -n storage-smoke rollout status deployment/storage-smoke --timeout=5m
kubectl -n storage-smoke exec deployment/storage-smoke -- sh -c \
  'cat /longhorn/marker /openebs/marker; cmp /longhorn/marker /openebs/marker'
```

Both markers must match the original value. The container never recreates them
on startup, so an empty replacement volume cannot give a false pass.

## Reboot and recover

Find the hosting node with `kubectl -n storage-smoke get pods -o wide`. Reboot
that test node using the normal Talos procedure, one node at a time and only with
healthy control-plane quorum. Wait for the node and storage services to recover,
then repeat the read/compare command above. Record the node, recovery time and
any mount errors. OpenEBS LocalPV remains tied to its original node; waiting for
that node during its reboot is expected, not automatic storage failover.

On a multinode cluster, repeat with a fresh pair of PVCs on another storage node
to cover control-plane and worker roles. Do not move an existing OpenEBS claim to
another node as part of this test.

After recording the results, remove only these disposable test resources if
desired: `kubectl delete namespace storage-smoke`. This also deletes the test
PVCs and, with the default Delete reclaim policy, their data.
