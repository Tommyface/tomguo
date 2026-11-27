package core

import (
	"archive/zip"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// 模块管理动作：列出 / 安装 / 卸载。安装来源是本地 zip（用户从别处下载后选入）。

type moduleStatus struct {
	moduleManifest
	Installed      bool     `json:"installed"`
	Builtin        bool     `json:"builtin,omitempty"`
	Error          string   `json:"error,omitempty"`
	Root           string   `json:"root,omitempty"`
	Warnings       []string `json:"warnings,omitempty"`
	Disabled       bool     `json:"disabled,omitempty"`
	DisabledReason string   `json:"disabledReason,omitempty"`
}

func (engine *nativeEngine) listModules() []moduleStatus {
	out := make([]moduleStatus, 0, len(bundledModules))
	seen := map[string]bool{}
	// 内置站源排前面，卸载不掉，只能停用。
	for _, manifest := range bundledModules {
		seen[manifest.ID] = true
		status := moduleStatus{moduleManifest: manifest, Installed: true, Builtin: true}
		engine.applySourceHealth(&status)
		out = append(out, status)
	}
	for _, manifest := range ModuleManifests() {
		if seen[manifest.ID] {
			continue
		}
		seen[manifest.ID] = true
		status := moduleStatus{
			moduleManifest: manifest, Installed: true,
			Root: filepath.Join(moduleRoot(engine.directory), manifest.ID),
		}
		if module := moduleSource(manifest.ID); module != nil {
			status.Warnings = module.Warnings()
		}
		engine.applySourceHealth(&status)
		out = append(out, status)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Builtin != out[j].Builtin {
			return out[i].Builtin
		}
		return out[i].ID < out[j].ID
	})
	return out
}

func (engine *nativeEngine) applySourceHealth(status *moduleStatus) {
	if status.Retired {
		status.Disabled = true
		status.DisabledReason = status.RetiredReason
		if status.DisabledReason == "" {
			status.DisabledReason = "该站源已停用"
		}
		return
	}
	if engine == nil {
		return
	}
	reachable, reason := engine.sourceReachable(status.ID)
	if !reachable {
		status.Disabled = true
		status.DisabledReason = reason
	}
}

// installModule 解压 zip 到 modules/<id>，校验后重新注册。
func (engine *nativeEngine) installModule(zipPath string) (moduleStatus, error) {
	zipPath = strings.TrimSpace(zipPath)
	if zipPath == "" {
		return moduleStatus{}, errors.New("请选择模块压缩包")
	}
	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		return moduleStatus{}, errors.New("模块压缩包无法打开")
	}
	defer reader.Close()

	// 先找 manifest.json，确定模块 id 与解压根目录。
	var manifestEntry *zip.File
	for _, file := range reader.File {
		if filepath.Base(file.Name) == moduleManifestFile {
			manifestEntry = file
			break
		}
	}
	if manifestEntry == nil {
		return moduleStatus{}, errors.New("压缩包内没有 manifest.json")
	}
	raw, err := readZipFile(manifestEntry)
	if err != nil {
		return moduleStatus{}, err
	}
	var manifest moduleManifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return moduleStatus{}, errors.New("manifest.json 无法解析")
	}
	if !moduleIDPattern.MatchString(manifest.ID) {
		return moduleStatus{}, errors.New("模块标识只能用小写字母、数字、下划线和连字符，最长 32 个字符")
	}

	prefix := strings.TrimSuffix(manifestEntry.Name, moduleManifestFile)
	target := filepath.Join(moduleRoot(engine.directory), manifest.ID)
	if err := os.MkdirAll(target, 0o700); err != nil {
		return moduleStatus{}, err
	}
	for _, file := range reader.File {
		name := strings.TrimPrefix(file.Name, prefix)
		if name == "" || strings.HasSuffix(name, "/") {
			continue
		}
		if strings.Contains(name, "..") {
			return moduleStatus{}, errors.New("压缩包内路径非法")
		}
		destination := filepath.Join(target, name)
		if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
			return moduleStatus{}, err
		}
		if err := extractZipFile(file, destination); err != nil {
			return moduleStatus{}, err
		}
	}

	if _, err := readModule(target); err != nil {
		_ = os.RemoveAll(target)
		return moduleStatus{}, err
	}
	dropModule(manifest.ID)
	if _, err := LoadModules(engine.directory); err != nil {
		return moduleStatus{}, err
	}
	status := moduleStatus{moduleManifest: manifest, Installed: true, Root: target}
	if module := moduleSource(manifest.ID); module != nil {
		// 用重新注册后的清单回报，capabilities 已是过滤后的结果。
		status.moduleManifest = module.manifest
		status.Warnings = module.Warnings()
	}
	return status, nil
}

func (engine *nativeEngine) removeModule(id string) error {
	id = strings.ToLower(strings.TrimSpace(id))
	if isBundledSource(id) {
		return errors.New("内置站源随应用分发，不能卸载；可在站源管理里停用")
	}
	if !moduleIDPattern.MatchString(id) {
		return errors.New("模块标识无效")
	}
	dropModule(id)
	// 卸载即清空该模块的存储，避免残留令牌与设备标识。
	DropModuleStorage(id)
	target := filepath.Join(moduleRoot(engine.directory), id)
	if err := os.RemoveAll(target); err != nil {
		return err
	}
	return nil
}

func readZipFile(file *zip.File) ([]byte, error) {
	reader, err := file.Open()
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	return io.ReadAll(io.LimitReader(reader, moduleEntryBytes))
}

func extractZipFile(file *zip.File, destination string) error {
	reader, err := file.Open()
	if err != nil {
		return err
	}
	defer reader.Close()
	out, err := os.OpenFile(destination, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, io.LimitReader(reader, moduleEntryBytes))
	return err
}
