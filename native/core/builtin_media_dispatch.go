package core

import (
	"context"
	"strconv"
	"strings"
)

// builtinMediaFunc 是内置源播放地址解析的统一签名。
//
// 返回值 handled 决定主流程是否接管：
//   - handled=true：该源走自己的解析路径，media 是最终结果（或 err）
//   - handled=false：主流程用 chapter.VideoURL 直接播放，后续再走占位地址补解析
//
// 这样每个源的「要不要特殊处理」由它自己决定，resolveProviderMedia 里不再堆满
// `if chapter.Source == X` 判断。
type builtinMediaFunc func(d *Downloader, ctx context.Context, chapter *Chapter, task Task, media *providerMedia) (handled bool, workerDone bool, err error)

// builtinMediaTable 是内置源播放解析的唯一分发表。
// 表里没有的源走直连兜底；运行时模块与自定义源仍由主流程先拦截。
var builtinMediaTable = map[string]builtinMediaFunc{
	sourceCloudFront: func(d *Downloader, ctx context.Context, chapter *Chapter, task Task, media *providerMedia) (bool, bool, error) {
		result, err := d.resolveLegacyMedia(ctx, task)
		*media = result
		return true, false, err
	},
	sourceHuangju: func(d *Downloader, ctx context.Context, chapter *Chapter, task Task, media *providerMedia) (bool, bool, error) {
		result, err := d.resolveHuangjuMedia(ctx, task)
		*media = result
		return true, false, err
	},
	sourceYeguo: func(d *Downloader, ctx context.Context, chapter *Chapter, task Task, media *providerMedia) (bool, bool, error) {
		result, err := d.resolveYeguoMedia(ctx, task)
		*media = result
		return true, false, err
	},
	sourceDSD: func(d *Downloader, ctx context.Context, chapter *Chapter, task Task, media *providerMedia) (bool, bool, error) {
		result, err := d.resolveDSDMedia(ctx, task)
		*media = result
		return true, false, err
	},
	sourceHanxiaoquan: func(d *Downloader, ctx context.Context, chapter *Chapter, task Task, media *providerMedia) (bool, bool, error) {
		result, err := d.resolveHanxiaoquanMedia(ctx, task)
		*media = result
		return true, false, err
	},
	sourceHuangguoMain: func(d *Downloader, ctx context.Context, chapter *Chapter, task Task, media *providerMedia) (bool, bool, error) {
		result, err := d.resolveHuangguoWebMedia(ctx, task)
		*media = result
		if err == nil {
			return true, true, nil
		}
		return true, false, err
	},
	sourceHuangguoSelected: huangguoPortalMedia(sourceHuangguoSelected),
	sourceHuangguoWelfare:  huangguoPortalMedia(sourceHuangguoWelfare),
	// 红果：只在 chapter 使用 cenc 占位协议时接管，其余情况回落到直连。
	sourceHongguo: func(d *Downloader, ctx context.Context, chapter *Chapter, task Task, media *providerMedia) (bool, bool, error) {
		if !strings.HasPrefix(chapter.VideoURL, "hongguo-cenc://") {
			return false, false, nil
		}
		result, err := d.resolveHongguoMedia(ctx, task)
		*media = result
		return true, false, err
	},
	// 黄豆：详情只存集数，播放时按集号再问一次接口。
	sourceHuangdou: func(d *Downloader, ctx context.Context, chapter *Chapter, task Task, media *providerMedia) (bool, bool, error) {
		_, sourceID, valid := splitProviderDramaID(task.DramaID)
		if !valid {
			return true, false, nil
		}
		sequence, err := strconv.Atoi(chapter.EpisodeString(task.Index))
		if err != nil || sequence < 1 {
			return true, false, errHuangdouInvalidEpisode
		}
		client := newHuangdouAPIClient(d)
		result, err := d.resolveHuangdouPlayback(ctx, client, sourceID, sequence)
		if err != nil {
			return true, false, err
		}
		result.Referer = client.host + "/home"
		*media = result
		return true, false, nil
	},
	sourceHema: placeholderMedia("hema-play://", func(d *Downloader, ctx context.Context, task Task) (providerMedia, error) {
		return d.resolveHemaMedia(ctx, task)
	}),
	sourceDuanju51: placeholderMedia(duanju51PlayScheme, func(d *Downloader, ctx context.Context, task Task) (providerMedia, error) {
		return d.resolveDuanju51Playback(ctx, task)
	}),
}

var errHuangdouInvalidEpisode = &providerGateError{message: "黄豆集数无效"}

type providerGateError struct{ message string }

func (e *providerGateError) Error() string { return e.message }

type mediaResolver func(d *Downloader, ctx context.Context, task Task) (providerMedia, error)

func placeholderMedia(scheme string, resolver mediaResolver) builtinMediaFunc {
	return func(d *Downloader, ctx context.Context, chapter *Chapter, task Task, media *providerMedia) (bool, bool, error) {
		if !strings.HasPrefix(chapter.VideoURL, scheme) {
			// 51短剧还接受带 auth_key 的裸 m3u8；其余源只处理占位协议。
			if scheme == duanju51PlayScheme && isProviderHTTPMediaURL(chapter.VideoURL) && strings.Contains(chapter.VideoURL, "auth_key=") {
				// 继续走下面的补解析。
			} else {
				return false, false, nil
			}
		}
		result, err := resolver(d, ctx, task)
		if err != nil {
			return true, false, err
		}
		*media = result
		return true, false, nil
	}
}

func huangguoPortalMedia(source string) builtinMediaFunc {
	return func(d *Downloader, ctx context.Context, chapter *Chapter, task Task, media *providerMedia) (bool, bool, error) {
		result, err := d.resolveHuangguoPortalMedia(ctx, task)
		*media = result
		if err == nil {
			return true, true, nil
		}
		return true, false, err
	}
}

// huangguoWorkerSources 是需要在播放前尝试 Worker 反代的源。
var huangguoWorkerSources = map[string]bool{
	sourceHuangguoAI:    true,
	sourceHuangguoVideo: true,
	sourceHuangguoMain:  true,
}

// refreshStaleChapterSources 是详情只存电影 URL、需要重新抓章节列表的源。
var refreshStaleChapterSources = map[string]bool{
	sourceHuangguoAI:    true,
	sourceHuangguoVideo: true,
}

// builtinMediaKnown 报告某源是否在播放分发表里。
func builtinMediaKnown(source string) bool {
	_, ok := builtinMediaTable[source]
	return ok
}

// parseMediaFromPage 把「抓页面后如何解析播放地址」也变成查表，
// 避免主流程再按源做 if/else。
var parseMediaFromPage = map[string]func(body, pageURL string) string{
	sourceHuangguoAI:    parseAIVideoURL,
	sourceHuangguoVideo: parseDataHLS,
}

// chapterNeedsPlaylistProbe 标记需要额外识别 EZDMW 伪播放列表地址的源。
var chapterNeedsPlaylistProbe = map[string]bool{
}
