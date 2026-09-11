# Multinode repair and verification

This audit repairs the native-Talos draft PR against the agreed multinode design.
The supported embedded CLI is Talos 1.14. No live cluster was changed during the
audit. Offline passing tests do not establish successful live HA upgrades.

## Repaired behavior

| Area | Repair |
| --- | --- |
| Apply routing | Inventory name/IP selection replaces the hard-coded first-node bootstrap decision. A new worker joins an authenticated existing cluster. |
| Commands | Argument arrays preserve paths with spaces. Forwarded flags cannot change the validated node, input, image or authentication mode. |
| Generation | Every node validates before publication; failed worker generation retains the old output and secrets. Static management IP and worker role/VIP consistency are checked. |
| Execution | Commands use generated files directly, with no mid-execution regeneration. Execution stops on command/readiness failure. |
| Readiness | `ready: false` is an error. Established apply waits for reboot identity changes when a reboot is reported; upgrade uses Talos `--wait`. |
| Bootstrap | CNI/CSR installation precedes joins. An identity-bound checkpoint permits explicit initial setup to resume; live etcd evidence prevents a duplicate bootstrap. Chart errors propagate. |
| Upgrade | Per-node image/schematic is retained; obsolete `--preserve` is removed. Version and quorum checks precede mutation. Kubernetes runs once, with an initial dry run; `--talos-only` skips it. |
| Control-plane failover | Cluster health and default kubeconfig/Kubernetes-upgrade commands select a reachable authenticated control plane. |
| Configuration | Required `clustertool.yaml` with `apiVersion: clustertool/v1` and `kind: ClusterConfig`; no legacy fallback or migration. Source variables are preserved without derived IP values. |
| Scaffold | The inventory and node patches remain copied embedded files. There is no programmatic template generator. |

One design correction is required: **Talos 1.14 permits `KubeTalosAPIAccessConfig`
and `KubeProxyConfig` only on control-plane machines.** Moving them to `patches/all/`
fails real worker validation. They remain control-plane documents. Tuppr continues
to discover schematics from running nodes; it does not consume this inventory.

## Automated evidence

Run from the repository with Go and the matching `talosctl` on PATH:

```sh
TALOSCTL_INTEGRATION=1 go test ./...
go vet ./...
```

The real-generation fixture uses two control planes and a worker, three distinct
schematic images and disk selectors, unique hostnames/static IPs, shared/role/node
patch precedence, and persistent secrets. It also checks invalid-worker rejection,
wrong static management address rejection, absence of control-plane-only worker
documents, client endpoint selection and preservation of previous outputs.

The version/storage change additionally verifies non-default central versions
(Talos v1.14.1 and Kubernetes v1.36.3 for offline generation), all control-plane
component images, native kubelet images, directory volumes, and each node's disk
selector. Missing/malformed versions are rejected. A stale TALOS_VERSION in the
environment neither overrides the configured version nor gets mutated.

The pinned Longhorn 1.12.0 and OpenEBS localpv-provisioner 4.6.0 chart source
templates were rendered with the HelmRelease values: Longhorn's default settings,
the OpenEBS StorageClass and the provisioner fallback path use `/var/mnt/`.
Chart rendering does not prove runtime mounting or data persistence; follow
[storage acceptance](storage-validation.md) on the operator's test cluster.

Execution tests use mocked nodes and processes, not a networked cluster. They cover
argument boundaries, maintenance endpoints, failure stopping, readiness recovery,
reboot identity changes, two-member quorum protection, downgrade rejection,
bootstrap-state decisions, checkpoint identity binding.

The Linux amd64 PR workflow runs the tests with the embedded Talos version before
packaging the test archive. Its successful run and commit SHA must be checked
before using a downloaded binary.

## Recorded operator results

On 2026-09-10, build `test-0124720f10ec` completed fresh single-control-plane
bootstrap and Flux installation on Talos v1.14.0 / Kubernetes v1.37.0. A second
control plane and a worker were subsequently added to that running cluster;
all three Kubernetes nodes reported Ready. A worker showing `<none>` in the
ROLES column is expected without a `node-role.kubernetes.io/worker` label.

On the same build, fresh Longhorn/OpenEBS PVCs retained their original marker
after pod recreation and a test-node reboot. The operator confirmed Longhorn's
disk path `/var/mnt/longhorn` and the OpenEBS PV path under `/var/mnt/openebs`.
This storage result covers the original control-plane node, not every later
joining node or Longhorn replica failover.

Earlier build `test-184f333f7628` was tested with three control planes, a
node-specific label change and a same-version schematic upgrade. Applying the
image setting alone did not change the running extensions; `upgrade --talos-only`
activated the new image. Those results precede the final layout/storage changes.

The cleanup adds subprocess tests for command failure/success exit codes and
stopping before further work after decryption errors, plus Helm readiness and
timeout tests with an in-memory release store. They do not replace live tests.
Controlled interrupted-bootstrap recovery, a combined Talos/Kubernetes upgrade,
VIP failover, storage on the additional roles and Tuppr acceptance remain live
checks to record separately. CI runner configuration is intentionally unchanged
in this fork; unavailable fork runners are not evidence of passing checks.

## Operator acceptance on a disposable cluster

Use **three control planes and at least one worker** to test rolling HA maintenance.
The two-control-plane offline fixture proves configuration generation, not HA:
two voting etcd members cannot keep quorum while one reboots.

1. Review each address in `clustertool.yaml`, hostname, installer image, disk selector and
   static network patch. Remove VIP documents from workers. Back up credentials.
2. Run `clustertool genconfig`. Confirm one generated file per inventory node
   and a talosconfig whose endpoints are all control planes. Compare intended node
   settings and verify that secrets have not changed.
3. Run `clustertool talos apply all` for initial installation. Confirm that bootstrap
   happens once, Cilium/CSR become available, and remaining nodes join in order.
   Confirm all Kubernetes nodes are Ready and Talos cluster health succeeds.
4. Interrupt a separate disposable bootstrap after etcd is established. Resume with
   `talos apply all`; confirm no second bootstrap RPC and no reinstall of already
   deployed Helm releases. A failed/pending Helm release must be recovered explicitly.
5. Add a new worker to an existing cluster. Apply by configured name; confirm that
   no new-cluster prompt appears and that the worker receives its own image/network.
6. Add a small native `KubeNodeConfig.labels` patch to one node, such as
   `clustertool.test: "true"`. Generate, apply that name, and verify the Kubernetes
   label only on that node. Remove the patch and verify the reverse change.
7. Introduce an invalid patch on a later node. Confirm genconfig fails and previous
   generated files remain intact. Restore the patch. Separately test an unavailable
   node during apply and confirm subsequent nodes are not processed after failure.
8. Test a deliberate same-version schematic change with
   `talos upgrade <name> --talos-only`. Confirm running extensions and recovery.
   Use a valid registered image, not the synthetic test schematic hashes.
9. Test the supported combined Talos/Kubernetes upgrade path. Confirm Talos nodes
   recover sequentially and only one cluster-wide Kubernetes phase runs. Test a
   stale requested Talos version after Tuppr: preflight must reject the downgrade.
10. Verify storage mounts, labels/taints, VIP failover and Tuppr permissions/image
    preservation on both roles. These behaviors require the live environment.

Do not merge based solely on offline tests. Record the tested commit, node versions
and results of the live steps in the draft PR.

## References

- [Talos 1.14 CLI](https://docs.siderolabs.com/talos/v1.14/reference/cli)
- [Talos configuration map](https://docs.siderolabs.com/talos/v1.14/reference/configuration/document-map)
- [Talos Kubernetes upgrade dry run](https://docs.siderolabs.com/kubernetes-guides/advanced-guides/upgrading-kubernetes)
- [Pinned etcd membership command](https://github.com/siderolabs/talos/blob/v1.14.0/cmd/talosctl/cmd/talos/etcd.go)
- [Tuppr 0.5.3 image resolution](https://github.com/home-operations/tuppr/blob/0.5.3/docs/talos-upgrades.md)
