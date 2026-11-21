package core

import (
	"strings"
	"testing"
)

const huangguoWebDetailFixture = `<html><head><title>办公室御姐攻略 - 黄果短剧</title></head><body>
<script>self.__next_f.push([1,"{\"episodes\":[{\"chapterId\":\"6ad9b47955f504b663193a9f\",\"episode\":1,\"playTime\":900,\"title\":\"第一集\",\"url\":\"bktadminup/sp/ub/3f/fs/ug/aa.m3u8\"},{\"chapterId\":\"6ad9b47955f504b663193b00\",\"episode\":2,\"playTime\":900,\"title\":\"第二集\",\"url\":\"bktadminup/sp/ub/3f/fs/ug/bb.m3u8\"}],\"firstChapterId\":\"6ad9b47955f504b663193a9f\",\"freeEpisodeCount\":1,\"cover\":\"https://zzzznnn.lkkwip.cn/cf230705/image/aa/bb/cc.jpg\"}"])</script>
</body></html>`

func unescapeHuangguoWebFixture(raw string) string {
	return strings.ReplaceAll(strings.ReplaceAll(raw, `\"`, `"`), `\/`, `/`)
}

func TestHuangguoWebEpisodesAndVIPBoundary(t *testing.T) {
	plain := unescapeHuangguoWebFixture(huangguoWebDetailFixture)
	episodes, free := huangguoWebEpisodes(plain)
	if len(episodes) != 2 || free != 1 {
		t.Fatalf("episodes = %d, free = %d", len(episodes), free)
	}
	if episodes[1].URL != "bktadminup/sp/ub/3f/fs/ug/bb.m3u8" {
		t.Fatalf("second url = %q", episodes[1].URL)
	}
	if !(free > 0 && episodes[1].Episode > free) {
		t.Fatal("episode 2 must be treated as vip when freeEpisodeCount=1")
	}
}

func TestHuangguoWebCatalogFiltersNonVideoLinks(t *testing.T) {
	body := `<a href="/videos/bangongshiyujiegonglue">办公室御姐攻略</a>
<a href="/videos/quanchengxunzhaode">全程寻找的</a>
<a href="/tags/wuxia">武侠</a>
<a href="/way-home">回家的路</a>`
	items := huangguoWebParseCatalog(body)
	if len(items) != 2 {
		t.Fatalf("items = %d, want 2", len(items))
	}
	if items[0].Source != sourceHuangguoMain || items[0].SourceID == "" {
		t.Fatalf("first item = %#v", items[0])
	}
}

func TestHuangguoWebRelativeMediaDoesNotUseCoverHost(t *testing.T) {
	episodes, _ := huangguoWebEpisodes(unescapeHuangguoWebFixture(huangguoWebDetailFixture))
	if len(episodes) == 0 || isProviderHTTPMediaURL(episodes[0].URL) {
		t.Fatal("fixture must contain a relative media path")
	}
	if !strings.Contains(episodes[0].URL, "bktadminup/") {
		t.Fatalf("unexpected relative path: %q", episodes[0].URL)
	}
}

func TestHuangguoWebCoverHostExtraction(t *testing.T) {
	plain := unescapeHuangguoWebFixture(huangguoWebDetailFixture)
	if host := firstRegexGroup(reHuangguoWebCoverHost, plain); host != "zzzznnn.lkkwip.cn" {
		t.Fatalf("cover host = %q", host)
	}
}

func TestHuangguoWebMediaURLUsesSiteProxy(t *testing.T) {
	rel := "bktadminup/sp/ub/3f/fs/ug/aa.m3u8"
	got := huangguoWebMediaURL("https://falpius.com/videos/abc", rel)
	want := "https://falpius.com/api/app/vid/h5/m3u8/" + rel
	if got != want {
		t.Fatalf("media = %q, want %q", got, want)
	}
}

func TestHuangguoWebMediaURLKeepsAbsolute(t *testing.T) {
	abs := "https://cdn.example.test/a.m3u8"
	if got := huangguoWebMediaURL("https://falpius.com/videos/abc", abs); got != abs {
		t.Fatalf("absolute media = %q", got)
	}
}

func TestHuangguoWebMediaURLRejectsEmpty(t *testing.T) {
	if got := huangguoWebMediaURL("https://falpius.com/videos/abc", "  "); got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
}
