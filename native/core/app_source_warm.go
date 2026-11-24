package core

import (
	"context"
	"strings"
	"sync"
	"time"
)

const nativeWarmConcurrency = 4

type nativeSourceWarm struct {
	Source    string    `json:"source"`
	Items     int       `json:"items"`
	ElapsedMS int64     `json:"elapsedMs"`
	Error     string    `json:"error,omitempty"`
	WarmedAt  time.Time `json:"warmedAt"`
}

// warmCatalogs 并发预热多个站源的首屏目录。
//
// 首屏慢的根源是各源域名首包握手叠加串行切换，与解析无关；
// 这里在用户切到站源页之前就把第 1 页拉好写进缓存，切过去时走缓存。
func (engine *nativeEngine) warmCatalogs(ctx context.Context, sources []string, force bool) []nativeSourceWarm {
	if len(sources) == 0 {
		seen := map[string]bool{}
		for _, manifest := range bundledModules {
			if manifest.Retired || seen[manifest.ID] {
				continue
			}
			seen[manifest.ID] = true
			sources = append(sources, manifest.ID)
		}
		for _, manifest := range ModuleManifests() {
			if seen[manifest.ID] {
				continue
			}
			seen[manifest.ID] = true
			sources = append(sources, manifest.ID)
		}
	}
	queue := make([]string, 0, len(sources))
	seen := map[string]bool{}
	for _, source := range sources {
		key := canonicalProviderSource(strings.ToLower(strings.TrimSpace(source)))
		if key == "" || seen[key] || isRetiredSource(key) {
			continue
		}
		seen[key] = true
		if !force {
			cached := engine.nativeCached(nativeCatalogKey(key, ""))
			if len(cached.Items) > 0 && cached.Fresh {
				continue
			}
		}
		queue = append(queue, key)
	}

	results := make([]nativeSourceWarm, len(queue))
	var wait sync.WaitGroup
	slots := make(chan struct{}, nativeWarmConcurrency)
	for index, source := range queue {
		wait.Add(1)
		go func(index int, source string) {
			defer wait.Done()
			slots <- struct{}{}
			defer func() { <-slots }()
			start := time.Now()
			warm := nativeSourceWarm{Source: source, WarmedAt: start}
			result, err := engine.nativeCatalog(ctx, nativeInput{Source: source, Page: 1, Force: force})
			warm.ElapsedMS = time.Since(start).Milliseconds()
			warm.Items = len(result.Items)
			if err != nil {
				warm.Error = publicError(err).Error()
			}
			results[index] = warm
		}(index, source)
	}
	wait.Wait()
	return results
}
