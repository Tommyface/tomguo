package core

import "strings"

// sourceSpec 是一个内置站源的静态描述。
//
// 这里是整个应用**唯一**的内置源清单：新增一个内置源必须在这张表里加一条，
// 而不是在分发函数里加一个 case。S1 之后，分发、别名解析、能力判断都查这张表。
//
// 阶段 S7 完成后，这张表会被云端清单取代（表里的每一项都变成一个 zip 模块），
// 届时本文件只剩 lookupSourceSpec 与 moduleRegistry 的联合查询。
type sourceSpec struct {
	ID     string
	Name   string
	Adults bool
	// Aliases 是除 ID 本身之外所有可被 canonicalProviderSource 识别的写法，
	// 包括域名与中文别名。匹配前统一小写。
	Aliases []string
	// Capabilities 声明该源支持的能力：catalog / detail / stream / search。
	Capabilities []string
}

// builtinSourceSpecs 是内置源清单。顺序不重要，查询走 map。
var builtinSourceSpecs = []sourceSpec{}

var capsAll = []string{"catalog", "detail", "stream", "search"}

var (
	sourceSpecByID    = map[string]*sourceSpec{}
	sourceSpecByAlias = map[string]*sourceSpec{}
)

func init() {
	for index := range builtinSourceSpecs {
		spec := &builtinSourceSpecs[index]
		sourceSpecByID[spec.ID] = spec
		for _, alias := range spec.Aliases {
			key := strings.ToLower(strings.TrimSpace(alias))
			if key == "" {
				continue
			}
			sourceSpecByAlias[key] = spec
		}
	}
}

// lookupSourceSpec 按 id 或别名查内置源。传入值会被小写并去空白。
func lookupSourceSpec(source string) (*sourceSpec, bool) {
	key := strings.ToLower(strings.TrimSpace(source))
	if key == "" {
		return nil, false
	}
	if spec, ok := sourceSpecByID[key]; ok {
		return spec, true
	}
	spec, ok := sourceSpecByAlias[key]
	return spec, ok
}

// builtinSourceIDs 返回全部内置源 id，供自检与清单比对使用。
func builtinSourceIDs() []string {
	ids := make([]string, 0, len(builtinSourceSpecs))
	for _, spec := range builtinSourceSpecs {
		ids = append(ids, spec.ID)
	}
	return ids
}

// supportsCapability 判断一个内置源是否声明了某项能力。
func supportsBuiltinCapability(source, capability string) bool {
	spec, ok := lookupSourceSpec(source)
	if !ok {
		return false
	}
	for _, item := range spec.Capabilities {
		if item == capability {
			return true
		}
	}
	return false
}
