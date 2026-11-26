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
	// 内置源已清零，影子清单允许为空；非空时必须有序。
	ids := shadowModesOrdered()
	for i := 1; i < len(ids); i++ {
		if ids[i-1] > ids[i] {
			t.Fatalf("not sorted: %q > %q", ids[i-1], ids[i])
		}
	}
}
