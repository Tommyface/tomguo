package core

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"
)

const rankingCacheTTL = 5 * time.Minute

type rankingBoard struct {
	ID          string `json:"id"`
	Source      string `json:"source"`
	Name        string `json:"name"`
	Description string `json:"description"`
	path        string
	upstreamKey string
}

var rankingBoards = []rankingBoard{
	{ID: "hongguo-hot", Source: sourceHongguo, Name: "总热播榜", Description: "红果观看、互动等综合热度；每日更新。", path: "hot-drama", upstreamKey: "hongguo"},
	{ID: "hongguo-real", Source: sourceHongguo, Name: "真人剧榜", Description: "红果真人剧热播榜；每日更新。", path: "hot-real-drama", upstreamKey: "real"},
	{ID: "hongguo-comic", Source: sourceHongguo, Name: "漫剧榜", Description: "红果漫剧热播榜；每日更新。", path: "hot-comic-drama", upstreamKey: "comic"},
	{ID: "hongguo-ai", Source: sourceHongguo, Name: "AI剧榜", Description: "红果 AI 剧热播榜；每日更新。", path: "hot-ai-drama", upstreamKey: "ai"},
	{ID: "huangdou-all", Source: sourceHuangdou, Name: "总榜", Description: "黄豆短剧总榜，保留站点返回的顺序。", upstreamKey: "all"},
	{ID: "huangdou-mogai", Source: sourceHuangdou, Name: "魔改榜", Description: "黄豆站点魔改榜。", upstreamKey: "mogai"},
	{ID: "huangdou-search", Source: sourceHuangdou, Name: "搜索榜", Description: "黄豆站点搜索榜。", upstreamKey: "search"},
	{ID: "huangdou-favorite", Source: sourceHuangdou, Name: "收藏榜", Description: "黄豆站点收藏榜。", upstreamKey: "favorite"},
	{ID: "huangdou-finish", Source: sourceHuangdou, Name: "完结榜", Description: "黄豆站点完结榜。", upstreamKey: "finish"},
	{ID: "huangguo-hot", Source: sourceHuangguoAI, Name: "热播榜", Description: "黄果站点热播 TOP 20；每日更新。", path: "hot"},
	{ID: "huangguo-recommend", Source: sourceHuangguoAI, Name: "推荐榜", Description: "黄果站点推荐 TOP 20。", path: "recommend"},
	{ID: "huangguo-potential", Source: sourceHuangguoAI, Name: "潜力榜", Description: "黄果站点潜力 TOP 20。", path: "potential"},
	{ID: "yeguo-recommend", Source: sourceYeguo, Name: "目录榜", Description: "野果默认目录顺序，按源站返回顺序展示。"},
	{ID: "dsd-catalog", Source: sourceDSD, Name: "目录榜", Description: "帝果分类目录聚合顺序，按源站返回顺序展示。"},
	// 参考 zhenguojianTV v0.8.31：51短剧改版后的三张单页榜。
}

type rankingItem struct {
	Rank   int    `json:"rank"`
	Drama  Drama  `json:"drama"`
	Metric string `json:"metric,omitempty"`
}

type rankingPage struct {
	BoardID     string        `json:"boardId"`
	Page        int           `json:"page"`
	Items       []rankingItem `json:"items"`
	HasMore     bool          `json:"hasMore"`
	TotalPages  int           `json:"totalPages,omitempty"`
	UpdatedText string        `json:"updatedText,omitempty"`
	FetchedAt   time.Time     `json:"fetchedAt"`
	Stale       bool          `json:"stale,omitempty"`
	Warning     string        `json:"warning,omitempty"`
}

type rankingCall struct {
	done chan struct{}
	page rankingPage
	err  error
}

type rankingCache struct {
	mu      sync.Mutex
	pages   map[string]rankingPage
	pending map[string]*rankingCall
}

func findRankingBoard(id string) (rankingBoard, bool) {
	for _, board := range rankingBoards {
		if board.ID == id {
			return board, true
		}
	}
	return rankingBoard{}, false
}

func cloneRankingPage(page rankingPage) rankingPage {
	page.Items = append([]rankingItem{}, page.Items...)
	return page
}

func singlePageRankingBoard(board rankingBoard) bool {
	return board.Source == sourceHuangguoAI || board.Source == sourceDSD
}

// nativeRankingBoardSupported 判断该榜单的站源在当前构建里有没有真实抓取实现。
// 没有实现的榜单项不能在 UI 里出现，否则点了只会报错。
//
// S7 之后内置榜单源（hongguo/huangdou/huangju）的 Go 实现已全部移除，
// 只有同名模块接管时才算有实现；模块自带榜单分类的板子由
// appendModuleRankingBoards 另行生成。
func nativeRankingBoardSupported(board rankingBoard) bool {
	return moduleTakesOver(board.Source)
}

func rankingDramaWithoutImages(drama Drama) Drama {
	drama.Cover, drama.CoverURL, drama.CoverURLSnake = nil, nil, nil
	drama.Image, drama.ImageURL, drama.ImageURLSnake = nil, nil, nil
	drama.Img, drama.Pic, drama.Picture = nil, nil, nil
	drama.Poster, drama.Thumb, drama.Thumbnail = nil, nil, nil
	return drama
}

func (d *Downloader) loadRankingPage(ctx context.Context, board rankingBoard, page int, refresh bool) (rankingPage, error) {
	key := board.ID + ":" + strconv.Itoa(page)
	cache := &d.rankings
	cache.mu.Lock()
	if cache.pages == nil {
		cache.pages = map[string]rankingPage{}
		cache.pending = map[string]*rankingCall{}
	}
	cached, exists := cache.pages[key]
	if exists && !refresh && time.Since(cached.FetchedAt) >= 0 && time.Since(cached.FetchedAt) < rankingCacheTTL {
		cache.mu.Unlock()
		return cloneRankingPage(cached), nil
	}
	if pending := cache.pending[key]; pending != nil {
		cache.mu.Unlock()
		select {
		case <-ctx.Done():
			return rankingPage{}, ctx.Err()
		case <-pending.done:
			if (errors.Is(pending.err, context.Canceled) || errors.Is(pending.err, context.DeadlineExceeded)) && ctx.Err() == nil {
				return d.loadRankingPage(ctx, board, page, refresh)
			}
			return cloneRankingPage(pending.page), pending.err
		}
	}
	pending := &rankingCall{done: make(chan struct{})}
	cache.pending[key] = pending
	cache.mu.Unlock()

	result, err := d.fetchRankingPage(ctx, board, page)
	cache.mu.Lock()
	defer cache.mu.Unlock()
	if err == nil {
		result.BoardID, result.Page, result.FetchedAt = board.ID, page, time.Now()
		if refresh && page == 1 {
			for cachedKey, old := range cache.pages {
				if old.BoardID == board.ID {
					delete(cache.pages, cachedKey)
				}
			}
		}
		if len(cache.pages) >= 128 {
			oldestKey, oldest := "", time.Now()
			for cachedKey, old := range cache.pages {
				if old.FetchedAt.Before(oldest) {
					oldestKey, oldest = cachedKey, old.FetchedAt
				}
			}
			delete(cache.pages, oldestKey)
		}
		cache.pages[key] = cloneRankingPage(result)
		d.writeRankingCacheLocked()
	} else if exists && ctx.Err() == nil && time.Since(cached.FetchedAt) >= 0 && time.Since(cached.FetchedAt) < 24*time.Hour {
		result = cloneRankingPage(cached)
		result.Stale = true
		result.Warning = "站点暂不可用，显示上次取得的榜单，请稍后刷新。"
		err = nil
	}
	pending.page, pending.err = cloneRankingPage(result), err
	delete(cache.pending, key)
	close(pending.done)
	return result, err
}

func (d *Downloader) fetchRankingPage(ctx context.Context, board rankingBoard, page int) (rankingPage, error) {
	if moduleTakesOver(board.Source) {
		return d.fetchModuleRankingPage(ctx, board, page)
	}
	switch board.Source {
	case sourceHongguo:
		return d.fetchHongguoRankingPage(ctx, board, page)
	case sourceHuangdou:
		var decoded any
		err := newHuangdouAPIClient(d).call(ctx, "/drama/rank", map[string]any{"tab": board.upstreamKey, "page": strconv.Itoa(page)}, &decoded)
		if err != nil {
			return rankingPage{}, err
		}
		return parseHuangdouRanking(decoded, page)
	case sourceHuangguoAI:
		if page != 1 {
			return rankingPage{}, errors.New("该榜单只有一页")
		}
		pageURL := d.providerBaseURL(sourceHuangguoAI) + "/ranks/" + board.path + "/"
		body, err := d.fetchProviderText(ctx, pageURL, d.providerBaseURL(sourceHuangguoAI)+"/")
		if err != nil {
			return rankingPage{}, err
		}
		return parseHuangguoRanking(body, board)
	case sourceHuangju, sourceYeguo, sourceDSD:
		return d.fetchCatalogRankingPage(ctx, board, page)
	default:
		return rankingPage{}, fmt.Errorf("不支持的榜单站源 %s", board.Source)
	}
}

// fetchModuleRankingPage 用模块的目录能力实现榜单：模块分类里带「榜」的
// 分类会被登记为榜单板，这里按分类名取目录第一页作为榜单内容。
func (d *Downloader) fetchModuleRankingPage(ctx context.Context, board rankingBoard, page int) (rankingPage, error) {
	if page != 1 {
		return rankingPage{}, errors.New("该榜单只有一页")
	}
	category := board.path
	if category == "" {
		return rankingPage{}, errors.New("榜单分类缺失")
	}
	items, _, err := d.moduleCatalog(ctx, board.Source, 1, category, "")
	if err != nil {
		return rankingPage{}, err
	}
	out := rankingPage{BoardID: board.ID, Page: 1}
	for index, drama := range items {
		out.Items = append(out.Items, rankingItem{Rank: index + 1, Drama: drama})
	}
	out.TotalPages = 1
	return out, nil
}

// appendModuleRankingBoards 把模块里带「榜」的分类登记成榜单板。
//
// 内置榜单源在 S7 已全部移除，硬编码白名单会让榜单永远为空。模块只要
// 暴露了榜单分类（如 51短剧的「热播榜」），就直接变成可点的榜单板，
// 数据走 moduleCatalog，不依赖任何 Go 侧站点实现。
func (engine *nativeEngine) appendModuleRankingBoards(boards []rankingBoard) []rankingBoard {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	return engine.appendModuleRankingBoardsWith(ctx, boards)
}

func (engine *nativeEngine) appendModuleRankingBoardsWith(ctx context.Context, boards []rankingBoard) []rankingBoard {
	seen := make(map[string]bool, len(boards))
	for _, board := range boards {
		seen[board.ID] = true
	}
	for _, manifest := range ModuleManifests() {
		source := canonicalProviderSource(manifest.ID)
		if source == "" || !moduleTakesOver(source) || isRetiredSource(source) {
			continue
		}
		categories, err := engine.downloader.moduleCategories(ctx, source, false)
		if err != nil {
			continue
		}
		for _, category := range categories {
			if category.ID == "" || !strings.Contains(category.Name, "榜") {
				continue
			}
			id := source + "-" + category.ID
			if seen[id] {
				continue
			}
			seen[id] = true
			boards = append(boards, rankingBoard{
				ID: id, Source: source, Name: category.Name,
				Description: manifest.Name + "站点「" + category.Name + "」实时顺序。",
				path:        category.ID,
			})
		}
	}
	return boards
}

// moduleRankingBoardSource 从模块榜单板 ID（<source>-<category>）解析出站源。
func moduleRankingBoardSource(boardID string) (string, bool) {
	for _, manifest := range ModuleManifests() {
		source := canonicalProviderSource(manifest.ID)
		if source == "" {
			continue
		}
		if strings.HasPrefix(boardID, source+"-") && len(boardID) > len(source)+1 {
			return source, true
		}
	}
	return "", false
}

func (d *Downloader) fetchCatalogRankingPage(ctx context.Context, board rankingBoard, page int) (rankingPage, error) {
	if singlePageRankingBoard(board) && page != 1 {
		return rankingPage{}, errors.New("该榜单只有一页")
	}
	category := board.path
	if board.Source == sourceYeguo && category == "recommend" {
		category = ""
		categories, err := d.fetchYeguoCategories(ctx)
		if err == nil {
			for _, item := range categories {
				if strings.HasPrefix(item.ID, "recommend:") {
					category = item.ID
					break
				}
			}
		}
	}
	var items []Drama
	var more bool
	var err error
	switch board.Source {
	case sourceYeguo:
		items, more, err = d.fetchYeguoCatalogPage(ctx, page, category, "")
	case sourceDSD:
		items, more, err = d.fetchDSDCatalogPage(ctx, page, category, "")
	}
	if err != nil {
		return rankingPage{}, err
	}
	if board.Source == sourceDSD {
		more = false
	}
	result := rankingPage{Items: make([]rankingItem, 0, len(items)), HasMore: more}
	for index, drama := range items {
		result.Items = append(result.Items, rankingItem{Rank: (page-1)*20 + index + 1, Drama: rankingDramaWithoutImages(drama)})
	}
	return result, nil
}
