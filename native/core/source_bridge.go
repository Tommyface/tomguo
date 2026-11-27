package core

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"duanjuapp/native/source"
)

const sourceCapabilityCatalog = source.CapabilityCatalog

// classifyAggregateError 把聚合路径的错误分成可重试与不可重试。
func classifyAggregateError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, errSourceRetired) {
		return source.Permanent("catalog", err)
	}
	return source.Transient("catalog", err)
}

// moduleSourceHandler 把 JS 模块包装成 source.Handler。
// 只做模型转换与参数映射，不含任何站点解析逻辑。
type moduleSourceHandler struct {
	provider *jsProvider
}

func newModuleSourceHandler(provider *jsProvider) *moduleSourceHandler {
	return &moduleSourceHandler{provider: provider}
}

func (h *moduleSourceHandler) Categories(ctx context.Context, force bool) ([]source.Category, error) {
	items, err := h.provider.Categories(ctx, force)
	if err != nil {
		return nil, source.Transient("categories", err)
	}
	out := make([]source.Category, 0, len(items))
	for _, item := range items {
		out = append(out, source.Category{ID: item.ID, Name: item.Name})
	}
	return out, nil
}

func (h *moduleSourceHandler) Catalog(ctx context.Context, page int, category string) ([]source.CatalogItem, bool, error) {
	dramas, more, err := h.provider.CatalogPage(ctx, page, category)
	if err != nil {
		return nil, false, source.Transient("catalog", err)
	}
	return catalogItemsFromDramas(dramas), more, nil
}

func (h *moduleSourceHandler) Search(ctx context.Context, page int, query string) ([]source.CatalogItem, bool, error) {
	dramas, more, err := h.provider.Search(ctx, page, query)
	if err != nil {
		return nil, false, source.Transient("search", err)
	}
	return catalogItemsFromDramas(dramas), more, nil
}

func (h *moduleSourceHandler) Detail(ctx context.Context, sourceID string) (source.DramaDetail, error) {
	drama, chapters, err := h.provider.Detail(ctx, sourceID)
	if err != nil {
		return source.DramaDetail{}, source.Transient("detail", err)
	}
	detail := source.DramaDetail{
		Item:     catalogItemFromDrama(drama),
		Intro:    firstNonEmpty(strings.TrimSpace(drama.Intro), strings.TrimSpace(drama.Desc)),
		Episodes: make([]source.Episode, 0, len(chapters)),
	}
	for index, chapter := range chapters {
		number, _ := strconv.Atoi(chapter.EpisodeString(index + 1))
		detail.Episodes = append(detail.Episodes, source.Episode{
			ID:     chapter.ID,
			Title:  chapter.Title,
			Number: number,
			VIP:    chapter.VIP,
		})
	}
	return detail, nil
}

func (h *moduleSourceHandler) Stream(ctx context.Context, request source.StreamRequest) (source.MediaStream, error) {
	task := Task{
		DramaID:    request.DramaID,
		DramaTitle: request.DramaTitle,
		Index:      request.Index,
		Chapter: Chapter{
			ID:      request.EpisodeID,
			Title:   request.Episode.Title,
			Source:  request.Episode.ID,
			VIP:     request.Episode.VIP,
			PageURL: request.Episode.ID,
		},
	}
	if request.Episode.Number > 0 {
		task.Chapter.CurrentEpisode = []byte(strconv.Quote(strconv.Itoa(request.Episode.Number)))
	}
	media, err := h.provider.ResolveMedia(ctx, task)
	if err != nil {
		return source.MediaStream{}, source.Transient("stream", err)
	}
	stream := source.MediaStream{URL: media.URL, Type: mediaStreamType(media.URL)}
	if media.Referer != "" {
		stream.Headers = map[string]string{"Referer": media.Referer}
	}
	return stream, nil
}

func catalogItemsFromDramas(dramas []Drama) []source.CatalogItem {
	items := make([]source.CatalogItem, 0, len(dramas))
	for _, drama := range dramas {
		items = append(items, catalogItemFromDrama(drama))
	}
	return items
}

func catalogItemFromDrama(drama Drama) source.CatalogItem {
	item := source.CatalogItem{
		ID:       firstNonEmpty(drama.ID, drama.SourceID),
		SourceID: drama.Source,
		Title:    firstNonEmpty(drama.Title, drama.Name),
		Desc:     firstNonEmpty(drama.Desc, drama.Intro),
		VIP:      drama.VIP,
	}
	for _, value := range []any{drama.Cover, drama.CoverURL, drama.CoverURLSnake, drama.Image, drama.ImageURL, drama.ImageURLSnake, drama.Img, drama.Pic, drama.Picture, drama.Poster, drama.Thumb, drama.Thumbnail} {
		if text, ok := value.(string); ok && strings.TrimSpace(text) != "" {
			item.Cover = strings.TrimSpace(text)
			break
		}
	}
	return item
}

func mediaStreamType(rawURL string) string {
	lower := strings.ToLower(rawURL)
	if strings.Contains(lower, "m3u8") {
		return "hls"
	}
	return "mp4"
}

// moduleSourceMeta 由模块清单构造适配层元信息。
func moduleSourceMeta(module *jsModule) source.Meta {
	meta := source.Meta{ID: module.manifest.ID, Name: module.manifest.Name, Version: module.manifest.Version}
	if module.manifest.Adult != nil {
		meta.Adult = *module.manifest.Adult
	}
	meta.Capabilities = module.manifest.Capabilities
	return meta
}

// sourceAdapters 保存当前已安装模块对应的适配器。
var sourceAdapters = source.NewRegistry()

// registerModuleSource 把已加载模块登记进适配层，供上层按 id 取用。
func registerModuleSource(module *jsModule) error {
	if module == nil || module.manifest.ID == "" {
		return nil
	}
	if module.active == nil {
		module.active = newJSProvider(module)
	}
	adapter := source.NewAdapter(moduleSourceMeta(module), newModuleSourceHandler(module.active), source.DefaultPolicy())
	return sourceAdapters.Register(adapter)
}

// storeModule 登记模块并同步注册适配器，模块进入运行期的唯一入口。
func storeModule(module *jsModule) {
	if module == nil || module.manifest.ID == "" {
		return
	}
	moduleRegistry.Lock()
	moduleRegistry.modules[module.manifest.ID] = module
	moduleRegistry.Unlock()
	_ = registerModuleSource(module)
}

// dropModule 注销模块并移除其适配器。
func dropModule(id string) {
	id = strings.ToLower(strings.TrimSpace(id))
	if id == "" {
		return
	}
	moduleRegistry.Lock()
	delete(moduleRegistry.modules, id)
	moduleRegistry.Unlock()
	sourceAdapters.Unregister(id)
}

// SourceAdapters 返回适配层注册表，供聚合引擎与 IPC 查询。
func SourceAdapters() *source.Registry { return sourceAdapters }

// adapterStream 按源 id 取播放地址，走适配层策略（超时、重试、限速、健康）。
func adapterStream(ctx context.Context, id string, request source.StreamRequest) (source.MediaStream, error) {
	item, ok := sourceAdapters.Lookup(id)
	if !ok {
		return source.MediaStream{}, source.Permanent("stream", errUnknownSource(id))
	}
	return item.Stream(ctx, request)
}

// adapterCatalog 按源 id 取目录页。
func adapterCatalog(ctx context.Context, id string, page int, category string) ([]source.CatalogItem, bool, error) {
	item, ok := sourceAdapters.Lookup(id)
	if !ok {
		return nil, false, source.Permanent("catalog", errUnknownSource(id))
	}
	return item.Catalog(ctx, page, category)
}

// adapterDetail 按源 id 取详情。
func adapterDetail(ctx context.Context, id string, sourceID string) (source.DramaDetail, error) {
	item, ok := sourceAdapters.Lookup(id)
	if !ok {
		return source.DramaDetail{}, source.Permanent("detail", errUnknownSource(id))
	}
	return item.Detail(ctx, sourceID)
}

type unknownSourceError string

func (e unknownSourceError) Error() string { return "未知站源：" + string(e) }

func errUnknownSource(id string) error { return unknownSourceError(id) }
