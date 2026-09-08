# Native Talos 1.14

Clustertool uses `talosctl` directly. Talhelper and its configuration schema are
removed. The embedded scaffold is inventory-first: `init` copies
`talos/inventory.yaml`, the role directories and a `nodes/control-1/` patch set.
The starter inventory uses `MASTER1IP` for its first control-plane address and
can be extended with additional nodes.

## Init, genconfig and apply

1. Run `clustertool init` in your cluster repository. On a new repository it creates
   the scaffold and stops so you can complete `clusters/main/clusterenv.yaml`.
2. Fill in the environment and review `clusters/main/talos/inventory.yaml` and
   the YAML files under `clusters/main/talos/all/`,
   `clusters/main/talos/control-plane/` and `clusters/main/talos/nodes/`. Run
   `clustertool init` again. It creates `talos/secrets.sops.yaml` once, using
   `talosctl gen secrets`. Repeating init preserves that identity.
3. Run `clustertool genconfig` (also available as `clustertool talos genconfig`). It renders the environment into YAML scalar values,
   generates a base with `talosctl gen config --with-secrets`, applies the loose
   documents with `talosctl machineconfig patch`, and runs `talosctl validate
   --mode metal`. Only a fully validated set replaces `talos/generated/<inventory-name>.yaml`
   and `talos/generated/talosconfig`. Client endpoints include every control-plane
   management address; the default client node is the inventory bootstrap node.
4. Review the generated configuration before running `clustertool talos apply`.
   Apply regenerates and validates all nodes first, then freezes the command inputs.
   Existing authenticated etcd membership selects the apply/join path, including
   new workers in maintenance. Only when all control planes report maintenance
   does the new-cluster confirmation appear. Unknown cluster state stops the command.

For inventory generation, files are loaded in filename order from `all/`, the
node's role directory and that node's `nodes/<name>/` directory. The legacy
single-node fallback loads `all/` and `control-plane/` when no inventory exists.
Optional documents are not generally loaded.
When both Docker Hub environment variables are set, `optional/44-registry-auth.yaml`
is included automatically. To enable NVIDIA, move its optional document into
`all/` and create an appropriate Image Factory schematic with the required NVIDIA
extensions; the default schematic does not include those drivers.

`secrets.sops.yaml` is the persistent cluster identity; generated files are disposable.
Run `clustertool encrypt` before committing. Decryption errors stop the workflow.
The generated and staging directories are ignored by Git. Do not run concurrent
generation processes. A remaining `generated.previous` after an interrupted
publication must be inspected and recovered before retrying.

## Image Factory schematic

The comments above `installer.image` in `talos/nodes/control-1/00-install.yaml` document the
Image Factory customization included in the image:

* `net.ifnames=0`
* `siderolabs/util-linux-tools`
* `siderolabs/iscsi-tools`
* `siderolabs/qemu-guest-agent`

Its registered schematic ID is
`4c4acaf75b4a51d6ec95b38dc8b49fb0af5f699e7fbd12fbf246821c649b5312`.
The installer image in `nodes/control-1/00-install.yaml` is
`factory.talos.dev/metal-installer/<schematic-id>:v1.14.0`.
An empty schematic would lose the existing extensions and interface naming.

To change the schematic, copy the commented `customization` block into a temporary
YAML file, remove the comment markers, and edit the desired settings. Register
that YAML with Image Factory and put the returned ID in the installer image.
Update the comments to match. Editing the comments alone has no effect.
For example, after saving the input as `/tmp/talos-schematic.yaml`:

```sh
curl --fail --request POST --data-binary @/tmp/talos-schematic.yaml https://factory.talos.dev/schematics
```

Use boot assets for the same schematic when installing. `clustertool talos upgrade`
reads the installer image from the validated generated configuration and passes it
explicitly to `talosctl upgrade --image`; it therefore retains the extensions.
Kubernetes upgrade reads the kubelet version from that configuration too.
Use supported Talos/Kubernetes upgrade sequences when migrating an existing cluster.

## Multi-node inventory

For a multi-node cluster, extend the copied `talos/inventory.yaml` and add a
patch directory for each node. The inventory is command-targeting metadata; all
Talos settings remain official YAML patches:

```yaml
version: 1
bootstrapNode: control-1
nodes:
  - name: control-1
    role: control-plane
    address: 192.0.2.11
  - name: control-2
    role: control-plane
    address: 192.0.2.12
  - name: worker-1
    role: worker
    address: 192.0.2.21
```

Use `talos/nodes/<name>/` for hostname, network, installer image and disk
patches. Shared patches stay in `talos/all/`; control-plane patches go in
`talos/control-plane/`, and worker-only patches go in `talos/worker/`. Run
`clustertool talos genconfig` to generate and validate one configuration per
node. `talos apply <name-or-ip>` targets one node; `talos apply` or `all` targets
the inventory in control-plane-first order. Bootstrap runs once for the
configured `bootstrapNode`; joining nodes never bootstrap a second cluster.

Each node may use a different Image Factory schematic in its own
`UnattendedInstallConfig.installer.image`. Tuppr does not read these source
files: it discovers the schematic from each running node when it performs an
upgrade.

`version: 1` is the inventory format version, not a Talos or Kubernetes version.
`bootstrapNode` identifies the control plane used to initialize a new cluster;
it does not make that node a permanent leader. `name` selects the patch directory
and output filename. The actual node hostname remains in `HostnameConfig`.

When adding another control plane, copy the starter node directory and change
its hostname, static address and disk/image settings. Its VIP link must match its
own interface. When adding a worker, also remove `Layer2VIPConfig` from the copied
network file. Generation rejects a worker VIP and a static address that does not
match its inventory address. Literal management IPs are supported; additional
`MASTER2IP` or `WORKER1IP` environment variables are not required.

`KubeTalosAPIAccessConfig` and `KubeProxyConfig` belong in the control-plane layer:
Talos 1.14 rejects them on workers. This corrects the earlier proposal's suggestion
to copy the API-access document onto every node. The system-upgrade permissions
remain configured through the control planes; Tuppr discovers each node's live
schematic independently.

### Applying, resuming and upgrading

`talos apply` and `talos apply all` run sequentially, control planes before workers.
Name or IP selection targets one node. All configurations validate before execution;
command inputs are copied into a private temporary snapshot. Failure stops subsequent
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

### Migrating the earlier native single-node layout

Back up the complete existing configuration and credentials securely. Keep
`secrets.sops.yaml` unchanged. Add an inventory entry with the existing management
address and create its `nodes/<name>/` directory. Move the existing install,
hostname and network patches from `all/` into that directory, preserving every
value. Retain shared settings in `all/` and control-plane settings in the role
directory. Run genconfig and compare the old and new complete outputs, particularly
CAs, hostname, IPs, image, disk, mounts, labels and taints, before applying.

`init` refuses an existing Talos layout without an inventory instead of silently
mixing new templates with the earlier layout. An existing Talhelper `talconfig.yaml`
must also be migrated explicitly as described below. No arbitrary custom patches
are translated automatically, and no secrets are regenerated to bypass migration.

See [multinode verification](multinode-validation.md) for offline evidence and the
remaining live-cluster acceptance procedure.

## Preserved settings

Values from the previous main-branch template are represented in these loose files.
Site-specific changes to an existing `talconfig.yaml` or patch still require manual
migration; init does not translate arbitrary customizations.

| Previous configuration | New file under `talos/` |
| --- | --- |
| Install disk at most 1600 GB; no disk wipe | `nodes/control-1/00-install.yaml` (`disk.size <= 1600u * GB`, excluding loop devices, read-only devices and CD-ROMs) |
| Hostname `k8s-control-1` | `nodes/control-1/01-hostname.yaml` |
| Machine certificate SANs `127.0.0.1` and VIP | `all/02-machine.yaml` |
| Cluster pod/service networks | `all/10-cluster.yaml` |
| `eth0`, static address, default gateway, VIP | `nodes/control-1/20-network.yaml` |
| DNS `1.1.1.1`, `8.8.8.8`; all three hostDNS flags | `all/21-resolver.yaml` |
| Server certificate rotation, maxPods 250, shutdown 15s/10s, GC 50/30/30m | `all/30-kubelet.yaml` |
| OpenEBS and Longhorn bind mounts with `bind,rshared,rw` | `all/30-kubelet.yaml` |
| Control-plane scheduling | `control-plane/30-scheduling.yaml` |
| All 14 sysctls, Cloudflare NTP and 13 registry mirrors with original endpoint order | `all/40-system.yaml` |
| `nvme_tcp`, `vfio_pci`, `uio_pci_generic` | `all/41-kernel.yaml` |
| Containerd `discard_unpacked_layers = false` | `all/42-cri.yaml` |
| `/etc/nfsmount.conf`, mode 0644, NFS 4.2/hard/nconnect 16/noatime | `all/43-nfs.yaml` |
| Kubernetes 1.37, API access for system-upgrade, aggregator routing, controller/scheduler bind addresses, scheduler policy, disabled proxy and metrics address, PodSecurity exemptions, no Flannel | `control-plane/10-kubernetes.yaml` |
| etcd metrics at `http://0.0.0.0:2381` | `control-plane/20-etcd.yaml` |
| Docker Hub authentication, when configured | `optional/44-registry-auth.yaml` |
| Optional NVIDIA modules and BPF hardening | `optional/50-nvidia.yaml` |

The API server endpoint is passed to `talosctl gen config`; its additional SANs
are in `control-plane/10-kubernetes.yaml`. The two existing storage mounts require the official legacy
`machine.kubelet` section: Talos 1.14's `KubeletConfig` has no `extraMounts` field.
The generated `KubeletConfig` document is deleted to avoid conflicting configuration.
The machine certificate SANs and etcd metrics also use official legacy fields.
These remain loose YAML patches; no replacement configuration schema is introduced.

## Select the actual installation disk

Inspect `talosctl get disks` on the target node before installation. A maximum
size alone also matches small loop devices; the template explicitly excludes
those and read-only/CD-ROM devices, and preserves the legacy selector's
`disk.transport != ""` condition. It can still match multiple writable disks.
Refine `provisioning.diskSelector.match` to the intended disk's `disk.serial` or
`disk.dev_path`. For example, **only if your disk inventory identifies `/dev/sda`
as the intended installation disk**, use `disk.dev_path == "/dev/sda"`.

Sizes in CEL use multiplication, e.g. `disk.size <= 2u * TB`, not `disk.size <= 2TB`.
The `booting` machine stage does not confirm installation has succeeded. If Talos
reports `bootstrap is not available yet`, inspect `talosctl dmesg` for installation
errors before retrying. Replacing the clustertool binary does not overwrite your
existing loose YAML files; update your own disk selector as well.

## Existing cluster migration: preserve PKI first

Keep a secure backup of the old secrets, talosconfig and a complete generated
control-plane machine configuration before updating the repository. Do not delete
the old identity and let init generate a new one. Init refuses to create new CAs
when legacy configuration or generated credentials are detected.

With the existing configuration decrypted, use the official Talos command to
extract its identity into the new location (replace the input path with your actual
old full control-plane configuration):

```sh
talosctl gen secrets --from-controlplane-config /secure-backup/controlplane.yaml --output-file clusters/main/talos/secrets.sops.yaml
```

Do not use a partial patch or the client `talosconfig` as input. Do not overwrite an
existing native secrets bundle. Preserve the old backup until migration is verified.
This extraction is preferable to manually converting Talhelper-specific encodings.

Migrate all custom settings to the loose files and register your customized
schematic if it differs from the supplied one. Move `talos/talconfig.yaml` out of
the active configuration only after that review; generation refuses to silently
ignore it. Then run init, genconfig and compare the resulting CA certificates,
cluster identity, networks, mounts, extensions and workload settings with the old
configuration. Encrypt the new secrets before committing. Apply only after this
comparison and the applicable Talos upgrade prerequisites are satisfied.

## Verification and references

### Download a Linux test build before merging

Open the **clustertool-linux-amd64-test-build** run in GitHub Actions and download
the `.tar.gz` file from its summary or Artifacts section. The archive contains
`clustertool`, `LICENSE` and `BUILD.txt`; the binary embeds the Linux amd64 Talos
CLI and pre-commit helper. The filename and build information identify the exact
PR commit. No tag or GitHub release is created, and artifacts expire after 14 days.

The workflow runs automatically on PR updates, including draft PRs. Use **Re-run
all jobs** on an existing run to build it again. The separate **Run workflow**
button becomes available once the workflow exists on the default branch; it then
lets you select the branch to build. No merge is needed to download a PR build.

Run `go test ./...`. With `talosctl` 1.14 on PATH, run
`TALOSCTL_INTEGRATION=1 go test ./...` for real generation, patching and validation
tests, including failure preservation and Docker Hub credentials. Building the
release requires downloading the embedded assets with `bash embed/download_talosctl.sh`.

* [Talos configuration document map](https://docs.siderolabs.com/talos/v1.14/reference/configuration/document-map)
* [KubeletConfig fields](https://docs.siderolabs.com/talos/v1.14/reference/configuration/kubernetes/kubeletconfig)
* [Official legacy configuration](https://docs.siderolabs.com/talos/v1.14/reference/configuration/v1alpha1/config)
* [Boot assets and Image Factory](https://docs.siderolabs.com/talos/v1.14/platform-specific-installations/boot-assets)
* [Image Factory API](https://github.com/siderolabs/image-factory/blob/main/docs/api.md)
