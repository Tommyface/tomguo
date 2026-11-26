package core

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCatalogShardRoundTrip(t *testing.T) {
	dir := t.TempDir()
	shard := catalogShard{
		Source: sourceHongguo,
		Items:  []nativeDrama{{ID: "hongguo:1", Title: "测试剧", Source: sourceHongguo}},
		State:  nativeCatalogState{Page: 1, HasMore: true, UpdatedAt: time.Now()},
	}
	meta, err := writeCatalogShard(dir, shard)
	if err != nil {
		t.Fatal(err)
	}
	if meta.Bytes == 0 {
		t.Fatal("分片字节数为 0")
	}
	got, ok := readCatalogShard(dir, sourceHongguo)
	if !ok {
		t.Fatal("分片读不回来")
	}
	if len(got.Items) != 1 || got.Items[0].ID != "hongguo:1" || got.Source != sourceHongguo {
		t.Fatalf("分片内容不符：%+v", got)
	}
}

func TestCatalogShardCorruptionIsolated(t *testing.T) {
	dir := t.TempDir()
	for _, source := range []string{sourceHongguo, sourceHuangju} {
		if _, err := writeCatalogShard(dir, catalogShard{
			Source: source,
			Items:  []nativeDrama{{ID: source + ":1", Title: "剧", Source: source}},
			State:  nativeCatalogState{Page: 1, UpdatedAt: time.Now()},
		}); err != nil {
			t.Fatal(err)
		}
	}
	// 破坏其中一个分片：另一个必须仍可读。
	broken := catalogShardPath(dir, sourceHuangju)
	if err := os.WriteFile(broken, []byte("{not json"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, ok := readCatalogShard(dir, sourceHuangju); ok {
		t.Fatal("损坏分片不应解析成功")
	}
	if _, ok := readCatalogShard(dir, sourceHongguo); !ok {
		t.Fatal("损坏一个分片不应影响其他源")
	}
	names := listCatalogShards(dir)
	if len(names) != 2 {
		t.Fatalf("分片数量 %d，期望 2", len(names))
	}
}

func TestCatalogShardIndex(t *testing.T) {
	dir := t.TempDir()
	shards := map[string]catalogShardMeta{
		"hongguo": {Bytes: 100, SavedAt: time.Now()},
	}
	if err := writeCatalogShardIndex(dir, shards); err != nil {
		t.Fatal(err)
	}
	index := readCatalogShardIndex(dir)
	if len(index.Shards) != 1 || index.Shards["hongguo"].Bytes != 100 {
		t.Fatalf("索引内容不符：%+v", index)
	}
	// 索引缺失时返回空索引而不是 panic。
	if empty := readCatalogShardIndex(t.TempDir()); len(empty.Shards) != 0 {
		t.Fatalf("空目录索引应为空：%+v", empty)
	}
}

func TestSafeShardName(t *testing.T) {
	cases := map[string]string{
		"hongguo":        "hongguo",
		"huangguo-main":  "huangguo-main",
		"huangguo|short": "huangguo_short",
		"../escape":      "escape",
		"":               "unknown",
	}
	for input, want := range cases {
		if got := safeShardName(input); got != want {
			t.Errorf("safeShardName(%q) = %q，期望 %q", input, got, want)
		}
	}
}

func TestPruneCatalogShards(t *testing.T) {
	dir := t.TempDir()
	for _, source := range []string{"keep", "stale"} {
		path := catalogShardPath(dir, source)
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("{}"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	// 把 stale 的修改时间往前拨，模拟长期未更新。
	old := time.Now().Add(-40 * 24 * time.Hour)
	if err := os.Chtimes(catalogShardPath(dir, "stale"), old, old); err != nil {
		t.Fatal(err)
	}
	removed := pruneCatalogShards(dir, map[string]bool{"keep": true}, 30*24*time.Hour)
	if removed != 1 {
		t.Fatalf("清理数量 %d，期望 1", removed)
	}
	if _, ok := readCatalogShard(dir, "keep"); ok {
		// 分片内容是我们写的空 {}，解析会失败，但文件必须还在。
		t.Log("keep 分片解析失败符合预期（内容为 {}）")
	}
	if _, err := os.Stat(catalogShardPath(dir, "keep")); err != nil {
		t.Fatal("keep 分片不应被删除")
	}
}
