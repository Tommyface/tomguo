package core

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"
)

type providerMedia struct {
	credentials *providerMediaCredentials
	URL         string
	Referer     string
	Duration    time.Duration
	Playlist    string
	HLSKey      []byte
	CENCKey     []byte
	Quality     int
	Variants    []providerMedia
}

func (d *Downloader) providerBaseURL(source string) string {
	source = canonicalProviderSource(source)
	// 运行时注入的自定义源：base 取配置，不参与内置源的编译期 switch。
	if isCustomSource(source) && d.customSources != nil {
		if cfg, ok := d.customSources.get(source); ok {
			return strings.TrimRight(cfg.BaseURL, "/")
		}
	}
	var configured, fallback string
	switch source {
	case sourceHuangguoAI:
		configured, fallback = d.cfg.HuangguoAIURL, huangguoAIBaseURL
	case sourceHuangguoVideo:
		configured, fallback = d.cfg.HuangguoVideoURL, huangguoVideoBaseURL
	case sourceHuangdou:
		configured, fallback = d.cfg.HuangdouURL, huangdouBaseURL
	case sourceHongguo:
		configured, fallback = d.cfg.HongguoURL, hongguoBaseURL
	case sourceHuangju:
		configured, fallback = d.cfg.HuangjuURL, huangjuBaseURL
	case sourceYeguo:
		configured, fallback = d.cfg.YeguoURL, yeguoBaseURL
	case sourceDSD:
		configured, fallback = d.cfg.DSDURL, dsdBaseURL
	case sourceSorani:
		configured, fallback = d.cfg.SoraniURL, soraniSiteBaseURL
	case sourceGuipian:
		configured, fallback = d.cfg.GuipianURL, guipianSiteBaseURL
	case sourceHanxiaoquan:
		configured, fallback = d.cfg.HanxiaoquanURL, hanxiaoquanSiteBaseURL
	// 参考 zhenguojianTV v0.8.31 新增站源（无独立配置项，用内置 baseURL）。
	case sourceDuanju51:
		fallback = duanju51BaseURL
	case sourceEZDMW:
		fallback = ezdmwBaseURL
	case sourceHema:
		fallback = hemaSiteBaseURL
	case sourceChengguo:
		configured, fallback = d.cfg.ChengguoURL, chengguoBaseURL
	case sourceHuakui:
		fallback = huakuiSiteBaseURL
	case sourceHuangguoMain:
		fallback = "https://falpius.com"
	case sourceHuangguoSelected, sourceHuangguoWelfare:
		fallback = portalOrigin(source)
	case sourceHuaguo:
		fallback = huaguoBaseURL
	case sourceWangguo:
		fallback = wangguoBaseURL
	case sourceFaguo:
		fallback = faguoBaseURL
	case sourceWuguo:
		fallback = wuguoBaseURL
	case sourceMxdm:
		fallback = mxdmBaseURL
	case sourceJcydm:
		fallback = jcydmBaseURL
	default:
		fallback = "https://d2pypzndaqisk.cloudfront.net"
	}
	return strings.TrimRight(firstNonEmpty(configured, fallback), "/")
}

func providerSourceForURL(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	host := strings.ToLower(parsed.Hostname())
	switch {
	case host == "huangguoai.com" || host == "huangguoai.io" || strings.HasSuffix(host, ".ediayikma.cc") || strings.HasSuffix(host, ".agdkczeyx.cc"):
		return sourceHuangguoAI
	case host == "huangguo.video":
		return sourceHuangguoVideo
	case host == "tideember.cc" || host == "xqjurgek.top":
		return sourceHuangdou
	case host == "hongguoduanju.com" || host == "www.hongguoduanju.com":
		return sourceHongguo
	case host == "huangju.net" || host == "www.huangju.net" || host == "api.huangju.net":
		return sourceHuangju
	case host == "ygdj7.com" || host == "www.ygdj7.com" ||
		host == "analyze.buxefaex.cc" || strings.HasSuffix(host, ".buxefaex.cc") ||
		strings.HasSuffix(host, ".fzchosdi.cc") ||
		host == "delta.ygrwdsgt.cc" || host == "yeguodj.com" || host == "www.yeguodj.com":
		return sourceYeguo
	case host == "dsd.com.se" || host == "www.dsd.com.se":
		return sourceDSD
	case host == "sorani.net" || host == "www.sorani.net" || host == "api.sorani.cc" || host == "sorani.cc":
		return sourceSorani
	case host == "guipianwu.com" || host == "www.guipianwu.com":
		return sourceGuipian
	case host == "jennyhow.com" || host == "www.jennyhow.com":
		return sourceHanxiaoquan
	// 参考 zhenguojianTV v0.8.31 新增站源。
	case duanju51MirrorHost(host):
		return sourceDuanju51
	case ezdmwMirrorHost(host):
		return sourceEZDMW
	case host == "kuaikaw.cn" || host == "www.kuaikaw.cn":
		return sourceHema
	case host == "chengguodj.com" || host == "www.chengguodj.com" || host == "api2.chengguodj.com" || host == "12.cyjoifcwr.cc":
		return sourceChengguo
	case host == "huakui.net" || host == "www.huakui.net" || host == "huakui1.com" || host == "huakui2.com" || host == "huakui3.com":
		return sourceHuakui
	case host == "falpius.com" || host == "whmxun.com" || host == "hgdhy11.com":
		return sourceHuangguoMain
	case host == "d2teten6pcf3ig.cloudfront.net":
		return sourceHuangguoSelected
	case host == "d24dalcptr65cw.cloudfront.net":
		return sourceHuangguoWelfare
	case maccmsMirrorHost(host):
		return maccmsSourceForHost(host)
	default:
		// 远程镜像配置里登记的备用域名。
		return providerSourceForHost(host)
	}
}

func (d *Downloader) providerURLCandidates(raw string) []string {
	source := providerSourceForURL(raw)
	if source == "" || source == sourceHuangju || source == sourceYeguo {
		return []string{raw}
	}
	parsed, _ := url.Parse(raw)
	d.providerMu.Lock()
	preferred := d.providerHosts[source]
	d.providerMu.Unlock()
	var candidates []string
	seen := map[string]bool{}
	add := func(candidate string) {
		if candidate != "" && !seen[candidate] {
			seen[candidate] = true
			candidates = append(candidates, candidate)
		}
	}
	add(rehostProviderURL(parsed, preferred))
	configured := d.providerBaseURL(source)
	add(rehostProviderURL(parsed, configured))
	if providerSourceForURL(configured) == "" {
		return candidates
	}
	for _, candidate := range providerURLCandidates(raw) {
		add(candidate)
	}
	return candidates
}

func (d *Downloader) resolveProviderMedia(ctx context.Context, task Task) (providerMedia, error) {
	chapter := task.Chapter
	chapter.Source = canonicalProviderSource(chapter.Source)
	if chapter.Source == "" {
		chapter.Source = sourceFromDramaID(task.DramaID)
	}
	if moduleSource(chapter.Source) != nil {
		return d.moduleResolveMedia(ctx, chapter.Source, task)
	}

	var media providerMedia
	if resolver, ok := builtinMediaTable[chapter.Source]; ok {
		handled, workerDone, err := resolver(d, ctx, &chapter, task, &media)
		if err != nil {
			return providerMedia{}, err
		}
		if handled {
			if workerDone {
				return media, nil
			}
			return d.finishProviderMedia(ctx, chapter, task, media)
		}
	}
	if isCustomSource(chapter.Source) && d.customSources != nil {
		if _, ok := d.customSources.get(chapter.Source); !ok {
			return providerMedia{}, fmt.Errorf("自定义站源配置已失效，请重新添加")
		}
		if strings.HasPrefix(chapter.VideoURL, customAESPlayScheme) {
			result, err := d.resolveCustomAESPlayback(ctx, task)
			if err != nil {
				return providerMedia{}, err
			}
			return d.finishProviderMedia(ctx, chapter, task, result)
		}
	}
	if strings.HasPrefix(chapter.VideoURL, "maccms-play://") {
		if _, found := d.maccmsProfileForDynamic(chapter.Source); found {
			result, err := d.resolveMaccmsPlayback(ctx, task)
			if err != nil {
				return providerMedia{}, err
			}
			return d.finishProviderMedia(ctx, chapter, task, result)
		}
	}

	if refreshStaleChapterSources[chapter.Source] && chapter.PageURL == "" {
		if source, sourceID, valid := splitProviderDramaID(task.DramaID); valid {
			_, chapters, err := d.GetHuangguoChapters(ctx, source, sourceID)
			if err != nil {
				return providerMedia{}, fmt.Errorf("刷新旧任务播放地址失败: %w", err)
			}
			matched := false
			for _, fresh := range chapters {
				if fresh.ID == chapter.ID {
					chapter = fresh
					matched = true
					break
				}
			}
			if !matched {
				return providerMedia{}, fmt.Errorf("原章节已变化，请更新该合集后重新下载")
			}
		}
	}

	media = providerMedia{URL: chapter.VideoURL, Referer: firstNonEmpty(chapter.Referer, d.providerBaseURL(chapter.Source)+"/")}

	workerDone := false
	if huangguoWorkerSources[chapter.Source] {
		if _, sourceID, ok := splitProviderDramaID(task.DramaID); ok {
			workerCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
			if workerMedia, workerErr := d.resolveHuangguoWorkerPlayback(workerCtx, sourceID, huangguoWorkerSequence(chapter, task.Index)); workerErr == nil {
				media = workerMedia
				workerDone = true
			}
			cancel()
		}
	}

	if !workerDone && refreshStaleChapterSources[chapter.Source] && chapter.PageURL != "" {
		responses := &playbackResponseURLs{}
		pageContext := context.WithValue(ctx, playbackResponseURLsKey{}, responses)
		body, err := d.fetchProviderText(pageContext, chapter.PageURL, media.Referer)
		if err != nil {
			return providerMedia{}, err
		}
		pageURL := chapter.PageURL
		if actual, ok := responses.values.Load(chapter.PageURL); ok {
			pageURL = actual.(string)
		}
		if parseMediaFromPage[chapter.Source] != nil {
			media.URL = parseMediaFromPage[chapter.Source](body, pageURL)
		}
		media.Referer = pageURL
	}

	if !workerDone && !isProviderHTTPMediaURL(media.URL) && huangguoWorkerSources[chapter.Source] {
		if _, sourceID, ok := splitProviderDramaID(task.DramaID); ok {
			workerCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 25*time.Second)
			workerMedia, workerErr := d.resolveHuangguoWorkerPlayback(workerCtx, sourceID, huangguoWorkerSequence(chapter, task.Index))
			cancel()
			if workerErr == nil {
				media = workerMedia
			} else {
				d.recordDiagnostic(diagnosticEvent{Level: "warn", Event: "huangguo-worker", Source: chapter.Source, DramaID: task.DramaID, Message: workerErr.Error()})
			}
		}
	}

	return d.finishProviderMedia(ctx, chapter, task, media)
}

func (d *Downloader) finishProviderMedia(ctx context.Context, chapter Chapter, task Task, media providerMedia) (providerMedia, error) {
	if !isProviderHTTPMediaURL(media.URL) {
		return providerMedia{}, fmt.Errorf("%s 未返回有效播放地址，请刷新章节或确认站点访问权限", chapter.Source)
	}
	parsed, _ := url.Parse(media.URL)
	if strings.HasSuffix(strings.ToLower(parsed.Path), ".m3u8") || (chapterNeedsPlaylistProbe[chapter.Source] && ezdmwPlaylistURL(media.URL)) {
		playlist, finalURL, err := d.fetchMediaPlaylist(ctx, media.URL, media.Referer)
		if err != nil {
			return providerMedia{}, fmt.Errorf("获取播放列表失败: %w", err)
		}
		stats := inspectPlaylist(playlist)
		if !stats.ValidM3U8 {
			return providerMedia{}, fmt.Errorf("站点未返回有效 M3U8，可能需要登录或链接已失效")
		}
		if stats.Duration > 0 {
			media.Duration = stats.Duration
		}
		if looksLikePreviewFragment(stats) {
			d.recordDiagnostic(diagnosticEvent{
				Event:   "playlist_preview_suspected",
				Source:  chapter.Source,
				Episode: task.Index + 1,
				Message: fmt.Sprintf("播放列表疑似试看/占位片段：%d 个分片，总时长 %s", stats.Segments, stats.Duration),
			})
		}
		media.Playlist = playlist
		media.URL = finalURL
	}
	return media, nil
}
