package core

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// S7 完成后内置源清单为空，站源只来自已安装模块；
// 模块清单不应再出现任何已退役的内置源。
func TestRetiredBuiltinSourcesAbsentFromModuleList(t *testing.T) {
	engine := &nativeEngine{sourceProbes: map[string]nativeSourceProbe{}}
	for _, item := range engine.listModules() {
		if isRetiredSource(item.ID) {
			t.Fatalf("退役内置源不应出现在模块清单：%q", item.ID)
		}
	}
	for _, id := range []string{sourceHuangguoAI, sourceYeguo, sourceSorani, sourceChengguo} {
		if isRetiredSource(id) {
			t.Fatalf("内置源清单为空时 %q 不应被判为退役源", id)
		}
	}
}

func TestProbeSourcesHidesUnreachableSource(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		http.Error(writer, "gone", http.StatusServiceUnavailable)
	}))
	defer server.Close()
	engine := &nativeEngine{sourceProbes: map[string]nativeSourceProbe{}, downloader: &Downloader{client: server.Client()}}
	probes := engine.probeSources(context.Background(), []string{sourceHongguo}, true)
	if len(probes) != 1 || probes[0].Source != sourceHongguo {
		t.Fatalf("probes = %#v", probes)
	}
	engine.sourceProbes[sourceHongguo] = nativeSourceProbe{Source: sourceHongguo, Reachable: false, Error: "HTTP 503", CheckedAt: probes[0].CheckedAt}
	reachable, reason := engine.sourceReachable(sourceHongguo)
	if reachable || reason == "" {
		t.Fatalf("unreachable source should be hidden, got %v %q", reachable, reason)
	}
}

// 内置源已清空，未安装的站源应被入口拒绝，而不是返回空目录。
func TestUnknownSourceCatalogIsRejected(t *testing.T) {
	engine := &nativeEngine{catalogs: map[string][]nativeDrama{}, catalogStates: map[string]nativeCatalogState{}, sourceProbes: map[string]nativeSourceProbe{}}
	_, err := engine.nativeCatalog(context.Background(), nativeInput{Source: sourceHuangguoAI, Page: 1})
	if err == nil {
		t.Fatal("未安装的站源应被拒绝")
	}
}
