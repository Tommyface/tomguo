package core

import "testing"

func TestSourceRegistryResolvesIDs(t *testing.T) {
	for _, spec := range builtinSourceSpecs {
		got, ok := lookupSourceSpec(spec.ID)
		if !ok {
			t.Fatalf("内置源 %s 无法按 id 查到", spec.ID)
		}
		if got.ID != spec.ID {
			t.Fatalf("内置源 %s 解析成了 %s", spec.ID, got.ID)
		}
	}
}

func TestSourceRegistryResolvesAliases(t *testing.T) {
	for _, spec := range builtinSourceSpecs {
		for _, alias := range spec.Aliases {
			got, ok := lookupSourceSpec(alias)
			if !ok {
				t.Fatalf("别名 %q 无法解析（应指向 %s）", alias, spec.ID)
			}
			if got.ID != spec.ID {
				t.Fatalf("别名 %q 解析成 %s，期望 %s", alias, got.ID, spec.ID)
			}
			// 大小写与空白不应影响解析。
			upper, ok := lookupSourceSpec("  " + alias + "  ")
			if !ok || upper.ID != spec.ID {
				t.Fatalf("别名 %q 带空白后解析失败", alias)
			}
			canonical := canonicalProviderSource(alias)
			if canonical != spec.ID {
				t.Fatalf("canonicalProviderSource(%q) = %s，期望 %s", alias, canonical, spec.ID)
			}
		}
	}
}

func TestSourceRegistryRejectsUnknown(t *testing.T) {
	for _, unknown := range []string{"", "   ", "not-a-source", "unknown.example.com"} {
		if _, ok := lookupSourceSpec(unknown); ok {
			t.Fatalf("%q 不应该被识别为内置源", unknown)
		}
		if isHuangguoProviderSource(unknown) {
			t.Fatalf("%q 不应该被识别为可用站源", unknown)
		}
	}
}

func TestSourceRegistryCoversAllBuiltinSources(t *testing.T) {
	// 内置源数量是重构基线：S1 期间应恒为 26，S7 迁移完成后这张表会被清空。
	if got := len(builtinSourceSpecs); got != 26 {
		t.Fatalf("内置源数量为 %d，期望 26", got)
	}
	seen := make(map[string]bool, len(builtinSourceSpecs))
	for _, id := range builtinSourceIDs() {
		if seen[id] {
			t.Fatalf("内置源 id 重复：%s", id)
		}
		seen[id] = true
		if !isHuangguoProviderSource(id) {
			t.Fatalf("内置源 %s 未被 isHuangguoProviderSource 认可", id)
		}
		if moduleSource(id) != nil {
			continue
		}
		if !supportsBuiltinCapability(id, "catalog") {
			t.Fatalf("内置源 %s 缺少 catalog 能力声明", id)
		}
	}
}

func TestSourceRegistryKeepsCanonicalBehaviour(t *testing.T) {
	cases := map[string]string{
		"huangguo":          sourceHuangguoAI,
		"HUANGGUOAI":        sourceHuangguoAI,
		" huangguo.video ":  sourceHuangguoVideo,
		"hongguoduanju.com": sourceHongguo,
		"51短剧":              sourceDuanju51,
		"囧次元":               sourceJcydm,
		"e站":                sourceEZDMW,
		"河马":                sourceHema,
	}
	for input, want := range cases {
		if got := canonicalProviderSource(input); got != want {
			t.Fatalf("canonicalProviderSource(%q) = %s，期望 %s", input, got, want)
		}
	}
}
