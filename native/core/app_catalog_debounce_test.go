package core

import (
	"testing"
	"time"
)

func TestScheduleCatalogSaveDebounces(t *testing.T) {
	engine := &nativeEngine{directory: t.TempDir(), catalogs: map[string][]nativeDrama{}, catalogStates: map[string]nativeCatalogState{}}
	engine.mu.Lock()
	engine.catalogs["debounce-source"] = []nativeDrama{{ID: "debounce-source:1", Source: "debounce-source", Title: "测试"}}
	for i := 0; i < 5; i++ {
		engine.scheduleCatalogSaveLocked()
	}
	if engine.catalogSaveTimer == nil {
		engine.mu.Unlock()
		t.Fatal("期望排入一个防抖定时器")
	}
	engine.mu.Unlock()
	// 立即 flush 应取消定时器并落盘，进程不残留待执行任务。
	if err := engine.flushCatalogSave(); err != nil {
		t.Fatalf("flush 失败: %v", err)
	}
	engine.mu.Lock()
	defer engine.mu.Unlock()
	if engine.catalogSaveTimer != nil {
		t.Fatal("flush 之后定时器应被清除")
	}
}

func TestStaleCatalogServedBeforeNetwork(t *testing.T) {
	engine := &nativeEngine{directory: t.TempDir(), catalogs: map[string][]nativeDrama{}, catalogStates: map[string]nativeCatalogState{}}
	key := nativeCatalogKey(sourceHuangguoAI, "")
	engine.mu.Lock()
	engine.catalogs[key] = []nativeDrama{{ID: key + ":1", Source: sourceHuangguoAI, Title: "旧快照"}}
	engine.catalogStates[key] = nativeCatalogState{
		Page: 1, UpdatedAt: time.Now().Add(-time.Hour),
	}
	engine.mu.Unlock()

	cached := engine.nativeCached(key)
	if cached.Fresh {
		t.Fatal("1 小时前的快照不应标记为 fresh")
	}
	if len(cached.Items) != 1 {
		t.Fatalf("期望拿到 1 条旧快照，got %d", len(cached.Items))
	}
	if time.Since(engine.catalogStates[key].UpdatedAt) >= nativeCatalogStaleServeLimit {
		t.Fatal("1 小时应在 stale 可服务范围内")
	}
}
