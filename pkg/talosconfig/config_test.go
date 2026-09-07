package talosconfig

import (
	"bytes"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/trueforge-org/clustertool/embed"
	"github.com/trueforge-org/clustertool/pkg/helper"
	"gopkg.in/yaml.v3"
)

func fixture(t *testing.T) {
	t.Helper()
	oldPath, oldGenerated, oldEnv, oldName := helper.TalosPath, helper.TalosGenerated, helper.TalEnv, helper.ClusterName
	helper.TalosPath = filepath.Join(t.TempDir(), "talos")
	helper.TalosGenerated = filepath.Join(helper.TalosPath, "generated")
	helper.ClusterName = "main"
	helper.TalEnv = map[string]string{"CLUSTERNAME": "main", "MASTER1IP_IP": "192.168.20.210", "MASTER1IP_CIDR": "192.168.20.210/24", "VIP_IP": "192.168.20.200", "GATEWAY": "192.168.20.1", "PODNET": "172.16.0.0/16", "SVCNET": "172.17.0.0/16"}
	t.Cleanup(func() {
		helper.TalosPath = oldPath
		helper.TalosGenerated = oldGenerated
		helper.TalEnv = oldEnv
		helper.ClusterName = oldName
	})
	sub, err := fs.Sub(embed.GenericFiles, "generic/base/talos")
	if err != nil {
		t.Fatal(err)
	}
	if err := fs.WalkDir(sub, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		dest := filepath.Join(helper.TalosPath, path)
		if d.IsDir() {
			return os.MkdirAll(dest, 0700)
		}
		data, err := fs.ReadFile(sub, path)
		if err != nil {
			return err
		}
		return os.WriteFile(dest, data, 0600)
	}); err != nil {
		t.Fatal(err)
	}
	// The embedded scaffold is inventory-first. Keep this fixture focused on
	// the legacy fallback tests by removing the copied inventory template.
	if err := os.Remove(filepath.Join(helper.TalosPath, "inventory.yaml")); err != nil {
		t.Fatal(err)
	}
	// Legacy generation still expects the node-specific documents in all/.
	for _, name := range []string{"00-install.yaml", "01-hostname.yaml", "20-network.yaml"} {
		source := filepath.Join(helper.TalosPath, "nodes", "control-1", name)
		destination := filepath.Join(helper.TalosPath, "all", name)
		data, err := os.ReadFile(source)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(destination, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestExistingSecretsAndLegacyGuard(t *testing.T) {
	fixture(t)
	for _, path := range []string{filepath.Join(helper.TalosPath, "talconfig.yaml"), filepath.Join(helper.TalosGenerated, "talsecret.yaml"), TalosconfigPath()} {
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("existing identity"), 0600); err != nil {
			t.Fatal(err)
		}
		if err := EnsureSecrets(); err == nil {
			t.Fatal("generated a new identity for a legacy cluster")
		}
		if _, err := os.Stat(SecretsPath()); !os.IsNotExist(err) {
			t.Fatal("created new secrets")
		}
		os.Remove(path)
	}
	original := []byte("opaque encrypted secrets must remain unchanged")
	os.WriteFile(SecretsPath(), original, 0600)
	if err := EnsureSecrets(); err != nil {
		t.Fatal(err)
	}
	actual, _ := os.ReadFile(SecretsPath())
	if !bytes.Equal(original, actual) {
		t.Fatal("existing identity overwritten")
	}
}

func TestYAMLSubstitution(t *testing.T) {
	secret := "quote\": value\npassword: surprise"
	data, err := render([]byte("password: \"${PASSWORD}\"\n"), map[string]string{"PASSWORD": secret})
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]string
	if err := yaml.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	if len(doc) != 1 || doc["password"] != secret {
		t.Fatal("credential changed YAML structure")
	}
	if _, err := render([]byte("value: ${MISSING}\n"), nil); err == nil {
		t.Fatal("unresolved variable accepted")
	}
}

func TestNativeGenerationIntegration(t *testing.T) {
	if os.Getenv("TALOSCTL_INTEGRATION") != "1" {
		t.Skip("set TALOSCTL_INTEGRATION=1 with talosctl 1.14 on PATH")
	}
	fixture(t)
	if err := EnsureSecrets(); err != nil {
		t.Fatal(err)
	}
	secrets, _ := os.ReadFile(SecretsPath())
	if err := Generate(); err != nil {
		t.Fatal(err)
	}
	config, _ := os.ReadFile(ControlPlanePath())
	tc, _ := os.ReadFile(TalosconfigPath())
	docs := readDocuments(t, config)
	legacy := docs["/"]
	at := func(v any, keys ...string) any {
		for _, key := range keys {
			m, ok := v.(map[string]any)
			if !ok {
				return nil
			}
			v = m[key]
		}
		return v
	}
	require := func(got, want any) {
		t.Helper()
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("got %#v, want %#v", got, want)
		}
	}
	require(at(legacy, "machine", "kubelet", "extraConfig", "imageGCHighThresholdPercent"), 50)
	require(at(legacy, "machine", "kubelet", "extraConfig", "imageGCLowThresholdPercent"), 30)
	require(at(legacy, "machine", "kubelet", "extraConfig", "imageMinimumGCAge"), "30m")
	require(at(legacy, "machine", "certSANs"), []any{"127.0.0.1", "192.168.20.200"})
	require(at(legacy, "machine", "kubelet", "extraConfig", "maxPods"), 250)
	require(at(legacy, "machine", "kubelet", "extraConfig", "shutdownGracePeriod"), "15s")
	require(at(legacy, "machine", "kubelet", "extraConfig", "shutdownGracePeriodCriticalPods"), "10s")
	require(at(legacy, "machine", "kubelet", "extraArgs", "rotate-server-certificates"), "true")
	require(at(docs["TimeSyncConfig/"], "ntp", "servers"), []any{"time.cloudflare.com"})
	require(at(docs["ResolverConfig/"], "nameservers"), []any{map[string]any{"address": "1.1.1.1"}, map[string]any{"address": "8.8.8.8"}})
	require(at(docs["KubeProxyConfig/"], "config", "metricsBindAddress"), "0.0.0.0:10249")
	if at(docs["KubeNodeConfig/"], "taints", "node-role.kubernetes.io/control-plane") != nil {
		t.Fatal("control-plane scheduling is disabled")
	}
	mounts := at(legacy, "machine", "kubelet", "extraMounts").([]any)
	require(len(mounts), 2)
	for i, path := range []string{"/var/openebs/local", "/var/lib/longhorn"} {
		require(at(mounts[i], "source"), path)
		require(at(mounts[i], "destination"), path)
		require(at(mounts[i], "options"), []any{"bind", "rshared", "rw"})
	}
	require(at(legacy, "cluster", "etcd", "extraArgs", "listen-metrics-urls"), "http://0.0.0.0:2381")
	require(at(docs["HostnameConfig/"], "hostname"), "k8s-control-1")
	for _, key := range []string{"enabled", "forwardKubeDNSToHost", "resolveMemberNames"} {
		require(at(docs["ResolverConfig/"], "hostDNS", key), true)
	}
	for _, name := range []string{"nvme_tcp", "vfio_pci", "uio_pci_generic"} {
		if docs["KernelModuleConfig/"+name] == nil {
			t.Fatal("missing module " + name)
		}
	}
	for _, name := range []string{"cgr.dev", "docker.io", "factory.talos.dev", "gcr.io", "ghcr.io", "k8s.gcr.io", "mcr.microsoft.com", "public.ecr.aws", "quay.io", "registry-1.docker.io", "registry.k8s.io", "tccr.io", "oci.trueforge.org"} {
		if docs["RegistryMirrorConfig/"+name] == nil {
			t.Fatal("missing mirror " + name)
		}
	}
	require(at(docs["KubeProxyConfig/"], "enabled"), false)
	if docs["KubeFlannelCNIConfig/"] != nil || docs["KubeletConfig/"] != nil {
		t.Fatal("conflicting generated document remains")
	}
	if !bytes.Contains(tc, []byte("192.168.20.210")) {
		t.Fatal("management endpoint not configured")
	}
	if !strings.Contains(at(docs["UnattendedInstallConfig/"], "installer", "image").(string), "4c4acaf75b4a51d6ec95b38dc8b49fb0af5f699e7fbd12fbf246821c649b5312:v1.14.0") {
		t.Fatal("wrong schematic")
	}
	if err := EnsureSecrets(); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(SecretsPath())
	if !bytes.Equal(secrets, after) {
		t.Fatal("PKI changed")
	}
	// A valid document with invalid Talos semantics must not publish either file.
	bad := filepath.Join(helper.TalosPath, "all", "99-invalid.yaml")
	os.WriteFile(bad, []byte("apiVersion: v1alpha1\nkind: UnattendedInstallConfig\nprovisioning:\n  diskSelector:\n    match: nonexistent.field > 0\n"), 0600)
	if err := Generate(); err == nil {
		t.Fatal("invalid Talos document accepted")
	}
	after, _ = os.ReadFile(ControlPlanePath())
	if !bytes.Equal(config, after) {
		t.Fatal("failed generation replaced valid machine config")
	}
	after, _ = os.ReadFile(TalosconfigPath())
	if !bytes.Equal(tc, after) {
		t.Fatal("failed generation replaced valid client config")
	}
	os.Remove(bad)
	helper.TalEnv["DOCKERHUB_USER"] = "test-user"
	helper.TalEnv["DOCKERHUB_PASSWORD"] = "a:\"b#c\\d"
	if err := Generate(); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(ControlPlanePath())
	docs = readDocuments(t, data)
	for _, name := range []string{"docker.io", "registry-1.docker.io"} {
		require(at(docs["RegistryAuthConfig/"+name], "password"), helper.TalEnv["DOCKERHUB_PASSWORD"])
	}
}

func TestLegacyIdentityExtractionIntegration(t *testing.T) {
	if os.Getenv("TALOSCTL_INTEGRATION") != "1" {
		t.Skip("requires talosctl 1.14")
	}
	fixture(t)
	if err := EnsureSecrets(); err != nil {
		t.Fatal(err)
	}
	oldDir := t.TempDir()
	if err := runTalosctl("gen", "config", "main", "https://192.168.20.200:6443", "--talos-version", "v1.13.10", "--with-secrets", SecretsPath(), "--output", filepath.Join(oldDir, "controlplane.yaml"), "--output-types", "controlplane"); err != nil {
		t.Fatal(err)
	}
	extracted := filepath.Join(t.TempDir(), "secrets.yaml")
	if err := runTalosctl("gen", "secrets", "--from-controlplane-config", filepath.Join(oldDir, "controlplane.yaml"), "--output-file", extracted); err != nil {
		t.Fatal(err)
	}
	oldData, err := os.ReadFile(SecretsPath())
	if err != nil {
		t.Fatal(err)
	}
	newData, err := os.ReadFile(extracted)
	if err != nil {
		t.Fatal(err)
	}
	var original, migrated map[string]any
	if err := yaml.Unmarshal(oldData, &original); err != nil {
		t.Fatal(err)
	}
	if err := yaml.Unmarshal(newData, &migrated); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"cluster", "secrets", "trustdinfo", "certs"} {
		if original[field] == nil || !reflect.DeepEqual(original[field], migrated[field]) {
			t.Fatalf("identity field %s changed during migration", field)
		}
	}
}

func readDocuments(t *testing.T, data []byte) map[string]map[string]any {
	t.Helper()
	docs := map[string]map[string]any{}
	dec := yaml.NewDecoder(bytes.NewReader(data))
	for {
		var doc map[string]any
		err := dec.Decode(&doc)
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		kind, _ := doc["kind"].(string)
		name, _ := doc["name"].(string)
		docs[kind+"/"+name] = doc
	}
	return docs
}
