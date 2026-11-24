package core

import (
	"context"
	"net/http"
)

// moduleResolveFallbackKey 标记：这条模块读请求允许在直连失败后
// 走 DoH 解析 + 钉 IP 重试（TLS SNI 仍是原域名）。
type moduleResolveFallbackKey struct{}

// withModuleResolveFallback 只给可安全重放的请求加标记。
// 有 body 的请求无法重放，加标记会让重试拿到空 body，因此跳过。
func withModuleResolveFallback(request *http.Request) *http.Request {
	if request == nil || request.URL == nil {
		return request
	}
	if request.Method != http.MethodGet && request.Method != http.MethodHead {
		return request
	}
	if request.Body != nil && request.Body != http.NoBody {
		return request
	}
	return request.WithContext(context.WithValue(request.Context(), moduleResolveFallbackKey{}, true))
}
