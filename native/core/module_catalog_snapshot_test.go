package core

import (
	"context"
	"testing"
)

// TestModuleCatalogPersistsToLocalShard 验证模块源目录拉取后落入本地分片，
// 冷启动可从磁盘恢复，避免每次从头全量翻页（本地快照增量更新的基础）。
func TestModuleCatalogPersistsToLocalShard(t *testing.T) {
	engine, err := newNativeEngine(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(engine.downloads.close)

	saved := moduleRegistry.modules
	defer func() { moduleRegistry.modules = saved }()
	manifest := moduleManifest{
		ID: sourceHuangguoAI, Name: "快照测试", Version: "1.0.0",
		Runtime: moduleRuntimeJS, API: 1, Adult: boolPtr(false),
		Capabilities: []string{"catalog"}, MainSha256: "",
	}
	provider, err := newTestJSProvider(manifest, []byte(`
		module.exports.catalogPage = function(page) {
			return { items: [{ id: "huangguoai:snap1", title: "本地快照剧", source: "huangguoai" }], hasMore: false };
		};
	`))
	if err != nil {
		t.Fatal(err)
	}
	moduleRegistry.modules = map[string]*jsModule{
		sourceHuangguoAI: {manifest: manifest, active: provider},
	}

	result, err := engine.nativeCatalog(context.Background(), nativeInput{Source: sourceHuangguoAI, Page: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Items) != 1 || result.Items[0].ID != "huangguoai:snap1" {
		t.Fatalf("模块目录结果不符：%+v", result.Items)
	}
	if got := len(engine.catalogs[sourceHuangguoAI]); got != 1 {
		t.Fatalf("模块目录未落入本地剧库，got %d", got)
	}
	if err := engine.flushCatalogSave(); err != nil {
		t.Fatal(err)
	}

	// 新 engine 指向同一目录，冷启动恢复分片。
	restarted, err := newNativeEngine(engine.directory)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(restarted.downloads.close)
	if !restarted.loadCatalogShards() {
		t.Fatal("冷启动未恢复任何分片")
	}
	if got := len(restarted.catalogs[sourceHuangguoAI]); got != 1 {
		t.Fatalf("冷启动未恢复模块源快照，got %d", got)
	}
}
