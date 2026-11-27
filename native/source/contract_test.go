package source

import (
	"context"
	"testing"
	"time"
)

type contractStub struct {
	id       string
	meta     Meta
	cats     []Category
	items    []CatalogItem
	detail   DramaDetail
	stream   MediaStream
	panicsOn string
}

func (s *contractStub) ID() string { return s.id }
func (s *contractStub) Meta() Meta {
	if s.meta.ID == "" {
		return Meta{ID: s.id}
	}
	return s.meta
}
func (s *contractStub) Categories(ctx context.Context, force bool) ([]Category, error) {
	if s.panicsOn == "categories" {
		panic("bad categories")
	}
	return s.cats, nil
}
func (s *contractStub) Catalog(ctx context.Context, page int, category string) ([]CatalogItem, bool, error) {
	if s.panicsOn == "catalog" {
		panic("bad catalog")
	}
	return s.items, false, nil
}
func (s *contractStub) Search(ctx context.Context, page int, query string) ([]CatalogItem, error) {
	if s.panicsOn == "search" {
		panic("bad search")
	}
	return s.items, nil
}
func (s *contractStub) Detail(ctx context.Context, sourceID string) (DramaDetail, error) {
	if s.panicsOn == "detail" {
		panic("bad detail")
	}
	return s.detail, nil
}
func (s *contractStub) Stream(ctx context.Context, request StreamRequest) (MediaStream, error) {
	if s.panicsOn == "stream" {
		panic("bad stream")
	}
	return s.stream, nil
}
func (s *contractStub) Health() Health { return Health{Success: 1} }

func validStub() *contractStub {
	return &contractStub{
		id:     "demo",
		cats:   []Category{{ID: "hot", Name: "热播"}},
		items:  []CatalogItem{{ID: "d1", Title: "作品"}},
		detail: DramaDetail{Item: CatalogItem{ID: "d1", Title: "作品"}},
		stream: MediaStream{URL: "https://cdn.example.com/a.m3u8", Type: "hls"},
	}
}

func TestContractPassesValidSource(t *testing.T) {
	report := ReportContract(context.Background(), validStub(), ContractOptions{})
	if !report.Passed {
		t.Fatalf("合规源应通过：%+v", report)
	}
}

func TestContractDetectsStaticViolations(t *testing.T) {
	s := validStub()
	s.meta = Meta{ID: "other"}
	violations := CheckContract(context.Background(), s, ContractOptions{SkipNetwork: true})
	found := false
	for _, v := range violations {
		if v.Action == "meta" {
			found = true
		}
	}
	if !found {
		t.Fatalf("Meta.ID 不一致应记为违规：%v", violations)
	}
}

func TestContractDetectsDuplicateItems(t *testing.T) {
	s := validStub()
	s.items = []CatalogItem{{ID: "d1", Title: "作品"}, {ID: "d1", Title: "作品"}}
	violations := CheckContract(context.Background(), s, ContractOptions{})
	found := false
	for _, v := range violations {
		if v.Action == "catalog" {
			found = true
		}
	}
	if !found {
		t.Fatalf("重复 id 应记为违规：%v", violations)
	}
}

func TestContractDetectsEmptyIDs(t *testing.T) {
	s := validStub()
	s.cats = []Category{{ID: "", Name: "空分类"}}
	violations := CheckContract(context.Background(), s, ContractOptions{})
	found := false
	for _, v := range violations {
		if v.Action == "categories" {
			found = true
		}
	}
	if !found {
		t.Fatalf("空 id 分类应记为违规：%v", violations)
	}
}

func TestContractRecoversPanic(t *testing.T) {
	for _, action := range []string{"categories", "catalog", "search", "detail", "stream"} {
		s := validStub()
		s.panicsOn = action
		violations := CheckContract(context.Background(), s, ContractOptions{})
		found := false
		for _, v := range violations {
			if v.Action == "panic" {
				found = true
			}
		}
		if !found {
			t.Errorf("%s panic 应被记为违规：%v", action, violations)
		}
	}
}

func TestContractNullSource(t *testing.T) {
	violations := CheckContract(context.Background(), nil, ContractOptions{})
	if len(violations) == 0 {
		t.Fatal("空源应记为违规")
	}
}

func TestRequiredExports(t *testing.T) {
	base := RequiredExports(nil)
	if len(base) != 4 {
		t.Fatalf("基础导出数 %d，期望 4（%v）", len(base), base)
	}
	withSearch := RequiredExports([]string{CapabilitySearch})
	if len(withSearch) != 5 {
		t.Fatalf("声明 search 后导出数 %d，期望 5（%v）", len(withSearch), withSearch)
	}
}

func TestContractRunsAgainstAdapter(t *testing.T) {
	handler := &stubHandler{
		items:  []CatalogItem{{ID: "d1", Title: "作品"}},
		detail: DramaDetail{Item: CatalogItem{ID: "d1", Title: "作品"}},
		stream: MediaStream{URL: "https://cdn.example.com/a.m3u8", Type: "hls"},
	}
	adapter := NewAdapter(Meta{ID: "demo", Name: "演示"}, handler, Policy{Timeout: time.Second, Attempts: 1})
	report := ReportContract(context.Background(), adapter, ContractOptions{Timeout: time.Second})
	if !report.Passed {
		t.Fatalf("合规适配器应通过契约：%+v", report)
	}
}
