package cmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitConfirmationBeforeDecryption(t *testing.T) {
	for _, tc := range []struct {
		name, input                string
		marker, secrets, cancelled bool
	}{
		{"decline", "n\n", false, true, true},
		{"default", "\n", false, true, true},
		{"closed input", "", false, true, true},
		{"accept", "y\n", false, true, false},
		{"retry", "", true, true, false},
		{"first init", "", false, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.MkdirAll(filepath.Join(dir, "clusters", "main", "talos"), 0755); err != nil {
				t.Fatal(err)
			}
			if tc.secrets {
				if err := os.WriteFile(filepath.Join(dir, "clusters", "main", "talos", "secrets.sops.yaml"), []byte("unchanged"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if tc.marker {
				if err := os.WriteFile(filepath.Join(dir, "RUNAGAIN"), nil, 0600); err != nil {
					t.Fatal(err)
				}
			}
			// Invalid SOPS config reveals whether execution reached decryption.
			if err := os.WriteFile(filepath.Join(dir, ".sops.yaml"), []byte("creation_rules: [\n"), 0600); err != nil {
				t.Fatal(err)
			}
			process := exec.Command(os.Args[0], "-test.run=^TestCommandErrorProcess$")
			process.Dir = dir
			process.Env = append(os.Environ(), `CLUSTERTOOL_TEST_ARGS=["init"]`)
			process.Stdin = strings.NewReader(tc.input)
			out, err := process.CombinedOutput()
			if err == nil {
				t.Fatalf("expected cancellation or SOPS error: %s", out)
			}
			if strings.Contains(string(out), "init cancelled") != tc.cancelled {
				t.Fatalf("unexpected cancellation: %s", out)
			}
			if strings.Contains(string(out), "parse .sops.yaml") == tc.cancelled {
				t.Fatalf("unexpected decryption attempt: %s", out)
			}
			if strings.Contains(string(out), "WARNING:") != (tc.secrets && !tc.marker) {
				t.Fatalf("unexpected warning: %s", out)
			}
		})
	}
}
