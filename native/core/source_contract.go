package core

import (
	"context"
	"time"

	"duanjuapp/native/source"
)

// RunSourceContracts 对当前已注册的每个源跑统一契约检查。
// 供导入前验收与 probe 自检使用，不经过 IPC 动作面。
func RunSourceContracts(ctx context.Context, timeout time.Duration) []source.ContractReport {
	reports := make([]source.ContractReport, 0, sourceAdapters.Len())
	for _, item := range sourceAdapters.List() {
		reports = append(reports, source.ReportContract(ctx, item, source.ContractOptions{Timeout: timeout}))
	}
	return reports
}
