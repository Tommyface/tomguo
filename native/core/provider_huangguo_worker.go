package core

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// 黄果官方 API 在大陆被 DNS 污染（本地解析与 DoH 结果完全不一致），
// 内置站源直接抓 huangguoai.com 会拿不到播放地址。
// 这里走用户自建的 Cloudflare Worker 反代，接口契约与 hghub-clone 一致：
//
//	POST {worker}/api/videos/{id}/episodes/{ep}/playback   体必须是 {}
//	响应 data.streams.main 为该集 HLS 清单，任意一集都可取
// 多入口轮换。实测（2026-11-21）：
//   workers.dev 的 HTTP(80) 通，0.6 秒；
//   自定义域名 hg1/hg3/api.tommyfac.ccwu.cc 的 80 与 443 都被 RST 阻断（SNI 过滤）。
// 所以 workers.dev 排第一，自定义域名作后备（换个网络环境可能反过来）。
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
