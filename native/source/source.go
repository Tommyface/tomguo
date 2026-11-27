package source

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"
)

// Handler 是源实现必须提供的五个数据动作，由适配器补齐生命周期与策略。
type Handler interface {
	Categories(ctx context.Context, force bool) ([]Category, error)
	Catalog(ctx context.Context, page int, category string) ([]CatalogItem, bool, error)
	Search(ctx context.Context, page int, query string) ([]CatalogItem, bool, error)
	Detail(ctx context.Context, sourceID string) (DramaDetail, error)
	Stream(ctx context.Context, request StreamRequest) (MediaStream, error)
}

// Source 是适配层对外暴露的源接口。
type Source interface {
	ID() string
	Meta() Meta
	Categories(ctx context.Context, force bool) ([]Category, error)
	Catalog(ctx context.Context, page int, category string) ([]CatalogItem, bool, error)
	Search(ctx context.Context, page int, query string) ([]CatalogItem, error)
	Detail(ctx context.Context, sourceID string) (DramaDetail, error)
	Stream(ctx context.Context, request StreamRequest) (MediaStream, error)
	Health() Health
}

// Adapter 用统一策略包装任意 Handler。
type Adapter struct {
	meta    Meta
	handler Handler
	policy  Policy
	limiter limiter

	mu     sync.Mutex
	health Health
}

func NewAdapter(meta Meta, handler Handler, policy Policy) *Adapter {
	policy = policy.normalized()
	return &Adapter{
		meta:    meta,
		handler: handler,
		policy:  policy,
		limiter: limiter{interval: policy.Interval},
	}
}

func (a *Adapter) ID() string { return a.meta.ID }

func (a *Adapter) Meta() Meta { return a.meta }

func (a *Adapter) Health() Health {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.health
}

func (a *Adapter) record(err error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.health.LastAt = time.Now()
	if err != nil {
		a.health.Failure++
		a.health.LastError = err.Error()
		return
	}
	a.health.Success++
	a.health.LastError = ""
}

func (a *Adapter) Categories(ctx context.Context, force bool) ([]Category, error) {
	value, err := invoke(ctx, a.policy, &a.limiter, "categories", func(callCtx context.Context) ([]Category, error) {
		return a.handler.Categories(callCtx, force)
	})
	a.record(err)
	return value, err
}

func (a *Adapter) Catalog(ctx context.Context, page int, category string) ([]CatalogItem, bool, error) {
	if page < 1 {
		page = 1
	}
	type outcome struct {
		items []CatalogItem
		more  bool
	}
	value, err := invoke(ctx, a.policy, &a.limiter, "catalog", func(callCtx context.Context) (outcome, error) {
		items, more, callErr := a.handler.Catalog(callCtx, page, category)
		return outcome{items: items, more: more}, callErr
	})
	a.record(err)
	if err != nil {
		return nil, false, err
	}
	return value.items, value.more, nil
}

func (a *Adapter) Search(ctx context.Context, page int, query string) ([]CatalogItem, error) {
	if page < 1 {
		page = 1
	}
	value, err := invoke(ctx, a.policy, &a.limiter, "search", func(callCtx context.Context) ([]CatalogItem, error) {
		items, _, callErr := a.handler.Search(callCtx, page, query)
		return items, callErr
	})
	a.record(err)
	return value, err
}

func (a *Adapter) Detail(ctx context.Context, sourceID string) (DramaDetail, error) {
	value, err := invoke(ctx, a.policy, &a.limiter, "detail", func(callCtx context.Context) (DramaDetail, error) {
		return a.handler.Detail(callCtx, sourceID)
	})
	a.record(err)
	return value, err
}

func (a *Adapter) Stream(ctx context.Context, request StreamRequest) (MediaStream, error) {
	value, err := invoke(ctx, a.policy, &a.limiter, "stream", func(callCtx context.Context) (MediaStream, error) {
		return a.handler.Stream(callCtx, request)
	})
	a.record(err)
	return value, err
}

// Registry 保存已注册的源，读写并发安全。
type Registry struct {
	mu      sync.RWMutex
	sources map[string]Source
	runners map[string]*Runner
}

func NewRegistry() *Registry {
	return &Registry{sources: map[string]Source{}, runners: map[string]*Runner{}}
}

// Runner 返回该源的策略执行器，按需创建并复用，保证限速与健康统计连续。
func (r *Registry) Runner(id string) *Runner {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.runners == nil {
		r.runners = map[string]*Runner{}
	}
	if runner, ok := r.runners[id]; ok {
		return runner
	}
	runner := NewRunner(DefaultPolicy())
	r.runners[id] = runner
	return runner
}

func (r *Registry) Register(item Source) error {
	if item == nil {
		return fmt.Errorf("注册源失败：空实现")
	}
	id := item.ID()
	if id == "" {
		return fmt.Errorf("注册源失败：id 为空")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.sources == nil {
		r.sources = map[string]Source{}
	}
	r.sources[id] = item
	return nil
}

func (r *Registry) Unregister(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.sources, id)
}

func (r *Registry) Lookup(id string) (Source, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	item, ok := r.sources[id]
	return item, ok
}

func (r *Registry) IDs() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	ids := make([]string, 0, len(r.sources))
	for id := range r.sources {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func (r *Registry) List() []Source {
	ids := r.IDs()
	r.mu.RLock()
	defer r.mu.RUnlock()
	items := make([]Source, 0, len(ids))
	for _, id := range ids {
		if item, ok := r.sources[id]; ok {
			items = append(items, item)
		}
	}
	return items
}

func (r *Registry) Len() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.sources)
}
