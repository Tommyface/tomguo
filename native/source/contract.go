package source

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"
)

// RequiredActions 是模块必须导出的四个基础动作。
// search 属于可选能力：manifest 声明 search 时才强制要求导出。
var RequiredActions = []string{
	"categories",
	"catalogPage",
	"detail",
	"resolveMedia",
}

// ConditionalActions 是按需强制的导出：能力声明里出现时才要求。
var ConditionalActions = map[string]string{
	"search": CapabilitySearch,
}

// RequiredExports 返回给定能力声明下必须存在的导出集合。
func RequiredExports(capabilities []string) []string {
	out := append([]string{}, RequiredActions...)
	for action, capability := range ConditionalActions {
		for _, item := range capabilities {
			if item == capability {
				out = append(out, action)
				break
			}
		}
	}
	return out
}

// ContractViolation 是一条契约违规记录。
type ContractViolation struct {
	Action string `json:"action"`
	Reason string `json:"reason"`
}

func (v ContractViolation) String() string {
	return v.Action + ": " + v.Reason
}

// ContractOptions 控制运行时契约的执行强度。
type ContractOptions struct {
	Timeout time.Duration
	// SkipNetwork 为真时只跑不联网的契约项（标识、元信息、Health）。
	SkipNetwork bool
}

// CheckStaticContract 校验源实现在不联网前提下必须满足的契约。
func CheckStaticContract(item Source) []ContractViolation {
	violations := []ContractViolation{}
	if item == nil {
		return append(violations, ContractViolation{Action: "id", Reason: "源实现为空"})
	}
	id := strings.TrimSpace(item.ID())
	if id == "" {
		violations = append(violations, ContractViolation{Action: "id", Reason: "标识为空"})
	}
	meta := item.Meta()
	if meta.ID != id {
		violations = append(violations, ContractViolation{Action: "meta", Reason: fmt.Sprintf("Meta.ID=%q 与 ID()=%q 不一致", meta.ID, id)})
	}
	return violations
}

// CheckContract 对任意 Source 实现跑统一契约用例。
// 每个动作都用 recover 兜底：panic 一律记为违规，不能让一个坏源拖垮调用方。
func CheckContract(ctx context.Context, item Source, opts ContractOptions) []ContractViolation {
	violations := CheckStaticContract(item)
	if len(violations) > 0 && item == nil {
		return violations
	}
	if opts.Timeout <= 0 {
		opts.Timeout = 5 * time.Second
	}
	checkAction := func(action string, fn func() []ContractViolation) {
		if ctx.Err() != nil {
			return
		}
		callCtx, cancel := context.WithTimeout(ctx, opts.Timeout)
		defer cancel()
		found := safeCheck(callCtx, fn)
		violations = append(violations, found...)
	}
	if !opts.SkipNetwork {
		checkAction("categories", func() []ContractViolation {
			items, err := item.Categories(ctx, false)
			if err != nil {
				return nil
			}
			seen := map[string]bool{}
			out := []ContractViolation{}
			for _, category := range items {
				if category.ID == "" {
					out = append(out, ContractViolation{Action: "categories", Reason: "返回了空 id 的分类"})
				}
				if seen[category.ID] {
					out = append(out, ContractViolation{Action: "categories", Reason: "分类 id 重复：" + category.ID})
				}
				seen[category.ID] = true
			}
			return out
		})
		checkAction("catalog", func() []ContractViolation {
			items, _, err := item.Catalog(ctx, 0, "")
			if err != nil {
				return nil
			}
			return checkCatalogItems("catalog", items)
		})
		checkAction("search", func() []ContractViolation {
			items, err := item.Search(ctx, 0, "契约测试")
			if err != nil {
				return nil
			}
			return checkCatalogItems("search", items)
		})
		checkAction("detail", func() []ContractViolation {
			detail, err := item.Detail(ctx, "契约测试")
			if err != nil {
				return nil
			}
			out := []ContractViolation{}
			if detail.Item.ID == "" {
				out = append(out, ContractViolation{Action: "detail", Reason: "成功返回但 item.id 为空"})
			}
			return out
		})
		checkAction("stream", func() []ContractViolation {
			stream, err := item.Stream(ctx, StreamRequest{})
			if err != nil {
				return nil
			}
			out := []ContractViolation{}
			if stream.URL == "" {
				out = append(out, ContractViolation{Action: "stream", Reason: "成功返回但 url 为空"})
			}
			if stream.Type != "" && stream.Type != "hls" && stream.Type != "mp4" {
				out = append(out, ContractViolation{Action: "stream", Reason: "未知播放类型 " + stream.Type})
			}
			return out
		})
	}
	checkAction("health", func() []ContractViolation {
		health := item.Health()
		if health.SuccessRate() < 0 || health.SuccessRate() > 1 {
			return []ContractViolation{{Action: "health", Reason: "成功率越界"}}
		}
		return nil
	})
	sort.SliceStable(violations, func(i, j int) bool { return violations[i].Action < violations[j].Action })
	return violations
}

func checkCatalogItems(action string, items []CatalogItem) []ContractViolation {
	out := []ContractViolation{}
	seen := map[string]bool{}
	for _, item := range items {
		if item.ID == "" {
			out = append(out, ContractViolation{Action: action, Reason: "返回了空 id 的条目"})
			continue
		}
		if seen[item.ID] {
			out = append(out, ContractViolation{Action: action, Reason: "条目 id 重复：" + item.ID})
		}
		seen[item.ID] = true
		if item.Title == "" {
			out = append(out, ContractViolation{Action: action, Reason: "条目 " + item.ID + " 标题为空"})
		}
	}
	return out
}

func safeCheck(ctx context.Context, fn func() []ContractViolation) (out []ContractViolation) {
	defer func() {
		if recovered := recover(); recovered != nil {
			out = append(out, ContractViolation{Action: "panic", Reason: fmt.Sprint(recovered)})
		}
	}()
	return fn()
}

// ContractReport 汇总一次契约检查，供导入前判定与日志使用。
type ContractReport struct {
	ID         string              `json:"id"`
	Passed     bool                `json:"passed"`
	Violations []ContractViolation `json:"violations,omitempty"`
}

func ReportContract(ctx context.Context, item Source, opts ContractOptions) ContractReport {
	violations := CheckContract(ctx, item, opts)
	id := ""
	if item != nil {
		id = item.ID()
	}
	return ContractReport{ID: id, Passed: len(violations) == 0, Violations: violations}
}
