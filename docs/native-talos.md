# Native Talos configuration

This major version uses native Talos 1.14 documents. Talhelper, the previous
configuration layout and automatic migration are not supported.

## Files and initialization

`clustertool init` copies the embedded scaffold and creates `secrets.sops.yaml`
only when no identity exists. Existing secrets are retained.

```text
clusters/main/talos/
├── examples/
│   ├── 44-registry-auth.yaml
│   └── 50-nvidia.yaml
├── patches/
│   ├── all/
│   ├── control-plane/
│   ├── worker/
│   └── nodes/
│       └── control-1/
├── generated/
├── clustertool.yaml
└── secrets.sops.yaml
```

Edit source documents in `patches/`, not `generated/`. Generation reads YAML files
in filename order from `patches/all/`, then `patches/<role>/`, then
`patches/nodes/<name>/`. Later patches can override earlier settings.
The worker role directory may be empty. Each configured node requires its own
node directory, hostname, installer image and static management address.

## ClusterTool configuration

`clustertool.yaml` is required, including for a single-node cluster. Its
`apiVersion: clustertool/v1` describes the file format, not the binary or Talos
version. `kind: ClusterConfig` identifies this as ClusterTool configuration;
it is not a Kubernetes resource or a Talos document.

```yaml
apiVersion: clustertool/v1
kind: ClusterConfig
# renovate: datasource=github-releases depName=siderolabs/talos
talosVersion: v1.14.0
# renovate: datasource=docker depName=ghcr.io/siderolabs/kubelet
kubernetesVersion: v1.37.0
bootstrapNode: control-1
nodes:
  - name: control-1
    role: control-plane
    address: ${CONTROL1IP}

  # Optional: configure these addresses and create the node patch directories.
  # - name: control-2
  #   role: control-plane
  #   address: ${CONTROL2IP}
  # - name: worker-1
  #   role: worker
  #   address: ${WORKER1IP}
```

`bootstrapNode` selects the control plane used for initial bootstrap, not a
permanent leader. `name` selects the patch directory and generated filename;
the actual hostname is set in that node's `HostnameConfig`.
Missing files, unsupported format/kind, unknown fields, duplicate names/IPs and
invalid bootstrap selections stop generation. No environment-only fallback exists.

Both version fields are required: use `vMAJOR.MINOR.PATCH`, optionally followed
by a prerelease suffix such as `-rc.1`. ClusterTool passes `kubernetesVersion` to
`talosctl gen config --kubernetes-version`, so generated Kubernetes component
images share that version. Standard patches do not pin those images separately.
An explicit image in a custom patch still overrides the generated base.

`${TALOS_VERSION}` in Talos patches is reserved for `talosVersion` from this file;
an environment variable with the same name cannot override it. The embedded
`talosctl` version is selected when ClusterTool is built, not by this setting.
Use versions supported by that CLI and follow Talos/Kubernetes upgrade rules.
Changing these values and applying configuration does not replace the upgrade
procedure. Talos upgrades read each node's generated installer image; Kubernetes
upgrades read the generated native kubelet images and require a common version.

The inherited TrueForge Renovate YAML custom manager recognizes both comments
above in the embedded scaffold and in initialized cluster repositories.

## Storage and kubelet

`patches/all/31-storage.yaml` declares `longhorn` and `openebs` user volumes with
`volumeType: directory`. Their paths are `/var/mnt/longhorn` and `/var/mnt/openebs`
on the existing EPHEMERAL filesystem. They do not allocate partitions, reserve
space or provide separate capacity limits. Both roles receive these documents.
Talos propagates user volumes to kubelet; the standard kubelet patch uses native
`KubeletConfig` without legacy `machine.kubelet.extraMounts`.

The Longhorn HelmRelease sets `defaultSettings.defaultDataPath` to
`/var/mnt/longhorn`. The OpenEBS HelmRelease sets
`localpv-provisioner.localpv.basePath` to `/var/mnt/openebs`; its default
`openebs-hostpath` StorageClass inherits that path. Custom StorageClasses with an
explicit `BasePath` must use the intended path too. Chart versions are unchanged.

These are defaults for new installations, not a data migration. Existing
Longhorn disks and OpenEBS PV paths are not relocated by changing Helm values.
Validate on a fresh test cluster or with explicitly reviewed storage settings
and new test volumes. See [storage acceptance](storage-validation.md), including
pod recreation and a node reboot. Live storage behavior is not proven by offline
configuration validation.

## Addresses and networking

In `clusterenv.yaml`, specify bare management addresses:

```yaml
CONTROL1IP: 192.168.20.210
VIP: 192.168.20.200
GATEWAY: 192.168.20.1
# CONTROL2IP: 192.168.20.220
# WORKER1IP: 192.168.20.221
```

Values are substituted as entered. ClusterTool does not synthesize `_IP`,
`_CIDR` or `_NETMASK` variables or assume a subnet prefix. Specify each node's
prefix in its network patch explicitly:

```yaml
apiVersion: v1alpha1
kind: LinkConfig
name: eth0
up: true
addresses:
  - address: ${CONTROL1IP}/24
routes:
  - gateway: ${GATEWAY}
```

Choose the prefix and interface for your network. Do not include a prefix in
`CONTROL1IP` when the patch appends one. `PODNET` and `SVCNET` remain explicit
network CIDRs. Undefined variables fail rendering. Generation checks that the
management address agrees with the rendered network patch.

For another control plane, copy the starter node patches and change its hostname,
address variable, interface, disk selector and image as appropriate. The
`Layer2VIPConfig` uses `${VIP}` and must refer to that node's interface.
For a worker, also remove the `Layer2VIPConfig`. Workers receive the shared and
worker role layers, not the control-plane layer. `KubeTalosAPIAccessConfig` and
`KubeProxyConfig` must remain control-plane-only.

## Examples are inactive

Nothing in `examples/` is loaded automatically. Copy a document to `patches/all/`
to activate it on every node or to `patches/nodes/<name>/` for selected nodes.
Do not copy the same example into both layers unless you intend an override.

To enable Docker Hub authentication, copy `44-registry-auth.yaml` into an active
patch directory and set its `DOCKERHUB_USER` and `DOCKERHUB_PASSWORD` variables.
Setting those variables alone does not enable registry authentication. There is
no special Docker Hub variable check; normal substitution and Talos validation
apply to the active document. Protect credentials with the existing SOPS workflow.

For NVIDIA, copy `50-nvidia.yaml` to the relevant node patch directories and use
an image schematic containing the matching NVIDIA extensions.

## Generate and apply

```sh
clustertool genconfig
clustertool talos apply
# Target one configured node:
clustertool talos apply control-1
```

Apply regenerates and validates every node first, then executes the selected
nodes using `generated/<name>.yaml` directly. Failed generation retains previous
output and secrets. No `.execute` copies or generation locks are used; do not
run generation and apply concurrently.

## Image Factory schematic

`patches/nodes/control-1/00-install.yaml` documents the default customization:
`net.ifnames=0`, iscsi-tools, qemu-guest-agent and util-linux-tools. Each node can
specify a different `UnattendedInstallConfig.installer.image`:

```yaml
installer:
  image: factory.talos.dev/metal-installer/<schematic-id>:${TALOS_VERSION}
```

Register the desired customization with Image Factory and replace the image's
schematic ID. Keep the extensions and boot arguments the node still needs.
Applying configuration does not replace the running OS image. Activate a changed
schematic, including at the same Talos version, with:

```sh
clustertool talos upgrade control-3 --talos-only
```

This reboots the selected node. Check its running schematic with `talosctl get
extensions`. Tuppr remains independent: it discovers each running node's schematic
and does not read ClusterTool configuration files.

### Applying, resuming and upgrading

`talos apply` and `talos apply all` run sequentially, control planes before workers.
Name or IP selection targets one node. All configurations validate before execution;
commands use the files in `generated/` directly. Failure stops subsequent
nodes. Maintenance-mode apply uses that node's direct endpoint and insecure API;
established nodes require authenticated access and readiness.

During initial setup, the bootstrap node is installed first. Cilium and the CSR
approver are installed before remaining nodes join. Other charts and Flux follow.
An identity-bound `.bootstrap-in-progress.json` checkpoint allows `talos apply all`
to resume setup after interruption. Live etcd membership prevents a second bootstrap.
Already deployed Helm releases are kept; an existing failed/pending release requires
operator recovery before retrying. The checkpoint is removed after successful setup.
It contains no credentials and is ignored by the copied Git ignore file.

`talos upgrade [name-or-IP|all]` uses each selected node's installer image, waits for
recovery, then runs **one cluster-wide Kubernetes upgrade**, even with a single-node
selector. `--talos-only` skips that Kubernetes phase. Read-only version/readiness and
Kubernetes dry-run checks run before any Talos upgrades. A requested Talos downgrade
(for example, stale source after Tuppr upgraded the node) is refused. Kubernetes
versions must agree across the desired node configurations. If the Kubernetes dry
run requires newer Talos first, perform the Talos phase with `--talos-only` and retry.

Before control-plane maintenance, live etcd membership must match known generated
hostnames and every member must be ready. A two-member etcd cluster cannot retain
quorum during a reboot: apply is restricted to `no-reboot`, and rolling upgrades
are refused until a third healthy control plane is present. A single-control-plane
upgrade necessarily causes control-plane downtime. Talos upgrades use the CLI's
`--wait`; a configuration apply that requests reboot waits for a changed Linux boot
ID and readiness before proceeding. Cluster health and default kubeconfig retrieval
select a reachable authenticated control plane.


## Settings represented in the native documents

| Previous configuration | New file under `talos/` |
| --- | --- |
| Install disk at most 1600 GB; no disk wipe | `patches/nodes/control-1/00-install.yaml` (`disk.size <= 1600u * GB`, excluding loop devices, read-only devices and CD-ROMs) |
| Hostname `k8s-control-1` | `patches/nodes/control-1/01-hostname.yaml` |
| Machine certificate SANs `127.0.0.1` and VIP | `patches/all/02-machine.yaml` |
| Cluster pod/service networks | `patches/all/10-cluster.yaml` |
| `eth0`, static address, default gateway, VIP | `patches/nodes/control-1/20-network.yaml` |
| DNS `1.1.1.1`, `8.8.8.8`; all three hostDNS flags | `patches/all/21-resolver.yaml` |
| Server certificate rotation, maxPods 250, shutdown 15s/10s, GC 50/30/30m | `patches/all/30-kubelet.yaml` |
| OpenEBS and Longhorn bind mounts with `bind,rshared,rw` | `patches/all/30-kubelet.yaml` |
| Control-plane scheduling | `patches/control-plane/30-scheduling.yaml` |
| All 14 sysctls, Cloudflare NTP and 13 registry mirrors with original endpoint order | `patches/all/40-system.yaml` |
| `nvme_tcp`, `vfio_pci`, `uio_pci_generic` | `patches/all/41-kernel.yaml` |
| Containerd `discard_unpacked_layers = false` | `patches/all/42-cri.yaml` |
| `/etc/nfsmount.conf`, mode 0644, NFS 4.2/hard/nconnect 16/noatime | `patches/all/43-nfs.yaml` |
| Kubernetes 1.37, API access for system-upgrade, aggregator routing, controller/scheduler bind addresses, scheduler policy, disabled proxy and metrics address, PodSecurity exemptions, no Flannel | `patches/control-plane/10-kubernetes.yaml` |
| etcd metrics at `http://0.0.0.0:2381` | `patches/control-plane/20-etcd.yaml` |
| Docker Hub authentication, when copied into active patches | `examples/44-registry-auth.yaml` |
| Optional NVIDIA modules and BPF hardening | `examples/50-nvidia.yaml` |

The API server endpoint is passed to `talosctl gen config`; its additional SANs
are in `patches/control-plane/10-kubernetes.yaml`. The two existing storage mounts require the official legacy
`machine.kubelet` section: Talos 1.14's `KubeletConfig` has no `extraMounts` field.
The generated `KubeletConfig` document is deleted to avoid conflicting configuration.
The machine certificate SANs and etcd metrics also use official legacy fields.
These remain loose YAML patches; no replacement configuration schema is introduced.


See [multinode verification](multinode-validation.md) for automated coverage and
live acceptance. Encrypt secrets with `clustertool encrypt` before committing.
