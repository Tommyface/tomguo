package core

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// builtinCatalogOutcome 是内置源目录抓取的统一返回。
//
// 在此之前，nativeCatalog 里每个源都在就地改 result 的字段（hongguo 还会塞 hongguo 状态、
// 搜索分支会改 Page 与 LocalSearch），分支之间互相看不见。把返回值收成一个结构体之后，
// 每个源的差异都表达在自己那一项里，主流程只剩「取函数 → 调用 → 合并结果」。
type builtinCatalogOutcome struct {
	Items       []Drama
	HasMore     bool
	Warning     string
	Page        int
	LocalSearch bool
	Hongguo     *hongguoCatalogState
}

// builtinCatalogFunc 是内置源目录抓取的统一签名。
// query 非空表示这是搜索请求；内置源要么自己处理，要么交给调用方回落本地搜索。
type builtinCatalogFunc func(engine *nativeEngine, d *Downloader, ctx context.Context, page int, category, query string) (builtinCatalogOutcome, error)

// catalogFetchFunc 是「已经统一签名」的内置抓取函数。
type catalogFetchFunc func(d *Downloader, ctx context.Context, page int, category, query string) ([]Drama, bool, error)

// simpleCatalog 适配不需要额外上下文的源：表里一行就能描述一个源。
func simpleCatalog(fetch catalogFetchFunc) builtinCatalogFunc {
	return func(engine *nativeEngine, d *Downloader, ctx context.Context, page int, category, query string) (builtinCatalogOutcome, error) {
		items, more, err := fetch(d, ctx, page, category, query)
		return builtinCatalogOutcome{Items: items, HasMore: more}, err
	}
}

// builtinCatalogTable 是内置源目录抓取的唯一分发表。
//
// 这张表取代了 nativeCatalog 里的两个 26 分支 switch：
// 一个用于搜索（query 非空），一个用于分类浏览。两者签名相同，所以只需要一张表。
var builtinCatalogTable = map[string]builtinCatalogFunc{
	sourceHongguo: func(engine *nativeEngine, d *Downloader, ctx context.Context, page int, category, query string) (builtinCatalogOutcome, error) {
		if query != "" {
			entry, err := d.searchHongguoDramas(ctx, query)
			if err != nil {
				return builtinCatalogOutcome{}, err
			}
			out := builtinCatalogOutcome{Items: entry.Dramas, Warning: entry.Warning, Page: 1}
			if entry.Limited && out.Warning == "" {
				out.Warning = "已显示当前可获取的匹配结果，使用更完整的剧名可继续查找"
			}
			return out, nil
		}
		engine.mu.Lock()
		known := make(map[string]bool, len(engine.catalogs[category]))
		for _, drama := range engine.catalogs[category] {
			known[drama.ID] = true
		}
		engine.mu.Unlock()
		ctx = context.WithValue(ctx, libraryKnownKey{}, known)
		if page > 1 {
			ctx = context.WithValue(ctx, libraryMoreKey{}, true)
		}
		items, err := d.fetchHongguoAppCatalogCategory(ctx, category)
		state := d.hongguoCatalogSnapshot()
		out := builtinCatalogOutcome{Items: items, Hongguo: state, HasMore: hongguoCatalogHasMore(state)}
		if category != "" && state != nil {
			out.HasMore = !state.Feeds["category:"+category].Exhausted
		}
		if len(items) == 0 && err != nil && ctx.Err() == nil && (category == "" || category == "short_play") {
			var totalPages int
			items, totalPages, err = d.fetchHongguoCategoryPage(ctx, "real-drama?page="+strconv.Itoa(page), "真人剧")
			out.Items = items
			out.HasMore = page < totalPages
		}
		return out, err
	},

	sourceHuangdou: func(engine *nativeEngine, d *Downloader, ctx context.Context, page int, category, query string) (builtinCatalogOutcome, error) {
		client := newHuangdouAPIClient(d)
		var decoded any
		err := client.call(ctx, "/drama/list", map[string]any{"page": strconv.Itoa(page), "page_size": "30"}, &decoded)
		if err != nil {
			return builtinCatalogOutcome{}, err
		}
		rows := huangdouList(decoded)
		out := builtinCatalogOutcome{HasMore: len(rows) >= 30}
		for _, row := range rows {
			if drama := huangdouDramaFromMap(row); drama.ID != "" {
				out.Items = append(out.Items, drama)
			}
		}
		return out, nil
	},

	sourceHuangguoAI: simpleCatalog(func(d *Downloader, ctx context.Context, page int, category, query string) ([]Drama, bool, error) {
		return d.fetchHuangguoAICatalogPage(ctx, page, category)
	}),

	sourceHuangju: simpleCatalog(func(d *Downloader, ctx context.Context, page int, category, query string) ([]Drama, bool, error) {
		return d.fetchHuangjuCatalogPage(ctx, page, category, query)
	}),

	sourceYeguo: simpleCatalog(func(d *Downloader, ctx context.Context, page int, category, query string) ([]Drama, bool, error) {
		return d.fetchYeguoCatalogPage(ctx, page, category, query)
	}),

	sourceDSD: simpleCatalog(func(d *Downloader, ctx context.Context, page int, category, query string) ([]Drama, bool, error) {
		return d.fetchDSDCatalogPage(ctx, page, category, query)
	}),

	sourceHanxiaoquan: simpleCatalog(func(d *Downloader, ctx context.Context, page int, category, query string) ([]Drama, bool, error) {
		return d.fetchHanxiaoquanCatalogPage(ctx, page, category, query)
	}),

	sourceDuanju51: simpleCatalog(func(d *Downloader, ctx context.Context, page int, category, query string) ([]Drama, bool, error) {
		return d.fetchDuanju51CatalogPage(ctx, page, category, query)
	}),

	sourceHema: simpleCatalog(func(d *Downloader, ctx context.Context, page int, category, query string) ([]Drama, bool, error) {
		return d.fetchHemaCatalogPage(ctx, page, category, query)
	}),

	sourceChengguo: simpleCatalog(func(d *Downloader, ctx context.Context, page int, category, query string) ([]Drama, bool, error) {
		return d.fetchChengguoCatalogPage(ctx, page, category, query)
	}),

	// 黄果主站只有一页：翻页请求直接返回空，由 AllowEmptyCatalog 决定是否报错。
	sourceHuangguoMain: func(engine *nativeEngine, d *Downloader, ctx context.Context, page int, category, query string) (builtinCatalogOutcome, error) {
		if page > 1 || query != "" {
			return builtinCatalogOutcome{}, nil
		}
		items, err := d.fetchHuangguoWebCatalog(ctx)
		return builtinCatalogOutcome{Items: items}, err
	},

	sourceHuangguoSelected: portalCatalogLoader(sourceHuangguoSelected),
	sourceHuangguoWelfare:  portalCatalogLoader(sourceHuangguoWelfare),

	sourceHuaguo:  malformedMaccms(sourceHuaguo),
	sourceWangguo: malformedMaccms(sourceWangguo),
	sourceFaguo:   malformedMaccms(sourceFaguo),
	sourceWuguo:   malformedMaccms(sourceWuguo),
	sourceMxdm:    malformedMaccms(sourceMxdm),
	sourceJcydm:   malformedMaccms(sourceJcydm),

	sourceHuangguoVideo: func(engine *nativeEngine, d *Downloader, ctx context.Context, page int, category, query string) (builtinCatalogOutcome, error) {
		address := fmt.Sprintf("%s/videos?page=%d", d.providerBaseURL(sourceHuangguoVideo), page)
		if category != "" {
			address += "&category=" + url.QueryEscape(category)
		}
		body, err := d.fetchProviderText(ctx, address, d.providerBaseURL(sourceHuangguoVideo)+"/")
		if err != nil {
			return builtinCatalogOutcome{}, err
		}
		items := parseHuangguoVideoCards(body, address)
		return builtinCatalogOutcome{Items: items, HasMore: len(items) >= 20}, nil
	},

	sourceCloudFront: func(engine *nativeEngine, d *Downloader, ctx context.Context, page int, category, query string) (builtinCatalogOutcome, error) {
		items, more, err := d.fetchLegacyCatalogCategoryPage(ctx, page, category)
		return builtinCatalogOutcome{Items: items, HasMore: more}, err
	},

	// 黄果官网：当前内置实现只提供详情与播放，目录无独立入口。
	// 在 S2 之后这张表会被云端清单取代；此处显式占位，保证每张表条目数一致。
	sourceHuangguoOfficial: func(engine *nativeEngine, d *Downloader, ctx context.Context, page int, category, query string) (builtinCatalogOutcome, error) {
		return builtinCatalogOutcome{Items: nil, HasMore: false}, nil
	},
}

func portalCatalogLoader(source string) builtinCatalogFunc {
	return func(engine *nativeEngine, d *Downloader, ctx context.Context, page int, category, query string) (builtinCatalogOutcome, error) {
		if page > 1 || query != "" {
			return builtinCatalogOutcome{}, nil
		}
		items, err := d.fetchHuangguoPortalCatalog(ctx, source)
		return builtinCatalogOutcome{Items: items}, err
	}
}

func malformedMaccms(source string) builtinCatalogFunc {
	return func(engine *nativeEngine, d *Downloader, ctx context.Context, page int, category, query string) (builtinCatalogOutcome, error) {
		items, more, err := d.fetchMaccmsCatalogPage(ctx, source, page, query)
		return builtinCatalogOutcome{Items: items, HasMore: more}, err
	}
}

// runBuiltinCatalog 按源查表执行。第二个返回值表示该源是否有内置实现。
func runBuiltinCatalog(engine *nativeEngine, d *Downloader, ctx context.Context, source string, page int, category, query string) (builtinCatalogOutcome, bool, error) {
	fetch, ok := builtinCatalogTable[source]
	if !ok {
		return builtinCatalogOutcome{}, false, nil
	}
	out, err := fetch(engine, d, ctx, page, category, query)
	return out, true, err
}

// builtinCatalogKnown 报告某源是否有内置的目录实现，用于分发前判定。
func builtinCatalogKnown(source string) bool {
	_, ok := builtinCatalogTable[strings.TrimSpace(source)]
	return ok
}
