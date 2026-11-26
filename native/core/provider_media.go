package core

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
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
	case sourceHuangju:
		configured, fallback = d.cfg.HuangjuURL, huangjuBaseURL
	case sourceYeguo:
		configured, fallback = d.cfg.YeguoURL, yeguoBaseURL
	case sourceDSD:
		configured, fallback = d.cfg.DSDURL, dsdBaseURL
	case sourceHanxiaoquan:
		configured, fallback = d.cfg.HanxiaoquanURL, hanxiaoquanSiteBaseURL
	// 参考 zhenguojianTV v0.8.31 新增站源（无独立配置项，用内置 baseURL）。
	case sourceDuanju51:
		fallback = duanju51BaseURL
	case sourceHema:
		fallback = hemaSiteBaseURL
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
	case host == "jennyhow.com" || host == "www.jennyhow.com":
		return sourceHanxiaoquan
	// 参考 zhenguojianTV v0.8.31 新增站源。
	case duanju51MirrorHost(host):
		return sourceDuanju51
	case host == "kuaikaw.cn" || host == "www.kuaikaw.cn":
		return sourceHema
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
	// 已安装的 JS 站源模块优先，内置实现作兜底。
	if moduleSource(chapter.Source) != nil {
		return d.moduleResolveMedia(ctx, chapter.Source, task)
	}
	if chapter.Source == sourceCloudFront {
		return d.resolveLegacyMedia(ctx, task)
	}
	if chapter.Source == sourceHuangju {
		return d.resolveHuangjuMedia(ctx, task)
	}
	if chapter.Source == sourceYeguo {
		return d.resolveYeguoMedia(ctx, task)
	}
	if chapter.Source == sourceDSD {
		return d.resolveDSDMedia(ctx, task)
	}
	if chapter.Source == sourceHanxiaoquan {
		return d.resolveHanxiaoquanMedia(ctx, task)
	}
	if chapter.Source == sourceHuangguoMain {
		return d.resolveHuangguoWebMedia(ctx, task)
	}
	if chapter.Source == sourceHuangguoSelected || chapter.Source == sourceHuangguoWelfare {
		return d.resolveHuangguoPortalMedia(ctx, task)
	}
	if chapter.PageURL == "" && (chapter.Source == sourceHuangguoAI || chapter.Source == sourceHuangguoVideo) {
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
	media := providerMedia{URL: chapter.VideoURL, Referer: firstNonEmpty(chapter.Referer, d.providerBaseURL(chapter.Source)+"/")}
	// 以下为参考 zhenguojianTV v0.8.31 新增站源的按需播放解析：
	// 这些源详情里只存占位地址，播放时需再抓一次页面换新地址。
	if chapter.Source == sourceHema && strings.HasPrefix(chapter.VideoURL, "hema-play://") {
		var err error
		media, err = d.resolveHemaMedia(ctx, task)
		if err != nil {
			return providerMedia{}, err
		}
	}
	if strings.HasPrefix(chapter.VideoURL, "maccms-play://") {
		if _, found := d.maccmsProfileForDynamic(chapter.Source); found {
			var err error
			media, err = d.resolveMaccmsPlayback(ctx, task)
			if err != nil {
				return providerMedia{}, err
			}
		}
	}
	if strings.HasPrefix(chapter.VideoURL, customAESPlayScheme) {
		var err error
		media, err = d.resolveCustomAESPlayback(ctx, task)
		if err != nil {
			return providerMedia{}, err
		}
	}
	// 51短剧（参考 zhenguojianTV v0.8.31）：改版后每集 m3u8 带时效 auth_key，
	// 详情只存占位地址，播放时按集抓页面换新地址；过期的裸 m3u8 同样按需解析。
	if chapter.Source == sourceDuanju51 &&
		(strings.HasPrefix(chapter.VideoURL, duanju51PlayScheme) ||
			(isProviderHTTPMediaURL(chapter.VideoURL) && strings.Contains(chapter.VideoURL, "auth_key="))) {
		var err error
		media, err = d.resolveDuanju51Playback(ctx, task)
		if err != nil {
			return providerMedia{}, err
		}
	}
	if chapter.Source == sourceHuangdou {
		_, sourceID, valid := splitProviderDramaID(task.DramaID)
		if valid {
			sequence, err := strconv.Atoi(chapter.EpisodeString(task.Index))
			if err != nil || sequence < 1 {
				return providerMedia{}, fmt.Errorf("黄豆集数无效")
			}
			client := newHuangdouAPIClient(d)
			media, err = d.resolveHuangdouPlayback(ctx, client, sourceID, sequence)
			if err != nil {
				return providerMedia{}, err
			}
			media.Referer = client.host + "/home"
		}
	}
	// 黄果官方站在大陆被 DNS 污染，抓页面必然超时（每次白等 25 秒）。
	// 有 Worker 时直接走 Worker，跳过这段。
	workerDone := false
	if chapter.Source == sourceHuangguoAI || chapter.Source == sourceHuangguoVideo || chapter.Source == sourceHuangguoMain {
		if _, sourceID, ok := splitProviderDramaID(task.DramaID); ok {
			workerCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
			if workerMedia, workerErr := d.resolveHuangguoWorkerPlayback(workerCtx, sourceID, huangguoWorkerSequence(chapter, task.Index)); workerErr == nil {
				media = workerMedia
				workerDone = true
			}
			cancel()
		}
	}
	if !workerDone && chapter.PageURL != "" && (chapter.Source == sourceHuangguoAI || chapter.Source == sourceHuangguoVideo) {
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
		if chapter.Source == sourceHuangguoAI {
			media.URL = parseAIVideoURL(body, pageURL)
		} else {
			media.URL = parseDataHLS(body, pageURL)
		}
		media.Referer = pageURL
	}
	// 内置黄果站源直连失败时，改用 Worker 反代取该集流（任意一集都能取）。
	if !workerDone && !isProviderHTTPMediaURL(media.URL) &&
		(chapter.Source == sourceHuangguoAI || chapter.Source == sourceHuangguoVideo || chapter.Source == sourceHuangguoMain) {
		if _, sourceID, ok := splitProviderDramaID(task.DramaID); ok {
			// 前面抓页面可能已经把 ctx 拖到超时，兜底要独立预算，否则必然失败。
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
	if !isProviderHTTPMediaURL(media.URL) {
		return providerMedia{}, fmt.Errorf("%s 未返回有效播放地址，请刷新章节或确认站点访问权限", chapter.Source)
	}
	parsed, _ := url.Parse(media.URL)
	if strings.HasSuffix(strings.ToLower(parsed.Path), ".m3u8") || (chapter.Source == sourceEZDMW && isListresPlaylist(media.URL)) {
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
		// 完整性提示：疑似预览/占位片段仅记录诊断，不拦截播放、不改变行为。
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
