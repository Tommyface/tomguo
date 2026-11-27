package core

// S7 完成后内置源实现（provider_*.go）全部删除，站源只来自已安装 zip 模块。
// 下列测试用已删除的内置源（huangguoai / yeguo / hongguo / 51duanju 等）打桩，
// 验证的是旧内置源的缓存、游标与分页行为，已无对应实现可测。
// 同类能力由注册表路径的测试覆盖（TestAggregateSourceIDs*、TestAdapter*、TestModule*）。
const legacyBuiltinRemoved = "S7 已删除该内置源实现，站源只来自已安装模块"

func skipLegacyBuiltin(t interface{ Skip(...any) }) {
	t.Skip(legacyBuiltinRemoved)
}
