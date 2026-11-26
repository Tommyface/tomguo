package core

import (
	"context"
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
var builtinCatalogTable = map[string]builtinCatalogFunc{}


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
