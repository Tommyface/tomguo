package core

import (
	"context"
	"strings"
	"testing"

	"github.com/dop251/goja"
)

// TestJSProviderAsyncExport 验证 api 2 的 async 导出能通过 jsProvider.call 正常返回。
func TestJSProviderAsyncExport(t *testing.T) {
	manifest := moduleManifest{
		ID: "async-test", Name: "异步测试", Version: "1.0.0",
		Runtime: moduleRuntimeJS, API: 1, Adult: boolPtr(false),
		Capabilities: []string{"catalog"}, MainSha256: "",
	}
	code := []byte(`
		module.exports.catalog = async function(args) {
			const first = await Promise.resolve("a");
			const second = await Promise.resolve("b");
			return { items: [{ id: first + second, title: "异步模块" }], hasMore: false };
		};
	`)
	provider, err := newTestJSProvider(manifest, code)
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Items   []struct{ ID, Title string } `json:"items"`
		HasMore bool                         `json:"hasMore"`
	}
	if err := provider.call(context.Background(), "catalog", &result, map[string]any{"page": 1}); err != nil {
		t.Fatal(err)
	}
	if len(result.Items) != 1 || result.Items[0].ID != "ab" {
		t.Fatalf("异步导出结果不符：%+v", result)
	}
}

// TestJSProviderSyncExportStillWorks 验证 api 1 的同步导出仍然兼容。
func TestJSProviderSyncExportStillWorks(t *testing.T) {
	manifest := moduleManifest{
		ID: "sync-test", Name: "同步测试", Version: "1.0.0",
		Runtime: moduleRuntimeJS, API: 1, Adult: boolPtr(false),
		Capabilities: []string{"catalog"}, MainSha256: "",
	}
	code := []byte(`exports.catalog = function(args) { return { items: [{id:"s1", title:"同步"}], hasMore:false }; };`)
	provider, err := newTestJSProvider(manifest, code)
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Items []struct{ ID, Title string } `json:"items"`
	}
	if err := provider.call(context.Background(), "catalog", &result, map[string]any{"page": 1}); err != nil {
		t.Fatal(err)
	}
	if len(result.Items) != 1 || result.Items[0].ID != "s1" {
		t.Fatalf("同步导出结果不符：%+v", result)
	}
}

func TestJSProviderExportError(t *testing.T) {
	manifest := moduleManifest{
		ID: "error-test", Name: "错误测试", Version: "1.0.0",
		Runtime: moduleRuntimeJS, API: 1, Adult: boolPtr(false),
		Capabilities: []string{"catalog"}, MainSha256: "",
	}
	code := []byte(`exports.catalog = async function() { throw new Error("boom"); };`)
	provider, err := newTestJSProvider(manifest, code)
	if err != nil {
		t.Fatal(err)
	}
	var result any
	err = provider.call(context.Background(), "catalog", &result)
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("期望错误信息包含 boom，收到 %v", err)
	}
}

func newTestJSProvider(manifest moduleManifest, code []byte) (*jsProvider, error) {
	// 真实模块会被 CommonJS wrapper 包成 function(module, exports, host){...}。
	wrapped := "(function(module, exports, host){\n" + string(code) + "\n})"
	program, err := goja.Compile(manifest.ID, wrapped, false)
	if err != nil {
		return nil, err
	}
	module := &jsModule{manifest: manifest, program: program}
	return newJSProvider(module), nil
}
