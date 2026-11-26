package core

import (
	"context"
	"testing"
)

func TestBuiltinCatalogTableCoversAllSourceSpecs(t *testing.T) {
	// 每个内置源都必须在目录分发表里有实现（含上面那几个“只有一页”的源）。
	for _, spec := range builtinSourceSpecs {
		if !builtinCatalogKnown(spec.ID) {
			t.Errorf("内置源 %s 没有目录分发实现", spec.ID)
		}
	}
	if got := len(builtinCatalogTable); got < len(builtinSourceSpecs) {
		t.Fatalf("分发表条目 %d 少于内置源数量 %d", got, len(builtinSourceSpecs))
	}
}

func TestBuiltinCatalogKnownRejectsUnknown(t *testing.T) {
	for _, unknown := range []string{"", "not-a-source", "unknown.example.com"} {
		if builtinCatalogKnown(unknown) {
			t.Errorf("%q 不应有内置目录实现", unknown)
		}
	}
}

func TestBuiltinCatalogSinglePageSources(t *testing.T) {
	// 主程序不内置站源，分发表应为空：任何内置源 id 都不应命中。
	for _, id := range []string{sourceHuangguoMain, sourceHuangguoSelected, sourceHuangguoWelfare} {
		if builtinCatalogKnown(id) {
			t.Fatalf("%s 不应再有内置目录实现", id)
		}
	}
	_ = context.Background
}

func TestCatalogDispatchHasNoHardcodedSwitch(t *testing.T) {
	// 回归锁：nativeCatalog 里不应该再出现 `switch source` 或
	// `if query != "" && source ==` 这类站点分支。
	src := readCoreSourceForTest(t, "app_runtime.go")
	start := indexOf(src, "func (engine *nativeEngine) nativeCatalog(")
	if start < 0 {
		t.Fatal("找不到 nativeCatalog 函数")
	}
	end := indexOf(src[start:], "\nfunc (engine *nativeEngine) nativeDetail(")
	if end < 0 {
		t.Fatal("找不到 nativeDetail 函数")
	}
	src = src[start : start+end]
	for _, forbidden := range []string{`switch source {`, `if query != "" && source ==`} {
		if contains(src, forbidden) {
			t.Errorf("app_runtime.go 又出现了硬编码分支：%s", forbidden)
		}
	}
}

func readCoreSourceForTest(t *testing.T, name string) string {
	t.Helper()
	data, err := readFileForTest(name)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
