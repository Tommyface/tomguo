package core

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// 模块清单（registry）：由构建脚本生成，用于在安装前完成分流与完整性校验。
// 与 manifest.json 的关系：清单是「市场」，manifest 是「商品标签」，
// 两者都必须声明 mainSha256，且必须一致。

// moduleRegistryAPIVersion 是清单格式版本，独立于模块接口版本 moduleAPIVersion。
const moduleRegistryAPIVersion = 2

// moduleRegistryEntry 是清单里的一条模块记录。
type moduleRegistryEntry struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Version      string   `json:"version"`
	Description  string   `json:"description"`
	MinAPI       int      `json:"minApi"`
	Adult        *bool    `json:"adult"`
	Capabilities []string `json:"capabilities"`
	Hosts        []string `json:"hosts"`
	MainSha256   string   `json:"mainSha256"`
	Zip          string   `json:"zip"`
	Size         int64    `json:"size"`

	// schema 2 的可新增性声明。零值保持旧清单的行为（全部可安装）。
	Available     *bool  `json:"available"`
	State         string `json:"state"`
	Reason        string `json:"reason"`
	MinAppVersion string `json:"minAppVersion"`
	ReplacedBy    string `json:"replacedBy"`
}

// moduleRegistryList 是一份清单。
type moduleRegistryList struct {
	Name        string                `json:"name"`
	Description string                `json:"description"`
	API         int                   `json:"api"`
	Entries     []moduleRegistryEntry `json:"plugins"`
	// Modules 兼容以 modules 作为键的历史写法。
	Modules []moduleRegistryEntry `json:"modules"`
}

// ParseModuleRegistry 解析并校验清单。任一条不合法即整体失败：
// 清单是安装入口，容错会让坏条目以「看起来正常」的形态流进模块目录。
func ParseModuleRegistry(raw []byte) (*moduleRegistryList, error) {
	var list moduleRegistryList
	if err := json.Unmarshal(raw, &list); err != nil {
		return nil, fmt.Errorf("模块清单无法解析：%w", err)
	}
	if len(list.Modules) > 0 {
		list.Entries = append(list.Entries, list.Modules...)
	}
	list.Modules = nil
	if list.API != 0 && list.API > moduleRegistryAPIVersion {
		return nil, fmt.Errorf("模块清单接口版本为 %d，当前应用支持 %d，请先升级应用", list.API, moduleRegistryAPIVersion)
	}
	if len(list.Entries) == 0 {
		return nil, fmt.Errorf("模块清单为空")
	}
	seen := make(map[string]bool, len(list.Entries))
	for index := range list.Entries {
		entry := &list.Entries[index]
		entry.ID = strings.ToLower(strings.TrimSpace(entry.ID))
		entry.MainSha256 = strings.ToLower(strings.TrimSpace(entry.MainSha256))
		if !moduleIDPattern.MatchString(entry.ID) {
			return nil, fmt.Errorf("清单第 %d 条的标识不合法：%q", index+1, entry.ID)
		}
		if seen[entry.ID] {
			return nil, fmt.Errorf("清单里 %q 重复出现", entry.ID)
		}
		seen[entry.ID] = true
		if strings.TrimSpace(entry.Name) == "" {
			return nil, fmt.Errorf("清单里的 %q 缺少名称", entry.ID)
		}
		if entry.Adult == nil {
			return nil, fmt.Errorf("清单里的 %q 缺少 adult 声明", entry.ID)
		}
		if len(entry.MainSha256) != moduleDigestHexLen {
			return nil, fmt.Errorf("清单里的 %q mainSha256 必须是 64 位十六进制", entry.ID)
		}
		if _, err := hex.DecodeString(entry.MainSha256); err != nil {
			return nil, fmt.Errorf("清单里的 %q mainSha256 不是合法的十六进制", entry.ID)
		}
		if strings.TrimSpace(entry.Zip) == "" {
			return nil, fmt.Errorf("清单里的 %q 缺少下载地址", entry.ID)
		}
		if entry.State != "" && !validRegistryState(entry.State) {
			return nil, fmt.Errorf("清单里的 %q state 不合法：%q", entry.ID, entry.State)
		}
		if entry.State == "retired" || entry.State == "blocked" || (entry.Available != nil && !*entry.Available) {
			if strings.TrimSpace(entry.Reason) == "" {
				return nil, fmt.Errorf("清单里的 %q 不可安装但缺少原因说明", entry.ID)
			}
		}
		if strings.TrimSpace(entry.ReplacedBy) != "" && !moduleIDPattern.MatchString(strings.ToLower(strings.TrimSpace(entry.ReplacedBy))) {
			return nil, fmt.Errorf("清单里的 %q replacedBy 标识不合法", entry.ID)
		}
	}
	sort.SliceStable(list.Entries, func(i, j int) bool { return list.Entries[i].ID < list.Entries[j].ID })
	return &list, nil
}

// EntriesFiltered 按成人内容开关返回可安装条目。
// includeAdult 为 false 时，成人条目在安装前就被挡掉，不需要先下载再判断。
func (list *moduleRegistryList) EntriesFiltered(includeAdult bool) []moduleRegistryEntry {
	if list == nil {
		return nil
	}
	out := make([]moduleRegistryEntry, 0, len(list.Entries))
	for _, entry := range list.Entries {
		isAdult := entry.Adult != nil && *entry.Adult
		if isAdult && !includeAdult {
			continue
		}
		if !entry.CanInstall() {
			continue
		}
		out = append(out, entry)
	}
	return out
}

// FindEntry 按 id 查条目。
func (list *moduleRegistryList) FindEntry(id string) (moduleRegistryEntry, bool) {
	if list == nil {
		return moduleRegistryEntry{}, false
	}
	id = strings.ToLower(strings.TrimSpace(id))
	for _, entry := range list.Entries {
		if entry.ID == id {
			return entry, true
		}
	}
	return moduleRegistryEntry{}, false
}

// VerifyEntryCode 用清单里的摘要校验即将安装的 main.js。
// 安装流程必须在写入模块目录之前调用它，否则先落盘再校验等于已信任外部字节。
func VerifyEntryCode(entry moduleRegistryEntry, code []byte) error {
	if strings.TrimSpace(entry.MainSha256) == "" {
		return moduleDigestError(entry.ID, "清单缺少 mainSha256，拒绝安装")
	}
	_, err := verifyModuleDigest(moduleManifest{ID: entry.ID, MainSha256: entry.MainSha256}, code)
	return err
}

// validRegistryState 返回清单支持的源状态。
func validRegistryState(state string) bool {
	switch strings.ToLower(strings.TrimSpace(state)) {
	case "active", "deprecated", "retired", "blocked":
		return true
	default:
		return false
	}
}

// CanInstall 判断清单条目是否允许新增安装。
// 缺省的 available=true 与 state="" 保持旧清单行为。
func (entry moduleRegistryEntry) CanInstall() bool {
	if entry.Available != nil && !*entry.Available {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(entry.State)) {
	case "retired", "blocked":
		return false
	default:
		return true
	}
}

// StateLabel 返回给用户看的状态说明；空串表示正常。
func (entry moduleRegistryEntry) StateLabel() string {
	if text := strings.TrimSpace(entry.Reason); text != "" {
		return text
	}
	switch strings.ToLower(strings.TrimSpace(entry.State)) {
	case "deprecated":
		return "即将停用"
	case "retired":
		return "已下架"
	case "blocked":
		return "已停用"
	default:
		return ""
	}
}
