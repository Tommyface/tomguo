package core

import (
	"context"
	"strings"
	"testing"

	"duanjuapp/native/source"
)

type bridgeStubHandler struct{}

func (bridgeStubHandler) Categories(ctx context.Context, force bool) ([]source.Category, error) {
	return []source.Category{{ID: "hot", Name: "热播"}}, nil
}

func (bridgeStubHandler) Catalog(ctx context.Context, page int, category string) ([]source.CatalogItem, bool, error) {
	return []source.CatalogItem{{ID: "d1", Title: "演示剧"}}, true, nil
}

func (bridgeStubHandler) Search(ctx context.Context, page int, query string) ([]source.CatalogItem, bool, error) {
	return []source.CatalogItem{{ID: "d1", Title: query}}, false, nil
}

func (bridgeStubHandler) Detail(ctx context.Context, sourceID string) (source.DramaDetail, error) {
	return source.DramaDetail{Item: source.CatalogItem{ID: sourceID}}, nil
}

func (bridgeStubHandler) Stream(ctx context.Context, request source.StreamRequest) (source.MediaStream, error) {
	return source.MediaStream{URL: "https://cdn.example.com/a.m3u8"}, nil
}

func TestCatalogItemFromDrama(t *testing.T) {
	drama := Drama{ID: "d1", Title: "", Name: "备用标题", Source: "demo"}
	drama.CoverURL = nil
	drama.Img = "https://img.example.com/c.jpg"
	item := catalogItemFromDrama(drama)
	if item.ID != "d1" || item.Title != "备用标题" {
		t.Fatalf("id/title 不符：%+v", item)
	}
	if item.Cover != "https://img.example.com/c.jpg" {
		t.Fatalf("封面回退失败：%q", item.Cover)
	}
	if item.SourceID != "demo" {
		t.Fatalf("sourceId 不符：%q", item.SourceID)
	}
}

func TestCatalogItemFromDramaPrefersFirstCover(t *testing.T) {
	drama := Drama{ID: "d2", Title: "标题"}
	drama.Cover = "https://img.example.com/first.jpg"
	drama.Poster = "https://img.example.com/later.jpg"
	if item := catalogItemFromDrama(drama); item.Cover != "https://img.example.com/first.jpg" {
		t.Fatalf("应按字段顺序取第一个封面，得到 %q", item.Cover)
	}
}

func TestMediaStreamType(t *testing.T) {
	cases := map[string]string{
		"https://cdn.example.com/a.m3u8":           "hls",
		"https://cdn.example.com/play?format=m3u8": "hls",
		"https://cdn.example.com/a.mp4":            "mp4",
		"https://cdn.example.com/video":            "mp4",
	}
	for input, want := range cases {
		if got := mediaStreamType(input); got != want {
			t.Errorf("mediaStreamType(%q) = %q，期望 %q", input, got, want)
		}
	}
}

func TestModuleSourceMetaAdultTriState(t *testing.T) {
	if meta := moduleSourceMeta(&jsModule{manifest: moduleManifest{ID: "x", Name: "X"}}); meta.Adult {
		t.Fatal("adult 未声明时应为 false")
	}
	yes := true
	if meta := moduleSourceMeta(&jsModule{manifest: moduleManifest{ID: "x", Adult: &yes}}); !meta.Adult {
		t.Fatal("adult=true 应透传")
	}
	no := false
	if meta := moduleSourceMeta(&jsModule{manifest: moduleManifest{ID: "x", Adult: &no}}); meta.Adult {
		t.Fatal("adult=false 应透传")
	}
}

func TestAdapterRegistryUnknownSource(t *testing.T) {
	if _, _, err := adapterCatalog(context.Background(), "不存在的源", 1, ""); err == nil {
		t.Fatal("未知源应返回错误")
	} else if source.IsTransient(err) {
		t.Fatal("未知源应为永久错误，不重试")
	}
	if _, err := adapterDetail(context.Background(), "不存在的源", "d1"); err == nil {
		t.Fatal("未知源详情应返回错误")
	}
	if _, err := adapterStream(context.Background(), "不存在的源", source.StreamRequest{}); err == nil {
		t.Fatal("未知源播放应返回错误")
	}
}

func TestAdapterRegistryRoundTrip(t *testing.T) {
	adapter := source.NewAdapter(source.Meta{ID: "bridge-demo", Name: "桥接演示"}, bridgeStubHandler{}, source.DefaultPolicy())
	if err := sourceAdapters.Register(adapter); err != nil {
		t.Fatalf("注册失败：%v", err)
	}
	defer sourceAdapters.Unregister("bridge-demo")

	items, more, err := adapterCatalog(context.Background(), "bridge-demo", 1, "hot")
	if err != nil {
		t.Fatalf("目录失败：%v", err)
	}
	if len(items) != 1 || !more || items[0].Title != "演示剧" {
		t.Fatalf("目录结果不符：%+v more=%v", items, more)
	}
	detail, err := adapterDetail(context.Background(), "bridge-demo", "d1")
	if err != nil || detail.Item.ID != "d1" {
		t.Fatalf("详情不符：%+v err=%v", detail, err)
	}
	stream, err := adapterStream(context.Background(), "bridge-demo", source.StreamRequest{DramaID: "d1", EpisodeID: "e1"})
	if err != nil {
		t.Fatalf("播放失败：%v", err)
	}
	if !strings.Contains(stream.URL, ".m3u8") {
		t.Fatalf("播放地址不符：%q", stream.URL)
	}
}

func TestAdapterRunsRealJSModule(t *testing.T) {
	manifest := moduleManifest{
		ID: "adapter-js", Name: "适配层JS", Version: "1.0.0",
		Runtime: moduleRuntimeJS, API: 1, Adult: boolPtr(false),
	}
	code := []byte(`
		module.exports.catalogPage = function(page, category) {
			return { items: [{ id: "d1", title: "适配剧" }], hasMore: false };
		};
		module.exports.detail = function(sourceId) {
			return { drama: { id: sourceId, title: "适配剧" }, chapters: [{ id: "c1", title: "第1集" }] };
		};
		module.exports.resolveMedia = function(task) {
			return { url: "https://cdn.example.com/a.m3u8", referer: "https://site.example.com/" };
		};
	`)
	provider, err := newTestJSProvider(manifest, code)
	if err != nil {
		t.Fatalf("构造模块失败：%v", err)
	}
	module := &jsModule{manifest: manifest, active: provider}
	if err := registerModuleSource(module); err != nil {
		t.Fatalf("注册适配器失败：%v", err)
	}
	defer sourceAdapters.Unregister("adapter-js")

	items, more, err := adapterCatalog(context.Background(), "adapter-js", 1, "")
	if err != nil {
		t.Fatalf("目录失败：%v", err)
	}
	if len(items) != 1 || more || items[0].Title != "适配剧" {
		t.Fatalf("目录结果不符：%+v more=%v", items, more)
	}

	detail, err := adapterDetail(context.Background(), "adapter-js", "d1")
	if err != nil {
		t.Fatalf("详情失败：%v", err)
	}
	if detail.Item.ID != "d1" || len(detail.Episodes) != 1 || detail.Episodes[0].ID != "c1" {
		t.Fatalf("详情结果不符：%+v", detail)
	}
	if detail.Episodes[0].Number != 1 {
		t.Fatalf("集号应为 1，得到 %d", detail.Episodes[0].Number)
	}

	stream, err := adapterStream(context.Background(), "adapter-js", source.StreamRequest{DramaID: "d1", EpisodeID: "c1"})
	if err != nil {
		t.Fatalf("播放失败：%v", err)
	}
	if stream.URL != "https://cdn.example.com/a.m3u8" || stream.Type != "hls" {
		t.Fatalf("播放结果不符：%+v", stream)
	}
	if stream.Headers["Referer"] != "https://site.example.com/" {
		t.Fatalf("Referer 未透传：%+v", stream.Headers)
	}
}

func TestAdapterHealthAfterModuleCall(t *testing.T) {
	manifest := moduleManifest{ID: "adapter-health", Name: "健康", Version: "1.0.0", Runtime: moduleRuntimeJS, API: 1, Adult: boolPtr(false)}
	code := []byte(`module.exports.catalogPage = function() { return { items: [], hasMore: false }; };`)
	provider, err := newTestJSProvider(manifest, code)
	if err != nil {
		t.Fatalf("构造模块失败：%v", err)
	}
	if err := registerModuleSource(&jsModule{manifest: manifest, active: provider}); err != nil {
		t.Fatalf("注册失败：%v", err)
	}
	defer sourceAdapters.Unregister("adapter-health")
	if _, _, err := adapterCatalog(context.Background(), "adapter-health", 1, ""); err != nil {
		t.Fatalf("目录失败：%v", err)
	}
	item, ok := sourceAdapters.Lookup("adapter-health")
	if !ok {
		t.Fatal("适配器未注册")
	}
	if health := item.Health(); health.Success != 1 || health.Failure != 0 {
		t.Fatalf("健康记录不符：%+v", health)
	}
	if rate := item.Health().SuccessRate(); rate != 1 {
		t.Fatalf("成功率 %f，期望 1", rate)
	}
}
