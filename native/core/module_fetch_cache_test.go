package core

import (
	"strings"
	"testing"
	"time"
)

func TestFetchCacheGetPut(t *testing.T) {
	DropModuleFetchCache()
	resp := jsFetchResponse{Status: 200, Body: "https://cdn.example.com/a.m3u8", BodyBase64: "YWJj"}
	moduleFetchCachePut("huangguo:publish", resp, 60)
	if got, ok := moduleFetchCacheGet("huangguo:publish"); !ok || got.Body != resp.Body {
		t.Fatalf("命中失败：%v %v", got, ok)
	}
	if _, ok := moduleFetchCacheGet("huangguo:other"); ok {
		t.Fatal("不存在的键不应命中")
	}
}

func TestFetchCacheRejects(t *testing.T) {
	DropModuleFetchCache()
	moduleFetchCachePut("err", jsFetchResponse{Status: 500, Body: "bad"}, 60)
	if _, ok := moduleFetchCacheGet("err"); ok {
		t.Fatal("非 2xx 不应缓存")
	}
	moduleFetchCachePut("", jsFetchResponse{Status: 200, Body: "x"}, 60)
	if _, ok := moduleFetchCacheGet(""); ok {
		t.Fatal("空键不应缓存")
	}
	moduleFetchCachePut(strings.Repeat("k", 200), jsFetchResponse{Status: 200, Body: "x"}, 60)
	if _, ok := moduleFetchCacheGet(strings.Repeat("k", 200)); ok {
		t.Fatal("超长键不应缓存")
	}
}

func TestFetchCacheExpiry(t *testing.T) {
	DropModuleFetchCache()
	moduleFetchCachePut("short", jsFetchResponse{Status: 200, Body: "v"}, 1)
	time.Sleep(1200 * time.Millisecond)
	if _, ok := moduleFetchCacheGet("short"); ok {
		t.Fatal("过期后不应命中")
	}
}

func TestFetchCacheDrop(t *testing.T) {
	DropModuleFetchCache()
	moduleFetchCachePut("x", jsFetchResponse{Status: 200, Body: "v"}, 60)
	DropModuleFetchCache()
	if _, ok := moduleFetchCacheGet("x"); ok {
		t.Fatal("清空后不应命中")
	}
}
