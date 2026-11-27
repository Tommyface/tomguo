package source

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

type stubHandler struct {
	categories   []Category
	items        []CatalogItem
	more         bool
	detail       DramaDetail
	stream       MediaStream
	catalogCalls int32
	catalogErr   error
	failTimes    int32
	delay        time.Duration
}

func (s *stubHandler) Categories(ctx context.Context, force bool) ([]Category, error) {
	return s.categories, nil
}

func (s *stubHandler) Catalog(ctx context.Context, page int, category string) ([]CatalogItem, bool, error) {
	atomic.AddInt32(&s.catalogCalls, 1)
	if s.delay > 0 {
		select {
		case <-ctx.Done():
			return nil, false, Transient("catalog", ctx.Err())
		case <-time.After(s.delay):
		}
	}
	if s.failTimes > 0 {
		atomic.AddInt32(&s.failTimes, -1)
		return nil, false, Transient("catalog", errors.New("临时失败"))
	}
	return s.items, s.more, s.catalogErr
}

func (s *stubHandler) Search(ctx context.Context, page int, query string) ([]CatalogItem, bool, error) {
	return s.items, false, nil
}

func (s *stubHandler) Detail(ctx context.Context, sourceID string) (DramaDetail, error) {
	return s.detail, nil
}

func (s *stubHandler) Stream(ctx context.Context, request StreamRequest) (MediaStream, error) {
	return s.stream, nil
}

func newTestAdapter(handler Handler, policy Policy) *Adapter {
	return NewAdapter(Meta{ID: "demo", Name: "演示"}, handler, policy)
}

func TestAdapterCatalogRetriesTransient(t *testing.T) {
	handler := &stubHandler{items: []CatalogItem{{ID: "a"}}, failTimes: 1}
	adapter := newTestAdapter(handler, Policy{Timeout: time.Second, Attempts: 2, Backoff: time.Millisecond})
	items, more, err := adapter.Catalog(context.Background(), 1, "")
	if err != nil {
		t.Fatalf("重试后应成功：%v", err)
	}
	if len(items) != 1 || more {
		t.Fatalf("结果不符：%v more=%v", items, more)
	}
	if calls := atomic.LoadInt32(&handler.catalogCalls); calls != 2 {
		t.Fatalf("调用次数 %d，期望 2", calls)
	}
	if rate := adapter.Health().SuccessRate(); rate != 1 {
		t.Fatalf("成功率 %f，期望 1", rate)
	}
}

func TestAdapterPermanentErrorNotRetried(t *testing.T) {
	handler := &stubHandler{catalogErr: Permanent("catalog", errors.New("永久失败"))}
	adapter := newTestAdapter(handler, Policy{Timeout: time.Second, Attempts: 3, Backoff: time.Millisecond})
	if _, _, err := adapter.Catalog(context.Background(), 1, ""); err == nil {
		t.Fatal("应返回错误")
	}
	if calls := atomic.LoadInt32(&handler.catalogCalls); calls != 1 {
		t.Fatalf("永久错误调用 %d 次，期望 1", calls)
	}
	if health := adapter.Health(); health.Failure != 1 || health.LastError == "" {
		t.Fatalf("健康记录不符：%+v", health)
	}
}

func TestAdapterTimeout(t *testing.T) {
	handler := &stubHandler{delay: 200 * time.Millisecond}
	adapter := newTestAdapter(handler, Policy{Timeout: 30 * time.Millisecond, Attempts: 1})
	start := time.Now()
	if _, _, err := adapter.Catalog(context.Background(), 1, ""); err == nil {
		t.Fatal("超时应返回错误")
	}
	if elapsed := time.Since(start); elapsed > 150*time.Millisecond {
		t.Fatalf("超时未生效，耗时 %v", elapsed)
	}
}

func TestAdapterRateLimitInterval(t *testing.T) {
	handler := &stubHandler{}
	adapter := newTestAdapter(handler, Policy{Timeout: time.Second, Attempts: 1, Interval: 60 * time.Millisecond})
	start := time.Now()
	for i := 0; i < 3; i++ {
		if _, _, err := adapter.Catalog(context.Background(), 1, ""); err != nil {
			t.Fatalf("第 %d 次失败：%v", i+1, err)
		}
	}
	if elapsed := time.Since(start); elapsed < 100*time.Millisecond {
		t.Fatalf("限速未生效，3 次耗时仅 %v", elapsed)
	}
}

func TestAdapterCapabilityGate(t *testing.T) {
	adapter := NewAdapter(Meta{ID: "demo", Capabilities: []string{CapabilityCatalog}}, &stubHandler{}, DefaultPolicy())
	if _, err := adapter.Search(context.Background(), 1, "q"); err == nil {
		t.Fatal("未声明搜索能力应拒绝")
	} else if IsTransient(err) {
		t.Fatal("能力缺失应为永久错误，不重试")
	}
	if _, err := adapter.Stream(context.Background(), StreamRequest{}); err == nil {
		t.Fatal("未声明播放能力应拒绝")
	}
	if _, _, err := adapter.Catalog(context.Background(), 0, ""); err != nil {
		t.Fatalf("已声明目录能力应放行：%v", err)
	}
}

func TestRegistryLifecycle(t *testing.T) {
	registry := NewRegistry()
	first := NewAdapter(Meta{ID: "b"}, &stubHandler{}, DefaultPolicy())
	second := NewAdapter(Meta{ID: "a"}, &stubHandler{}, DefaultPolicy())
	if err := registry.Register(first); err != nil {
		t.Fatalf("注册失败：%v", err)
	}
	if err := registry.Register(second); err != nil {
		t.Fatalf("注册失败：%v", err)
	}
	if err := registry.Register(nil); err == nil {
		t.Fatal("空实现应注册失败")
	}
	if err := registry.Register(NewAdapter(Meta{}, &stubHandler{}, DefaultPolicy())); err == nil {
		t.Fatal("空 id 应注册失败")
	}
	ids := registry.IDs()
	if len(ids) != 2 || ids[0] != "a" || ids[1] != "b" {
		t.Fatalf("id 列表不符：%v", ids)
	}
	if item, ok := registry.Lookup("a"); !ok || item.ID() != "a" {
		t.Fatal("按 id 查找失败")
	}
	registry.Unregister("a")
	if registry.Len() != 1 {
		t.Fatalf("注销后数量 %d，期望 1", registry.Len())
	}
	if _, ok := registry.Lookup("a"); ok {
		t.Fatal("注销后不应再查到")
	}
}

func TestMetaSupports(t *testing.T) {
	if !(Meta{}).Supports(CapabilityStream) {
		t.Fatal("未声明能力时按全能力处理")
	}
	meta := Meta{Capabilities: []string{CapabilityCatalog, CapabilityDetail}}
	if meta.Supports(CapabilityStream) {
		t.Fatal("未声明播放能力不应通过")
	}
	if !meta.Supports(CapabilityDetail) {
		t.Fatal("已声明详情能力应通过")
	}
}

func TestHealthSuccessRate(t *testing.T) {
	if rate := (Health{}).SuccessRate(); rate != 0 {
		t.Fatalf("无记录成功率 %f，期望 0", rate)
	}
	health := Health{Success: 3, Failure: 1}
	if rate := health.SuccessRate(); rate != 0.75 {
		t.Fatalf("成功率 %f，期望 0.75", rate)
	}
}
