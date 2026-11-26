package core

import (
	"context"
	"errors"
	"net/http"
	"testing"
)

func TestAggregateModeNormalization(t *testing.T) {
	cases := map[string]string{
		"":        "refresh",
		"instant": "instant",
		"INSTANT": "instant",
		" force ": "force",
		"refresh": "refresh",
		"unknown": "refresh",
	}
	for input, want := range cases {
		if got := aggregateMode(input); got != want {
			t.Errorf("aggregateMode(%q) = %q，期望 %q", input, got, want)
		}
	}
}

func TestAggregateInstantNeverFetches(t *testing.T) {
	// instant 模式下即使缓存为空也必须立即返回，不能触发任何网络请求。
	var requests int
	engine := sourceFixtureEngine(t, func(request *http.Request) (*http.Response, error) {
		requests++
		return nil, errors.New("network forbidden")
	})
	result, err := engine.nativeAggregateCatalog(context.Background(), nativeInput{Mode: "instant"})
	if err != nil {
		t.Fatalf("instant 模式不应返回错误：%v", err)
	}
	if requests != 0 {
		t.Fatalf("instant 模式发起了 %d 次网络请求", requests)
	}
	if !result.Cached {
		t.Fatal("instant 结果应标记 Cached")
	}
}
