package core

import (
	"strings"
	"testing"
)

const huakuiDetailFixture = `<!DOCTYPE html><html><head>
<title>办公室御姐攻略 - 第一集 | 花魁短剧</title>
<meta property="og:image" content="/static/brand/cover.jpg" />
</head><body>
<h1>办公室御姐攻略</h1>
<a href="/drama/bgsyjgldej/">办公室御姐攻略</a>
<a href="/drama/bgsyjgldej/10951/">1</a>
<a href="/drama/bgsyjgldej/10952/">2</a>
<a href="/drama/bgsyjgldej/11385/">3</a>
<a href="/drama/fanpainixishoufuquanmennvtu/">番外</a>
</body></html>`

func TestHuakuiParseChaptersKeepsOwnDramaOnly(t *testing.T) {
	chapters := huakuiParseChapters(huakuiDetailFixture, huakuiSiteBaseURL+"/drama/bgsyjgldej/", "bgsyjgldej")
	if len(chapters) != 3 {
		t.Fatalf("chapters = %d, want 3", len(chapters))
	}
	if chapters[0].ID != providerChapterID(sourceHuakui, "bgsyjgldej", "10951") {
		t.Fatalf("first chapter id = %q", chapters[0].ID)
	}
	if chapters[2].PageURL != huakuiSiteBaseURL+"/drama/bgsyjgldej/11385/" {
		t.Fatalf("third chapter url = %q", chapters[2].PageURL)
	}
}

func TestHuakuiParseCatalogRejectsForeignLinks(t *testing.T) {
	body := `<a href="/drama/bgsyjgldej/">办公室御姐攻略</a>
<a href="/curated/dramas/hkyc/">短剧合集</a>
<a href="/other/page/">其他</a>`
	items := huakuiParseCatalog(body, huakuiSiteBaseURL+"/", "")
	if len(items) != 1 || items[0].SourceID != "bgsyjgldej" {
		t.Fatalf("catalog items = %#v", items)
	}
	if items[0].Source != sourceHuakui {
		t.Fatalf("source = %q", items[0].Source)
	}
}

func TestHuakuiLockedPlayerTagIsRecognized(t *testing.T) {
	locked := `<div class="hk-playback-player hk-playback-player--locked" data-play-player="" data-can-watch="0"></div>`
	player := reHuakuiPlayer.FindString(locked)
	if !strings.Contains(player, "playback-player--locked") || !strings.Contains(player, `data-can-watch="0"`) {
		t.Fatalf("locked player tag not matched: %q", player)
	}
}

func TestHuakuiMediaURLRewritesRelativePathToProxy(t *testing.T) {
	body := `{"playletChapterInfo":[{"id":11640,"currentEpisode":1,"videoUrl":"bktappupdata/sp/zw/zs/uz/dn/aa.m3u8","canWatch":true}]}`
	media := huakuiMediaURL("https://huakui.net/drama/luolibianlidian/", body)
	want := "https://huakui.net/h5/m3u8/bktappupdata/sp/zw/zs/uz/dn/aa.m3u8"
	if media != want {
		t.Fatalf("media = %q, want %q", media, want)
	}
}

func TestHuakuiMediaURLKeepsAbsoluteAddress(t *testing.T) {
	body := `{"videoUrl":"https://cdn.example.test/a/b.m3u8?x=1"}`
	if got := huakuiMediaURL("https://huakui.net/drama/x/", body); got != "https://cdn.example.test/a/b.m3u8?x=1" {
		t.Fatalf("absolute media = %q", got)
	}
}

func TestHuakuiMediaURLRejectsEmptyPayload(t *testing.T) {
	if got := huakuiMediaURL("https://huakui.net/drama/x/", `<div data-play-player></div>`); got != "" {
		t.Fatalf("expected empty media, got %q", got)
	}
}

const huakuiCoverCardFixture = `<article class="hk-drama-card">
<a class="hk-drama-card__cover hk-lazy" href="/drama/baiwandajiangjiatingjingjitiaozhan/" style="--cover:url(&quot;/static/brand/placeholder.svg?v=huakui-20261010&quot;)"><span class="hk-drama-card__ep">全 8 集</span></a>
<h3 class="hk-drama-card__title"><a href="/drama/baiwandajiangjiatingjingjitiaozhan/">百万大奖：家庭禁忌挑战</a></h3>
</article>
<script>self.__next_f.push([1,"{\"playName\":\"百万大奖：家庭禁忌挑战\",\"slug\":\"baiwandajiangjiatingjingjitiaozhan\",\"cover\":\"https://d3n8qx4rvy4vol.cloudfront.net/v3/image/is/ex/1045/cover.jpg\"}"])</script>`

func TestHuakuiCatalogUsesCardTitleAndRSCCover(t *testing.T) {
	items := huakuiParseCatalog(huakuiCoverCardFixture, huakuiSiteBaseURL+"/", "")
	if len(items) != 1 {
		t.Fatalf("items = %d, want 1", len(items))
	}
	if items[0].Title != "百万大奖：家庭禁忌挑战" {
		t.Fatalf("title = %q", items[0].Title)
	}
	want := "https://d3n8qx4rvy4vol.cloudfront.net/v3/image/is/ex/1045/cover.jpg"
	if items[0].Cover != want || items[0].CoverURL != want {
		t.Fatalf("cover = %q / %q, want %q", items[0].Cover, items[0].CoverURL, want)
	}
}
