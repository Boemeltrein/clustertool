package initfiles

import (
	"os"
	"testing"

	"github.com/trueforge-org/clustertool/pkg/helper"
)

func TestFailedInitKeepsRunAgain(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := createRunAgainFile(); err != nil {
		t.Fatal(err)
	}
	old := helper.RootCache
	t.Cleanup(func() { helper.RootCache = old })
	helper.RootCache = "missing-template-cache"
	if err := InitFiles(); err == nil {
		t.Fatal("expected init to fail")
	}
	if _, err := os.Stat("RUNAGAIN"); err != nil {
		t.Fatalf("retry marker removed: %v", err)
	}
}
