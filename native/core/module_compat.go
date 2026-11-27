package core

import (
	"encoding/hex"
	"net/url"
	"strconv"
	"strings"
	"time"
	"golang.org/x/net/html"

	"github.com/dop251/goja"
)

// guoapp 模块协议兼容层。
//
// Xanerix/guoapp 的站源模块使用一组比本项目更宽的 host.* 接口。这里把它们
// 全部映射到本项目既有能力（fetch / 模块私有存储 / 加密原语），不新增网络出口，
// 也不改变 goja 根的五导出契约。模块只是本地 zip 插件，失效时换 zip 即可。

// jsTextRequest / jsTextResponse 是 fetchText 的文本形态。
// Body 直接以字符串给出；二进制仍走原 host.fetch。
type jsTextRequest struct {
	Method    string            `json:"method"`
	URL       string            `json:"url"`
	Headers   map[string]string `json:"headers"`
	Body      string            `json:"body"`
	TimeoutMS int64             `json:"timeoutMs"`
	Limit     int64             `json:"limit"`
	Referer   string            `json:"referer"`
	UserAgent string            `json:"userAgent"`
	Cookie    string            `json:"cookie"`
}

type jsTextResponse struct {
	Status   int    `json:"status"`
	Body     string `json:"body"`
	URL      string `json:"url,omitempty"`
	FinalURL string `json:"finalUrl,omitempty"`
}

func moduleCompatMembers(p *jsProvider, instance *jsInstance) map[string]any {
	vm := instance.vm
	bytesOf := func(value goja.Value) []byte {
		if text, ok := value.Export().(string); ok {
			return []byte(text)
		}
		var data []byte
		if err := vm.ExportTo(value, &data); err != nil {
			panic(vm.NewTypeError("需要字符串或字节数组"))
		}
		return append([]byte(nil), data...)
	}
	bytesValue := func(data []byte) goja.Value {
		array, err := vm.New(vm.Get("Uint8Array"), vm.ToValue(vm.NewArrayBuffer(data)))
		if err != nil {
			panic(err)
		}
		return array
	}
	return map[string]any{
		// fetchText 走原 fetch，再把 Body 原样作为文本返回。
		"fetchText": func(request jsTextRequest) jsTextResponse {
			return moduleFetchText(p, instance, request)
		},
		"preserveIntegers": modulePreserveIntegers,
		"parseUrl": func(raw string) goja.Value {
			parsed, err := url.Parse(strings.TrimSpace(raw))
			if err != nil {
				parsed = &url.URL{}
			}
			result := vm.NewObject()
			_ = result.Set("href", parsed.String())
			_ = result.Set("protocol", parsed.Scheme)
			_ = result.Set("hostname", parsed.Hostname())
			_ = result.Set("host", parsed.Host)
			_ = result.Set("port", parsed.Port())
			_ = result.Set("userInfo", parsed.User != nil)
			_ = result.Set("query", parsed.RawQuery)
			_ = result.Set("path", parsed.Path)
			_ = result.Set("pathname", parsed.Path)
			_ = result.Set("search", parsed.RawQuery)
			_ = result.Set("hash", parsed.Fragment)
			search := vm.NewObject()
			for key, values := range parsed.Query() {
				if len(values) > 0 {
					_ = search.Set(key, values[0])
				}
			}
			_ = result.Set("searchParams", search)
			return result
		},
		// parseHtml 把 HTML 转成轻量 DOM 树，字段与 guoapp 模块期望的 tag/attrs/children/text 对齐。
		"parseHtml": func(text string) goja.Value {
			if len(text) > moduleHTMLBytes {
				panic(vm.NewTypeError("网页内容过大"))
			}
			document, err := html.Parse(strings.NewReader(text))
			if err != nil {
				panic(vm.NewTypeError("网页无法解析"))
			}
			return compatHTMLNode(vm, document)
		},
		"resolveUrl": func(base, ref string) (string, error) {
			baseURL, err := url.Parse(base)
			if err != nil {
				return "", err
			}
			refURL, err := url.Parse(ref)
			if err != nil {
				return "", err
			}
			return baseURL.ResolveReference(refURL).String(), nil
		},
		"utf8Encode": func(text string) goja.Value { return bytesValue([]byte(text)) },
		"utf8Decode": func(data goja.Value) string {
			return strings.ToValidUTF8(string(bytesOf(data)), "")
		},
		"toLowerCase": func(text goja.Value) string {
			if goja.IsNull(text) || goja.IsUndefined(text) {
				return ""
			}
			return strings.ToLower(text.String())
		},
		"hexEncode": func(data goja.Value) string {
			return hex.EncodeToString(bytesOf(data))
		},
		"hexDecode": func(text string) (goja.Value, error) {
			data, err := hex.DecodeString(strings.TrimSpace(text))
			if err != nil {
				return nil, err
			}
			return bytesValue(data), nil
		},
		// cacheGet/cacheSet 是 storageGet/storageSet 的别名；TTL 入参单位为毫秒。
		"cacheGet": func(key string) goja.Value {
			store := moduleStorageFor(p.module.manifest.ID)
			value, ok := store.Get(key)
			if !ok {
				return goja.Null()
			}
			return vm.ToValue(value)
		},
		"cacheSet": func(key string, value goja.Value, ttlMS int64) error {
			store := moduleStorageFor(p.module.manifest.ID)
			if value == nil || goja.IsNull(value) || goja.IsUndefined(value) {
				store.Delete(key)
				return nil
			}
			ttl := time.Duration(ttlMS) * time.Millisecond
			if ttl <= 0 {
				ttl = 30 * time.Minute
			}
			return store.Set(key, value.String(), ttl)
		},
		"loadState": func(key string) goja.Value {
			store := moduleStorageFor(p.module.manifest.ID)
			if value, ok := store.Get("state:" + key); ok {
				return vm.ToValue(value)
			}
			return goja.Null()
		},
		"saveState": func(key, value string) error {
			return moduleStorageFor(p.module.manifest.ID).Set("state:"+key, value, 0)
		},
		// 播放类模块用 prepareWebMedia / finishMedia 透传 Drama / Task。
		"prepareWebMedia": func(value any) any { return value },
		"finishMedia":     func(value any) any { return value },
		"exclusive": func(call goja.FunctionCall) goja.Value {
			fn, ok := goja.AssertFunction(call.Argument(0))
			if !ok {
				panic(vm.NewTypeError("exclusive 需要一个函数"))
			}
			result, err := fn(goja.Undefined(), call.Arguments[1:]...)
			if err != nil {
				panic(err)
			}
			return result
		},
		"sleep": func(ms int64) {
			if ms <= 0 || ms > 5000 || instance.ctx.Err() != nil {
				return
			}
			timer := time.NewTimer(time.Duration(ms) * time.Millisecond)
			defer timer.Stop()
			select {
			case <-timer.C:
			case <-instance.ctx.Done():
			}
		},
		"log": func(args ...goja.Value) {
			for _, arg := range args {
				_ = arg
			}
		},
		"console": map[string]any{
			"log": func(args ...goja.Value) {
				for _, arg := range args {
					_ = arg
				}
			},
		},
		"preferredSite": func() string { return p.module.manifest.ID },
		"fetchPlaylist": func(request jsTextRequest) jsTextResponse {
			return moduleCompatMembers(p, instance)["fetchText"].(func(jsTextRequest) jsTextResponse)(request)
		},
	}
}

// moduleFetchText 是 fetchText 与全局 req/http 共用的取文本实现。
func moduleFetchText(p *jsProvider, instance *jsInstance, request jsTextRequest) jsTextResponse {
	headers := map[string]string{}
	for key, value := range request.Headers {
		headers[key] = value
	}
	if request.Referer != "" {
		headers["Referer"] = request.Referer
	}
	if request.UserAgent != "" {
		headers["User-Agent"] = request.UserAgent
	}
	if request.Cookie != "" {
		headers["Cookie"] = request.Cookie
	}
	if request.Method == "" {
		request.Method = "GET"
	}
	native := jsFetchRequest{
		Method: request.Method, URL: request.URL, Headers: headers,
		Body: request.Body, TimeoutMS: request.TimeoutMS, Limit: request.Limit,
		FollowRedirects: true,
	}
	response, err := p.fetch(instance.ctx, native)
	if err != nil {
		return jsTextResponse{Status: 0, Body: "", URL: request.URL}
	}
	return jsTextResponse{Status: response.Status, Body: response.Body, URL: request.URL, FinalURL: request.URL}
}

// installModuleGlobals 注入兼容 FongMi/TVBox 站点脚本的全局符号。
//
// global / window / self 三个别名都指向 globalThis，让浏览器风格的脚本原样可跑；
// req / http 是同步取文本函数，对应 TVBox 的 req(url, options) 契约，返回
// { status, body, content, url, finalUrl }。模块自身定义的同名局部函数会遮蔽全局，
// 不会影响既有模块。
func installModuleGlobals(vm *goja.Runtime, p *jsProvider, instance *jsInstance) {
	global := vm.GlobalObject()
	for _, name := range []string{"global", "window", "self"} {
		if existing := global.Get(name); existing != nil && !goja.IsUndefined(existing) {
			continue
		}
		_ = global.DefineDataProperty(name, global, goja.FLAG_TRUE, goja.FLAG_TRUE, goja.FLAG_TRUE)
	}
	requestText := func(call goja.FunctionCall) goja.Value {
		var url, method, body string
		headers := map[string]string{}
		timeout := int64(0)
		for index := 0; index < len(call.Arguments); index++ {
			arg := call.Argument(index)
			if goja.IsUndefined(arg) || goja.IsNull(arg) {
				continue
			}
			switch index {
			case 0:
				url = arg.String()
			case 1:
				options := arg.ToObject(vm)
				if value := options.Get("method"); value != nil && !goja.IsUndefined(value) {
					method = strings.ToUpper(value.String())
				}
				if value := options.Get("body"); value != nil && !goja.IsUndefined(value) {
					body = value.String()
				}
				if value := options.Get("timeout"); value != nil && !goja.IsUndefined(value) {
					timeout = value.ToInteger()
				}
				if value := options.Get("headers"); value != nil && !goja.IsUndefined(value) {
					object := value.ToObject(vm)
					for _, key := range object.Keys() {
						if item := object.Get(key); item != nil && !goja.IsUndefined(item) {
							headers[key] = item.String()
						}
					}
				}
				if value := options.Get("referer"); value != nil && !goja.IsUndefined(value) {
					headers["Referer"] = value.String()
				}
				if value := options.Get("userAgent"); value != nil && !goja.IsUndefined(value) {
					headers["User-Agent"] = value.String()
				}
				if value := options.Get("cookie"); value != nil && !goja.IsUndefined(value) {
					headers["Cookie"] = value.String()
				}
			}
		}
		response := moduleFetchText(p, instance, jsTextRequest{
			Method: method, URL: url, Headers: headers, Body: body, TimeoutMS: timeout,
		})
		result := vm.NewObject()
		_ = result.Set("status", response.Status)
		_ = result.Set("body", response.Body)
		_ = result.Set("content", response.Body)
		_ = result.Set("url", response.URL)
		_ = result.Set("finalUrl", response.FinalURL)
		return result
	}
	_ = global.Set("req", requestText)
	_ = global.Set("http", requestText)
}

// compatHTMLNode 把 html.Node 转成 guoapp 模块读取的普通对象。
// 只暴露模块用到的 tag / attrs / children / text，不引入外部依赖。
func compatHTMLNode(vm *goja.Runtime, node *html.Node) goja.Value {
	return compatHTMLNodeWithParent(vm, node, nil)
}

func compatHTMLNodeWithParent(vm *goja.Runtime, node *html.Node, parent goja.Value) goja.Value {
	object := vm.NewObject()
	if node.Type == html.TextNode || node.Type == html.CommentNode {
		_ = object.Set("tag", goja.Undefined())
		_ = object.Set("text", node.Data)
	} else {
		_ = object.Set("tag", strings.ToLower(node.Data))
		_ = object.Set("text", "")
	}
	if parent != nil {
		_ = object.Set("parent", parent)
	}
	attrs := vm.NewObject()
	for _, attr := range node.Attr {
		_ = attrs.Set(attr.Key, attr.Val)
	}
	// guoapp 模块直接 htmlAttr(node,"type").toLowerCase()；保证该键永远是字符串。
	if attrs.Get("type") == nil {
		_ = attrs.Set("type", "")
	}
	_ = object.Set("attrs", attrs)
	children := vm.NewArray()
	index := 0
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		_ = children.Set(strconv.Itoa(index), compatHTMLNodeWithParent(vm, child, object))
		index++
	}
	_ = object.Set("children", children)
	return object
}

func modulePreserveIntegers(text string) string {
	var out strings.Builder
	out.Grow(len(text) + 64)
	inString, escaped := false, false
	for index := 0; index < len(text); {
		char := text[index]
		switch {
		case inString:
			if escaped {
				escaped = false
			} else if char == '\\' {
				escaped = true
			} else if char == '"' {
				inString = false
			}
		case char == '"':
			inString = true
		case char == '-' || char >= '0' && char <= '9':
			end := index + 1
			for end < len(text) && strings.IndexByte("0123456789+-.eE", text[end]) >= 0 {
				end++
			}
			token := text[index:end]
			if value, err := strconv.ParseInt(token, 10, 64); !strings.ContainsAny(token, ".eE+") && (err != nil || value > 1<<53-1 || value < -(1<<53-1)) {
				out.WriteByte('"')
				out.WriteString(token)
				out.WriteByte('"')
			} else {
				out.WriteString(token)
			}
			index = end
			continue
		}
		out.WriteByte(char)
		index++
	}
	return out.String()
}
