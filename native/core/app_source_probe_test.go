package core

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRetiredSourcesAreMarkedDisabled(t *testing.T) {
	engine := &nativeEngine{sourceProbes: map[string]nativeSourceProbe{}}
	items := engine.listModules()
	byID := map[string]moduleStatus{}
	for _, item := range items {
		byID[item.ID] = item
	}
	for _, id := range []string{sourceHuangguoAI, sourceHuangguoVideo, sourceYeguo, sourceDSD, sourceSorani, sourceChengguo} {
		status, found := byID[id]
		if !found {
			t.Fatalf("source %q missing from module list", id)
		}
		if !status.Disabled || status.DisabledReason == "" {
			t.Fatalf("source %q should be disabled, got %#v", id, status)
		}
	}
	if status := byID[sourceHongguo]; status.Disabled {
		t.Fatalf("healthy source marked disabled: %#v", status)
	}
}

func TestProbeSourcesHidesUnreachableSource(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		http.Error(writer, "gone", http.StatusServiceUnavailable)
	}))
	defer server.Close()
	engine := &nativeEngine{sourceProbes: map[string]nativeSourceProbe{}, downloader: &Downloader{client: server.Client()}}
	probes := engine.probeSources(context.Background(), []string{sourceHongguo}, true)
	if len(probes) != 1 || probes[0].Source != sourceHongguo {
		t.Fatalf("probes = %#v", probes)
	}
	engine.sourceProbes[sourceHongguo] = nativeSourceProbe{Source: sourceHongguo, Reachable: false, Error: "HTTP 503", CheckedAt: probes[0].CheckedAt}
	reachable, reason := engine.sourceReachable(sourceHongguo)
	if reachable || reason == "" {
		t.Fatalf("unreachable source should be hidden, got %v %q", reachable, reason)
	}
}

func TestRetiredSourceCatalogIsRejected(t *testing.T) {
	engine := &nativeEngine{catalogs: map[string][]nativeDrama{}, catalogStates: map[string]nativeCatalogState{}, sourceProbes: map[string]nativeSourceProbe{}}
	_, err := engine.nativeCatalog(context.Background(), nativeInput{Source: sourceHuangguoAI, Page: 1})
	if err == nil || !strings.Contains(err.Error(), "停用") {
		t.Fatalf("retired source should be rejected, got %v", err)
	}
}
