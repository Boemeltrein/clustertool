package helper

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestHookPropagatesWriteAndCloseErrors(t *testing.T) {
	old := hookWriteStringFn
	t.Cleanup(func() { hookWriteStringFn = old })
	for _, failWrite := range []bool{false, true} {
		hookWriteStringFn = func(file *os.File, content string) (int, error) {
			if err := file.Close(); err != nil {
				t.Fatal(err)
			}
			if failWrite {
				return 0, os.ErrPermission
			}
			return len(content), nil
		}
		err := writeHookScript(filepath.Join(t.TempDir(), "pre-commit"), "hook")
		if !errors.Is(err, os.ErrClosed) || (failWrite && !errors.Is(err, os.ErrPermission)) {
			t.Fatalf("expected write/close errors, got %v", err)
		}
	}
}
