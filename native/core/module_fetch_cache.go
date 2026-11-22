package core

import (
	"strings"
	"sync"
	"time"
)

// 跨模块共享的 fetch 缓存：发布页、入口发现页这类多个源都要用的 URL，
// 由宿主统一缓存一次，避免每个模块各拉一遍把 60 秒调用预算耗光。
// 只缓存 2xx；错误不缓存，让下一个模块有机会重试。

const (
	moduleFetchCacheDefaultTTL = 30 * time.Minute
	moduleFetchCacheMaxTTL     = 24 * time.Hour
	moduleFetchCacheMaxEntries = 64
	moduleFetchCacheMaxBytes   = 4 << 20
)

type moduleFetchCacheItem struct {
	response jsFetchResponse
	expires  time.Time
	bytes    int
}

var moduleFetchCache = struct {
	sync.Mutex
	items map[string]moduleFetchCacheItem
	used  int
}{items: map[string]moduleFetchCacheItem{}}

// moduleFetchCacheKey 归一化键名：忽略首尾空白，最长 128 字符。
func moduleFetchCacheKey(key string) (string, bool) {
	key = strings.TrimSpace(key)
	if key == "" || len(key) > 128 {
		return "", false
	}
	return key, true
}

// moduleFetchCacheGet 读取未过期的缓存；命中时返回 (响应, true)。
func moduleFetchCacheGet(key string) (jsFetchResponse, bool) {
	key, ok := moduleFetchCacheKey(key)
	if !ok {
		return jsFetchResponse{}, false
	}
	moduleFetchCache.Lock()
	defer moduleFetchCache.Unlock()
	item, ok := moduleFetchCache.items[key]
	if !ok {
		return jsFetchResponse{}, false
	}
	if time.Now().After(item.expires) {
		delete(moduleFetchCache.items, key)
		moduleFetchCache.used -= item.bytes
		return jsFetchResponse{}, false
	}
	return item.response, true
}

// moduleFetchCachePut 写入缓存；超出条目数或总字节时按最早过期淘汰。
func moduleFetchCachePut(key string, response jsFetchResponse, ttlSec int) {
	key, ok := moduleFetchCacheKey(key)
	if !ok {
		return
	}
	if response.Status < 200 || response.Status >= 300 {
		return
	}
	ttl := time.Duration(ttlSec) * time.Second
	if ttl <= 0 {
		ttl = moduleFetchCacheDefaultTTL
	}
	if ttl > moduleFetchCacheMaxTTL {
		ttl = moduleFetchCacheMaxTTL
	}
	item := moduleFetchCacheItem{
		response: response,
		expires:  time.Now().Add(ttl),
		bytes:    len(response.Body) + len(response.BodyBase64) + len(key),
	}
	moduleFetchCache.Lock()
	defer moduleFetchCache.Unlock()
	if old, exists := moduleFetchCache.items[key]; exists {
		moduleFetchCache.used -= old.bytes
	}
	moduleFetchCache.items[key] = item
	moduleFetchCache.used += item.bytes
	for len(moduleFetchCache.items) > moduleFetchCacheMaxEntries || moduleFetchCache.used > moduleFetchCacheMaxBytes {
		oldestKey, oldestExpiry := "", time.Time{}
		for candidate, entry := range moduleFetchCache.items {
			if oldestKey == "" || entry.expires.Before(oldestExpiry) {
				oldestKey, oldestExpiry = candidate, entry.expires
			}
		}
		if oldestKey == "" {
			return
		}
		moduleFetchCache.used -= moduleFetchCache.items[oldestKey].bytes
		delete(moduleFetchCache.items, oldestKey)
	}
}

// DropModuleFetchCache 清空全部缓存；模块升级或卸载后调用，避免读到旧响应。
func DropModuleFetchCache() {
	moduleFetchCache.Lock()
	defer moduleFetchCache.Unlock()
	moduleFetchCache.items = map[string]moduleFetchCacheItem{}
	moduleFetchCache.used = 0
}
