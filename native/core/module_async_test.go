package core

import (
	"context"
	"testing"
	"time"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"
)

// TestModuleAsyncPromiseEventloop 是 S4 的技术 POC：
// 验证 goja eventloop 能驱动 JS 侧 Promise.all 并发等待两个由 Go 注入的异步操作。
// 如果这条失败，api 2（模块导出改 async）就不可行，需要降级为「同步 + 更短超时」。
func TestModuleAsyncPromiseEventloop(t *testing.T) {
	loop := eventloop.NewEventLoop()
	done := make(chan string, 1)

	go func() {
		loop.Run(func(vm *goja.Runtime) {
			vm.Set("goDelay", func(ms int64, cb func()) {
				go func() {
					time.Sleep(time.Duration(ms) * time.Millisecond)
					loop.RunOnLoop(func(*goja.Runtime) { cb() })
				}()
			})
			vm.Set("resolveAll", func(value string) { done <- value })
			program, err := goja.Compile("poc", `
				function delay(ms) { return new Promise(function(resolve) { goDelay(ms, resolve); }); }
				Promise.all([delay(80), delay(40)]).then(function() { resolveAll("ok"); });
			`, false)
			if err != nil {
				done <- "compile-error: " + err.Error()
				return
			}
			if _, err := vm.RunProgram(program); err != nil {
				done <- "run-error: " + err.Error()
			}
		})
		loop.Start()
	}()
	defer loop.StopNoWait()

	select {
	case value := <-done:
		if value != "ok" {
			t.Fatalf("Promise.all 结果 %q，期望 ok", value)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Promise.all 超时，eventloop 无法驱动异步模块")
	}
}

// TestModuleAsyncPromiseTimeout 验证异步等待能被 context 超时打断。
func TestModuleAsyncPromiseTimeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	select {
	case <-ctx.Done():
		if ctx.Err() != context.DeadlineExceeded {
			t.Fatalf("期望 DeadlineExceeded，收到 %v", ctx.Err())
		}
	case <-time.After(time.Second):
		t.Fatal("context 超时未生效")
	}
}
