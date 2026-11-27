package core

import "testing"

func TestBuiltinMediaTableContainsKeyResolvers(t *testing.T) {
	// 主程序不内置站源：播放分发表应为空，任何内置源 id 都不应命中。
	want := []string{sourceCloudFront}
	for _, source := range want {
		if builtinMediaKnown(source) {
			t.Errorf("源 %s 不应再有内置播放解析实现", source)
		}
	}
}

func TestBuiltinMediaKnownRejectsUnknown(t *testing.T) {
	for _, unknown := range []string{"", "not-a-source", "unknown.example.com"} {
		if builtinMediaKnown(unknown) {
			t.Errorf("%q 不应有播放解析实现", unknown)
		}
	}
}

func TestResolveProviderMediaHasNoSourceBranches(t *testing.T) {
	// 回归锁：resolveProviderMedia 中不应再有 `chapter.Source == sourceXxx`
	// 这类站点分支。worker / 刷新章节的判定使用 map，不算硬编码分支。
	src := readCoreSourceForTest(t, "provider_media.go")
	start := indexOf(src, "func (d *Downloader) resolveProviderMedia(")
	if start < 0 {
		t.Fatal("找不到 resolveProviderMedia")
	}
	// 终止锚点：下一个顶层函数声明。finishProviderMedia 已在 S7 清理中移除，
	// 不能再拿它当边界，否则锚点缺失会让这条回归锁永远失效。
	rest := src[start+1:]
	var body string
	if end := indexOf(rest, "\nfunc "); end < 0 {
		body = rest
	} else {
		body = rest[:end]
	}
	if contains(body, "chapter.Source == source") {
		t.Error("resolveProviderMedia 又出现了 chapter.Source == sourceXxx 分支")
	}
}
