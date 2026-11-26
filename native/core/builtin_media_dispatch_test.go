package core

import "testing"

func TestBuiltinMediaTableContainsKeyResolvers(t *testing.T) {
	want := []string{
		sourceCloudFront, sourceHuangju, sourceYeguo, sourceDSD,
		sourceHanxiaoquan, sourceHuangguoMain, sourceHuangguoSelected,
		sourceHuangguoWelfare, sourceHuakui, sourceChengguo, sourceHongguo,
		sourceHuangdou, sourceHema, sourceDuanju51,
	}
	for _, source := range want {
		if !builtinMediaKnown(source) {
			t.Errorf("源 %s 缺少播放解析实现", source)
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
	rest := src[start:]
	end := indexOf(rest, "\nfunc (d *Downloader) finishProviderMedia(")
	if end < 0 {
		t.Fatal("找不到 finishProviderMedia")
	}
	body := rest[:end]
	if contains(body, "chapter.Source == source") {
		t.Error("resolveProviderMedia 又出现了 chapter.Source == sourceXxx 分支")
	}
}
