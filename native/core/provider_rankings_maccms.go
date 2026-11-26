package core

// 苹果CMS 系站源的榜单解析（0.8.25 第三批）。
//
// 网果 /label/top.html 与发果 /xzyxvt/<slug>.html 都是普通卡片列表页，
// 直接复用 maccmsCards；名次按页面出现顺序推（这些站不标注序号）。
// 囧次元 label/hot.html 特殊：是「日漫榜 + 剧场榜」两张纯文字 paper 榜，
// 每条带序号但没有封面，单独解析。

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// buildMaccmsRanking 把 maccmsCards 的结果包装成榜单页。
// 页面上不写名次，按出现顺序编号；同一部剧去重后重排。
func buildMaccmsRanking(items []Drama, board rankingBoard) (rankingPage, error) {
	if len(items) == 0 {
		return rankingPage{}, fmt.Errorf("%s 榜单未解析到内容，站点页面可能已变化", board.Name)
	}
	result := rankingPage{Items: make([]rankingItem, 0, len(items)), TotalPages: 1}
	seen := map[string]bool{}
	for _, item := range items {
		// maccmsCards 认卡片的锚点是详情链接，ID 恒非空；空 ID / 空标题
		// 只可能来自模板残片，直接跳过。
		if item.ID == "" || item.Title == "" || seen[item.ID] {
			continue
		}
		seen[item.ID] = true
		result.Items = append(result.Items, rankingItem{Rank: len(result.Items) + 1, Drama: item})
	}
	if len(result.Items) == 0 {
		return rankingPage{}, fmt.Errorf("%s 榜单条目全部无效", board.Name)
	}
	return result, nil
}

// buildHuangjuRanking / buildDuanju51Ranking：这两个站点的目录接口
// （黄剧 /dramas、51 的 /ranks/ 榜单页）已经产出 Drama 列表，只补名次。
// 51 改版后是单页榜，HasMore / TotalPages 由调用方强制收敛成 false / 1。
func buildHuangjuRanking(items []Drama, board rankingBoard, page int) (rankingPage, error) {
	if len(items) == 0 {
		return rankingPage{}, errors.New("黄剧榜单未返回内容，站点可能暂不可用")
	}
	pageSize := 20
	result := rankingPage{
		Items:      make([]rankingItem, 0, len(items)),
		HasMore:    len(items) >= pageSize,
		TotalPages: page + 1,
	}
	seen := map[string]bool{}
	for index, item := range items {
		if seen[item.ID] {
			continue
		}
		seen[item.ID] = true
		rank := (page-1)*pageSize + index + 1
		metric := ""
		if item.Heat != "" {
			metric = item.Heat + " 热度"
		}
		result.Items = append(result.Items, rankingItem{Rank: rank, Drama: item, Metric: metric})
	}
	if len(result.Items) == 0 {
		return rankingPage{}, errors.New("黄剧榜单条目全部无效")
	}
	return result, nil
}


// ---------------------------------------------------------------- E站新番榜

// E站榜单条目：`<a href="/Index/bangumi/81018.html"...><p><span>1</span>
// <strong>标题</strong></p><img src="封面"/><strong>简介</strong></a>`。
// 按 <a> 整块切，块内再取序号 / 片名 / 封面 —— 只匹配前半段会丢封面。
var (
	ezdmwRankingAnchor = regexp.MustCompile(`(?is)<a\s+href="(/Index/bangumi/[0-9]+\.html)"[^>]*>(.*?)</a>`)
	ezdmwRankingRank   = regexp.MustCompile(`(?is)<p>\s*<span>([0-9]{1,3})</span>\s*<strong[^>]*>([^<]{1,120})</strong>`)
	ezdmwRankingImg    = regexp.MustCompile(`(?is)<img[^>]+src="([^"]+)"`)
)

// parseEZDMWRanking 解析 E站 fan_ranking 页。
//
// 页面顶部的月份链接（2026年10月 / 7月 / 1月）只做展示，不参与解析 ——
// 不带 name 参数访问 /Index/fan_ranking.html 只会回「参数有误」（12 字节），
// 所以抓取侧固定带 `?atype=hor`（站点默认编排的动漫总榜，实测 30 条）。
func parseEZDMWRanking(body, base string) (rankingPage, error) {
	failure := errors.New("E站新番榜格式或页面已变化，请稍后重试")
	result := rankingPage{Items: make([]rankingItem, 0, 32), TotalPages: 1}
	seen := map[string]bool{}
	previous := 0
	for _, match := range ezdmwRankingAnchor.FindAllStringSubmatch(body, -1) {
		id := strings.TrimSuffix(strings.TrimPrefix(match[1], "/Index/bangumi/"), ".html")
		inner := match[2]
		head := ezdmwRankingRank.FindStringSubmatch(inner)
		if len(head) < 3 || seen[id] {
			continue
		}
		rank, err := strconv.Atoi(strings.TrimSpace(head[1]))
		title := strings.TrimSpace(head[2])
		if err != nil || rank <= previous || title == "" {
			return rankingPage{}, failure
		}
		seen[id], previous = true, rank
		drama := Drama{
			ID:          providerDramaID(sourceEZDMW, id),
			Source:      sourceEZDMW,
			SourceID:    id,
			Title:       title,
			Name:        title,
			ChannelName: "E站",
		}
		if image := ezdmwRankingImg.FindStringSubmatch(inner); len(image) > 1 {
			if cover := providerCoverAddress(image[1], base+"/"); cover != "" {
				drama.Cover, drama.CoverURL = cover, cover
			}
		}
		result.Items = append(result.Items, rankingItem{Rank: rank, Drama: drama})
	}
	if len(result.Items) == 0 {
		return rankingPage{}, failure
	}
	return result, nil
}

// ---------------------------------------------------------------- 囧次元 paper 榜

