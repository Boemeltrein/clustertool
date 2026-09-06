package initfiles

import (
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/trueforge-org/clustertool/pkg/helper"
)

func TestRepeatedLoadTalEnvKeepsEnvironmentBounded(t *testing.T) {
	oldPath, oldEnv := helper.ClusterPath, helper.TalEnv
	originalProcessEnv := map[string]string{}
	for _, entry := range os.Environ() {
		key, value, _ := strings.Cut(entry, "=")
		originalProcessEnv[key] = value
	}
	t.Cleanup(func() {
		for key := range helper.TalEnv {
			if value, ok := originalProcessEnv[key]; ok {
				os.Setenv(key, value)
			} else {
				os.Unsetenv(key)
			}
		}
		helper.ClusterPath, helper.TalEnv = oldPath, oldEnv
	})
	helper.ClusterPath = t.TempDir()
	helper.TalEnv = map[string]string{}
	source := "MASTER1IP: 192.168.1.151/24\nVIP: 192.168.1.150\nGATEWAY: 192.168.1.1\nHEADLAMP_IP: 192.168.1.152\nPODNET: 172.16.0.0/16\nSVCNET: 172.17.0.0/16\n"
	if err := os.WriteFile(filepath.Join(helper.ClusterPath, "clusterenv.yaml"), []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	var first map[string]string
	for i := 0; i < 20; i++ {
		if err := LoadTalEnv(false); err != nil {
			t.Fatal(err)
		}
		if len(helper.TalEnv) != 25 {
			t.Fatalf("load %d generated %d keys; expected 6 source keys, 18 derived keys and CLUSTERNAME", i, len(helper.TalEnv))
		}
		if helper.TalEnv["MASTER1IP_IP_IP"] != "" {
			t.Fatal("recursively derived IP variable")
		}
		if i == 0 {
			first = maps.Clone(helper.TalEnv)
		} else if !reflect.DeepEqual(first, helper.TalEnv) {
			t.Fatalf("load %d changed the environment", i)
		}
	}
	if helper.TalEnv["MASTER1IP_IP"] != "192.168.1.151" || helper.TalEnv["MASTER1IP_CIDR"] != "192.168.1.151/24" {
		t.Fatal("node address normalization changed")
	}
	// A child process must still be launchable with the repeatedly loaded environment.
	if out, err := exec.Command(os.Args[0], "-test.run=^$").CombinedOutput(); err != nil {
		t.Fatalf("start child: %v: %s", err, out)
	}
}
