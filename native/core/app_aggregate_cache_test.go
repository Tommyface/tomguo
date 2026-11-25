package core

import (
	"testing"
	"time"
)

func TestAggregateCacheKeyStable(t *testing.T) {
	a := nativeInput{Sources: []string{"b", "a"}, ContentKind: kindShortDrama, AdultView: true, Page: 1, SourcesTagged: true}
	b := nativeInput{Sources: []string{"a", "b"}, ContentKind: kindShortDrama, AdultView: true, Page: 1, SourcesTagged: true}
	if aggregateCacheKey(a) != aggregateCacheKey(b) {
		t.Fatal("源顺序不同应生成相同缓存键")
	}
	b.ContentKind = kindLiveAction
	if aggregateCacheKey(a) == aggregateCacheKey(b) {
		t.Fatal("kind 不同必须生成不同键")
	}
}

func TestAggregateCacheHitExpiry(t *testing.T) {
	key := "test-key-cache-hit"
	aggregateResultRemember(key, nativeAggregateResult{Items: []nativeAggregateItem{{Key: "x"}}})
	if got, ok := aggregateResultCached(key); !ok || len(got.Items) != 1 {
		t.Fatalf("期望命中缓存，got=%v ok=%v", got, ok)
	}
	aggregateResultCacheMu.Lock()
	aggregateResultCache[key] = aggregateResultCacheEntry{expires: time.Now().Add(-time.Second)}
	aggregateResultCacheMu.Unlock()
	if _, ok := aggregateResultCached(key); ok {
		t.Fatal("过期条目必须失效")
	}
}

func TestAggregateCacheSkipsPartial(t *testing.T) {
	key := "test-key-partial"
	aggregateResultRemember(key, nativeAggregateResult{Partial: true, Items: []nativeAggregateItem{{Key: "x"}}})
	if _, ok := aggregateResultCached(key); ok {
		t.Fatal("partial 结果不得入缓存")
	}
}
