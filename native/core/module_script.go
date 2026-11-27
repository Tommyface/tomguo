package core

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"
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
	loop   *eventloop.EventLoop
	dead   uint32
}

func (i *jsInstance) retire()      { atomic.StoreUint32(&i.dead, 1) }
func (i *jsInstance) isDead() bool { return atomic.LoadUint32(&i.dead) != 0 }

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

// call 通过 eventloop 驱动模块导出。
//
// 同步模块（api 1）与异步模块（api 2）共用 moduleInvoker 的 callback 协议：
// 两者都会在回调里返回 (jsonText, errorText)，因此调用方不关心模块是哪种协议。
// eventloop 让 JS 的 await / Promise.all 真正可并发，超时则通过 Interrupt 打断。
var errModuleLoopDead = errors.New("模块运行环境已停止")

func (p *jsProvider) call(ctx context.Context, function string, result any, arguments ...any) error {
	callCtx, cancel := context.WithTimeout(ctx, moduleCallTimeout)
	defer cancel()
	encoded, err := json.Marshal(arguments)
	if err != nil {
		return err
	}
	for attempt := 0; attempt < moduleCallRetries; attempt++ {
		instance, err := p.acquire(callCtx)
		if err != nil {
			return err
		}
		err = p.callOnInstance(callCtx, instance, function, result, encoded)
		if errors.Is(err, errModuleLoopDead) {
			instance.retire()
			continue
		}
		return err
	}
	return errModuleLoopDead
}

func (p *jsProvider) callOnInstance(callCtx context.Context, instance *jsInstance, function string, result any, encoded []byte) error {
	// 只有 loop 仍存活时才把实例放回空闲池；超时后 loop 已 Terminate，
	// 放回池里会让下一次调用拿到死实例。
	reusable := false
	defer func() {
		if reusable && !instance.isDead() {
			instance.ctx = context.Background()
			p.idle <- instance
		} else {
			<-p.slots
		}
	}()
	instance.ctx = callCtx

	type callbackResult struct {
		jsonText string
		errText  string
	}
	done := make(chan callbackResult, 1)
	started := make(chan struct{})
	queued := instance.loop.RunOnLoop(func(vm *goja.Runtime) {
		close(started)
		_, invokeErr := instance.invoke(goja.Undefined(),
			vm.ToValue(function), vm.ToValue(string(encoded)),
			vm.ToValue(func(jsonValue, errorValue goja.Value) {
				text := ""
				if jsonValue != nil && !goja.IsUndefined(jsonValue) && !goja.IsNull(jsonValue) {
					text = jsonValue.String()
				}
				message := ""
				if errorValue != nil && !goja.IsUndefined(errorValue) && !goja.IsNull(errorValue) {
					message = errorValue.String()
				}
				select {
				case done <- callbackResult{jsonText: text, errText: message}:
				default:
				}
			}))
		if invokeErr != nil {
			select {
			case done <- callbackResult{errText: invokeErr.Error()}:
			default:
			}
		}
	})
	if !queued {
		return errModuleLoopDead
	}

	select {
	case <-callCtx.Done():
		instance.retire()
		instance.loop.Terminate()
		return callCtx.Err()
	case <-started:
	}
	select {
	case <-callCtx.Done():
		instance.retire()
		instance.loop.Terminate()
		return callCtx.Err()
	case got := <-done:
		reusable = true
		if got.errText != "" {
			return errors.New(got.errText)
		}
		return json.Unmarshal([]byte(got.jsonText), result)
	}
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
	for {
		var idle *jsInstance
		select {
		case instance := <-p.idle:
			idle = instance
		default:
		}
		if idle != nil {
			if !idle.isDead() {
				return idle, nil
			}
			<-p.slots
			continue
		}
		select {
		case instance := <-p.idle:
			if !instance.isDead() {
				return instance, nil
			}
			<-p.slots
			continue
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
}

func (p *jsProvider) newInstance() (*jsInstance, error) {
	loop := eventloop.NewEventLoop()
	instance := &jsInstance{ctx: context.Background(), loop: loop}
	ready := make(chan error, 1)
	go func() {
		loop.Run(func(vm *goja.Runtime) {
			instance.vm = vm
			vm.SetFieldNameMapper(goja.TagFieldNameMapper("json", true))
			installModuleGlobals(vm, p, instance)
			module, exports := vm.NewObject(), vm.NewObject()
			if err := module.Set("exports", exports); err != nil {
				ready <- err
				return
			}
			wrapper, err := vm.RunProgram(p.module.program)
			if err != nil {
				ready <- moduleError(instance.ctx, err)
				return
			}
			load, _ := goja.AssertFunction(wrapper)
			if _, err := load(goja.Undefined(), module, exports, p.hostObject(instance)); err != nil {
				ready <- moduleError(instance.ctx, err)
				return
			}
			invoker, err := vm.RunProgram(moduleInvoker)
			if err != nil {
				ready <- err
				return
			}
			bind, _ := goja.AssertFunction(invoker)
			bound, err := bind(goja.Undefined(), module.Get("exports"))
			if err != nil {
				ready <- moduleError(instance.ctx, err)
				return
			}
			instance.invoke, _ = goja.AssertFunction(bound)
			ready <- nil
		})
		loop.Start()
	}()
	if err := <-ready; err != nil {
		loop.StopNoWait()
		return nil, err
	}
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
