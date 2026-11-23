package core

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// 从清单安装模块：清单与模块包都强制 HTTPS；成人开关由宿主（Flutter）传入，
// 本层不持有解锁状态、不校验密码，只按传入布尔值过滤。

type registryInstallRequest struct {
	RegistryURL  string `json:"registryUrl"`
	ModuleID     string `json:"moduleId"`
	IncludeAdult bool   `json:"includeAdult"`
	Force        bool   `json:"force"`
}

const (
	registryMaxBytes = 2 << 20
	moduleZipMax     = 8 << 20
	registryTimeout  = 20 * time.Second
	moduleZipTimeout = 30 * time.Second
)

var registryHTTPClient = &http.Client{
	Timeout: moduleZipTimeout,
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if err := requireHTTPS(req.URL.String()); err != nil {
			return err
		}
		if len(via) >= 3 {
			return errors.New("重定向次数过多")
		}
		return nil
	},
}

// requireHTTPS 只允许 https 且带 host 的地址。
func requireHTTPS(raw string) error {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || !strings.EqualFold(parsed.Scheme, "https") || parsed.Host == "" {
		return errors.New("地址必须是 HTTPS")
	}
	return nil
}

// InstallFromRegistry 完成清单下载、条目选择、包下载、摘要校验与版本落盘。
func (engine *nativeEngine) InstallFromRegistry(request registryInstallRequest) (moduleStatus, error) {
	if err := requireHTTPS(request.RegistryURL); err != nil {
		return moduleStatus{}, fmt.Errorf("清单地址无效：%w", err)
	}
	raw, err := downloadRegistry(request.RegistryURL, registryMaxBytes)
	if err != nil {
		return moduleStatus{}, fmt.Errorf("模块清单无法下载：%w", err)
	}
	list, err := ParseModuleRegistry(raw)
	if err != nil {
		return moduleStatus{}, err
	}
	entry, ok := list.FindEntry(request.ModuleID)
	if !ok {
		return moduleStatus{}, fmt.Errorf("清单里没有「%s」", request.ModuleID)
	}
	isAdult := entry.Adult != nil && *entry.Adult
	if isAdult && !request.IncludeAdult {
		return moduleStatus{}, errors.New("成人源未解锁，请先在设置中开启")
	}
	if err := checkModuleAPI(moduleManifest{MinAPI: entry.MinAPI}); err != nil {
		return moduleStatus{}, err
	}
	if err := requireHTTPS(entry.Zip); err != nil {
		return moduleStatus{}, fmt.Errorf("模块包地址无效：%w", err)
	}

	zipPath, err := downloadModuleZip(entry.Zip, moduleRoot(engine.directory))
	if err != nil {
		return moduleStatus{}, fmt.Errorf("模块包无法下载：%w", err)
	}
	defer os.Remove(zipPath)

	code, err := readMainFromZip(zipPath)
	if err != nil {
		return moduleStatus{}, err
	}
	if err := VerifyEntryCode(entry, code); err != nil {
		return moduleStatus{}, err
	}

	version := strings.TrimSpace(entry.Version)
	if version == "" {
		version = "0.0.0"
	}
	target := filepath.Join(moduleVersionRoot(engine.directory, entry.ID), version)
	if _, statErr := os.Stat(target); statErr == nil && !request.Force {
		return moduleStatus{}, fmt.Errorf("模块 %s 版本 %s 已安装", entry.ID, version)
	}

	if err := extractModuleZip(zipPath, target); err != nil {
		_ = os.RemoveAll(target)
		return moduleStatus{}, fmt.Errorf("模块安装失败：%w", err)
	}
	// 第二道闸：readModule → parseModule 会再按 manifest 声明比对摘要。
	module, err := readModule(target)
	if err != nil {
		_ = os.RemoveAll(target) // 坏版本自动回退到上一版
		return moduleStatus{}, err
	}
	DropModuleFetchCache()

	moduleRegistry.Lock()
	moduleRegistry.modules[entry.ID] = module
	moduleRegistry.Unlock()
	return moduleStatus{
		moduleManifest: module.manifest,
		Installed:      true,
		Root:           target,
		Warnings:       module.Warnings(),
	}, nil
}

// registryMirrorBases 是 ccwu.cc 不通时依次尝试的镜像前缀。
// 只做域名替换，路径与文件名保持一致，摘要校验照旧由 VerifyEntryCode 兜底。
var registryMirrorBases = []string{
	"https://fastly.jsdelivr.net/gh/Tommyface/tomguo-app@main",
	"https://gcore.jsdelivr.net/gh/Tommyface/tomguo-app@main",
	"https://cdn.jsdelivr.net/gh/Tommyface/tomguo-app@main",
}

// mirrorCandidates 返回与 rawURL 等价的镜像地址；原地址不在列表里。
func mirrorCandidates(rawURL string) []string {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || parsed.Host == "" {
		return nil
	}
	if !strings.HasSuffix(strings.ToLower(parsed.Host), "ccwu.cc") {
		return nil
	}
	path := strings.TrimPrefix(parsed.EscapedPath(), "/")
	if path == "" {
		return nil
	}
	out := make([]string, 0, len(registryMirrorBases))
	for _, base := range registryMirrorBases {
		out = append(out, base+"/"+path)
	}
	return out
}

// withMirrors 先试原地址，失败后依次试镜像，返回首个成功结果。
func withMirrors[T any](rawURL string, fetch func(string) (T, error)) (T, error) {
	value, err := fetch(rawURL)
	if err == nil {
		return value, nil
	}
	var lastErr error = err
	for _, candidate := range mirrorCandidates(rawURL) {
		mirrored, mirrorErr := fetch(candidate)
		if mirrorErr == nil {
			return mirrored, nil
		}
		lastErr = mirrorErr
	}
	var zero T
	return zero, lastErr
}

func downloadRegistry(rawURL string, max int64) ([]byte, error) {
	return withMirrors(rawURL, func(target string) ([]byte, error) {
		return downloadRegistryOnce(target, max)
	})
}

func downloadModuleZip(rawURL, tempDir string) (string, error) {
	return withMirrors(rawURL, func(target string) (string, error) {
		return downloadModuleZipOnce(target, tempDir)
	})
}

// moduleTempDir 选一个确实可写的临时目录：优先应用私有目录，失败再退回系统临时目录。
func moduleTempDir(dataDir string) string {
	for _, candidate := range []string{dataDir, os.TempDir()} {
		if candidate == "" {
			continue
		}
		if err := os.MkdirAll(candidate, 0o755); err != nil {
			continue
		}
		probe, err := os.CreateTemp(candidate, "tomguo-probe-*")
		if err != nil {
			continue
		}
		name := probe.Name()
		probe.Close()
		os.Remove(name)
		return candidate
	}
	return os.TempDir()
}

func downloadRegistryOnce(rawURL string, max int64) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), registryTimeout)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	response, err := registryHTTPClient.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("HTTP %d", response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, max+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > max {
		return nil, errors.New("清单超出大小上限")
	}
	return data, nil
}

func downloadModuleZipOnce(rawURL, tempDir string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), moduleZipTimeout)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", err
	}
	response, err := registryHTTPClient.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", fmt.Errorf("HTTP %d", response.StatusCode)
	}
	temporary, err := os.CreateTemp(tempDir, "tomguo-module-*.zip")
	if err != nil {
		return "", fmt.Errorf("无法创建临时文件（%s）：%w", tempDir, err)
	}
	defer temporary.Close()
	written, err := io.Copy(temporary, io.LimitReader(response.Body, moduleZipMax+1))
	if err != nil {
		os.Remove(temporary.Name())
		return "", err
	}
	if written > moduleZipMax {
		os.Remove(temporary.Name())
		return "", errors.New("模块包超出大小上限")
	}
	return temporary.Name(), nil
}

// readMainFromZip 从 zip 里取出 main.js，供落盘前摘要校验。
func readMainFromZip(zipPath string) ([]byte, error) {
	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		return nil, errors.New("模块包无法打开")
	}
	defer reader.Close()
	for _, file := range reader.File {
		if filepath.Base(file.Name) != moduleEntryFile {
			continue
		}
		stream, err := file.Open()
		if err != nil {
			return nil, err
		}
		defer stream.Close()
		data, err := io.ReadAll(io.LimitReader(stream, moduleEntryBytes))
		if err != nil {
			return nil, err
		}
		if int64(len(data)) > moduleEntryBytes {
			return nil, errors.New("main.js 超过 4 MB")
		}
		return data, nil
	}
	return nil, errors.New("模块包内没有 main.js")
}

// extractModuleZip 解压包到目标目录，并拦截路径穿越。
func extractModuleZip(zipPath, target string) error {
	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer reader.Close()
	if err := os.MkdirAll(target, 0o700); err != nil {
		return err
	}
	for _, file := range reader.File {
		name := filepath.Clean(filepath.FromSlash(file.Name))
		if name == "." || filepath.IsAbs(name) || strings.HasPrefix(name, "..") {
			return errors.New("压缩包内路径非法")
		}
		destination := filepath.Join(target, name)
		if file.FileInfo().IsDir() {
			if err := os.MkdirAll(destination, 0o700); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
			return err
		}
		if err := extractZipFile(file, destination); err != nil {
			return err
		}
	}
	return nil
}

// RollbackModule 删除指定版本目录，让 moduleActiveDir 回到次新版本。
func RollbackModule(dataDir, id, version string) error {
	id = strings.ToLower(strings.TrimSpace(id))
	if !moduleIDPattern.MatchString(id) {
		return errors.New("模块标识无效")
	}
	if version == "" || strings.Contains(version, "..") || strings.ContainsAny(version, `/\`) {
		return errors.New("版本号无效")
	}
	target := filepath.Join(moduleVersionRoot(dataDir, id), version)
	if err := os.RemoveAll(target); err != nil {
		return err
	}
	_, err := LoadModules(dataDir)
	return err
}
