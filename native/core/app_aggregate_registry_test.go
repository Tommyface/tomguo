package core

import (
	"context"
	"testing"

	"duanjuapp/native/source"
)

func TestAggregateSourceIDsFromRegistry(t *testing.T) {
	withCatalog := source.NewAdapter(source.Meta{
		ID: "agg-a", Capabilities: []string{source.CapabilityCatalog},
	}, bridgeStubHandler{}, source.DefaultPolicy())
	emptyCatalog := source.NewAdapter(source.Meta{
		ID: "agg-empty", Capabilities: []string{source.CapabilityCatalog}, AllowEmptyCatalog: true,
	}, bridgeStubHandler{}, source.DefaultPolicy())
	extraOnly := source.NewAdapter(source.Meta{
		ID: "agg-extra", Capabilities: []string{"cover"},
	}, bridgeStubHandler{}, source.DefaultPolicy())
	for _, item := range []source.Source{withCatalog, emptyCatalog, extraOnly} {
		if err := sourceAdapters.Register(item); err != nil {
			t.Fatalf("注册失败：%v", err)
		}
		defer sourceAdapters.Unregister(item.ID())
	}

	ids := aggregateSourceIDs()
	found := map[string]bool{}
	for _, id := range ids {
		found[id] = true
	}
	if !found["agg-a"] {
		t.Fatalf("有目录能力的源应参与聚合：%v", ids)
	}
	if found["agg-empty"] {
		t.Fatalf("allowEmptyCatalog 的源不应参与聚合：%v", ids)
	}
	if !found["agg-extra"] {
		t.Fatalf("capabilities 只列额外能力的源仍应参与聚合：%v", ids)
	}
}

func TestAggregateSourceIDsDeduplicates(t *testing.T) {
	item := source.NewAdapter(source.Meta{ID: "agg-dup", Capabilities: []string{source.CapabilityCatalog}}, bridgeStubHandler{}, source.DefaultPolicy())
	if err := sourceAdapters.Register(item); err != nil {
		t.Fatalf("注册失败：%v", err)
	}
	defer sourceAdapters.Unregister("agg-dup")
	ids := aggregateSourceIDs()
	count := 0
	for _, id := range ids {
		if id == "agg-dup" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("agg-dup 出现 %d 次，期望 1（%v）", count, ids)
	}
}

func TestAggregateCatalogOnceRecordsHealth(t *testing.T) {
	engine := &nativeEngine{}
	item := source.NewAdapter(source.Meta{ID: "agg-health", Capabilities: []string{source.CapabilityCatalog}}, bridgeStubHandler{}, source.DefaultPolicy())
	if err := sourceAdapters.Register(item); err != nil {
		t.Fatalf("注册失败：%v", err)
	}
	defer sourceAdapters.Unregister("agg-health")

	// 适配器已注册但没有同名模块，nativeCatalog 会返回错误；
	// 这里验证错误经策略执行器后确实落到健康统计上。
	if _, err := engine.aggregateCatalogOnce(context.Background(), "agg-health", nativeInput{Source: "agg-health", Page: 1}); err == nil {
		t.Fatal("应返回错误")
	}
	health := sourceAdapters.Runner("agg-health").Health()
	if health.Failure == 0 || health.LastError == "" {
		t.Fatalf("健康统计未记录失败：%+v", health)
	}
	if health.Success != 0 {
		t.Fatalf("不应记录成功：%+v", health)
	}
}

func TestAggregateRunnerReusedPerSource(t *testing.T) {
	first := sourceAdapters.Runner("runner-demo")
	second := sourceAdapters.Runner("runner-demo")
	if first != second {
		t.Fatal("同一源的执行器应复用，否则限速与健康统计会断裂")
	}
	if sourceAdapters.Runner("runner-other") == first {
		t.Fatal("不同源应使用不同执行器")
	}
}
