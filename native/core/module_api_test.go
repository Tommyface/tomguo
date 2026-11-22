package core

import "testing"

func TestCheckModuleAPI(t *testing.T) {
	if err := checkModuleAPI(moduleManifest{MinAPI: 0}); err != nil { t.Fatal("0 应通过") }
	if err := checkModuleAPI(moduleManifest{MinAPI: 1}); err != nil { t.Fatal("等于当前版本应通过") }
	if err := checkModuleAPI(moduleManifest{MinAPI: 2}); err == nil { t.Fatal("高于当前版本应失败") }
	if err := checkModuleAPI(moduleManifest{MinAPI: -1}); err == nil { t.Fatal("负数应失败") }
}

func TestFilterModuleCapabilities(t *testing.T) {
	known, unknown := filterModuleCapabilities([]string{"search", "download", " search ", "recommendations", ""})
	if len(known) != 2 || known[0] != "search" || known[1] != "recommendations" { t.Fatalf("known=%v", known) }
	if len(unknown) != 1 || unknown[0] != "download" { t.Fatalf("unknown=%v", unknown) }
	known, unknown = filterModuleCapabilities(nil)
	if len(known) != 0 || len(unknown) != 0 { t.Fatal("空输入应返回空") }
}
