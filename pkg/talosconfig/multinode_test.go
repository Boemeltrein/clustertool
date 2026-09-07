package talosconfig

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/trueforge-org/clustertool/pkg/helper"
)

func multiFixture(t *testing.T) {
	fixture(t)
	write := func(path, content string) {
		t.Helper()
		p := filepath.Join(helper.TalosPath, path)
		if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	install, err := os.ReadFile(filepath.Join(helper.TalosPath, "nodes", "control-1", "00-install.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range []string{"00-install.yaml", "01-hostname.yaml", "20-network.yaml"} {
		_ = os.Remove(filepath.Join(helper.TalosPath, "all", file))
	}
	write("inventory.yaml", "version: 1\nbootstrapNode: control-1\nnodes:\n  - {name: control-1, role: control-plane, address: 192.0.2.11}\n  - {name: control-2, role: control-plane, address: 192.0.2.12}\n  - {name: worker-1, role: worker, address: 192.0.2.21}\n")
	for index, name := range []string{"control-1", "control-2", "worker-1"} {
		// Synthetic schematic hashes are used only for offline generation, never for image pulls.
		id := strings.Repeat(fmt.Sprint(index+1), 64)
		image := strings.Replace(string(install), "4c4acaf75b4a51d6ec95b38dc8b49fb0af5f699e7fbd12fbf246821c649b5312", id, 1)
		start := strings.Index(image, "    match:")
		end := strings.Index(image[start:], "\n") + start
		image = image[:start] + fmt.Sprintf("    match: disk.dev_path == '/dev/sd%c'", 'a'+index) + image[end:]
		write("nodes/"+name+"/00-install.yaml", image)
		write("nodes/"+name+"/10-hostname.yaml", "apiVersion: v1alpha1\nkind: HostnameConfig\nauto: off\nhostname: "+name+"\n")
		write("nodes/"+name+"/90-label.yaml", "apiVersion: v1alpha1\nkind: KubeNodeConfig\nlabels:\n  clustertool.test/node: "+name+"\n")
	}
	write("all/99-label.yaml", "apiVersion: v1alpha1\nkind: KubeNodeConfig\nlabels:\n  clustertool.test/layer: shared\n")
	write("control-plane/00-label.yaml", "apiVersion: v1alpha1\nkind: KubeNodeConfig\nlabels:\n  clustertool.test/layer: role\n")
}

func TestInventorySelection(t *testing.T) {
	multiFixture(t)
	inv, err := LoadInventory()
	if err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{"", "all"} {
		nodes, err := inv.Select(target)
		if err != nil || len(nodes) != 3 {
			t.Fatal(nodes, err)
		}
	}
	for _, target := range []string{"worker-1", "192.0.2.21"} {
		nodes, err := inv.Select(target)
		if err != nil || len(nodes) != 1 || nodes[0].Role != "worker" {
			t.Fatal(nodes, err)
		}
	}
	if _, err := inv.Select("unknown"); err == nil {
		t.Fatal("unknown node accepted")
	}
	data, _ := os.ReadFile(filepath.Join(helper.TalosPath, "inventory.yaml"))
	for _, bad := range []string{strings.Replace(string(data), "192.0.2.12", "192.0.2.11", 1), strings.Replace(string(data), "bootstrapNode: control-1", "bootstrapNode: worker-1", 1), strings.Replace(string(data), "name: worker-1", "name: ../worker", 1)} {
		os.WriteFile(filepath.Join(helper.TalosPath, "inventory.yaml"), []byte(bad), 0600)
		if _, err := LoadInventory(); err == nil {
			t.Fatal("invalid inventory accepted")
		}
	}
}

func TestMultiNodeGenerationIntegration(t *testing.T) {
	if os.Getenv("TALOSCTL_INTEGRATION") != "1" {
		t.Skip("requires real talosctl")
	}
	multiFixture(t)
	if err := EnsureSecrets(); err != nil {
		t.Fatal(err)
	}
	secrets, _ := os.ReadFile(SecretsPath())
	if err := Generate(); err != nil {
		t.Fatal(err)
	}
	previous := map[string][]byte{}
	for index, name := range []string{"control-1", "control-2", "worker-1"} {
		path := filepath.Join(helper.TalosGenerated, name+".yaml")
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		previous[name] = data
		image, err := GeneratedNodeValue(path, "UnattendedInstallConfig", "installer", "image")
		if err != nil || !strings.Contains(image, strings.Repeat(fmt.Sprint(index+1), 64)) {
			t.Fatal(name, image, err)
		}
		role, err := GeneratedNodeValue(path, "", "machine", "type")
		expected := "controlplane"
		if index == 2 {
			expected = "worker"
		}
		if err != nil || role != expected {
			t.Fatal(name, role, err)
		}
		value, err := GeneratedNodeValue(path, "KubeNodeConfig", "labels", "clustertool.test/layer")
		expected = "role"
		if index == 2 {
			expected = "shared"
		}
		if err != nil || value != expected {
			t.Fatal(name, value, err)
		}
	}
	os.WriteFile(filepath.Join(helper.TalosPath, "nodes", "worker-1", "99-invalid.yaml"), []byte("apiVersion: v1alpha1\nkind: UnattendedInstallConfig\nprovisioning:\n  diskSelector:\n    match: unknown.field > 0\n"), 0600)
	if err := Generate(); err == nil {
		t.Fatal("invalid worker accepted")
	}
	for name, old := range previous {
		now, _ := os.ReadFile(filepath.Join(helper.TalosGenerated, name+".yaml"))
		if !bytes.Equal(old, now) {
			t.Fatal("partial publication", name)
		}
	}
	now, _ := os.ReadFile(SecretsPath())
	if !bytes.Equal(secrets, now) {
		t.Fatal("cluster identity changed")
	}
}
