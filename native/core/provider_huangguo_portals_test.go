package core

import (
	"strings"
	"testing"
)

const selectedCatalogFixture = `<article class="card">
<a class="card__cover lazy" href="/play/lbsr018kvA/" data-src="https://d3n8qx4rvy4vol.cloudfront.net/v3/image/a.jpg"><span class="card__ep">全 4 集</span></a>
<h3 class="card__title"><a href="/play/lbsr018kvA/">媳妇帮我操幼师 EP2</a></h3>
</article>
<article class="card">
<h3 class="card__title"><a href="/play/lm003/">黎沫-《酒店的余温》</a></h3>
</article>`

const selectedDetailFixture = `<html><body>
<h1 class="play-title">黎沫-《酒店的余温》</h1>
<div class="play-player play-player--locked" data-play-player data-play-mode="main" data-playlet-id="14622" data-chapter-id="30832" data-can-watch="0" data-main-url="" data-preview-url=""></div>
<section class="play-episodes"><div class="play-episodes__grid">
<a class="play-ep is-active play-ep--coin" href="/play/lm003/" data-target-episode="1" data-target-video-id="30832">1</a>
<a class="play-ep" href="/play/lm003/" data-target-episode="2" data-target-video-id="30833">2</a>
</div></section>
</body></html>`

const welfareDetailFixture = `<html><body>
<h1 class="detail-title">与纯阴圣体大小姐双修 全10集完整版</h1>
<div class="detail-episode-list">
<a class="detail-episode" href="/play/zyb003WTS/28673/" data-ep="1"><span>第1集</span></a>
<a class="detail-episode" href="javascript:;" data-coin-dialog-open data-ep="2" data-chapter-id="28674" data-single-coins="48"><span>第2集</span></a>
</div>
<div class="play-player" data-play-player data-chapter-id="28673" data-pay-type="0" data-can-watch="1" data-need-unlock="0" data-need-vip="0"></div>
<div class="dplayer-video" data-url="v3/bktappupdata/sp/aa.m3u8?id=28673&amp;t=1795123981"></div>
</body></html>`

func TestPortalCatalogParsesTitlesNotPlayButtons(t *testing.T) {
	items := portalCatalog(selectedCatalogFixture, sourceHuangguoSelected)
	if len(items) != 2 {
		t.Fatalf("items = %d, want 2", len(items))
	}
	if items[0].Title != "媳妇帮我操幼师 EP2" {
		t.Fatalf("title = %q", items[0].Title)
	}
	if items[0].Source != sourceHuangguoSelected {
		t.Fatalf("source = %q", items[0].Source)
	}
}

func TestSelectedEpisodesUseTargetVideoIDAndCoinFlag(t *testing.T) {
	chapters := portalEpisodes(selectedDetailFixture, sourceHuangguoSelected, "lm003")
	if len(chapters) != 2 {
		t.Fatalf("chapters = %d", len(chapters))
	}
	if chapters[0].ID != providerChapterID(sourceHuangguoSelected, "lm003", "30832") {
		t.Fatalf("first id = %q", chapters[0].ID)
	}
	if chapters[0].VIP != true {
		t.Fatal("coin episode must be marked vip")
	}
	if chapters[1].VIP == true {
		t.Fatal("free episode must not be marked vip")
	}
}

func TestWelfareEpisodesUseChapterIDAndCoinDialog(t *testing.T) {
	chapters := portalEpisodes(welfareDetailFixture, sourceHuangguoWelfare, "zyb003WTS")
	if len(chapters) != 2 {
		t.Fatalf("chapters = %d", len(chapters))
	}
	if chapters[0].PageURL == "" || chapters[0].VIP == true {
		t.Fatalf("first chapter = %#v", chapters[0])
	}
	if chapters[1].VIP != true {
		t.Fatal("coin dialog episode must be marked vip")
	}
}

func TestPortalRejectsLockedPlayerAsMedia(t *testing.T) {
	player := portalPlayer.FindString(selectedDetailFixture)
	attrs := portalAttributes(player)
	if attrs["data-can-watch"] != "0" {
		t.Fatalf("fixture must be locked, got %q", attrs["data-can-watch"])
	}
	if attrs["data-main-url"] != "" {
		t.Fatal("locked page must not expose main url")
	}
}

func TestHuangguoPublishEntriesMapByName(t *testing.T) {
	body := `[{"name":"黄果福利站","path":"https://d24dalcptr65cw.cloudfront.net"},{"name":"黄果精选站","path":"https://d2teten6pcf3ig.cloudfront.net"}]`
	decoded := strings.ReplaceAll(strings.ReplaceAll(body, `\"`, `"`), `\/`, `/`)
	got := map[string]string{}
	for _, match := range reHuangguoPublishEntry.FindAllStringSubmatch(decoded, -1) {
		got[match[1]] = match[2]
	}
	if got["黄果福利站"] != "https://d24dalcptr65cw.cloudfront.net" {
		t.Fatalf("welfare entry = %q", got["黄果福利站"])
	}
	if got["黄果精选站"] != "https://d2teten6pcf3ig.cloudfront.net" {
		t.Fatalf("selected entry = %q", got["黄果精选站"])
	}
}

const welfareCatalogFixture = `<a class="hero-card" href="/detail/zyb003WTS">
<img src="/static/images/placeholder.png?v=2" class="hero-image lazy" data-src="https://d3n8qx4rvy4vol.cloudfront.net/v3/image/64/jy/1043/cover.jpg" alt="与纯阴圣体大小姐双修" />
</a>
<a class="card" href="/detail/plug001">
<img class="poster lazy" src="/static/images/placeholder.png?v=2" data-original="https://d3n8qx4rvy4vol.cloudfront.net/v3/image/aa/bb/poster.jpg" alt="占位封面测试" />
</a>`

func TestPortalCatalogExtractsLazyCover(t *testing.T) {
	selected := portalCatalog(selectedCatalogFixture, sourceHuangguoSelected)
	if len(selected) != 2 {
		t.Fatalf("selected items = %d, want 2", len(selected))
	}
	want := "https://d3n8qx4rvy4vol.cloudfront.net/v3/image/a.jpg"
	if selected[0].Cover != want || selected[0].CoverURL != want {
		t.Fatalf("selected cover = %q / %q, want %q", selected[0].Cover, selected[0].CoverURL, want)
	}
	if selected[1].Cover != "" {
		t.Fatalf("coverless card must stay empty, got %q", selected[1].Cover)
	}

	welfare := portalCatalog(welfareCatalogFixture, sourceHuangguoWelfare)
	if len(welfare) != 2 {
		t.Fatalf("welfare items = %d, want 2", len(welfare))
	}
	if welfare[0].Title != "与纯阴圣体大小姐双修" {
		t.Fatalf("welfare title = %q", welfare[0].Title)
	}
	if welfare[0].Cover != "https://d3n8qx4rvy4vol.cloudfront.net/v3/image/64/jy/1043/cover.jpg" {
		t.Fatalf("welfare cover = %q", welfare[0].Cover)
	}
	if welfare[1].Cover != "https://d3n8qx4rvy4vol.cloudfront.net/v3/image/aa/bb/poster.jpg" {
		t.Fatalf("data-original cover = %q", welfare[1].Cover)
	}
}
