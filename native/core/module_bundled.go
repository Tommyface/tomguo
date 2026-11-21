package core

// 内置站源改为「随应用分发的模块」：
// 它们的实现仍在 Go 侧（各 provider_*.go），这里只登记清单，
// 让站源列表、搜索、换源都统一从模块清单读取，不再有两套来源。

func boolPtr(value bool) *bool { return &value }

var bundledModules = []moduleManifest{
	{ID: sourceHongguo, Name: "红果", Description: "官方短剧 · 分类与搜索", Runtime: moduleRuntimeBuiltin, Capabilities: []string{"search", "recommendations"}},
	{ID: sourceHanxiaoquan, Name: "韩小圈", Description: "短剧 · 分类与搜索", Runtime: moduleRuntimeBuiltin, Capabilities: []string{"search"}},
	{ID: sourceGuipian, Name: "鬼片", Description: "短剧 · 分类与搜索", Runtime: moduleRuntimeBuiltin, Capabilities: []string{"search"}},
	{ID: sourceSorani, Name: "青空次元", Description: "动漫 · 分类与搜索", Runtime: moduleRuntimeBuiltin, Capabilities: []string{"search"}},
	{ID: sourceHuangdou, Name: "黄豆", Description: "精选短剧 · AES + HMAC 加密接口", Runtime: moduleRuntimeBuiltin, Capabilities: []string{"search"}},
	{ID: sourceHuangju, Name: "剧果", Description: "热门 · 最新 · 分类短剧", Runtime: moduleRuntimeBuiltin, Capabilities: []string{"search"}},
	{ID: sourceChengguo, Name: "橙果", Description: "原创 · AI 短剧 · 分类短剧", Runtime: moduleRuntimeBuiltin, Capabilities: []string{"search"}},
	{ID: sourceHuakui, Name: "花魁", Description: "剧集站 huakui.net", Runtime: moduleRuntimeBuiltin, Capabilities: []string{"search"}},
	{ID: sourceHuangguoMain, Name: "黄果主站", Description: "falpius / whmxun 主站", Runtime: moduleRuntimeBuiltin, Capabilities: []string{"search"}},
	{ID: sourceHuangguoSelected, Name: "黄果精选", Description: "发布页精选入口", Runtime: moduleRuntimeBuiltin, Capabilities: []string{"search"}},
	{ID: sourceHuangguoWelfare, Name: "黄果福利", Description: "发布页福利入口", Runtime: moduleRuntimeBuiltin, Capabilities: []string{"search"}},
	{ID: sourceYeguo, Name: "野果", Description: "分类短剧 · 在线搜索", Runtime: moduleRuntimeBuiltin, Capabilities: []string{"search"}},
	{ID: sourceHuangguoVideo, Name: "黄果视频", Description: "视频剧集", Runtime: moduleRuntimeBuiltin, Capabilities: []string{"search"}},
	{ID: sourceHuangguoAI, Name: "黄果 AI", Description: "AI 短剧 · 走 Worker 反代", Runtime: moduleRuntimeBuiltin, Capabilities: []string{"search"}},
	{ID: sourceCloudFront, Name: "黄果旧版", Description: "旧 API 剧库", Runtime: moduleRuntimeBuiltin},
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
