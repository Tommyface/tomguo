package core

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/dop251/goja"
)

var moduleIDPattern = regexp.MustCompile(`^[a-z0-9_\-]{1,32}$`)

type jsModule struct {
	manifest moduleManifest
	program  *goja.Program
	active   *jsProvider
}

// moduleRegistry 保存已加载的模块，key 为模块 id。
var moduleRegistry = struct {
	sync.RWMutex
	modules map[string]*jsModule
}{modules: map[string]*jsModule{}}

// moduleRoot 返回模块目录：<dataDir>/modules
func moduleRoot(dataDir string) string {
	return filepath.Join(dataDir, "modules")
}

// LoadModules 扫描模块目录并注册全部合法模块。
func LoadModules(dataDir string) ([]moduleManifest, error) {
	root := moduleRoot(dataDir)
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var loaded []moduleManifest
	var firstErr error
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		module, err := readModule(filepath.Join(root, entry.Name()))
		if err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("%s: %w", entry.Name(), err)
			}
			continue
		}
		moduleRegistry.Lock()
		moduleRegistry.modules[module.manifest.ID] = module
		moduleRegistry.Unlock()
		loaded = append(loaded, module.manifest)
	}
	return loaded, firstErr
}

// ModuleManifests 返回全部站源清单：内置（随应用分发）+ 已安装的 JS 模块。
// 界面只认这一份清单，不再区分「内置源」与「模块源」。
func ModuleManifests() []moduleManifest {
	moduleRegistry.RLock()
	defer moduleRegistry.RUnlock()
	out := make([]moduleManifest, 0, len(bundledModules)+len(moduleRegistry.modules))
	out = append(out, bundledModules...)
	for _, module := range moduleRegistry.modules {
		out = append(out, module.manifest)
	}
	return out
}

// isBundledSource 判断某个 id 是否为随应用分发的内置站源。
func isBundledSource(id string) bool {
	for _, manifest := range bundledModules {
		if manifest.ID == id {
			return true
		}
	}
	return false
}

// moduleSource 查找模块；未注册返回 nil。只做小写归一，不回调 canonicalProviderSource，
// 否则与 canonicalProviderSource 的模块判断互递归。
func moduleSource(id string) *jsModule {
	normalized := strings.ToLower(strings.TrimSpace(id))
	if normalized == "" {
		return nil
	}
	moduleRegistry.RLock()
	defer moduleRegistry.RUnlock()
	return moduleRegistry.modules[normalized]
}

func readModule(directory string) (*jsModule, error) {
	raw, err := os.ReadFile(filepath.Join(directory, moduleManifestFile))
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(filepath.Join(directory, moduleEntryFile))
	if err != nil {
		return nil, err
	}
	if info.Size() > moduleEntryBytes {
		return nil, fmt.Errorf("%s 超过 %d MB", moduleEntryFile, moduleEntryBytes>>20)
	}
	code, err := os.ReadFile(filepath.Join(directory, moduleEntryFile))
	if err != nil {
		return nil, err
	}
	return parseModule(raw, code)
}

func parseModule(raw, code []byte) (*jsModule, error) {
	var manifest moduleManifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return nil, fmt.Errorf("%s 无法解析：%w", moduleManifestFile, err)
	}
	if !moduleIDPattern.MatchString(manifest.ID) {
		return nil, errors.New("模块标识只能用小写字母、数字、下划线和连字符，最长 32 个字符")
	}
	if strings.TrimSpace(manifest.Name) == "" {
		return nil, errors.New("模块缺少名称")
	}
	if manifest.Runtime != moduleRuntimeJS {
		return nil, fmt.Errorf("不支持的模块运行时 %q", manifest.Runtime)
	}
	if manifest.API != moduleAPIVersion {
		return nil, fmt.Errorf("模块接口版本为 %d，当前应用支持 %d", manifest.API, moduleAPIVersion)
	}
	if manifest.Adult == nil {
		return nil, errors.New("模块缺少 adult 声明")
	}
	for _, name := range manifest.Capabilities {
		if _, known := moduleCapabilityNames[name]; !known {
			return nil, fmt.Errorf("未知的模块能力 %q", name)
		}
	}
	// CommonJS 包装：模块代码里用 module.exports，需要注入 module/exports/host。
	wrapped := "(function(module, exports, host){" + string(code) + "\n})"
	program, err := goja.Compile(manifest.ID, wrapped, true)
	if err != nil {
		return nil, fmt.Errorf("模块脚本编译失败：%w", err)
	}
	module := &jsModule{manifest: manifest, program: program}
	module.active = newJSProvider(module)
	return module, nil
}
