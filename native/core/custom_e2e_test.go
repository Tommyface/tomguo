package core

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// 端到端：注入一个运行时苹果CMS配置 → 用假站点 HTML 验证目录/详情解析链路。
// 证明动态源不是空壳，而是复用现有通用解析器真正产出剧集与分集。
func TestCustomMaccmsEndToEnd(t *testing.T) {
	const listHTML = `<ul class="vodlist"><li>` +
		`<a href="/voddetail/101.html" title="测试短剧"><img data-original="/cover.jpg"></a>` +
		`<h4><a href="/voddetail/101.html">测试短剧</a></h4>` +
		`</li></ul>`
	const detailHTML = `<div class="detail"><h1>测试短剧</h1>` +
		`<div class="intro">这是一部测试剧</div>` +
		`<a href="/vodplay/101-1-1.html">第1集</a>` +
		`<a href="/vodplay/101-1-2.html">第2集</a></div>`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		switch {
		case strings.HasPrefix(r.URL.Path, "/vodtype/"):
			fmt.Fprint(w, listHTML)
		case strings.HasPrefix(r.URL.Path, "/voddetail/"):
			fmt.Fprint(w, detailHTML)
		case strings.HasPrefix(r.URL.Path, "/vodplay/"):
			fmt.Fprint(w, `<script>var player_aaaa={"url":"/video/1.m3u8"}</script>`)
		default:
			fmt.Fprint(w, listHTML)
		}
	}))
	defer srv.Close()

	cfg, err := normalizeCustomConfig(customSourceConfig{
		ID:                "custom_e2e",
		Name:              "测试站",
		BaseURL:           srv.URL,
		Type:              "maccms",
		DetailPathPattern: `^/voddetail/([0-9]+)\.html$`,
		PlayPathPattern:   `^/vodplay/[0-9]+-[0-9]+-[0-9]+\.html$`,
		DetailURLTemplate: `{base}/voddetail/{id}.html`,
		ListURLTemplate:   `{base}/vodtype/1-{page}.html`,
	})
	if err != nil {
		t.Fatal(err)
	}

	d := &Downloader{
		cfg:           defaultConfig(),
		client:        srv.Client(),
		limiter:       newRequestLimiter(100, 0),
		customSources: newCustomRegistry(),
	}
	if _, err := d.customSources.replace([]customSourceConfig{cfg}); err != nil {
		t.Fatal(err)
	}

	// 1) 目录解析
	items, _, err := d.fetchMaccmsCatalogPage(context.Background(), "custom_e2e", 1, "")
	if err != nil {
		t.Fatalf("目录解析失败: %v", err)
	}
	if len(items) == 0 {
		t.Fatal("目录解析应返回剧集")
	}
	found := false
	for _, it := range items {
		if strings.Contains(it.DisplayTitle(), "测试短剧") {
			found = true
		}
	}
	if !found {
		t.Fatalf("目录未找到测试剧: %+v", items)
	}

	// 2) 详情解析
	_, chapters, err := d.fetchMaccmsDetail(context.Background(), "custom_e2e", "101")
	if err != nil {
		t.Fatalf("详情解析失败: %v", err)
	}
	if len(chapters) == 0 {
		t.Fatal("详情应返回分集")
	}
	// 播放地址占位 scheme 正确（播放时再解析）。
	hasPlayScheme := false
	for _, ch := range chapters {
		if strings.HasPrefix(ch.VideoURL, "maccms-play://") {
			hasPlayScheme = true
		}
	}
	if !hasPlayScheme {
		t.Fatalf("分集应带 maccms-play:// 占位: %+v", chapters)
	}
}
