package core

// custom_sources.go
//
// 运行时可配置站源注册表。
//
// 设计目标：把站源从「编译期硬编码」变为「运行时数据」，让新增苹果CMS /
// 参数化 AES 站源无需重新编译 Go 核心（.so）与重新发版。
//
// 数据来源：Dart 侧通过 `registerCustomSources` op 注入 JSON 配置；配置在
// 本地持久化，App 冷启动时重新注入。
//
// 能力边界（如实声明）：
//   - 苹果CMS 类（type=maccms）：路由规律、解析逻辑由现有通用解析器承担，
//     配置只需描述「差异」（详情/播放路由正则、各页 URL 模板）。真正免构建。
//   - 参数化 AES 类（type=aes）：覆盖「AES-CBC 解密响应 + 固定签名拼接」
//     这一常见形态，key/iv/签名模板可配。算法非标（如带 gzip、动态 key、
//     自定义签名）的站仍需专属代码 —— UI 层会如实提示，避免误导。
//
// 所有自定义 source id 统一使用 `custom_` 前缀，避免与内置源冲突。

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
)

const customSourcePrefix = "custom_"

// customSourceConfig 是 Dart 与 Go 共用的配置 schema。
//
// 字段命名与 JSON tag 保持一致，Dart 侧按同名字段序列化。
type customSourceConfig struct {
	ID          string `json:"id"`                    // 唯一 id，建议 custom_ 前缀；缺省时按 name 生成
	Name        string `json:"name"`                  // 展示名
	Description string `json:"description,omitempty"` // 简介
	BaseURL     string `json:"baseUrl"`               // 站点根地址
	Type        string `json:"type"`                  // "maccms" | "aes"

	// ---- 苹果CMS 参数 ----
	DetailPathPattern string `json:"detailPathPattern,omitempty"` // 详情页 path 正则，含 1 个捕获组取 ID
	PlayPathPattern   string `json:"playPathPattern,omitempty"`   // 播放页 path 正则
	DetailURLTemplate string `json:"detailUrlTemplate,omitempty"` // 详情 URL 模板，{id} 占位
	ListURLTemplate   string `json:"listUrlTemplate,omitempty"`   // 目录 URL 模板，{page} 占位
	SearchURLTemplate string `json:"searchUrlTemplate,omitempty"` // 搜索 URL 模板，{query} {page} 占位

	// ---- AES 参数 ----
	APIURL       string `json:"apiUrl,omitempty"`       // API 根地址，缺省用 baseUrl
	AESKey       string `json:"aesKey,omitempty"`       // 明文密钥（UTF-8 字符串）
	AESIV        string `json:"aesIv,omitempty"`        // IV（明文字符串）；为空时用全 0 IV
	KeyIsHex     bool   `json:"keyIsHex,omitempty"`     // key/iv 是否为 hex 编码
	SignKey      string `json:"signKey,omitempty"`      // 签名盐
	SignTemplate string `json:"signTemplate,omitempty"` // 签名拼接模板，如 "{path}&{timestamp}&{signKey}"，依次 SHA256→MD5
	CatalogPath  string `json:"catalogPath,omitempty"`  // 目录接口 path
	DetailEP     string `json:"detailPath,omitempty"`   // 详情接口 path（与 maccms 的 DetailPathPattern 区分）
	PlayPath     string `json:"playApiPath,omitempty"`  // 播放接口 path
}

// customRegistry 是 Downloader 上的运行时注册表。用 mutex 保护，支持热更新。
type customRegistry struct {
	mu     sync.RWMutex
	byID   map[string]customSourceConfig
	byHost map[string]string // hostname -> source id
}

func newCustomRegistry() *customRegistry {
	return &customRegistry{
		byID:   make(map[string]customSourceConfig),
		byHost: make(map[string]string),
	}
}

// replace 原子替换全部配置（Dart 每次注入传全量，天然支持增删改）。
func (r *customRegistry) replace(configs []customSourceConfig) (int, error) {
	next := make(map[string]customSourceConfig, len(configs))
	hosts := make(map[string]string, len(configs))
	for _, cfg := range configs {
		normalized, err := normalizeCustomConfig(cfg)
		if err != nil {
			return 0, fmt.Errorf("站源 %q 配置无效: %w", cfg.Name, err)
		}
		next[normalized.ID] = normalized
		if u, err := url.Parse(normalized.BaseURL); err == nil && u.Hostname() != "" {
			hosts[strings.ToLower(u.Hostname())] = normalized.ID
		}
	}
	r.mu.Lock()
	r.byID = next
	r.byHost = hosts
	r.mu.Unlock()
	return len(next), nil
}

func (r *customRegistry) get(id string) (customSourceConfig, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	cfg, ok := r.byID[strings.ToLower(strings.TrimSpace(id))]
	return cfg, ok
}

func (r *customRegistry) sourceForHost(host string) string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.byHost[strings.ToLower(strings.TrimSpace(host))]
}

func (r *customRegistry) list() []customSourceConfig {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]customSourceConfig, 0, len(r.byID))
	for _, cfg := range r.byID {
		out = append(out, cfg)
	}
	return out
}

// maccmsProfile 把运行时的苹果CMS配置转换为解析器可用的 profile。
// URL 模板中的占位符：{base} 站点根、{id} 剧集ID、{page} 页码、{query} 搜索词。
func (c customSourceConfig) maccmsProfile() (maccmsProfile, error) {
	if c.Type != "maccms" {
		return maccmsProfile{}, errors.New("非苹果CMS源")
	}
	detailRe, err := regexp.Compile(c.DetailPathPattern)
	if err != nil {
		return maccmsProfile{}, err
	}
	var playRe *regexp.Regexp
	if c.PlayPathPattern != "" {
		playRe, err = regexp.Compile(c.PlayPathPattern)
		if err != nil {
			return maccmsProfile{}, err
		}
	}
	fill := func(tmpl string, page int, query string) string {
		s := strings.ReplaceAll(tmpl, "{base}", c.BaseURL)
		s = strings.ReplaceAll(s, "{page}", strconv.Itoa(page))
		s = strings.ReplaceAll(s, "{query}", url.QueryEscape(query))
		return s
	}
	profile := maccmsProfile{
		Source:     c.ID,
		Name:       c.Name,
		Base:       c.BaseURL,
		DetailPath: detailRe,
		PlayPath:   playRe,
	}
	profile.DetailURL = func(base, id string) string {
		s := strings.ReplaceAll(c.DetailURLTemplate, "{base}", base)
		return strings.ReplaceAll(s, "{id}", id)
	}
	profile.ListURL = func(base string, page int) string {
		return fill(c.ListURLTemplate, page, "")
	}
	if c.SearchURLTemplate != "" {
		profile.SearchURL = func(base, query string, page int) string {
			return fill(c.SearchURLTemplate, page, query)
		}
	}
	return profile, nil
}

// normalizeCustomConfig 校验并补全配置，返回规范化副本。
func normalizeCustomConfig(cfg customSourceConfig) (customSourceConfig, error) {
	cfg.Name = strings.TrimSpace(cfg.Name)
	cfg.BaseURL = strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	cfg.Type = strings.ToLower(strings.TrimSpace(cfg.Type))
	if cfg.Name == "" {
		return cfg, errors.New("名称不能为空")
	}
	if cfg.BaseURL == "" {
		return cfg, errors.New("站点地址不能为空")
	}
	parsed, err := url.Parse(cfg.BaseURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return cfg, errors.New("站点地址必须是完整的 http(s) URL")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return cfg, errors.New("站点地址仅支持 http/https")
	}
	if cfg.ID == "" {
		cfg.ID = customSourcePrefix + slugify(cfg.Name)
	}
	if !strings.HasPrefix(cfg.ID, customSourcePrefix) {
		cfg.ID = customSourcePrefix + strings.TrimPrefix(cfg.ID, customSourcePrefix)
	}
	cfg.ID = strings.ToLower(strings.TrimSpace(cfg.ID))
	switch cfg.Type {
	case "maccms":
		if cfg.DetailPathPattern == "" {
			return cfg, errors.New("苹果CMS 源缺少详情路由正则")
		}
		if _, err := regexp.Compile(cfg.DetailPathPattern); err != nil {
			return cfg, fmt.Errorf("详情路由正则无效: %w", err)
		}
		if cfg.PlayPathPattern != "" {
			if _, err := regexp.Compile(cfg.PlayPathPattern); err != nil {
				return cfg, fmt.Errorf("播放路由正则无效: %w", err)
			}
		}
		if cfg.DetailURLTemplate == "" {
			cfg.DetailURLTemplate = "{base}{id}"
		}
		if cfg.ListURLTemplate == "" {
			cfg.ListURLTemplate = "{base}"
		}
	case "aes":
		if cfg.AESKey == "" {
			return cfg, errors.New("AES 源缺少密钥")
		}
		if cfg.APIURL == "" {
			cfg.APIURL = cfg.BaseURL
		}
	default:
		return cfg, fmt.Errorf("未知站源类型 %q（仅支持 maccms/aes）", cfg.Type)
	}
	return cfg, nil
}

func slugify(name string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(name) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	s := strings.Trim(b.String(), "_")
	if s == "" {
		return "site"
	}
	return s
}

// isCustomSource 判断一个 source id 是否为运行时动态源。
func isCustomSource(source string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(source)), customSourcePrefix)
}

// decodeCustomSources 解析 Dart 注入的 JSON。兼容 {"sources":[...]} 与裸数组两种形态。
func decodeCustomSources(raw json.RawMessage) ([]customSourceConfig, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	trimmed := strings.TrimSpace(string(raw))
	var configs []customSourceConfig
	if strings.HasPrefix(trimmed, "[") {
		if err := json.Unmarshal(raw, &configs); err != nil {
			return nil, err
		}
		return configs, nil
	}
	var wrapper struct {
		Sources []customSourceConfig `json:"sources"`
	}
	if err := json.Unmarshal(raw, &wrapper); err != nil {
		return nil, err
	}
	return wrapper.Sources, nil
}
