package core

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"
)

// 同名换源搜索：按标题在其他站源里找同一部剧，优先挑无 VIP 且集数更全的版本。
// 目的只有一个——某站源把正片锁在服务端时，用另一站源的同名条目拿到全集。

type crossSourceCandidate struct {
	ID           string `json:"id"`
	Source       string `json:"source"`
	SourceID     string `json:"sourceId"`
	Title        string `json:"title"`
	Cover        string `json:"cover"`
	Episodes     int    `json:"episodes"`
	VIPEpisodes  int    `json:"vipEpisodes"`
	Playable     bool   `json:"playable"`
	LastPlayable bool   `json:"lastPlayable"`
	Error        string `json:"error,omitempty"`
	Score        int    `json:"score"`
}

type crossSourceResult struct {
	Query       string                 `json:"query"`
	Title       string                 `json:"title"`
	Episodes    int                    `json:"episodes"`
	VIPEpisodes int                    `json:"vipEpisodes"`
	Candidates  []crossSourceCandidate `json:"candidates"`
	Errors      map[string]string      `json:"errors,omitempty"`
	ElapsedMS   int64                  `json:"elapsedMs"`
}

const (
	// crossSourceSourceBudget 是单个站源在整轮换源里的总预算。
	crossSourceSourceBudget = 20 * time.Second
	// crossSourceVariantLimit 是单次搜索变体的上限。
	crossSourceVariantLimit = 8 * time.Second
)

// crossSourceOrder 是可搜索站源的固定顺序：先跑实测稳定的，再跑可能超时的。
// 运行时安装的 JS 站源模块会追加在末尾（见 moduleCrossSourceOrder）。
var crossSourceOrder = []string{
	sourceChengguo,
	sourceHuangju,
	sourceGuipian,
	sourceHanxiaoquan,
	sourceDuanju51,
	sourceHuangdou,
	sourceHema,
	sourceEZDMW,
	sourceSorani,
	sourceHongguo,
}

// moduleCrossSourceOrder 返回已安装模块的站源 id，按注册顺序稳定排序。
func moduleCrossSourceOrder() []string {
	manifests := ModuleManifests()
	out := make([]string, 0, len(manifests))
	for _, manifest := range manifests {
		if manifest.AllowEmptyCatalog {
			continue
		}
		out = append(out, manifest.ID)
	}
	sort.Strings(out)
	return out
}

// crossSourceSearch 用各站源自己的搜索入口。返回空表示该源搜不到。
func (d *Downloader) crossSourceSearch(ctx context.Context, source, query string) ([]Drama, error) {
	switch source {
	case sourceChengguo:
		items, _, err := d.fetchChengguoCatalogPage(ctx, 1, "", query)
		return items, err
	case sourceHuangju:
		items, _, err := d.fetchHuangjuCatalogPage(ctx, 1, "", query)
		return items, err
	case sourceGuipian:
		items, _, err := d.fetchGuipianCatalogPage(ctx, 1, "", query)
		return items, err
	case sourceHanxiaoquan:
		items, _, err := d.fetchHanxiaoquanCatalogPage(ctx, 1, "", query)
		return items, err
	case sourceDuanju51:
		items, _, err := d.fetchDuanju51CatalogPage(ctx, 1, "", query)
		return items, err
	case sourceHema:
		items, _, err := d.fetchHemaCatalogPage(ctx, 1, "", query)
		return items, err
	case sourceEZDMW:
		items, _, err := d.fetchEZDMWCatalogPage(ctx, 1, "", query)
		return items, err
	case sourceSorani:
		items, _, err := d.fetchSoraniCatalogPage(ctx, 1, "", query)
		return items, err
	case sourceYeguo:
		items, _, err := d.fetchYeguoCatalogPage(ctx, 1, "", query)
		return items, err
	case sourceDSD:
		items, _, err := d.fetchDSDCatalogPage(ctx, 1, "", query)
		return items, err
	case sourceHongguo:
		entry, err := d.searchHongguoDramas(ctx, query)
		if err != nil {
			return nil, err
		}
		return entry.Dramas, nil
	default:
		// 已安装的 JS 站源模块。
		if moduleSource(source) != nil {
			items, _, err := d.moduleCatalog(ctx, source, 1, "", query)
			return items, err
		}
	case sourceHuangdou:
		// 黄豆没有独立搜索接口，用搜索榜结果在本地按标题过滤。
		client := newHuangdouAPIClient(d)
		var decoded any
		if err := client.call(ctx, "/drama/rank", map[string]any{"tab": "search", "page": "1"}, &decoded); err != nil {
			return nil, err
		}
		needle := hongguoSearchText(query)
		var out []Drama
		for _, row := range huangdouList(decoded) {
			drama := huangdouDramaFromMap(row)
			if drama.ID == "" {
				continue
			}
			if strings.Contains(hongguoSearchText(drama.DisplayTitle()), needle) {
				out = append(out, drama)
			}
		}
		return out, nil
	}
	return nil, nil
}

// crossSourceQueries 生成搜索变体：站点对全角标点处理不一致，
// 用原串、去标点串、以及去标点后的前 4/6 字依次尝试，命中即停。
func crossSourceQueries(title string) []string {
	seen := map[string]bool{}
	var out []string
	add := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			return
		}
		seen[value] = true
		out = append(out, value)
	}
	add(title)
	cleaned := hongguoSearchText(title)
	add(cleaned)
	// 逐字截短：站点对「存在感：零」这种带副标题的全名往往搜不到，
	// 但用主干「存在感」能命中，所以从全长往下退到 2 个字。
	runes := []rune(cleaned)
	for length := len(runes) - 1; length >= 2; length-- {
		add(string(runes[:length]))
	}
	return out
}

// crossSourceTitleMatch 判定标题是否算“同一部”。
// 去掉标点空格后必须互相包含，且长度差不超过原串的一半，避免「存在感」误配「存在感：零」这种跨剧。
func crossSourceTitleMatch(query, title string) bool {
	q, t := hongguoSearchText(query), hongguoSearchText(title)
	if q == "" || t == "" {
		return false
	}
	if q == t {
		return true
	}
	short, long := q, t
	if len(short) > len(long) {
		short, long = long, short
	}
	if !strings.Contains(long, short) {
		return false
	}
	// 短串至少占长串 60%，且长度不短于 3 个字，才认作同一部。
	if len([]rune(short)) < 3 {
		return false
	}
	return float64(len([]rune(short)))/float64(len([]rune(long))) >= 0.6
}

func (engine *nativeEngine) nativeCrossSource(ctx context.Context, input nativeInput) (crossSourceResult, error) {
	started := time.Now()
	title := strings.TrimSpace(input.Query)
	if title == "" {
		title = strings.TrimSpace(input.Drama.Title)
	}
	if title == "" {
		return crossSourceResult{}, errCrossSourceNoTitle
	}
	originSource, originID, _ := splitProviderDramaID(input.Drama.ID)
	result := crossSourceResult{Query: title, Title: title, Errors: map[string]string{}}
	// CachedOnly：只用已缓存的目录按标题匹配，不联网。
	// 换源页先拿它秒出候选，再走完整搜索补详情。
	if input.CachedOnly {
		return engine.crossSourceCached(input, title), nil
	}
	if input.Drama.Episodes > 0 {
		result.Episodes = input.Drama.Episodes
	}
	if input.Drama.VIP != nil && *input.Drama.VIP {
		result.VIPEpisodes = 1
	}

	type found struct {
		drama Drama
		src   string
	}
	var (
		mu        sync.Mutex
		collected []found
		wg        sync.WaitGroup
	)
	sem := make(chan struct{}, 3) // 并发 3，别把站点打急
	for _, source := range append(crossSourceOrder, moduleCrossSourceOrder()...) {
		source := source
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			var (
				items []Drama
				err   error
			)
			// 单个站源总预算：多变体共享 20 秒，避免一个慢源把整轮换源拖死。
			deadline := time.Now().Add(crossSourceSourceBudget)
			for _, variant := range crossSourceQueries(title) {
				if ctx.Err() != nil {
					break
				}
				remaining := time.Until(deadline)
				if remaining <= 0 {
					if err == nil && len(items) == 0 {
						err = context.DeadlineExceeded
					}
					break
				}
				if remaining > crossSourceVariantLimit {
					remaining = crossSourceVariantLimit
				}
				searchCtx, cancel := context.WithTimeout(ctx, remaining)
				items, err = engine.downloader.crossSourceSearch(searchCtx, source, variant)
				cancel()
				if err == nil && len(items) > 0 {
					break
				}
			}
			if err != nil && len(items) == 0 {
				mu.Lock()
				result.Errors[source] = err.Error()
				mu.Unlock()
				return
			}
			for _, drama := range items {
				if !crossSourceTitleMatch(title, drama.DisplayTitle()) {
					continue
				}
				if drama.ID == input.Drama.ID {
					continue
				}
				mu.Lock()
				collected = append(collected, found{drama: drama, src: source})
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	_ = originSource
	_ = originID

	// 逐个补详情，拿真实集数与 VIP 分布，再验证首末集能否取流。
	for _, item := range collected {
		if err := ctx.Err(); err != nil {
			break
		}
		candidate := crossSourceCandidate{
			ID: item.drama.ID, Source: item.src, SourceID: item.drama.SourceID,
			Title: item.drama.DisplayTitle(), Cover: nativeNormalize(item.drama).Cover,
			Episodes: nativeNormalize(item.drama).Episodes,
		}
		detailCtx, cancel := context.WithTimeout(ctx, 25*time.Second)
		payload, err := engine.nativeDetail(detailCtx, nativeNormalize(item.drama))
		cancel()
		if err != nil {
			candidate.Error = err.Error()
			result.Candidates = append(result.Candidates, candidate)
			continue
		}
		detail, ok := payload.(map[string]any)
		if !ok {
			candidate.Error = "详情格式异常"
			result.Candidates = append(result.Candidates, candidate)
			continue
		}
		chapters, _ := detail["chapters"].([]Chapter)
		candidate.Episodes = len(chapters)
		for _, chapter := range chapters {
			if chapter.VIP {
				candidate.VIPEpisodes++
			}
		}
		if fresh, ok := detail["drama"].(nativeDrama); ok && fresh.Cover != "" {
			candidate.Cover = fresh.Cover
		}
		if len(chapters) > 0 {
			probeCtx, probeCancel := context.WithTimeout(ctx, 30*time.Second)
			first := chapters[0]
			if _, err := engine.downloader.resolveProviderMedia(probeCtx, Task{DramaID: candidate.ID, Chapter: first, Index: 0}); err == nil {
				candidate.Playable = true
			} else {
				candidate.Error = err.Error()
			}
			if len(chapters) > 1 {
				last := chapters[len(chapters)-1]
				if _, err := engine.downloader.resolveProviderMedia(probeCtx, Task{DramaID: candidate.ID, Chapter: last, Index: len(chapters) - 1}); err == nil {
					candidate.LastPlayable = true
				}
			}
			probeCancel()
		}
		candidate.Score = crossSourceScore(candidate, result.Episodes, result.VIPEpisodes)
		result.Candidates = append(result.Candidates, candidate)
	}

	sort.SliceStable(result.Candidates, func(i, j int) bool {
		if result.Candidates[i].Score != result.Candidates[j].Score {
			return result.Candidates[i].Score > result.Candidates[j].Score
		}
		return result.Candidates[i].Episodes > result.Candidates[j].Episodes
	})
	result.ElapsedMS = time.Since(started).Milliseconds()
	return result, nil
}

// crossSourceScore 打分：能播 > 无 VIP > 集数全 > 末集也能播。
func crossSourceScore(candidate crossSourceCandidate, wantEpisodes, wantVIP int) int {
	score := 0
	if candidate.Playable {
		score += 50
	}
	if candidate.LastPlayable {
		score += 25
	}
	if candidate.VIPEpisodes == 0 {
		score += 30
	} else {
		score -= candidate.VIPEpisodes * 3
	}
	if wantEpisodes > 0 && candidate.Episodes >= wantEpisodes {
		score += 20
	} else if candidate.Episodes > 0 {
		score += candidate.Episodes / 4
	}
	if candidate.Error != "" {
		score -= 10
	}
	return score
}

var errCrossSourceNoTitle = &crossSourceError{"缺少剧名，无法换源搜索"}

type crossSourceError struct{ message string }

func (e *crossSourceError) Error() string { return e.message }

// crossSourceCached 只在内存缓存里按归一化标题找同名剧，不发起任何请求。
func (engine *nativeEngine) crossSourceCached(input nativeInput, title string) crossSourceResult {
	key := normalizeAggregateTitle(title)
	result := crossSourceResult{Query: title, Title: title, Errors: map[string]string{}}
	if input.Drama.Episodes > 0 {
		result.Episodes = input.Drama.Episodes
	}
	if input.Drama.VIP != nil && *input.Drama.VIP {
		result.VIPEpisodes = 1
	}
	if key == "" {
		return result
	}
	engine.mu.Lock()
	snapshot := make(map[string][]nativeDrama, len(engine.catalogs))
	for source, items := range engine.catalogs {
		snapshot[source] = append([]nativeDrama{}, items...)
	}
	engine.mu.Unlock()
	for source, items := range snapshot {
		for _, item := range items {
			if item.ID == input.Drama.ID {
				continue
			}
			if normalizeAggregateTitle(item.Title) != key {
				continue
			}
			candidate := crossSourceCandidate{
				ID: item.ID, Source: source, SourceID: item.SourceID,
				Title: item.Title, Cover: item.Cover,
				Episodes: item.Episodes, Playable: item.Episodes > 0,
			}
			candidate.Score = crossSourceScore(candidate, result.Episodes, result.VIPEpisodes)
			result.Candidates = append(result.Candidates, candidate)
		}
	}
	sort.SliceStable(result.Candidates, func(i, j int) bool {
		if result.Candidates[i].Score != result.Candidates[j].Score {
			return result.Candidates[i].Score > result.Candidates[j].Score
		}
		return result.Candidates[i].Episodes > result.Candidates[j].Episodes
	})
	return result
}
