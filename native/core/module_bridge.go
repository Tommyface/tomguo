package core

import (
	"strings"
	"context"
	"net/http"
	"time"
)

// downloaderModuleHost 把 Downloader 的请求栈暴露给 JS 模块。
type downloaderModuleHost struct {
	d *Downloader
}

func (h downloaderModuleHost) Do(request *http.Request, timeout time.Duration) (*http.Response, error) {
	if host, ok := request.Context().Value(pinnedHostKey{}).(string); ok && host != "" {
		return h.d.doPinnedRequest(request, timeout, host)
	}
	return h.d.doCatalogRequestWithTimeout(request, timeout)
}

// 模块化站源接入：canonicalProviderSource 识别模块 id 后，
// catalog / detail / resolve 走 JS 模块。

// isBuiltinModule 判断该 id 是否为内置站源（实现仍在 Go 侧）。
func isBuiltinModule(source string) bool {
	return isBundledSource(strings.ToLower(strings.TrimSpace(source)))
}

func (d *Downloader) moduleProvider(source string) *jsProvider {
	module := moduleSource(source)
	if module == nil {
		return nil
	}
	module.active.host = downloaderModuleHost{d: d}
	return module.active
}

func (d *Downloader) moduleCatalog(ctx context.Context, source string, page int, category, query string) ([]Drama, bool, error) {
	provider := d.moduleProvider(source)
	if provider == nil {
		return nil, false, errModuleMissing(source)
	}
	if query != "" {
		return provider.Search(ctx, page, query)
	}
	return provider.CatalogPage(ctx, page, category)
}

func (d *Downloader) moduleDetail(ctx context.Context, source, sourceID string) (Drama, []Chapter, error) {
	provider := d.moduleProvider(source)
	if provider == nil {
		return Drama{}, nil, errModuleMissing(source)
	}
	return provider.Detail(ctx, sourceID)
}

func (d *Downloader) moduleResolveMedia(ctx context.Context, source string, task Task) (providerMedia, error) {
	provider := d.moduleProvider(source)
	if provider == nil {
		return providerMedia{}, errModuleMissing(source)
	}
	return provider.ResolveMedia(ctx, task)
}

func errModuleMissing(source string) error {
	return &moduleMissingError{source: source}
}

type moduleMissingError struct{ source string }

func (e *moduleMissingError) Error() string {
	return "站源模块 " + e.source + " 未安装，请在 我的 › 模块 中安装后重试"
}
