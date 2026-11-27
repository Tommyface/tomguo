package core

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"
)

func providerParityDownloader(t *testing.T, transport func(*http.Request, url.Values) (*http.Response, error)) *Downloader {
	t.Helper()
	d := sourceFixtureDownloader(t, func(request *http.Request) (*http.Response, error) {
		form := request.URL.Query()
		if request.Body != nil {
			body, err := io.ReadAll(request.Body)
			if err != nil {
				t.Fatal(err)
			}
			bodyForm, _ := url.ParseQuery(string(body))
			for key, values := range bodyForm {
				form[key] = append(form[key], values...)
			}
		}
		return transport(request, form)
	})
	d.cfg.HuangjuAPIURL = "https://api.huangju.test"
	d.cfg.HuangjuURL = "https://huangju.test"
	d.cfg.YeguoURL = "https://yeguo.test"
	d.cfg.DSDURL = "https://dsd.test"
	return d
}

func TestYeguoDomainAliasesAndDefaultEndpoint(t *testing.T) {
	if got := (&Downloader{cfg: defaultConfig()}).providerBaseURL(sourceYeguo); got != "https://analyze.buxefaex.cc" {
		t.Fatalf("unexpected default yeguo endpoint: %s", got)
	}
	for _, address := range []string{
		"https://analyze.buxefaex.cc/",
		"https://some-line.buxefaex.cc/drama/video/1/",
		"https://some-backup.fzchosdi.cc/drama/video/1/",
		"https://delta.ygrwdsgt.cc/",
		"https://yeguodj.com/",
		"https://ygdj7.com/",
	} {
		if got := providerSourceForURL(address); got != sourceYeguo {
			t.Fatalf("yeguo domain was not recognized: %s -> %s", address, got)
		}
	}
	for _, alias := range []string{
		"analyze.buxefaex.cc",
		"delta.ygrwdsgt.cc",
		"yeguodj.com",
		"ygdj7.com",
	} {
		if got := canonicalProviderSource(alias); got != sourceYeguo {
			t.Fatalf("yeguo source alias was not canonicalized: %s -> %s", alias, got)
		}
	}
}

func TestYeguoTransitPageDiscoversCurrentLineDomains(t *testing.T) {
	encoded := base64.StdEncoding.EncodeToString([]byte(`<script>
words = 'abandon,ability,analyze,chair'.split(',');
lineAry = Vx.map(Vx.range(1, 3), function () { return location.protocol + '//' + words.random() + '.buxefaex.cc' });
backupLine = Vx.map(Vx.range(1, 3), function () { return location.protocol + '//' + words.random() + '.fzchosdi.cc'; });
</script>`))
	sites := yeguoTransitSites(`<script>document.write(Base64.decode("` + encoded + `"));</script>`)
	want := []string{"https://analyze.buxefaex.cc", "https://ability.buxefaex.cc", "https://abandon.buxefaex.cc", "https://analyze.fzchosdi.cc"}
	for _, site := range want {
		found := false
		for _, candidate := range sites {
			found = found || candidate == site
		}
		if !found {
			t.Fatalf("transit discovery missed %s in %v", site, sites)
		}
	}
}

func TestProviderCatalogUpdateDefaultsToFiftyYeguoPostPages(t *testing.T) {
	skipLegacyBuiltin(t)
	var pages []string
	engine := sourceFixtureEngine(t, func(request *http.Request) (*http.Response, error) {
		if request.URL.Host != "api.yeguo.test" || request.URL.Path != "/api/theater/exploreList" {
			t.Fatalf("unexpected yeguo catalog request: %s", request.URL.String())
		}
		if request.Method != http.MethodPost {
			t.Fatalf("yeguo catalog must use POST for pagination, got %s", request.Method)
		}
		if err := request.ParseForm(); err != nil {
			t.Fatal(err)
		}
		page := request.Form.Get("page")
		pages = append(pages, page)
		body := fmt.Sprintf(`{"status":"1","data":{"list":[{"video_id":"8%s","title":"野果默认分页%s","description":"完整资料","episode_count":"6","serialize_status":"2"}],"page":%s,"limit":1,"total":50,"has_more":%q}}`, page, page, page, map[bool]string{true: "1", false: "0"}[page != "50"])
		return sourceFixtureResponse(request, http.StatusOK, body), nil
	})
	engine.downloader.yeguoClient().access = &yeguoAccess{base: "https://api.yeguo.test", identifier: "fixture-trace", loadedAt: time.Now()}

	if err := engine.updateSource(context.Background(), sourceYeguo, "update"); err != nil {
		t.Fatal(err)
	}
	if len(pages) != 50 || pages[0] != "1" || pages[49] != "50" {
		t.Fatalf("default source update did not request 50 pages: %v", pages)
	}
	page := engine.nativeCached(sourceYeguo)
	if len(page.Items) != 50 || page.Page != 50 || page.HasMore {
		t.Fatalf("wrong yeguo source cache after default update: %+v", page)
	}
}

func TestYeguoCatalogAcceptsSourcePageSize(t *testing.T) {
	d := providerParityDownloader(t, func(request *http.Request, form url.Values) (*http.Response, error) {
		if request.URL.Host != "api.yeguo.test" || request.URL.Path != "/api/theater/exploreList" {
			t.Fatalf("unexpected yeguo request: %s", request.URL.String())
		}
		if request.Method != http.MethodPost {
			t.Fatalf("yeguo catalog must use POST for pagination, got %s", request.Method)
		}
		rows := make([]any, 0, 30)
		for index := 1; index <= 30; index++ {
			id := strconv.Itoa(9000 + index)
			rows = append(rows, map[string]any{"video_id": id, "title": "野果目录样本 " + id, "episode_count": "6", "serialize_status": "2"})
		}
		body, _ := json.Marshal(map[string]any{"status": "1", "data": map[string]any{"list": rows, "limit": 20, "total": 30, "has_more": "0"}})
		return sourceFixtureResponse(request, http.StatusOK, string(body)), nil
	})
	d.yeguoClient().access = &yeguoAccess{base: "https://api.yeguo.test", identifier: "fixture-trace", loadedAt: time.Now()}
	items, more, err := d.fetchYeguoCatalogPage(context.Background(), 1, "", "")
	if err != nil || more || len(items) != 30 {
		t.Fatalf("yeguo source-sized page rejected: items=%d more=%t err=%v", len(items), more, err)
	}
}

func TestProviderMediaCredentialsLimitOriginAndUseBackupAddresses(t *testing.T) {
	primary := "https://media.example.test/main.mp4"
	backup := "https://media.example.test/backup.mp4"
	model := map[string]any{"video_duration": "6", "video_list": []any{map[string]any{
		"main_url": "invalid", "backup_url": base64.StdEncoding.EncodeToString([]byte(primary)), "backup_urls": []any{backup},
		"video_meta": map[string]any{"codec_type": "h264", "definition": "720p"},
	}}}
	media, err := selectHongguoAppMedia(model)
	if err != nil || media.URL != primary || len(media.Variants) != 2 || media.Variants[1].URL != backup || media.Duration != 6*time.Second {
		t.Fatalf("hongguo backup media fields were lost: %+v %v", media, err)
	}
	credentials := &providerMediaCredentials{origin: "https://media.example.test", cookie: "Signed=fixture", referer: "https://source.example.test/page", userAgent: "Agent"}
	request, _ := http.NewRequest(http.MethodGet, "https://media.example.test/part", nil)
	if err := credentials.apply(request); err != nil || request.Header.Get("Cookie") != "Signed=fixture" || request.Header.Get("Origin") != "https://source.example.test" || request.Header.Get("User-Agent") != "Agent" {
		t.Fatal("credentials not applied on exact origin", err, request.Header)
	}
	other, _ := http.NewRequest(http.MethodGet, "https://media.example.test:444/part", nil)
	other.Header.Set("Cookie", "old=value")
	if err := credentials.apply(other); err != nil || other.Header.Get("Cookie") != "" {
		t.Fatal("credentials leaked to another origin", err, other.Header)
	}
}

func TestProviderDSDMediaSigningPropagatesFetchErrors(t *testing.T) {
	d := rankingTestDownloader(t, func(request *http.Request) (*http.Response, error) {
		if !strings.Contains(request.URL.Path, "/addons/vplayer/") {
			t.Fatalf("unexpected request: %s", request.URL.String())
		}
		return rankingHTTPResponse(request, http.StatusBadGateway, "unavailable"), nil
	})
	_, found, err := d.signDSDMedia(context.Background(), "https://media.example.test/video/index.m3u8", "https://www.dsd.com.se/index.php/vod/play/id/1/sid/1/nid/1.html", "https://www.dsd.com.se")
	if err == nil || found {
		t.Fatal("dsd signing failure was swallowed", found, err)
	}
	if path := dsdVplayerMediaPath("https://media.example.test/video/index.m3u8?token=a%2Fb"); path != "/video/index.m3u8?token=a%2Fb" {
		t.Fatal("dsd media path changed", path)
	}
}
