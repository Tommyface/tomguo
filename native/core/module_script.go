package core

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"net/http"
	"time"

	"github.com/dop251/goja"
)

// moduleInvoker 把 exports[name] 包成统一调用，进出都是 JSON 字符串。
//
// api 1 模块导出同步函数，api 2 模块导出 async 函数（返回 Promise）。
// 调用方只传一个回调：同步结果立即回调，Promise 结果在 resolve/reject 时回调。
// Go 侧在 eventloop 上运行这段代码，因此 JS 的 await / Promise.all 可以真正并发。
var moduleInvoker = goja.MustCompile("module-invoker", `(function(exports) {
	return function(name, encoded, callback) {
		var target = exports[name];
		if (typeof target !== "function") throw new Error("模块未导出函数 " + name);
		var result;
		try {
			result = target.apply(exports, JSON.parse(encoded));
		} catch (error) {
			callback(null, String(error && error.message ? error.message : error));
			return;
		}
		if (result && typeof result.then === "function") {
			result.then(function(value) {
				callback(JSON.stringify(value === undefined ? null : value), null);
			}, function(error) {
				callback(null, String(error && error.message ? error.message : error));
			});
			return;
		}
		callback(JSON.stringify(result === undefined ? null : result), null);
	};
})`, true)

type jsInstance struct {
	vm     *goja.Runtime
	invoke goja.Callable
	ctx    context.Context
}

// moduleHost 是模块可用的宿主能力：请求与限速。
type moduleHost interface {
	Do(request *http.Request, timeout time.Duration) (*http.Response, error)
}

type jsProvider struct {
	module  *jsModule
	host    moduleHost
	slots   chan struct{}
	idle    chan *jsInstance
	cacheMu sync.Mutex
	cache   map[string]moduleCacheEntry
}

func newJSProvider(module *jsModule) *jsProvider {
	return &jsProvider{
		module: module,
		slots:  make(chan struct{}, moduleInstanceMax),
		idle:   make(chan *jsInstance, moduleInstanceMax),
		cache:  map[string]moduleCacheEntry{},
	}
}

func (p *jsProvider) call(ctx context.Context, function string, result any, arguments ...any) error {
	callCtx, cancel := context.WithTimeout(ctx, moduleCallTimeout)
	defer cancel()
	encoded, err := json.Marshal(arguments)
	if err != nil {
		return err
	}
	instance, err := p.acquire(callCtx)
	if err != nil {
		return err
	}
	defer func() { p.idle <- instance }()
	instance.ctx = callCtx
	interrupted := make(chan struct{})
	stop := context.AfterFunc(callCtx, func() {
		instance.vm.Interrupt(callCtx.Err())
		close(interrupted)
	})
	value, err := instance.invoke(goja.Undefined(), instance.vm.ToValue(function), instance.vm.ToValue(string(encoded)))
	if !stop() {
		<-interrupted
		instance.vm.ClearInterrupt()
	}
	if err != nil {
		return moduleError(callCtx, err)
	}
	return json.Unmarshal([]byte(value.String()), result)
}

func moduleError(ctx context.Context, err error) error {
	var interrupted *goja.InterruptedError
	if errors.As(err, &interrupted) && ctx.Err() != nil {
		return ctx.Err()
	}
	var exception *goja.Exception
	if !errors.As(err, &exception) {
		return err
	}
	if cause := exception.Unwrap(); cause != nil {
		return cause
	}
	if thrown, ok := exception.Value().(*goja.Object); ok {
		if message := thrown.Get("message"); message != nil && !goja.IsUndefined(message) {
			return errors.New(message.String())
		}
	}
	return errors.New(exception.Value().String())
}

func (p *jsProvider) acquire(ctx context.Context) (*jsInstance, error) {
	select {
	case instance := <-p.idle:
		return instance, nil
	default:
	}
	select {
	case instance := <-p.idle:
		return instance, nil
	case p.slots <- struct{}{}:
		instance, err := p.newInstance()
		if err != nil {
			<-p.slots
		}
		return instance, err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (p *jsProvider) newInstance() (*jsInstance, error) {
	vm := goja.New()
	vm.SetFieldNameMapper(goja.TagFieldNameMapper("json", true))
	instance := &jsInstance{vm: vm, ctx: context.Background()}
	module, exports := vm.NewObject(), vm.NewObject()
	if err := module.Set("exports", exports); err != nil {
		return nil, err
	}
	wrapper, err := vm.RunProgram(p.module.program)
	if err != nil {
		return nil, moduleError(instance.ctx, err)
	}
	load, _ := goja.AssertFunction(wrapper)
	if _, err := load(goja.Undefined(), module, exports, p.hostObject(instance)); err != nil {
		return nil, moduleError(instance.ctx, err)
	}
	invoker, err := vm.RunProgram(moduleInvoker)
	if err != nil {
		return nil, err
	}
	bind, _ := goja.AssertFunction(invoker)
	bound, err := bind(goja.Undefined(), module.Get("exports"))
	if err != nil {
		return nil, moduleError(instance.ctx, err)
	}
	instance.invoke, _ = goja.AssertFunction(bound)
	return instance, nil
}

func (p *jsProvider) allowedAddress(raw string) error {
	return nil
}

// 下面是接入 core 各处的调用点。

func (p *jsProvider) Categories(ctx context.Context, force bool) ([]jsCategory, error) {
	var categories []jsCategory
	return categories, p.call(ctx, "categories", &categories, force)
}

func (p *jsProvider) CatalogPage(ctx context.Context, page int, category string) ([]Drama, bool, error) {
	var result jsDramaPage
	if err := p.call(ctx, "catalogPage", &result, page, category); err != nil {
		return nil, false, err
	}
	return result.Items, result.HasMore, nil
}

func (p *jsProvider) Search(ctx context.Context, page int, query string) ([]Drama, bool, error) {
	var result jsDramaPage
	if err := p.call(ctx, "search", &result, page, query); err != nil {
		return nil, false, err
	}
	return result.Items, result.HasMore, nil
}

func (p *jsProvider) Detail(ctx context.Context, sourceID string) (Drama, []Chapter, error) {
	var result jsDetail
	if err := p.call(ctx, "detail", &result, sourceID); err != nil {
		return Drama{}, nil, err
	}
	return result.Drama, result.Chapters, nil
}

func (p *jsProvider) ResolveMedia(ctx context.Context, task Task) (providerMedia, error) {
	var media jsMedia
	if err := p.call(ctx, "resolveMedia", &media, task); err != nil {
		return providerMedia{}, err
	}
	return media.providerMedia(), nil
}
