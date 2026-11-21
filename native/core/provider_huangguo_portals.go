package core

import (
	"context"
	"errors"
	"fmt"
	"html"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

const (
	huangguoSelectedOrigin = "https://d2teten6pcf3ig.cloudfront.net"
	huangguoWelfareOrigin  = "https://d24dalcptr65cw.cloudfront.net"
)

var (
	selectedCard    = regexp.MustCompile(`(?is)<h3\b[^>]*class="[^"]*card__title[^"]*"[^>]*>\s*<a\b[^>]*href="/play/([a-zA-Z0-9_-]{2,64})/"[^>]*>(.*?)</a>`)
	selectedEpisode = regexp.MustCompile(`(?is)<a\b([^>]*class="[^"]*play-ep[^"]*"[^>]*)>(.*?)</a>`)
	welfareCard     = regexp.MustCompile(`(?is)<a\b[^>]*href="/detail/([a-zA-Z0-9_-]{2,64})/?"[^>]*>\s*<img\b[^>]*alt="([^"]+)"`)
	welfareEpisode  = regexp.MustCompile(`(?is)<a\b([^>]*class="[^"]*detail-episode[^"]*"[^>]*)>(.*?)</a>`)
	portalAttr      = regexp.MustCompile(`(?is)([a-z-]+)="([^"]*)"`)
	portalTitle     = regexp.MustCompile(`(?is)<h1\b[^>]*>(.*?)</h1>`)
	portalPlayer    = regexp.MustCompile(`(?is)<div\b[^>]*data-play-player\b[^>]*>`)
	welfareMedia    = regexp.MustCompile(`(?is)<div\b[^>]*class="[^"]*dplayer-video[^"]*"[^>]*data-url="([^"]+)"`)
)

func portalAttributes(tag string) map[string]string {
	result := map[string]string{}
	for _, m := range portalAttr.FindAllStringSubmatch(tag, -1) {
		result[strings.ToLower(m[1])] = html.UnescapeString(m[2])
	}
	return result
}

func portalCatalog(body, source string) []Drama {
	seen := map[string]bool{}
	var result []Drama
	var matches [][]string
	if source == sourceHuangguoSelected {
		matches = selectedCard.FindAllStringSubmatch(body, -1)
	} else {
		matches = welfareCard.FindAllStringSubmatch(body, -1)
	}
	for _, m := range matches {
		id, title := m[1], cleanText(m[2])
		if title == "" || seen[id] {
			continue
		}
		seen[id] = true
		result = append(result, Drama{ID: providerDramaID(source, id), Source: source, SourceID: id, Title: title, Name: title})
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
