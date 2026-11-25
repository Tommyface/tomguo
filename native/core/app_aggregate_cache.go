package core

import (
	"sort"
	"strings"
	"sync"
	"time"
)

// 聚合结果短期缓存。
//
// Dart 侧原来每次切换都生成一个时间戳 groupId，只用于同一次 refine 的
// 配对，跨请求没有任何复用。这里给完整的 aggregateCatalog 请求做一个
// 稳定键 + 短 TTL 的结果缓存：切回上一个类型/视图时直接命中，不联网。
type aggregateResultCacheEntry struct {
	result  nativeAggregateResult
	expires time.Time
}

var (
	aggregateResultCacheMu   sync.Mutex
	aggregateResultCache     = map[string]aggregateResultCacheEntry{}
	aggregateResultCacheTTL  = 2 * time.Minute
	aggregateResultCacheSize = 64
)

// aggregateCacheKey 用请求的实际语义字段拼键；不含 groupId（那是时间戳）。
func aggregateCacheKey(input nativeInput) string {
	sources := append([]string{}, input.Sources...)
	sort.Strings(sources)
	var b strings.Builder
	b.WriteString(strings.Join(sources, ","))
	b.WriteByte('|')
	b.WriteString(strings.TrimSpace(input.ContentKind))
	b.WriteByte('|')
	if input.AdultView {
		b.WriteByte('1')
	} else {
		b.WriteByte('0')
	}
	b.WriteByte('|')
	if input.SourcesTagged {
		b.WriteByte('1')
	}
	b.WriteByte('|')
	b.WriteString(strings.TrimSpace(input.Category))
	b.WriteByte('|')
	b.WriteString(strings.TrimSpace(input.Query))
	b.WriteByte('|')
	if input.Page > 1 {
		b.WriteByte('p')
	}
	return b.String()
}

func aggregateResultCached(key string) (nativeAggregateResult, bool) {
	aggregateResultCacheMu.Lock()
	defer aggregateResultCacheMu.Unlock()
	entry, ok := aggregateResultCache[key]
	if !ok || time.Now().After(entry.expires) {
		if ok {
			delete(aggregateResultCache, key)
		}
		return nativeAggregateResult{}, false
	}
	return entry.result, true
}

func aggregateResultRemember(key string, result nativeAggregateResult) {
	if result.Partial || len(result.FailedSources) > 0 {
		// 部分结果 / 有源失败不能当完整缓存，否则下一轮会一直停在半成品上。
		return
	}
	aggregateResultCacheMu.Lock()
	defer aggregateResultCacheMu.Unlock()
	if len(aggregateResultCache) >= aggregateResultCacheSize {
		now := time.Now()
		for k, entry := range aggregateResultCache {
			if now.After(entry.expires) {
				delete(aggregateResultCache, k)
			}
		}
		for len(aggregateResultCache) >= aggregateResultCacheSize {
			for k := range aggregateResultCache {
				delete(aggregateResultCache, k)
				break
			}
		}
	}
	aggregateResultCache[key] = aggregateResultCacheEntry{
		result:  result,
		expires: time.Now().Add(aggregateResultCacheTTL),
	}
}
