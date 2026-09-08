package gencmd

import (
	"strings"
	"testing"

	"github.com/trueforge-org/clustertool/pkg/helper"
	"github.com/trueforge-org/clustertool/pkg/talosconfig"
)

func withSingleNodeFixture(t *testing.T) {
	t.Helper()
	previous := helper.TalEnv
	helper.TalEnv = map[string]string{"MASTER1IP_IP": "10.0.0.1"}
	t.Cleanup(func() { helper.TalEnv = previous })
}

func TestGenPlainUsesSingleConfiguredNode(t *testing.T) {
	withSingleNodeFixture(t)
	cmds := GenPlain("health", "", []string{"--wait-timeout=1m"})
	if len(cmds) != 1 {
		t.Fatalf("expected 1 command, got %d", len(cmds))
	}
	if !strings.Contains(cmds[0].String(), " -n 10.0.0.1") || !strings.Contains(cmds[0].String(), "--talosconfig "+talosconfig.TalosconfigPath()) {
		t.Fatalf("unexpected command %q", cmds[0])
	}
	if !strings.HasSuffix(cmds[0].String(), " --wait-timeout=1m") {
		t.Fatalf("expected extra flag, got %q", cmds[0])
	}
}

func TestGenApplySingleNode(t *testing.T) {
	withSingleNodeFixture(t)
	cmds := GenApply("", []string{"--timeout=1m"})
	if len(cmds) != 1 {
		t.Fatalf("expected 1 command, got %d", len(cmds))
	}
	if !strings.Contains(cmds[0].String(), " apply-config ") || !strings.Contains(cmds[0].String(), " -f "+talosconfig.ControlPlanePath()) {
		t.Fatalf("unexpected command %q", cmds[0])
	}
	if !strings.Contains(cmds[0].String(), " -n 10.0.0.1") || !strings.HasSuffix(cmds[0].String(), " --timeout=1m") {
		t.Fatalf("unexpected node or flags in %q", cmds[0])
	}
}
