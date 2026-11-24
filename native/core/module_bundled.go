package core

// 内置站源改为「随应用分发的模块」：
// 它们的实现仍在 Go 侧（各 provider_*.go），这里只登记清单，
// 让站源列表、搜索、换源都统一从模块清单读取，不再有两套来源。

func boolPtr(value bool) *bool { return &value }

// retiredSources 是实测长期不可用的内置源，默认不展示、不再发起请求。
// 站点恢复后从这里与 bundledModules 的 Retired 标记一起删掉即可。
func isRetiredSource(source string) bool {
	source = canonicalProviderSource(source)
	for _, manifest := range bundledModules {
		if manifest.Retired && manifest.ID == source {
			return true
		}
	}
	return false
}

var bundledModules = []moduleManifest{
	{ID: sourceHongguo, Name: "红果", Description: "官方短剧 · 分类与搜索", Runtime: moduleRuntimeBuiltin, Capabilities: []string{"search", "recommendations"}},
	{ID: sourceHanxiaoquan, Name: "韩小圈", Description: "短剧 · 分类与搜索", Runtime: moduleRuntimeBuiltin, Capabilities: []string{"search"}},
	{ID: sourceGuipian, Name: "鬼片", Description: "短剧 · 分类与搜索", Runtime: moduleRuntimeBuiltin, Capabilities: []string{"search"}},
	{ID: sourceSorani, Name: "青空次元", Description: "动漫 · 分类与搜索", Runtime: moduleRuntimeBuiltin, Capabilities: []string{"search"}, Retired: true, RetiredReason: "青空次元：api.sorani.cc 域名解析失败"},
	{ID: sourceHuangdou, Name: "黄豆", Description: "精选短剧 · AES + HMAC 加密接口", Runtime: moduleRuntimeBuiltin, Capabilities: []string{"search"}},
	{ID: sourceHuangju, Name: "剧果", Description: "热门 · 最新 · 分类短剧", Runtime: moduleRuntimeBuiltin, Capabilities: []string{"search"}},
	{ID: sourceChengguo, Name: "橙果", Description: "原创 · AI 短剧 · 分类短剧", Runtime: moduleRuntimeBuiltin, Capabilities: []string{"search"}, Retired: true, RetiredReason: "橙果：内置实现依赖空密钥，已由同 id 插件取代"},
	{ID: sourceHuakui, Name: "花魁", Description: "剧集站 huakui.net", Runtime: moduleRuntimeBuiltin, Capabilities: []string{"search"}},
	{ID: sourceHuangguoMain, Name: "黄果主站", Description: "falpius / whmxun 主站", Runtime: moduleRuntimeBuiltin, Capabilities: []string{"search"}},
	{ID: sourceHuangguoSelected, Name: "黄果精选", Description: "发布页精选入口", Runtime: moduleRuntimeBuiltin, Capabilities: []string{"search"}},
	{ID: sourceHuangguoWelfare, Name: "黄果福利", Description: "发布页福利入口", Runtime: moduleRuntimeBuiltin, Capabilities: []string{"search"}},
	{ID: sourceYeguo, Name: "野果", Description: "分类短剧 · 在线搜索", Runtime: moduleRuntimeBuiltin, Capabilities: []string{"search"}, Retired: true, RetiredReason: "野果：全部线路不可用，发布页无有效接口入口"},
	{ID: sourceHuangguoVideo, Name: "黄果视频", Description: "视频剧集", Runtime: moduleRuntimeBuiltin, Capabilities: []string{"search"}, Retired: true, RetiredReason: "黄果视频：服务器拒绝连接（130.211.15.150:443）"},
	{ID: sourceHuangguoAI, Name: "黄果 AI", Description: "AI 短剧 · 走 Worker 反代", Runtime: moduleRuntimeBuiltin, Capabilities: []string{"search"}, Retired: true, RetiredReason: "黄果 AI：全部镜像 60s 超时"},
	{ID: sourceCloudFront, Name: "黄果旧版", Description: "旧 API 剧库", Runtime: moduleRuntimeBuiltin},
	{ID: sourceDSD, Name: "DSD", Description: "旧聚合接口", Runtime: moduleRuntimeBuiltin, Retired: true, RetiredReason: "DSD：主域 26s 超时"},
	{ID: sourceDuanju51, Name: "51短剧", Description: "短剧 · 全集合集", Runtime: moduleRuntimeBuiltin, Capabilities: []string{"search"}},
	{ID: sourceHema, Name: "河马", Description: "短剧 · 全集合集", Runtime: moduleRuntimeBuiltin, Capabilities: []string{"search"}},
	{ID: sourceEZDMW, Name: "E站", Description: "动漫 · 新番 · 完结", Runtime: moduleRuntimeBuiltin, Capabilities: []string{"search"}},
	{ID: sourceHuaguo, Name: "花果", Description: "短剧 · 花生短剧", Runtime: moduleRuntimeBuiltin},
	{ID: sourceWangguo, Name: "网果", Description: "短剧 · 短剧网", Runtime: moduleRuntimeBuiltin},
	{ID: sourceFaguo, Name: "发果", Description: "短剧 · 168 影院", Runtime: moduleRuntimeBuiltin},
	{ID: sourceWuguo, Name: "伍果", Description: "短剧 · 五五短剧", Runtime: moduleRuntimeBuiltin},
	{ID: sourceMxdm, Name: "MX动漫", Description: "日漫 · 国漫 · 剧场版", Runtime: moduleRuntimeBuiltin},
	{ID: sourceJcydm, Name: "囧次元", Description: "日漫 · 剧场 · 热搜榜", Runtime: moduleRuntimeBuiltin},
}
