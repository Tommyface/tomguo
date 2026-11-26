package core

import "context"

// builtinDetailOutcome 是内置源详情抓取的统一返回。
//
// 与目录那层一样，每个源的特殊处理都收进结构体字段里：
// 之前 nativeDetail 在 switch 之后还跟着三段 `if source == X` 的补充逻辑，
// 现在这些差异由表项自己声明（Enrich / EpisodesFromRaw），主流程不再认识任何站点。
type builtinDetailOutcome struct {
	Title    string
	Raw      Drama
	Chapters []Chapter
	// EpisodesFromRaw 表示集数应取 max(归一化 raw 的 Episodes)，用于源返回的分集数与详情页不一致的站点。
	EpisodesFromRaw bool
	// Enrich 在通用收尾之前调用，供源补充缓存里的额外信息（封面、简介等）。
	Enrich func(d *Downloader, ctx context.Context, drama *nativeDrama)
}

// builtinDetailFunc 是内置源详情抓取的统一签名。
type builtinDetailFunc func(engine *nativeEngine, d *Downloader, ctx context.Context, source, sourceID string) (builtinDetailOutcome, error)

// detailFetchFunc 是「签名已经统一」的详情抓取函数。
type detailFetchFunc func(d *Downloader, ctx context.Context, sourceID string) (Drama, []Chapter, error)

func simpleDetail(fetch detailFetchFunc) builtinDetailFunc {
	return func(engine *nativeEngine, d *Downloader, ctx context.Context, source, sourceID string) (builtinDetailOutcome, error) {
		raw, chapters, err := fetch(d, ctx, sourceID)
		return builtinDetailOutcome{Raw: raw, Chapters: chapters}, err
	}
}

// sourceDetailFetch 是「带 source 参数」的详情抓取函数。
type sourceDetailFetch func(d *Downloader, ctx context.Context, source, sourceID string) (Drama, []Chapter, error)

func sourceDetail(fetch sourceDetailFetch) builtinDetailFunc {
	return func(engine *nativeEngine, d *Downloader, ctx context.Context, source, sourceID string) (builtinDetailOutcome, error) {
		raw, chapters, err := fetch(d, ctx, source, sourceID)
		return builtinDetailOutcome{Raw: raw, Chapters: chapters}, err
	}
}

func maccmsDetail(source string) builtinDetailFunc {
	return sourceDetail(func(d *Downloader, ctx context.Context, _ string, sourceID string) (Drama, []Chapter, error) {
		return d.fetchMaccmsDetail(ctx, source, sourceID)
	})
}

// builtinDetailTable 是内置源详情抓取的唯一分发表，与 builtinCatalogTable 一一对应。
var builtinDetailTable = map[string]builtinDetailFunc{
	sourceHongguo: func(engine *nativeEngine, d *Downloader, ctx context.Context, source, sourceID string) (builtinDetailOutcome, error) {
		title, chapters, err := d.GetHuangguoChapters(ctx, source, sourceID)
		out := builtinDetailOutcome{Title: title, Chapters: chapters}
		out.Enrich = func(d *Downloader, ctx context.Context, drama *nativeDrama) {
			cached := d.hongguoCachedDrama(Drama{ID: drama.ID, Title: drama.Title, Source: sourceHongguo})
			*drama = mergeNativeDrama(*drama, nativeNormalize(cached))
		}
		return out, err
	},

	sourceHuangdou: func(engine *nativeEngine, d *Downloader, ctx context.Context, source, sourceID string) (builtinDetailOutcome, error) {
		title, chapters, err := d.GetHuangguoChapters(ctx, source, sourceID)
		out := builtinDetailOutcome{Title: title, Chapters: chapters}
		out.Enrich = func(d *Downloader, ctx context.Context, drama *nativeDrama) {
			row, rowErr := d.huangdouDetail(ctx, sourceID)
			if rowErr != nil {
				return
			}
			fresh := nativeNormalize(huangdouDramaFromMap(row))
			if fresh.ID == drama.ID {
				*drama = mergeNativeDrama(*drama, fresh)
			}
		}
		return out, err
	},

	sourceHuangguoAI: simpleDetail(func(d *Downloader, ctx context.Context, sourceID string) (Drama, []Chapter, error) {
		return d.fetchHuangguoAIDetail(ctx, sourceID)
	}),
	sourceHuangguoVideo: simpleDetail(func(d *Downloader, ctx context.Context, sourceID string) (Drama, []Chapter, error) {
		return d.fetchHuangguoVideoDetail(ctx, sourceID)
	}),
	sourceCloudFront: simpleDetail(func(d *Downloader, ctx context.Context, sourceID string) (Drama, []Chapter, error) {
		return d.fetchLegacyDetail(ctx, sourceID)
	}),
	sourceHuangju: simpleDetail(func(d *Downloader, ctx context.Context, sourceID string) (Drama, []Chapter, error) {
		return d.fetchHuangjuDetail(ctx, sourceID)
	}),
	sourceYeguo: simpleDetail(func(d *Downloader, ctx context.Context, sourceID string) (Drama, []Chapter, error) {
		return d.fetchYeguoDetail(ctx, sourceID)
	}),
	sourceDSD: simpleDetail(func(d *Downloader, ctx context.Context, sourceID string) (Drama, []Chapter, error) {
		return d.fetchDSDDetail(ctx, sourceID)
	}),
	sourceHanxiaoquan: simpleDetail(func(d *Downloader, ctx context.Context, sourceID string) (Drama, []Chapter, error) {
		return d.fetchHanxiaoquanDetail(ctx, sourceID)
	}),
	sourceDuanju51: simpleDetail(func(d *Downloader, ctx context.Context, sourceID string) (Drama, []Chapter, error) {
		return d.fetchDuanju51Detail(ctx, sourceID)
	}),
	sourceHema: simpleDetail(func(d *Downloader, ctx context.Context, sourceID string) (Drama, []Chapter, error) {
		return d.fetchHemaDetail(ctx, sourceID)
	}),
	sourceHuangguoMain: simpleDetail(func(d *Downloader, ctx context.Context, sourceID string) (Drama, []Chapter, error) {
		return d.fetchHuangguoWebDetail(ctx, sourceID)
	}),
	sourceHuangguoSelected: sourceDetail(func(d *Downloader, ctx context.Context, source, sourceID string) (Drama, []Chapter, error) {
		return d.fetchHuangguoPortalDetail(ctx, source, sourceID)
	}),
	sourceHuangguoWelfare: sourceDetail(func(d *Downloader, ctx context.Context, source, sourceID string) (Drama, []Chapter, error) {
		return d.fetchHuangguoPortalDetail(ctx, source, sourceID)
	}),
	// 黄果官网在旧实现里没有独立 case，走的是 default 分支的 GetHuangguoChapters。
	sourceHuangguoOfficial: func(engine *nativeEngine, d *Downloader, ctx context.Context, source, sourceID string) (builtinDetailOutcome, error) {
		title, chapters, err := d.GetHuangguoChapters(ctx, source, sourceID)
		return builtinDetailOutcome{Title: title, Chapters: chapters}, err
	},
	sourceHuaguo:  maccmsDetail(sourceHuaguo),
	sourceWangguo: maccmsDetail(sourceWangguo),
	sourceFaguo:   maccmsDetail(sourceFaguo),
	sourceWuguo:   maccmsDetail(sourceWuguo),
	sourceMxdm:    maccmsDetail(sourceMxdm),
	sourceJcydm:   maccmsDetail(sourceJcydm),
}

// 有些源的列表页集数少于详情页，收尾时取两者的较大值。
var detailEpisodesFromRaw = map[string]bool{
	sourceHuangju:  true,
	sourceYeguo:    true,
}

func init() {
	for source := range detailEpisodesFromRaw {
		entry, ok := builtinDetailTable[source]
		if !ok {
			panic("builtinDetailTable 缺少源 " + source)
		}
		builtinDetailTable[source] = withEpisodesFromRaw(entry)
	}
}

func withEpisodesFromRaw(inner builtinDetailFunc) builtinDetailFunc {
	return func(engine *nativeEngine, d *Downloader, ctx context.Context, source, sourceID string) (builtinDetailOutcome, error) {
		out, err := inner(engine, d, ctx, source, sourceID)
		out.EpisodesFromRaw = true
		return out, err
	}
}

// runBuiltinDetail 按源查表执行。第二个返回值表示该源是否有内置实现。
func runBuiltinDetail(engine *nativeEngine, d *Downloader, ctx context.Context, source, sourceID string) (builtinDetailOutcome, bool, error) {
	fetch, ok := builtinDetailTable[source]
	if !ok {
		return builtinDetailOutcome{}, false, nil
	}
	out, err := fetch(engine, d, ctx, source, sourceID)
	return out, true, err
}

// builtinDetailKnown 报告某源是否有内置的详情实现。
func builtinDetailKnown(source string) bool {
	_, ok := builtinDetailTable[source]
	return ok
}
