# Native Talos 1.14

Clustertool uses `talosctl` directly. Talhelper and its configuration schema are
removed. This workflow manages the existing single control-plane node defined by
`MASTER1IP`; it does not generate worker or additional control-plane configurations.

## Init, genconfig and apply

1. Run `clustertool init` in your cluster repository. On a new repository it creates
   the scaffold and stops so you can complete `clusters/main/clusterenv.yaml`.
2. Fill in the environment and review the YAML files under
   `clusters/main/talos/all/` and `clusters/main/talos/control-plane/`. Run
   `clustertool init` again. It creates `talos/secrets.sops.yaml` once, using
   `talosctl gen secrets`. Repeating init preserves that identity.
3. Run `clustertool genconfig`. It renders the environment into YAML scalar values,
   generates a base with `talosctl gen config --with-secrets`, applies the loose
   documents with `talosctl machineconfig patch`, and runs `talosctl validate
   --mode metal`. Only validated output replaces `talos/generated/controlplane.yaml`
   and `talos/generated/talosconfig`. The client endpoints and nodes use `MASTER1IP`.
4. Review the generated configuration before running `clustertool talos apply`.
   Apply regenerates and validates first. For a node in maintenance mode, the
   existing bootstrap prompt starts installation, etcd bootstrap and the existing
   Kubernetes/Helm/Flux setup. For an installed node it applies the configuration.

Both document directories are required. Files are loaded in filename order from
`all/`, then `control-plane/`. Optional documents are not generally loaded.
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

`talos/schematic.yaml` contains the Image Factory input, not a machine-config patch:

* `net.ifnames=0`
* `siderolabs/util-linux-tools`
* `siderolabs/iscsi-tools`
* `siderolabs/qemu-guest-agent`

Its registered schematic ID is
`82866c01b2842b490c27a6f1a4996aae05f096c83db40ecda166b03da9deae46`.
The installer image in `all/00-install.yaml` is
`factory.talos.dev/metal-installer/<schematic-id>:v1.14.0`.
An empty schematic would lose the existing extensions and interface naming.

If you change the schematic, register the changed YAML with Image Factory and put
the returned ID in the installer image. Editing `schematic.yaml` alone has no
effect. For example, from the repository root:

```sh
curl --fail --request POST --data-binary @clusters/main/talos/schematic.yaml https://factory.talos.dev/schematics
```

Use boot assets for the same schematic when installing. `clustertool talos upgrade`
reads the installer image from the validated generated configuration and passes it
explicitly to `talosctl upgrade --image`; it therefore retains the extensions.
Kubernetes upgrade reads the kubelet version from that configuration too.
Use supported Talos/Kubernetes upgrade sequences when migrating an existing cluster.

## Preserved settings

Values from the previous main-branch template are represented in these loose files.
Site-specific changes to an existing `talconfig.yaml` or patch still require manual
migration; init does not translate arbitrary customizations.

| Previous configuration | New file under `talos/` |
| --- | --- |
| Install disk at most 1600 GB; no disk wipe | `all/00-install.yaml` (CEL uses bytes: `disk.size <= 1600000000000u`) |
| Hostname `k8s-control-1` | `all/01-hostname.yaml` |
| Machine certificate SANs `127.0.0.1` and VIP | `all/02-machine.yaml` |
| Cluster pod/service networks | `all/10-cluster.yaml` |
| `eth0`, static address, default gateway, VIP | `all/20-network.yaml` |
| DNS `1.1.1.1`, `8.8.8.8`; all three hostDNS flags | `all/21-resolver.yaml` |
| Server certificate rotation, maxPods 250, shutdown 15s/10s, GC 50/30/30m | `all/30-kubelet.yaml` |
| OpenEBS and Longhorn bind mounts with `bind,rshared,rw`; control-plane scheduling | `all/30-kubelet.yaml` |
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

Run `go test ./...`. With `talosctl` 1.14 on PATH, run
`TALOSCTL_INTEGRATION=1 go test ./...` for real generation, patching and validation
tests, including failure preservation and Docker Hub credentials. Building the
release requires downloading the embedded assets with `bash embed/download_talosctl.sh`.

* [Talos configuration document map](https://docs.siderolabs.com/talos/v1.14/reference/configuration/document-map)
* [KubeletConfig fields](https://docs.siderolabs.com/talos/v1.14/reference/configuration/kubernetes/kubeletconfig)
* [Official legacy configuration](https://docs.siderolabs.com/talos/v1.14/reference/configuration/v1alpha1/config)
* [Boot assets and Image Factory](https://docs.siderolabs.com/talos/v1.14/platform-specific-installations/boot-assets)
* [Image Factory API](https://github.com/siderolabs/image-factory/blob/main/docs/api.md)
