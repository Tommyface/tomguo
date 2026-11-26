package core

import "testing"

func TestShadowModeSwitch(t *testing.T) {
	const source = "jcydm"
	setShadowMode(source, "builtin")
	if !shadowUseBuiltin(source) {
		t.Fatal("builtin shadow mode not active")
	}
	if moduleTakesOver(source) {
		t.Fatal("module must not take over while shadow mode is builtin")
	}
	setShadowMode(source, "module")
	if shadowUseBuiltin(source) {
		t.Fatal("module shadow mode not restored")
	}
	if shadowUseBuiltin(source) {
		t.Fatal("builtin flag should be cleared")
	}
}

func TestShadowModesOrdered(t *testing.T) {
	ids := shadowModesOrdered()
	if len(ids) == 0 {
		t.Fatal("expected builtin source IDs")
	}
	for i := 1; i < len(ids); i++ {
		if ids[i-1] > ids[i] {
			t.Fatalf("not sorted: %q > %q", ids[i-1], ids[i])
		}
	}
}
