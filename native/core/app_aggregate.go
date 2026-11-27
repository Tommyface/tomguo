package core

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"
)

// 站源整合：把各站源各自的 Category 归一成统一的内容类型 contentKind。
// 原始 Category 只是站源自己的筛选参数（line:/tag:/short_play/空值都有），
// 不能当全局分组键，所以这里单独推断一层统一类型。

type nativeAggregateSource struct {
	Source   string `json:"source"`
	SourceID string `json:"sourceId"`
	DramaID  string `json:"dramaId"`
	Title    string `json:"title"`
	Cover    string `json:"cover"`
	Episodes int    `json:"episodes"`
	VIP      *bool  `json:"vip,omitempty"`
	Playable bool   `json:"playable"`
	Score    int    `json:"score"`
	Adult    bool   `json:"adult"`
	Category string `json:"category,omitempty"`
}

type nativeAggregateItem struct {
	Key         string                  `json:"key"`
	Title       string                  `json:"title"`
	Cover       string                  `json:"cover"`
	ContentKind string                  `json:"contentKind"`
	Episodes    int                     `json:"episodes"`
	SourceCount int                     `json:"sourceCount"`
	Sources     []nativeAggregateSource `json:"sources"`
}

type nativeAggregateResult struct {
	Items         []nativeAggregateItem `json:"items"`
	HasMore       bool                  `json:"hasMore"`
	Page          int                   `json:"page"`
	TotalSources  int                   `json:"totalSources"`
	FailedSources map[string]string     `json:"failedSources,omitempty"`
	Partial       bool                  `json:"partial,omitempty"`
	Cached        bool                  `json:"cached,omitempty"`
	GroupID       string                `json:"groupId,omitempty"`
}

const (
	kindShortDrama = "short_drama"
	kindLiveAction = "live_action"
	kindVideo      = "video"
	kindAIDuanju   = "ai_duanju"
	kindAIManhua   = "ai_manhua"
	kindAIMogai    = "ai_mogai"
	kindAIHuanlian = "ai_huanlian"
)

// normalizeAggregateTitle 生成聚合键：去噪后缀、去标点空白、全角转半角、小写。
// normalizeAggregateTitle 生成聚合键：三段式（剥噪 → 繁简/数字归一 → 字符过滤）。
// 实现位于 aggregate_title.go，与 titlesMatch 共用同一套规则。
func normalizeAggregateTitle(title string) string {
	return aggregateTitleKey(title)
}

// inferContentKind 按优先级推断统一内容类型。
// 顺序固定：漫剧 > 魔改 > 换脸 > AI 短剧 > 视频 > 真人 > 普通短剧。
func inferContentKind(drama nativeDrama) string {
	haystack := strings.ToLower(strings.Join([]string{
		drama.Category, drama.Title, strings.Join(drama.Tags, " "),
	}, " "))
	has := func(needles ...string) bool {
		for _, needle := range needles {
			if strings.Contains(haystack, needle) {
				return true
			}
		}
		return false
	}
	switch {
	case has("ai-manju", "漫剧", "动态漫", "漫画", "manju"):
		return kindAIManhua
	case has("ai-mogai", "魔改", "mogai"):
		return kindAIMogai
	case has("ai-huanlian", "换脸", "huanlian"):
		return kindAIHuanlian
	case has("ai-duanju", "aishop", "ai短剧", "ai剧", "aiduanju"):
		return kindAIDuanju
	case has("vlog", "花絮", "预告", "幕后", "视频", "clip", "trailer", "video"):
		return kindVideo
	case has("真人", "都市", "甜宠", "逆袭", "古装", "悬疑", "短剧", "duanju"):
		return kindLiveAction
	default:
		return kindShortDrama
	}
}

var contentKindPriority = map[string]int{
	kindAIManhua: 7, kindAIMogai: 6, kindAIHuanlian: 5,
	kindAIDuanju: 4, kindVideo: 3, kindLiveAction: 2, kindShortDrama: 1,
}

func aggregateKindByVote(items []nativeDrama) string {
	counts := map[string]int{}
	for _, item := range items {
		kind := inferContentKind(item)
		counts[kind]++
	}
	best, bestCount := kindShortDrama, -1
	for kind, count := range counts {
		if count > bestCount || (count == bestCount && contentKindPriority[kind] > contentKindPriority[best]) {
			best, bestCount = kind, count
		}
	}
	return best
}

func sourceIsAdult(source string) bool {
	manifest := moduleManifestByID(source)
	if manifest == nil {
		return false
	}
	return manifest.Adult != nil && *manifest.Adult
}

func moduleManifestByID(id string) *moduleManifest {
	for _, manifest := range ModuleManifests() {
		if manifest.ID == canonicalProviderSource(id) {
			copy := manifest
			return &copy
		}
	}
	return nil
}

// aggregatePriorityCount 是首屏优先拉取的站源数量。
// 首屏只等这批源即可出卡片，剩余源在后台继续补，避免被慢源拖住。
const aggregatePriorityCount = 4

// aggregateSourceIDs 返回聚合要跑的源集合：注册表里有目录能力的源优先，
// 再并上调用方给的固定顺序。源清单不再从模块清单或内置分发表推导。
func aggregateSourceIDs() []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(crossSourceOrder)+sourceAdapters.Len())
	add := func(id string) {
		normalized := canonicalProviderSource(id)
		if normalized == "" {
			normalized = strings.ToLower(strings.TrimSpace(id))
		}
		if normalized == "" || seen[normalized] {
			return
		}
		seen[normalized] = true
		out = append(out, normalized)
	}
	for _, id := range crossSourceOrder {
		add(id)
	}
	for _, item := range sourceAdapters.List() {
		if item.Meta().AllowEmptyCatalog {
			continue
		}
		add(item.ID())
	}
	return out
}

// aggregateCatalogOnce 走适配层策略执行单源目录：超时、重试、限速、健康统计。
func (engine *nativeEngine) aggregateCatalogOnce(ctx context.Context, src string, input nativeInput) (nativeCatalogResult, error) {
	var result nativeCatalogResult
	runner := sourceAdapters.Runner(src)
	err := runner.Run(ctx, "catalog", func(callCtx context.Context) error {
		value, callErr := engine.nativeCatalog(callCtx, input)
		if callErr != nil {
			return classifyAggregateError(callErr)
		}
		result = value
		return nil
	})
	return result, err
}

// aggregateSourceItems 并发拉取每个源的目录页；单源失败不影响整体。
func (engine *nativeEngine) aggregateSourceItems(ctx context.Context, sources []string, page int, category, query string) (map[string][]nativeDrama, map[string]string) {
	out := make(map[string][]nativeDrama, len(sources))
	failed := map[string]string{}
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, 6)
	for _, raw := range sources {
		source := canonicalProviderSource(raw)
		if source == "" {
			source = strings.ToLower(strings.TrimSpace(raw))
		}
		// 已退役的内置源由同名插件接管时仍可用：只用裸 id 判断退役。
		bare := strings.ToLower(strings.TrimSpace(raw))
		if moduleSource(bare) == nil && isRetiredSource(source) {
			continue
		}
		wg.Add(1)
		go func(src string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			child, cancel := context.WithTimeout(ctx, 60*time.Second)
			defer cancel()
			input := nativeInput{Source: src, Page: page, Category: category, Query: query}
			result, err := engine.aggregateCatalogOnce(child, src, input)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				failed[src] = err.Error()
				return
			}
			out[src] = append(out[src], result.Items...)
		}(source)
	}
	wg.Wait()
	return out, failed
}

// aggregateByTitle 把同一归一化标题的多源条目合并成一条作品卡。
func aggregateByTitle(perSource map[string][]nativeDrama, adultView bool) []nativeAggregateItem {
	return aggregateByTitleTagged(perSource, adultView, false)
}

// aggregateByTitleTagged 在 sourcesTagged 时不再按模块清单判断成人源，改用调用方给的源集合。
func aggregateByTitleTagged(perSource map[string][]nativeDrama, adultView, sourcesTagged bool) []nativeAggregateItem {
	buckets := map[string][]nativeDrama{}
	// 同一源里可能有多个同名条目（分季/重复上架），只保留一条参与跨源合并，
	// 否则 sourceCount 会把同一个源算成多个来源。
	seenInSource := map[string]map[string]bool{}
	for source, items := range perSource {
		if !sourcesTagged && sourceIsAdult(source) != adultView {
			continue
		}
		if seenInSource[source] == nil {
			seenInSource[source] = map[string]bool{}
		}
		for _, item := range items {
			if isGenericAggregateTitle(item.Title) {
				continue
			}
			key := normalizeAggregateTitle(item.Title)
			if key == "" || seenInSource[source][key] {
				continue
			}
			seenInSource[source][key] = true
			buckets[key] = append(buckets[key], item)
		}
	}
	keys := make([]string, 0, len(buckets))
	for key := range buckets {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]nativeAggregateItem, 0, len(keys))
	for _, key := range keys {
		items := buckets[key]
		entry := nativeAggregateItem{
			Key: key, Title: strings.TrimSpace(items[0].Title),
			ContentKind: aggregateKindByVote(items),
			Sources:     make([]nativeAggregateSource, 0, len(items)),
		}
		for _, item := range items {
			source := canonicalProviderSource(item.Source)
			src := nativeAggregateSource{
				Source: source, SourceID: item.SourceID, DramaID: item.ID,
				Title: item.Title, Cover: item.Cover, Episodes: item.Episodes,
				VIP: item.VIP, Playable: item.Episodes > 0,
				Adult: sourceIsAdult(source), Category: item.Category,
			}
			src.Score = src.Episodes
			if src.VIP != nil && *src.VIP {
				src.Score -= 1000
			}
			entry.Sources = append(entry.Sources, src)
			if entry.Cover == "" && item.Cover != "" {
				entry.Cover = item.Cover
			}
			if item.Episodes > entry.Episodes {
				entry.Episodes = item.Episodes
			}
		}
		sort.SliceStable(entry.Sources, func(i, j int) bool {
			a, b := entry.Sources[i], entry.Sources[j]
			if a.Playable != b.Playable {
				return a.Playable
			}
			return a.Score > b.Score
		})
		entry.SourceCount = len(entry.Sources)
		out = append(out, entry)
	}
	return out
}

// aggregateMode 归一化聚合取数策略。
func aggregateMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "instant":
		return "instant"
	case "force":
		return "force"
	default:
		return "refresh"
	}
}

func (engine *nativeEngine) nativeAggregateCatalog(ctx context.Context, input nativeInput) (result nativeAggregateResult, err error) {
	// instant：只读缓存，绝不联网。首屏用它拿到零延迟的第一帧。
	if aggregateMode(input.Mode) == "instant" {
		cached := engine.nativeAggregateCached(input)
		cached.Cached = true
		return cached, nil
	}
	// 完整聚合结果短期复用：切类型/切视图来回翻时不再每轮重新联网。
	// refine 是后台补拉，必须绕过缓存；分页只缓存当前页。
	if !input.Refine && strings.TrimSpace(input.Query) == "" {
		key := aggregateCacheKey(input)
		if hit, ok := aggregateResultCached(key); ok {
			hit.Cached = true
			return hit, nil
		}
		defer func() {
			if err == nil {
				aggregateResultRemember(key, result)
			}
		}()
	}
	sources := input.Sources
	if len(sources) == 0 {
		sources = aggregateSourceIDs()
	}
	// 首屏只等优先级高的前几路源，返回 partial=true；其余源后台继续取。
	// 调用方拿到 partial 后可用同一 groupId 再发一次请求合并完整结果。
	partial := len(sources) > aggregatePriorityCount && input.Page <= 1 &&
		strings.TrimSpace(input.Query) == "" && !input.Refine
	if partial {
		first := append([]string{}, sources[:aggregatePriorityCount]...)
		perSource, failed := engine.aggregateSourceItems(ctx, first, 1, strings.TrimSpace(input.Category), "")
		items := aggregateByTitleTagged(perSource, input.AdultView, input.SourcesTagged)
		if input.ContentKind != "" {
			filtered := items[:0]
			for _, item := range items {
				if item.ContentKind == input.ContentKind {
					filtered = append(filtered, item)
				}
			}
			items = filtered
		}
		result = nativeAggregateResult{
			Items: items, Page: 1, TotalSources: len(sources), FailedSources: failed,
			HasMore: true, Partial: true, GroupID: input.GroupID,
		}
		if result.Items == nil {
			result.Items = []nativeAggregateItem{}
		}
		return result, nil
	}
	perSource, failed := engine.aggregateSourceItems(ctx, sources, max(1, input.Page), strings.TrimSpace(input.Category), strings.TrimSpace(input.Query))
	items := aggregateByTitleTagged(perSource, input.AdultView, input.SourcesTagged)
	if input.ContentKind != "" {
		filtered := items[:0]
		for _, item := range items {
			if item.ContentKind == input.ContentKind {
				filtered = append(filtered, item)
			}
		}
		items = filtered
	}
	sourceSet := map[string]bool{}
	for _, raw := range sources {
		source := canonicalProviderSource(raw)
		if source == "" {
			source = strings.ToLower(strings.TrimSpace(raw))
		}
		sourceSet[source] = true
	}
	result = nativeAggregateResult{
		Items: items, Page: max(1, input.Page), TotalSources: len(sourceSet), FailedSources: failed,
		HasMore: len(items) >= 30,
	}
	if result.Items == nil {
		result.Items = []nativeAggregateItem{}
	}
	return result, nil
}

func (engine *nativeEngine) nativeAggregateSources(ctx context.Context, input nativeInput) (nativeAggregateResult, error) {
	perSource, failed := engine.aggregateSourceItems(ctx, input.Sources, 1, "", input.Query)
	items := aggregateByTitleTagged(perSource, input.AdultView, input.SourcesTagged)
	key := normalizeAggregateTitle(input.Query)
	if input.AggregateKey != "" {
		key = input.AggregateKey
	}
	matched := []nativeAggregateItem{}
	for _, item := range items {
		if item.Key == key || strings.EqualFold(item.Title, input.Query) {
			matched = append(matched, item)
		}
	}
	return nativeAggregateResult{Items: matched, FailedSources: failed, TotalSources: len(perSource)}, nil
}

// nativeAggregateCached 用各源已有目录缓存拼聚合页，不发起任何网络请求。
// 首帧靠它秒出，随后再走 aggregateCatalog 拿新数据。
func (engine *nativeEngine) nativeAggregateCached(input nativeInput) nativeAggregateResult {
	sources := input.Sources
	if len(sources) == 0 {
		sources = aggregateSourceIDs()
	}
	perSource := make(map[string][]nativeDrama, len(sources))
	for _, raw := range sources {
		source := canonicalProviderSource(raw)
		if source == "" {
			source = strings.ToLower(strings.TrimSpace(raw))
		}
		if items := engine.nativeCached(source).Items; len(items) > 0 {
			perSource[source] = items
		}
	}
	items := aggregateByTitleTagged(perSource, input.AdultView, input.SourcesTagged)
	if input.ContentKind != "" {
		filtered := items[:0]
		for _, item := range items {
			if item.ContentKind == input.ContentKind {
				filtered = append(filtered, item)
			}
		}
		items = filtered
	}
	result := nativeAggregateResult{
		Items: items, Page: 1, TotalSources: len(sources),
		HasMore: len(items) >= 30, Cached: true,
	}
	if result.Items == nil {
		result.Items = []nativeAggregateItem{}
	}
	return result
}
