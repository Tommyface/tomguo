package core

import (
	"encoding/json"
	"time"
)

// JS 站源模块：运行时加载，不参与编译期 switch。
// 契约：manifest.json 声明元信息，main.js 通过 module.exports 暴露函数。

const (
	moduleAPIVersion  = 1
	moduleRuntimeJS      = "js"
	moduleRuntimeBuiltin = "builtin"
	moduleManifestFile = "manifest.json"
	moduleEntryFile     = "main.js"
	moduleEntryBytes  = 4 << 20
	moduleInstanceMax = 4
	moduleCallTimeout = 60 * time.Second
	moduleQuickMs     = 5 * time.Second
	moduleCacheMax    = 512
	moduleCacheBytes  = 1 << 20
	moduleHTMLBytes   = 8 << 20
)

// moduleManifest 是 manifest.json 的结构。
type moduleManifest struct {
	ID                string   `json:"id"`
	Name              string   `json:"name"`
	Version           string   `json:"version"`
	Description       string   `json:"description"`
	Runtime           string   `json:"runtime"`
	API               int      `json:"api"`
	Adult             *bool    `json:"adult"`
	Hosts             []string `json:"hosts"`
	Capabilities      []string `json:"capabilities"`
	AllowEmptyCatalog bool     `json:"allowEmptyCatalog"`
}

// moduleCapabilityNames 是 capabilities 的中文说明，供界面展示。
var moduleCapabilityNames = map[string]string{
	"search":        "搜索",
	"suggestions":   "搜索联想",
	"recommendations": "推荐",
	"danmaku":       "弹幕",
	"rankings":      "榜单",
	"cover":         "封面",
	"metadata":      "补充信息",
	"catalogState":  "目录状态",
}

type moduleCacheEntry struct {
	value   string
	expires time.Time
}

// JS 侧结构：与 core 的 Drama/Chapter/providerMedia 对应。

type jsDramaPage struct {
	Items   []Drama `json:"items"`
	HasMore bool    `json:"hasMore"`
	Warning string  `json:"warning"`
}

type jsDetail struct {
	Drama    Drama     `json:"drama"`
	Chapters []Chapter `json:"chapters"`
}

type jsMedia struct {
	URL         string   `json:"url"`
	Referer     string   `json:"referer"`
	Playlist    string   `json:"playlist"`
	HLSKey      []byte   `json:"hlsKey"`
	Quality     int      `json:"quality"`
	DurationMS  int64    `json:"durationMs"`
	Variants    []jsMedia `json:"variants"`
}

func (m jsMedia) providerMedia() providerMedia {
	out := providerMedia{
		URL: m.URL, Referer: m.Referer, Playlist: m.Playlist,
		HLSKey: m.HLSKey, Quality: m.Quality,
	}
	if m.DurationMS > 0 {
		out.Duration = time.Duration(m.DurationMS) * time.Millisecond
	}
	for _, variant := range m.Variants {
		out.Variants = append(out.Variants, variant.providerMedia())
	}
	return out
}

type jsFetchRequest struct {
	Method          string            `json:"method"`
	URL             string            `json:"url"`
	Headers         map[string]string `json:"headers"`
	Body            string            `json:"body"`
	BodyBase64      string            `json:"bodyBase64"`
	TimeoutMS       int64             `json:"timeoutMs"`
	Limit           int64             `json:"limit"`
	FollowRedirects bool              `json:"followRedirects"`
	// ResolveIP 非空时，把连接目标改成该 IP，但 Host 头与 TLS SNI 仍用原域名。
	// 用于绕开本地 DNS 污染（域名解析到假 IP）。
	ResolveIP string `json:"resolveIP"`
}

type jsFetchResponse struct {
	Status     int                 `json:"status"`
	Headers    map[string][]string `json:"headers"`
	Body       string              `json:"body"`
	BodyBase64 string              `json:"bodyBase64"`
}

type jsCategory struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

var _ = json.Marshal
