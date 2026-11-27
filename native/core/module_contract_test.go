package core

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestExportScannerDirectStyle(t *testing.T) {
	code := []byte(`
		exports.categories = function() { return []; };
		exports.catalogPage = function() { return {}; };
		exports.detail = function() { return {}; };
		exports.resolveMedia = function() { return {}; };
	`)
	found := exportedActions(code)
	for _, action := range []string{"categories", "catalogPage", "detail", "resolveMedia"} {
		if !found[action] {
			t.Errorf("直接赋值写法未识别 %s：%v", action, found)
		}
	}
	if err := checkModuleContract("t", code, nil); err != nil {
		t.Fatalf("四导出模块应过闸：%v", err)
	}
}

func TestExportScannerObjectStyle(t *testing.T) {
	code := []byte(`
		module.exports = {
			categories: function() { return []; },
			catalogPage: function(page) { return { items: [], inner: { id: 1 } }; },
			detail: function() { return {}; },
			resolveMedia: function() { return {}; },
		};
	`)
	found := exportedActions(code)
	for _, action := range []string{"categories", "catalogPage", "detail", "resolveMedia"} {
		if !found[action] {
			t.Errorf("对象字面量写法未识别 %s：%v", action, found)
		}
	}
	if found["id"] {
		t.Errorf("内部对象键不应被当作顶层导出：%v", found)
	}
	if err := checkModuleContract("t", code, nil); err != nil {
		t.Fatalf("四导出模块应过闸：%v", err)
	}
}

func TestExportScannerConditionalSearch(t *testing.T) {
	code := []byte(`
		module.exports = {
			categories: function(){}, catalogPage: function(){},
			detail: function(){}, resolveMedia: function(){},
		};
	`)
	if err := checkModuleContract("t", code, nil); err != nil {
		t.Fatalf("未声明 search 时应过闸：%v", err)
	}
	if err := checkModuleContract("t", code, []string{"search"}); err == nil {
		t.Fatal("声明 search 但缺导出应拒绝")
	}
}

func TestExportScannerRejectsMissing(t *testing.T) {
	code := []byte(`exports.categories = function(){}; exports.detail = function(){};`)
	if err := checkModuleContract("t", code, nil); err == nil {
		t.Fatal("缺 catalogPage / resolveMedia 应拒绝")
	}
}

func TestRealModulesPassContractGate(t *testing.T) {
	dirs := []string{"jinpai", "xingya", "huangdou", "huaguo"}
	for _, id := range dirs {
		dir := filepath.Join("/root/workspace/modules", id)
		code, err := os.ReadFile(filepath.Join(dir, "main.js"))
		if err != nil {
			t.Logf("%s: 跳过（文件不存在）%v", id, err)
			continue
		}
		var manifest struct {
			Capabilities []string `json:"capabilities"`
		}
		if raw, err := os.ReadFile(filepath.Join(dir, "manifest.json")); err == nil {
			_ = json.Unmarshal(raw, &manifest)
		}
		if err := checkModuleContract(id, code, manifest.Capabilities); err != nil {
			t.Errorf("%s 未过契约闸：%v", id, err)
		}
	}
}
