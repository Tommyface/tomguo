package core

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newChengguoStub(t *testing.T, routes map[string]string) (*Downloader, *httptest.Server) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.URL.Path
		if r.URL.RawQuery != "" {
			key += "?" + r.URL.RawQuery
		}
		payload, ok := routes[key]
		if !ok {
			if payload, ok = routes[r.URL.Path]; !ok {
				http.NotFound(w, r)
				return
			}
		}
		encoded, err := chengguoEncrypt([]byte(`{"success":true,"data":` + payload + `}`))
		if err != nil {
			http.Error(w, "encrypt failed", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		timestamp := "1700000000"
		signature := chengguoSignature(map[string]string{"errcode": "0", "data": encoded, "timestamp": timestamp})
		_ = json.NewEncoder(w).Encode(map[string]any{"errcode": 0, "timestamp": timestamp, "data": encoded, "sign": signature})
	}))
	t.Cleanup(server.Close)
	d := &Downloader{
		cfg:           Config{ChengguoURL: server.URL, ChengguoAPIURL: server.URL},
		client:        server.Client(),
		providerHosts: map[string]string{},
		limiter:       newRequestLimiter(2, 0),
	}
	return d, server
}

func TestChengguoGatewayErrorReportsStatusBeforeReadingBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "100")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte("down"))
	}))
	defer server.Close()
	d := &Downloader{
		cfg:     Config{ChengguoURL: server.URL, ChengguoAPIURL: server.URL},
		client:  server.Client(),
		limiter: newRequestLimiter(2, 0),
	}
	_, err := d.chengguoPost(context.Background(), "/api/v1/dramas/test", map[string]any{}, false)
	if err == nil || !strings.Contains(err.Error(), "HTTP 503") || strings.Contains(err.Error(), "数据过大") {
		t.Fatalf("gateway error = %v", err)
	}
}

func TestChengguoGatewayFailureFallsBackToNextHost(t *testing.T) {
	first := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer first.Close()
	second := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		encoded, err := chengguoEncrypt([]byte(`{"success":true,"data":{"id":"fallback"}}`))
		if err != nil {
			t.Errorf("encrypt: %v", err)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": encoded})
	}))
	defer second.Close()
	d := &Downloader{
		cfg:     Config{ChengguoURL: second.URL},
		client:  first.Client(),
		limiter: newRequestLimiter(2, 0),
	}
	result, err := d.chengguoPostHosts(context.Background(), []string{first.URL, second.URL}, "/api/v1/dramas/test", map[string]any{}, false)
	if err != nil || mapString(result, "id") != "fallback" {
		t.Fatalf("fallback result = %#v, error = %v", result, err)
	}
}

func TestChengguoCipherRoundTrip(t *testing.T) {
	plain := []byte(`{"page":1,"keyword":"测试"}`)
	cipherText, err := chengguoEncrypt(plain)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	decoded, err := chengguoDecrypt(cipherText)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if string(decoded) != string(plain) {
		t.Fatalf("round trip mismatch: %q", decoded)
	}
}

func TestChengguoSignatureIsStable(t *testing.T) {
	first := chengguoSignature(map[string]string{
		"_ver": "v0", "client": "ios", "data": "abc+def=", "timestamp": "1700000000",
	})
	second := chengguoSignature(map[string]string{
		"timestamp": "1700000000", "data": "abc+def=", "client": "ios", "_ver": "v0",
	})
	if first != second {
		t.Fatalf("signature depends on map order")
	}
	if first != "bc47a7d3716e8be7934d9b732b20d891" {
		t.Fatalf("unexpected signature: %s", first)
	}
}

func TestChengguoDramaFromRow(t *testing.T) {
	row := map[string]any{
		"id": float64(558), "slug": "dj-c4875de9", "title": "发情女上司调教指南",
		"intro": "简介", "total_episode_count": float64(8), "serial_status": "ongoing",
		"tags": []any{map[string]any{"name": "都市邻里", "slug": "dushi"}},
		"cover": map[string]any{
			"encrypted":    true,
			"url":          "https://pic.example.com/cover.jpeg",
			"fallback_url": "/static/posters/up-4102.jpg",
		},
	}
	drama, err := chengguoDramaFromRow(row, "https://chengguodj.com")
	if err != nil {
		t.Fatalf("drama: %v", err)
	}
	if drama.SourceID != "dj-c4875de9" || drama.Source != sourceChengguo {
		t.Fatalf("unexpected identity: %+v", drama)
	}
	if drama.ID != "chengguo:dj-c4875de9" {
		t.Fatalf("unexpected drama id: %s", drama.ID)
	}
	if drama.Title != "发情女上司调教指南" || drama.ReleaseStatus != "ongoing" {
		t.Fatalf("unexpected metadata: %+v", drama)
	}
	if nativeText(drama.TotalEpisode) != "8" {
		t.Fatalf("unexpected episode count: %v", drama.TotalEpisode)
	}
	if !strings.Contains(nativeText(drama.Cover), "up-4102.jpg") {
		t.Fatalf("unexpected cover: %v", drama.Cover)
	}
	if len(drama.Tags) != 1 || drama.Tags[0] != "都市邻里" {
		t.Fatalf("unexpected tags: %v", drama.Tags)
	}
}

func TestChengguoCoverFallsBackToRelative(t *testing.T) {
	row := map[string]any{"cover": map[string]any{"fallback_url": "/static/posters/up-1.png"}}
	cover := chengguoCover(row, "https://chengguodj.com")
	if !strings.HasPrefix(cover, "https://chengguodj.com/static/posters/up-1.png") {
		t.Fatalf("unexpected cover: %s", cover)
	}
}

func TestChengguoDramaRejectsIncomplete(t *testing.T) {
	if _, err := chengguoDramaFromRow(map[string]any{"slug": "dj-1"}, "https://x"); err == nil {
		t.Fatalf("expected error for missing title")
	}
	if _, err := chengguoDramaFromRow(map[string]any{"title": "无 slug"}, "https://x"); err == nil {
		t.Fatalf("expected error for missing slug")
	}
}

func TestChengguoCatalogParsesPagination(t *testing.T) {
	d, _ := newChengguoStub(t, map[string]string{
		"/api/v1/dramas": `{"items":[{"slug":"dj-1","title":"甲","total_episodes":3,"serial_status":"ongoing"}],"pagination":{"page":1,"page_size":30,"total":31,"total_pages":2}}`,
		"/api/v1/search": `{"dramas":[{"slug":"dj-2","title":"乙","total_episodes":5,"status":"completed"}]}`,
	})
	items, more, err := d.fetchChengguoCatalogPage(context.Background(), 1, "", "")
	if err != nil {
		t.Fatalf("catalog: %v", err)
	}
	if len(items) != 1 || items[0].SourceID != "dj-1" || !more {
		t.Fatalf("unexpected catalog result: %+v more=%v", items, more)
	}
	found, _, err := d.fetchChengguoCatalogPage(context.Background(), 1, "", "逆袭")
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(found) != 1 || found[0].SourceID != "dj-2" || found[0].ReleaseStatus != "finished" {
		t.Fatalf("unexpected search result: %+v", found)
	}
}

func TestChengguoDetailBuildsChapters(t *testing.T) {
	d, _ := newChengguoStub(t, map[string]string{
		"/api/v1/dramas/dj-1": `{"slug":"dj-1","title":"甲","total_episodes":2,"episodes":[{"number":1,"id":11},{"number":2,"id":12}]}`,
	})
	drama, chapters, err := d.fetchChengguoDetail(context.Background(), "dj-1")
	if err != nil {
		t.Fatalf("detail: %v", err)
	}
	if drama.SourceID != "dj-1" || len(chapters) != 2 {
		t.Fatalf("unexpected detail: %+v chapters=%d", drama, len(chapters))
	}
	if chapters[1].VideoURL != chengguoPlayScheme+"dj-1/2" {
		t.Fatalf("unexpected video url: %s", chapters[1].VideoURL)
	}
	if chapters[0].ID != providerChapterID(sourceChengguo, "dj-1", "1") {
		t.Fatalf("unexpected chapter id: %s", chapters[0].ID)
	}
}

func TestChengguoResolveMedia(t *testing.T) {
	d, server := newChengguoStub(t, map[string]string{
		"/api/v1/dramas/dj-1/episodes/2": `{"episode":{"number":2,"duration_seconds":120},"media":{"format":"mp4","source_url":"https://cdn.example.com/v.mp4"}}`,
	})
	media, err := d.resolveChengguoMedia(context.Background(), Task{
		DramaID: providerDramaID(sourceChengguo, "dj-1"),
		Chapter: Chapter{
			ID: providerChapterID(sourceChengguo, "dj-1", "2"), Source: sourceChengguo,
			VideoURL: chengguoPlayScheme + "dj-1/2",
		},
	})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if media.URL != "https://cdn.example.com/v.mp4" {
		t.Fatalf("unexpected media url: %s", media.URL)
	}
	if media.Referer != server.URL+"/" {
		t.Fatalf("unexpected referer: %s", media.Referer)
	}
}

func TestChengguoResolveRejectsBadChapter(t *testing.T) {
	d := &Downloader{cfg: defaultConfig(), providerHosts: map[string]string{}}
	for _, videoURL := range []string{
		chengguoPlayScheme + "dj-1/0",
		chengguoPlayScheme + "dj-1/x",
		"https://cdn.example.com/v.mp4",
	} {
		if _, err := d.resolveChengguoMedia(context.Background(), Task{
			DramaID: providerDramaID(sourceChengguo, "dj-1"),
			Chapter: Chapter{
				ID: providerChapterID(sourceChengguo, "dj-1", "1"), Source: sourceChengguo,
				VideoURL: videoURL,
			},
		}); err == nil {
			t.Fatalf("expected error for %q", videoURL)
		}
	}
}

func TestChengguoCategoriesRejectUnsafeSlug(t *testing.T) {
	if !validChengguoID("dushi") {
		t.Fatalf("expected valid slug")
	}
	for _, bad := range []string{"", "a/b", "a?b", "@new", " padded"} {
		if validChengguoID(bad) {
			t.Fatalf("expected invalid slug %q", bad)
		}
	}
}

func TestChengguoAPIHostsFallbackOrder(t *testing.T) {
	d := &Downloader{cfg: Config{}}
	hosts := d.chengguoAPIHosts()
	if len(hosts) < 2 || hosts[0] != chengguoAPIURL {
		t.Fatalf("unexpected hosts: %#v", hosts)
	}
	seen := map[string]bool{}
	for _, host := range hosts {
		if seen[host] {
			t.Fatalf("duplicate host %q in %#v", host, hosts)
		}
		seen[host] = true
	}
	d.cfg.ChengguoAPIURL = "https://api.example.test/"
	if got := d.chengguoAPIHosts(); len(got) != 1 || got[0] != "https://api.example.test" {
		t.Fatalf("configured host should be used alone: %#v", got)
	}
}
