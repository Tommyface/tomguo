package core

import (
	"context"
	"testing"
	"time"
)

// TestJSProviderTimeoutDoesNotPoisonNextCall 验证超时终止 loop 后，
// 死实例不会回到空闲池污染下一次调用（真机「模块运行环境已停止」回归）。
func TestJSProviderTimeoutDoesNotPoisonNextCall(t *testing.T) {
	manifest := moduleManifest{
		ID: "timeout-test", Name: "超时测试", Version: "1.0.0",
		Runtime: moduleRuntimeJS, API: 1, Adult: boolPtr(false),
		Capabilities: []string{"catalog"}, MainSha256: "",
	}
	code := []byte(`
		module.exports.catalog = function(args) {
			if (args && args.block) return new Promise(function() {});
			return { ok: true };
		};
	`)
	provider, err := newTestJSProvider(manifest, code)
	if err != nil {
		t.Fatal(err)
	}

	shortCtx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	var first any
	if err := provider.call(shortCtx, "catalog", &first, map[string]any{"block": true}); err == nil {
		t.Fatal("期望超时错误，实际成功")
	}

	select {
	case inst := <-provider.idle:
		t.Fatalf("超时实例仍留在空闲池：dead=%v", inst.isDead())
	default:
	}

	var second struct {
		OK bool `json:"ok"`
	}
	if err := provider.call(context.Background(), "catalog", &second, map[string]any{"block": false}); err != nil {
		t.Fatalf("超时后的下一次调用失败：%v", err)
	}
	if !second.OK {
		t.Fatalf("下一次调用结果不符：%+v", second)
	}
}
