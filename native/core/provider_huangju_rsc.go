package core

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

var (
	huangjuRSCChunkPattern    = regexp.MustCompile(`(?s)self\.__next_f\.push\(\[1,"(.*?)"\]\)</script>`)
	huangjuJSONLDPattern      = regexp.MustCompile(`(?s)<script type="application/ld\+json"[^>]*>(.*?)</script>`)
	huangjuRSCJSONLDPattern   = regexp.MustCompile(`(?s)"__html":"((?:[^"\\]|\\.)*)"`)
	huangjuCardPattern        = regexp.MustCompile(`\{[^{}]*?"totalEpisodes":\d+[^{}]*\}`)
	huangjuFeedEpisodePattern = regexp.MustCompile(`(?s)"drama":\{"id":(\d+).*?"episode":\{"id":(\d+),"epNo":(\d+)[^}]*\}`)
	huangjuEpisodeLinkPattern = regexp.MustCompile(`/drama/[^/"]+/ep-(\d+)`)
	huangjuHrefSlugPattern    = regexp.MustCompile(`drama=([^&#]+)`)
)

type huangjuTheaterItem struct {
	sourceID string
	title    string
	desc     string
	cover    string
	score    string
	status   string
	total    int
}

type huangjuSeries struct {
	title string
	desc  string
	cover string
	total int
}

func huangjuRSC(html string) string {
	var builder strings.Builder
	for _, match := range huangjuRSCChunkPattern.FindAllStringSubmatch(html, -1) {
		var chunk string
		if json.Unmarshal([]byte(`"`+match[1]+`"`), &chunk) == nil {
			builder.WriteString(chunk)
		}
	}
	return builder.String()
}

func huangjuJSONLD(html, rsc string) []map[string]any {
	var rows []map[string]any
	appendValue := func(value any) {
		switch value := value.(type) {
		case []any:
			for _, entry := range value {
				if row, ok := entry.(map[string]any); ok {
					rows = append(rows, row)
				}
			}
		case map[string]any:
			rows = append(rows, value)
		}
	}
	for _, match := range huangjuJSONLDPattern.FindAllStringSubmatch(html, -1) {
		var decoded any
		if json.Unmarshal([]byte(strings.TrimSpace(match[1])), &decoded) == nil {
			appendValue(decoded)
		}
	}
	if len(rows) == 0 && rsc != "" {
		for _, match := range huangjuRSCJSONLDPattern.FindAllStringSubmatch(rsc, -1) {
			var text string
			if json.Unmarshal([]byte(`"`+match[1]+`"`), &text) != nil {
				continue
			}
			var decoded any
			if json.Unmarshal([]byte(text), &decoded) == nil {
				appendValue(decoded)
			}
		}
	}
	return rows
}

func huangjuString(value any) string {
	switch value := value.(type) {
	case string:
		return strings.TrimSpace(value)
	case []any:
		for _, entry := range value {
			if text, ok := entry.(string); ok && strings.TrimSpace(text) != "" {
				return strings.TrimSpace(text)
			}
		}
	}
	return ""
}

func huangjuCanonicalSlug(href, slug string, id int) string {
	if match := huangjuHrefSlugPattern.FindStringSubmatch(href); match != nil {
		value := strings.TrimSpace(match[1])
		if validHuangjuID(value) {
			return value
		}
	}
	slug = strings.TrimSpace(slug)
	if slug == "" {
		return ""
	}
	if id > 0 {
		suffix := "-" + strconv.Itoa(id)
		if !strings.HasSuffix(slug, suffix) {
			slug += suffix
		}
	}
	return slug
}

func huangjuTheaterItems(rsc string) []huangjuTheaterItem {
	var items []huangjuTheaterItem
	seen := map[string]bool{}
	for _, match := range huangjuCardPattern.FindAllString(rsc, -1) {
		var row map[string]any
		if json.Unmarshal([]byte(match), &row) != nil {
			continue
		}
		id, _ := huangjuInteger(row["id"], 100000000)
		sourceID := huangjuCanonicalSlug(mapString(row, "href"), mapString(row, "slug"), id)
		title := strings.TrimSpace(mapString(row, "title"))
		if !validHuangjuID(sourceID) || title == "" || seen[sourceID] {
			continue
		}
		seen[sourceID] = true
		total, _ := huangjuInteger(row["totalEpisodes"], 100000)
		desc := mapString(row, "subtitle")
		if desc == "" {
			desc = mapString(row, "summary")
		}
		cover := mapString(row, "image")
		if cover == "" {
			cover = mapString(row, "cover")
		}
		items = append(items, huangjuTheaterItem{
			sourceID: sourceID, title: truncate(title, 256), desc: truncate(desc, 12000),
			cover: cover, score: mapString(row, "score"), status: mapString(row, "status"), total: total,
		})
	}
	return items
}

func huangjuDramaFromTheater(item huangjuTheaterItem, site string) Drama {
	status := item.status
	if status == "completed" {
		status = "finished"
	}
	if status != "finished" && status != "ongoing" {
		status = ""
	}
	var tags []string
	score := item.score
	if value, err := strconv.ParseFloat(score, 64); err == nil && value > 0 && value <= 10 {
		tags = append(tags, "评分 "+score)
	} else {
		score = ""
	}
	cover := providerCoverAddress(item.cover, site+"/")
	return Drama{
		ID: providerDramaID(sourceHuangju, item.sourceID), Source: sourceHuangju, SourceID: item.sourceID,
		Title: truncate(item.title, 256), Name: truncate(item.title, 256),
		Desc: item.desc, Intro: item.desc, Cover: cover, CoverURL: cover,
		TotalEpisode: item.total, EpisodeCount: item.total,
		Category: "短剧", ChannelName: "剧果", Tags: tags, ReleaseStatus: status, Score: score,
	}
}

func huangjuFeedEpisode(rsc string, expected int) (string, int, bool) {
	for _, match := range huangjuFeedEpisodePattern.FindAllStringSubmatch(rsc, -1) {
		number, err := strconv.Atoi(match[3])
		if err != nil || number != expected || !validHuangjuID(match[2]) {
			continue
		}
		return match[2], number, true
	}
	return "", 0, false
}

func huangjuEpisodeNumbers(html string) []int {
	seen := map[int]bool{}
	var numbers []int
	for _, match := range huangjuEpisodeLinkPattern.FindAllStringSubmatch(html, -1) {
		number, err := strconv.Atoi(match[1])
		if err != nil || number <= 0 || number > 100000 || seen[number] {
			continue
		}
		seen[number] = true
		numbers = append(numbers, number)
	}
	sort.Ints(numbers)
	return numbers
}

func huangjuSeriesFromJSONLD(rows []map[string]any) huangjuSeries {
	for _, row := range rows {
		kind, _ := row["@type"].(string)
		if kind != "TVSeries" && kind != "VideoObject" {
			continue
		}
		series := huangjuSeries{
			title: huangjuString(row["name"]),
			desc:  huangjuString(row["description"]),
			cover: huangjuString(row["image"]),
		}
		if total, ok := huangjuInteger(row["numberOfEpisodes"], 100000); ok {
			series.total = total
		}
		if series.title != "" {
			return series
		}
	}
	return huangjuSeries{}
}

func (d *Downloader) huangjuSiteURL(route string) string {
	return strings.TrimRight(d.providerBaseURL(sourceHuangju), "/") + route
}

func (d *Downloader) fetchHuangjuRSCCatalog(ctx context.Context) ([]Drama, error) {
	site := d.providerBaseURL(sourceHuangju)
	html, err := d.fetchProviderText(ctx, d.huangjuSiteURL("/theater"), site+"/")
	if err != nil {
		return nil, err
	}
	items := huangjuTheaterItems(huangjuRSC(html))
	if len(items) == 0 {
		return nil, errors.New("剧果目录页未包含剧集")
	}
	dramas := make([]Drama, 0, len(items))
	for _, item := range items {
		dramas = append(dramas, huangjuDramaFromTheater(item, site))
	}
	return dramas, nil
}

func (d *Downloader) fetchHuangjuRSCEpisode(ctx context.Context, sourceID, site string, number int) (string, bool) {
	address := d.huangjuSiteURL("/?drama=" + url.QueryEscape(sourceID) + "&ep=" + strconv.Itoa(number))
	html, err := d.fetchProviderText(ctx, address, site+"/")
	if err != nil {
		return "", false
	}
	id, _, ok := huangjuFeedEpisode(huangjuRSC(html), number)
	return id, ok
}

func (d *Downloader) fetchHuangjuRSCDetail(ctx context.Context, sourceID string) (Drama, []Chapter, error) {
	site := d.providerBaseURL(sourceHuangju)
	html, err := d.fetchProviderText(ctx, d.huangjuSiteURL("/drama/"+url.PathEscape(sourceID)), site+"/")
	if err != nil {
		return Drama{}, nil, err
	}
	rsc := huangjuRSC(html)
	series := huangjuSeriesFromJSONLD(huangjuJSONLD(html, rsc))
	if series.title == "" {
		return Drama{}, nil, errors.New("剧果详情页解析失败")
	}
	total := series.total
	if total <= 0 {
		if numbers := huangjuEpisodeNumbers(html); len(numbers) > 0 {
			total = numbers[len(numbers)-1]
		}
	}
	if total <= 0 || total > 500 {
		return Drama{}, nil, errors.New("剧果详情集数无效")
	}
	chapters := make([]Chapter, 0, total)
	seen := map[string]bool{}
	for number := 1; number <= total; number++ {
		if err := ctx.Err(); err != nil {
			return Drama{}, nil, err
		}
		id, ok := d.fetchHuangjuRSCEpisode(ctx, sourceID, site, number)
		if !ok || seen[id] {
			continue
		}
		seen[id] = true
		chapters = append(chapters, Chapter{
			ID: providerChapterID(sourceHuangju, sourceID, id), Source: sourceHuangju,
			Title: "第 " + strconv.Itoa(number) + " 集", CurrentEpisode: json.RawMessage(strconv.Itoa(number)),
			VideoURL: "huangju-play://" + url.PathEscape(id), Referer: site + "/",
		})
	}
	if len(chapters) == 0 {
		return Drama{}, nil, errors.New("剧果详情页未返回分集")
	}
	cover := providerCoverAddress(series.cover, site+"/")
	drama := Drama{
		ID: providerDramaID(sourceHuangju, sourceID), Source: sourceHuangju, SourceID: sourceID,
		Title: truncate(series.title, 256), Name: truncate(series.title, 256),
		Desc: truncate(series.desc, 12000), Intro: truncate(series.desc, 12000),
		Cover: cover, CoverURL: cover, TotalEpisode: total, EpisodeCount: total,
		Category: "短剧", ChannelName: "剧果",
	}
	return drama, chapters, nil
}
