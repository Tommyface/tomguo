package core

import (
	"context"
	"testing"
	"time"

	"duanjuapp/native/source"
)

func TestRunSourceContracts(t *testing.T) {
	adapter := source.NewAdapter(source.Meta{ID: "contract-demo"}, bridgeStubHandler{}, source.DefaultPolicy())
	if err := sourceAdapters.Register(adapter); err != nil {
		t.Fatalf("注册失败：%v", err)
	}
	defer sourceAdapters.Unregister("contract-demo")
	reports := RunSourceContracts(context.Background(), time.Second)
	if len(reports) == 0 {
		t.Fatal("应返回契约报告")
	}
	found := false
	for _, report := range reports {
		if report.ID == "contract-demo" {
			found = true
			if !report.Passed {
				t.Fatalf("合规适配器应通过：%+v", report)
			}
		}
	}
	if !found {
		t.Fatal("未找到 contract-demo 的报告")
	}
}
