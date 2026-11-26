package core

import (
	"sort"
	"strings"
	"sync"
)

// S7 影子模式：已安装的模块与内置实现同时在册时，默认由模块接管同名站源。
// 观察期内可以把某个源临时切回内置实现，用来做逐源输出对比；切回只影响本次
// 进程内的分发，不改磁盘、不改清单，重启即回到默认（模块优先）。
//
// 用法：NativeRequest {"action":"shadow","mode":"builtin","source":"jcydm"}
//       NativeRequest {"action":"shadow","mode":"module","source":"jcydm"}
//       NativeRequest {"action":"shadow","mode":"list"}
// mode=list 返回每个源当前生效的实现（builtin / module / none）。
var shadowState = struct {
	sync.RWMutex
	builtin map[string]bool
}{builtin: map[string]bool{}}

func shadowUseBuiltin(source string) bool {
	normalized := strings.ToLower(strings.TrimSpace(canonicalProviderSource(source)))
	shadowState.RLock()
	defer shadowState.RUnlock()
	return shadowState.builtin[normalized]
}

func setShadowMode(source, mode string) {
	normalized := strings.ToLower(strings.TrimSpace(canonicalProviderSource(source)))
	if normalized == "" {
		return
	}
	shadowState.Lock()
	defer shadowState.Unlock()
	switch mode {
	case "builtin":
		shadowState.builtin[normalized] = true
	case "module":
		delete(shadowState.builtin, normalized)
	}
}

// moduleTakesOver 决定某个源由谁实现。影子观察期内可被切回内置；
// 所有源分发入口都走这个函数，避免某条路径漏掉导致对比结果不可信。
func moduleTakesOver(source string) bool {
	if shadowUseBuiltin(source) {
		return false
	}
	return moduleSource(source) != nil
}

// shadowModes 列出全部内置源当前的实现归属，供对比脚本读取。
func shadowModes() map[string]string {
	out := map[string]string{}
	for _, id := range builtinSourceIDs() {
		hasModule := moduleSource(id) != nil
		switch {
		case !hasModule:
			out[id] = "none"
		case shadowUseBuiltin(id):
			out[id] = "builtin"
		default:
			out[id] = "module"
		}
	}
	return out
}

func shadowModesOrdered() []string {
	modes := shadowModes()
	ids := make([]string, 0, len(modes))
	for id := range modes {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}
