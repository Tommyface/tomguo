package core

import (
	"strings"
	"testing"
	"time"
)

func TestStorageSetGet(t *testing.T) {
	store := &moduleStorage{entries: map[string]moduleStorageEntry{}}
	if err := store.Set("token", "abc123", 0); err != nil {
		t.Fatalf("写入失败：%v", err)
	}
	if value, ok := store.Get("token"); !ok || value != "abc123" {
		t.Fatalf("读取错误：%q %v", value, ok)
	}
	if _, ok := store.Get("missing"); ok {
		t.Fatal("不存在的键不应命中")
	}
	store.Delete("token")
	if _, ok := store.Get("token"); ok {
		t.Fatal("删除后不应命中")
	}
}

func TestStorageExpiry(t *testing.T) {
	store := &moduleStorage{entries: map[string]moduleStorageEntry{}}
	_ = store.Set("short", "v", 50*time.Millisecond)
	if _, ok := store.Get("short"); !ok {
		t.Fatal("未到期应命中")
	}
	time.Sleep(80 * time.Millisecond)
	if _, ok := store.Get("short"); ok {
		t.Fatal("已到期不应命中")
	}
}

func TestStorageValidation(t *testing.T) {
	store := &moduleStorage{entries: map[string]moduleStorageEntry{}}
	if err := store.Set("", "v", 0); err == nil {
		t.Fatal("空键应失败")
	}
	if err := store.Set(strings.Repeat("k", 200), "v", 0); err == nil {
		t.Fatal("超长键应失败")
	}
	if err := store.Set("a\x01b", "v", 0); err == nil {
		t.Fatal("控制字符应失败")
	}
	if err := store.Set("big", strings.Repeat("x", moduleStorageValueMax+1), 0); err == nil {
		t.Fatal("超大值应失败")
	}
	if err := store.Set("ttl", "v", -time.Second); err == nil {
		t.Fatal("负 TTL 应失败")
	}
}

func TestStorageQuotaEviction(t *testing.T) {
	store := &moduleStorage{entries: map[string]moduleStorageEntry{}}
	if err := store.Set("keeper", "no-expiry", 0); err != nil {
		t.Fatalf("写入失败：%v", err)
	}
	if err := store.Set("oldest", "v", time.Hour); err != nil {
		t.Fatalf("写入失败：%v", err)
	}
	if err := store.Set("newest", "v", 10*time.Hour); err != nil {
		t.Fatalf("写入失败：%v", err)
	}
	// 撑满条目上限，触发按最早过期淘汰。
	for i := 0; i < moduleStorageEntryMax+5; i++ {
		key := "fill" + strings.Repeat("z", i%3) + string(rune('a'+i%26)) + strings.Repeat("q", i%5)
		_ = store.Set(key, "v", time.Duration(i+1)*time.Hour)
	}
	if len(store.entries) > moduleStorageEntryMax {
		t.Fatalf("条目数 %d 超过上限", len(store.entries))
	}
	if _, ok := store.Get("keeper"); !ok {
		t.Fatal("无过期时间的条目不应被淘汰")
	}
}

func TestDropModuleStorage(t *testing.T) {
	store := moduleStorageFor("Demo")
	_ = store.Set("token", "v", 0)
	if _, ok := moduleStorageFor("demo").Get("token"); !ok {
		t.Fatal("模块 id 应大小写归一")
	}
	DropModuleStorage("demo")
	if _, ok := moduleStorageFor("demo").Get("token"); ok {
		t.Fatal("Drop 后不应命中")
	}
}
