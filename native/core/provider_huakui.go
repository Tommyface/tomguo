package core

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// 花魁短剧（huakui）原生站源。
//
// 花魁有三个入口域名 huakui1.com / huakui2.com / huakui3.com，实测三者为同一份
// Next.js 导航页（54,349 字节、无剧集路由、无 /api），只负责导流；真正的剧集站在
// huakui.net（导航页「永久域名」），页面结构如下：
//
//	GET /drama/{剧标识}/            详情，分集链接形如 /drama/{剧标识}/{章节ID}/
//	GET /drama/{剧标识}/{章节ID}/    播放页
//
// 播放页的播放器节点带权限标记：解锁可看的集是 data-can-watch 非 0，锁定的集
// data-can-watch="0" 且 class 含 hk-playback-player--locked。全站分集走
// /api/playlet/chapter/unlock 与 /api/coin/buy 解锁，属于付费链路，因此本实现
// 只解析已下发媒体地址的分集，锁定的集如实报权限限制，绝不用试看冒充正片。
var (
	huakuiSiteBaseURL = "https://huakui.net"
)

var (
	reHuakuiDramaLink = regexp.MustCompile(`(?is)<a\b[^>]*href="(/drama/([a-z0-9]{4,40})/)"[^>]*>(.*?)</a>`)
	reHuakuiEpisode   = regexp.MustCompile(`(?is)<a\b[^>]*href="(/drama/([a-z0-9]{4,40})/(\d{1,8})/)"[^>]*>(.*?)</a>`)
	reHuakuiTitle     = regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)
	reHuakuiH1        = regexp.MustCompile(`(?is)<h1[^>]*>(.*?)</h1>`)
	reHuakuiOgImage   = regexp.MustCompile(`(?is)<meta\b[^>]*(?:property|name)=["']og:image["'][^>]*content=["']([^"']+)["']`)
	reHuakuiChapter   = regexp.MustCompile(`(?i)(?:第\s*0*(\d+)\s*集)`)
	reHuakuiPlayer    = regexp.MustCompile(`(?is)<div\b[^>]*data-play-player[^>]*>`)
	reHuakuiPlayAttr  = regexp.MustCompile(`(?is)data-(?:play-url|play-src|hls|video-src|src)=["']([^"']+)["']`)
	reHuakuiMedia     = regexp.MustCompile(`(?is)(https?://[^"'\s<>]+\.(?:m3u8|mp4)(?:\?[^"'\s<>]*)?)`)
	reHuakuiCurated   = regexp.MustCompile(`(?is)<a\b[^>]*href="/curated/dramas/([a-z0-9-]{2,40})/"[^>]*>(.*?)</a>`)
	// 播放数据在页面 RSC 载荷里：playletChapterInfo[].videoUrl 是相对路径，
	// 站点前端统一改写为同域 /h5/m3u8/<相对路径>（实测返回 #EXTM3U）。
	reHuakuiVideoURL  = regexp.MustCompile(`(?is)"videoUrl"\s*:\s*"([^"]+\.m3u8[^"]*)"`)
	reHuakuiChapterID = regexp.MustCompile(`(?is)"currentEpisode"\s*:\s*(\d{1,6})`)
)

func validHuakuiID(value string) bool {
	value = strings.TrimSpace(value)
	return value != "" && len(value) <= 64 && regexp.MustCompile(`^[a-z0-9-]+$`).MatchString(value)
}

func huakuiAbsolute(pageURL, raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	return resolveProviderURL(pageURL, raw)
}

func (d *Downloader) fetchHuakuiCatalog(ctx context.Context, category string) ([]Drama, error) {
	address := huakuiSiteBaseURL + "/"
	if category != "" {
		if !validHuakuiID(category) {
			return nil, errors.New("花魁分类标识无效")
		}
		address = huakuiSiteBaseURL + "/curated/dramas/" + url.PathEscape(category) + "/"
	}
	body, err := d.fetchProviderText(ctx, address, huakuiSiteBaseURL+"/")
	if err != nil {
		return nil, err
	}
	return huakuiParseCatalog(body, address, category), nil
}

func huakuiParseCatalog(rawHTML, pageURL, category string) []Drama {
	seen := map[string]bool{}
	items := make([]Drama, 0, 32)
	for _, match := range reHuakuiDramaLink.FindAllStringSubmatch(rawHTML, -1) {
		href, slug := match[1], match[2]
		title := cleanText(match[3])
		if title == "" || seen[slug] {
			continue
		}
		seen[slug] = true
		items = append(items, Drama{
			ID:           providerDramaID(sourceHuakui, slug),
			Source:       sourceHuakui,
			SourceID:     slug,
			Title:        title,
			Name:         title,
			CategoryName: category,
			ChannelName:  "huakui.net",
			Remark:       huakuiAbsolute(pageURL, href),
		})
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].SourceID < items[j].SourceID })
	return items
}

func (d *Downloader) fetchHuakuiCategories(ctx context.Context) ([]nativeCategory, error) {
	body, err := d.fetchProviderText(ctx, huakuiSiteBaseURL+"/", huakuiSiteBaseURL+"/")
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	items := make([]nativeCategory, 0, 12)
	for _, match := range reHuakuiCurated.FindAllStringSubmatch(body, -1) {
		id := match[1]
		name := cleanText(match[2])
		if !validHuakuiID(id) || name == "" || seen[id] {
			continue
		}
		seen[id] = true
		items = append(items, nativeCategory{ID: id, Name: name})
	}
	return items, nil
}

func (d *Downloader) fetchHuakuiDetail(ctx context.Context, sourceID string) (Drama, []Chapter, error) {
	if !validHuakuiID(sourceID) {
		return Drama{}, nil, errors.New("花魁剧集 ID 无效")
	}
	address := huakuiSiteBaseURL + "/drama/" + url.PathEscape(sourceID) + "/"
	body, err := d.fetchProviderText(ctx, address, huakuiSiteBaseURL+"/")
	if err != nil {
		return Drama{}, nil, err
	}
	title := cleanText(firstRegexGroup(reHuakuiH1, body))
	if title == "" {
		title = cleanText(firstRegexGroup(reHuakuiTitle, body))
	}
	if title == "" {
		return Drama{}, nil, errors.New("花魁详情页缺少剧名")
	}
	cover := huakuiAbsolute(address, firstRegexGroup(reHuakuiOgImage, body))
	drama := Drama{
		ID:          providerDramaID(sourceHuakui, sourceID),
		Source:      sourceHuakui,
		SourceID:    sourceID,
		Title:       title,
		Name:        title,
		Cover:       cover,
		CoverURL:    cover,
		ChannelName: "huakui.net",
		Remark:      address,
	}
	chapters := huakuiParseChapters(body, address, sourceID)
	drama.TotalEpisode = strconv.Itoa(len(chapters))
	drama.EpisodeCount = drama.TotalEpisode
	return drama, chapters, nil
}

func huakuiParseChapters(rawHTML, pageURL, sourceID string) []Chapter {
	seen := map[string]bool{}
	chapters := make([]Chapter, 0, 32)
	for _, match := range reHuakuiEpisode.FindAllStringSubmatch(rawHTML, -1) {
		if match[2] != sourceID {
			continue
		}
		chapterKey := match[3]
		if chapterKey == "" || seen[chapterKey] {
			continue
		}
		seen[chapterKey] = true
		number := len(chapters) + 1
		if labelled := episodeIndex(cleanText(match[4]), 0); labelled > 0 {
			number = labelled
		}
		chapters = append(chapters, Chapter{
			ID:             providerChapterID(sourceHuakui, sourceID, chapterKey),
			Source:         sourceHuakui,
			Title:          fmt.Sprintf("第%d集", number),
			PageURL:        huakuiAbsolute(pageURL, match[1]),
			CurrentEpisode: rawEpisode(number),
		})
	}
	sortProviderChapters(chapters)
	return uniqueChapters(chapters)
}

func (d *Downloader) resolveHuakuiMedia(ctx context.Context, task Task) (providerMedia, error) {
	_, sourceID, valid := splitProviderDramaID(task.DramaID)
	if !valid || !validHuakuiID(sourceID) {
		return providerMedia{}, errors.New("花魁剧集 ID 无效")
	}
	pageURL := strings.TrimSpace(task.Chapter.PageURL)
	if pageURL == "" {
		number := task.Chapter.EpisodeString(task.Index)
		if _, err := strconv.Atoi(number); err != nil {
			return providerMedia{}, errors.New("花魁分集序号无效")
		}
		pageURL = huakuiSiteBaseURL + "/drama/" + url.PathEscape(sourceID) + "/" + number + "/"
	}
	if providerSourceForURL(pageURL) != sourceHuakui {
		return providerMedia{}, errors.New("花魁分集地址不属于该站源")
	}
	body, err := d.fetchProviderText(ctx, pageURL, huakuiSiteBaseURL+"/")
	if err != nil {
		return providerMedia{}, err
	}
	player := reHuakuiPlayer.FindString(body)
	if player == "" {
		return providerMedia{}, errors.New("花魁分集页未提供播放器")
	}
	if strings.Contains(strings.ToLower(player), "playback-player--locked") ||
		strings.Contains(player, `data-can-watch="0"`) {
		return providerMedia{}, errors.New("花魁该集需要解锁或会员权限，未取得正片地址")
	}
	media := huakuiMediaURL(pageURL, body)
	if media == "" {
		return providerMedia{}, errors.New("花魁未返回有效的播放地址")
	}
	return providerMedia{URL: media, Referer: pageURL}, nil
}

// huakuiMediaURL 按站点自身规则把 videoUrl 转成可播地址：
// 绝对地址原样返回，相对路径改写到同域 /h5/m3u8/ 代理。
func huakuiMediaURL(pageURL, raw string) string {
	plain := strings.ReplaceAll(strings.ReplaceAll(raw, `\"`, `"`), `\/`, `/`)
	videoURL := strings.TrimSpace(firstRegexGroup(reHuakuiVideoURL, plain))
	if videoURL == "" {
		if absolute := firstRegexGroup(reHuakuiMedia, plain); absolute != "" {
			return absolute
		}
		return ""
	}
	if strings.HasPrefix(videoURL, "http://") || strings.HasPrefix(videoURL, "https://") {
		if isProviderHTTPMediaURL(videoURL) {
			return videoURL
		}
		return ""
	}
	relative := strings.TrimLeft(videoURL, "/")
	if !strings.HasPrefix(relative, "h5/m3u8/") {
		relative = "h5/m3u8/" + relative
	}
	origin := huakuiSiteBaseURL
	if parsed, err := url.Parse(pageURL); err == nil && parsed.Scheme == "https" && parsed.Host != "" {
		origin = parsed.Scheme + "://" + parsed.Host
	}
	media := origin + "/" + relative
	if !isProviderHTTPMediaURL(media) {
		return ""
	}
	return media
}

func firstRegexGroup(re *regexp.Regexp, raw string) string {
	match := re.FindStringSubmatch(raw)
	if len(match) < 2 {
		return ""
	}
	return match[1]
}
