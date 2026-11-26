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

var bundledModules = []moduleManifest{}
