package core

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/dop251/goja"
)

var moduleIDPattern = regexp.MustCompile(`^[a-z0-9_\-]{1,32}$`)

// moduleUnknownCapabilityFatal 控制未知 capabilities 的处理策略。
// false → 忽略未知项并记告警（默认：模块可以按新能力先行发布，旧应用照常可用）
// true  → 拒绝加载（只在需要与规范严格对齐时打开）
var moduleUnknownCapabilityFatal = false

type jsModule struct {
	manifest moduleManifest
	program  *goja.Program
	active   *jsProvider
	warnings []string
}

// Warnings 返回加载期的非致命问题，供站源状态与日志展示。
func (module *jsModule) Warnings() []string {
	if module == nil {
		return nil
	}
	return module.warnings
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

// moduleVersionRoot 返回某个模块的版本根目录：<dataDir>/modules/<id>/versions
func moduleVersionRoot(dataDir, id string) string {
	return filepath.Join(moduleRoot(dataDir), id, "versions")
}

// moduleActiveDir 返回模块当前生效版本目录；没有版本目录时回退到扁平结构。
func moduleActiveDir(dataDir, id string) string {
	versionRoot := moduleVersionRoot(dataDir, id)
	entries, err := os.ReadDir(versionRoot)
	if err != nil {
		return filepath.Join(moduleRoot(dataDir), id)
	}
	var versions []string
	for _, entry := range entries {
		if entry.IsDir() {
			versions = append(versions, entry.Name())
		}
	}
	if len(versions) == 0 {
		return filepath.Join(moduleRoot(dataDir), id)
	}
	sort.Strings(versions)
	return filepath.Join(versionRoot, versions[len(versions)-1])
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
	// 传入模块根目录时，按版本目录规则定位当前生效版本。
	if _, err := os.Stat(filepath.Join(directory, moduleManifestFile)); err != nil {
		parts := strings.Split(filepath.ToSlash(directory), "/")
		if len(parts) > 0 {
			directory = moduleActiveDir(filepath.Dir(filepath.Dir(directory)), parts[len(parts)-1])
		}
	}
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
	if err := checkModuleAPI(manifest); err != nil {
		return nil, err
	}
	knownCapabilities, unknownCapabilities := filterModuleCapabilities(manifest.Capabilities)
	if len(unknownCapabilities) > 0 && moduleUnknownCapabilityFatal {
		return nil, fmt.Errorf("未知的模块能力 %q", unknownCapabilities[0])
	}
	// 只保留当前版本认识的能力，下游不会按未知能力发起调用。
	manifest.Capabilities = knownCapabilities
	// 内容摘要校验：必须在校验通过后再编译，否则等于执行未校验的代码。
	digestWarning, err := verifyModuleDigest(manifest, code)
	if err != nil {
		return nil, err
	}
	warnings := make([]string, 0, 2)
	if digestWarning != "" {
		warnings = append(warnings, digestWarning)
	}
	if len(unknownCapabilities) > 0 {
		warnings = append(warnings, "模块声明了当前版本不认识的能力："+strings.Join(unknownCapabilities, "、")+"，已忽略")
	}
	// CommonJS 包装：模块代码里用 module.exports，需要注入 module/exports/host。
	wrapped := "(function(module, exports, host){" + string(code) + "\n})"
	program, err := goja.Compile(manifest.ID, wrapped, true)
	if err != nil {
		return nil, fmt.Errorf("模块脚本编译失败：%w", err)
	}
	module := &jsModule{manifest: manifest, program: program, warnings: warnings}
	module.active = newJSProvider(module)
	return module, nil
}

// checkModuleAPI 校验模块声明的最低接口版本。
// minApi 为 0 表示不声明；大于当前接口版本说明模块比应用新，只能拒绝。
func checkModuleAPI(manifest moduleManifest) error {
	if manifest.MinAPI < 0 {
		return errors.New("模块的 minApi 不能为负数")
	}
	if manifest.MinAPI > moduleAPIVersion {
		return fmt.Errorf("模块要求接口版本 %d，当前应用支持 %d，请先升级应用", manifest.MinAPI, moduleAPIVersion)
	}
	return nil
}

// filterModuleCapabilities 按声明顺序拆分已知与未知能力，去重并忽略空白项。
func filterModuleCapabilities(declared []string) (known []string, unknown []string) {
	seen := make(map[string]bool, len(declared))
	for _, raw := range declared {
		name := strings.TrimSpace(raw)
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		if _, ok := moduleCapabilityNames[name]; ok {
			known = append(known, name)
		} else {
			unknown = append(unknown, name)
		}
	}
	return known, unknown
}
