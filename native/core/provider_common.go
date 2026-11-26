package core

import (
	"context"
	"net"
	"sync"
	"crypto/sha256"

	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"

	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	sourceHuangguoAI    = "huangguoai"
	sourceHuangguoVideo = "huangguo-video"
	sourceHuangdou      = "huangdou"
	sourceHongguo       = "hongguo"
	sourceHuangju       = "huangju"
	sourceYeguo         = "yeguo"
	sourceDSD           = "dsd"
	sourceCloudFront    = "cloudfront"
	sourceSorani        = "sorani"
	sourceGuipian       = "guipian"
	sourceHanxiaoquan   = "hanxiaoquan"
	// 新增站源（参考 zhenguojianTV v0.8.31）：E站动漫、河马剧场。
	sourceEZDMW = "ezdmw"
	sourceHema  = "hema"
	// 橙果短剧：加密 JSON 接口站源。
	sourceChengguo = "chengguo"
	// 花魁短剧：Next.js 页面站源，入口域名 huakui1/2/3.com，剧集站在 huakui.net。
	sourceHuakui = "huakui"
	// 黄果主站：地址发布页 hgdhy11.com/way-home 指向 falpius.com（备用 whmxun.com）。
	// 黄果官网：falpius.com / whmxun.com，接口走 cloudfront + 加密响应。
	sourceHuangguoOfficial = "huangguo-official"
	sourceHuangguoMain     = "huangguo-main"
	sourceHuangguoSelected = "huangguo-selected"
	sourceHuangguoWelfare  = "huangguo-welfare"

	providerMaxBodyBytes = 20 * 1024 * 1024
	providerTimeout      = 12 * time.Second
)

var (
	huangguoAIBaseURL = "https://huangguoai.com"
	// 镜像顺序参考 zhenguojianTV v0.8.31：主域实测超时时把仍存活的
	// thu.agdkczeyx.cc 提前，减少回退等待；其余镜像保留兜底。
	huangguoAIBaseMirrors = []string{"https://huangguoai.com", "https://thu.agdkczeyx.cc", "https://ttvoij.ediayikma.cc", "https://thu.ediayikma.cc", "https://pku.ediayikma.cc", "https://fdu.ediayikma.cc"}
	huangguoVideoBaseURL  = "https://huangguo.video"

	huangguoAISlugs     = []string{"recommend", "newest", "ai-duanju", "ai-manju", "ai-huanlian", "ai-mogai", "ranks/hot"}
	huangguoAITypeNames = map[string]string{"recommend": "精选推荐", "newest": "最近上新", "ai-duanju": "AI成人短剧", "ai-manju": "AI成人漫剧", "ai-huanlian": "AI换脸", "ai-mogai": "AI魔改", "ranks/hot": "排行榜"}

	reDetailHref      = regexp.MustCompile(`(?is)<a\b[^>]*href=["']([^"']*/detail/([^"'/?#]+)[^"']*)["'][^>]*>.*?</a>`)
	reDramaCardStart  = regexp.MustCompile(`(?is)<div\b[^>]*class=["'][^"']*\bhg-drama-card\b[^"']*["'][^>]*>`)
	reVideoCard       = regexp.MustCompile(`(?is)<article\b[^>]*class=["'][^"']*\bvideo-card\b[^"']*["'][^>]*>.*?</article>`)
	reAnyArticle      = regexp.MustCompile(`(?is)<article\b[^>]*>.*?</article>`)
	reHuangguoLink    = regexp.MustCompile(`(?is)<a\b[^>]*href=["']([^"']*/(?:series|video)/([^"'/?#]+)[^"']*)["'][^>]*>.*?</a>`)
	reAIEpisodeLink   = regexp.MustCompile(`(?is)<a\b[^>]*href=["']([^"']*/video/[^"']+)["'][^>]*>.*?</a>`)
	reTitleTag        = regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)
	reH1Tag           = regexp.MustCompile(`(?is)<h1[^>]*>(.*?)</h1>`)
	reDescMeta        = regexp.MustCompile(`(?is)<meta\b[^>]*(?:name|property)=["'](?:description|og:description)["'][^>]*content=["']([^"']+)["'][^>]*>`)
	reEpisodeNumber   = regexp.MustCompile(`(?i)(?:第\s*0*(\d+)\s*(?:集|话|期)|(?:更新至|共|全)\s*0*(\d+)\s*(?:集|话|期)|(?:episode|ep)\s*#?\s*0*(\d+))`)
	reResolution      = regexp.MustCompile(`(?i)RESOLUTION\s*=\s*(\d+)x(\d+)`)
	reBandwidth       = regexp.MustCompile(`(?i)(?:AVERAGE-)?BANDWIDTH\s*=\s*(\d+)`)
	reDataHLS         = regexp.MustCompile(`(?is)data-hls=["']([^"']+)["']`)
	reDataPlaySrc     = regexp.MustCompile(`(?is)data-play-src=["']([^"']+)["']`)
	reMediaFieldValue = regexp.MustCompile(`(?is)["']?(?:videoSrc|videoUrl|playUrl|src|url)["']?\s*[:=]\s*["']([^"']+\.(?:m3u8|mp4)(?:[^"']*)?)["']`)
	reDateText        = regexp.MustCompile(`\d{4}-\d{1,2}-\d{1,2}`)
	reViewsText       = regexp.MustCompile(`(?i)[0-9]+(?:\.[0-9]+)?\s*[w万]?\s*次播放`)
	reScoreText       = regexp.MustCompile(`[0-9]+(?:\.[0-9]+)?\s*分`)
)

type providerEpisode struct {
	Key   string
	Title string
	URL   string
	Index int
	HLS   string
}

func providerDramaID(source, sourceID string) string {
	return source + ":" + strings.TrimSpace(sourceID)
}

func providerChapterID(source, sourceID, chapterKey string) string {
	return source + ":" + strings.TrimSpace(sourceID) + ":" + strings.TrimSpace(chapterKey)
}

func splitProviderDramaID(id string) (source, sourceID string, ok bool) {
	source, sourceID, ok = strings.Cut(strings.TrimSpace(id), ":")
	source = canonicalProviderSource(source)
	if !ok || strings.TrimSpace(sourceID) == "" || !isHuangguoProviderSource(source) {
		return "", "", false
	}
	return source, strings.TrimSpace(sourceID), true
}

func isHuangguoProviderSource(source string) bool {
	// 运行时注入的自定义源（苹果CMS / AES）也走统一解析链路。
	if isCustomSource(source) {
		return true
	}
	switch canonicalProviderSource(source) {
	case sourceHuangguoAI, sourceHuangguoVideo, sourceHuangdou, sourceHongguo, sourceHuangju, sourceYeguo, sourceDSD, sourceCloudFront, sourceHanxiaoquan,
		sourceDuanju51, sourceHema, sourceHuangguoOfficial, sourceHuangguoMain, sourceHuangguoSelected, sourceHuangguoWelfare:
		return true
	default:
		return moduleSource(canonicalProviderSource(source)) != nil
	}
}

func canonicalProviderSource(source string) string {
	normalized := strings.ToLower(strings.TrimSpace(source))
	// 已安装的 JS 站源模块优先：允许用模块替换内置实现，模块 id 与内置同名时以模块为准。
	if moduleSource(normalized) != nil {
		return normalized
	}
	switch normalized {
	case "huangguo", "huangguoai", "huangguoai.com":
		return sourceHuangguoAI
	case "huangguo-video", "huangguo.video":
		return sourceHuangguoVideo
	case "huangdou", "tideember.cc", "xqjurgek.top", "iwgsqufx.cc", "roufquunbu.cc", "ahievheved.cc", "momodrift.top", "eyeonneb.cc", "lzlukvca.cc", "glowcanvas.cc", "zuzuspot.top", "zuzucast.top":
		return sourceHuangdou
	case "hongguo", "hongguoduanju.com":
		return sourceHongguo
	case "huangju", "huangju.net", "api.huangju.net":
		return sourceHuangju
	case "yeguo", "ygdj7.com", "www.ygdj7.com", "analyze.buxefaex.cc", "delta.ygrwdsgt.cc", "yeguodj.com", "www.yeguodj.com":
		return sourceYeguo
	case "dsd", "dsd.com.se", "www.dsd.com.se":
		return sourceDSD
	case "cloudfront":
		return sourceCloudFront
	case "hanxiaoquan", "jennyhow.com", "www.jennyhow.com":
		return sourceHanxiaoquan
	// 以下为参考 zhenguojianTV v0.8.31 新增站源的别名映射。
	case "51duanju", "51短剧", "51dj", "blue.xdrgcewe.cc", "51duanju.com", "www.51duanju.com", "51duanju.app":
		return sourceDuanju51
	case "hema", "河马", "河马剧场", "河马短剧", "www.kuaikaw.cn", "kuaikaw.cn":
		return sourceHema
	case "huangguo-official", "黄果官网", "officialWebsite", "dqsibjrdsxx5r.cloudfront.net":
		return sourceHuangguoOfficial
	case "huangguo-main", "黄果主站", "falpius.com", "whmxun.com":
		return sourceHuangguoMain
	case "huangguo-selected", "黄果精选", "d2teten6pcf3ig.cloudfront.net":
		return sourceHuangguoSelected
	case "huangguo-welfare", "黄果福利", "d24dalcptr65cw.cloudfront.net":
		return sourceHuangguoWelfare
			default:
		return strings.TrimSpace(source)
	}
}

func mergeDramaMetadata(base, extra Drama) Drama {
	if base.VIP == nil && extra.VIP != nil {
		value := *extra.VIP
		base.VIP = &value
	}
	if extra.SortMetadata != nil && (base.SortMetadata == nil || extra.SortMetadata.CheckedAt.After(base.SortMetadata.CheckedAt)) {
		base.SortMetadata = extra.SortMetadata
	}
	if strings.TrimSpace(base.ID) == "" {
		base.ID = extra.ID
	}
	if strings.TrimSpace(base.Source) == "" {
		base.Source = extra.Source
	}
	if strings.TrimSpace(base.SourceID) == "" {
		base.SourceID = extra.SourceID
	}
	if strings.TrimSpace(base.Title) == "" {
		base.Title = extra.Title
	}
	if strings.TrimSpace(base.Name) == "" {
		base.Name = extra.Name
	}
	if strings.TrimSpace(base.Desc) == "" {
		base.Desc = extra.Desc
	}
	if strings.TrimSpace(base.Intro) == "" {
		base.Intro = extra.Intro
	}
	if coverPathFromAny(base.Cover) == "" {
		base.Cover = extra.Cover
	}
	if coverPathFromAny(base.CoverURL) == "" {
		base.CoverURL = extra.CoverURL
	}
	if valueEmpty(base.TotalEpisode) {
		base.TotalEpisode = extra.TotalEpisode
	}
	if valueEmpty(base.EpisodeCount) {
		base.EpisodeCount = extra.EpisodeCount
	}
	if strings.TrimSpace(base.ChannelName) == "" {
		base.ChannelName = extra.ChannelName
	}
	if strings.TrimSpace(base.CategoryName) == "" || (base.CategoryName == "首页" && extra.CategoryName != "") {
		base.CategoryName = extra.CategoryName
	}
	if strings.TrimSpace(base.Remark) == "" || base.Remark == "在线观看" {
		base.Remark = extra.Remark
	}
	if strings.TrimSpace(base.Score) == "" {
		base.Score = extra.Score
	}
	if strings.TrimSpace(base.Views) == "" {
		base.Views = extra.Views
	}
	if strings.TrimSpace(base.Heat) == "" {
		base.Heat = extra.Heat
	}
	if strings.TrimSpace(base.OnlineDate) == "" {
		base.OnlineDate = extra.OnlineDate
	}
	if len(base.Tags) == 0 {
		base.Tags = extra.Tags
	}
	if base.ReleaseStatus == "" || base.ReleaseStatus == "unknown" {
		base.ReleaseStatus = extra.ReleaseStatus
	}
	if dramaProvider(base) == sourceHuangguoAI && base.ID == extra.ID {
		_, sourceID, _ := splitProviderDramaID(base.ID)
		if title := firstHuangguoTitle(sourceID, base.Title, base.Name, extra.Title, extra.Name); title != "" {
			base.Title, base.Name = title, title
		}
	}
	return base
}

func valueEmpty(v any) bool {
	if v == nil {
		return true
	}
	s := strings.TrimSpace(fmt.Sprint(v))
	return s == "" || s == "0" || s == "<nil>"
}



func (d *Downloader) fetchHuangguoAIDetail(ctx context.Context, sourceID string) (Drama, []Chapter, error) {
	sourceID = strings.Trim(strings.TrimSpace(sourceID), "/")
	sourceID = strings.TrimPrefix(sourceID, "detail/")
	if sourceID == "" {
		return Drama{}, nil, fmt.Errorf("empty huangguoai sourceID")
	}
	// 官方站被 DNS 污染，抓页面还会把「相关推荐」误当分集（21 集的剧解析出 44 条）。
	// 有 Worker 时直接用官方 API 的分集列表。
	if drama, chapters, err := d.fetchHuangguoWorkerDetail(ctx, sourceID); err == nil && len(chapters) > 0 {
		return drama, chapters, nil
	}
	base := strings.TrimRight(d.providerBaseURL(sourceHuangguoAI), "/")
	detailURL := base + "/detail/" + url.PathEscape(sourceID) + "/"
	body, err := d.fetchProviderText(ctx, detailURL, base+"/")
	if err != nil {
		return Drama{}, nil, err
	}
	drama := huangguoDetailMetadata(body, detailURL, sourceHuangguoAI, sourceID)
	title := drama.DisplayTitle()
	episodes := parseHuangguoAIEpisodes(body, detailURL, sourceID)
	if len(episodes) == 0 {
		if media := parseAIVideoURL(body, detailURL); media != "" {
			episodes = []providerEpisode{{Key: "1", Title: title, URL: detailURL, Index: 1, HLS: media}}
		}
	}
	if len(episodes) == 0 {
		return drama, nil, nil
	}
	var chapters []Chapter
	for _, ep := range episodes {
		mediaURL := ep.HLS
		idx := ep.Index
		if idx <= 0 {
			idx = len(chapters) + 1
		}
		chapterTitle := strings.TrimSpace(ep.Title)
		if chapterTitle == "" {
			chapterTitle = fmt.Sprintf("第%d集", idx)
		}
		key := ep.Key
		if key == "" {
			key = strconv.Itoa(idx)
		}
		chapters = append(chapters, Chapter{ID: providerChapterID(sourceHuangguoAI, sourceID, key), Source: sourceHuangguoAI, Title: chapterTitle, VideoURL: mediaURL, PageURL: ep.URL, CurrentEpisode: rawEpisode(idx)})
	}
	sortProviderChapters(chapters)
	return drama, uniqueChapters(chapters), nil
}


func (d *Downloader) fetchHuangguoVideoDetail(ctx context.Context, sourceID string) (Drama, []Chapter, error) {
	sourceID = strings.Trim(strings.TrimSpace(sourceID), "/")
	if sourceID == "" {
		return Drama{}, nil, fmt.Errorf("empty huangguo-video sourceID")
	}
	detailPath := sourceID
	if !strings.HasPrefix(detailPath, "series/") && !strings.HasPrefix(detailPath, "video/") {
		detailPath = "series/" + detailPath
	}
	detailURL := strings.TrimRight(huangguoVideoBaseURL, "/") + "/" + detailPath
	body, err := d.fetchProviderText(ctx, detailURL, huangguoVideoBaseURL+"/")
	if err != nil {
		return Drama{}, nil, err
	}
	drama := huangguoDetailMetadata(body, detailURL, sourceHuangguoVideo, sourceID)
	title := drama.DisplayTitle()
	var episodes []providerEpisode
	if strings.HasPrefix(detailPath, "video/") {
		if hls := parseDataHLS(body, detailURL); hls != "" {
			episodes = []providerEpisode{{Key: "1", Title: title, URL: detailURL, Index: 1, HLS: hls}}
		}
	} else {
		episodes = parseHuangguoVideoEpisodes(body, detailURL)
	}
	if len(episodes) == 0 {
		if hls := parseDataHLS(body, detailURL); hls != "" {
			episodes = []providerEpisode{{Key: "1", Title: title, URL: detailURL, Index: 1, HLS: hls}}
		}
	}
	var chapters []Chapter
	for _, ep := range episodes {
		hlsURL := ep.HLS
		idx := ep.Index
		if idx <= 0 {
			idx = len(chapters) + 1
		}
		chapterTitle := strings.TrimSpace(ep.Title)
		if chapterTitle == "" {
			chapterTitle = fmt.Sprintf("第%d集", idx)
		}
		key := ep.Key
		if key == "" {
			key = strconv.Itoa(idx)
		}
		chapters = append(chapters, Chapter{ID: providerChapterID(sourceHuangguoVideo, sourceID, key), Source: sourceHuangguoVideo, Title: chapterTitle, VideoURL: hlsURL, PageURL: ep.URL, CurrentEpisode: rawEpisode(idx)})
	}
	sortProviderChapters(chapters)
	return drama, uniqueChapters(chapters), nil
}

func (d *Downloader) fetchProviderText(ctx context.Context, rawURL, referer string) (string, error) {
	retries := d.cfg.Retries
	if retries <= 0 {
		retries = 3
	}
	if referer == "" {
		referer = rawURL
	}
	candidates := d.providerURLCandidates(rawURL)
	var lastErr error
	tried := 0
	for _, candidate := range candidates {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		attempts := retries
		if len(candidates) > 1 {
			attempts = 1
		}
		for attempt := 1; attempt <= attempts; attempt++ {
			tried++
			if attempt > 1 {
				select {
				case <-time.After(time.Duration(attempt) * time.Second):
				case <-ctx.Done():
					return "", ctx.Err()
				}
			}
			timeout := providerTimeout
			if len(candidates) > 1 {
				timeout = 5 * time.Second
			}
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, candidate, nil)
			if err != nil {
				return "", err
			}
			req.Header.Set("User-Agent", userAgent)
			if agent, ok := ctx.Value(providerTextUserAgentKey{}).(string); ok && agent != "" {
				req.Header.Set("User-Agent", agent)
			}
			if providerSourceForURL(rawURL) != "" {
				req.Header.Set("Referer", providerRefererForURL(candidate, referer))
			} else {
				req.Header.Set("Referer", referer)
			}
			req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9")
			if noCache, _ := ctx.Value(providerTextNoCacheKey{}).(bool); noCache {
				req.Header.Set("Cache-Control", "no-cache")
			}
			resp, err := d.doCatalogRequestWithTimeout(req, timeout)
			if err != nil {
				lastErr = err
				var backoff *requestBackoff
				if errors.As(err, &backoff) {
					break
				}
				continue
			}
			body, readErr := io.ReadAll(io.LimitReader(resp.Body, providerMaxBodyBytes+1))
			closeErr := resp.Body.Close()
			if readErr != nil {
				lastErr = readErr
				continue
			}
			if closeErr != nil {
				lastErr = closeErr
				continue
			}
			if len(body) > providerMaxBodyBytes {
				lastErr = fmt.Errorf("response exceeds %d bytes", providerMaxBodyBytes)
				continue
			}
			if resp.StatusCode < 200 || resp.StatusCode >= 300 || catalogResponseBlockReason(resp, body) != "" {
				lastErr = d.catalogResponseError(req, resp, body)
				if catalogResponseBlockReason(resp, body) != "" || resp.StatusCode >= 400 && resp.StatusCode < 500 && resp.StatusCode != 408 {
					break
				}
				continue
			}
			if source := providerSourceForURL(rawURL); source != "" {
				effectiveURL := req.URL
				if resp.Request != nil {
					effectiveURL = resp.Request.URL
				}
				d.providerMu.Lock()
				d.providerHosts[source] = effectiveURL.Scheme + "://" + effectiveURL.Host
				d.providerMu.Unlock()
			}
			rememberPlaybackResponseURL(ctx, rawURL, resp)
			return string(body), nil
		}
	}
	if len(candidates) > 1 && lastErr != nil {
		return "", fmt.Errorf("黄果来源请求失败：已尝试 %d 个域名，最后错误：%w", tried, lastErr)
	}
	return "", lastErr
}

func providerURLCandidates(rawURL string) []string {
	u, err := url.Parse(rawURL)
	if err != nil || (u.Hostname() != "huangguoai.com" && u.Hostname() != "huangguoai.io") {
		return []string{rawURL}
	}
	out := make([]string, 0, len(huangguoAIBaseMirrors)+8)
	seen := map[string]bool{}
	add := func(candidate string) {
		if candidate == "" || seen[candidate] {
			return
		}
		seen[candidate] = true
		out = append(out, candidate)
	}
	for i, mirror := range huangguoAIBaseMirrors {
		if i == 1 {
			for _, generated := range generatedHuangguoAIMirrors(rawURL, 8) {
				add(rehostProviderURL(u, generated))
			}
		}
		add(rehostProviderURL(u, mirror))
	}
	if len(out) == 0 {
		return []string{rawURL}
	}
	return out
}

func rehostProviderURL(u *url.URL, mirror string) string {
	mu, err := url.Parse(mirror)
	if err != nil || mu.Host == "" {
		return ""
	}
	copyURL := *u
	copyURL.Scheme = mu.Scheme
	copyURL.Host = mu.Host
	return copyURL.String()
}

func generatedHuangguoAIMirrors(seed string, n int) []string {
	if n <= 0 {
		return nil
	}
	out := make([]string, 0, n)
	seen := map[string]bool{}
	for i := 0; len(out) < n && i < n*4; i++ {
		sum := sha256.Sum256([]byte(fmt.Sprintf("%s:%d", seed, i)))
		hexText := fmt.Sprintf("%x", sum[:])
		sub := strings.TrimLeft(hexText[:6], "0123456789")
		if len(sub) < 4 {
			sub = hexText[6:12]
		}
		if len(sub) > 6 {
			sub = sub[:6]
		}
		if len(sub) < 3 || seen[sub] {
			continue
		}
		seen[sub] = true
		out = append(out, "https://"+sub+".ediayikma.cc")
	}
	return out
}

func providerRefererForURL(candidate, fallback string) string {
	target, err := url.Parse(candidate)
	previous, previousErr := url.Parse(fallback)
	if err != nil || target.Host == "" || previousErr != nil || previous.Host == "" {
		return fallback
	}
	if source := providerSourceForURL(fallback); source != "" && (source == providerSourceForURL(candidate) || providerSourceForURL(candidate) == "") {
		previous.Scheme, previous.Host = target.Scheme, target.Host
	}
	previous.Fragment = ""
	return previous.String()
}

func parseHuangguoAIDramaCards(rawHTML, pageURL, category string) []Drama {
	positions := map[string]int{}
	var out []Drama
	add := func(dr Drama) {
		if dr.ID == "" || dr.SourceID == "" {
			return
		}
		if position, found := positions[dr.SourceID]; found {
			out[position] = mergeDramaMetadata(out[position], dr)
			return
		}
		positions[dr.SourceID] = len(out)
		out = append(out, dr)
	}
	markup := huangguoNonContent.ReplaceAllString(rawHTML, "")
	for _, block := range splitHuangguoAICardBlocks(markup) {
		sourceID := cleanID(firstNonEmpty(extractAttr(block, "data-track-id"), detailIDFromString(extractAttr(block, "href"))))
		if sourceID == "" {
			if m := reDetailHref.FindStringSubmatch(block); len(m) > 2 {
				sourceID = cleanID(m[2])
			}
		}
		if sourceID == "" {
			continue
		}
		title := huangguoCardTitle(block, sourceID)
		cover := resolveProviderURL(pageURL, firstNonEmpty(extractAttr(block, "data-src"), extractAttr(block, "data-original"), extractAttr(block, "src")))
		desc := firstNonEmpty(extractByClassText(block, "hg-drama-card__desc"), extractDescription(block))
		score := firstNonEmpty(extractByClassText(block, "hg-drama-card__score"), firstMatchText(reScoreText, block))
		episodeBlock := huangguoClassBlock(block, "hg-drama-card__episode")
		episode := firstNonEmpty(extractAttr(episodeBlock, "data-ep-base"), cleanText(episodeBlock))
		badge := extractByClassText(block, "hg-drama-card__badge")
		remark := strings.TrimSpace(strings.Join(nonEmptyStrings(badge, episode), " "))
		if remark == "" {
			remark = "在线观看"
		}
		views := normalizeViews(firstNonEmpty(firstMatchText(reViewsText, block), extractByClassText(block, "hg-drama-card__views"), extractByClassText(block, "hg-drama-card__play")))
		online := normalizeDate(firstMatchText(reDateText, block))
		tags := extractTags(block)
		typeName := firstNonEmpty(extractAttr(block, "data-track-type-name"), category)
		if title == "" {
			title = sourceID
		}
		dr := Drama{ID: providerDramaID(sourceHuangguoAI, sourceID), Source: sourceHuangguoAI, SourceID: sourceID, Title: title, Name: title, Desc: desc, Intro: desc, Cover: cover, CoverURL: cover, CategoryName: typeName, ChannelName: "huangguoai.com", Remark: remark, Score: score, Views: views, OnlineDate: online, Tags: tags}
		if n := episodeIndex(episode, 0); n > 0 {
			dr.TotalEpisode = n
			dr.EpisodeCount = n
		}
		add(dr)
	}
	for _, m := range reDetailHref.FindAllStringSubmatchIndex(markup, -1) {
		if len(m) < 6 || m[4] < 0 || m[5] < 0 {
			continue
		}
		sourceID := cleanID(markup[m[4]:m[5]])
		block := markup[m[0]:m[1]]
		title := firstHuangguoTitle(sourceID, huangguoCardTitle(block, sourceID), cleanText(block))
		if title == "" {
			continue
		}
		cover := resolveProviderURL(pageURL, extractAttr(block, "data-src", "data-original", "src"))
		add(Drama{ID: providerDramaID(sourceHuangguoAI, sourceID), Source: sourceHuangguoAI, SourceID: sourceID, Title: title, Name: title, Cover: cover, CoverURL: cover, CategoryName: category, ChannelName: "huangguoai.com"})
	}
	for _, dr := range parseHuangguoAIJSONLDDramas(rawHTML, category) {
		add(dr)
	}
	return out
}

func splitHuangguoAICardBlocks(raw string) []string {
	starts := reDramaCardStart.FindAllStringIndex(raw, -1)
	blocks := make([]string, 0, len(starts))
	for i, m := range starts {
		end := len(raw)
		if i+1 < len(starts) {
			end = starts[i+1][0]
		}
		blocks = append(blocks, huangguoElementBlock(raw[:end], m[0], m[1], "div"))
	}
	return blocks
}

func parseHuangguoAIJSONLDDramas(raw, category string) []Drama {
	var out []Drama
	seen := map[string]bool{}
	for _, block := range regexp.MustCompile(`(?is)<script[^>]*type=["']application/ld\+json["'][^>]*>(.*?)</script>`).FindAllStringSubmatch(raw, -1) {
		if len(block) < 2 {
			continue
		}
		var value any
		if json.Unmarshal([]byte(html.UnescapeString(block[1])), &value) != nil {
			continue
		}
		var walk func(any)
		walk = func(v any) {
			switch x := v.(type) {
			case []any:
				for _, item := range x {
					walk(item)
				}
			case map[string]any:
				name, _ := x["name"].(string)
				itemURL, _ := x["url"].(string)
				id := detailIDFromString(itemURL)
				if id != "" && !huangguoTitleNeedsRepair(name, id) && !seen[id] {
					seen[id] = true
					title := cleanText(name)
					out = append(out, Drama{ID: providerDramaID(sourceHuangguoAI, id), Source: sourceHuangguoAI, SourceID: id, Title: title, Name: title, CategoryName: category, ChannelName: "huangguoai.com"})
				}
				for _, child := range x {
					walk(child)
				}
			}
		}
		walk(value)
	}
	return out
}

func parseHuangguoAIJSONCards(raw []byte, pageURL, category string) []Drama {
	var decoded any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return nil
	}
	seen := map[string]bool{}
	var out []Drama
	var walk func(any)
	walk = func(v any) {
		switch x := v.(type) {
		case []any:
			for _, item := range x {
				walk(item)
			}
		case map[string]any:
			if dr, ok := dramaFromAIMap(x, pageURL, category); ok {
				if !seen[dr.SourceID] {
					seen[dr.SourceID] = true
					out = append(out, dr)
				}
				return
			}
			for _, child := range x {
				walk(child)
			}
		}
	}
	walk(decoded)
	return out
}

func dramaFromAIMap(m map[string]any, pageURL, category string) (Drama, bool) {
	sourceID := firstNonEmpty(mapString(m, "id"), mapString(m, "videoId"), mapString(m, "video_id"), mapString(m, "vid"), mapString(m, "slug"))
	if sourceID == "" {
		for _, v := range m {
			if s, ok := v.(string); ok {
				if id := detailIDFromString(s); id != "" {
					sourceID = id
					break
				}
			}
		}
	}
	if detailID := detailIDFromString(sourceID); detailID != "" {
		sourceID = detailID
	}
	sourceID = cleanID(sourceID)
	if sourceID == "" {
		return Drama{}, false
	}
	title := firstNonEmpty(mapString(m, "title"), mapString(m, "name"), mapString(m, "videoTitle"), mapString(m, "video_title"), mapString(m, "videoName"), mapString(m, "video_name"))
	if huangguoTitleNeedsRepair(title, sourceID) {
		return Drama{}, false
	}
	cover := resolveProviderURL(pageURL, firstNonEmpty(mapString(m, "cover"), mapString(m, "coverUrl"), mapString(m, "cover_url"), mapString(m, "coverImage"), mapString(m, "cover_image"), mapString(m, "image"), mapString(m, "imageUrl"), mapString(m, "thumbnail"), mapString(m, "poster"), mapString(m, "posterUrl")))
	desc := firstNonEmpty(mapString(m, "desc"), mapString(m, "description"), mapString(m, "intro"), mapString(m, "summary"))
	eps := huangguoMapEpisodes(m)
	remark := firstNonEmpty(mapString(m, "vod_remarks"), mapString(m, "remark"), mapString(m, "remarks"))
	if remark == "" && eps != "" {
		if strings.EqualFold(mapString(m, "is_finished"), "true") || mapString(m, "is_finished") == "1" {
			remark = "全" + eps + "集"
		} else {
			remark = "更新至 " + eps + " 集"
		}
	}
	score := firstNonEmpty(mapString(m, "score"), mapString(m, "rating"))
	if score != "" && !strings.Contains(score, "分") {
		score += "分"
	}
	views := normalizeViews(firstNonEmpty(mapString(m, "views"), mapString(m, "view_count"), mapString(m, "viewCount"), mapString(m, "view_num"), mapString(m, "viewNum"), mapString(m, "views_text"), mapString(m, "view_count_text"), mapString(m, "play_count"), mapString(m, "playCount"), mapString(m, "play_num"), mapString(m, "playNum"), mapString(m, "play_count_text"), mapString(m, "hits"), mapString(m, "hit_count"), mapString(m, "plays"), mapString(m, "play")))
	online := providerReleaseDate(firstNonEmpty(mapString(m, "onlineDate"), mapString(m, "online_date"), mapString(m, "online_time"), mapString(m, "onlineTime"), mapString(m, "publish_date"), mapString(m, "publishDate"), mapString(m, "publish_time"), mapString(m, "publishTime"), mapString(m, "release_date"), mapString(m, "releaseDate")))
	tags := mapStringSlice(m, "tags", "tag", "categories", "category")
	return Drama{ID: providerDramaID(sourceHuangguoAI, sourceID), Source: sourceHuangguoAI, SourceID: sourceID, Title: title, Name: title, Desc: desc, Intro: desc, Cover: cover, CoverURL: cover, TotalEpisode: eps, EpisodeCount: eps, CategoryName: category, ChannelName: "huangguoai.com", Remark: remark, Score: score, Views: views, OnlineDate: online, Tags: tags}, true
}

func parseHuangguoVideoCards(rawHTML, pageURL string) []Drama {
	blocks := reVideoCard.FindAllString(rawHTML, -1)
	if len(blocks) == 0 {
		for _, block := range reAnyArticle.FindAllString(rawHTML, -1) {
			if strings.Contains(strings.ToLower(block), "video-card") {
				blocks = append(blocks, block)
			}
		}
	}
	seen := map[string]bool{}
	var out []Drama
	for _, block := range blocks {
		link := reHuangguoLink.FindStringSubmatch(block)
		if len(link) < 3 {
			continue
		}
		path, sourceID := huangguoVideoSourceID(link[1])
		if sourceID == "" || seen[sourceID] {
			continue
		}
		seen[sourceID] = true
		title := firstNonEmpty(extractClosestAttr(block, 0, len(block), "title", "alt", "aria-label"), cleanText(block))
		cover := resolveProviderURL(pageURL, extractClosestAttr(block, 0, len(block), "data-src", "data-original", "src"))
		category := "video"
		if strings.HasPrefix(path, "series/") {
			category = "series"
		}
		desc := firstNonEmpty(extractClosestAttr(block, 0, len(block), "data-description", "description"), extractDescription(block))
		out = append(out, Drama{ID: providerDramaID(sourceHuangguoVideo, sourceID), Source: sourceHuangguoVideo, SourceID: sourceID, Title: title, Name: title, Desc: desc, Intro: desc, Cover: cover, CoverURL: cover, CategoryName: category, ChannelName: "huangguo.video", Remark: firstNonEmpty(extractByClassText(block, "bg-black/55"), extractByClassText(block, "text-gold-dim"))})
	}
	return out
}

func parseHuangguoAIEpisodes(rawHTML, pageURL, sourceID string) []providerEpisode {
	matches := reAIEpisodeLink.FindAllStringSubmatchIndex(rawHTML, -1)
	seen := map[string]bool{}
	var episodes []providerEpisode
	usedIndex := map[int]bool{}
	for _, m := range matches {
		if len(m) < 4 || m[2] < 0 || m[3] < 0 {
			continue
		}
		href := rawHTML[m[2]:m[3]]
		fullURL := resolveProviderURL(pageURL, href)
		linkHTML := rawHTML[m[0]:m[1]]
		title := firstNonEmpty(extractAttr(linkHTML, "title", "aria-label"), cleanText(linkHTML))
		key := aiEpisodeKey(href, sourceID)
		if key == "" {
			key = fmt.Sprintf("auto-%d", len(episodes)+1)
		}
		if seen[key] || seen[fullURL] {
			continue
		}
		seen[key] = true
		seen[fullURL] = true
		idx := episodeIndex(title, 0)
		if idx <= 0 {
			idx = len(episodes) + 1
		}
		idx = nextUnusedEpisodeIndex(idx, usedIndex)
		usedIndex[idx] = true
		episodes = append(episodes, providerEpisode{Key: key, Title: title, URL: fullURL, Index: idx})
	}
	sortProviderEpisodes(episodes)
	return episodes
}

func parseHuangguoVideoEpisodes(rawHTML, pageURL string) []providerEpisode {
	blocks := reVideoCard.FindAllString(rawHTML, -1)
	if len(blocks) == 0 {
		blocks = reHuangguoLink.FindAllString(rawHTML, -1)
	}
	positions := map[string]int{}
	var episodes []providerEpisode
	numbered := false
	for _, block := range blocks {
		link := reHuangguoLink.FindStringSubmatch(block)
		if len(link) < 2 {
			continue
		}
		path, key := huangguoVideoSourceID(link[1])
		if key == "" || !strings.HasPrefix(path, "video/") {
			continue
		}
		fullURL := resolveProviderURL(pageURL, link[1])
		title := firstNonEmpty(extractAttr(block, "title"), extractAttr(block, "alt"), cleanText(block))
		index := episodeIndex(cleanText(block), episodeIndex(title, 0))
		numbered = numbered || index > 0
		episode := providerEpisode{Key: key, Title: title, URL: fullURL, Index: index, HLS: parseDataHLS(block, pageURL)}
		if position, found := positions[fullURL]; found {
			if episodes[position].Index <= 0 && index > 0 {
				episodes[position] = episode
			}
		} else {
			positions[fullURL] = len(episodes)
			episodes = append(episodes, episode)
		}
	}
	var result []providerEpisode
	used := map[int]bool{}
	for _, episode := range episodes {
		if numbered && episode.Index <= 0 {
			continue
		}
		episode.Index = nextUnusedEpisodeIndex(episode.Index, used)
		used[episode.Index] = true
		result = append(result, episode)
	}
	sortProviderEpisodes(result)
	return result
}

func parseAIVideoURL(rawHTML, pageURL string) string {
	if media := parseAIVideoInitialData(rawHTML, pageURL); media != "" {
		return media
	}
	if m := reDataPlaySrc.FindStringSubmatch(rawHTML); len(m) > 1 {
		if media := normalizeProviderMediaURL(pageURL, m[1]); media != "" {
			return media
		}
	}
	for _, m := range reMediaFieldValue.FindAllStringSubmatch(rawHTML, -1) {
		if len(m) > 1 {
			if media := normalizeProviderMediaURL(pageURL, m[1]); media != "" {
				return media
			}
		}
	}
	return ""
}

func parseAIVideoInitialData(rawHTML, pageURL string) string {
	re := regexp.MustCompile(`(?is)<script\b[^>]*id=["']videoInitialData["'][^>]*>(.*?)</script>`)
	m := re.FindStringSubmatch(rawHTML)
	if len(m) < 2 {
		return ""
	}
	var data map[string]any
	if err := json.Unmarshal([]byte(html.UnescapeString(m[1])), &data); err != nil {
		return ""
	}
	if media := normalizeProviderMediaURL(pageURL, mapString(data, "videoSrc", "videoUrl", "playUrl")); media != "" {
		return media
	}
	eps, _ := data["epPlaySrcs"].(map[string]any)
	if len(eps) == 0 {
		return ""
	}
	preferred := firstNonEmpty(mapString(data, "ep", "episode"))
	if m := regexp.MustCompile(`/ep-(\d+)(?:/|$)`).FindStringSubmatch(pageURL); len(m) > 1 {
		preferred = m[1]
	}
	if preferred != "" {
		if media := normalizeProviderMediaURL(pageURL, fmt.Sprint(eps[preferred])); media != "" {
			return media
		}
	}
	keys := make([]string, 0, len(eps))
	for key := range eps {
		keys = append(keys, key)
	}
	sort.SliceStable(keys, func(i, j int) bool { return episodeIndex(keys[i], i+1) < episodeIndex(keys[j], j+1) })
	for _, key := range keys {
		if media := normalizeProviderMediaURL(pageURL, fmt.Sprint(eps[key])); media != "" {
			return media
		}
	}
	return ""
}

func parseDataHLS(rawHTML, pageURL string) string {
	if m := reDataHLS.FindStringSubmatch(rawHTML); len(m) > 1 {
		return normalizeProviderMediaURL(pageURL, m[1])
	}
	return ""
}

func sortProviderEpisodes(episodes []providerEpisode) {
	sort.SliceStable(episodes, func(i, j int) bool {
		if episodes[i].Index != episodes[j].Index {
			return episodes[i].Index < episodes[j].Index
		}
		return episodes[i].Key < episodes[j].Key
	})
}

func sortProviderChapters(chapters []Chapter) {
	sort.SliceStable(chapters, func(i, j int) bool {
		ei := chapterEpisodeNumber(chapters[i], i+1)
		ej := chapterEpisodeNumber(chapters[j], j+1)
		if ei != ej {
			return ei < ej
		}
		return chapters[i].ID < chapters[j].ID
	})
}

func chapterEpisodeNumber(ch Chapter, fallback int) int {
	if len(ch.CurrentEpisode) > 0 && string(ch.CurrentEpisode) != "null" {
		var n int
		if err := json.Unmarshal(ch.CurrentEpisode, &n); err == nil && n > 0 {
			return n
		}
		var s string
		if err := json.Unmarshal(ch.CurrentEpisode, &s); err == nil {
			return episodeIndex(s, fallback)
		}
	}
	return episodeIndex(ch.Title, fallback)
}

func rawEpisode(n int) json.RawMessage {
	if n <= 0 {
		n = 1
	}
	return json.RawMessage(strconv.Itoa(n))
}

func episodeIndex(s string, fallback int) int {
	if m := reEpisodeNumber.FindStringSubmatch(s); len(m) > 1 {
		for _, group := range m[1:] {
			if group == "" {
				continue
			}
			if n, err := strconv.Atoi(group); err == nil && n > 0 {
				return n
			}
		}
	}
	return fallback
}

func nextUnusedEpisodeIndex(preferred int, used map[int]bool) int {
	if preferred <= 0 {
		preferred = 1
	}
	if !used[preferred] {
		return preferred
	}
	for i := 1; ; i++ {
		if !used[i] {
			return i
		}
	}
}

func aiEpisodeKey(href, sourceID string) string {
	u, err := url.Parse(html.UnescapeString(href))
	path := href
	if err == nil {
		path = u.Path
	}
	path = strings.Trim(strings.TrimPrefix(path, "/video/"), "/")
	parts := strings.Split(path, "/")
	if len(parts) > 1 && parts[0] == sourceID {
		return cleanID(parts[len(parts)-1])
	}
	if len(parts) == 1 && parts[0] == sourceID {
		return ""
	}
	return cleanID(strings.Join(parts, "-"))
}

func huangguoVideoSourceID(href string) (path, sourceID string) {
	u, err := url.Parse(html.UnescapeString(href))
	if err == nil {
		path = strings.Trim(u.Path, "/")
	} else {
		path = strings.Trim(href, "/")
	}
	parts := strings.Split(path, "/")
	for i := 0; i+1 < len(parts); i++ {
		if parts[i] == "series" || parts[i] == "video" {
			code := cleanID(parts[i+1])
			if code == "" {
				return "", ""
			}
			return parts[i] + "/" + code, parts[i] + "/" + code
		}
	}
	return "", ""
}

func detailIDFromString(s string) string {
	u, err := url.Parse(html.UnescapeString(s))
	path := s
	if err == nil {
		path = u.Path
	}
	parts := strings.Split(strings.Trim(path, "/"), "/")
	for i := 0; i+1 < len(parts); i++ {
		if parts[i] == "detail" {
			return cleanID(parts[i+1])
		}
	}
	return ""
}

func cleanID(s string) string {
	s = strings.TrimSpace(html.UnescapeString(s))
	s = strings.Trim(s, " /\t\r\n\"'")
	if q := strings.IndexAny(s, "?#"); q >= 0 {
		s = s[:q]
	}
	return s
}

func extractPageTitle(rawHTML string) string {
	if m := reH1Tag.FindStringSubmatch(rawHTML); len(m) > 1 {
		if t := cleanText(m[1]); t != "" {
			return t
		}
	}
	if m := regexp.MustCompile(`(?is)<meta\b[^>]*property=["']og:title["'][^>]*content=["']([^"']+)["'][^>]*>`).FindStringSubmatch(rawHTML); len(m) > 1 {
		if t := strings.TrimSpace(html.UnescapeString(m[1])); t != "" {
			return t
		}
	}
	if m := reTitleTag.FindStringSubmatch(rawHTML); len(m) > 1 {
		return cleanText(m[1])
	}
	return ""
}

func extractDescription(rawHTML string) string {
	if m := reDescMeta.FindStringSubmatch(rawHTML); len(m) > 1 {
		return strings.TrimSpace(html.UnescapeString(m[1]))
	}
	return ""
}

func extractAttr(block string, names ...string) string {
	for _, name := range names {
		re := regexp.MustCompile(`(?is)\b` + regexp.QuoteMeta(name) + `\s*=\s*["']([^"']+)["']`)
		if m := re.FindStringSubmatch(block); len(m) > 1 {
			return strings.TrimSpace(html.UnescapeString(m[1]))
		}
	}
	return ""
}

func extractByClassText(raw, className string) string {
	if className == "" {
		return ""
	}
	re := regexp.MustCompile(`(?is)<[^>]+class=["'][^"']*` + regexp.QuoteMeta(className) + `[^"']*["'][^>]*>(.*?)</[^>]+>`)
	if m := re.FindStringSubmatch(raw); len(m) > 1 {
		return cleanText(m[1])
	}
	return ""
}

func firstMatchText(re *regexp.Regexp, raw string) string {
	if m := re.FindString(raw); m != "" {
		return strings.TrimSpace(m)
	}
	return ""
}

func extractTags(raw string) []string {
	re := regexp.MustCompile(`(?is)<[^>]+class=["'][^"']*hg-tag[^"']*["'][^>]*>(.*?)</[^>]+>`)
	seen := map[string]bool{}
	var out []string
	for _, m := range re.FindAllStringSubmatch(raw, -1) {
		if len(m) < 2 {
			continue
		}
		v := cleanText(m[1])
		if v != "" && !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	return out
}

func normalizeDate(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	matches := reDateText.FindAllString(s, -1)
	for _, m := range matches {
		parts := strings.Split(m, "-")
		if len(parts) != 3 {
			continue
		}
		y, yErr := strconv.Atoi(parts[0])
		mo, moErr := strconv.Atoi(parts[1])
		d, dErr := strconv.Atoi(parts[2])
		if yErr == nil && moErr == nil && dErr == nil && y >= 2000 && y <= 2100 && mo >= 1 && mo <= 12 && d >= 1 && d <= 31 {
			return fmt.Sprintf("%04d-%02d-%02d", y, mo, d)
		}
	}
	return ""
}

func normalizeViews(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if strings.Contains(s, "次播放") {
		return s
	}
	if strings.ContainsAny(strings.ToLower(s), "w万") {
		return s + "次播放"
	}
	n, err := strconv.ParseFloat(s, 64)
	if err != nil || n <= 0 {
		return s
	}
	return strconv.FormatFloat(n, 'f', -1, 64) + "次播放"
}

func cleanText(s string) string {
	s = regexp.MustCompile(`(?is)<script\b.*?</script>`).ReplaceAllString(s, " ")
	s = regexp.MustCompile(`(?is)<style\b.*?</style>`).ReplaceAllString(s, " ")
	s = regexp.MustCompile(`(?is)<[^>]+>`).ReplaceAllString(s, " ")
	s = html.UnescapeString(s)
	s = spaceChars.ReplaceAllString(s, " ")
	return strings.TrimSpace(s)
}

func contextBlock(s string, start, end, padding int) string {
	if start < 0 {
		start = 0
	}
	if end < start {
		end = start
	}
	lo := start - padding
	if lo < 0 {
		lo = 0
	}
	hi := end + padding
	if hi > len(s) {
		hi = len(s)
	}
	return s[lo:hi]
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func nonEmptyStrings(values ...string) []string {
	var out []string
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			out = append(out, strings.TrimSpace(v))
		}
	}
	return out
}

func mapString(m map[string]any, keys ...string) string {
	for _, key := range keys {
		if v, ok := m[key]; ok {
			switch x := v.(type) {
			case string:
				if text := strings.TrimSpace(x); text != "" {
					return text
				}
			case bool:
				return strconv.FormatBool(x)
			case float64:
				if x == float64(int64(x)) {
					return strconv.FormatInt(int64(x), 10)
				}
				return strconv.FormatFloat(x, 'f', -1, 64)
			case int:
				return strconv.Itoa(x)
			case json.Number:
				return x.String()
			}
		}
	}
	return ""
}

func mapStringSlice(m map[string]any, keys ...string) []string {
	seen := map[string]bool{}
	var out []string
	add := func(s string) {
		s = strings.TrimSpace(s)
		if s != "" && !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	for _, key := range keys {
		v, ok := m[key]
		if !ok {
			continue
		}
		switch x := v.(type) {
		case string:
			for _, part := range strings.FieldsFunc(x, func(r rune) bool { return r == ',' || r == '/' || r == '，' || r == '、' }) {
				add(part)
			}
		case []any:
			for _, item := range x {
				if row, ok := item.(map[string]any); ok {
					// 上游 tags/categoryTags 可能是对象数组（如 [{name:"奇幻",custom:true}]），
					// 直接 fmt.Sprint 会打印成 "map[custom:true name:奇幻]" 这种 Go 内部格式。
					if name := mapString(row, "name", "title", "label", "tag"); name != "" {
						add(name)
					}
					continue
				}
				add(fmt.Sprint(item))
			}
		}
	}
	return out
}

func extractClosestAttr(raw string, start, end int, names ...string) string {
	if start < 0 {
		start = 0
	}
	if end < start {
		end = start
	}
	if end > len(raw) {
		end = len(raw)
	}
	for _, value := range []string{raw[start:end], contextBlock(raw, start, end, 600)} {
		if value == "" {
			continue
		}
		if attr := extractAttr(value, names...); attr != "" {
			return attr
		}
	}
	return ""
}

func titleNearDetail(raw, sourceID string) string {
	needle := "/detail/" + sourceID
	idx := strings.Index(raw, needle)
	if idx < 0 {
		return ""
	}
	return extractByClassText(contextBlock(raw, idx, idx+len(needle), 1800), "hg-drama-card__title")
}

func resolveProviderURL(baseURL, ref string) string {
	ref = strings.TrimSpace(html.UnescapeString(ref))
	if ref == "" {
		return ""
	}
	ref = strings.ReplaceAll(ref, `\/`, `/`)
	if strings.HasPrefix(ref, "//") {
		return "https:" + ref
	}
	u, err := url.Parse(ref)
	if err != nil {
		return ref
	}
	if u.IsAbs() {
		if u.Scheme == "http" {
			u.Scheme = "https"
		}
		return u.String()
	}
	base, err := url.Parse(baseURL)
	if err != nil {
		return ref
	}
	resolved := base.ResolveReference(u)
	if resolved.Scheme == "http" {
		resolved.Scheme = "https"
	}
	return resolved.String()
}

func normalizeProviderMediaURL(pageURL, raw string) string {
	raw = strings.TrimSpace(html.UnescapeString(raw))
	raw = strings.Trim(raw, " \t\r\n\"'")
	if raw == "" {
		return ""
	}
	if unquoted, err := strconv.Unquote("\"" + strings.ReplaceAll(raw, "\"", "\\\"") + "\""); err == nil {
		raw = unquoted
	}
	raw = strings.ReplaceAll(raw, `\/`, `/`)
	resolved := resolveProviderURL(pageURL, raw)
	if !strings.Contains(strings.ToLower(resolved), ".m3u8") && !strings.Contains(strings.ToLower(resolved), ".mp4") {
		return ""
	}
	return resolved
}

func isProviderHTTPMediaURL(raw string) bool {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	return err == nil && (parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.Hostname() != "" && parsed.User == nil
}

// --- migrated from provider_huangguo_cards.go ---
var huangguoHTMLTag = regexp.MustCompile(`(?is)<(/?)([a-z][a-z0-9:-]*)\b(?:[^<>"']|"[^"]*"|'[^']*')*>`)

var huangguoNonContent = regexp.MustCompile(`(?is)<!--.*?-->|<script\b[^>]*>.*?</script\s*>|<style\b[^>]*>.*?</style\s*>`)

var huangguoTitleFragment = regexp.MustCompile(`(?i)(?:\b(?:class|type|title|aria-label)\s*=|(?:data|ata)-search-input|hg-search(?:__|-))`)

func huangguoElementBlock(raw string, start, end int, tag string) string {
	depth := 1
	for _, match := range huangguoHTMLTag.FindAllStringSubmatchIndex(raw[end:], -1) {
		if !strings.EqualFold(raw[end+match[4]:end+match[5]], tag) {
			continue
		}
		if match[3] > match[2] {
			depth--
		} else if !strings.HasSuffix(strings.TrimSpace(raw[end+match[0]:end+match[1]]), "/>") {
			depth++
		}
		if depth == 0 {
			return raw[start : end+match[1]]
		}
	}
	return raw[start:end]
}

func huangguoClassBlock(raw, className string) string {
	for _, match := range huangguoHTMLTag.FindAllStringSubmatchIndex(raw, -1) {
		if match[3] > match[2] {
			continue
		}
		for _, class := range strings.Fields(extractAttr(raw[match[0]:match[1]], "class")) {
			if class == className {
				return huangguoElementBlock(raw, match[0], match[1], raw[match[4]:match[5]])
			}
		}
	}
	return ""
}

func huangguoClassText(raw, className string) string {
	return cleanText(huangguoClassBlock(raw, className))
}

func huangguoTitleNeedsRepair(title, sourceID string) bool {
	title = strings.TrimSpace(title)
	return title == "" || title == sourceID || title == "短剧" || title == "搜索" ||
		huangguoTitleFragment.MatchString(title) || strings.Contains(title, "搜索记录") && strings.Contains(title, "暂无搜索记录")
}

func firstHuangguoTitle(sourceID string, values ...string) string {
	for _, title := range values {
		if !huangguoTitleNeedsRepair(title, sourceID) {
			return strings.TrimSpace(title)
		}
	}
	return ""
}

func huangguoCardTitle(block, sourceID string) string {
	return firstHuangguoTitle(sourceID,
		extractAttr(block, "data-track-title"), huangguoClassText(block, "hg-drama-card__title"),
		extractAttr(block, "data-search-hot"), huangguoClassText(block, "hg-search-suggest__hot-title"),
		extractAttr(block, "title"), extractAttr(block, "alt"), extractAttr(block, "aria-label"))
}

func huangguoMapEpisodes(row map[string]any) string {
	for _, key := range []string{"episode_count", "episodeCount", "total_episodes", "totalEpisodes", "totalEpisode", "total_episode", "episodes", "total"} {
		if count, err := strconv.Atoi(mapString(row, key)); err == nil && count > 0 {
			return strconv.Itoa(count)
		}
	}
	return ""
}

// --- migrated from provider_huangguo_portals.go ---
const (
	huangguoSelectedOrigin = "https://d2teten6pcf3ig.cloudfront.net"
	huangguoWelfareOrigin  = "https://d24dalcptr65cw.cloudfront.net"
)

var (
	// 精选站首页每部剧是一个 <article class="card"> 块：封面在 card__cover 的
	// data-src 上（占位图在 src 上），标题在 card__title 的 <a> 里。整块扫描
	// 才能把封面和标题对上同一个剧标识。
	selectedCard      = regexp.MustCompile(`(?is)<article\b[^>]*class="[^"]*card[^"]*"[^>]*>(.*?)</article>`)
	selectedCardLink  = regexp.MustCompile(`(?i)href="/play/([a-zA-Z0-9_-]{2,64})/"`)
	selectedCardTitle = regexp.MustCompile(`(?is)<h3\b[^>]*class="[^"]*card__title[^"]*"[^>]*>\s*<a\b[^>]*>(.*?)</a>`)
	selectedEpisode   = regexp.MustCompile(`(?is)<a\b([^>]*class="[^"]*play-ep[^"]*"[^>]*)>(.*?)</a>`)
	welfareCard       = regexp.MustCompile(`(?is)<a\b([^>]*href="/detail/([a-zA-Z0-9_-]{2,64})/?"[^>]*)>\s*<img\b([^>]*)>`)
	// 懒加载封面：真图在 data-src / data-original 上，src 是占位图。
	cardCoverAttr  = regexp.MustCompile(`(?is)\b(?:data-src|data-original|data-lazy-src|src)=["']([^"']+)["']`)
	cardImgTag     = regexp.MustCompile(`(?is)<img\b[^>]*>`)
	cardCoverCSS   = regexp.MustCompile(`(?is)--cover:\s*url\(\s*['"]?([^'")]+)`)
	welfareEpisode = regexp.MustCompile(`(?is)<a\b([^>]*class="[^"]*detail-episode[^"]*"[^>]*)>(.*?)</a>`)
	portalAttr     = regexp.MustCompile(`(?is)([a-z-]+)="([^"]*)"`)
	portalTitle    = regexp.MustCompile(`(?is)<h1\b[^>]*>(.*?)</h1>`)
	portalPlayer   = regexp.MustCompile(`(?is)<div\b[^>]*data-play-player\b[^>]*>`)
	welfareMedia   = regexp.MustCompile(`(?is)<div\b[^>]*class="[^"]*dplayer-video[^"]*"[^>]*data-url="([^"]+)"`)
)

func portalAttributes(tag string) map[string]string {
	result := map[string]string{}
	for _, m := range portalAttr.FindAllStringSubmatch(tag, -1) {
		result[strings.ToLower(m[1])] = html.UnescapeString(m[2])
	}
	return result
}

// portalCardCover 取卡片封面的真实地址。懒加载卡片的 src 是占位图，
// 真图在 data-src / data-original / data-lazy-src 上，个别卡片直接用 CSS 变量。
func portalCardCover(tag, pageURL string) string {
	attrs := portalAttributes(tag)
	for _, key := range []string{"data-src", "data-original", "data-lazy-src", "src"} {
		address := strings.TrimSpace(attrs[key])
		if address == "" || strings.Contains(address, "placeholder") {
			continue
		}
		return resolveProviderURL(pageURL, address)
	}
	if m := cardCoverCSS.FindStringSubmatch(tag); len(m) == 2 {
		if address := strings.TrimSpace(m[1]); address != "" && !strings.Contains(address, "placeholder") {
			return resolveProviderURL(pageURL, address)
		}
	}
	return ""
}

func portalCatalog(body, source string) []Drama {
	pageURL := portalOrigin(source) + "/"
	seen := map[string]bool{}
	var result []Drama
	if source == sourceHuangguoSelected {
		for _, m := range selectedCard.FindAllStringSubmatch(body, -1) {
			block := m[1]
			link := selectedCardLink.FindStringSubmatch(block)
			if len(link) < 2 {
				continue
			}
			id := link[1]
			title := ""
			if t := selectedCardTitle.FindStringSubmatch(block); len(t) >= 2 {
				title = cleanText(t[1])
			} else if img := cardImgTag.FindString(block); img != "" {
				title = cleanText(portalAttributes(img)["alt"])
			}
			if title == "" || seen[id] {
				continue
			}
			seen[id] = true
			cover := ""
			if img := cardImgTag.FindString(block); img != "" {
				cover = portalCardCover(img, pageURL)
			} else {
				cover = portalCardCover(block, pageURL)
			}
			result = append(result, Drama{ID: providerDramaID(source, id), Source: source, SourceID: id, Title: title, Name: title, Cover: cover, CoverURL: cover})
		}
		return result
	}
	for _, m := range welfareCard.FindAllStringSubmatch(body, -1) {
		id := m[2]
		imgAttrs := portalAttributes(m[3])
		title := cleanText(imgAttrs["alt"])
		if title == "" || seen[id] {
			continue
		}
		seen[id] = true
		cover := portalCardCover(m[3], pageURL)
		result = append(result, Drama{ID: providerDramaID(source, id), Source: source, SourceID: id, Title: title, Name: title, Cover: cover, CoverURL: cover})
	}
	return result
}

func portalOrigin(source string) string {
	if source == sourceHuangguoSelected {
		return huangguoSelectedOrigin
	}
	return huangguoWelfareOrigin
}

func portalPageAt(origin, source, id string) string {
	if source == sourceHuangguoSelected {
		return origin + "/play/" + url.PathEscape(id) + "/"
	}
	return origin + "/detail/" + url.PathEscape(id) + "/"
}

func portalPage(source, id string) string {
	if source == sourceHuangguoSelected {
		return portalOrigin(source) + "/play/" + url.PathEscape(id) + "/"
	}
	return portalOrigin(source) + "/detail/" + url.PathEscape(id) + "/"
}

func (d *Downloader) fetchHuangguoPortalCatalog(ctx context.Context, source string) ([]Drama, error) {
	var lastErr error
	for _, origin := range d.huangguoPortalOrigins(ctx, source) {
		body, err := d.fetchProviderText(ctx, origin+"/", origin+"/")
		if err != nil {
			lastErr = err
			continue
		}
		if result := portalCatalog(body, source); len(result) > 0 {
			return result, nil
		}
		lastErr = errors.New("黄果站首页未解析到剧集")
	}
	if lastErr == nil {
		lastErr = errors.New("黄果站暂无可用入口")
	}
	return nil, lastErr
}

func portalEpisodes(body, source, id string) []Chapter {
	var re *regexp.Regexp
	if source == sourceHuangguoSelected {
		re = selectedEpisode
	} else {
		re = welfareEpisode
	}
	seen := map[string]bool{}
	var result []Chapter
	for _, m := range re.FindAllStringSubmatch(body, -1) {
		attrs := portalAttributes(m[1])
		href := attrs["href"]
		key := attrs["data-target-video-id"]
		if source == sourceHuangguoWelfare {
			key = attrs["data-chapter-id"]
		}
		if key == "" && source == sourceHuangguoWelfare {
			parts := strings.Split(strings.Trim(href, "/"), "/")
			if len(parts) == 3 && parts[0] == "play" && parts[1] == id {
				key = parts[2]
			}
		}
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		episode, _ := strconv.Atoi(attrs["data-target-episode"])
		if source == sourceHuangguoWelfare {
			episode, _ = strconv.Atoi(attrs["data-ep"])
		}
		if episode < 1 {
			episode = episodeIndex(cleanText(m[2]), 0)
		}
		if episode < 1 {
			episode = len(result) + 1
		}
		chapter := Chapter{ID: providerChapterID(source, id, key), Source: source, Title: fmt.Sprintf("第%d集", episode), CurrentEpisode: rawEpisode(episode)}
		if strings.Contains(m[1], "play-ep--coin") || strings.Contains(m[1], "data-coin-dialog-open") {
			chapter.VIP = true
		}
		if strings.HasPrefix(href, "/play/") {
			chapter.PageURL = resolveProviderURL(portalOrigin(source)+"/", href)
		}
		result = append(result, chapter)
	}
	sortProviderChapters(result)
	return result
}

func (d *Downloader) fetchHuangguoPortalDetail(ctx context.Context, source, id string) (Drama, []Chapter, error) {
	if !validHuangguoWebID(id) {
		return Drama{}, nil, errors.New("黄果剧集 ID 无效")
	}
	var body string
	var lastErr error
	for _, origin := range d.huangguoPortalOrigins(ctx, source) {
		candidate := portalPageAt(origin, source, id)
		text, err := d.fetchProviderText(ctx, candidate, origin+"/")
		if err != nil {
			lastErr = err
			continue
		}
		body = text
		break
	}
	if body == "" {
		if lastErr == nil {
			lastErr = errors.New("黄果详情页不可用")
		}
		return Drama{}, nil, lastErr
	}
	title := cleanText(firstRegexGroup(portalTitle, body))
	if title == "" {
		title = cleanText(firstRegexGroup(reHuangguoWebTitle, body))
	}
	if title == "" {
		return Drama{}, nil, errors.New("黄果详情页没有剧名")
	}
	chapters := portalEpisodes(body, source, id)
	if len(chapters) == 0 {
		return Drama{}, nil, errors.New("黄果详情页没有分集")
	}
	drama := Drama{ID: providerDramaID(source, id), Source: source, SourceID: id, Title: title, Name: title, TotalEpisode: strconv.Itoa(len(chapters)), EpisodeCount: strconv.Itoa(len(chapters))}
	return drama, chapters, nil
}

func (d *Downloader) resolveHuangguoPortalMedia(ctx context.Context, task Task) (providerMedia, error) {
	source, id, ok := splitProviderDramaID(task.DramaID)
	if !ok || (source != sourceHuangguoSelected && source != sourceHuangguoWelfare) || task.Chapter.Source != source {
		return providerMedia{}, errors.New("黄果剧集来源无效")
	}
	page := task.Chapter.PageURL
	if page == "" {
		page = portalPage(source, id)
	}
	if providerSourceForURL(page) != source {
		return providerMedia{}, errors.New("黄果分集页面来源不匹配")
	}
	body, err := d.fetchProviderText(ctx, page, portalOrigin(source)+"/")
	if err != nil {
		return providerMedia{}, err
	}
	player := portalPlayer.FindString(body)
	if player == "" {
		return providerMedia{}, errors.New("黄果分集页没有播放器")
	}
	attrs := portalAttributes(player)
	if attrs["data-can-watch"] != "1" || attrs["data-need-unlock"] == "1" || attrs["data-need-vip"] == "1" {
		return providerMedia{}, errors.New("黄果该集尚未解锁，无法取得正片")
	}
	raw := attrs["data-main-url"]
	if source == sourceHuangguoWelfare {
		raw = firstRegexGroup(welfareMedia, body)
	}
	if raw == "" {
		return providerMedia{}, errors.New("黄果页面未提供完整播放地址")
	}
	media := resolveProviderURL(page, raw)
	if !isProviderHTTPMediaURL(media) {
		return providerMedia{}, errors.New("黄果播放地址无效")
	}
	return providerMedia{URL: media, Referer: page}, nil
}

// --- migrated from provider_huangguo_web.go ---
const (
	huangguoWebMaxPages   = 1
	huangguoPublishSource = "https://hgdhy11.com/way-home"
	huangguoWebMaxKeyword = 64
)

var (
	huangguoWebBaseURLs = []string{"https://falpius.com", "https://whmxun.com"}

	reHuangguoWebVideo     = regexp.MustCompile(`(?is)<a\b[^>]*href="(/videos/([a-z0-9]{4,60}))"[^>]*>(.*?)</a>`)
	reHuangguoWebTitle     = regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)
	reHuangguoWebEpisodes  = regexp.MustCompile(`(?s)"episodes":(\[.*?\]),"firstChapterId"`)
	reHuangguoWebFree      = regexp.MustCompile(`(?s)"freeEpisodeCount":(\d{1,5})`)
	reHuangguoWebCoverHost = regexp.MustCompile(`https://([a-z0-9.\-]+)/cf[0-9]+/`)
	reHuangguoWebVideoSlug = regexp.MustCompile(`\\?"seoSlug\\?":\\?"([a-z0-9_-]{2,64})\\?"`)
	reHuangguoRscPush      = regexp.MustCompile(`(?s)self\.__next_f\.push\(\[1,("(?:[^"\\]|\\.)*")\]`)
	reHuangguoWebSeoSlug   = regexp.MustCompile(`\\?"seoSlug\\?":\\?"([a-z0-9_-]{2,64})\\?"`)
	reHuangguoWebCover     = regexp.MustCompile(`\\?"cover\\?":\\?"((?:https?:\\?/\\?/[^"\\]+|cf[0-9]+/[^"\\]+))\\?"`)
	reHuangguoWebName      = regexp.MustCompile(`\\?"title\\?":\\?"([^"\\]{1,200})\\?"`)
	reHuangguoWebCurrent   = regexp.MustCompile(`\\?"currentEpisode\\?":(\d{1,5})`)

	reHuangguoPublishEntry = regexp.MustCompile(`"name":"([^"]{2,20})"[^}]*?"path":"(https://[^"]+)"`)

	huangguoPublishMu       sync.Mutex
	huangguoPublishCached   map[string][]string
	huangguoPublishCachedAt time.Time
)

type huangguoWebEpisode struct {
	ChapterID string `json:"chapterId"`
	Episode   int    `json:"episode"`
	PlayTime  int    `json:"playTime"`
	Title     string `json:"title"`
	URL       string `json:"url"`
}

func validHuangguoWebID(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 64 {
		return false
	}
	for _, r := range value {
		if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' || r == '_') {
			return false
		}
	}
	return true
}

// huangguoPublishOrigins 从地址发布页解析各站点的可用入口。
//
// 发布页把入口按名称登记在 JSON 里（实测字段 name + path），例如
// 「黄果精选站」→ d2teten6pcf3ig.cloudfront.net、「黄果福利站」→ d24dalcptr65cw.cloudfront.net。
// 因此精选与福利也走发布页，域名变化时无需改代码。
func (d *Downloader) huangguoPublishOrigins(ctx context.Context) map[string][]string {
	huangguoPublishMu.Lock()
	cached, cachedAt := huangguoPublishCached, huangguoPublishCachedAt
	huangguoPublishMu.Unlock()
	if len(cached) > 0 && time.Since(cachedAt) < 30*time.Minute {
		return cached
	}
	origins := map[string][]string{
		sourceHuangguoMain:     append([]string{}, huangguoWebBaseURLs...),
		sourceHuangguoSelected: {huangguoSelectedOrigin},
		sourceHuangguoWelfare:  {huangguoWelfareOrigin},
	}
	body, err := d.fetchProviderText(ctx, huangguoPublishSource, "https://hgdhy11.com/")
	if err != nil {
		return origins
	}
	decoded := strings.ReplaceAll(strings.ReplaceAll(body, `\"`, `"`), `\/`, `/`)
	seen := map[string]bool{}
	for _, match := range reHuangguoPublishEntry.FindAllStringSubmatch(decoded, -1) {
		name, path := match[1], strings.TrimRight(match[2], "/")
		if !strings.HasPrefix(path, "https://") {
			continue
		}
		switch {
		case strings.Contains(name, "精选"):
			if !seen[path] {
				seen[path] = true
				origins[sourceHuangguoSelected] = append(origins[sourceHuangguoSelected], path)
			}
		case strings.Contains(name, "福利"):
			if !seen[path] {
				seen[path] = true
				origins[sourceHuangguoWelfare] = append(origins[sourceHuangguoWelfare], path)
			}
		}
	}
	for _, match := range regexp.MustCompile(`https://([a-z0-9.\-]+\.[a-z]{2,})`).FindAllStringSubmatch(body, -1) {
		host := strings.ToLower(match[1])
		if !strings.Contains(host, "falpius") && !strings.Contains(host, "whmxun") {
			continue
		}
		origin := "https://" + host
		if seen[origin] {
			continue
		}
		seen[origin] = true
		origins[sourceHuangguoMain] = append([]string{origin}, origins[sourceHuangguoMain]...)
	}
	huangguoPublishMu.Lock()
	huangguoPublishCached, huangguoPublishCachedAt = origins, time.Now()
	huangguoPublishMu.Unlock()
	return origins
}

func (d *Downloader) huangguoPublishHosts(ctx context.Context) []string {
	origins := d.huangguoPublishOrigins(ctx)
	if hosts := origins[sourceHuangguoMain]; len(hosts) > 0 {
		return hosts
	}
	return huangguoWebBaseURLs
}

// huangguoPortalOrigins 返回指定黄果站点的候选入口，发布页优先、内置兜底。
func (d *Downloader) huangguoPortalOrigins(ctx context.Context, source string) []string {
	origins := d.huangguoPublishOrigins(ctx)
	if hosts := origins[source]; len(hosts) > 0 {
		return hosts
	}
	return []string{portalOrigin(source)}
}

func (d *Downloader) fetchHuangguoWebCatalog(ctx context.Context) ([]Drama, error) {
	hosts := d.huangguoPublishHosts(ctx)
	var lastErr error
	for _, origin := range hosts {
		body, err := d.fetchProviderText(ctx, origin+"/", origin+"/")
		if err != nil {
			lastErr = err
			continue
		}
		items := huangguoWebParseCatalog(body)
		if len(items) > 0 {
			return items, nil
		}
		lastErr = errors.New("黄果主站首页未解析到剧集")
	}
	if lastErr == nil {
		lastErr = errors.New("黄果主站暂无可解析入口")
	}
	return nil, lastErr
}

// huangguoWebJSONObjects 把页面里 RSC 载荷的 JSON 对象切开。
// 载荷用 \" 转义，直接按花括号配对扫描即可，不依赖字段顺序。
func huangguoWebJSONObjects(raw string) []string {
	objects := make([]string, 0, 128)
	seen := map[string]bool{}
	// Next.js RSC 的每条 push 里含转义后的 JSON 片段；先逐层反转义，
	// 再用带引号的字段正则直接扫整段字符串，不依赖对象边界配对。
	payloads := []string{raw}
	for _, match := range reHuangguoRscPush.FindAllStringSubmatch(raw, -1) {
		payload := match[1]
		for depth := 0; depth < 4; depth++ {
			decoded, err := strconv.Unquote(`"` + payload + `"`)
			if err != nil || decoded == payload {
				break
			}
			payload = decoded
		}
		payloads = append(payloads, payload)
	}
	for _, payload := range payloads {
		for _, pattern := range []*regexp.Regexp{reHuangguoWebSeoSlug, reHuangguoWebVideoSlug} {
			for _, match := range pattern.FindAllStringSubmatch(payload, -1) {
				if len(match) < 2 || seen[match[1]] {
					continue
				}
				seen[match[1]] = true
				objects = append(objects, match[1])
			}
		}
	}
	return objects
}

// huangguoWebCoverIndex 从首页载荷里取「剧标识 → 封面」。
// 封面是相对图床路径（cf230705/image/…），主机名取页面里出现的绝对封面域名。
func huangguoWebCoverIndex(rawHTML string) map[string]string {
	host := ""
	if m := reHuangguoWebCoverHost.FindStringSubmatch(rawHTML); len(m) == 2 {
		host = "https://" + m[1] + "/"
	}
	index := map[string]string{}
	payloads := []string{rawHTML}
	for _, match := range reHuangguoRscPush.FindAllStringSubmatch(rawHTML, -1) {
		payload := match[1]
		for depth := 0; depth < 4; depth++ {
			decoded, err := strconv.Unquote(`"` + payload + `"`)
			if err != nil || decoded == payload {
				break
			}
			payload = decoded
		}
		payloads = append(payloads, payload)
	}
	for _, payload := range payloads {
		for _, match := range reHuangguoWebSeoSlug.FindAllStringSubmatchIndex(payload, -1) {
			slug := payload[match[2]:match[3]]
			window := payload[max(0, match[0]-1200):min(len(payload), match[1]+300)]
			coverMatch := reHuangguoWebCover.FindStringSubmatch(window)
			if len(coverMatch) != 2 {
				continue
			}
			cover := strings.TrimSpace(coverMatch[1])
			if cover == "" {
				continue
			}
			if !strings.HasPrefix(cover, "http") {
				if host == "" {
					continue
				}
				cover = host + cover
			}
			index[slug] = cover
		}
	}
	return index
}

func huangguoWebParseCatalog(rawHTML string) []Drama {
	covers := huangguoWebCoverIndex(rawHTML)
	names := map[string]string{}
	episodes := map[string]int{}
	payloads := []string{rawHTML}
	for _, match := range reHuangguoRscPush.FindAllStringSubmatch(rawHTML, -1) {
		payload := match[1]
		for depth := 0; depth < 4; depth++ {
			decoded, err := strconv.Unquote(`"` + payload + `"`)
			if err != nil || decoded == payload {
				break
			}
			payload = decoded
		}
		payloads = append(payloads, payload)
	}
	for _, payload := range payloads {
		for _, match := range reHuangguoWebSeoSlug.FindAllStringSubmatchIndex(payload, -1) {
			slug := payload[match[2]:match[3]]
			window := payload[max(0, match[0]-1200):min(len(payload), match[1]+300)]
			if name := reHuangguoWebName.FindStringSubmatch(window); len(name) == 2 {
				names[slug] = cleanText(name[1])
			}
			if count := reHuangguoWebCurrent.FindStringSubmatch(window); len(count) == 2 {
				if total, err := strconv.Atoi(count[1]); err == nil && total > 0 {
					episodes[slug] = total
				}
			}
		}
	}

	seen := map[string]bool{}
	items := make([]Drama, 0, 32)
	appendItem := func(slug, title string) {
		if !validHuangguoWebID(slug) || title == "" || seen[slug] {
			return
		}
		seen[slug] = true
		item := Drama{
			ID:          providerDramaID(sourceHuangguoMain, slug),
			Source:      sourceHuangguoMain,
			SourceID:    slug,
			Title:       title,
			Name:        title,
			Cover:       covers[slug],
			CoverURL:    covers[slug],
			ChannelName: "falpius.com",
		}
		if total := episodes[slug]; total > 0 {
			item.TotalEpisode = total
			item.EpisodeCount = total
		}
		items = append(items, item)
	}

	for _, match := range reHuangguoWebVideo.FindAllStringSubmatch(rawHTML, -1) {
		appendItem(match[2], cleanText(match[3]))
	}
	for slug, title := range names {
		appendItem(slug, title)
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].SourceID < items[j].SourceID })
	return items
}
func (d *Downloader) fetchHuangguoWebDetail(ctx context.Context, sourceID string) (Drama, []Chapter, error) {
	if !validHuangguoWebID(sourceID) {
		return Drama{}, nil, errors.New("黄果主站剧集 ID 无效")
	}
	hosts := d.huangguoPublishHosts(ctx)
	var body string
	var pageURL string
	var lastErr error
	for _, origin := range hosts {
		address := origin + "/videos/" + url.PathEscape(sourceID)
		text, err := d.fetchProviderText(ctx, address, origin+"/")
		if err != nil {
			lastErr = err
			continue
		}
		body, pageURL = text, address
		break
	}
	if body == "" {
		if lastErr == nil {
			lastErr = errors.New("黄果主站详情不可用")
		}
		return Drama{}, nil, lastErr
	}
	plain := strings.ReplaceAll(strings.ReplaceAll(body, `\"`, `"`), `\/`, `/`)
	title := cleanText(firstRegexGroup(reHuangguoWebTitle, plain))
	title = strings.Split(title, "-")[0]
	title = strings.TrimSpace(title)
	if title == "" {
		title = sourceID
	}
	drama := Drama{
		ID:          providerDramaID(sourceHuangguoMain, sourceID),
		Source:      sourceHuangguoMain,
		SourceID:    sourceID,
		Title:       title,
		Name:        title,
		ChannelName: "falpius.com",
		Remark:      pageURL,
	}
	if cover := firstRegexGroup(reHuangguoWebCoverHost, plain); cover != "" {
		drama.Cover = "https://" + cover + "/"
		drama.CoverURL = drama.Cover
	}
	episodes, freeCount := huangguoWebEpisodes(plain)
	chapters := make([]Chapter, 0, len(episodes))
	for _, episode := range episodes {
		if episode.ChapterID == "" || episode.Episode < 1 {
			continue
		}
		vip := freeCount > 0 && episode.Episode > freeCount
		chapter := Chapter{
			ID:             providerChapterID(sourceHuangguoMain, sourceID, episode.ChapterID),
			Source:         sourceHuangguoMain,
			Title:          fmt.Sprintf("第%d集", episode.Episode),
			CurrentEpisode: rawEpisode(episode.Episode),
			PageURL:        pageURL,
		}
		if vip {
			chapter.VIP = true
		}
		chapters = append(chapters, chapter)
	}
	sortProviderChapters(chapters)
	drama.TotalEpisode = strconv.Itoa(len(chapters))
	drama.EpisodeCount = drama.TotalEpisode
	if len(chapters) == 0 {
		return drama, nil, errors.New("黄果主站详情页未提供分集")
	}
	return drama, uniqueChapters(chapters), nil
}

// huangguoWebMediaURL 与主站前端一致：绝对地址原样使用，相对路径走同域
// /api/app/vid/h5/m3u8/ 代理（脚本 4606-*.js 的实现），不做跨主机拼接。
func huangguoWebMediaURL(pageURL, raw string) string {
	raw = strings.TrimSpace(raw)
	if isProviderHTTPMediaURL(raw) {
		return raw
	}
	if raw == "" {
		return ""
	}
	origin := ""
	if parsed, err := url.Parse(pageURL); err == nil && parsed.Scheme == "https" && parsed.Host != "" {
		origin = parsed.Scheme + "://" + parsed.Host
	}
	if origin == "" {
		return ""
	}
	media := origin + "/api/app/vid/h5/m3u8/" + strings.TrimLeft(raw, "/")
	if !isProviderHTTPMediaURL(media) {
		return ""
	}
	return media
}

func huangguoWebEpisodes(plain string) ([]huangguoWebEpisode, int) {
	free := 0
	if match := reHuangguoWebFree.FindStringSubmatch(plain); len(match) > 1 {
		if value, err := strconv.Atoi(match[1]); err == nil {
			free = value
		}
	}
	match := reHuangguoWebEpisodes.FindStringSubmatch(plain)
	if len(match) < 2 {
		return nil, free
	}
	var episodes []huangguoWebEpisode
	if err := json.Unmarshal([]byte(match[1]), &episodes); err != nil {
		return nil, free
	}
	return episodes, free
}

func (d *Downloader) resolveHuangguoWebMedia(ctx context.Context, task Task) (providerMedia, error) {
	_, sourceID, valid := splitProviderDramaID(task.DramaID)
	if !valid || !validHuangguoWebID(sourceID) {
		return providerMedia{}, errors.New("黄果主站剧集 ID 无效")
	}
	pageURL := strings.TrimSpace(task.Chapter.PageURL)
	if pageURL == "" {
		hosts := d.huangguoPublishHosts(ctx)
		if len(hosts) == 0 {
			return providerMedia{}, errors.New("黄果主站暂无可用入口")
		}
		pageURL = hosts[0] + "/videos/" + url.PathEscape(sourceID)
	}
	if providerSourceForURL(pageURL) != sourceHuangguoMain {
		return providerMedia{}, errors.New("黄果主站分集地址不属于该站源")
	}
	body, err := d.fetchProviderText(ctx, pageURL, pageURL)
	if err != nil {
		return providerMedia{}, err
	}
	plain := strings.ReplaceAll(strings.ReplaceAll(body, `\"`, `"`), `\/`, `/`)
	episodes, _ := huangguoWebEpisodes(plain)
	chapterKey := task.Chapter.ID
	for _, episode := range episodes {
		if providerChapterID(sourceHuangguoMain, sourceID, episode.ChapterID) != chapterKey {
			continue
		}
		path := strings.TrimSpace(episode.URL)
		if path == "" {
			return providerMedia{}, errors.New("黄果主站该集未下发媒体地址")
		}
		media := huangguoWebMediaURL(pageURL, path)
		if media == "" {
			return providerMedia{}, errors.New("黄果主站未提供可核实的播放地址")
		}
		return providerMedia{URL: media, Referer: pageURL}, nil
	}
	return providerMedia{}, errors.New("黄果主站详情中未找到该集")
}

// --- migrated from provider_huangguo_worker.go ---
var huangguoWorkerBases = []string{
	"http://hghub-api.gyqingfengxulai.workers.dev",
	"https://hg1.tommyfac.ccwu.cc",
	"https://hg3.tommyfac.ccwu.cc",
	"https://api.tommyfac.ccwu.cc",
}

const huangguoWorkerBaseURL = "http://hghub-api.gyqingfengxulai.workers.dev"

// huangguoWorkerCandidates 返回候选入口：配置优先，其次内置多域名。
func (d *Downloader) huangguoWorkerCandidates() []string {
	if value := strings.TrimSpace(d.cfg.HuangguoWorkerURL); value != "" {
		return []string{strings.TrimRight(value, "/")}
	}
	out := make([]string, 0, len(huangguoWorkerBases))
	out = append(out, huangguoWorkerBases...)
	return out
}

func (d *Downloader) huangguoWorkerBase() string {
	if value := strings.TrimSpace(d.cfg.HuangguoWorkerURL); value != "" {
		return strings.TrimRight(value, "/")
	}
	return huangguoWorkerBaseURL
}

type huangguoWorkerPlayback struct {
	Status int `json:"status"`
	Data   struct {
		Streams struct {
			Main    string `json:"main"`
			Hevc    string `json:"hevc"`
			Preview string `json:"preview"`
		} `json:"streams"`
	} `json:"data"`
	Streams struct {
		Main    string `json:"main"`
		Hevc    string `json:"hevc"`
		Preview string `json:"preview"`
	} `json:"streams"`
}

// resolveHuangguoWorkerPlayback 通过 Worker 取某一集的播放地址。
func (d *Downloader) resolveHuangguoWorkerPlayback(ctx context.Context, sourceID string, sequence int) (providerMedia, error) {
	sourceID = strings.Trim(strings.TrimSpace(sourceID), "/")
	if sourceID == "" {
		return providerMedia{}, fmt.Errorf("黄果剧集 ID 为空")
	}
	if sequence < 1 {
		sequence = 1
	}
	var (
		body    []byte
		lastErr error
		base    string
	)
	for _, candidate := range d.huangguoWorkerCandidates() {
		target := fmt.Sprintf("%s/api/videos/%s/episodes/%d/playback", candidate, sourceID, sequence)
		body, lastErr = d.huangguoWorkerPost(ctx, target)
		if lastErr == nil {
			base = candidate
			break
		}
	}
	if lastErr != nil {
		return providerMedia{}, lastErr
	}
	var payload huangguoWorkerPlayback
	if err := json.Unmarshal(body, &payload); err != nil {
		return providerMedia{}, fmt.Errorf("黄果 Worker 返回了无效 JSON")
	}
	streams := payload.Data.Streams
	if streams.Main == "" && streams.Hevc == "" && streams.Preview == "" {
		streams = payload.Streams
	}
	media := strings.TrimSpace(streams.Main)
	if media == "" {
		media = strings.TrimSpace(streams.Hevc)
	}
	if media == "" {
		media = strings.TrimSpace(streams.Preview)
	}
	if media == "" {
		return providerMedia{}, fmt.Errorf("黄果 Worker 未下发该集地址")
	}
	return providerMedia{URL: media, Referer: base + "/"}, nil
}

func (d *Downloader) huangguoWorkerPost(ctx context.Context, target string) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, target, strings.NewReader("{}"))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", userAgent)
	response, err := d.huangguoWorkerDo(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("黄果 Worker HTTP %d", response.StatusCode)
	}
	data, readErr := io.ReadAll(io.LimitReader(response.Body, providerMaxBodyBytes+1))
	if readErr != nil {
		return nil, readErr
	}
	if int64(len(data)) > providerMaxBodyBytes {
		return nil, fmt.Errorf("黄果 Worker 响应超出上限")
	}
	return data, nil
}

// huangguoWorkerDo 发出请求：本地 DNS 对 workers.dev 被污染，
// 用 DoH 解析出的真实 IP 建连，Host 头与 TLS SNI 仍是原域名。
func (d *Downloader) huangguoWorkerDo(request *http.Request) (*http.Response, error) {
	client := &http.Client{
		Timeout: 20 * time.Second,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
				host, port, splitErr := net.SplitHostPort(address)
				if splitErr != nil {
					return nil, splitErr
				}
				ips, lookupErr := resolveModuleDNS(ctx, host)
				if lookupErr != nil || len(ips) == 0 {
					return (&net.Dialer{Timeout: 8 * time.Second}).DialContext(ctx, network, address)
				}
				dialer := &net.Dialer{Timeout: 8 * time.Second}
				var lastErr error
				for _, ip := range ips {
					conn, dialErr := dialer.DialContext(ctx, network, net.JoinHostPort(ip, port))
					if dialErr == nil {
						return conn, nil
					}
					lastErr = dialErr
				}
				return nil, lastErr
			},
		},
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("黄果 Worker 请求失败: %w", err)
	}
	return response, nil
}

// huangguoWorkerSequence 从章节取集数，取不到时回落到 1。
func huangguoWorkerSequence(chapter Chapter, index int) int {
	if value, err := strconv.Atoi(chapter.EpisodeString(index)); err == nil && value > 0 {
		return value
	}
	return 1
}

// ---- Worker 详情：官方 API 给出真实分集，比抓页面可靠 ----

type huangguoWorkerDetail struct {
	Data struct {
		ID           json.Number `json:"id"`
		Title        string `json:"title"`
		Description  string `json:"description"`
		Cover        string `json:"cover"`
		EpisodeCount int    `json:"episode_count"`
		Episodes     []struct {
			EpNum    int    `json:"ep_num"`
			Title    string `json:"title"`
			Duration int    `json:"duration"`
			Status   int    `json:"status"`
		} `json:"episodes"`
	} `json:"data"`
}

// fetchHuangguoWorkerDetail 通过 Worker 拿官方详情与分集。
// 页面抓取会把「相关推荐」误当分集（实测 21 集的剧解析出 44 条），
// 官方 API 只返回真实分集，所以优先用它。
func (d *Downloader) fetchHuangguoWorkerDetail(ctx context.Context, sourceID string) (Drama, []Chapter, error) {
	sourceID = strings.Trim(strings.TrimSpace(sourceID), "/")
	if sourceID == "" {
		return Drama{}, nil, fmt.Errorf("黄果剧集 ID 为空")
	}
	var (
		body    []byte
		lastErr error
	)
	for _, candidate := range d.huangguoWorkerCandidates() {
		target := fmt.Sprintf("%s/api/videos/%s", candidate, sourceID)
		body, lastErr = d.huangguoWorkerGet(ctx, target)
		if lastErr == nil {
			break
		}
	}
	if lastErr != nil {
		return Drama{}, nil, lastErr
	}
	var payload huangguoWorkerDetail
	if err := json.Unmarshal(body, &payload); err != nil {
		return Drama{}, nil, fmt.Errorf("黄果 Worker 详情不是有效 JSON")
	}
	detail := payload.Data
	if len(detail.Episodes) == 0 {
		return Drama{}, nil, fmt.Errorf("黄果 Worker 详情没有分集")
	}
	title := strings.TrimSpace(detail.Title)
	if title == "" {
		title = sourceID
	}
	drama := Drama{
		ID: providerDramaID(sourceHuangguoAI, sourceID), Source: sourceHuangguoAI,
		SourceID: sourceID, Title: title, Name: title,
		Desc: detail.Description, Intro: detail.Description,
		Cover: detail.Cover, CoverURL: detail.Cover,
		ChannelName: "huangguoai.com",
	}
	var chapters []Chapter
	for _, episode := range detail.Episodes {
		sequence := episode.EpNum
		if sequence <= 0 {
			sequence = len(chapters) + 1
		}
		chapterTitle := fmt.Sprintf("第%d集", sequence)
		chapters = append(chapters, Chapter{
			ID:             providerChapterID(sourceHuangguoAI, sourceID, strconv.Itoa(sequence)),
			Source:         sourceHuangguoAI,
			Title:          chapterTitle,
			CurrentEpisode: rawEpisode(sequence),
		})
	}
	drama.Episodes = len(chapters)
	return drama, chapters, nil
}

func (d *Downloader) huangguoWorkerGet(ctx context.Context, target string) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", userAgent)
	response, err := d.huangguoWorkerDo(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("黄果 Worker HTTP %d", response.StatusCode)
	}
	data, readErr := io.ReadAll(io.LimitReader(response.Body, providerMaxBodyBytes+1))
	if readErr != nil {
		return nil, readErr
	}
	if int64(len(data)) > providerMaxBodyBytes {
		return nil, fmt.Errorf("黄果 Worker 响应超出上限")
	}
	return data, nil
}

func (d *Downloader) GetHuangguoChapters(ctx context.Context, source, sourceID string) (string, []Chapter, error) {
	switch canonicalProviderSource(source) {
	case sourceHuangdou:
		return d.fetchHuangdouChapters(ctx, sourceID)
	case sourceHuangju:
		drama, chapters, err := d.fetchHuangjuDetail(ctx, sourceID)
		return drama.DisplayTitle(), chapters, err
	case sourceYeguo:
		drama, chapters, err := d.fetchYeguoDetail(ctx, sourceID)
		return drama.DisplayTitle(), chapters, err
	case sourceDSD:
		drama, chapters, err := d.fetchDSDDetail(ctx, sourceID)
		return drama.DisplayTitle(), chapters, err
	case sourceHanxiaoquan:
		drama, chapters, err := d.fetchHanxiaoquanDetail(ctx, sourceID)
		return drama.DisplayTitle(), chapters, err
	case sourceHuangguoMain:
		drama, chapters, err := d.fetchHuangguoWebDetail(ctx, sourceID)
		return drama.DisplayTitle(), chapters, err
	case sourceHuangguoSelected, sourceHuangguoWelfare:
		drama, chapters, err := d.fetchHuangguoPortalDetail(ctx, source, sourceID)
		return drama.DisplayTitle(), chapters, err
	case sourceCloudFront:
		return d.fetchLegacyChapters(ctx, sourceID)
	default:
		if moduleSource(source) != nil {
			drama, chapters, err := d.moduleDetail(ctx, source, sourceID)
			return drama.DisplayTitle(), chapters, err
		}
		return "", nil, fmt.Errorf("unsupported provider source: %s", source)
	}
}
