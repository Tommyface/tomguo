package core

import (
	"os"
	"regexp"
	"sort"
	"testing"
)

// TestIPCFacadeActionsAreDispatched 四个门面动作必须真的能被分发。
func TestIPCFacadeActionsAreDispatched(t *testing.T) {
	source := readCoreSourceForTest(t, "app_runtime.go")
	for _, action := range IPCFacadeActions() {
		pattern := regexp.MustCompile(`case\s+"` + regexp.QuoteMeta(action) + `"`)
		if !pattern.MatchString(source) {
			t.Errorf("门面动作 %q 在分发里找不到 case", action)
		}
	}
}

// TestIPCInternalActionsAreRegistered 内部动作清单必须与实际分发一致，
// 否则清单会变成一份「看起来像真的」的文档。
func TestIPCInternalActionsAreRegistered(t *testing.T) {
	source := readCoreSourceForTest(t, "app_runtime.go")
	declared := map[string]bool{}
	for _, action := range append(IPCFacadeActions(), IPCInternalActions()...) {
		declared[action] = true
	}
	dispatching := map[string]bool{}
	for _, match := range regexp.MustCompile(`(?m)^\tcase\s+((?:"[a-zA-Z]+"(?:\s*,\s*)?)+):`).FindAllStringSubmatch(source, -1) {
		for _, quoted := range regexp.MustCompile(`"([a-zA-Z]+)"`).FindAllStringSubmatch(match[1], -1) {
			dispatching[quoted[1]] = true
		}
	}
	for action := range dispatching {
		if !declared[action] {
			t.Errorf("分发里有 action %q 但没在 ipc_surface.go 登记", action)
		}
	}
	for action := range declared {
		if !dispatching[action] && action != "initialize" {
			t.Errorf("ipc_surface.go 登记了 %q 但分发里没有", action)
		}
	}
}

// TestIPCInternalActionsDoNotGrow 内部动作清单只减不增（S6 的收敛目标）。
func TestIPCInternalActionsDoNotGrow(t *testing.T) {
	const allowed = 53
	if got := len(IPCInternalActions()); got > allowed {
		t.Fatalf("内部动作 %d 个，超过上限 %d——新功能请走四个门面", got, allowed)
	}
}

// TestIPCSurfaceNoDuplicate 同一动作不能既算门面又算内部。
func TestIPCSurfaceNoDuplicate(t *testing.T) {
	seen := map[string]bool{}
	for _, action := range append(IPCFacadeActions(), IPCInternalActions()...) {
		if seen[action] {
			t.Errorf("动作 %q 重复登记", action)
		}
		seen[action] = true
	}
	if len(IPCInternalActions()) == 0 {
		t.Log("内部动作已清空，S6 完成")
	}
	_ = sort.Strings
	_ = os.Getenv
}
