package core

import (
	"os"
	"path/filepath"
	"testing"
)

func TestModuleActiveDir(t *testing.T) {
	root := t.TempDir()
	if got := moduleActiveDir(root, "demo"); got != filepath.Join(moduleRoot(root), "demo") {
		t.Fatalf("无版本目录应回退扁平结构，得到 %s", got)
	}
	versions := filepath.Join(moduleVersionRoot(root, "demo"), "1.0.0")
	if err := os.MkdirAll(versions, 0o755); err != nil {
		t.Fatal(err)
	}
	if got := moduleActiveDir(root, "demo"); got != versions {
		t.Fatalf("单版本应命中，得到 %s", got)
	}
	newer := filepath.Join(moduleVersionRoot(root, "demo"), "1.0.1")
	if err := os.MkdirAll(newer, 0o755); err != nil {
		t.Fatal(err)
	}
	if got := moduleActiveDir(root, "demo"); got != newer {
		t.Fatalf("应取字典序最大版本，得到 %s", got)
	}
}
