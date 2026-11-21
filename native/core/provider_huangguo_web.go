package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// 黄果主站（huangguo-main）原生站源。
//
// 地址发布页 hgdhy11.com/way-home 会更新可用入口，实测其中「主站」按钮指向
// falpius.com，备用线路为 whmxun.com（两者首页字节一致，属同一站点）。剧集路径：
//
//	GET /videos/{标识}     详情，页面内嵌 RSC 数据，含 episodes 数组
//
// episodes 每项字段：chapterId、episode、playTime、title、url（相对媒体主机的 .m3u8 路径）；
// 同一段数据里还有 freeEpisodeCount（免费集数）。媒体主机取自页面封面 URL 的主机
// （实测 zzzznnn.lkkwip.cn），拼上 url 才是播放地址。
//
// 播放列表实测直接请求返回 HTTP 400，说明上游对请求仍有限制（签名/来源校验），
// 因此本实现只按页面下发的相对路径拼地址；实际可播性由播放器与站点返回决定。
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

func huangguoWebParseCatalog(rawHTML string) []Drama {
	seen := map[string]bool{}
	items := make([]Drama, 0, 32)
	for _, match := range reHuangguoWebVideo.FindAllStringSubmatch(rawHTML, -1) {
		slug := match[2]
		title := cleanText(match[3])
		if !validHuangguoWebID(slug) || title == "" || seen[slug] {
			continue
		}
		seen[slug] = true
		items = append(items, Drama{
			ID:          providerDramaID(sourceHuangguoMain, slug),
			Source:      sourceHuangguoMain,
			SourceID:    slug,
			Title:       title,
			Name:        title,
			ChannelName: "falpius.com",
		})
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
