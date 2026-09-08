package gencmd

import (
	"github.com/trueforge-org/clustertool/pkg/helper"
	"os"
	"path/filepath"
	"testing"
)

func TestSnapshotRetainsAllNodeInputsAndCleansUp(t *testing.T) {
	previous := helper.TalosPath
	helper.TalosPath = t.TempDir()
	t.Cleanup(func() { helper.TalosPath = previous })
	config := filepath.Join(helper.TalosPath, "worker with spaces.yaml")
	client := filepath.Join(helper.TalosPath, "talosconfig")
	for _, p := range []string{config, client} {
		if err := os.WriteFile(p, []byte("original"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	planned := []Command{{Args: []string{"talosctl", "apply-config", "--talosconfig", client, "-f", config}, Snapshot: true}}
	frozen, cleanup, err := freezeCommands(planned)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	if err := os.WriteFile(config, []byte("changed"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, index := range []int{3, 5} {
		data, err := os.ReadFile(frozen[0].Args[index])
		if err != nil || string(data) != "original" {
			t.Fatal(string(data), err)
		}
	}
	if planned[0].Args[5] != config {
		t.Fatal("mutated plan")
	}
	cleanup()
	if _, err := os.Stat(frozen[0].Args[5]); !os.IsNotExist(err) {
		t.Fatal("snapshot retained")
	}
}

func TestBootstrapCheckpointBindsIdentity(t *testing.T) {
	withSingleNodeFixture(t)
	oldPath := helper.TalosPath
	helper.TalosPath = t.TempDir()
	t.Cleanup(func() { helper.TalosPath = oldPath })
	secrets := filepath.Join(helper.TalosPath, "secrets.sops.yaml")
	if err := os.WriteFile(secrets, []byte("identity-one"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := beginBootstrap(); err != nil {
		t.Fatal(err)
	}
	if pending, err := BootstrapPending(); err != nil || !pending {
		t.Fatal(pending, err)
	}
	if err := os.WriteFile(secrets, []byte("identity-two"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := BootstrapPending(); err == nil {
		t.Fatal("resumed with different identity")
	}
}
