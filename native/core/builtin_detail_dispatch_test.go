package core

import "testing"

func TestBuiltinDetailTableCoversAllSourceSpecs(t *testing.T) {
	for _, spec := range builtinSourceSpecs {
		if !builtinDetailKnown(spec.ID) {
			t.Errorf("内置源 %s 没有详情分发实现", spec.ID)
		}
	}
	if got := len(builtinDetailTable); got != len(builtinSourceSpecs) {
		t.Fatalf("详情表条目 %d 与内置源数量 %d 不一致", got, len(builtinSourceSpecs))
	}
}

func TestBuiltinDetailKnownRejectsUnknown(t *testing.T) {
	for _, unknown := range []string{"", "not-a-source", "unknown.example.com"} {
		if builtinDetailKnown(unknown) {
			t.Errorf("%q 不应有内置详情实现", unknown)
		}
	}
}

func TestBuiltinTablesCoverSameSources(t *testing.T) {
	// 两张表必须覆盖完全相同的源集合：只加目录不加详情（或反之）会让某个源
	// 在首页可见但点进去报错。
	for source := range builtinCatalogTable {
		if _, ok := builtinDetailTable[source]; !ok {
			t.Errorf("源 %s 有目录实现但没有详情实现", source)
		}
	}
	for source := range builtinDetailTable {
		if _, ok := builtinCatalogTable[source]; !ok {
			t.Errorf("源 %s 有详情实现但没有目录实现", source)
		}
	}
}

func TestNativeDetailHasNoHardcodedSwitch(t *testing.T) {
	// 回归锁：nativeDetail 里不应再出现 `switch source` 或 `source == source`。
	src := readCoreSourceForTest(t, "app_runtime.go")
	start := indexOf(src, "func (engine *nativeEngine) nativeDetail(")
	if start < 0 {
		t.Fatal("找不到 nativeDetail 函数")
	}
	rest := src[start:]
	end := indexOf(rest, "\nfunc ")
	body := rest[:end]
	if contains(body, "switch source {") {
		t.Error("nativeDetail 又出现了 switch source 分支")
	}
	if contains(body, "source == source") {
		t.Error("nativeDetail 又出现了 source == sourceXxx 判断")
	}
}
