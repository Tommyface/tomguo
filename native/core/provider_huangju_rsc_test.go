package core

import (
	"encoding/json"
	"strings"
	"testing"
)

func huangjuTestRSCPage(chunks ...string) string {
	var builder strings.Builder
	for _, chunk := range chunks {
		encoded, err := json.Marshal(chunk)
		if err != nil {
			continue
		}
		builder.WriteString(`<script>self.__next_f.push([1,`)
		builder.Write(encoded)
		builder.WriteString(`])</script>`)
	}
	return builder.String()
}

func TestHuangjuRSCDecode(t *testing.T) {
	page := huangjuTestRSCPage(`{"title":"第一段"}`, `{"title":"第二段"}`)
	decoded := huangjuRSC(page)
	if !strings.Contains(decoded, "第一段") || !strings.Contains(decoded, "第二段") {
		t.Fatalf("RSC 分片未正确拼接: %q", decoded)
	}
}

func TestHuangjuCanonicalSlug(t *testing.T) {
	cases := []struct {
		href string
		slug string
		id   int
		want string
	}{
		{"/?drama=yanyu-1186-1690&ep=1", "", 0, "yanyu-1186-1690"},
		{"", "yanyu-4792", 4597, "yanyu-4792-4597"},
		{"", "yanyu-3334", 0, "yanyu-3334"},
		{"", "yanyu-4792-4597", 4597, "yanyu-4792-4597"},
	}
	for _, c := range cases {
		if got := huangjuCanonicalSlug(c.href, c.slug, c.id); got != c.want {
			t.Fatalf("huangjuCanonicalSlug(%q,%q,%d)=%q want %q", c.href, c.slug, c.id, got, c.want)
		}
	}
}

func TestHuangjuTheaterItems(t *testing.T) {
	cardHref := `{"key":"b-8","image":"https://video.huangju.net/posters/yanyu/drama-1186.jpg","title":"B中爱情故事","subtitle":"共 29 集","href":"/?drama=yanyu-1186-1690&ep=1","score":8.5,"status":"ongoing","totalEpisodes":29}`
	cardSlug := `{"id":4597,"slug":"yanyu-4792","title":"儿媳觉醒","summary":"儿媳觉醒：公公藏不住的秘密","cover":"https://video.huangju.net/posters/yanyu/drama-4792.jpg","status":"ongoing","totalEpisodes":9,"score":8.5}`
	items := huangjuTheaterItems(cardHref + "," + cardSlug)
	if len(items) != 2 {
		t.Fatalf("want 2 items, got %d", len(items))
	}
	if items[0].sourceID != "yanyu-1186-1690" || items[0].total != 29 {
		t.Fatalf("href card parsed wrong: %+v", items[0])
	}
	if items[1].sourceID != "yanyu-4792-4597" || items[1].total != 9 {
		t.Fatalf("slug card parsed wrong: %+v", items[1])
	}
	if items[1].desc != "儿媳觉醒：公公藏不住的秘密" {
		t.Fatalf("summary fallback failed: %q", items[1].desc)
	}
	if !strings.HasSuffix(items[1].cover, "drama-4792.jpg") {
		t.Fatalf("cover fallback failed: %q", items[1].cover)
	}
}

func TestHuangjuFeedEpisode(t *testing.T) {
	rsc := `{"drama":{"id":1690,"category":{"slug":"adult","name":"成人"}},"episode":{"id":12271,"epNo":2,"title":"第2集","playable":true}}`
	if id, number, ok := huangjuFeedEpisode(rsc, 2); !ok || id != "12271" || number != 2 {
		t.Fatalf("expected match failed: %q %d %v", id, number, ok)
	}
	if _, _, ok := huangjuFeedEpisode(rsc, 3); ok {
		t.Fatalf("mismatched episode should not match")
	}
}

func TestHuangjuJSONLDFromHTML(t *testing.T) {
	ld := `[{"@context":"https://schema.org","@type":"WebSite","name":"站点"},{"@context":"https://schema.org","@type":"TVSeries","name":"棺中艳娘","numberOfEpisodes":2}]`
	html := `<script type="application/ld+json">` + ld + `</script>`
	series := huangjuSeriesFromJSONLD(huangjuJSONLD(html, ""))
	if series.title != "棺中艳娘" || series.total != 2 {
		t.Fatalf("series parse wrong: %+v", series)
	}
}

func TestHuangjuJSONLDFromRSC(t *testing.T) {
	ld := `[{"@context":"https://schema.org","@type":"TVSeries","name":"测试剧","numberOfEpisodes":3,"image":"https://video.huangju.net/x.jpg"}]`
	escaped := strings.ReplaceAll(ld, `"`, `\"`)
	rsc := `"__html":"` + escaped + `"`
	series := huangjuSeriesFromJSONLD(huangjuJSONLD("", rsc))
	if series.title != "测试剧" || series.total != 3 || !strings.HasSuffix(series.cover, "x.jpg") {
		t.Fatalf("rsc jsonld parse wrong: %+v", series)
	}
}

func TestHuangjuEpisodeNumbers(t *testing.T) {
	html := `<a href="/drama/yanyu-1-100/ep-1">1</a><a href="/drama/yanyu-1-100/ep-3">3</a><a href="/drama/yanyu-1-100/ep-2">2</a><a href="/drama/yanyu-1-100/ep-1">1</a>`
	numbers := huangjuEpisodeNumbers(html)
	if len(numbers) != 3 || numbers[0] != 1 || numbers[1] != 2 || numbers[2] != 3 {
		t.Fatalf("episode numbers wrong: %v", numbers)
	}
}
